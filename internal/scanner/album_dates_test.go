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
	names := []string{"[2024.02.29] Album", "No title date", "Image filename", "Image modification"}
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	modified := time.Date(2022, 7, 8, 18, 0, 0, 0, time.UTC)
	for _, item := range []struct{ album, name string }{
		{names[0], "2020 1 2.jpg"},
		{names[2], "a-no-date.jpg"},
		{names[2], "b-23-12-31.jpg"},
		{names[2], "c-2020-01-01.jpg"},
		{names[3], "a-photo.jpg"},
		{names[3], "b-photo.jpg"},
	} {
		path := filepath.Join(root, item.album, item.name)
		writeJPEG(t, path, 20, 30)
		stamp := modified
		if item.name == "b-photo.jpg" {
			stamp = modified.Add(-24 * time.Hour)
		}
		if err := os.Chtimes(path, stamp, stamp); err != nil {
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
	for name, want := range map[string]string{names[2]: "2023-12-31", names[3]: "2022-07-08"} {
		var album database.Album
		if err := db.Where("album_path = ?", filepath.Join(root, name)).First(&album).Error; err != nil {
			t.Fatal(err)
		}
		if album.PublishedDate != want {
			t.Fatalf("%s: got %s want %s", name, album.PublishedDate, want)
		}
	}
	// Simulate records indexed on an earlier day and verify rescan stability.
	if err := db.Model(&fallback).Updates(map[string]any{"published_date": "2020-01-02", "album_name": "CSV edited title"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&fallback, fallback.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fallback.PublishedDate != "2020-01-02" || fallback.AlbumName != "CSV edited title" {
		t.Fatal("rescan replaced original publication date")
	}
	var count int64
	if err := db.Model(&database.Album{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(len(names)) {
		t.Fatalf("rescan created duplicate albums: %d", count)
	}
}
