package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"airy/internal/config"
	"airy/internal/database"
	"github.com/gin-gonic/gin"
)

func TestCreateDirectorySplitsAlbumParents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "roots.db")})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	root := t.TempDir()
	for _, path := range []string{"A/one", "A/two", "B/one"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	router := newRouter(db, "sqlite", nil, nil, false)
	for pass := range 2 {
		body, _ := json.Marshal(map[string]any{"path": root, "album_depth": 2})
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/directories", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		wantCode := http.StatusCreated
		if pass == 1 {
			wantCode = http.StatusOK
		}
		if response.Code != wantCode {
			t.Fatalf("%d: %s", response.Code, response.Body.String())
		}
		var result struct {
			Directories []directoryResponse
			Created     int
			Existing    int
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Directories) != 2 || result.Created != 2*(1-pass) || result.Existing != 2*pass {
			t.Fatalf("unexpected %+v", result)
		}
		for i, name := range []string{"A", "B"} {
			directory := result.Directories[i]
			if directory.Name != name || directory.Path != filepath.Join(root, name) || directory.AlbumDepth != 1 {
				t.Fatalf("unexpected %+v", directory)
			}
		}
	}
	var count int64
	db.Model(&database.AssetDirectory{}).Count(&count)
	if count != 2 {
		t.Fatalf("got %d resource roots", count)
	}
}
