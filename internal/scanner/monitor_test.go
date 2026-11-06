package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
	"airy/internal/database"

	"github.com/fsnotify/fsnotify"
)

func TestDirectoryMonitorLimitsWatchesToAlbumDepth(t *testing.T) {
	root := t.TempDir()
	levelOne := filepath.Join(root, "level-one")
	levelTwo := filepath.Join(levelOne, "level-two")
	levelThree := filepath.Join(levelTwo, "level-three")
	if err := os.MkdirAll(levelThree, 0o755); err != nil {
		t.Fatal(err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watcher.Close() })
	monitor := &directoryMonitor{
		watcher: watcher,
		watched: make(map[uint]map[string]struct{}),
	}
	directory := database.AssetDirectory{ID: 1, DirectoryPath: root, AlbumDepth: 1}
	if err := monitor.addTree(directory, root); err != nil {
		t.Fatal(err)
	}
	if _, exists := monitor.watched[directory.ID][root]; !exists {
		t.Fatal("resource root was not watched")
	}
	if _, exists := monitor.watched[directory.ID][levelOne]; exists {
		t.Fatal("album directory itself was watched")
	}
	if _, exists := monitor.watched[directory.ID][levelTwo]; exists {
		t.Fatal("directory below album depth was watched")
	}

	monitor.removeWatch(directory.ID)
	directory.AlbumDepth = 2
	if err := monitor.addTree(directory, root); err != nil {
		t.Fatal(err)
	}
	if _, exists := monitor.watched[directory.ID][levelOne]; !exists {
		t.Fatal("directory above updated album depth was not watched")
	}
	if _, exists := monitor.watched[directory.ID][levelTwo]; exists {
		t.Fatal("updated album directory itself was watched")
	}
	if _, exists := monitor.watched[directory.ID][levelThree]; exists {
		t.Fatal("directory below updated album depth was watched")
	}
}

func TestDirectoryMonitorScansAfterFilesystemChange(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "monitor.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	directory := database.AssetDirectory{
		DirectoryName: filepath.Base(root),
		DirectoryPath: root,
	}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}

	mediaScanner := New(db, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	if err := mediaScanner.startAutomation(ctx, 100*time.Millisecond); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mediaScanner.StopAutomation()
		cancel()
	})

	deadline := time.Now().Add(3 * time.Second)
	for !mediaScanner.DirectoryAutomationStatus(directory.ID).Watching && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !mediaScanner.DirectoryAutomationStatus(directory.ID).Watching {
		t.Fatal("directory watcher did not become active")
	}

	albumPath := filepath.Join(root, "New Album")
	if err := os.Mkdir(albumPath, 0o755); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := db.Model(&database.Album{}).Where("album_path = ? AND scan_status = ?", albumPath, true).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("filesystem change did not trigger a directory scan")
}

func TestEnqueuePathsScansDirectoriesSerially(t *testing.T) {
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "queue.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	roots := []string{t.TempDir(), t.TempDir()}
	for index, root := range roots {
		if err := os.Mkdir(filepath.Join(root, "Album"), 0o755); err != nil {
			t.Fatal(err)
		}
		directory := database.AssetDirectory{DirectoryName: filepath.Base(root), DirectoryPath: root}
		if err := db.Create(&directory).Error; err != nil {
			t.Fatalf("create directory %d: %v", index, err)
		}
	}

	mediaScanner := New(db, "", "")
	status, added, err := mediaScanner.EnqueuePaths(roots)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateScanning || added != 2 {
		t.Fatalf("unexpected queue result: status=%+v added=%d", status, added)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if mediaScanner.Status().State != StateScanning {
			var scanned int64
			if err := db.Model(&database.AssetDirectory{}).Where("scan_status = ?", true).Count(&scanned).Error; err != nil {
				t.Fatal(err)
			}
			if scanned == 2 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("queued directories were not scanned")
}

func TestGlobalScheduleExcludesDisabledDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "global-schedule.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	directory := database.AssetDirectory{DirectoryName: filepath.Base(root), DirectoryPath: root}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	disabledRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(disabledRoot, "Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	disabled := database.AssetDirectory{DirectoryName: filepath.Base(disabledRoot), DirectoryPath: disabledRoot, AutomationDisabled: true}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	due := NextDailyScan(time.Now().Add(-48 * time.Hour))
	if err := db.Model(&database.ScanSchedule{}).Where("id = ?", 1).Updates(map[string]any{
		"enabled": true, "next_scan_at": due,
	}).Error; err != nil {
		t.Fatal(err)
	}

	mediaScanner := New(db, "", "missing-ffmpeg-for-test")
	ctx, cancel := context.WithCancel(context.Background())
	if err := mediaScanner.StartAutomation(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mediaScanner.StopAutomation()
		cancel()
	})

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var current database.AssetDirectory
		if err := db.First(&current, directory.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.ScanStatus && current.LastScannedAt != nil {
			var untouched database.AssetDirectory
			if err := db.First(&untouched, disabled.ID).Error; err != nil {
				t.Fatal(err)
			}
			if untouched.ScanStatus || untouched.LastScannedAt != nil {
				t.Fatalf("scheduled scan included a disabled directory: %+v", untouched)
			}
			var schedule database.ScanSchedule
			if err := db.First(&schedule, 1).Error; err != nil {
				t.Fatal(err)
			}
			if schedule.NextScanAt == nil || !schedule.NextScanAt.After(due) {
				t.Fatalf("schedule was not advanced: %+v", schedule)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("global schedule did not trigger a scan")
}
