package database

import (
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
)

func TestRefreshEntityImageIDs(t *testing.T) {
	db, err := Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "entity-images.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	directory := AssetDirectory{DirectoryName: "library", DirectoryPath: "/library", ScanStatus: true}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	series := Series{Name: "Collection"}
	if err := db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	albums := []Album{
		{DirectoryID: directory.ID, SeriesID: &series.ID, AlbumName: "older", AlbumPath: "/library/older", ScanStatus: true, CreatedAt: now.Add(-time.Hour)},
		{DirectoryID: directory.ID, SeriesID: &series.ID, AlbumName: "newer", AlbumPath: "/library/newer", ScanStatus: true, CreatedAt: now},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	olderThumbnail := "/library/older/.thumb/01.webp"
	newerThumbnail := "/library/newer/.thumb/01.webp"
	photos := []Photo{
		{AlbumID: albums[0].ID, PhotoName: "01.jpg", PhotoPath: "/library/older/01.jpg", ThumbnailPath: &olderThumbnail, MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: now, LastSeenAt: now},
		{AlbumID: albums[1].ID, PhotoName: "01.jpg", PhotoPath: "/library/newer/01.jpg", ThumbnailPath: &newerThumbnail, MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: now, LastSeenAt: now},
	}
	if err := db.Create(&photos).Error; err != nil {
		t.Fatal(err)
	}
	person := Person{Name: "Alice"}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	for _, album := range albums {
		if err := db.Create(&PersonAlbum{PersonID: person.ID, AlbumID: album.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := RefreshAllEntityImageIDs(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&person, person.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&series, series.ID).Error; err != nil {
		t.Fatal(err)
	}
	if person.AvatarPhotoID == nil || *person.AvatarPhotoID != photos[1].ID {
		t.Fatalf("expected newest album photo %d as avatar, got %v", photos[1].ID, person.AvatarPhotoID)
	}
	if series.CoverPhotoID == nil || *series.CoverPhotoID != photos[1].ID {
		t.Fatalf("expected newest album photo %d as cover, got %v", photos[1].ID, series.CoverPhotoID)
	}

	if err := db.Model(&photos[1]).Update("status", "missing").Error; err != nil {
		t.Fatal(err)
	}
	if err := RefreshEntityImageIDsForAlbumIDs(db, []uint{albums[1].ID}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&person, person.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&series, series.ID).Error; err != nil {
		t.Fatal(err)
	}
	if person.AvatarPhotoID == nil || *person.AvatarPhotoID != photos[0].ID {
		t.Fatalf("expected avatar to fall back to %d, got %v", photos[0].ID, person.AvatarPhotoID)
	}
	if series.CoverPhotoID == nil || *series.CoverPhotoID != photos[0].ID {
		t.Fatalf("expected cover to fall back to %d, got %v", photos[0].ID, series.CoverPhotoID)
	}
}
