package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"math"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"airy/internal/database"
	"airy/internal/derivative"

	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StateIdle      = "idle"
	StateScanning  = "scanning"
	StateCompleted = "completed"
	StateFailed    = "failed"
	StateCanceled  = "canceled"

	StageDiscoverAlbums     = "discovering_albums"
	StageScanMedia          = "scanning_media"
	StageGenerateThumbnails = "generating_thumbnails"

	MediaTypeImage = "image"
	MediaTypeVideo = "video"

	AssetStatusActive  = "active"
	AssetStatusMissing = "missing"
	AssetStatusTrashed = "trashed"
)

var (
	ErrAlreadyRunning = errors.New("media scan is already running")
	ErrNotRunning     = errors.New("no media scan is running")
	ErrCanceling      = errors.New("media scan is being canceled")
	ErrNotConfigured  = errors.New("媒体根目录未配置")
)

type Stats struct {
	Albums           int `json:"albums"`
	Files            int `json:"files"`
	Images           int `json:"images"`
	Videos           int `json:"videos"`
	SkippedFiles     int `json:"skipped_files"`
	MetadataErrors   int `json:"metadata_errors"`
	DerivativeErrors int `json:"derivative_errors"`
	ThumbnailsReady  int `json:"thumbnails_ready"`
}

type Status struct {
	State      string     `json:"state"`
	Stage      string     `json:"stage,omitempty"`
	Root       string     `json:"root,omitempty"`
	Queued     int        `json:"queued"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Stats      Stats      `json:"stats"`
	Error      string     `json:"error,omitempty"`
	Canceling  bool       `json:"canceling"`
}

type Scanner struct {
	db          *gorm.DB
	ffprobePath string
	derivatives *derivative.Generator

	mu         sync.RWMutex
	status     Status
	queue      []string
	queued     map[string]struct{}
	scanCancel context.CancelFunc

	automationMu sync.RWMutex
	automation   *directoryMonitor
}

func New(db *gorm.DB, ffprobePath, ffmpegPath string) *Scanner {
	return &Scanner{
		db:          db,
		ffprobePath: strings.TrimSpace(ffprobePath),
		derivatives: derivative.NewGenerator(ffmpegPath),
		queued:      make(map[string]struct{}),
		status: Status{
			State: StateIdle,
		},
	}
}

func (s *Scanner) Status() Status {
	if s == nil {
		return Status{State: StateIdle}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// PathStatus reports whether a resource directory is currently scanning or
// waiting in the serialized scan queue.
func (s *Scanner) PathStatus(root string) (bool, bool) {
	if s == nil {
		return false, false
	}
	root = filepath.Clean(root)
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, queued := s.queued[root]
	return s.status.State == StateScanning && s.status.Root == root, queued
}

// StartPath begins a background scan for a registered resource directory.
// Scans are serialized because derivative generation is intentionally bounded.
func (s *Scanner) StartPath(root string) (Status, error) {
	if s == nil || s.db == nil {
		return Status{State: StateIdle}, ErrNotConfigured
	}
	resolvedRoot, err := ResolveRoot(root)
	if err != nil {
		return s.Status(), fmt.Errorf("open media root: %w", err)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if s.status.State == StateScanning {
		status := s.status
		s.mu.Unlock()
		return status, ErrAlreadyRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.scanCancel = cancel
	s.status = Status{State: StateScanning, Stage: StageDiscoverAlbums, Root: resolvedRoot, StartedAt: &now, Queued: len(s.queue)}
	status := s.status
	s.mu.Unlock()

	go s.runQueued(ctx, resolvedRoot)

	return status, nil
}

// EnqueuePaths schedules one or more resource directories for serialized
// background scanning. Paths already waiting in the queue are not duplicated.
func (s *Scanner) EnqueuePaths(paths []string) (Status, int, error) {
	if s == nil || s.db == nil {
		return Status{State: StateIdle}, 0, ErrNotConfigured
	}

	resolvedPaths := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		resolved, err := ResolveRoot(path)
		if err != nil {
			return s.Status(), 0, fmt.Errorf("open media root: %w", err)
		}
		if _, exists := seen[resolved]; exists {
			continue
		}
		seen[resolved] = struct{}{}
		resolvedPaths = append(resolvedPaths, resolved)
	}

	s.mu.Lock()
	if s.status.State == StateScanning && s.status.Canceling {
		status := s.status
		s.mu.Unlock()
		return status, 0, ErrCanceling
	}
	added := 0
	for _, path := range resolvedPaths {
		if _, exists := s.queued[path]; exists {
			continue
		}
		s.queue = append(s.queue, path)
		s.queued[path] = struct{}{}
		added++
	}

	if s.status.State != StateScanning && len(s.queue) > 0 {
		next := s.popQueueLocked()
		now := time.Now().UTC()
		ctx, cancel := context.WithCancel(context.Background())
		s.scanCancel = cancel
		s.status = Status{State: StateScanning, Stage: StageDiscoverAlbums, Root: next, StartedAt: &now, Queued: len(s.queue)}
		status := s.status
		s.mu.Unlock()
		go s.runQueued(ctx, next)
		return status, added, nil
	}
	s.status.Queued = len(s.queue)
	status := s.status
	s.mu.Unlock()
	return status, added, nil
}

// EnqueueAll schedules every resource directory that has not opted out of
// filesystem watching and global scans.
func (s *Scanner) EnqueueAll(ctx context.Context) (Status, int, error) {
	return s.enqueueActiveDirectories(ctx)
}

func (s *Scanner) enqueueActiveDirectories(ctx context.Context) (Status, int, error) {
	if s == nil || s.db == nil {
		return Status{State: StateIdle}, 0, ErrNotConfigured
	}
	var directories []database.AssetDirectory
	query := s.db.WithContext(ctx).Where("automation_disabled = ?", false).Order("created_at ASC")
	if err := query.Find(&directories).Error; err != nil {
		return s.Status(), 0, fmt.Errorf("list media roots: %w", err)
	}
	if len(directories) == 0 {
		return s.Status(), 0, nil
	}
	current := s.Status()
	paths := make([]string, 0, len(directories))
	for _, directory := range directories {
		resolved, err := ResolveRoot(directory.DirectoryPath)
		if err != nil {
			_ = s.db.WithContext(ctx).Model(&database.AssetDirectory{}).
				Where("id = ?", directory.ID).
				Updates(map[string]any{"scan_status": false, "scan_error": err.Error()}).Error
			continue
		}
		if current.State == StateScanning && current.Root == resolved {
			continue
		}
		paths = append(paths, resolved)
	}
	if len(paths) == 0 {
		return s.Status(), 0, nil
	}
	return s.EnqueuePaths(paths)
}

func (s *Scanner) popQueueLocked() string {
	next := s.queue[0]
	s.queue = s.queue[1:]
	delete(s.queued, next)
	return next
}

// Cancel stops the active scan and discards all queued resource directories.
func (s *Scanner) Cancel() (Status, error) {
	if s == nil {
		return Status{State: StateIdle}, ErrNotRunning
	}
	s.mu.Lock()
	if s.status.State != StateScanning {
		status := s.status
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	s.status.Canceling = true
	s.queue = nil
	clear(s.queued)
	s.status.Queued = 0
	cancel := s.scanCancel
	if cancel != nil {
		cancel()
	}
	status := s.status
	s.mu.Unlock()
	return status, nil
}

func (s *Scanner) runQueued(ctx context.Context, root string) {
	for {
		stats, err := s.scan(ctx, root, s.setProgress)
		finishedAt := time.Now().UTC()

		s.mu.Lock()
		s.status.Stats = stats
		s.status.FinishedAt = &finishedAt
		s.status.Queued = len(s.queue)
		if errors.Is(err, context.Canceled) || s.status.Canceling {
			if s.scanCancel != nil {
				s.scanCancel()
			}
			s.queue = nil
			clear(s.queued)
			s.status.State = StateCanceled
			s.status.Stage = ""
			s.status.Error = ""
			s.status.Queued = 0
			s.status.Canceling = false
			s.scanCancel = nil
			s.mu.Unlock()
			return
		}
		if err != nil {
			s.status.State = StateFailed
			s.status.Error = err.Error()
		} else {
			s.status.State = StateCompleted
		}

		if len(s.queue) == 0 {
			if s.scanCancel != nil {
				s.scanCancel()
			}
			s.scanCancel = nil
			s.mu.Unlock()
			return
		}
		root = s.popQueueLocked()
		now := time.Now().UTC()
		if s.scanCancel != nil {
			s.scanCancel()
		}
		ctx, s.scanCancel = context.WithCancel(context.Background())
		s.status = Status{State: StateScanning, Stage: StageDiscoverAlbums, Root: root, StartedAt: &now, Queued: len(s.queue)}
		s.mu.Unlock()
	}
}

func (s *Scanner) setProgress(stage string, stats Stats) {
	s.mu.Lock()
	s.status.Stage = stage
	s.status.Stats = stats
	s.mu.Unlock()
}

func (s *Scanner) scan(ctx context.Context, requestedRoot string, progress func(string, Stats)) (stats Stats, scanErr error) {
	root, err := ResolveRoot(requestedRoot)
	if err != nil {
		return stats, fmt.Errorf("open media root: %w", err)
	}
	defer releaseIdleScanMemory()

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	directory := database.AssetDirectory{}
	err = s.db.WithContext(ctx).Where("directory_path = ?", root).First(&directory).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		directory = database.AssetDirectory{
			DirectoryName: filepath.Base(root),
			DirectoryPath: root,
		}
		err = s.db.WithContext(ctx).Create(&directory).Error
	}
	if err != nil {
		return stats, fmt.Errorf("save media root: %w", err)
	}

	previousStatus := directory.ScanStatus
	previousError := directory.ScanError
	if err := s.db.WithContext(ctx).Model(&directory).Updates(map[string]any{
		"directory_name": filepath.Base(root),
		"scan_status":    false,
		"scan_error":     "",
	}).Error; err != nil {
		return stats, fmt.Errorf("mark media root scanning: %w", err)
	}

	defer func() {
		updates := make(map[string]any, 3)
		if errors.Is(scanErr, context.Canceled) {
			updates["scan_status"] = previousStatus
			updates["scan_error"] = previousError
		} else if scanErr != nil {
			updates["last_scanned_at"] = time.Now().UTC()
			updates["scan_status"] = false
			updates["scan_error"] = scanErr.Error()
		} else {
			updates["last_scanned_at"] = time.Now().UTC()
			updates["scan_status"] = true
			updates["scan_error"] = ""
		}
		_ = s.db.Model(&database.AssetDirectory{}).Where("id = ?", directory.ID).Updates(updates).Error
	}()

	if progress != nil {
		progress(StageDiscoverAlbums, stats)
	}

	albumPaths, err := discoverAlbumPaths(ctx, root, int(normalizedAlbumDepth(directory.AlbumDepth)))
	if err != nil {
		return stats, fmt.Errorf("discover albums: %w", err)
	}

	albums := make([]database.Album, 0, len(albumPaths))
	albumIDs := make([]uint, 0, len(albumPaths))
	newAlbumIDs := make(map[uint]bool)
	for _, albumPath := range albumPaths {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		albumName := filepath.Base(albumPath)
		album := database.Album{}
		result := s.db.WithContext(ctx).
			Where("album_path = ?", albumPath).
			Attrs(database.Album{
				PublishedDate: database.AlbumPublicationDate(albumName, startedAt),
				DirectoryID:   directory.ID,
				AlbumName:     albumName,
				AlbumPath:     albumPath,
			}).FirstOrCreate(&album)
		if result.Error != nil {
			return stats, fmt.Errorf("save album %q: %w", albumName, result.Error)
		}
		newAlbumIDs[album.ID] = result.RowsAffected > 0 || album.PublishedDate == ""
		// Existing titles may have been edited through CSV; keep them on rescan.
		if album.DirectoryID != directory.ID {
			if err := s.db.WithContext(ctx).Model(&album).Updates(map[string]any{
				"directory_id": directory.ID,
			}).Error; err != nil {
				return stats, fmt.Errorf("update album %q: %w", albumName, err)
			}
		}
		albums = append(albums, album)
		albumIDs = append(albumIDs, album.ID)
		stats.Albums++
		if progress != nil {
			progress(StageDiscoverAlbums, stats)
		}
	}

	// Albums that disappeared are left in the database for auditability, but
	// their active media are marked missing before the remaining scan stages.
	missingAlbums := s.db.WithContext(ctx).Model(&database.Album{}).
		Where("directory_id = ?", directory.ID)
	if len(albumIDs) > 0 {
		missingAlbums = missingAlbums.Where("id NOT IN ?", albumIDs)
	}
	var missingAlbumIDs []uint
	if err := missingAlbums.Pluck("id", &missingAlbumIDs).Error; err != nil {
		return stats, fmt.Errorf("load missing albums: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&database.Album{}).
		Where("id IN ?", missingAlbumIDs).
		Update("scan_status", false).Error; err != nil {
		return stats, fmt.Errorf("mark missing albums: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(&database.Photo{}).
		Where("album_id IN ? AND status = ?", missingAlbumIDs, AssetStatusActive).
		Update("status", AssetStatusMissing).Error; err != nil {
		return stats, fmt.Errorf("mark missing albums: %w", err)
	}
	if err := database.RefreshEntityImageIDsForAlbumIDs(s.db.WithContext(ctx), missingAlbumIDs); err != nil {
		return stats, fmt.Errorf("refresh missing album covers: %w", err)
	}

	for index := range albums {
		album := albums[index]
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		// Each album is a complete unit of work: hide the album while its media
		// records are refreshed, generate every thumbnail, then enable it before
		// moving to the next album. Albums that have not been reached yet keep
		// their previous state, which makes canceling a scan less disruptive.
		if err := s.db.WithContext(ctx).Model(&database.Album{}).
			Where("id = ?", album.ID).
			Update("scan_status", false).Error; err != nil {
			return stats, fmt.Errorf("disable album %q: %w", album.AlbumName, err)
		}
		if progress != nil {
			progress(StageScanMedia, stats)
		}
		albumStats, err := s.scanAlbumMedia(ctx, album, startedAt)
		if err != nil {
			return stats, fmt.Errorf("scan album %q: %w", album.AlbumName, err)
		}
		if newAlbumIDs[album.ID] {
			date, err := database.ResolveAlbumPublicationDate(s.db.WithContext(ctx), album, startedAt)
			if err != nil {
				return stats, fmt.Errorf("resolve album publication date: %w", err)
			}
			if err := s.db.WithContext(ctx).Model(&database.Album{}).
				Where("id = ? AND album_name = ? AND published_date = ?", album.ID, album.AlbumName, album.PublishedDate).
				UpdateColumn("published_date", date).Error; err != nil {
				return stats, fmt.Errorf("save album publication date: %w", err)
			}
		}
		stats.Files += albumStats.Files
		stats.Images += albumStats.Images
		stats.Videos += albumStats.Videos
		stats.SkippedFiles += albumStats.SkippedFiles
		stats.MetadataErrors += albumStats.MetadataErrors
		if progress != nil {
			progress(StageScanMedia, stats)
		}
		if err := s.generateAlbumDerivatives(ctx, album, &stats, progress); err != nil {
			return stats, err
		}
		// Drop references held by the discovery slice as soon as this album is
		// complete. Periodically return a large idle heap to the OS without
		// forcing a collection after every album.
		albums[index] = database.Album{}
		if (index+1)%8 == 0 {
			releaseIdleScanMemory()
		}
	}
	return stats, nil
}

func normalizedAlbumDepth(depth uint) uint {
	if depth == 0 {
		return 1
	}
	return depth
}

func discoverAlbumPaths(
	ctx context.Context,
	root string,
	depth int,
) ([]string, error) {
	if depth < 1 {
		depth = 1
	}
	albums := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") || derivative.IsGeneratedDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		currentDepth := len(strings.Split(relative, string(filepath.Separator)))
		if currentDepth == depth {
			albums = append(albums, path)
			return filepath.SkipDir
		}
		return nil
	})
	return albums, err
}

func (s *Scanner) scanAlbumMedia(ctx context.Context, album database.Album, seenAt time.Time) (Stats, error) {
	stats := Stats{}

	var stored []storedMediaSnapshot
	if err := s.db.WithContext(ctx).
		Model(&database.Photo{}).
		Select("id", "photo_path", "thumbnail_path", "file_size", "width_px", "height_px", "duration_ms", "modified_at").
		Where("album_id = ?", album.ID).Find(&stored).Error; err != nil {
		return stats, err
	}
	existing := make(map[string]storedMediaSnapshot, len(stored))
	for _, photo := range stored {
		existing[photo.PhotoPath] = photo
	}
	stored = nil

	type candidate struct {
		path      string
		name      string
		mediaType string
	}

	type result struct {
		photo         database.Photo
		mediaType     string
		metadataError bool
		err           error
	}
	workerCount := runtime.GOMAXPROCS(0) * 2
	if workerCount < 2 {
		workerCount = 2
	}
	if workerCount > 8 {
		workerCount = 8
	}

	queueSize := max(workerCount*2, 1)
	jobs := make(chan candidate, queueSize)
	results := make(chan result, queueSize)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				if err := ctx.Err(); err != nil {
					results <- result{mediaType: item.mediaType, err: err}
					continue
				}
				photo, metadataError, err := s.readMedia(
					ctx, album, item.path, item.name, item.mediaType, seenAt, existing[item.path],
				)
				results <- result{photo: photo, mediaType: item.mediaType, metadataError: metadataError, err: err}
			}
		}()
	}
	type walkOutcome struct {
		skipped int
		err     error
	}
	walkDone := make(chan walkOutcome, 1)
	go func() {
		defer close(jobs)
		outcome := walkOutcome{}
		outcome.err = filepath.WalkDir(album.AlbumPath, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if path == album.AlbumPath {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				if path != album.AlbumPath && derivative.IsGeneratedDirectory(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			mediaType, supported := classifyMedia(path)
			if !supported {
				outcome.skipped++
				return nil
			}
			select {
			case jobs <- candidate{path: path, name: entry.Name(), mediaType: mediaType}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		walkDone <- outcome
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	const mediaBatchSize = 200
	photos := make([]database.Photo, 0, mediaBatchSize)
	var canceledErr error
	var persistErr error
	for item := range results {
		stats.Files++
		if item.mediaType == MediaTypeImage {
			stats.Images++
		} else {
			stats.Videos++
		}
		if item.metadataError || item.err != nil {
			stats.MetadataErrors++
		}
		if errors.Is(item.err, context.Canceled) {
			canceledErr = item.err
		}
		if item.err == nil && persistErr == nil {
			photos = append(photos, item.photo)
			if len(photos) == mediaBatchSize {
				if err := s.saveScannedMedia(ctx, photos); err != nil {
					persistErr = err
				} else {
					photos = photos[:0]
				}
			}
		}
	}
	walkResult := <-walkDone
	stats.SkippedFiles += walkResult.skipped
	if persistErr != nil {
		return stats, persistErr
	}
	if walkResult.err != nil {
		return stats, walkResult.err
	}
	if canceledErr != nil {
		return stats, canceledErr
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	if err := s.saveScannedMedia(ctx, photos); err != nil {
		return stats, err
	}

	if err := s.db.WithContext(ctx).Model(&database.Photo{}).
		Where("album_id = ? AND status = ? AND last_seen_at < ?", album.ID, AssetStatusActive, seenAt).
		Update("status", AssetStatusMissing).Error; err != nil {
		return stats, err
	}

	return stats, nil
}

type storedMediaSnapshot struct {
	ID            uint
	PhotoPath     string
	ThumbnailPath *string
	FileSize      int64
	WidthPX       *uint
	HeightPX      *uint
	DurationMS    *int64
	ModifiedAt    time.Time
}

func (s *Scanner) saveScannedMedia(ctx context.Context, photos []database.Photo) error {
	if len(photos) == 0 {
		return nil
	}
	paths := make([]string, 0, len(photos))
	for _, photo := range photos {
		paths = append(paths, photo.PhotoPath)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "photo_path"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"album_id", "photo_name", "thumbnail_path", "media_type", "file_size", "height_px",
				"width_px", "duration_ms", "mime_type", "modified_at", "last_seen_at", "updated_at",
			}),
		}).CreateInBatches(photos, len(photos)).Error; err != nil {
			return err
		}
		return tx.Model(&database.Photo{}).
			Where("photo_path IN ? AND status = ?", paths, AssetStatusMissing).
			Update("status", AssetStatusActive).Error
	})
}

func (s *Scanner) generateAlbumDerivatives(
	ctx context.Context,
	album database.Album,
	stats *Stats,
	progress func(string, Stats),
) error {
	if progress != nil {
		progress(StageGenerateThumbnails, *stats)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	const pageSize = 200
	var lastPhotoID uint
	for {
		var photos []database.Photo
		if err := s.db.WithContext(ctx).
			Select("id", "photo_path", "thumbnail_path", "media_type", "modified_at").
			Where("album_id = ? AND status = ? AND id > ?", album.ID, AssetStatusActive, lastPhotoID).
			Order("id ASC").Limit(pageSize).Find(&photos).Error; err != nil {
			return fmt.Errorf("load derivatives for album %q: %w", album.AlbumName, err)
		}
		if len(photos) == 0 {
			break
		}
		lastPhotoID = photos[len(photos)-1].ID

		updates, err := s.generateAlbumThumbnails(ctx, album, photos, stats, progress)
		if err != nil {
			return err
		}
		if err := s.saveThumbnailPaths(ctx, updates); err != nil {
			return fmt.Errorf("save thumbnails for album %q: %w", album.AlbumName, err)
		}
	}
	if err := s.db.WithContext(ctx).Model(&database.Album{}).
		Where("id = ?", album.ID).
		Update("scan_status", true).Error; err != nil {
		return fmt.Errorf("enable album %q: %w", album.AlbumName, err)
	}
	if err := database.RefreshEntityImageIDsForAlbumIDs(s.db.WithContext(ctx), []uint{album.ID}); err != nil {
		return fmt.Errorf("refresh album covers for %q: %w", album.AlbumName, err)
	}
	return nil
}

func (s *Scanner) generateAlbumThumbnails(
	ctx context.Context,
	album database.Album,
	photos []database.Photo,
	stats *Stats,
	progress func(string, Stats),
) ([]derivativePathUpdate, error) {
	type result struct {
		photo database.Photo
		path  string
		err   error
	}

	workerCount := min(s.derivatives.Concurrency(), len(photos))
	if workerCount == 0 {
		return nil, nil
	}
	queueSize := max(workerCount*2, 1)
	jobs := make(chan database.Photo, queueSize)
	results := make(chan result, queueSize)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for photo := range jobs {
				if err := ctx.Err(); err != nil {
					results <- result{photo: photo, err: err}
					continue
				}
				path, err := s.derivatives.EnsureThumbnail(
					ctx, album.AlbumPath, photo.PhotoPath, photo.MediaType, photo.ModifiedAt,
				)
				results <- result{photo: photo, path: path, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, photo := range photos {
			select {
			case jobs <- photo:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	updates := make([]derivativePathUpdate, 0, len(photos))
	var canceledErr error
	for item := range results {
		if errors.Is(item.err, context.Canceled) {
			canceledErr = item.err
		} else if item.err != nil {
			stats.DerivativeErrors++
		} else {
			if item.photo.ThumbnailPath == nil || *item.photo.ThumbnailPath != item.path {
				updates = append(updates, derivativePathUpdate{id: item.photo.ID, path: item.path})
			}
			stats.ThumbnailsReady++
		}
		if progress != nil {
			progress(StageGenerateThumbnails, *stats)
		}
	}
	if canceledErr != nil {
		return nil, canceledErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return updates, nil
}

func releaseIdleScanMemory() {
	const minimumIdleHeap = 64 << 20
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	if memory.HeapIdle > memory.HeapReleased && memory.HeapIdle-memory.HeapReleased >= minimumIdleHeap {
		debug.FreeOSMemory()
	}
}

type derivativePathUpdate struct {
	id   uint
	path string
}

// saveThumbnailPaths writes one CASE update per batch. A large scan used to
// commit one UPDATE for every generated file, which made remote PostgreSQL
// scans progressively spend more time waiting on database round trips.
func (s *Scanner) saveThumbnailPaths(ctx context.Context, updates []derivativePathUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	const batchSize = 200
	for start := 0; start < len(updates); start += batchSize {
		end := min(start+batchSize, len(updates))
		batch := updates[start:end]
		ids := make([]uint, 0, len(batch))
		arguments := make([]any, 0, len(batch)*2)
		var expression strings.Builder
		expression.WriteString("CASE id")
		for _, update := range batch {
			expression.WriteString(" WHEN ? THEN ?")
			arguments = append(arguments, update.id, update.path)
			ids = append(ids, update.id)
		}
		expression.WriteString(" ELSE ")
		expression.WriteString("thumbnail_path")
		expression.WriteString(" END")

		if err := s.db.WithContext(ctx).Model(&database.Photo{}).
			Where("id IN ?", ids).
			Update("thumbnail_path", gorm.Expr(expression.String(), arguments...)).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) readMedia(
	ctx context.Context,
	album database.Album,
	path, name, mediaType string,
	seenAt time.Time,
	existing storedMediaSnapshot,
) (database.Photo, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return database.Photo{}, false, err
	}

	modifiedAt := info.ModTime().UTC().Truncate(time.Microsecond)
	photo := database.Photo{
		AlbumID:    album.ID,
		PhotoName:  name,
		PhotoPath:  path,
		MediaType:  mediaType,
		Status:     AssetStatusActive,
		FileSize:   info.Size(),
		MIMEType:   mediaMIMEType(path, mediaType),
		ModifiedAt: modifiedAt,
		LastSeenAt: seenAt,
	}

	unchanged := existing.ID != 0 && existing.FileSize == info.Size() && existing.ModifiedAt.UTC().Equal(modifiedAt)
	if unchanged {
		photo.ThumbnailPath = existing.ThumbnailPath
	}
	metadataComplete := existing.WidthPX != nil && existing.HeightPX != nil
	if mediaType == MediaTypeVideo {
		metadataComplete = metadataComplete && existing.DurationMS != nil
	}
	metadataError := false
	if unchanged && metadataComplete {
		photo.WidthPX = existing.WidthPX
		photo.HeightPX = existing.HeightPX
		photo.DurationMS = existing.DurationMS
	} else if mediaType == MediaTypeImage {
		width, height, err := imageDimensions(path)
		if err != nil {
			metadataError = true
		} else {
			photo.WidthPX = &width
			photo.HeightPX = &height
		}
	} else {
		metadata, err := probeVideo(ctx, s.ffprobePath, path)
		if err != nil {
			metadataError = true
		} else {
			photo.WidthPX = metadata.Width
			photo.HeightPX = metadata.Height
			photo.DurationMS = metadata.DurationMS
		}
	}

	return photo, metadataError, nil
}

// ResolveRoot normalizes a resource path and verifies that it is an existing directory.
func ResolveRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("directory path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func classifyMedia(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return MediaTypeImage, true
	case ".mp4", ".m4v", ".mov", ".webm", ".mkv", ".avi":
		return MediaTypeVideo, true
	default:
		return "", false
	}
}

func mediaMIMEType(path, mediaType string) string {
	if detected := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); detected != "" {
		if separator := strings.IndexByte(detected, ';'); separator >= 0 {
			return detected[:separator]
		}
		return detected
	}
	if mediaType == MediaTypeVideo {
		return "video/octet-stream"
	}
	return "image/octet-stream"
}

func imageDimensions(path string) (uint, uint, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	return uint(config.Width), uint(config.Height), nil
}

type videoMetadata struct {
	Width      *uint
	Height     *uint
	DurationMS *int64
}

type ffprobeOutput struct {
	Streams []struct {
		Width    uint   `json:"width"`
		Height   uint   `json:"height"`
		Duration string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probeVideo(ctx context.Context, ffprobePath, path string) (videoMetadata, error) {
	if ffprobePath == "" {
		return videoMetadata{}, errors.New("ffprobe is not configured")
	}
	output, err := exec.CommandContext(
		ctx,
		ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration:format=duration",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return videoMetadata{}, fmt.Errorf("run ffprobe: %w", err)
	}
	return parseFFprobeOutput(output)
}

func parseFFprobeOutput(output []byte) (videoMetadata, error) {
	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return videoMetadata{}, fmt.Errorf("decode ffprobe output: %w", err)
	}
	if len(probe.Streams) == 0 {
		return videoMetadata{}, errors.New("ffprobe returned no video stream")
	}

	metadata := videoMetadata{}
	stream := probe.Streams[0]
	if stream.Width > 0 {
		width := stream.Width
		metadata.Width = &width
	}
	if stream.Height > 0 {
		height := stream.Height
		metadata.Height = &height
	}

	duration := probe.Format.Duration
	if duration == "" || duration == "N/A" {
		duration = stream.Duration
	}
	if duration != "" && duration != "N/A" {
		seconds, err := strconv.ParseFloat(duration, 64)
		if err != nil {
			return videoMetadata{}, fmt.Errorf("parse video duration %q: %w", duration, err)
		}
		durationMS := int64(math.Round(seconds * 1000))
		metadata.DurationMS = &durationMS
	}

	return metadata, nil
}
