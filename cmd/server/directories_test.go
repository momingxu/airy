package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
	"airy/internal/database"

	"github.com/gin-gonic/gin"
)

func TestDirectoryManagementHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "directories.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	root := t.TempDir()
	resourcePath := filepath.Join(root, "Series")
	if err := os.MkdirAll(filepath.Join(resourcePath, "Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	body, _ := json.Marshal(map[string]any{"path": root, "album_depth": 2})
	request := httptest.NewRequest(http.MethodPost, "/api/directories", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create directory status %d: %s", recorder.Code, recorder.Body.String())
	}

	var created struct {
		Directory directoryResponse `json:"directory"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Directory.ID == 0 || created.Directory.Path != resourcePath || created.Directory.Name != "Series" || created.Directory.AlbumDepth != 1 {
		t.Fatalf("unexpected created directory: %+v", created.Directory)
	}

	album := database.Album{
		DirectoryID: created.Directory.ID,
		AlbumName:   "Album",
		AlbumPath:   filepath.Join(resourcePath, "Album"),
		ScanStatus:  true,
	}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	photo := database.Photo{
		AlbumID: album.ID, PhotoName: "one.jpg", PhotoPath: filepath.Join(album.AlbumPath, "one.jpg"),
		MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: now, LastSeenAt: now,
	}
	if err := db.Create(&photo).Error; err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/directories", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list directories status %d: %s", recorder.Code, recorder.Body.String())
	}
	var listed struct {
		Directories []directoryResponse `json:"directories"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Directories) != 1 || listed.Directories[0].AlbumCount != 1 || listed.Directories[0].MediaCount != 1 {
		t.Fatalf("unexpected directory list: %+v", listed.Directories)
	}

	settingsBody := bytes.NewBufferString(`{"automation_disabled":true}`)
	request = httptest.NewRequest(http.MethodPatch, "/api/directories/"+formatUint(created.Directory.ID)+"/settings", settingsBody)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update directory settings status %d: %s", recorder.Code, recorder.Body.String())
	}
	var settings struct {
		Directory directoryResponse `json:"directory"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if !settings.Directory.AutomationDisabled {
		t.Fatalf("unexpected directory settings: %+v", settings.Directory)
	}

	settingsBody = bytes.NewBufferString(`{"album_depth":3}`)
	request = httptest.NewRequest(http.MethodPatch, "/api/directories/"+formatUint(created.Directory.ID)+"/settings", settingsBody)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected album depth update status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/directories/"+formatUint(created.Directory.ID)+"/settings", bytes.NewBufferString(`{"scan_interval_minutes":30}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid interval status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/directories/"+formatUint(created.Directory.ID), nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete directory status %d: %s", recorder.Code, recorder.Body.String())
	}

	var directoryCount, albumCount, photoCount int64
	if err := db.Model(&database.AssetDirectory{}).Count(&directoryCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.Album{}).Count(&albumCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.Photo{}).Count(&photoCount).Error; err != nil {
		t.Fatal(err)
	}
	if directoryCount != 0 || albumCount != 0 || photoCount != 0 {
		t.Fatalf("metadata was not removed: directories=%d albums=%d photos=%d", directoryCount, albumCount, photoCount)
	}
}

func TestCreateDirectoryRejectsMissingPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "missing.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	router := newRouter(db, "sqlite", nil, nil, false)
	request := httptest.NewRequest(http.MethodPost, "/api/directories", bytes.NewBufferString(`{"path":"/path/that/does/not/exist"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateDirectoryRejectsInvalidAlbumDepth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "invalid-depth.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	body, _ := json.Marshal(map[string]any{"path": t.TempDir(), "album_depth": 11})
	request := httptest.NewRequest(http.MethodPost, "/api/directories", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	newRouter(db, "sqlite", nil, nil, false).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
