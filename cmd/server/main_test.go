package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
