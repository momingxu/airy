package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBrowseDirectoriesBoundary(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "photos"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.jpg"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/browse", browseDirectoriesHandler(root))
	for _, tc := range []struct {
		path   string
		status int
	}{{root, 200}, {outside, 403}, {filepath.Join(root, "escape"), 403}, {filepath.Join(root, "photos"), 200}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/browse?path="+tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: status %d, body %s", tc.path, w.Code, w.Body.String())
		}
		if tc.path == root {
			var body struct {
				Directories []struct {
					Name string `json:"name"`
				} `json:"directories"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Directories) != 1 || body.Directories[0].Name != "photos" {
				t.Fatalf("unexpected folders: %s", w.Body.String())
			}
		}
	}
}
