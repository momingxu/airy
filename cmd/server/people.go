package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"airy/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type personResponse struct {
	ID         uint      `json:"id"`
	Name       string    `json:"name"`
	Note       string    `json:"note"`
	Aliases    []string  `json:"aliases"`
	Favorite   bool      `json:"favorite"`
	AvatarURL  string    `json:"avatar_url"`
	AlbumCount int64     `json:"album_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type personAlbumCountRow struct {
	PersonID   uint
	AlbumCount int64
}

func loadPersonResponses(ctx context.Context, db *gorm.DB, people []database.Person) ([]personResponse, error) {
	items := make([]personResponse, 0, len(people))
	if len(people) == 0 {
		return items, nil
	}

	personIDs := make([]uint, 0, len(people))
	for _, person := range people {
		personIDs = append(personIDs, person.ID)
	}

	var countRows []personAlbumCountRow
	if err := db.WithContext(ctx).Table("person_albums").
		Select("person_albums.person_id, COUNT(*) AS album_count").
		Joins("JOIN albums ON albums.id = person_albums.album_id").
		Where("person_albums.person_id IN ? AND albums.scan_status = ?", personIDs, true).
		Group("person_albums.person_id").Scan(&countRows).Error; err != nil {
		return nil, err
	}
	albumCounts := make(map[uint]int64, len(countRows))
	for _, row := range countRows {
		albumCounts[row.PersonID] = row.AlbumCount
	}

	for _, person := range people {
		aliases := person.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		avatarURL := ""
		if person.AvatarPhotoID != nil {
			avatarURL = fmt.Sprintf("/api/media/%d/thumbnail", *person.AvatarPhotoID)
		}
		items = append(items, personResponse{
			ID: person.ID, Name: person.Name, Note: person.Note, Aliases: aliases, Favorite: person.Favorite,
			AvatarURL: avatarURL, AlbumCount: albumCounts[person.ID], CreatedAt: person.CreatedAt,
		})
	}
	return items, nil
}

type personAlbumOptionResponse struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Associated bool   `json:"associated"`
}

type albumSearchResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func listPeopleHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var people []database.Person
		if err := db.WithContext(c.Request.Context()).Order("name ASC").Order("id ASC").Find(&people).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特失败"})
			return
		}

		items, err := loadPersonResponses(c.Request.Context(), db, people)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"people": items})
	}
}

func createPersonHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Name     string   `json:"name"`
		Note     string   `json:"note"`
		Aliases  []string `json:"aliases"`
		AlbumIDs []uint   `json:"album_ids"`
	}
	return func(c *gin.Context) {
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入模特名称"})
			return
		}
		name := strings.TrimSpace(body.Name)
		person := database.Person{}
		err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			person = database.Person{Name: name, Note: strings.TrimSpace(body.Note), Aliases: database.CleanPersonAliases(body.Aliases, name)}
			if err := tx.Create(&person).Error; err != nil {
				return err
			}
			if len(body.AlbumIDs) > 0 {
				var albumIDs []uint
				if err := tx.Model(&database.Album{}).
					Where("id IN ? AND scan_status = ?", body.AlbumIDs, true).
					Pluck("id", &albumIDs).Error; err != nil {
					return err
				}
				links := make([]database.PersonAlbum, 0, len(albumIDs))
				for _, albumID := range albumIDs {
					links = append(links, database.PersonAlbum{PersonID: person.ID, AlbumID: albumID})
				}
				if len(links) > 0 {
					if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error; err != nil {
						return err
					}
				}
			}
			return database.RefreshPersonAvatarPhotoIDs(tx, []uint{person.ID})
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "添加模特失败"})
			return
		}
		if err := db.WithContext(c.Request.Context()).First(&person, person.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特信息失败"})
			return
		}
		items, err := loadPersonResponses(c.Request.Context(), db, []database.Person{person})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特相册统计失败"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"person": items[0]})
	}
}

func updatePersonHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Name           *string   `json:"name"`
		Note           *string   `json:"note"`
		Aliases        *[]string `json:"aliases"`
		AlbumIDs       *[]uint   `json:"album_ids"`
		RemoveAlbumIDs []uint    `json:"remove_album_ids"`
	}
	return func(c *gin.Context) {
		person, ok := findPerson(c, db)
		if !ok {
			return
		}
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || (body.Name == nil && body.Note == nil && body.Aliases == nil && body.AlbumIDs == nil && len(body.RemoveAlbumIDs) == 0) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入需要修改的模特信息"})
			return
		}
		updates := map[string]any{}
		if body.Name != nil {
			name := strings.TrimSpace(*body.Name)
			if name == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "模特名称不能为空"})
				return
			}
			updates["name"] = name
			person.Name = name
		}
		if body.Note != nil {
			note := strings.TrimSpace(*body.Note)
			updates["note"] = note
			person.Note = note
		}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if len(updates) > 0 {
				if err := tx.Model(&person).Updates(updates).Error; err != nil {
					return err
				}
			}
			if body.Aliases != nil || body.Name != nil {
				aliases := person.Aliases
				if body.Aliases != nil {
					aliases = *body.Aliases
				}
				person.Aliases = database.CleanPersonAliases(aliases, person.Name)
				if err := tx.Model(&person).Select("Aliases").Updates(&person).Error; err != nil {
					return err
				}
			}
			if len(body.RemoveAlbumIDs) > 0 {
				if err := tx.Where("person_id = ? AND album_id IN ?", person.ID, body.RemoveAlbumIDs).Delete(&database.PersonAlbum{}).Error; err != nil {
					return err
				}
			}
			if body.AlbumIDs != nil && len(*body.AlbumIDs) > 0 {
				var albumIDs []uint
				if err := tx.Model(&database.Album{}).Where("id IN ? AND scan_status = ?", *body.AlbumIDs, true).Pluck("id", &albumIDs).Error; err != nil {
					return err
				}
				links := make([]database.PersonAlbum, 0, len(albumIDs))
				for _, albumID := range albumIDs {
					links = append(links, database.PersonAlbum{PersonID: person.ID, AlbumID: albumID})
				}
				if len(links) > 0 {
					if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error; err != nil {
						return err
					}
					if err := database.RefreshPersonAvatarPhotoIDs(tx, []uint{person.ID}); err != nil {
						return err
					}
				}
			}
			if len(body.RemoveAlbumIDs) > 0 {
				if err := database.RefreshPersonAvatarPhotoIDs(tx, []uint{person.ID}); err != nil {
					return err
				}
			}
			return tx.First(&person, person.ID).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新模特信息失败"})
			return
		}
		items, err := loadPersonResponses(c.Request.Context(), db, []database.Person{person})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"person": items[0]})
	}
}

func updatePersonFavoriteHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Favorite *bool `json:"favorite"`
	}
	return func(c *gin.Context) {
		person, ok := findPerson(c, db)
		if !ok {
			return
		}
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil || body.Favorite == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的收藏状态"})
			return
		}
		if err := db.WithContext(c.Request.Context()).Model(&person).Update("favorite", *body.Favorite).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新模特收藏失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": person.ID, "favorite": *body.Favorite})
	}
}

func searchAlbumsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		keywords := strings.Fields(c.Query("q"))
		if len(keywords) == 0 {
			c.JSON(http.StatusOK, gin.H{"albums": []albumSearchResponse{}})
			return
		}
		var albums []database.Album
		query := db.WithContext(c.Request.Context()).Model(&database.Album{}).Where("scan_status = ?", true)
		for _, keyword := range keywords {
			query = query.Where("LOWER(album_name) LIKE ?", "%"+strings.ToLower(keyword)+"%")
		}
		if err := query.Order("published_date DESC").Order("id DESC").Find(&albums).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "搜索相册失败"})
			return
		}
		items := make([]albumSearchResponse, 0, len(albums))
		for _, album := range albums {
			items = append(items, albumSearchResponse{ID: album.ID, Name: album.AlbumName})
		}
		c.JSON(http.StatusOK, gin.H{"albums": items})
	}
}

func deletePersonHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		person, ok := findPerson(c, db)
		if !ok {
			return
		}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("person_id = ?", person.ID).Delete(&database.PersonAlbum{}).Error; err != nil {
				return err
			}
			return tx.Delete(&person).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除模特失败"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func personGalleryHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		person, ok := findPerson(c, db)
		if !ok {
			return
		}
		offset := boundedQueryInt(c, "offset", 0, 0, 1_000_000)
		limit := boundedQueryInt(c, "limit", 10, 1, 24)
		mediaLimit := boundedQueryInt(c, "media_limit", -1, 1, 48)

		base := db.WithContext(c.Request.Context()).Model(&database.Album{}).
			Joins("JOIN person_albums ON person_albums.album_id = albums.id").
			Where("person_albums.person_id = ? AND albums.scan_status = ?", person.ID, true)
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
			Joins("JOIN person_albums ON person_albums.album_id = albums.id").
			Where("person_albums.person_id = ? AND photos.status = ? AND albums.scan_status = ?", person.ID, "active", true).
			Count(&totalMedia).Error; err != nil {
			galleryDatabaseError(c, err)
			return
		}

		personItems, err := loadPersonResponses(c.Request.Context(), db, []database.Person{person})
		if err != nil {
			galleryDatabaseError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"person":      personItems[0],
			"albums":      responses,
			"total_media": totalMedia,
			"pagination": paginationResponse{
				Offset: offset, Limit: limit, Total: totalAlbums, HasMore: int64(offset+len(albums)) < totalAlbums,
			},
		})
	}
}

func personAlbumOptionsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		person, ok := findPerson(c, db)
		if !ok {
			return
		}
		query := db.WithContext(c.Request.Context()).Model(&database.Album{}).
			Where("scan_status = ?", true)
		if search := strings.TrimSpace(c.Query("q")); search != "" {
			query = query.Where("LOWER(album_name) LIKE ?", "%"+strings.ToLower(search)+"%")
		}
		var albums []database.Album
		if err := query.Order("published_date DESC").Order("id DESC").Find(&albums).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "搜索相册失败"})
			return
		}

		ids := make([]uint, 0, len(albums))
		for _, album := range albums {
			ids = append(ids, album.ID)
		}
		associated := make(map[uint]struct{}, len(ids))
		if len(ids) > 0 {
			var links []database.PersonAlbum
			if err := db.WithContext(c.Request.Context()).
				Where("person_id = ? AND album_id IN ?", person.ID, ids).Find(&links).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特相册关联失败"})
				return
			}
			for _, link := range links {
				associated[link.AlbumID] = struct{}{}
			}
		}

		items := make([]personAlbumOptionResponse, 0, len(albums))
		for _, album := range albums {
			_, linked := associated[album.ID]
			items = append(items, personAlbumOptionResponse{ID: album.ID, Name: album.AlbumName, Associated: linked})
		}
		c.JSON(http.StatusOK, gin.H{"albums": items})
	}
}

func addPersonAlbumHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		person, album, ok := findPersonAndAlbum(c, db)
		if !ok {
			return
		}
		link := database.PersonAlbum{PersonID: person.ID, AlbumID: album.ID}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.FirstOrCreate(&link, link).Error; err != nil {
				return err
			}
			return database.RefreshPersonAvatarPhotoIDs(tx, []uint{person.ID})
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "关联模特与相册失败"})
			return
		}
		if err := db.WithContext(c.Request.Context()).First(&person, person.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特信息失败"})
			return
		}
		items, err := loadPersonResponses(c.Request.Context(), db, []database.Person{person})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"person": items[0]})
	}
}

func removePersonAlbumHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		person, album, ok := findPersonAndAlbum(c, db)
		if !ok {
			return
		}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("person_id = ? AND album_id = ?", person.ID, album.ID).
				Delete(&database.PersonAlbum{}).Error; err != nil {
				return err
			}
			return database.RefreshPersonAvatarPhotoIDs(tx, []uint{person.ID})
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "取消模特与相册关联失败"})
			return
		}
		if err := db.WithContext(c.Request.Context()).First(&person, person.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特信息失败"})
			return
		}
		items, err := loadPersonResponses(c.Request.Context(), db, []database.Person{person})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"person": items[0]})
	}
}

func findPerson(c *gin.Context, db *gorm.DB) (database.Person, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模特编号"})
		return database.Person{}, false
	}
	var person database.Person
	if err := db.WithContext(c.Request.Context()).First(&person, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "模特不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模特失败"})
		}
		return database.Person{}, false
	}
	return person, true
}

func findPersonAndAlbum(c *gin.Context, db *gorm.DB) (database.Person, database.Album, bool) {
	person, ok := findPerson(c, db)
	if !ok {
		return database.Person{}, database.Album{}, false
	}
	albumID, err := strconv.ParseUint(c.Param("album_id"), 10, 64)
	if err != nil || albumID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的相册编号"})
		return database.Person{}, database.Album{}, false
	}
	var album database.Album
	if err := db.WithContext(c.Request.Context()).First(&album, uint(albumID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "相册不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取相册失败"})
		}
		return database.Person{}, database.Album{}, false
	}
	return person, album, true
}
