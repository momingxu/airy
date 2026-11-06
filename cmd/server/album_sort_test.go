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

func TestAlbumListsOrderByPublicationDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "sort.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	directory := database.AssetDirectory{DirectoryName: "sort", DirectoryPath: "/sort"}
	person := database.Person{Name: "sort"}
	series := database.Series{Name: "sort"}
	for _, record := range []any{&directory, &person, &series} {
		if err := db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	albums := make([]database.Album, 3)
	for i, date := range []string{"2025-01-01", "2025-01-01", "2020-01-01"} {
		albums[i] = database.Album{DirectoryID: directory.ID, SeriesID: &series.ID, AlbumName: "sort", AlbumPath: fmt.Sprintf("/sort/%d", i), PublishedDate: date, ScanStatus: true, Favorite: true, CreatedAt: time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC)}
		if err := db.Create(&albums[i]).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&database.PersonAlbum{PersonID: person.ID, AlbumID: albums[i].ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	router.GET("/gallery", galleryHandler(db))
	router.GET("/people/:id/gallery", personGalleryHandler(db))
	router.GET("/series/:id/gallery", seriesGalleryHandler(db))
	router.GET("/search", searchAlbumsHandler(db))
	router.GET("/people/:id/options", personAlbumOptionsHandler(db))
	router.GET("/series/:id/options", seriesAlbumOptionsHandler(db))
	for _, path := range []string{"/gallery", "/gallery?favorites=true", "/gallery?q=sort", fmt.Sprintf("/people/%d/gallery", person.ID), fmt.Sprintf("/series/%d/gallery", series.ID), "/search?q=sort", fmt.Sprintf("/people/%d/options", person.ID), fmt.Sprintf("/series/%d/options", series.ID)} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			var result struct {
				Albums []struct {
					ID uint `json:"id"`
				} `json:"albums"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || len(result.Albums) != 3 {
				t.Fatalf("unexpected response: %s", recorder.Body.String())
			}
			for i, want := range []uint{albums[1].ID, albums[0].ID, albums[2].ID} {
				if result.Albums[i].ID != want {
					t.Fatalf("position %d: got %d, want %d", i, result.Albums[i].ID, want)
				}
			}
		})
	}
}
