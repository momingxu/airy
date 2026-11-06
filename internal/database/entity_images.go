package database

import "gorm.io/gorm"

const personAvatarPhotoSelection = `(
	SELECT entity_photos.id
	FROM person_albums AS entity_links
	JOIN albums AS entity_albums ON entity_albums.id = entity_links.album_id
	JOIN photos AS entity_photos ON entity_photos.album_id = entity_albums.id
	WHERE entity_links.person_id = people.id
		AND entity_albums.scan_status = ?
		AND entity_photos.status = ?
		AND entity_photos.thumbnail_path IS NOT NULL
		AND entity_photos.thumbnail_path <> ?
	ORDER BY entity_albums.created_at DESC, entity_albums.id DESC,
		entity_photos.photo_name ASC, entity_photos.id ASC
	LIMIT 1
)`

const seriesCoverPhotoSelection = `(
	SELECT entity_photos.id
	FROM albums AS entity_albums
	JOIN photos AS entity_photos ON entity_photos.album_id = entity_albums.id
	WHERE entity_albums.series_id = series.id
		AND entity_albums.scan_status = ?
		AND entity_photos.status = ?
		AND entity_photos.thumbnail_path IS NOT NULL
		AND entity_photos.thumbnail_path <> ?
	ORDER BY entity_albums.created_at DESC, entity_albums.id DESC,
		entity_photos.photo_name ASC, entity_photos.id ASC
	LIMIT 1
)`

// RefreshPersonAvatarPhotoIDs stores the selected thumbnail photo directly on
// each person so list requests do not repeat the album/photo selection query.
func RefreshPersonAvatarPhotoIDs(db *gorm.DB, personIDs []uint) error {
	if len(personIDs) == 0 {
		return nil
	}
	return db.Model(&Person{}).
		Where("id IN ?", personIDs).
		UpdateColumn("avatar_photo_id", gorm.Expr(personAvatarPhotoSelection, true, "active", "")).Error
}

// RefreshSeriesCoverPhotoIDs stores the selected thumbnail photo directly on
// each series for the same reason as RefreshPersonAvatarPhotoIDs.
func RefreshSeriesCoverPhotoIDs(db *gorm.DB, seriesIDs []uint) error {
	if len(seriesIDs) == 0 {
		return nil
	}
	return db.Model(&Series{}).
		Where("id IN ?", seriesIDs).
		UpdateColumn("cover_photo_id", gorm.Expr(seriesCoverPhotoSelection, true, "active", "")).Error
}

// RefreshEntityImageIDsForAlbumIDs updates only entities affected by albums
// whose media, scan state, or relationship changed.
func RefreshEntityImageIDsForAlbumIDs(db *gorm.DB, albumIDs []uint) error {
	if len(albumIDs) == 0 {
		return nil
	}

	var personIDs []uint
	if err := db.Model(&PersonAlbum{}).Distinct().Where("album_id IN ?", albumIDs).
		Pluck("person_id", &personIDs).Error; err != nil {
		return err
	}
	if err := RefreshPersonAvatarPhotoIDs(db, personIDs); err != nil {
		return err
	}

	var seriesIDs []uint
	if err := db.Model(&Album{}).Distinct().Where("id IN ? AND series_id IS NOT NULL", albumIDs).
		Pluck("series_id", &seriesIDs).Error; err != nil {
		return err
	}
	return RefreshSeriesCoverPhotoIDs(db, seriesIDs)
}

// RefreshAllEntityImageIDs backfills cached image IDs after schema migration.
// Normal list requests only read these stored fields.
func RefreshAllEntityImageIDs(db *gorm.DB) error {
	if err := db.Model(&Person{}).Where("id > ?", 0).
		UpdateColumn("avatar_photo_id", gorm.Expr(personAvatarPhotoSelection, true, "active", "")).Error; err != nil {
		return err
	}
	return db.Model(&Series{}).Where("id > ?", 0).
		UpdateColumn("cover_photo_id", gorm.Expr(seriesCoverPhotoSelection, true, "active", "")).Error
}
