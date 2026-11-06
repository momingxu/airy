package database

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
)

func TestAlbumPublicationDate(t *testing.T) {
	scannedAt := time.Date(2026, 11, 5, 18, 30, 0, 0, time.UTC)
	for _, test := range []struct{ title, want string }{
		{"[2024-02-29] Album", "2024-02-29"},
		{"Album 2023.8.2", "2023-08-02"},
		{"2022_12_03 Album", "2022-12-03"},
		{"2021/1/9 Album", "2021-01-09"},
		{"发布于2020年2月29日", "2020-02-29"},
		{"2026年11月5日12时", "2026-11-05"},
		{"Album20261105特辑", "2026-11-05"},
		{"Album 2026-03-02 and 2026-01-01", "2026-03-02"},
		{"2025-02-29 then 2024-02-29", "2024-02-29"},
		{"2025-02-29", "2026-11-06"},
		{"2026-04-31", "2026-11-06"},
		{"2026-00-01 2026-13-01 2026-01-00", "2026-11-06"},
		{"No.1202611059", "2026-11-06"},
		{"2026-11-051", "2026-11-06"},
		{"2026-11", "2026-11-06"},
		{"No.1234 ordinary album", "2026-11-06"},
		{"", "2026-11-06"},
	} {
		t.Run(test.title, func(t *testing.T) {
			if got := AlbumPublicationDate(test.title, scannedAt); got != test.want {
				t.Fatalf("got %s want %s", got, test.want)
			}
		})
	}
}

func TestAlbumPublicationDateMigrationAndDefaults(t *testing.T) {
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "dates.db") + "?_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	directory := AssetDirectory{DirectoryName: "dates", DirectoryPath: "/dates"}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	indexedAt := time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC)
	albums := []Album{
		{DirectoryID: directory.ID, AlbumName: "2023.7.8 Album", AlbumPath: "/dates/title", CreatedAt: indexedAt},
		{DirectoryID: directory.ID, AlbumName: "No date", AlbumPath: "/dates/fallback", CreatedAt: indexedAt},
	}
	if err := db.Create(&albums).Error; err != nil {
		t.Fatal(err)
	}
	if albums[0].PublishedDate != "2023-07-08" || albums[1].PublishedDate != "2024-01-02" {
		t.Fatalf("create defaults: %+v", albums)
	}
	// Reproduce an existing database before this field was introduced.
	if err := db.Exec("ALTER TABLE albums DROP COLUMN published_date").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"2023-07-08", "2024-01-02"} {
		var album Album
		if err := db.First(&album, albums[i].ID).Error; err != nil {
			t.Fatal(err)
		}
		if album.PublishedDate != want {
			t.Fatalf("migrated date %s, want %s", album.PublishedDate, want)
		}
		if !album.CreatedAt.Equal(indexedAt) || !album.UpdatedAt.Equal(albums[i].UpdatedAt) {
			t.Fatal("migration changed album timestamps")
		}
	}
	if err := db.Model(&albums[0]).UpdateColumn("published_date", "2020-01-01").Error; err != nil {
		t.Fatal(err)
	}
	var batch []Album
	for i := 0; i < 205; i++ {
		batch = append(batch, Album{DirectoryID: directory.ID, AlbumName: "Legacy", AlbumPath: fmt.Sprintf("/dates/legacy-%d", i), CreatedAt: indexedAt})
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Album{}).Where("album_name = ?", "Legacy").UpdateColumn("published_date", "").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var preserved Album
	if err := db.First(&preserved, albums[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if preserved.PublishedDate != "2020-01-01" {
		t.Fatal("existing publication date overwritten")
	}
	var filled int64
	if err := db.Model(&Album{}).Where("album_name = ? AND published_date = ?", "Legacy", "2024-01-02").Count(&filled).Error; err != nil {
		t.Fatal(err)
	}
	if filled != 205 {
		t.Fatalf("backfill did not cover every batch: %d", filled)
	}
}
