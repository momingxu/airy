package media

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
	"airy/internal/database"
)

func TestOpenThumbnailAndOriginal(t *testing.T) {
	root := t.TempDir()
	albumPath := filepath.Join(root, "Album")
	if err := os.Mkdir(albumPath, 0o755); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(albumPath, "photo.jpg")
	createTestImage(t, imagePath, 900, 600)
	thumbnailPath := filepath.Join(albumPath, "thumbnails", "photo.jpg.webp")
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thumbnailPath, []byte("generated webp"), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "media.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	directory := database.AssetDirectory{DirectoryName: "root", DirectoryPath: root, ScanStatus: true}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	album := database.Album{DirectoryID: directory.ID, AlbumName: "Album", AlbumPath: albumPath, ScanStatus: true}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(imagePath)
	now := time.Now().UTC()
	photo := database.Photo{AlbumID: album.ID, PhotoName: "photo.jpg", PhotoPath: imagePath, ThumbnailPath: &thumbnailPath, MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: info.ModTime(), LastSeenAt: now}
	if err := db.Create(&photo).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(db)
	_, thumbnail, thumbnailInfo, err := service.OpenThumbnail(context.Background(), photo.ID)
	if err != nil {
		t.Fatalf("open thumbnail: %v", err)
	}
	defer thumbnail.Close()
	if thumbnailInfo.Size() == 0 {
		t.Fatal("thumbnail is empty")
	}
	resolved, file, originalInfo, err := service.OpenOriginal(context.Background(), photo.ID)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer file.Close()
	if resolved.ID != photo.ID || originalInfo.Size() != info.Size() {
		t.Fatalf("unexpected original: %+v, size=%d", resolved, originalInfo.Size())
	}
}

func createTestImage(t *testing.T, path string, width, height int) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	imageData := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			imageData.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	if err := jpeg.Encode(file, imageData, &jpeg.Options{Quality: 80}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
