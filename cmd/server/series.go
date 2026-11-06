package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"airy/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type seriesResponse struct {
	ID         uint      `json:"id"`
	Name       string    `json:"name"`
	Favorite   bool      `json:"favorite"`
	CoverURL   string    `json:"cover_url"`
	AlbumCount int64     `json:"album_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type seriesAlbumCountRow struct {
	SeriesID   uint
	AlbumCount int64
}

func loadSeriesResponses(ctx context.Context, db *gorm.DB, series []database.Series) ([]seriesResponse, error) {
	items := make([]seriesResponse, 0, len(series))
	if len(series) == 0 {
		return items, nil
	}

	seriesIDs := make([]uint, 0, len(series))
	for _, item := range series {
		seriesIDs = append(seriesIDs, item.ID)
	}

	var countRows []seriesAlbumCountRow
	if err := db.WithContext(ctx).Model(&database.Album{}).
		Select("series_id, COUNT(*) AS album_count").
		Where("series_id IN ? AND scan_status = ?", seriesIDs, true).
		Group("series_id").Scan(&countRows).Error; err != nil {
		return nil, err
	}
	albumCounts := make(map[uint]int64, len(countRows))
	for _, row := range countRows {
		albumCounts[row.SeriesID] = row.AlbumCount
	}

	for _, item := range series {
		coverURL := ""
		if item.CoverPhotoID != nil {
			coverURL = fmt.Sprintf("/api/media/%d/thumbnail", *item.CoverPhotoID)
		}
		items = append(items, seriesResponse{
			ID: item.ID, Name: item.Name, Favorite: item.Favorite,
			CoverURL: coverURL, AlbumCount: albumCounts[item.ID], CreatedAt: item.CreatedAt,
		})
	}
	return items, nil
}

func listSeriesHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var series []database.Series
		if err := db.WithContext(c.Request.Context()).Order("name ASC").Find(&series).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列失败"})
			return
		}
		items, err := loadSeriesResponses(c.Request.Context(), db, series)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"series": items})
	}
}

func createSeriesHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Name     string `json:"name"`
		AlbumIDs []uint `json:"album_ids"`
	}
	return func(c *gin.Context) {
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入系列名称"})
			return
		}
		name := strings.TrimSpace(body.Name)
		item := database.Series{}
		created := false
		err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			result := tx.Where("name = ?", name).FirstOrCreate(&item, database.Series{Name: name})
			if result.Error != nil {
				return result.Error
			}
			created = result.RowsAffected > 0
			seriesToRefresh := []uint{item.ID}
			if len(body.AlbumIDs) > 0 {
				var albums []database.Album
				if err := tx.Where("id IN ? AND scan_status = ?", body.AlbumIDs, true).
					Find(&albums).Error; err != nil {
					return err
				}
				albumIDs := make([]uint, 0, len(albums))
				for _, album := range albums {
					albumIDs = append(albumIDs, album.ID)
					if album.SeriesID != nil && *album.SeriesID != item.ID {
						seriesToRefresh = append(seriesToRefresh, *album.SeriesID)
					}
				}
				if len(albumIDs) > 0 {
					if err := tx.Model(&database.Album{}).Where("id IN ?", albumIDs).
						UpdateColumn("series_id", item.ID).Error; err != nil {
						return err
					}
				}
			}
			return database.RefreshSeriesCoverPhotoIDs(tx, uniqueUintIDs(seriesToRefresh))
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "添加系列失败"})
			return
		}
		if err := db.WithContext(c.Request.Context()).First(&item, item.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列信息失败"})
			return
		}
		items, err := loadSeriesResponses(c.Request.Context(), db, []database.Series{item})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列相册统计失败"})
			return
		}
		code := http.StatusCreated
		if !created {
			code = http.StatusOK
		}
		c.JSON(code, gin.H{"series": items[0]})
	}
}

func updateSeriesHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Name           *string `json:"name"`
		Favorite       *bool   `json:"favorite"`
		AlbumIDs       *[]uint `json:"album_ids"`
		RemoveAlbumIDs []uint  `json:"remove_album_ids"`
	}
	return func(c *gin.Context) {
		item, ok := findSeries(c, db)
		if !ok {
			return
		}
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || (body.Name == nil && body.Favorite == nil && body.AlbumIDs == nil && len(body.RemoveAlbumIDs) == 0) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入需要修改的系列信息"})
			return
		}
		updates := map[string]any{}
		if body.Name != nil {
			name := strings.TrimSpace(*body.Name)
			if name == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "系列名称不能为空"})
				return
			}
			updates["name"] = name
			item.Name = name
		}
		if body.Favorite != nil {
			updates["favorite"] = *body.Favorite
			item.Favorite = *body.Favorite
		}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if len(updates) > 0 {
				if err := tx.Model(&item).Updates(updates).Error; err != nil {
					return err
				}
			}
			if len(body.RemoveAlbumIDs) > 0 {
				if err := tx.Model(&database.Album{}).Where("series_id = ? AND id IN ?", item.ID, body.RemoveAlbumIDs).UpdateColumn("series_id", nil).Error; err != nil {
					return err
				}
			}
			if body.AlbumIDs != nil && len(*body.AlbumIDs) > 0 {
				var albums []database.Album
				if err := tx.Where("id IN ? AND scan_status = ?", *body.AlbumIDs, true).Find(&albums).Error; err != nil {
					return err
				}
				albumIDs := make([]uint, 0, len(albums))
				seriesToRefresh := []uint{item.ID}
				for _, album := range albums {
					albumIDs = append(albumIDs, album.ID)
					if album.SeriesID != nil {
						seriesToRefresh = append(seriesToRefresh, *album.SeriesID)
					}
				}
				if len(albumIDs) > 0 {
					if err := tx.Model(&database.Album{}).Where("id IN ?", albumIDs).UpdateColumn("series_id", item.ID).Error; err != nil {
						return err
					}
					if err := database.RefreshSeriesCoverPhotoIDs(tx, uniqueUintIDs(seriesToRefresh)); err != nil {
						return err
					}
				}
			}
			if len(body.RemoveAlbumIDs) > 0 {
				if err := database.RefreshSeriesCoverPhotoIDs(tx, []uint{item.ID}); err != nil {
					return err
				}
			}
			return tx.First(&item, item.ID).Error
		}); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "更新系列失败，名称可能已经存在"})
			return
		}
		items, err := loadSeriesResponses(c.Request.Context(), db, []database.Series{item})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"series": items[0]})
	}
}

func deleteSeriesHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, ok := findSeries(c, db)
		if !ok {
			return
		}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&database.Album{}).Where("series_id = ?", item.ID).
				UpdateColumn("series_id", nil).Error; err != nil {
				return err
			}
			return tx.Delete(&item).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除系列失败"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func seriesGalleryHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, ok := findSeries(c, db)
		if !ok {
			return
		}
		offset := boundedQueryInt(c, "offset", 0, 0, 1_000_000)
		limit := boundedQueryInt(c, "limit", 10, 1, 24)
		mediaLimit := boundedQueryInt(c, "media_limit", -1, 1, 48)

		base := db.WithContext(c.Request.Context()).Model(&database.Album{}).
			Where("series_id = ? AND scan_status = ?", item.ID, true)
		var totalAlbums int64
		if err := base.Count(&totalAlbums).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}
		var albums []database.Album
		if err := base.Order("albums.published_date DESC").Order("albums.id DESC").
			Offset(offset).Limit(limit).Find(&albums).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}
		responses := make([]albumResponse, 0, len(albums))
		for _, album := range albums {
			response, err := loadAlbumResponse(c, db, album, 0, mediaLimit)
			if err != nil {
				galleryDatabaseError(c, err)
				return
			}
			responses = append(responses, response)
		}
		var totalMedia int64
		if err := db.WithContext(c.Request.Context()).Model(&database.Photo{}).
			Joins("JOIN albums ON albums.id = photos.album_id").
			Where("albums.series_id = ? AND photos.status = ? AND albums.scan_status = ?", item.ID, "active", true).
			Count(&totalMedia).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}
		items, err := loadSeriesResponses(c.Request.Context(), db, []database.Series{item})
		if err != nil {
			galleryDatabaseError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"series": items[0], "albums": responses, "total_media": totalMedia,
			"pagination": paginationResponse{
				Offset: offset, Limit: limit, Total: totalAlbums, HasMore: int64(offset+len(albums)) < totalAlbums,
			},
		})
	}
}

type seriesAlbumOptionResponse struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Associated bool   `json:"associated"`
}

func seriesAlbumOptionsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, ok := findSeries(c, db)
		if !ok {
			return
		}
		var pattern *regexp.Regexp
		if c.Query("regex") == "true" {
			var err error
			pattern, err = regexp.Compile("(?i)" + c.Query("q"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "正则表达式无效：" + err.Error()})
				return
			}
		}
		query := db.WithContext(c.Request.Context()).Model(&database.Album{}).
			Select("id", "album_name", "series_id").Where("scan_status = ?", true)
		if pattern == nil {
			for _, keyword := range strings.Fields(c.Query("q")) {
				query = query.Where("LOWER(album_name) LIKE ?", "%"+strings.ToLower(keyword)+"%")
			}
		}
		var albums []database.Album
		if err := query.Order("published_date DESC").Order("id DESC").Find(&albums).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "搜索相册失败"})
			return
		}
		items := make([]seriesAlbumOptionResponse, 0, len(albums))
		for _, album := range albums {
			if pattern != nil && !pattern.MatchString(album.AlbumName) {
				continue
			}
			linked := album.SeriesID != nil && *album.SeriesID == item.ID
			items = append(items, seriesAlbumOptionResponse{ID: album.ID, Name: album.AlbumName, Associated: linked})
		}
		c.JSON(http.StatusOK, gin.H{"albums": items})
	}
}

func addSeriesAlbumHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, album, ok := findSeriesAndAlbum(c, db)
		if !ok {
			return
		}
		seriesToRefresh := []uint{item.ID}
		if album.SeriesID != nil && *album.SeriesID != item.ID {
			seriesToRefresh = append(seriesToRefresh, *album.SeriesID)
		}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&database.Album{}).Where("id = ?", album.ID).
				UpdateColumn("series_id", item.ID).Error; err != nil {
				return err
			}
			return database.RefreshSeriesCoverPhotoIDs(tx, uniqueUintIDs(seriesToRefresh))
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "关联系列与相册失败"})
			return
		}
		if err := db.WithContext(c.Request.Context()).First(&item, item.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列信息失败"})
			return
		}
		items, err := loadSeriesResponses(c.Request.Context(), db, []database.Series{item})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"series": items[0]})
	}
}

func removeSeriesAlbumHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, album, ok := findSeriesAndAlbum(c, db)
		if !ok {
			return
		}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&database.Album{}).
				Where("id = ? AND series_id = ?", album.ID, item.ID).
				UpdateColumn("series_id", nil).Error; err != nil {
				return err
			}
			return database.RefreshSeriesCoverPhotoIDs(tx, []uint{item.ID})
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "取消系列与相册关联失败"})
			return
		}
		if err := db.WithContext(c.Request.Context()).First(&item, item.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列信息失败"})
			return
		}
		items, err := loadSeriesResponses(c.Request.Context(), db, []database.Series{item})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"series": items[0]})
	}
}

func uniqueUintIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func findSeries(c *gin.Context, db *gorm.DB) (database.Series, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的系列编号"})
		return database.Series{}, false
	}
	var item database.Series
	if err := db.WithContext(c.Request.Context()).First(&item, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "系列不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取系列失败"})
		}
		return database.Series{}, false
	}
	return item, true
}

func findSeriesAndAlbum(c *gin.Context, db *gorm.DB) (database.Series, database.Album, bool) {
	item, ok := findSeries(c, db)
	if !ok {
		return database.Series{}, database.Album{}, false
	}
	albumID, err := strconv.ParseUint(c.Param("album_id"), 10, 64)
	if err != nil || albumID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的相册编号"})
		return database.Series{}, database.Album{}, false
	}
	var album database.Album
	if err := db.WithContext(c.Request.Context()).First(&album, uint(albumID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "相册不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取相册失败"})
		}
		return database.Series{}, database.Album{}, false
	}
	return item, album, true
}
