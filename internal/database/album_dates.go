package database

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Publication dates are calendar dates, stored as ISO text to avoid timezone
// conversions when reading them from either SQLite or PostgreSQL.
var albumPublicationDatePattern = regexp.MustCompile(`[0-9]{4}(?:[-._/][0-9]{1,2}[-._/][0-9]{1,2}| +[0-9]{1,2} +[0-9]{1,2}|年[0-9]{1,2}月[0-9]{1,2}日?|[0-9]{4})|[0-9]{2}-[0-9]{1,2}-[0-9]{1,2}`)
var albumDateSeparators = strings.NewReplacer(".", "-", "_", "-", "/", "-", "年", "-", "月", "-", "日", "")
var albumPublicationLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// AlbumPublicationDate uses the first valid full date in a title, or the scan
// date in Beijing time when the title contains no valid date.
func AlbumPublicationDate(title string, scannedAt time.Time) string {
	if date := publicationDateFromTitle(title); date != "" {
		return date
	}
	return scannedAt.In(albumPublicationLocation).Format("2006-01-02")
}

func publicationDateFromTitle(title string) string {
	for _, match := range albumPublicationDatePattern.FindAllStringIndex(title, -1) {
		start, end := match[0], match[1]
		if start > 0 && isDateDigit(title[start-1]) {
			continue
		}
		if end < len(title) && isDateDigit(title[end-1]) && isDateDigit(title[end]) {
			continue
		}
		candidate := albumDateSeparators.Replace(title[start:end])
		candidate = strings.Join(strings.Fields(candidate), "-")
		var parts []string
		if len(candidate) == 8 && !strings.Contains(candidate, "-") {
			parts = []string{candidate[:4], candidate[4:6], candidate[6:]}
		} else {
			parts = strings.Split(candidate, "-")
		}
		if len(parts) != 3 {
			continue
		}
		year, _ := strconv.Atoi(parts[0])
		// Short hyphenated years always refer to 2000–2099.
		if len(parts[0]) == 2 {
			year += 2000
		}
		month, _ := strconv.Atoi(parts[1])
		day, _ := strconv.Atoi(parts[2])
		if year < 1 || month < 1 || month > 12 || day < 1 || day > 31 {
			continue
		}
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if date.Year() == year && int(date.Month()) == month && date.Day() == day {
			return date.Format("2006-01-02")
		}
	}
	return ""
}

func isDateDigit(value byte) bool { return value >= '0' && value <= '9' }

func (album *Album) BeforeCreate(tx *gorm.DB) error {
	if album.PublishedDate == "" {
		scannedAt := album.CreatedAt
		if scannedAt.IsZero() {
			scannedAt = tx.NowFunc()
		}
		album.PublishedDate = AlbumPublicationDate(album.AlbumName, scannedAt)
	}
	return nil
}

// ResolveAlbumPublicationDate prefers the album title, then the first dated image
// filename in lexical order, then the earliest image modification time.
// Stream records to avoid loading a large album into memory.
func ResolveAlbumPublicationDate(db *gorm.DB, album Album, now time.Time) (string, error) {
	if date := publicationDateFromTitle(album.AlbumName); date != "" {
		return date, nil
	}
	rows, err := db.Model(&Photo{}).Select("photo_name", "modified_at").
		Where("album_id = ? AND media_type = ? AND status = ?", album.ID, "image", "active").
		Order("photo_name ASC, photo_path ASC, id ASC").Rows()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var earliest time.Time
	for rows.Next() {
		var photo Photo
		if err := db.ScanRows(rows, &photo); err != nil {
			return "", err
		}
		if date := publicationDateFromTitle(photo.PhotoName); date != "" {
			return date, nil
		}
		if !photo.ModifiedAt.IsZero() && (earliest.IsZero() || photo.ModifiedAt.Before(earliest)) {
			earliest = photo.ModifiedAt
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if !earliest.IsZero() {
		return AlbumPublicationDate("", earliest), nil
	}
	return AlbumPublicationDate("", now), nil
}

// Only fill empty values so later restarts cannot change saved dates.
func backfillAlbumPublicationDates(db *gorm.DB) error {
	var lastID uint
	for {
		var albums []Album
		if err := db.Select("id", "album_name", "created_at").
			Where("id > ? AND (published_date = ? OR published_date IS NULL)", lastID, "").
			Order("id ASC").Limit(200).Find(&albums).Error; err != nil {
			return err
		}
		if len(albums) == 0 {
			return nil
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			for _, album := range albums {
				value, err := ResolveAlbumPublicationDate(tx, album, tx.NowFunc())
				if err != nil {
					return fmt.Errorf("resolve album %d publication date: %w", album.ID, err)
				}
				if err := tx.Model(&Album{}).Where("id = ? AND (published_date = ? OR published_date IS NULL)", album.ID, "").UpdateColumn("published_date", value).Error; err != nil {
					return fmt.Errorf("backfill album %d publication date: %w", album.ID, err)
				}
			}
			return nil
		}); err != nil {
			return err
		}
		lastID = albums[len(albums)-1].ID
	}
}
