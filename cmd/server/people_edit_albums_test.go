package main

import (
	"airy/internal/config"
	"airy/internal/database"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestEditPersonAddsAlbumsWithProfile(t *testing.T) {
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "edit.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	directory := database.AssetDirectory{DirectoryPath: "/edit", DirectoryName: "edit"}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	albums := []database.Album{
		{DirectoryID: directory.ID, AlbumName: "Existing", AlbumPath: "/edit/existing", ScanStatus: true},
		{DirectoryID: directory.ID, AlbumName: "New", AlbumPath: "/edit/new", ScanStatus: true},
		{DirectoryID: directory.ID, AlbumName: "Hidden", AlbumPath: "/edit/hidden"},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	person := database.Person{Name: "Before"}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.PersonAlbum{PersonID: person.ID, AlbumID: albums[0].ID}).Error; err != nil {
		t.Fatal(err)
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/people/"+formatUint(person.ID), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}
	body := fmt.Sprintf(`{"name":"After","note":"Updated","aliases":["Alias"],"album_ids":[%d,%d,%d,%d,999999]}`, albums[0].ID, albums[1].ID, albums[1].ID, albums[2].ID)
	for i := 0; i < 2; i++ {
		recorder := patch(body)
		if recorder.Code != http.StatusOK {
			t.Fatal(recorder.Body.String())
		}
		var result struct {
			Person personResponse `json:"person"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Person.Name != "After" || result.Person.Note != "Updated" || len(result.Person.Aliases) != 1 || result.Person.AlbumCount != 2 {
			t.Fatalf("unexpected person: %+v", result.Person)
		}
	}
	recorder := patch(`{"album_ids":[]}`)
	if recorder.Code != http.StatusOK {
		t.Fatal(recorder.Body.String())
	}
	var count int64
	if err := db.Model(&database.PersonAlbum{}).Where("person_id = ?", person.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected existing associations retained without duplicates, got %d", count)
	}
	// A failed association must roll back the profile update in the same request.
	if err := db.Exec("CREATE TRIGGER reject_person_album BEFORE INSERT ON person_albums BEGIN SELECT RAISE(ABORT, 'test failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	recorder = patch(fmt.Sprintf(`{"name":"Should roll back","album_ids":[%d]}`, albums[1].ID))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected rollback failure: %s", recorder.Body.String())
	}
	if err := db.First(&person, person.ID).Error; err != nil {
		t.Fatal(err)
	}
	if person.Name != "After" {
		t.Fatal("profile update was not rolled back")
	}
}
