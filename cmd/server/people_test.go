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

func TestPeopleManageAlbumAssociationsAndGallery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "people.db") + "?_pragma=foreign_keys(1)",
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
		{DirectoryID: directory.ID, AlbumName: "Older", AlbumPath: "/library/Older", ScanStatus: true, CreatedAt: now.Add(-time.Hour)},
		{DirectoryID: directory.ID, AlbumName: "Newer", AlbumPath: "/library/Newer", ScanStatus: true, CreatedAt: now},
		{DirectoryID: directory.ID, AlbumName: "Britney-Song-08", AlbumPath: "/library/Britney-Song-08", ScanStatus: true, CreatedAt: now.Add(time.Hour)},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	olderThumbnail := "/library/Older/thumbnails/one.webp"
	newerThumbnail := "/library/Newer/thumbnails/two.webp"
	photos := []database.Photo{
		{AlbumID: albums[0].ID, PhotoName: "one.jpg", PhotoPath: "/library/Older/one.jpg", ThumbnailPath: &olderThumbnail, MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: now, LastSeenAt: now},
		{AlbumID: albums[1].ID, PhotoName: "two.jpg", PhotoPath: "/library/Newer/two.jpg", ThumbnailPath: &newerThumbnail, MediaType: "image", Status: "active", MIMEType: "image/jpeg", ModifiedAt: now, LastSeenAt: now},
	}
	if err := db.Create(&photos).Error; err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, "sqlite", nil, nil, false)
	requestBody := fmt.Sprintf(`{"name":"Alice","album_ids":[%d,%d]}`, albums[0].ID, albums[1].ID)
	request := httptest.NewRequest(http.MethodPost, "/api/people", bytes.NewBufferString(requestBody))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create person status %d: %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		Person personResponse `json:"person"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Person.AvatarURL != "/api/media/"+formatUint(photos[1].ID)+"/thumbnail" {
		t.Fatalf("unexpected person avatar: %q", created.Person.AvatarURL)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/people", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var people struct {
		People []personResponse `json:"people"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &people); err != nil {
		t.Fatal(err)
	}
	if len(people.People) != 1 || people.People[0].AlbumCount != 2 {
		t.Fatalf("unexpected people response: %+v", people)
	}
	if people.People[0].Favorite {
		t.Fatal("new person should not be favorited")
	}

	favoritePerson := database.Person{Name: "Zoe"}
	if err := db.Create(&favoritePerson).Error; err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(
		http.MethodPatch,
		"/api/people/"+formatUint(favoritePerson.ID)+"/favorite",
		bytes.NewBufferString(`{"favorite":true}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("favorite person status %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/people", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if err := json.Unmarshal(recorder.Body.Bytes(), &people); err != nil {
		t.Fatal(err)
	}
	if len(people.People) != 2 || people.People[1].ID != favoritePerson.ID || !people.People[1].Favorite {
		t.Fatalf("expected name order regardless of favorite: %+v", people.People)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/people/"+formatUint(created.Person.ID)+"/gallery", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var gallery struct {
		Albums     []albumResponse    `json:"albums"`
		TotalMedia int64              `json:"total_media"`
		Pagination paginationResponse `json:"pagination"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &gallery); err != nil {
		t.Fatal(err)
	}
	if len(gallery.Albums) != 2 || gallery.Albums[0].Name != "Newer" || gallery.TotalMedia != 2 {
		t.Fatalf("unexpected person gallery: %+v", gallery)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/people/"+formatUint(created.Person.ID)+"/album-options?q=Older", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var options struct {
		Albums []personAlbumOptionResponse `json:"albums"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &options); err != nil {
		t.Fatal(err)
	}
	if len(options.Albums) != 1 || !options.Albums[0].Associated {
		t.Fatalf("unexpected album options: %+v", options)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/albums/search?q=New", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var search struct {
		Albums []albumSearchResponse `json:"albums"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &search); err != nil {
		t.Fatal(err)
	}
	if len(search.Albums) != 1 || search.Albums[0].Name != "Newer" {
		t.Fatalf("unexpected album search response: %+v", search)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/albums/search?q=Britney+++Song", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if err := json.Unmarshal(recorder.Body.Bytes(), &search); err != nil {
		t.Fatal(err)
	}
	if len(search.Albums) != 1 || search.Albums[0].Name != "Britney-Song-08" {
		t.Fatalf("unexpected space-separated album search response: %+v", search)
	}

	request = httptest.NewRequest(
		http.MethodDelete,
		"/api/people/"+formatUint(created.Person.ID)+"/albums/"+formatUint(albums[0].ID),
		nil,
	)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("remove association status %d: %s", recorder.Code, recorder.Body.String())
	}
	var updated struct {
		Person personResponse `json:"person"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Person.AlbumCount != 1 || updated.Person.AvatarURL != "/api/media/"+formatUint(photos[1].ID)+"/thumbnail" {
		t.Fatalf("unexpected person after removing album: %+v", updated.Person)
	}
	var linkCount int64
	if err := db.Model(&database.PersonAlbum{}).Where("person_id = ?", created.Person.ID).Count(&linkCount).Error; err != nil {
		t.Fatal(err)
	}
	if linkCount != 1 {
		t.Fatalf("expected one remaining association, got %d", linkCount)
	}
}

func TestAlbumSearchReturnsEveryMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "album-search.db") + "?_pragma=foreign_keys(1)",
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
	albums := make([]database.Album, 125)
	for index := range albums {
		name := fmt.Sprintf("Winny-Sung-%03d", index+1)
		albums[index] = database.Album{
			DirectoryID: directory.ID,
			AlbumName:   name,
			AlbumPath:   "/library/" + name,
			ScanStatus:  true,
		}
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, "sqlite", nil, nil, false)
	request := httptest.NewRequest(http.MethodGet, "/api/albums/search?q=Winny+Sung", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("search status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Albums []albumSearchResponse `json:"albums"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Albums) != len(albums) {
		t.Fatalf("expected all %d matching albums, got %d", len(albums), len(response.Albums))
	}
}

func TestPeopleSupportDuplicateNamesAndAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "people-identities.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	router := newRouter(db, "sqlite", nil, nil, false)
	create := func(body string) personResponse {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/people", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("create duplicate-name person status %d: %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Person personResponse `json:"person"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Person
	}

	first := create(`{"name":"Alex","note":"摄影棚 A","aliases":["Alexa"," ALEXA ","Alex"]}`)
	second := create(`{"name":"Alex","note":"摄影棚 B","aliases":["Alexa","Lex"]}`)
	if first.ID == second.ID {
		t.Fatal("people with the same display name must keep different identities")
	}
	if len(first.Aliases) != 1 || first.Aliases[0] != "Alexa" {
		t.Fatalf("expected normalized and deduplicated aliases, got %#v", first.Aliases)
	}
	if len(second.Aliases) != 2 {
		t.Fatalf("same alias should be allowed for another person, got %#v", second.Aliases)
	}

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/people/"+formatUint(first.ID),
		bytes.NewBufferString(`{"name":"Alex Chen","note":"第一位","aliases":["Alex","A. Chen"]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update person identity status %d: %s", recorder.Code, recorder.Body.String())
	}
	var updated struct {
		Person personResponse `json:"person"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Person.Name != "Alex Chen" || updated.Person.Note != "第一位" || len(updated.Person.Aliases) != 2 {
		t.Fatalf("unexpected updated identity: %+v", updated.Person)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/people/"+formatUint(first.ID), nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete person status %d: %s", recorder.Code, recorder.Body.String())
	}
	var personCount int64
	if err := db.Model(&database.Person{}).Where("id = ?", first.ID).Count(&personCount).Error; err != nil {
		t.Fatal(err)
	}
	if personCount != 0 {
		t.Fatal("expected person and embedded aliases to be deleted")
	}
}
