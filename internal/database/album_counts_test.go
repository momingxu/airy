package database

import (
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
)

func TestAlbumCountsBackfillAndRefresh(t *testing.T) {
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "counts.db")})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	directory := AssetDirectory{DirectoryName: "library", DirectoryPath: "/library"}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	album := Album{DirectoryID: directory.ID, AlbumName: "album", AlbumPath: "/library/album", PublishedDate: "2020-01-01"}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	photos := []Photo{
		{AlbumID: album.ID, PhotoName: "one.jpg", PhotoPath: "/one.jpg", MediaType: "image", Status: "active", ModifiedAt: now, LastSeenAt: now, MIMEType: "image/jpeg"},
		{AlbumID: album.ID, PhotoName: "two.mp4", PhotoPath: "/two.mp4", MediaType: "video", Status: "active", ModifiedAt: now, LastSeenAt: now, MIMEType: "video/mp4"},
		{AlbumID: album.ID, PhotoName: "gone.jpg", PhotoPath: "/gone.jpg", MediaType: "image", Status: "missing", ModifiedAt: now, LastSeenAt: now, MIMEType: "image/jpeg"},
	}
	if err := db.Create(&photos).Error; err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"image_count", "video_count"} {
		if err := db.Exec("ALTER TABLE albums ADD COLUMN " + column + " integer NOT NULL DEFAULT 0").Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"image_count", "video_count"} {
		if db.Migrator().HasColumn(&Album{}, column) {
			t.Fatalf("obsolete column %s retained", column)
		}
	}
	var got Album
	if err := db.First(&got, album.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.MediaCount != 2 || got.CountsUpdatedAt == nil {
		t.Fatalf("unexpected counts %+v", got)
	}
	if !got.UpdatedAt.Equal(album.UpdatedAt) || got.PublishedDate != album.PublishedDate {
		t.Fatal("backfill changed album metadata")
	}
	if err := db.Model(&Photo{}).Where("album_id = ?", album.ID).Update("status", "missing").Error; err != nil {
		t.Fatal(err)
	}
	if err := RefreshAlbumCounts(db, []uint{album.ID}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&got, album.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.MediaCount != 0 {
		t.Fatalf("stale empty counts %+v", got)
	}
}
