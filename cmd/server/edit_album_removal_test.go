package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"airy/internal/config"
	"airy/internal/database"
)

func TestEditAlbumRemovalIsScopedAndTransactional(t *testing.T) {
	for _, kind := range []string{"people", "series"} {
		t.Run(kind, func(t *testing.T) {
			db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "remove.db") + "?_pragma=foreign_keys(1)"})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			t.Cleanup(func() { _ = sqlDB.Close() })
			directory := database.AssetDirectory{DirectoryName: "library", DirectoryPath: "/remove"}
			if err := db.Create(&directory).Error; err != nil {
				t.Fatal(err)
			}
			people := []database.Person{{Name: "Current"}, {Name: "Other"}}
			series := []database.Series{{Name: "Current"}, {Name: "Other"}}
			if err := db.Create(&people).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&series).Error; err != nil {
				t.Fatal(err)
			}
			albums := []database.Album{
				{DirectoryID: directory.ID, AlbumName: "Keep", AlbumPath: "/remove/keep", ScanStatus: true, SeriesID: &series[0].ID},
				{DirectoryID: directory.ID, AlbumName: "Remove", AlbumPath: "/remove/remove", ScanStatus: true, SeriesID: &series[0].ID},
				{DirectoryID: directory.ID, AlbumName: "Other", AlbumPath: "/remove/other", ScanStatus: true, SeriesID: &series[1].ID},
			}
			if err := db.Create(&albums).Error; err != nil {
				t.Fatal(err)
			}
			links := []database.PersonAlbum{
				{PersonID: people[0].ID, AlbumID: albums[0].ID}, {PersonID: people[0].ID, AlbumID: albums[1].ID},
				{PersonID: people[1].ID, AlbumID: albums[1].ID}, {PersonID: people[1].ID, AlbumID: albums[2].ID},
			}
			if err := db.Create(&links).Error; err != nil {
				t.Fatal(err)
			}
			thumbnail := "/remove/remove/thumbnails/cover.webp"
			photo := database.Photo{AlbumID: albums[1].ID, PhotoName: "cover.jpg", PhotoPath: "/remove/remove/cover.jpg", ThumbnailPath: &thumbnail, Status: "active"}
			if err := db.Create(&photo).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.RefreshPersonAvatarPhotoIDs(db, []uint{people[0].ID}); err != nil {
				t.Fatal(err)
			}
			if err := database.RefreshSeriesCoverPhotoIDs(db, []uint{series[0].ID}); err != nil {
				t.Fatal(err)
			}
			id := people[0].ID
			if kind == "series" {
				id = series[0].ID
			}
			router := newRouter(db, "sqlite", nil, nil, false)
			patch := func(body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPatch, "/api/"+kind+"/"+formatUint(id), bytes.NewBufferString(body))
				req.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				return recorder
			}
			checkMembership := func(albumID uint, expected bool) {
				t.Helper()
				var count int64
				query := db.Model(&database.PersonAlbum{}).Where("person_id = ? AND album_id = ?", id, albumID)
				if kind == "series" {
					query = db.Model(&database.Album{}).Where("series_id = ? AND id = ?", id, albumID)
				}
				if err := query.Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if (count > 0) != expected {
					t.Fatalf("album %d membership = %d, want %v", albumID, count, expected)
				}
			}
			for i := 0; i < 2; i++ {
				recorder := patch(fmt.Sprintf(`{"remove_album_ids":[%d,%d,999999]}`, albums[1].ID, albums[2].ID))
				if recorder.Code != http.StatusOK {
					t.Fatal(recorder.Body.String())
				}
			}
			checkMembership(albums[0].ID, true)
			checkMembership(albums[1].ID, false)
			if kind == "people" {
				var person database.Person
				if err := db.First(&person, id).Error; err != nil {
					t.Fatal(err)
				}
				if person.AvatarPhotoID != nil {
					t.Fatal("stale avatar after removal")
				}
				var otherLinks int64
				if err := db.Model(&database.PersonAlbum{}).Where("person_id = ?", people[1].ID).Count(&otherLinks).Error; err != nil {
					t.Fatal(err)
				}
				if otherLinks != 2 {
					t.Fatal("another person's associations changed")
				}
			} else {
				var item database.Series
				if err := db.First(&item, id).Error; err != nil {
					t.Fatal(err)
				}
				if item.CoverPhotoID != nil {
					t.Fatal("stale cover after removal")
				}
				var other database.Album
				if err := db.First(&other, albums[2].ID).Error; err != nil {
					t.Fatal(err)
				}
				if other.SeriesID == nil || *other.SeriesID != series[1].ID {
					t.Fatal("another series association changed")
				}
			}
			recorder := patch(fmt.Sprintf(`{"album_ids":[%d],"remove_album_ids":[%d]}`, albums[1].ID, albums[0].ID))
			if recorder.Code != http.StatusOK {
				t.Fatal(recorder.Body.String())
			}
			checkMembership(albums[0].ID, false)
			checkMembership(albums[1].ID, true)
			trigger := "CREATE TRIGGER reject_add BEFORE INSERT ON person_albums BEGIN SELECT RAISE(ABORT, 'test failure'); END"
			if kind == "series" {
				trigger = "CREATE TRIGGER reject_add BEFORE UPDATE OF series_id ON albums WHEN NEW.series_id IS NOT NULL BEGIN SELECT RAISE(ABORT, 'test failure'); END"
			}
			if err := db.Exec(trigger).Error; err != nil {
				t.Fatal(err)
			}
			recorder = patch(fmt.Sprintf(`{"name":"Should roll back","album_ids":[%d],"remove_album_ids":[%d]}`, albums[0].ID, albums[1].ID))
			if recorder.Code < 400 {
				t.Fatal("expected failed batch")
			}
			checkMembership(albums[0].ID, false)
			checkMembership(albums[1].ID, true)
			var name string
			if err := db.Table(kind).Where("id = ?", id).Pluck("name", &name).Error; err != nil {
				t.Fatal(err)
			}
			if name != "Current" {
				t.Fatal("name update not rolled back")
			}
			var mediaCount int64
			if err := db.Model(&database.Photo{}).Count(&mediaCount).Error; err != nil {
				t.Fatal(err)
			}
			if mediaCount != 1 {
				t.Fatal("media removed when removing association")
			}
		})
	}
}
