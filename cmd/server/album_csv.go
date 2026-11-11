package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"airy/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const albumCSVLimit = 5 * 1024 * 1024

var albumCSVLegacyHeader = []string{"相册ID", "相册标题", "发布日期"}
var albumCSVHeader = []string{"相册ID", "相册标题", "发布日期", "系列", "模特"}

type albumCSVRow struct {
	OldSeries    string   `json:"old_series"`
	Series       string   `json:"series"`
	OldPeople    []string `json:"old_people"`
	People       []string `json:"people"`
	OldSeriesID  *uint    `json:"old_series_id"`
	SeriesID     *uint    `json:"series_id"`
	OldPersonIDs []uint   `json:"old_person_ids"`
	PersonIDs    []uint   `json:"person_ids"`
	NewPeople    []string `json:"new_people"`
	CreateSeries bool     `json:"create_series"`
	UpdateSeries bool     `json:"update_series"`
	UpdatePeople bool     `json:"update_people"`
	Line         int      `json:"line"`
	ID           uint     `json:"id"`
	OldTitle     string   `json:"old_title"`
	Title        string   `json:"title"`
	OldDate      string   `json:"old_date"`
	Date         string   `json:"date"`
	Error        string   `json:"error"`
	Changed      bool     `json:"changed"`
}
type albumCSVPreview struct {
	Rows    []albumCSVRow `json:"rows"`
	Changes int           `json:"changes"`
	Valid   bool          `json:"valid"`
	Token   string        `json:"token"`
}

func csvDirectory(c *gin.Context, db *gorm.DB) (uint, bool) {
	// Global CSV routes have no directory parameter.
	if c.Param("id") == "" {
		return 0, true
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(400, gin.H{"error": "资源目录 ID 无效"})
		return 0, false
	}
	var directory database.AssetDirectory
	if err := db.First(&directory, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"error": "资源目录不存在"})
		} else {
			c.JSON(500, gin.H{"error": "读取资源目录失败"})
		}
		return 0, false
	}
	return uint(id), true
}

// Prevent a title from becoming a spreadsheet formula. Unchanged escaped titles
// are mapped back to their stored value during import.
func spreadsheetTitle(title string) string {
	trimmed := strings.TrimLeft(title, " \t\r\n")
	if strings.HasPrefix(title, "'") || (len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + title
	}
	return title
}

func exportAlbumCSVHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := db.WithContext(c.Request.Context())
		id, ok := csvDirectory(c, query)
		if !ok {
			return
		}
		var albums []database.Album
		if err := csvAlbumScope(query, id).Select("id", "album_name", "published_date", "series_id").Order("id ASC").Find(&albums).Error; err != nil {
			c.JSON(500, gin.H{"error": "读取相册失败"})
			return
		}
		ids := make([]uint, 0, len(albums))
		for _, album := range albums {
			ids = append(ids, album.ID)
		}
		entities, err := loadCSVEntities(query, ids)
		if err != nil {
			c.JSON(500, gin.H{"error": "读取系列和模特失败"})
			return
		}
		var output bytes.Buffer
		output.WriteString("\xef\xbb\xbf")
		writer := csv.NewWriter(&output)
		writer.UseCRLF = true
		_ = writer.Write(albumCSVHeader)
		for _, album := range albums {
			_ = writer.Write([]string{strconv.FormatUint(uint64(album.ID), 10), spreadsheetTitle(album.AlbumName), album.PublishedDate, spreadsheetTitle(entities.seriesName(album.SeriesID)), spreadsheetTitle(csvPeopleText(entities.personNames(entities.links[album.ID])))})
		}
		writer.Flush()
		if writer.Error() != nil {
			c.JSON(500, gin.H{"error": "生成 CSV 失败"})
			return
		}
		filename := "airy-albums.csv"
		if id != 0 {
			filename = fmt.Sprintf("airy-albums-%d.csv", id)
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/csv; charset=utf-8", output.Bytes())
	}
}

func previewAlbumCSV(db *gorm.DB, directoryID uint, data []byte) (albumCSVPreview, error) {
	preview := albumCSVPreview{Rows: []albumCSVRow{}, Valid: true}
	if !utf8.Valid(data) {
		return preview, errors.New("请使用 UTF-8 编码的 CSV 文件")
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil || (!slices.Equal(header, albumCSVHeader) && !slices.Equal(header, albumCSVLegacyHeader)) {
		return preview, errors.New("CSV 表头须为：相册ID,相册标题,发布日期,系列,模特（也兼容旧版三列表头）")
	}
	reader.FieldsPerRecord = len(header)
	records := make([][]string, 0)
	lines := make([]int, 0)
	ids := make([]uint, 0)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return preview, fmt.Errorf("CSV 格式错误：%w", err)
		}
		if len(records) >= 20000 {
			return preview, errors.New("每次最多导入 20000 行，请拆分文件")
		}
		line, _ := reader.FieldPos(0)
		records = append(records, record)
		lines = append(lines, line)
		id, parseErr := strconv.ParseUint(strings.TrimSpace(record[0]), 10, 32)
		if parseErr != nil {
			id = 0
		}
		ids = append(ids, uint(id))
	}
	if len(records) == 0 {
		return preview, errors.New("CSV 没有相册数据")
	}
	albums := make(map[uint]database.Album)
	// Bound SQL parameter counts for SQLite as well as PostgreSQL.
	for start := 0; start < len(ids); start += 200 {
		end := min(start+200, len(ids))
		var batch []database.Album
		if err := csvAlbumScope(db, directoryID).Select("id", "album_name", "published_date", "series_id").Where("id IN ?", ids[start:end]).Find(&batch).Error; err != nil {
			return preview, err
		}
		for _, album := range batch {
			albums[album.ID] = album
		}
	}
	entities, err := loadCSVEntities(db, ids)
	if err != nil {
		return preview, err
	}
	counts := make(map[uint]int)
	for _, id := range ids {
		counts[id]++
	}
	for i, record := range records {
		row := albumCSVRow{Line: lines[i], ID: ids[i], Title: record[1], Date: strings.TrimSpace(record[2])}
		album, exists := albums[row.ID]
		row.OldTitle = album.AlbumName
		row.OldDate = album.PublishedDate
		if exists && row.Title == spreadsheetTitle(album.AlbumName) {
			row.Title = album.AlbumName
		}
		if row.Date == "" {
			row.Date = album.PublishedDate
		}
		var issues []string
		if row.ID == 0 {
			issues = append(issues, "相册 ID 必须为正整数")
		} else if counts[row.ID] > 1 {
			issues = append(issues, "相册 ID 重复")
		}
		if row.ID != 0 && !exists {
			if directoryID == 0 {
				issues = append(issues, "相册不存在")
			} else {
				issues = append(issues, "相册不存在或不属于当前资源目录")
			}
		}
		if strings.TrimSpace(row.Title) == "" {
			issues = append(issues, "相册标题不能为空")
		}
		if utf8.RuneCountInString(row.Title) > 255 {
			issues = append(issues, "相册标题不能超过 255 个字符")
		}
		if strings.ContainsRune(row.Title, 0) {
			issues = append(issues, "标题包含无效字符")
		}
		if record[2] != "" && strings.TrimSpace(record[2]) != "" {
			date, err := time.Parse("2006-01-02", row.Date)
			if err != nil || date.Year() < 1 || date.Format("2006-01-02") != row.Date {
				issues = append(issues, "发布日期必须为有效的 YYYY-MM-DD 日期")
			}
		}
		issues = append(issues, entities.fillRow(&row, album, record)...)
		row.Error = strings.Join(issues, "；")
		row.Changed = exists && (row.Title != row.OldTitle || row.Date != row.OldDate || row.CreateSeries || !equalSeriesID(row.SeriesID, row.OldSeriesID) || len(row.NewPeople) > 0 || !slices.Equal(row.PersonIDs, row.OldPersonIDs))
		if row.Error != "" {
			preview.Valid = false
		}
		if row.Changed {
			preview.Changes++
		}
		preview.Rows = append(preview.Rows, row)
	}
	snapshot, _ := json.Marshal(struct {
		DirectoryID uint
		Rows        []albumCSVRow
	}{directoryID, preview.Rows})
	sum := sha256.Sum256(snapshot)
	preview.Token = hex.EncodeToString(sum[:])
	return preview, nil
}

func importAlbumCSVHandler(db *gorm.DB, apply bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := db.WithContext(c.Request.Context())
		id, ok := csvDirectory(c, query)
		if !ok {
			return
		}
		c.Header("Cache-Control", "no-store")
		data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, albumCSVLimit))
		if err != nil {
			c.JSON(413, gin.H{"error": "CSV 文件不能超过 5 MB"})
			return
		}
		status := http.StatusBadRequest
		var preview albumCSVPreview
		err = query.Transaction(func(tx *gorm.DB) error {
			var err error
			preview, err = previewAlbumCSV(tx, id, data)
			if err != nil {
				return err
			}
			if !apply {
				return nil
			}
			if !preview.Valid {
				return errors.New("CSV 存在错误，请修正后重新导入")
			}
			if c.GetHeader("X-Preview-Token") == "" || c.GetHeader("X-Preview-Token") != preview.Token {
				status = 409
				return errors.New("相册信息已变化，请重新导入预览后再保存")
			}
			for _, row := range preview.Rows {
				if !row.Changed {
					continue
				}
				result := csvAlbumScope(tx, id).Model(&database.Album{}).Where("id = ? AND album_name = ? AND published_date = ?", row.ID, row.OldTitle, row.OldDate).
					Updates(map[string]any{"album_name": row.Title, "published_date": row.Date})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					status = 409
					return errors.New("相册信息已变化，全部修改已撤销，请重新预览")
				}
			}
			return applyCSVEntities(tx, preview.Rows)
		})
		if err != nil {
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, preview)
	}
}

func csvAlbumScope(db *gorm.DB, directoryID uint) *gorm.DB {
	if directoryID != 0 {
		return db.Where("directory_id = ?", directoryID)
	}
	return db
}
