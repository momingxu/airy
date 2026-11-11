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

func TestAlbumCSVWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "csv.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	dirs := []database.AssetDirectory{{DirectoryName: "one", DirectoryPath: "/one"}, {DirectoryName: "two", DirectoryPath: "/two"}}
	if err := db.Create(&dirs).Error; err != nil {
		t.Fatal(err)
	}
	albums := []database.Album{
		{DirectoryID: dirs[0].ID, AlbumName: "中文,\"标题\"\n第二行", AlbumPath: "/one/a", PublishedDate: "2023-12-31"},
		{DirectoryID: dirs[0].ID, AlbumName: "=1+1", AlbumPath: "/one/b", PublishedDate: "2024-02-29"},
		{DirectoryID: dirs[1].ID, AlbumName: "other", AlbumPath: "/two/a", PublishedDate: "2025-01-01"},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	base := fmt.Sprintf("/api/directories/%d/", dirs[0].ID)
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "text/csv")
		req.Header.Set("X-Preview-Token", token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	encode := func(rows ...[]string) string {
		var b bytes.Buffer
		b.WriteString("\xef\xbb\xbf")
		w := csv.NewWriter(&b)
		_ = w.Write(albumCSVLegacyHeader)
		for _, r := range rows {
			_ = w.Write(r)
		}
		w.Flush()
		return b.String()
	}
	id := func(i int) string { return fmt.Sprint(albums[i].ID) }
	preview := func(body string) albumCSVPreview {
		rec := call("POST", base+"albums/csv-preview", body, "")
		if rec.Code != 200 {
			t.Fatalf("preview %d: %s", rec.Code, rec.Body.String())
		}
		var p albumCSVPreview
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	load := func(i int) database.Album {
		var a database.Album
		if err := db.First(&a, albums[i].ID).Error; err != nil {
			t.Fatal(err)
		}
		return a
	}
	exported := call("GET", base+"albums.csv", "", "")
	if exported.Code != 200 || !strings.HasPrefix(exported.Body.String(), "\ufeff") {
		t.Fatalf("export: %s", exported.Body.String())
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(exported.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(records) != 3 || len(records[0]) != 5 || records[1][1] != albums[0].AlbumName || records[2][1] != "'=1+1" {
		t.Fatalf("export records: %v %v", records, err)
	}
	roundtrip := preview(exported.Body.String())
	if !roundtrip.Valid || roundtrip.Changes != 0 {
		t.Fatalf("roundtrip: %+v", roundtrip)
	}
	body := encode([]string{id(0), "新标题,保留日期", ""}, []string{id(1), "新标题二", "2026-02-05"})
	p := preview(body)
	if !p.Valid || p.Changes != 2 || load(0).AlbumName != albums[0].AlbumName {
		t.Fatal("preview changed data or invalid preview")
	}
	applied := call("POST", base+"albums/csv-import", body, p.Token)
	if applied.Code != 200 || load(0).PublishedDate != "2023-12-31" || load(1).PublishedDate != "2026-02-05" {
		t.Fatalf("apply: %s", applied.Body.String())
	}
	// Global export includes both resource directories; import matches solely by ID.
	globalExport := call("GET", "/api/albums.csv", "", "")
	globalRecords, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(globalExport.Body.String(), "\ufeff"))).ReadAll()
	if globalExport.Code != 200 || err != nil || len(globalRecords) != 4 {
		t.Fatalf("global export: %d %v %v", globalExport.Code, globalRecords, err)
	}
	globalBody := encode([]string{id(0), "新标题,保留日期", ""}, []string{id(2), "跨资源库修改", "2026-03-04"})
	globalPreview := call("POST", "/api/albums/csv-preview", globalBody, "")
	var gp albumCSVPreview
	if err := json.Unmarshal(globalPreview.Body.Bytes(), &gp); err != nil {
		t.Fatal(err)
	}
	if globalPreview.Code != 200 || !gp.Valid || gp.Changes != 1 {
		t.Fatalf("global preview: %s", globalPreview.Body.String())
	}
	globalApply := call("POST", "/api/albums/csv-import", globalBody, gp.Token)
	if globalApply.Code != 200 || load(2).AlbumName != "跨资源库修改" || load(2).PublishedDate != "2026-03-04" {
		t.Fatalf("global apply: %s", globalApply.Body.String())
	}
	invalidGlobal := encode([]string{id(0), "不应保存", ""}, []string{"999999", "不存在", ""})
	response := call("POST", "/api/albums/csv-preview", invalidGlobal, "")
	var invalidPreview albumCSVPreview
	if err := json.Unmarshal(response.Body.Bytes(), &invalidPreview); err != nil {
		t.Fatal(err)
	}
	if invalidPreview.Valid || !strings.Contains(invalidPreview.Rows[1].Error, "不存在") {
		t.Fatal("unknown global ID accepted")
	}
	if rec := call("POST", "/api/albums/csv-import", invalidGlobal, invalidPreview.Token); rec.Code != 400 || load(0).AlbumName != "新标题,保留日期" {
		t.Fatal("invalid global import wrote data")
	}
	for _, bad := range []string{
		encode([]string{id(0), "valid", "2026-01-01"}, []string{id(1), "invalid", "2026-02-30"}),
		encode([]string{id(0), "first", ""}, []string{id(0), "duplicate", ""}),
		encode([]string{id(2), "foreign", ""}),
		encode([]string{"0", "zero", ""}),
		encode([]string{id(0), " ", ""}),
		encode([]string{id(0), strings.Repeat("中", 256), ""}),
	} {
		invalid := preview(bad)
		if invalid.Valid {
			t.Fatal("invalid CSV accepted")
		}
		response := call("POST", base+"albums/csv-import", bad, invalid.Token)
		if response.Code != 400 || load(0).AlbumName != "新标题,保留日期" {
			t.Fatal("invalid import was not rejected atomically")
		}
	}
	if rec := call("POST", base+"albums/csv-preview", "相册ID,相册路径,相册标题,发布日期\n1,/path,title,2026-01-01", ""); rec.Code != 400 {
		t.Fatal("path column accepted")
	}
	// A concurrent edit must invalidate the preview, not overwrite the newer value.
	body = encode([]string{id(0), "stale overwrite", ""})
	p = preview(body)
	if err := db.Model(&albums[0]).UpdateColumn("album_name", "concurrent edit").Error; err != nil {
		t.Fatal(err)
	}
	if rec := call("POST", base+"albums/csv-import", body, p.Token); rec.Code != 409 || load(0).AlbumName != "concurrent edit" {
		t.Fatal("stale preview accepted")
	}
	// Force the second update to fail and verify that the first is rolled back.
	body = encode([]string{id(0), "first write", ""}, []string{id(1), "force failure", ""})
	p = preview(body)
	if err := db.Exec("CREATE TRIGGER reject_csv_title BEFORE UPDATE OF album_name ON albums WHEN NEW.album_name = 'force failure' BEGIN SELECT RAISE(ABORT, 'forced failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if rec := call("POST", base+"albums/csv-import", body, p.Token); rec.Code == 200 || load(0).AlbumName != "concurrent edit" {
		t.Fatal("partial CSV import persisted")
	}
	if rec := call("POST", base+"albums/csv-preview", strings.Repeat("x", albumCSVLimit+1), ""); rec.Code != 413 {
		t.Fatal("oversize upload accepted")
	}
}
