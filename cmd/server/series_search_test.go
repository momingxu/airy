package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"airy/internal/config"
	"airy/internal/database"
)

func TestSeriesAlbumSearchRegex(t *testing.T) {
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "regex.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	directory := database.AssetDirectory{DirectoryName: "search", DirectoryPath: "/search"}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	series := database.Series{Name: "MichaelWu"}
	if err := db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	albums := []database.Album{
		{DirectoryID: directory.ID, AlbumName: "MichaelWu 海边", AlbumPath: "/search/1", ScanStatus: true, SeriesID: &series.ID},
		{DirectoryID: directory.ID, AlbumName: "michaelwu 泳池", AlbumPath: "/search/2", ScanStatus: true},
		{DirectoryID: directory.ID, AlbumName: "Other 海边", AlbumPath: "/search/3", ScanStatus: true},
		{DirectoryID: directory.ID, AlbumName: "MichaelWu [特辑]", AlbumPath: "/search/4", ScanStatus: true},
		{DirectoryID: directory.ID, AlbumName: "MichaelWu 海边 hidden", AlbumPath: "/search/5", ScanStatus: false},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	cases := []struct {
		name, query, regex string
		want               []uint
		bad                bool
	}{
		{name: "anchors alternation and case", query: `^MichaelWu.*(海边|泳池)$`, regex: "true", want: []uint{albums[1].ID, albums[0].ID}},
		{name: "literal brackets", query: `\[特辑\]$`, regex: "true", want: []uint{albums[3].ID}},
		{name: "no match", query: `^Missing$`, regex: "true", want: []uint{}},
		{name: "all visible", query: `.*`, regex: "true", want: []uint{albums[3].ID, albums[2].ID, albums[1].ID, albums[0].ID}},
		{name: "plain keywords", query: `海边 michaelwu`, regex: "false", want: []uint{albums[0].ID}},
		{name: "plain brackets", query: `[特辑]`, regex: "false", want: []uint{albums[3].ID}},
		{name: "invalid regex", query: `(`, regex: "true", bad: true},
		{name: "unsupported syntax", query: `(?=海边)`, regex: "true", bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := url.Values{"q": {tc.query}, "regex": {tc.regex}}
			req := httptest.NewRequest(http.MethodGet, "/api/series/"+formatUint(series.ID)+"/album-options?"+query.Encode(), nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if tc.bad {
				if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "正则表达式无效") {
					t.Fatalf("invalid pattern response: %d %s", recorder.Code, recorder.Body.String())
				}
				return
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("search response: %d %s", recorder.Code, recorder.Body.String())
			}
			var result struct {
				Albums []seriesAlbumOptionResponse `json:"albums"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Albums) != len(tc.want) {
				t.Fatalf("unexpected results: %s", recorder.Body.String())
			}
			for i, album := range result.Albums {
				if album.ID != tc.want[i] || album.Associated != (album.ID == albums[0].ID) {
					t.Fatalf("unexpected album or association: %+v", album)
				}
			}
		})
	}
}
