package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFrontendHandlerServesAssetsAndSPAFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<main>Airy</main>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("window.Airy=true"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sw.js"), []byte("self.skipWaiting()"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.webmanifest"), []byte(`{"name":"Airy"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.NoRoute(frontendFileSystemHandler(os.DirFS(root)))

	for _, path := range []string{"/", "/albums/42"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Airy") {
			t.Fatalf("SPA request %s failed: %d %s", path, recorder.Code, recorder.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset response failed: %d %q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}

	request = httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("service worker response failed: %d %q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}

	request = httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest response failed: %d %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
}
