package main

import (
	"errors"
	"net/http"
	"time"

	"airy/internal/database"
	"airy/internal/scanner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type scanScheduleResponse struct {
	Enabled    bool       `json:"enabled"`
	ScanTime   string     `json:"scan_time"`
	Timezone   string     `json:"timezone"`
	NextScanAt *time.Time `json:"next_scan_at,omitempty"`
}

func cancelScanHandler(mediaScanner *scanner.Scanner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if mediaScanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "扫描服务不可用"})
			return
		}
		status, err := mediaScanner.Cancel()
		if errors.Is(err, scanner.ErrNotRunning) {
			c.JSON(http.StatusConflict, gin.H{"error": "当前没有正在进行的扫描", "scan": status})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "scan": status})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"scan": status})
	}
}

func getScanScheduleHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		schedule, err := loadScanSchedule(c, db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取定时扫描设置失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"schedule": buildScanScheduleResponse(schedule)})
	}
}

func updateScanScheduleHandler(db *gorm.DB, mediaScanner *scanner.Scanner) gin.HandlerFunc {
	type requestBody struct {
		Enabled *bool `json:"enabled"`
	}

	return func(c *gin.Context) {
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的定时扫描设置"})
			return
		}
		schedule, err := loadScanSchedule(c, db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取定时扫描设置失败"})
			return
		}
		schedule.Enabled = *body.Enabled
		if schedule.Enabled {
			next := scanner.NextDailyScan(time.Now())
			schedule.NextScanAt = &next
		} else {
			schedule.NextScanAt = nil
		}
		if err := db.WithContext(c.Request.Context()).Model(&schedule).Updates(map[string]any{
			"enabled":      schedule.Enabled,
			"next_scan_at": schedule.NextScanAt,
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存定时扫描设置失败"})
			return
		}
		if mediaScanner != nil {
			mediaScanner.RefreshAutomation()
		}
		c.JSON(http.StatusOK, gin.H{"schedule": buildScanScheduleResponse(schedule)})
	}
}

func loadScanSchedule(c *gin.Context, db *gorm.DB) (database.ScanSchedule, error) {
	schedule := database.ScanSchedule{ID: 1}
	err := db.WithContext(c.Request.Context()).FirstOrCreate(&schedule, database.ScanSchedule{ID: 1}).Error
	return schedule, err
}

func buildScanScheduleResponse(schedule database.ScanSchedule) scanScheduleResponse {
	return scanScheduleResponse{
		Enabled:    schedule.Enabled,
		ScanTime:   scanner.DailyScanTime,
		Timezone:   scanner.DailyScanTimezone,
		NextScanAt: schedule.NextScanAt,
	}
}
