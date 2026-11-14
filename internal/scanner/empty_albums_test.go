package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"airy/internal/config"
	"airy/internal/database"
)

func TestEmptyAlbumsRemainUnpublishedAndRecoverOnRescan(t *testing.T) {
	root := t.TempDir()
	albumPath := filepath.Join(root, "Album")
	if err := os.Mkdir(albumPath, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "empty.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	scanner := New(db, "missing-ffprobe-for-test", filepath.Join("..", "..", "tools", "ffmpeg", "ffmpeg"))
	var albumID uint
	scanAndCheck := func(wantPublished bool) {
		t.Helper()
		if _, err := scanner.scan(context.Background(), root, nil); err != nil {
			t.Fatal(err)
		}
		var album database.Album
		if err := db.Where("album_path = ?", albumPath).First(&album).Error; err != nil {
			t.Fatal(err)
		}
		if albumID != 0 && album.ID != albumID {
			t.Fatal("rescan replaced album identity")
		}
		albumID = album.ID
		wantCount := int64(0)
		if wantPublished {
			wantCount = 1
		}
		if album.MediaCount != wantCount || album.CountsUpdatedAt == nil {
			t.Fatalf("unexpected persisted counts: %+v", album)
		}
		if album.ScanStatus != wantPublished {
			t.Fatalf("published = %v, want %v", album.ScanStatus, wantPublished)
		}
	}
	scanAndCheck(false)
	if err := os.WriteFile(filepath.Join(albumPath, "notes.txt"), []byte("no media"), 0o644); err != nil {
		t.Fatal(err)
	}
	scanAndCheck(false)
	photoPath := filepath.Join(albumPath, "image.jpg")
	writeJPEG(t, photoPath, 40, 30)
	scanAndCheck(true)
	if err := os.Remove(photoPath); err != nil {
		t.Fatal(err)
	}
	scanAndCheck(false)
	var active int64
	if err := db.Model(&database.Photo{}).Where("album_id = ? AND status = ?", albumID, AssetStatusActive).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("empty album retained %d active media", active)
	}
	writeJPEG(t, photoPath, 40, 30)
	scanAndCheck(true)
}
