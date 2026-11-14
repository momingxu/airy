package database

import (
	"fmt"
	"time"

	"airy/internal/config"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", cfg.Driver, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get database connection: %w", err)
	}

	if cfg.Driver == "sqlite" {
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping %s database: %w", cfg.Driver, err)
	}

	if err := Migrate(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate %s database: %w", cfg.Driver, err)
	}
	if err := RefreshAllEntityImageIDs(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("刷新模特头像和系列封面: %w", err)
	}

	return db, nil
}

// Migrate creates and updates the application's database schema.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&User{},
		&UserSession{},
		&Series{},
		&AssetDirectory{},
		&Album{},
		&Person{},
		&PersonAlbum{},
		&Photo{},
		&ScanSchedule{},
	); err != nil {
		return err
	}
	for _, column := range []string{"image_count", "video_count"} {
		if err := dropColumnIfExists(db, &Album{}, "albums", column); err != nil {
			return err
		}
	}
	if err := RefreshAlbumCounts(db, nil); err != nil {
		return err
	}
	if err := backfillAlbumPublicationDates(db); err != nil {
		return err
	}
	if err := migrateLegacySeriesAlbums(db); err != nil {
		return err
	}
	if err := migrateLegacyPersonAliases(db); err != nil {
		return err
	}
	if err := dropColumnIfExists(db, &Photo{}, "photos", "preview_path"); err != nil {
		return err
	}
	if err := dropColumnIfExists(db, &AssetDirectory{}, "asset_directories", "scan_interval_minutes"); err != nil {
		return err
	}
	if err := dropColumnIfExists(db, &AssetDirectory{}, "asset_directories", "watch_enabled"); err != nil {
		return err
	}
	if err := dropColumnIfExists(db, &ScanSchedule{}, "scan_schedules", "interval_minutes"); err != nil {
		return err
	}
	// Older versions treated a person's display name as the identity. Remove
	// that unique index so different models may intentionally share a name;
	// idx_people_name_lookup remains as the non-unique lookup index.
	if db.Migrator().HasIndex(&Person{}, "idx_people_name") {
		if err := db.Migrator().DropIndex(&Person{}, "idx_people_name"); err != nil {
			return err
		}
	}
	return db.FirstOrCreate(&ScanSchedule{ID: 1}, ScanSchedule{ID: 1}).Error
}

type legacySeriesAlbum struct {
	SeriesID  uint
	AlbumID   uint
	CreatedAt time.Time
}

// migrateLegacySeriesAlbums converts the former many-to-many join table into
// albums.series_id. If old data linked an album to multiple series, the most
// recently created link wins; a stable series ID order resolves old rows that
// have the same timestamp.
func migrateLegacySeriesAlbums(db *gorm.DB) error {
	if !db.Migrator().HasTable("series_albums") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var links []legacySeriesAlbum
		if err := tx.Table("series_albums").
			Order("album_id ASC").Order("created_at DESC").Order("series_id ASC").
			Find(&links).Error; err != nil {
			return err
		}
		assigned := make(map[uint]struct{}, len(links))
		for _, link := range links {
			if _, exists := assigned[link.AlbumID]; exists {
				continue
			}
			assigned[link.AlbumID] = struct{}{}
			if err := tx.Model(&Album{}).
				Where("id = ? AND series_id IS NULL", link.AlbumID).
				UpdateColumn("series_id", link.SeriesID).Error; err != nil {
				return err
			}
		}
		return tx.Migrator().DropTable("series_albums")
	})
}

func dropColumnIfExists(db *gorm.DB, model any, table, columnName string) error {
	columns, err := db.Migrator().ColumnTypes(model)
	if err != nil {
		return err
	}
	for _, column := range columns {
		if column.Name() == columnName {
			return db.Exec("ALTER TABLE " + table + " DROP COLUMN " + columnName).Error
		}
	}
	return nil
}
