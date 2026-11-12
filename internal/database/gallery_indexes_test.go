package database

import (
	"path/filepath"
	"strings"
	"testing"

	"airy/internal/config"
)

// Check the actual query plan: separate single-column indexes cannot satisfy
// both the equality filters and ordering used by gallery pagination.
func TestGalleryIndexesAvoidSorting(t *testing.T) {
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "indexes.db")})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Simulate an installation with only the former single-column indexes.
	for _, name := range []string{"idx_albums_gallery", "idx_albums_favorite_gallery", "idx_albums_series_gallery"} {
		if err := db.Migrator().DropIndex(&Album{}, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Migrator().DropIndex(&Photo{}, "idx_photos_album_status_name"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	// Repeating startup migrations must also succeed.
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, query, index string }{
		{"gallery", "SELECT * FROM albums WHERE scan_status = true ORDER BY published_date DESC, id DESC LIMIT 10", "idx_albums_gallery"},
		{"favorites", "SELECT * FROM albums WHERE scan_status = true AND favorite = true ORDER BY published_date DESC, id DESC LIMIT 10", "idx_albums_favorite_gallery"},
		{"series", "SELECT * FROM albums WHERE series_id = 1 AND scan_status = true ORDER BY published_date DESC, id DESC LIMIT 10", "idx_albums_series_gallery"},
		{"media", "SELECT * FROM photos WHERE album_id = 1 AND status = 'active' ORDER BY photo_name ASC LIMIT 48", "idx_photos_album_status_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rows []struct{ Detail string }
			if err := db.Raw("EXPLAIN QUERY PLAN " + tc.query).Scan(&rows).Error; err != nil {
				t.Fatal(err)
			}
			var plan string
			for _, row := range rows {
				plan += row.Detail + "\n"
			}
			if !strings.Contains(plan, tc.index) || strings.Contains(plan, "TEMP B-TREE") {
				t.Fatalf("expected ordered lookup using %s, got:\n%s", tc.index, plan)
			}
		})
	}
}
