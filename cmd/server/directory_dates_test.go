package main

import (
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

func TestRefreshAlbumPublicationDates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "dates.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	directories := []database.AssetDirectory{{DirectoryName: "one", DirectoryPath: "/one"}, {DirectoryName: "other", DirectoryPath: "/other"}}
	if err := db.Create(&directories).Error; err != nil {
		t.Fatal(err)
	}
	albums := []database.Album{
		{DirectoryID: directories[0].ID, AlbumName: "Set 2024-02-29", AlbumPath: "/one/a", PublishedDate: "2020-01-01", ScanStatus: true},
		{DirectoryID: directories[0].ID, AlbumName: "黃琪雯(Chi-wen Huang) 新社麻竹坑", AlbumPath: "/one/b", PublishedDate: "2020-01-01"},
		{DirectoryID: directories[0].ID, AlbumName: "modified", AlbumPath: "/one/c", PublishedDate: "2020-01-01"},
		{DirectoryID: directories[1].ID, AlbumName: "Other 2025-01-01", AlbumPath: "/other/a", PublishedDate: "2020-01-01"},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	modified := time.Date(2023, 5, 6, 12, 0, 0, 0, time.UTC)
	for i, name := range []string{"49558720661-2020-01-18_09-47-03-DSC_9059.jpg", "plain.jpg"} {
		photo := database.Photo{AlbumID: albums[i+1].ID, PhotoName: name, PhotoPath: fmt.Sprintf("/image-%d", i), MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: modified, LastSeenAt: modified}
		if err := db.Create(&photo).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	for pass := range 2 {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/albums/publication-dates/refresh", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%d: %s", response.Code, response.Body.String())
		}
		var result struct {
			Total   int
			Updated int
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		wantUpdated := 4
		if pass == 1 {
			wantUpdated = 0
		}
		if result.Total != 4 || result.Updated != wantUpdated {
			t.Fatalf("unexpected result %+v", result)
		}
	}
	for i, want := range []string{"2024-02-29", "2020-01-18", "2023-05-06", "2025-01-01"} {
		var got database.Album
		if err := db.First(&got, albums[i].ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.PublishedDate != want || got.ScanStatus != albums[i].ScanStatus || !got.UpdatedAt.Equal(albums[i].UpdatedAt) {
			t.Fatalf("unexpected album %+v", got)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/directories/%d/publication-dates/refresh", directories[0].ID), nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("per-directory action still available: %d", response.Code)
	}
}
