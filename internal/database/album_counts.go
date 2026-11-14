package database

import "gorm.io/gorm"

// RefreshAlbumCounts persists counts from active media without touching the
// album's publication date or UpdatedAt. Nil IDs backfill only legacy rows.
func RefreshAlbumCounts(db *gorm.DB, ids []uint) error {
	query := db.Model(&Album{})
	if ids == nil {
		query = query.Where("counts_updated_at IS NULL")
	} else {
		if len(ids) == 0 {
			return nil
		}
		query = query.Where("id IN ?", ids)
	}
	return query.UpdateColumns(map[string]any{
		"media_count":       gorm.Expr("(SELECT COUNT(*) FROM photos WHERE photos.album_id = albums.id AND photos.status = ?)", "active"),
		"counts_updated_at": db.NowFunc(),
	}).Error
}
