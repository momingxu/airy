package main

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"airy/internal/database"
	"airy/internal/scanner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type directoryResponse struct {
	ID                 uint       `json:"id"`
	Name               string     `json:"name"`
	Path               string     `json:"path"`
	AlbumDepth         uint       `json:"album_depth"`
	ScanStatus         bool       `json:"scan_status"`
	LastScannedAt      *time.Time `json:"last_scanned_at,omitempty"`
	ScanError          string     `json:"scan_error,omitempty"`
	AutomationDisabled bool       `json:"automation_disabled"`
	Watching           bool       `json:"watching"`
	WatchError         string     `json:"watch_error,omitempty"`
	AlbumCount         int64      `json:"album_count"`
	MediaCount         int64      `json:"media_count"`
	Scanning           bool       `json:"scanning"`
	Queued             bool       `json:"queued"`
}

func listDirectoriesHandler(db *gorm.DB, mediaScanner *scanner.Scanner) gin.HandlerFunc {
	return func(c *gin.Context) {
		var directories []database.AssetDirectory
		if err := db.WithContext(c.Request.Context()).Order("created_at ASC").Find(&directories).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取资源文件夹失败"})
			return
		}

		status := scanner.Status{State: scanner.StateIdle}
		if mediaScanner != nil {
			status = mediaScanner.Status()
		}
		albumCounts, mediaCounts, err := loadDirectoryCounts(c, db, directories)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取资源文件夹统计失败"})
			return
		}
		items := make([]directoryResponse, 0, len(directories))
		for _, directory := range directories {
			items = append(items, buildDirectoryResponse(
				directory, status, albumCounts[directory.ID], mediaCounts[directory.ID], mediaScanner,
			))
		}

		c.JSON(http.StatusOK, gin.H{"directories": items, "scan": status})
	}
}

func createDirectoryHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Path       string `json:"path"`
		AlbumDepth *uint  `json:"album_depth"`
	}

	return func(c *gin.Context) {
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Path) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入资源文件夹路径"})
			return
		}
		if !filepath.IsAbs(strings.TrimSpace(body.Path)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "资源文件夹必须使用绝对路径"})
			return
		}
		albumDepth := uint(1)
		if body.AlbumDepth != nil {
			albumDepth = *body.AlbumDepth
		}
		if albumDepth < 1 || albumDepth > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册目录深度必须在 1 到 10 之间"})
			return
		}

		resolved, err := scanner.ResolveRoot(body.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		roots, err := scanner.ResourceRootsForAlbumDepth(c.Request.Context(), resolved, albumDepth)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取相册目录失败：" + err.Error()})
			return
		}
		if len(roots) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "所选层级下没有相册目录，请检查路径和相册深度"})
			return
		}
		items := make([]directoryResponse, 0, len(roots))
		var created int64
		err = db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			for _, root := range roots {
				var directory database.AssetDirectory
				result := tx.Where("directory_path = ?", root).FirstOrCreate(&directory, database.AssetDirectory{
					DirectoryName: filepath.Base(root), DirectoryPath: root, AlbumDepth: 1,
				})
				if result.Error != nil {
					return result.Error
				}
				created += result.RowsAffected
				items = append(items, directoryResponse{
					ID: directory.ID, Name: directory.DirectoryName, Path: directory.DirectoryPath,
					AlbumDepth: normalizedAlbumDepth(directory.AlbumDepth), ScanStatus: directory.ScanStatus,
					LastScannedAt: directory.LastScannedAt, ScanError: directory.ScanError,
					AutomationDisabled: directory.AutomationDisabled,
				})
			}
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "添加资源文件夹失败"})
			return
		}
		code := http.StatusOK
		if created > 0 {
			code = http.StatusCreated
		}
		c.JSON(code, gin.H{"directory": items[0], "directories": items, "created": created, "existing": int64(len(items)) - created})
	}
}

func updateDirectorySettingsHandler(db *gorm.DB, mediaScanner *scanner.Scanner) gin.HandlerFunc {
	type requestBody struct {
		AutomationDisabled *bool `json:"automation_disabled"`
	}

	return func(c *gin.Context) {
		directory, ok := findDirectory(c, db)
		if !ok {
			return
		}
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || body.AutomationDisabled == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的资源文件夹设置"})
			return
		}
		updates := map[string]any{"automation_disabled": *body.AutomationDisabled}
		if err := db.WithContext(c.Request.Context()).Model(&directory).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存资源文件夹设置失败"})
			return
		}
		if err := db.WithContext(c.Request.Context()).First(&directory, directory.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取资源文件夹设置失败"})
			return
		}
		if mediaScanner != nil {
			mediaScanner.RefreshAutomation()
		}
		status := scanner.Status{State: scanner.StateIdle}
		if mediaScanner != nil {
			status = mediaScanner.Status()
		}
		response, err := loadDirectoryResponse(c, db, directory, status, mediaScanner)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取资源文件夹统计失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"directory": response})
	}
}

func scanDirectoryHandler(db *gorm.DB, mediaScanner *scanner.Scanner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if mediaScanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "扫描服务不可用"})
			return
		}
		directory, ok := findDirectory(c, db)
		if !ok {
			return
		}

		status, err := mediaScanner.StartPath(directory.DirectoryPath)
		if errors.Is(err, scanner.ErrAlreadyRunning) {
			c.JSON(http.StatusConflict, gin.H{"error": "已有资源文件夹正在扫描", "scan": status})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"scan": status})
	}
}

func deleteDirectoryHandler(db *gorm.DB, mediaScanner *scanner.Scanner) gin.HandlerFunc {
	return func(c *gin.Context) {
		directory, ok := findDirectory(c, db)
		if !ok {
			return
		}
		if mediaScanner != nil {
			status := mediaScanner.Status()
			if status.State == scanner.StateScanning && status.Root == directory.DirectoryPath {
				c.JSON(http.StatusConflict, gin.H{"error": "资源文件夹正在扫描，暂时不能移除"})
				return
			}
		}

		err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			var albums []database.Album
			if err := tx.Select("id", "series_id").Where("directory_id = ?", directory.ID).
				Find(&albums).Error; err != nil {
				return err
			}
			albumIDs := make([]uint, 0, len(albums))
			seriesIDs := make([]uint, 0, len(albums))
			for _, album := range albums {
				albumIDs = append(albumIDs, album.ID)
				if album.SeriesID != nil {
					seriesIDs = append(seriesIDs, *album.SeriesID)
				}
			}
			var personIDs []uint
			if len(albumIDs) > 0 {
				if err := tx.Model(&database.PersonAlbum{}).Distinct().Where("album_id IN ?", albumIDs).
					Pluck("person_id", &personIDs).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("album_id IN ?", albumIDs).Delete(&database.PersonAlbum{}).Error; err != nil {
				return err
			}
			if err := tx.Where("album_id IN ?", albumIDs).Delete(&database.Photo{}).Error; err != nil {
				return err
			}
			if err := tx.Where("directory_id = ?", directory.ID).Delete(&database.Album{}).Error; err != nil {
				return err
			}
			if err := database.RefreshPersonAvatarPhotoIDs(tx, personIDs); err != nil {
				return err
			}
			if err := database.RefreshSeriesCoverPhotoIDs(tx, uniqueUintIDs(seriesIDs)); err != nil {
				return err
			}
			return tx.Delete(&directory).Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "移除资源文件夹失败"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func findDirectory(c *gin.Context, db *gorm.DB) (database.AssetDirectory, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的资源文件夹编号"})
		return database.AssetDirectory{}, false
	}
	var directory database.AssetDirectory
	if err := db.WithContext(c.Request.Context()).First(&directory, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "资源文件夹不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取资源文件夹失败"})
		}
		return database.AssetDirectory{}, false
	}
	return directory, true
}

func loadDirectoryResponse(c *gin.Context, db *gorm.DB, directory database.AssetDirectory, scanStatus scanner.Status, mediaScanner ...*scanner.Scanner) (directoryResponse, error) {
	var albumCount int64
	if err := db.WithContext(c.Request.Context()).Model(&database.Album{}).
		Where("directory_id = ? AND scan_status = ?", directory.ID, true).Count(&albumCount).Error; err != nil {
		return directoryResponse{}, err
	}

	var mediaCount int64
	if err := db.WithContext(c.Request.Context()).Table("photos").
		Joins("JOIN albums ON albums.id = photos.album_id").
		Where("albums.directory_id = ? AND photos.status = ?", directory.ID, scanner.AssetStatusActive).
		Count(&mediaCount).Error; err != nil {
		return directoryResponse{}, err
	}

	return buildDirectoryResponse(directory, scanStatus, albumCount, mediaCount, mediaScanner...), nil
}

type groupedDirectoryCount struct {
	DirectoryID uint
	Count       int64
}

func loadDirectoryCounts(c *gin.Context, db *gorm.DB, directories []database.AssetDirectory) (map[uint]int64, map[uint]int64, error) {
	albumCounts := make(map[uint]int64, len(directories))
	mediaCounts := make(map[uint]int64, len(directories))
	if len(directories) == 0 {
		return albumCounts, mediaCounts, nil
	}

	ids := make([]uint, 0, len(directories))
	for _, directory := range directories {
		ids = append(ids, directory.ID)
	}
	var albums []groupedDirectoryCount
	if err := db.WithContext(c.Request.Context()).Model(&database.Album{}).
		Select("directory_id, COUNT(*) AS count").
		Where("directory_id IN ? AND scan_status = ?", ids, true).
		Group("directory_id").Scan(&albums).Error; err != nil {
		return nil, nil, err
	}
	for _, item := range albums {
		albumCounts[item.DirectoryID] = item.Count
	}

	var media []groupedDirectoryCount
	if err := db.WithContext(c.Request.Context()).Table("photos").
		Select("albums.directory_id AS directory_id, COUNT(*) AS count").
		Joins("JOIN albums ON albums.id = photos.album_id").
		Where("albums.directory_id IN ? AND photos.status = ?", ids, scanner.AssetStatusActive).
		Group("albums.directory_id").Scan(&media).Error; err != nil {
		return nil, nil, err
	}
	for _, item := range media {
		mediaCounts[item.DirectoryID] = item.Count
	}
	return albumCounts, mediaCounts, nil
}

func buildDirectoryResponse(directory database.AssetDirectory, scanStatus scanner.Status, albumCount, mediaCount int64, mediaScanner ...*scanner.Scanner) directoryResponse {
	automation := scanner.AutomationStatus{}
	queued := false
	if len(mediaScanner) > 0 && mediaScanner[0] != nil {
		automation = mediaScanner[0].DirectoryAutomationStatus(directory.ID)
		_, queued = mediaScanner[0].PathStatus(directory.DirectoryPath)
	}

	return directoryResponse{
		ID:                 directory.ID,
		Name:               directory.DirectoryName,
		Path:               directory.DirectoryPath,
		AlbumDepth:         normalizedAlbumDepth(directory.AlbumDepth),
		ScanStatus:         directory.ScanStatus,
		LastScannedAt:      directory.LastScannedAt,
		ScanError:          directory.ScanError,
		AutomationDisabled: directory.AutomationDisabled,
		Watching:           automation.Watching,
		WatchError:         automation.WatchError,
		AlbumCount:         albumCount,
		MediaCount:         mediaCount,
		Scanning:           scanStatus.State == scanner.StateScanning && scanStatus.Root == directory.DirectoryPath,
		Queued:             queued,
	}
}

func normalizedAlbumDepth(depth uint) uint {
	if depth == 0 {
		return 1
	}
	return depth
}
