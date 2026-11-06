package main

import (
	"bytes"
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

func TestGalleryAndAlbumMediaHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "gallery.db") + "?_pragma=foreign_keys(1)",
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
	album := database.Album{DirectoryID: directory.ID, AlbumName: "Album A", AlbumPath: "/library/Album A", PublishedDate: "2024-02-29", ScanStatus: true}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	photos := []database.Photo{
		{AlbumID: album.ID, PhotoName: "001.jpg", PhotoPath: "/library/Album A/001.jpg", MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: now, LastSeenAt: now},
		{AlbumID: album.ID, PhotoName: "002.mp4", PhotoPath: "/library/Album A/002.mp4", MediaType: "video", Status: "active", MIMEType: "video/mp4", ModifiedAt: now, LastSeenAt: now},
	}
	if err := db.Create(&photos).Error; err != nil {
		t.Fatal(err)
	}
	person := database.Person{Name: "Alice"}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.PersonAlbum{PersonID: person.ID, AlbumID: album.ID}).Error; err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, "sqlite", nil, nil, false)
	defaultRecorder := httptest.NewRecorder()
	router.ServeHTTP(defaultRecorder, httptest.NewRequest(http.MethodGet, "/api/gallery", nil))
	var defaultGallery struct {
		Albums     []albumResponse    `json:"albums"`
		Pagination paginationResponse `json:"pagination"`
	}
	if err := json.Unmarshal(defaultRecorder.Body.Bytes(), &defaultGallery); err != nil {
		t.Fatal(err)
	}
	if defaultRecorder.Code != http.StatusOK || defaultGallery.Pagination.Limit != 10 || len(defaultGallery.Albums) != 1 || len(defaultGallery.Albums[0].Media) != len(photos) {
		t.Fatalf("default gallery must return batches of 10 with complete media: %s", defaultRecorder.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/gallery?limit=4&media_limit=1", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("gallery status %d: %s", recorder.Code, recorder.Body.String())
	}
	var gallery struct {
		Albums     []albumResponse    `json:"albums"`
		TotalMedia int64              `json:"total_media"`
		Pagination paginationResponse `json:"pagination"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &gallery); err != nil {
		t.Fatal(err)
	}
	if len(gallery.Albums) != 1 || gallery.Albums[0].MediaCount != 2 || len(gallery.Albums[0].Media) != 1 {
		t.Fatalf("unexpected gallery response: %+v", gallery)
	}
	if gallery.Albums[0].PublishedDate != "2024-02-29" {
		t.Fatalf("missing publication date: %+v", gallery.Albums[0])
	}
	if gallery.TotalMedia != 2 || gallery.Albums[0].Media[0].ThumbnailURL == "" {
		t.Fatalf("missing gallery totals or media URLs: %+v", gallery)
	}
	if gallery.Albums[0].Media[0].Favorite {
		t.Fatal("new media should not be favorited")
	}
	if gallery.Albums[0].Favorite {
		t.Fatal("new album should not be favorited")
	}
	if len(gallery.Albums[0].People) != 1 || gallery.Albums[0].People[0].ID != person.ID {
		t.Fatalf("missing associated people: %+v", gallery.Albums[0].People)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/gallery?q=Album+A&limit=4&media_limit=1", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("search gallery status %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &gallery); err != nil {
		t.Fatal(err)
	}
	if len(gallery.Albums) != 1 || gallery.Pagination.Total != 1 {
		t.Fatalf("unexpected searched gallery: %+v", gallery)
	}

	request = httptest.NewRequest(
		http.MethodPatch,
		"/api/media/"+formatUint(photos[0].ID)+"/favorite",
		bytes.NewBufferString(`{"favorite":true}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("favorite media status %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/favorites/photos?limit=500", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("favorite photos status %d: %s", recorder.Code, recorder.Body.String())
	}
	var favoritePhotos struct {
		Media      []mediaResponse    `json:"media"`
		Pagination paginationResponse `json:"pagination"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &favoritePhotos); err != nil {
		t.Fatal(err)
	}
	if len(favoritePhotos.Media) != 1 || !favoritePhotos.Media[0].Favorite || favoritePhotos.Pagination.Total != 1 {
		t.Fatalf("unexpected favorite photos: %+v", favoritePhotos)
	}

	request = httptest.NewRequest(
		http.MethodPatch,
		"/api/albums/"+formatUint(album.ID)+"/favorite",
		bytes.NewBufferString(`{"favorite":true}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("favorite album status %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/gallery?favorites=true&limit=4&media_limit=1", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("favorite gallery status %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &gallery); err != nil {
		t.Fatal(err)
	}
	if len(gallery.Albums) != 1 || !gallery.Albums[0].Favorite || gallery.Pagination.Total != 1 {
		t.Fatalf("unexpected favorite gallery: %+v", gallery)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/albums/"+formatUint(album.ID)+"/media?limit=500", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("album status %d: %s", recorder.Code, recorder.Body.String())
	}
	var albumMedia struct {
		Album albumResponse `json:"album"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &albumMedia); err != nil {
		t.Fatal(err)
	}
	if len(albumMedia.Album.Media) != 2 {
		t.Fatalf("expected 2 media, got %+v", albumMedia.Album)
	}
}

func TestGalleryOrdersAlbumsByNewestFirst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "gallery-order.db") + "?_pragma=foreign_keys(1)",
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
		{
			DirectoryID: directory.ID, AlbumName: "Zulu Old", AlbumPath: "/library/Zulu Old",
			ScanStatus: true, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
		},
		{
			DirectoryID: directory.ID, AlbumName: "Alpha New", AlbumPath: "/library/Alpha New",
			ScanStatus: true, CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, "sqlite", nil, nil, false)
	request := httptest.NewRequest(http.MethodGet, "/api/gallery?limit=4&media_limit=1", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("gallery status %d: %s", recorder.Code, recorder.Body.String())
	}
	var gallery struct {
		Albums []albumResponse `json:"albums"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &gallery); err != nil {
		t.Fatal(err)
	}
	if len(gallery.Albums) != 2 || gallery.Albums[0].Name != "Alpha New" || gallery.Albums[1].Name != "Zulu Old" {
		t.Fatalf("albums are not ordered newest first: %+v", gallery.Albums)
	}
}

func formatUint(value uint) string {
	return fmt.Sprintf("%d", value)
}
