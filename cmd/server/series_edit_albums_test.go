package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"airy/internal/config"
	"airy/internal/database"
)

func TestEditSeriesAddsAlbumsAndRefreshesPreviousSeries(t *testing.T) {
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "series-edit.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	directory := database.AssetDirectory{DirectoryName: "library", DirectoryPath: "/series-edit"}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	current, previous := database.Series{Name: "Current"}, database.Series{Name: "Previous"}
	if err := db.Create(&current).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatal(err)
	}
	albums := []database.Album{
		{DirectoryID: directory.ID, AlbumName: "Existing", AlbumPath: "/series-edit/existing", ScanStatus: true, SeriesID: &current.ID},
		{DirectoryID: directory.ID, AlbumName: "Moving", AlbumPath: "/series-edit/moving", ScanStatus: true, SeriesID: &previous.ID},
		{DirectoryID: directory.ID, AlbumName: "Hidden", AlbumPath: "/series-edit/hidden", SeriesID: &previous.ID},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	thumbnail := "/series-edit/moving/thumbnails/photo.webp"
	photo := database.Photo{AlbumID: albums[1].ID, PhotoName: "photo.jpg", PhotoPath: "/series-edit/moving/photo.jpg", ThumbnailPath: &thumbnail, Status: "active"}
	if err := db.Create(&photo).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshSeriesCoverPhotoIDs(db, []uint{previous.ID}); err != nil {
		t.Fatal(err)
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/series/"+formatUint(current.ID), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}
	body := fmt.Sprintf(`{"name":"Updated","album_ids":[%d,%d,%d,%d,999999]}`, albums[0].ID, albums[1].ID, albums[1].ID, albums[2].ID)
	for i := 0; i < 2; i++ {
		recorder := patch(body)
		if recorder.Code != http.StatusOK {
			t.Fatal(recorder.Body.String())
		}
		var result struct {
			Series seriesResponse `json:"series"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Series.Name != "Updated" || result.Series.AlbumCount != 2 || result.Series.CoverURL != fmt.Sprintf("/api/media/%d/thumbnail", photo.ID) {
			t.Fatalf("unexpected series: %+v", result.Series)
		}
	}
	var old database.Series
	if err := db.First(&old, previous.ID).Error; err != nil {
		t.Fatal(err)
	}
	if old.CoverPhotoID != nil {
		t.Fatal("previous series retained stale cover")
	}
	var hidden database.Album
	if err := db.First(&hidden, albums[2].ID).Error; err != nil {
		t.Fatal(err)
	}
	if hidden.SeriesID == nil || *hidden.SeriesID != previous.ID {
		t.Fatal("hidden album should not be reassigned")
	}
	recorder := patch(`{"album_ids":[]}`)
	if recorder.Code != http.StatusOK {
		t.Fatal(recorder.Body.String())
	}
	var count int64
	if err := db.Model(&database.Album{}).Where("series_id = ?", current.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected existing albums retained, got %d", count)
	}
	if err := db.Exec("CREATE TRIGGER reject_series_assignment BEFORE UPDATE OF series_id ON albums BEGIN SELECT RAISE(ABORT, 'test failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	recorder = patch(fmt.Sprintf(`{"name":"Should roll back","album_ids":[%d]}`, albums[1].ID))
	if recorder.Code < 400 {
		t.Fatal("expected failure")
	}
	if err := db.First(&current, current.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Name != "Updated" {
		t.Fatal("name update was not rolled back")
	}
}
