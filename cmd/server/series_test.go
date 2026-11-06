package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
	"airy/internal/database"

	"github.com/gin-gonic/gin"
)

func TestSeriesCRUDAndGallery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "series.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	directory := database.AssetDirectory{DirectoryName: "library", DirectoryPath: "/library", ScanStatus: true}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	albums := []database.Album{
		{DirectoryID: directory.ID, AlbumName: "XINGYAN 001", AlbumPath: "/library/001", ScanStatus: true, CreatedAt: now.Add(-time.Hour)},
		{DirectoryID: directory.ID, AlbumName: "XINGYAN 002", AlbumPath: "/library/002", ScanStatus: true, CreatedAt: now},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	thumbnail := "/library/002/thumbnails/cover.webp"
	photo := database.Photo{
		AlbumID: albums[1].ID, PhotoName: "cover.jpg", PhotoPath: "/library/002/cover.jpg",
		ThumbnailPath: &thumbnail, MediaType: "image", Status: "active", MIMEType: "image/jpeg",
		ModifiedAt: now, LastSeenAt: now,
	}
	if err := db.Create(&photo).Error; err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, "sqlite", nil, nil, false)
	body := fmt.Sprintf(`{"name":"XINGYAN星颜社","album_ids":[%d,%d]}`, albums[0].ID, albums[1].ID)
	request := httptest.NewRequest(http.MethodPost, "/api/series", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create series status %d: %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		Series seriesResponse `json:"series"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Series.AlbumCount != 2 || created.Series.CoverURL != "/api/media/"+formatUint(photo.ID)+"/thumbnail" {
		t.Fatalf("unexpected created series: %+v", created.Series)
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/series/"+formatUint(created.Series.ID), bytes.NewBufferString(`{"name":"星颜社","favorite":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update series status %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/series/"+formatUint(created.Series.ID)+"/gallery", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var gallery struct {
		Series     seriesResponse     `json:"series"`
		Albums     []albumResponse    `json:"albums"`
		TotalMedia int64              `json:"total_media"`
		Pagination paginationResponse `json:"pagination"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &gallery); err != nil {
		t.Fatal(err)
	}
	if gallery.Series.Name != "星颜社" || !gallery.Series.Favorite || len(gallery.Albums) != 2 || gallery.TotalMedia != 1 {
		t.Fatalf("unexpected series gallery: %+v", gallery)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/series/"+formatUint(created.Series.ID), nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete series status %d: %s", recorder.Code, recorder.Body.String())
	}
	var seriesCount, linkedAlbumCount int64
	if err := db.Model(&database.Series{}).Count(&seriesCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.Album{}).Where("series_id IS NOT NULL").Count(&linkedAlbumCount).Error; err != nil {
		t.Fatal(err)
	}
	if seriesCount != 0 || linkedAlbumCount != 0 {
		t.Fatalf("series delete left data: series=%d linked_albums=%d", seriesCount, linkedAlbumCount)
	}
}

func TestAlbumCanBelongToOnlyOneSeries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "one-series.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	directory := database.AssetDirectory{DirectoryName: "library", DirectoryPath: "/library", ScanStatus: true}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	album := database.Album{DirectoryID: directory.ID, AlbumName: "Album", AlbumPath: "/library/album", ScanStatus: true}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	thumbnail := "/library/album/thumbnails/cover.webp"
	now := time.Now().UTC()
	photo := database.Photo{
		AlbumID: album.ID, PhotoName: "cover.jpg", PhotoPath: "/library/album/cover.jpg",
		ThumbnailPath: &thumbnail, MediaType: "image", Status: "active", MIMEType: "image/jpeg",
		ModifiedAt: now, LastSeenAt: now,
	}
	if err := db.Create(&photo).Error; err != nil {
		t.Fatal(err)
	}
	first := database.Series{Name: "First"}
	second := database.Series{Name: "Second"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, "sqlite", nil, nil, false)
	for _, seriesID := range []uint{first.ID, second.ID} {
		request := httptest.NewRequest(http.MethodPut,
			"/api/series/"+formatUint(seriesID)+"/albums/"+formatUint(album.ID), nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("associate series %d status %d: %s", seriesID, recorder.Code, recorder.Body.String())
		}
	}

	if err := db.First(&album, album.ID).Error; err != nil {
		t.Fatal(err)
	}
	if album.SeriesID == nil || *album.SeriesID != second.ID {
		t.Fatalf("expected album to move to series %d, got %v", second.ID, album.SeriesID)
	}
	if err := db.First(&first, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&second, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.CoverPhotoID != nil {
		t.Fatalf("expected old series cover to be cleared, got %v", first.CoverPhotoID)
	}
	if second.CoverPhotoID == nil || *second.CoverPhotoID != photo.ID {
		t.Fatalf("expected new series cover %d, got %v", photo.ID, second.CoverPhotoID)
	}
	firstItems, err := loadSeriesResponses(context.Background(), db, []database.Series{first})
	if err != nil {
		t.Fatal(err)
	}
	secondItems, err := loadSeriesResponses(context.Background(), db, []database.Series{second})
	if err != nil {
		t.Fatal(err)
	}
	if firstItems[0].AlbumCount != 0 || secondItems[0].AlbumCount != 1 {
		t.Fatalf("unexpected album counts after move: first=%d second=%d",
			firstItems[0].AlbumCount, secondItems[0].AlbumCount)
	}
}
