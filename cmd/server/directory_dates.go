package main

import (
	"net/http"

	"airy/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func refreshAlbumPublicationDatesHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestDB := db.WithContext(c.Request.Context())
		var total, updated int64
		err := requestDB.Transaction(func(tx *gorm.DB) error {
			now := tx.NowFunc()
			var lastID uint
			for {
				var albums []database.Album
				if err := tx.Where("id > ?", lastID).Order("id ASC").Limit(200).Find(&albums).Error; err != nil {
					return err
				}
				if len(albums) == 0 {
					return nil
				}
				for _, album := range albums {
					date, err := database.ResolveAlbumPublicationDate(tx, album, now)
					if err != nil {
						return err
					}
					total++
					if date == album.PublishedDate {
						continue
					}
					result := tx.Model(&database.Album{}).Where("id = ? AND directory_id = ? AND album_name = ? AND published_date = ?", album.ID, album.DirectoryID, album.AlbumName, album.PublishedDate).UpdateColumn("published_date", date)
					if result.Error != nil {
						return result.Error
					}
					updated += result.RowsAffected
				}
				lastID = albums[len(albums)-1].ID
			}
		})
		if err != nil {
			galleryDatabaseError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"total": total, "updated": updated})
	}
}
