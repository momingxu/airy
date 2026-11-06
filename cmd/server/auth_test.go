package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"airy/internal/config"
	"airy/internal/database"

	"github.com/gin-gonic/gin"
)

func TestSingleUserAuthenticationLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "auth.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	router := newRouter(db, "sqlite", nil, nil)

	response := performJSONRequest(t, router, http.MethodGet, "/api/auth/status", nil, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"configured":false`) {
		t.Fatalf("unexpected initial status %d: %s", response.Code, response.Body.String())
	}
	response = performJSONRequest(t, router, http.MethodGet, "/api/gallery", nil, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected protected gallery to return 401, got %d: %s", response.Code, response.Body.String())
	}

	response = performJSONRequest(t, router, http.MethodPost, "/api/auth/setup", map[string]string{
		"username": "admin", "password": "strong-password",
	}, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("setup status %d: %s", response.Code, response.Body.String())
	}
	sessionCookie := findSessionCookie(t, response)

	response = performJSONRequest(t, router, http.MethodPost, "/api/auth/setup", map[string]string{
		"username": "other", "password": "another-password",
	}, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("expected duplicate setup conflict, got %d: %s", response.Code, response.Body.String())
	}
	response = performJSONRequest(t, router, http.MethodGet, "/api/gallery", nil, sessionCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated gallery status %d: %s", response.Code, response.Body.String())
	}

	response = performJSONRequest(t, router, http.MethodPatch, "/api/settings/account", map[string]string{
		"username": "owner", "current_password": "strong-password", "new_password": "new-strong-password",
	}, sessionCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("update account status %d: %s", response.Code, response.Body.String())
	}
	newCookie := findSessionCookie(t, response)
	response = performJSONRequest(t, router, http.MethodGet, "/api/gallery", nil, sessionCookie)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected old session invalidation, got %d", response.Code)
	}
	response = performJSONRequest(t, router, http.MethodGet, "/api/gallery", nil, newCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("replacement session status %d: %s", response.Code, response.Body.String())
	}

	response = performJSONRequest(t, router, http.MethodPost, "/api/auth/logout", nil, newCookie)
	if response.Code != http.StatusNoContent {
		t.Fatalf("logout status %d: %s", response.Code, response.Body.String())
	}
	response = performJSONRequest(t, router, http.MethodPost, "/api/auth/login", map[string]string{
		"username": "owner", "password": "new-strong-password",
	}, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("login status %d: %s", response.Code, response.Body.String())
	}
}

func performJSONRequest(
	t *testing.T,
	handler http.Handler,
	method, path string,
	body any,
	cookie *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, requestBody)
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func findSessionCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			return cookie
		}
	}
	t.Fatal("session cookie was not set")
	return nil
}
