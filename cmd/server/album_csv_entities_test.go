package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"airy/internal/config"
	"airy/internal/database"
	"github.com/gin-gonic/gin"
)

func TestAlbumCSVEntities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "entities.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	dir := database.AssetDirectory{DirectoryName: "dir", DirectoryPath: "/dir"}
	if err := db.Create(&dir).Error; err != nil {
		t.Fatal(err)
	}
	albums := []database.Album{{DirectoryID: dir.ID, AlbumName: "A", AlbumPath: "/dir/a", ScanStatus: true}, {DirectoryID: dir.ID, AlbumName: "B", AlbumPath: "/dir/b", ScanStatus: true}}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	existing := database.Person{Name: "已有模特", Aliases: []string{}}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	thumbnail := "/thumbnail.jpg"
	photo := database.Photo{AlbumID: albums[0].ID, PhotoName: "photo.jpg", PhotoPath: "/dir/a/photo.jpg", ThumbnailPath: &thumbnail, Status: "active"}
	if err := db.Create(&photo).Error; err != nil {
		t.Fatal(err)
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	base := fmt.Sprintf("/api/directories/%d/", dir.ID)
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, base+path, strings.NewReader(body))
		req.Header.Set("X-Preview-Token", token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	encode := func(rows ...[]string) string {
		var b bytes.Buffer
		w := csv.NewWriter(&b)
		_ = w.Write(albumCSVHeader)
		for _, r := range rows {
			_ = w.Write(r)
		}
		w.Flush()
		return b.String()
	}
	row := func(i int, series, people string) []string {
		return []string{fmt.Sprint(albums[i].ID), albums[i].AlbumName, "", series, people}
	}
	preview := func(body string) albumCSVPreview {
		r := call("POST", "albums/csv-preview", body, "")
		if r.Code != 200 {
			t.Fatalf("preview: %s", r.Body.String())
		}
		var p albumCSVPreview
		if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	apply := func(body string) {
		p := preview(body)
		if !p.Valid {
			t.Fatalf("invalid: %+v", p.Rows)
		}
		r := call("POST", "albums/csv-import", body, p.Token)
		if r.Code != 200 {
			t.Fatalf("apply: %s", r.Body.String())
		}
	}
	count := func(model any) int64 {
		var n int64
		if err := db.Model(model).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	body := encode(row(0, "新系列", "已有模特;新模特;新模特"), row(1, "新系列", "新模特"))
	p := preview(body)
	if !p.Valid || p.Changes != 2 || !p.Rows[0].CreateSeries || len(p.Rows[0].NewPeople) != 1 || count(&database.Series{}) != 0 || count(&database.Person{}) != 1 {
		t.Fatalf("preview created entities or wrong resolution: %+v", p)
	}
	apply(body)
	if count(&database.Series{}) != 1 || count(&database.Person{}) != 2 || count(&database.PersonAlbum{}) != 3 {
		t.Fatal("new entities not reused/deduplicated")
	}
	var created database.Person
	var series database.Series
	if err := db.Where("name = ?", "新模特").First(&created).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&series).Error; err != nil {
		t.Fatal(err)
	}
	if created.AvatarPhotoID == nil || *created.AvatarPhotoID != photo.ID || series.CoverPhotoID == nil {
		t.Fatal("entity images not refreshed")
	}
	exported := call("GET", "albums.csv", "", "")
	p = preview(exported.Body.String())
	if !p.Valid || p.Changes != 0 {
		t.Fatalf("entity roundtrip: %+v", p)
	}
	// Blank fields keep links, while explicit clear removes them and refreshes covers.
	p = preview(encode(row(0, "", "")))
	if !p.Valid || p.Changes != 0 {
		t.Fatal("blank values changed associations")
	}
	apply(encode(row(0, csvClear, csvClear), row(1, csvClear, csvClear)))
	if count(&database.PersonAlbum{}) != 0 {
		t.Fatal("clear retained links")
	}
	if err := db.First(&series, series.ID).Error; err != nil {
		t.Fatal(err)
	}
	if series.CoverPhotoID != nil {
		t.Fatal("clear retained stale cover")
	}
	// Duplicate existing names must not be guessed, and preview must never create.
	duplicates := []database.Person{{Name: "同名", Aliases: []string{}}, {Name: "同名", Aliases: []string{}}}
	if err := db.Create(&duplicates).Error; err != nil {
		t.Fatal(err)
	}
	p = preview(encode(row(0, "不会创建", "同名")))
	if p.Valid || !strings.Contains(p.Rows[0].Error, "同名") || count(&database.Series{}) != 1 {
		t.Fatal("ambiguous model accepted")
	}
	// Existing names containing delimiters can round-trip via JSON arrays.
	apply(encode(row(0, "其他系列", `["分号;模特","已有模特"]`)))
	p = preview(call("GET", "albums.csv", "", "").Body.String())
	if !p.Valid || p.Changes != 0 {
		t.Fatal("special name roundtrip failed")
	}
	// Changing links after preview invalidates the token.
	body = encode(row(0, "新系列", "新模特"))
	p = preview(body)
	if err := db.Where("album_id = ?", albums[0].ID).Delete(&database.PersonAlbum{}).Error; err != nil {
		t.Fatal(err)
	}
	if r := call("POST", "albums/csv-import", body, p.Token); r.Code != 409 {
		t.Fatal("stale association preview accepted")
	}
	// Force a later association write to fail; all newly created entities roll back.
	beforePeople, beforeSeries := count(&database.Person{}), count(&database.Series{})
	body = encode(row(0, "回滚系列", "回滚模特"), row(1, "回滚系列", "回滚模特"))
	p = preview(body)
	if err := db.Exec(fmt.Sprintf("CREATE TRIGGER reject_csv_link BEFORE INSERT ON person_albums WHEN NEW.album_id = %d BEGIN SELECT RAISE(ABORT, 'forced failure'); END", albums[1].ID)).Error; err != nil {
		t.Fatal(err)
	}
	if r := call("POST", "albums/csv-import", body, p.Token); r.Code == 200 {
		t.Fatal("forced association error succeeded")
	}
	if count(&database.Person{}) != beforePeople || count(&database.Series{}) != beforeSeries || count(&database.PersonAlbum{}) != 0 {
		t.Fatal("failed import left entities or links behind")
	}
}
