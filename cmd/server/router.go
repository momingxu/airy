package main

import (
	"net/http"

	mediaservice "airy/internal/media"
	"airy/internal/scanner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Driver   string `json:"driver"`
}

func newRouter(db *gorm.DB, driver string, mediaScanner *scanner.Scanner, mediaService *mediaservice.Service, authentication ...bool) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	api := router.Group("/api")
	api.GET("/health", healthHandler(db, driver))
	api.GET("/auth/status", authStatusHandler(db))
	api.POST("/auth/setup", setupHandler(db))
	api.POST("/auth/login", loginHandler(db))
	api.POST("/auth/logout", logoutHandler(db))

	protected := api.Group("")
	authenticationEnabled := len(authentication) == 0 || authentication[0]
	if authenticationEnabled {
		protected.Use(requireAuthentication(db))
	}
	protected.PATCH("/settings/account", updateAccountHandler(db))
	protected.GET("/scan", scanStatusHandler(mediaScanner))
	protected.POST("/scan", startAllScansHandler(mediaScanner))
	protected.POST("/scan/all", startAllScansHandler(mediaScanner))
	protected.POST("/scan/cancel", cancelScanHandler(mediaScanner))
	protected.GET("/scan/schedule", getScanScheduleHandler(db))
	protected.PATCH("/scan/schedule", updateScanScheduleHandler(db, mediaScanner))
	protected.GET("/directories", listDirectoriesHandler(db, mediaScanner))
	protected.POST("/directories", createDirectoryHandler(db))
	protected.PATCH("/directories/:id/settings", updateDirectorySettingsHandler(db, mediaScanner))
	protected.POST("/directories/:id/scan", scanDirectoryHandler(db, mediaScanner))
	protected.DELETE("/directories/:id", deleteDirectoryHandler(db, mediaScanner))
	protected.GET("/gallery", galleryHandler(db))
	protected.GET("/favorites/photos", favoritePhotosHandler(db))
	protected.GET("/albums/:id/media", albumMediaHandler(db))
	protected.PATCH("/albums/:id/favorite", updateAlbumFavoriteHandler(db))
	protected.PATCH("/media/:id/favorite", updatePhotoFavoriteHandler(db))
	protected.GET("/albums/search", searchAlbumsHandler(db))
	protected.GET("/people", listPeopleHandler(db))
	protected.POST("/people", createPersonHandler(db))
	protected.PATCH("/people/:id", updatePersonHandler(db))
	protected.PATCH("/people/:id/favorite", updatePersonFavoriteHandler(db))
	protected.DELETE("/people/:id", deletePersonHandler(db))
	protected.GET("/people/:id/gallery", personGalleryHandler(db))
	protected.GET("/people/:id/album-options", personAlbumOptionsHandler(db))
	protected.PUT("/people/:id/albums/:album_id", addPersonAlbumHandler(db))
	protected.DELETE("/people/:id/albums/:album_id", removePersonAlbumHandler(db))
	protected.GET("/series", listSeriesHandler(db))
	protected.POST("/series", createSeriesHandler(db))
	protected.PATCH("/series/:id", updateSeriesHandler(db))
	protected.DELETE("/series/:id", deleteSeriesHandler(db))
	protected.GET("/series/:id/gallery", seriesGalleryHandler(db))
	protected.GET("/series/:id/album-options", seriesAlbumOptionsHandler(db))
	protected.PUT("/series/:id/albums/:album_id", addSeriesAlbumHandler(db))
	protected.DELETE("/series/:id/albums/:album_id", removeSeriesAlbumHandler(db))
	if mediaService != nil {
		protected.GET("/media/:id/thumbnail", thumbnailHandler(mediaService))
		protected.GET("/media/:id/original", originalHandler(mediaService))
	}

	router.NoRoute(frontendHandler())

	return router
}

func startAllScansHandler(mediaScanner *scanner.Scanner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if mediaScanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "扫描服务不可用"})
			return
		}
		status, queued, err := mediaScanner.EnqueueAll(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"queued": queued, "scan": status})
	}
}

func scanStatusHandler(mediaScanner *scanner.Scanner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if mediaScanner == nil {
			c.JSON(http.StatusOK, gin.H{"configured": false, "scan": scanner.Status{State: scanner.StateIdle}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"configured": true, "scan": mediaScanner.Status()})
	}
}

func healthHandler(db *gorm.DB, driver string) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			c.JSON(http.StatusServiceUnavailable, healthResponse{
				Status:   "unavailable",
				Database: "offline",
				Driver:   driver,
			})
			return
		}

		c.JSON(http.StatusOK, healthResponse{
			Status:   "ok",
			Database: "online",
			Driver:   driver,
		})
	}
}
