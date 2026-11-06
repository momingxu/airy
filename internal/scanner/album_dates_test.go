package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
	"airy/internal/database"
)

func TestScanAlbumPublicationDatesAndRescan(t *testing.T) {
	root := t.TempDir()
	names := []string{"[2024.02.29] Album", "No title date"}
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "scan-dates.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	service := New(db, "missing-ffprobe", "missing-ffmpeg")
	before := time.Now()
	if _, err := service.scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	var dated, fallback database.Album
	if err := db.Where("album_path = ?", filepath.Join(root, names[0])).First(&dated).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("album_path = ?", filepath.Join(root, names[1])).First(&fallback).Error; err != nil {
		t.Fatal(err)
	}
	if dated.PublishedDate != "2024-02-29" {
		t.Fatalf("date from title: %s", dated.PublishedDate)
	}
	if fallback.PublishedDate != database.AlbumPublicationDate("", before) && fallback.PublishedDate != database.AlbumPublicationDate("", after) {
		t.Fatalf("scan day fallback: %s", fallback.PublishedDate)
	}
	// Simulate records indexed on an earlier day and verify rescan stability.
	if err := db.Model(&fallback).UpdateColumn("published_date", "2020-01-02").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&fallback, fallback.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fallback.PublishedDate != "2020-01-02" {
		t.Fatal("rescan replaced original publication date")
	}
	var count int64
	if err := db.Model(&database.Album{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rescan created duplicate albums: %d", count)
	}
}
