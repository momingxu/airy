package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
	"airy/internal/database"
	"airy/internal/scanner"

	"github.com/gin-gonic/gin"
)

func TestScanScheduleHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "schedule.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	router := newRouter(db, "sqlite", nil, nil, false)

	request := httptest.NewRequest(http.MethodGet, "/api/scan/schedule", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get schedule status %d: %s", recorder.Code, recorder.Body.String())
	}
	var initial struct {
		Schedule scanScheduleResponse `json:"schedule"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Schedule.Enabled || initial.Schedule.ScanTime != scanner.DailyScanTime || initial.Schedule.Timezone != scanner.DailyScanTimezone {
		t.Fatalf("unexpected initial schedule: %+v", initial.Schedule)
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/scan/schedule", bytes.NewBufferString(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	before := time.Now().UTC()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update schedule status %d: %s", recorder.Code, recorder.Body.String())
	}
	var updated struct {
		Schedule scanScheduleResponse `json:"schedule"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Schedule.Enabled || updated.Schedule.ScanTime != scanner.DailyScanTime || updated.Schedule.Timezone != scanner.DailyScanTimezone || updated.Schedule.NextScanAt == nil {
		t.Fatalf("unexpected updated schedule: %+v", updated.Schedule)
	}
	wantNext := scanner.NextDailyScan(before)
	if updated.Schedule.NextScanAt.Before(wantNext.Add(-time.Second)) || updated.Schedule.NextScanAt.After(wantNext.Add(time.Second)) {
		t.Fatalf("unexpected next scan time: %v", updated.Schedule.NextScanAt)
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/scan/schedule", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected empty settings status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
