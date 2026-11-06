package database

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"airy/internal/config"

	"gorm.io/gorm/schema"
)

func TestOpenRejectsUnsupportedDriver(t *testing.T) {
	if _, err := Open(config.DatabaseConfig{Driver: "mysql"}); err == nil {
		t.Fatal("expected an unsupported driver to be rejected")
	}
}

func TestOpenSQLite(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "airy.db") + "?_pragma=foreign_keys(1)"
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQLite connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
}

func TestMigrateDropsLegacyColumns(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "legacy-preview.db") + "?_pragma=foreign_keys(1)"
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Exec("ALTER TABLE photos ADD COLUMN preview_path text").Error; err != nil {
		t.Fatalf("add legacy preview column: %v", err)
	}
	if err := db.Exec("ALTER TABLE asset_directories ADD COLUMN scan_interval_minutes integer").Error; err != nil {
		t.Fatalf("add legacy per-directory scan interval column: %v", err)
	}
	if err := db.Exec("ALTER TABLE scan_schedules ADD COLUMN interval_minutes integer").Error; err != nil {
		t.Fatalf("add legacy global scan interval column: %v", err)
	}
	if !db.Migrator().HasColumn(&Photo{}, "preview_path") {
		t.Fatal("expected legacy preview_path column")
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate legacy preview column: %v", err)
	}
	if db.Migrator().HasColumn(&Photo{}, "preview_path") {
		t.Fatal("expected preview_path column to be removed")
	}
	if db.Migrator().HasColumn(&AssetDirectory{}, "scan_interval_minutes") {
		t.Fatal("expected per-directory scan_interval_minutes column to be removed")
	}
	if db.Migrator().HasColumn(&ScanSchedule{}, "interval_minutes") {
		t.Fatal("expected global interval_minutes column to be removed")
	}
}

func TestMigrateLegacySeriesAlbumsKeepsNewestAssociation(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "legacy-series.db") + "?_pragma=foreign_keys(1)"
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	directory := AssetDirectory{DirectoryName: "library", DirectoryPath: "/library"}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	album := Album{DirectoryID: directory.ID, AlbumName: "album", AlbumPath: "/library/album"}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	older := Series{Name: "older"}
	newer := Series{Name: "newer"}
	if err := db.Create(&older).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE series_albums (
		series_id integer NOT NULL,
		album_id integer NOT NULL,
		created_at datetime NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO series_albums(series_id, album_id, created_at) VALUES (?, ?, ?)",
		older.ID, album.ID, now.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO series_albums(series_id, album_id, created_at) VALUES (?, ?, ?)",
		newer.ID, album.ID, now).Error; err != nil {
		t.Fatal(err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate legacy series links: %v", err)
	}
	if db.Migrator().HasTable("series_albums") {
		t.Fatal("expected legacy series_albums table to be removed")
	}
	if err := db.First(&album, album.ID).Error; err != nil {
		t.Fatal(err)
	}
	if album.SeriesID == nil || *album.SeriesID != newer.ID {
		t.Fatalf("expected newest series %d, got %v", newer.ID, album.SeriesID)
	}
}

func TestAssetModelSchemas(t *testing.T) {
	tests := []struct {
		model   any
		table   string
		columns []string
	}{
		{
			model: &User{},
			table: "users",
			columns: []string{
				"id", "username", "password_hash", "created_at", "updated_at",
			},
		},
		{
			model: &UserSession{},
			table: "user_sessions",
			columns: []string{
				"id", "user_id", "token_hash", "expires_at", "created_at",
			},
		},
		{
			model: &AssetDirectory{},
			table: "asset_directories",
			columns: []string{
				"id", "directory_name", "directory_path", "album_depth", "scan_status",
				"last_scanned_at", "scan_error", "automation_disabled",
				"created_at", "updated_at",
			},
		},
		{
			model: &ScanSchedule{},
			table: "scan_schedules",
			columns: []string{
				"id", "enabled", "next_scan_at", "created_at", "updated_at",
			},
		},
		{
			model: &Album{},
			table: "albums",
			columns: []string{
				"id", "directory_id", "series_id", "album_name", "album_path", "published_date", "scan_status",
				"favorite", "created_at", "updated_at",
			},
		},
		{
			model: &Person{},
			table: "people",
			columns: []string{
				"id", "name", "note", "aliases", "favorite", "avatar_photo_id", "created_at", "updated_at",
			},
		},
		{
			model: &PersonAlbum{},
			table: "person_albums",
			columns: []string{
				"person_id", "album_id", "created_at",
			},
		},
		{
			model: &Series{},
			table: "series",
			columns: []string{
				"id", "name", "favorite", "cover_photo_id", "created_at", "updated_at",
			},
		},
		{
			model: &Photo{},
			table: "photos",
			columns: []string{
				"id", "album_id", "photo_name", "photo_path", "thumbnail_path",
				"media_type", "status", "favorite", "file_size", "height_px", "width_px",
				"duration_ms", "mime_type", "modified_at", "last_seen_at", "created_at", "updated_at",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			parsed, err := schema.Parse(tt.model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatalf("parse model schema: %v", err)
			}
			if parsed.Table != tt.table {
				t.Fatalf("expected table %q, got %q", tt.table, parsed.Table)
			}
			if tt.table == "photos" && parsed.LookUpField("preview_path") != nil {
				t.Error("photos.preview_path must not exist")
			}
			for _, column := range tt.columns {
				if parsed.LookUpField(column) == nil {
					t.Errorf("expected %s.%s column", tt.table, column)
				}
			}
		})
	}
}
