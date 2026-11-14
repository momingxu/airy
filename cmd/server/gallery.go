package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"airy/internal/database"
	mediaservice "airy/internal/media"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type paginationResponse struct {
	Offset  int   `json:"offset"`
	Limit   int   `json:"limit"`
	Total   int64 `json:"total"`
	HasMore bool  `json:"has_more"`
}

type mediaResponse struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Favorite     bool   `json:"favorite"`
	MediaType    string `json:"media_type"`
	MIMEType     string `json:"mime_type"`
	WidthPX      *uint  `json:"width_px,omitempty"`
	HeightPX     *uint  `json:"height_px,omitempty"`
	DurationMS   *int64 `json:"duration_ms,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	ThumbnailURL string `json:"thumbnail_url"`
	OriginalURL  string `json:"original_url"`
}

func favoritePhotosHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := boundedQueryInt(c, "offset", 0, 0, 1_000_000)
		limit := boundedQueryInt(c, "limit", 500, 1, 500)
		query := db.WithContext(c.Request.Context()).Model(&database.Photo{}).
			Where("status = ? AND favorite = ?", "active", true)

		var total int64
		if err := query.Count(&total).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}
		var photos []database.Photo
		if err := query.Order("updated_at DESC").Order("id DESC").
			Offset(offset).Limit(limit).Find(&photos).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}
		media := make([]mediaResponse, 0, len(photos))
		for _, photo := range photos {
			media = append(media, toMediaResponse(photo))
		}
		c.JSON(http.StatusOK, gin.H{
			"media": media,
			"pagination": paginationResponse{
				Offset: offset, Limit: limit, Total: total, HasMore: int64(offset+len(media)) < total,
			},
		})
	}
}

type albumResponse struct {
	PublishedDate string                `json:"published_date"`
	ID            uint                  `json:"id"`
	Name          string                `json:"name"`
	Favorite      bool                  `json:"favorite"`
	People        []albumPersonResponse `json:"people"`
	Series        []albumSeriesResponse `json:"series"`
	MediaCount    int64                 `json:"media_count"`
	Media         []mediaResponse       `json:"media"`
}

type albumPersonResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
}

type albumSeriesResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func galleryHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := boundedQueryInt(c, "offset", 0, 0, 1_000_000)
		limit := boundedQueryInt(c, "limit", 10, 1, 24)
		mediaLimit := boundedQueryInt(c, "media_limit", -1, 1, 48)
		favoritesOnly := c.Query("favorites") == "true"
		keywords := strings.Fields(c.Query("q"))

		albumQuery := db.WithContext(c.Request.Context()).Model(&database.Album{}).
			Where("scan_status = ?", true)
		if favoritesOnly {
			albumQuery = albumQuery.Where("favorite = ?", true)
		}
		for _, keyword := range keywords {
			albumQuery = albumQuery.Where("LOWER(album_name) LIKE ?", "%"+strings.ToLower(keyword)+"%")
		}

		var totalAlbums int64
		if err := albumQuery.Count(&totalAlbums).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}

		var albums []database.Album
		if err := albumQuery.
			Order("published_date DESC").Order("id DESC").Offset(offset).Limit(limit).Find(&albums).Error; err != nil {
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
		mediaQuery := db.WithContext(c.Request.Context()).Model(&database.Photo{}).
			Joins("JOIN albums ON albums.id = photos.album_id").
			Where("photos.status = ? AND albums.scan_status = ?", "active", true)
		if favoritesOnly {
			mediaQuery = mediaQuery.Where("albums.favorite = ?", true)
		}
		for _, keyword := range keywords {
			mediaQuery = mediaQuery.Where("LOWER(albums.album_name) LIKE ?", "%"+strings.ToLower(keyword)+"%")
		}
		if err := mediaQuery.Count(&totalMedia).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"albums":      responses,
			"total_media": totalMedia,
			"pagination": paginationResponse{
				Offset: offset, Limit: limit, Total: totalAlbums, HasMore: int64(offset+len(albums)) < totalAlbums,
			},
		})
	}
}

func updateAlbumFavoriteHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Favorite *bool `json:"favorite"`
	}
	return func(c *gin.Context) {
		albumID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || albumID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid album id"})
			return
		}
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || body.Favorite == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "favorite must be a boolean"})
			return
		}

		result := db.WithContext(c.Request.Context()).Model(&database.Album{}).
			Where("id = ? AND scan_status = ?", uint(albumID), true).
			Update("favorite", *body.Favorite)
		if result.Error != nil {
			galleryDatabaseError(c, result.Error)
			return
		}
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": uint(albumID), "favorite": *body.Favorite})
	}
}

func updatePhotoFavoriteHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Favorite *bool `json:"favorite"`
	}
	return func(c *gin.Context) {
		photoID, ok := parseMediaID(c)
		if !ok {
			return
		}
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || body.Favorite == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "favorite must be a boolean"})
			return
		}

		result := db.WithContext(c.Request.Context()).Model(&database.Photo{}).
			Where("id = ? AND status = ?", photoID, "active").
			Update("favorite", *body.Favorite)
		if result.Error != nil {
			galleryDatabaseError(c, result.Error)
			return
		}
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": photoID, "favorite": *body.Favorite})
	}
}

func albumMediaHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		albumID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || albumID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid album id"})
			return
		}
		offset := boundedQueryInt(c, "offset", 0, 0, 1_000_000)
		limit := boundedQueryInt(c, "limit", 200, 1, 500)

		var album database.Album
		if err := db.WithContext(c.Request.Context()).
			Where("id = ? AND scan_status = ?", uint(albumID), true).First(&album).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
				return
			}
			galleryDatabaseError(c, err)
			return
		}

		response, err := loadAlbumResponse(c, db, album, offset, limit)
		if err != nil {
			galleryDatabaseError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"album": response,
			"pagination": paginationResponse{
				Offset: offset, Limit: limit, Total: response.MediaCount, HasMore: int64(offset+len(response.Media)) < response.MediaCount,
			},
		})
	}
}

func loadAlbumResponse(c *gin.Context, db *gorm.DB, album database.Album, offset, limit int) (albumResponse, error) {
	count := album.MediaCount
	query := db.WithContext(c.Request.Context()).Model(&database.Photo{}).
		Where("album_id = ? AND status = ?", album.ID, "active")
	// Unscanned records created outside the scanner may not have persisted counts yet.
	if album.CountsUpdatedAt == nil {
		if err := query.Count(&count).Error; err != nil {
			return albumResponse{}, err
		}
	}

	var photos []database.Photo
	if err := query.Order("photo_name ASC").Offset(offset).Limit(limit).Find(&photos).Error; err != nil {
		return albumResponse{}, err
	}
	media := make([]mediaResponse, 0, len(photos))
	for _, photo := range photos {
		media = append(media, toMediaResponse(photo))
	}

	var people []database.Person
	if err := db.WithContext(c.Request.Context()).Model(&database.Person{}).
		Joins("JOIN person_albums ON person_albums.person_id = people.id").
		Where("person_albums.album_id = ?", album.ID).
		Order("people.favorite DESC").Order("people.name ASC").Find(&people).Error; err != nil {
		return albumResponse{}, err
	}
	personItems := make([]albumPersonResponse, 0, len(people))
	for _, person := range people {
		personItems = append(personItems, albumPersonResponse{ID: person.ID, Name: person.Name, Note: person.Note})
	}

	seriesItems := make([]albumSeriesResponse, 0, 1)
	if album.SeriesID != nil {
		var item database.Series
		if err := db.WithContext(c.Request.Context()).First(&item, *album.SeriesID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return albumResponse{}, err
			}
		} else {
			seriesItems = append(seriesItems, albumSeriesResponse{ID: item.ID, Name: item.Name})
		}
	}

	return albumResponse{
		PublishedDate: album.PublishedDate, ID: album.ID, Name: album.AlbumName, Favorite: album.Favorite, People: personItems, Series: seriesItems,
		MediaCount: count, Media: media,
	}, nil
}

func toMediaResponse(photo database.Photo) mediaResponse {
	base := fmt.Sprintf("/api/media/%d", photo.ID)
	return mediaResponse{
		ID: photo.ID, Name: photo.PhotoName, Favorite: photo.Favorite, MediaType: photo.MediaType, MIMEType: photo.MIMEType,
		WidthPX: photo.WidthPX, HeightPX: photo.HeightPX, DurationMS: photo.DurationMS, SizeBytes: photo.FileSize,
		ThumbnailURL: base + "/thumbnail", OriginalURL: base + "/original",
	}
}

func thumbnailHandler(service *mediaservice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseMediaID(c)
		if !ok {
			return
		}
		_, file, info, err := service.OpenThumbnail(c.Request.Context(), id)
		if err != nil {
			mediaError(c, err)
			return
		}
		defer file.Close()
		c.Header("Content-Type", "image/webp")
		c.Header("Cache-Control", "private, max-age=31536000, immutable")
		http.ServeContent(c.Writer, c.Request, "thumbnail.webp", info.ModTime(), file)
	}
}

func originalHandler(service *mediaservice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseMediaID(c)
		if !ok {
			return
		}
		photo, file, info, err := service.OpenOriginal(c.Request.Context(), id)
		if err != nil {
			mediaError(c, err)
			return
		}
		defer file.Close()

		contentType := photo.MIMEType
		if contentType == "" {
			contentType = mime.TypeByExtension(filepath.Ext(photo.PhotoName))
		}
		if contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": photo.PhotoName}))
		c.Header("Cache-Control", "private, max-age=3600")
		http.ServeContent(c.Writer, c.Request, photo.PhotoName, info.ModTime(), file)
	}
}

func parseMediaID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media id"})
		return 0, false
	}
	return uint(id), true
}

func mediaError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, mediaservice.ErrDerivativeAbsent) {
		c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
		return
	}
	slog.Error("媒体请求失败", "path", c.Request.URL.Path, "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to read media"})
}

func galleryDatabaseError(c *gin.Context, err error) {
	slog.Error("图库查询失败", "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to load gallery"})
}

func boundedQueryInt(c *gin.Context, key string, fallback, minimum, maximum int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil || value < minimum {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}
