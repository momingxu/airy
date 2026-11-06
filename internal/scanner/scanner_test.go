package scanner

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"airy/internal/config"
	"airy/internal/database"
)

func TestCancelScanClearsQueueAndRestoresDirectoryState(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "cancel.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	lastScannedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	directory := database.AssetDirectory{
		DirectoryName: root, DirectoryPath: root, ScanStatus: true,
		ScanError: "previous error", LastScannedAt: &lastScannedAt,
	}
	if err := db.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}

	mediaScanner := New(db, "", "missing-ffmpeg-for-test")
	ctx, cancel := context.WithCancel(context.Background())
	startedAt := time.Now().UTC()
	queuedRoot := t.TempDir()
	mediaScanner.mu.Lock()
	mediaScanner.status = Status{State: StateScanning, Root: root, StartedAt: &startedAt, Queued: 1}
	mediaScanner.scanCancel = cancel
	mediaScanner.queue = []string{queuedRoot}
	mediaScanner.queued[queuedRoot] = struct{}{}
	mediaScanner.mu.Unlock()

	status, err := mediaScanner.Cancel()
	if err != nil || !status.Canceling || status.Queued != 0 {
		t.Fatalf("cancel scan: status=%+v err=%v", status, err)
	}
	mediaScanner.runQueued(ctx, root)
	status = mediaScanner.Status()
	if status.State != StateCanceled || status.Canceling || status.Queued != 0 {
		t.Fatalf("unexpected canceled status: %+v", status)
	}
	var restored database.AssetDirectory
	if err := db.First(&restored, directory.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !restored.ScanStatus || restored.ScanError != "previous error" {
		t.Fatalf("canceled scan did not restore directory state: %+v", restored)
	}
	if restored.LastScannedAt == nil || !restored.LastScannedAt.UTC().Equal(lastScannedAt) {
		t.Fatalf("canceled scan changed last scan time: got %v want %v", restored.LastScannedAt, lastScannedAt)
	}
}

func TestScanCreatesAlbumsAndMediaAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	firstAlbum := filepath.Join(root, "Album One")
	secondAlbum := filepath.Join(root, "Album Two")
	if err := os.Mkdir(firstAlbum, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(secondAlbum, 0o755); err != nil {
		t.Fatal(err)
	}

	firstPhoto := filepath.Join(firstAlbum, "one.jpg")
	secondPhoto := filepath.Join(secondAlbum, "two.JPG")
	writeJPEG(t, firstPhoto, 40, 30)
	writeJPEG(t, secondPhoto, 80, 60)
	if err := os.WriteFile(filepath.Join(firstAlbum, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondAlbum, "clip.mp4"), []byte("not a real video"), 0o644); err != nil {
		t.Fatal(err)
	}

	dsn := "file:" + filepath.Join(t.TempDir(), "scanner.db") + "?_pragma=foreign_keys(1)"
	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get test database connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ffmpegPath := filepath.Clean(filepath.Join("..", "..", "tools", "ffmpeg", "ffmpeg"))
	hasFFmpeg := true
	if _, err := os.Stat(ffmpegPath); err != nil {
		hasFFmpeg = false
		ffmpegPath = "missing-ffmpeg-for-test"
	}
	svc := New(db, "missing-ffprobe-for-test", ffmpegPath)
	var stages []string
	firstAlbumEnabledBeforeSecond := false
	stats, err := svc.scan(context.Background(), root, func(stage string, _ Stats) {
		if stage == StageScanMedia && len(stages) > 0 && stages[len(stages)-1] == StageGenerateThumbnails {
			var completed database.Album
			if err := db.Where("album_path = ?", firstAlbum).First(&completed).Error; err != nil {
				t.Fatalf("load first album during second album scan: %v", err)
			}
			if !completed.ScanStatus {
				t.Fatal("first album was not enabled before the second album started")
			}
			firstAlbumEnabledBeforeSecond = true
		}
		if len(stages) == 0 || stages[len(stages)-1] != stage {
			stages = append(stages, stage)
		}
	})
	if err != nil {
		t.Fatalf("scan media: %v", err)
	}
	// Albums are processed as complete units: media scan and thumbnail
	// generation finish for the first album before the second album starts.
	expectedStages := []string{
		StageDiscoverAlbums,
		StageScanMedia, StageGenerateThumbnails,
		StageScanMedia, StageGenerateThumbnails,
	}
	if len(stages) != len(expectedStages) {
		t.Fatalf("unexpected scan stages: got %v, want %v", stages, expectedStages)
	}
	for index := range expectedStages {
		if stages[index] != expectedStages[index] {
			t.Fatalf("unexpected scan stages: got %v, want %v", stages, expectedStages)
		}
	}
	if !firstAlbumEnabledBeforeSecond {
		t.Fatal("did not observe the first album enabled before processing the second album")
	}
	if stats.Albums != 2 || stats.Files != 3 || stats.Images != 2 || stats.Videos != 1 || stats.SkippedFiles != 1 {
		t.Fatalf("unexpected scan stats: %+v", stats)
	}
	if stats.MetadataErrors != 1 {
		t.Fatalf("expected the invalid test video to have one metadata error, got %+v", stats)
	}
	expectedDerivativeErrors := 1
	if !hasFFmpeg {
		expectedDerivativeErrors = 3
	}
	if stats.DerivativeErrors != expectedDerivativeErrors {
		t.Fatalf("expected %d derivative errors, got %+v", expectedDerivativeErrors, stats)
	}

	var albumCount int64
	if err := db.Model(&database.Album{}).Count(&albumCount).Error; err != nil {
		t.Fatal(err)
	}
	if albumCount != 2 {
		t.Fatalf("expected 2 albums, got %d", albumCount)
	}
	var enabledAlbumCount int64
	if err := db.Model(&database.Album{}).Where("scan_status = ?", true).Count(&enabledAlbumCount).Error; err != nil {
		t.Fatal(err)
	}
	if enabledAlbumCount != 2 {
		t.Fatalf("expected thumbnails to enable 2 albums, got %d", enabledAlbumCount)
	}

	var mediaCount int64
	if err := db.Model(&database.Photo{}).Count(&mediaCount).Error; err != nil {
		t.Fatal(err)
	}
	if mediaCount != 3 {
		t.Fatalf("expected 3 media records, got %d", mediaCount)
	}

	var first database.Photo
	if err := db.Where("photo_path = ?", firstPhoto).First(&first).Error; err != nil {
		t.Fatal(err)
	}
	if first.WidthPX == nil || *first.WidthPX != 40 || first.HeightPX == nil || *first.HeightPX != 30 {
		t.Fatalf("unexpected dimensions: width=%v height=%v", first.WidthPX, first.HeightPX)
	}
	if hasFFmpeg {
		if first.ThumbnailPath == nil {
			t.Fatalf("expected generated derivative paths, got %+v", first)
		}
		assertImageSize(t, *first.ThumbnailPath, 533, 400)
		if filepath.Base(filepath.Dir(*first.ThumbnailPath)) != "thumbnails" {
			t.Fatalf("thumbnail was not stored in the album directory: %q", *first.ThumbnailPath)
		}
	}

	if _, err := svc.scan(context.Background(), root, nil); err != nil {
		t.Fatalf("repeat scan media: %v", err)
	}
	if err := db.Model(&database.Photo{}).Count(&mediaCount).Error; err != nil {
		t.Fatal(err)
	}
	if mediaCount != 3 {
		t.Fatalf("repeat scan created duplicates: got %d records", mediaCount)
	}

	if err := db.Model(&database.Photo{}).Where("photo_path = ?", firstPhoto).Update("status", AssetStatusTrashed).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(secondPhoto); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.scan(context.Background(), root, nil); err != nil {
		t.Fatalf("scan after file changes: %v", err)
	}

	if err := db.Where("photo_path = ?", firstPhoto).First(&first).Error; err != nil {
		t.Fatal(err)
	}
	if first.Status != AssetStatusTrashed {
		t.Fatalf("expected trashed status to be preserved, got %q", first.Status)
	}

	var missing database.Photo
	if err := db.Where("photo_path = ?", secondPhoto).First(&missing).Error; err != nil {
		t.Fatal(err)
	}
	if missing.Status != AssetStatusMissing {
		t.Fatalf("expected removed file to be missing, got %q", missing.Status)
	}
}

func TestScanUsesConfiguredAlbumDepth(t *testing.T) {
	root := t.TempDir()
	groupOne := filepath.Join(root, "Group One")
	groupTwo := filepath.Join(root, "Group Two")
	albumOne := filepath.Join(groupOne, "Album One")
	albumTwo := filepath.Join(groupTwo, "Album Two")
	for _, path := range []string{albumOne, albumTwo, filepath.Join(groupOne, ".hidden", "Ignored")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeJPEG(t, filepath.Join(albumOne, "one.jpg"), 20, 20)
	writeJPEG(t, filepath.Join(albumTwo, "two.jpg"), 20, 20)

	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    "file:" + filepath.Join(t.TempDir(), "depth.db") + "?_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Create(&database.AssetDirectory{
		DirectoryName: filepath.Base(root),
		DirectoryPath: root,
		AlbumDepth:    2,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := New(db, "missing-ffprobe-for-test", "missing-ffmpeg-for-test")
	stats, err := svc.scan(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("scan depth-two library: %v", err)
	}
	if stats.Albums != 2 || stats.Images != 2 {
		t.Fatalf("unexpected depth-two stats: %+v", stats)
	}
	var albums []database.Album
	if err := db.Order("album_path ASC").Find(&albums).Error; err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 || albums[0].AlbumPath != albumOne || albums[1].AlbumPath != albumTwo {
		t.Fatalf("unexpected depth-two albums: %+v", albums)
	}
}

func assertImageSize(t *testing.T, path string, width, height int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		t.Fatalf("decode generated image %q: %v", path, err)
	}
	if config.Width != width || config.Height != height {
		t.Fatalf("generated image %q is %dx%d; want %dx%d", path, config.Width, config.Height, width, height)
	}
}

func TestParseFFprobeOutput(t *testing.T) {
	metadata, err := parseFFprobeOutput([]byte(`{
		"streams": [{"width": 1920, "height": 1080, "duration": "N/A"}],
		"format": {"duration": "12.3456"}
	}`))
	if err != nil {
		t.Fatalf("parse ffprobe output: %v", err)
	}
	if metadata.Width == nil || *metadata.Width != 1920 || metadata.Height == nil || *metadata.Height != 1080 {
		t.Fatalf("unexpected video dimensions: %+v", metadata)
	}
	if metadata.DurationMS == nil || *metadata.DurationMS != 12346 {
		t.Fatalf("unexpected video duration: %+v", metadata.DurationMS)
	}
}

func TestClassifyMedia(t *testing.T) {
	tests := map[string]string{
		"photo.JPG":  MediaTypeImage,
		"photo.webp": MediaTypeImage,
		"clip.MP4":   MediaTypeVideo,
		"clip.mov":   MediaTypeVideo,
	}
	for filename, expected := range tests {
		actual, ok := classifyMedia(filename)
		if !ok || actual != expected {
			t.Errorf("classifyMedia(%q) = %q, %v; want %q, true", filename, actual, ok, expected)
		}
	}
	if _, ok := classifyMedia("notes.txt"); ok {
		t.Fatal("expected text file to be ignored")
	}
}

func writeJPEG(t *testing.T, path string, width, height int) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	imageData := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			imageData.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	if err := jpeg.Encode(file, imageData, nil); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
