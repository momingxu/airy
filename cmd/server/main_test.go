package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverDefaultMediaRootsFromDockerMounts(t *testing.T) {
	mountRoot := t.TempDir()
	for _, name := range []string{"Library B", "Library A", ".hidden"} {
		if err := os.Mkdir(filepath.Join(mountRoot, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(mountRoot, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(mountRoot, "Library A"), filepath.Join(mountRoot, "linked-library")); err != nil {
		t.Fatal(err)
	}

	roots, err := discoverDockerMediaRoots(true, mountRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(mountRoot, "Library A"),
		filepath.Join(mountRoot, "Library B"),
	}
	if !reflect.DeepEqual(roots, want) {
		t.Fatalf("unexpected Docker media roots: got %v want %v", roots, want)
	}
}

func TestDiscoverDockerMediaRootsDoesNothingOutsideDocker(t *testing.T) {
	roots, err := discoverDockerMediaRoots(false, filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(roots) != 0 {
		t.Fatalf("host mode should not discover media roots: roots=%v err=%v", roots, err)
	}
}

func TestRedirectToHTTPSPreservesHostPathAndQuery(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://photos.example:8080/albums/42?name=a%20b", nil)
	response := httptest.NewRecorder()

	redirectToHTTPS(response, request)

	if response.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusPermanentRedirect, response.Code)
	}
	if location := response.Header().Get("Location"); location != "https://photos.example:8080/albums/42?name=a%20b" {
		t.Fatalf("unexpected redirect location %q", location)
	}
}

func TestRedirectToHTTPSRejectsMissingHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = ""
	response := httptest.NewRecorder()

	redirectToHTTPS(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}
