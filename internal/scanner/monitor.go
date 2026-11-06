package scanner

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"airy/internal/database"
	"airy/internal/derivative"

	"github.com/fsnotify/fsnotify"
)

const (
	albumCreateScanDelay = 5 * time.Minute
	configSyncInterval   = 5 * time.Second
)

// AutomationStatus describes the runtime state of a directory watcher.
type AutomationStatus struct {
	Watching   bool   `json:"watching"`
	WatchError string `json:"watch_error,omitempty"`
}

type directoryMonitor struct {
	scanner *Scanner
	watcher *fsnotify.Watcher
	cancel  context.CancelFunc
	wake    chan struct{}
	done    chan struct{}

	configs   map[uint]database.AssetDirectory
	watched   map[uint]map[string]struct{}
	pending   map[uint]time.Time
	schedule  database.ScanSchedule
	scanDelay time.Duration

	statusMu sync.RWMutex
	statuses map[uint]AutomationStatus
}

// StartAutomation starts per-directory filesystem watching and the single
// global recurring scan schedule. Both are disabled until explicitly enabled.
func (s *Scanner) StartAutomation(parent context.Context) error {
	return s.startAutomation(parent, albumCreateScanDelay)
}

func (s *Scanner) startAutomation(parent context.Context, scanDelay time.Duration) error {
	if s == nil || s.db == nil {
		return ErrNotConfigured
	}

	s.automationMu.Lock()
	defer s.automationMu.Unlock()
	if s.automation != nil {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	monitor := &directoryMonitor{
		scanner:   s,
		watcher:   watcher,
		cancel:    cancel,
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
		configs:   make(map[uint]database.AssetDirectory),
		watched:   make(map[uint]map[string]struct{}),
		pending:   make(map[uint]time.Time),
		statuses:  make(map[uint]AutomationStatus),
		scanDelay: scanDelay,
	}
	s.automation = monitor
	go monitor.run(ctx)
	return nil
}

// StopAutomation stops all active filesystem watchers.
func (s *Scanner) StopAutomation() {
	if s == nil {
		return
	}
	s.automationMu.Lock()
	monitor := s.automation
	s.automation = nil
	s.automationMu.Unlock()
	if monitor == nil {
		return
	}
	monitor.cancel()
	<-monitor.done
}

// RefreshAutomation applies directory setting changes without restarting the
// server. The monitor also reconciles automatically every few seconds.
func (s *Scanner) RefreshAutomation() {
	if s == nil {
		return
	}
	s.automationMu.RLock()
	monitor := s.automation
	s.automationMu.RUnlock()
	if monitor == nil {
		return
	}
	select {
	case monitor.wake <- struct{}{}:
	default:
	}
}

func (s *Scanner) DirectoryAutomationStatus(id uint) AutomationStatus {
	if s == nil {
		return AutomationStatus{}
	}
	s.automationMu.RLock()
	monitor := s.automation
	s.automationMu.RUnlock()
	if monitor == nil {
		return AutomationStatus{}
	}
	monitor.statusMu.RLock()
	defer monitor.statusMu.RUnlock()
	return monitor.statuses[id]
}

func (m *directoryMonitor) run(ctx context.Context) {
	defer close(m.done)
	defer m.watcher.Close()

	if err := m.syncConfigs(ctx, time.Now()); err != nil {
		slog.Warn("初始化目录自动化失败", "error", err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastSync := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
			if err := m.syncConfigs(ctx, time.Now()); err != nil {
				slog.Warn("刷新目录自动化失败", "error", err)
			}
			lastSync = time.Now()
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			m.handleEvent(event)
		case err, ok := <-m.watcher.Errors:
			if ok {
				slog.Warn("目录监听器发生错误", "error", err)
			}
		case now := <-ticker.C:
			if now.Sub(lastSync) >= configSyncInterval {
				if err := m.syncConfigs(ctx, now); err != nil {
					slog.Warn("同步目录自动化状态失败", "error", err)
				}
				lastSync = now
			}
			m.dispatchDue(ctx, now)
		}
	}
}

func (m *directoryMonitor) syncConfigs(ctx context.Context, now time.Time) error {
	var schedule database.ScanSchedule
	if err := m.scanner.db.WithContext(ctx).First(&schedule, 1).Error; err != nil {
		return err
	}
	if schedule.Enabled && (schedule.NextScanAt == nil || !isDailyScanTime(*schedule.NextScanAt)) {
		next := NextDailyScan(now)
		if err := m.scanner.db.WithContext(ctx).Model(&schedule).Update("next_scan_at", next).Error; err != nil {
			return err
		}
		schedule.NextScanAt = &next
	} else if !schedule.Enabled && schedule.NextScanAt != nil {
		if err := m.scanner.db.WithContext(ctx).Model(&schedule).Update("next_scan_at", nil).Error; err != nil {
			return err
		}
		schedule.NextScanAt = nil
	}
	m.schedule = schedule

	var directories []database.AssetDirectory
	if err := m.scanner.db.WithContext(ctx).Order("created_at ASC").Find(&directories).Error; err != nil {
		return err
	}

	active := make(map[uint]struct{}, len(directories))
	for _, directory := range directories {
		active[directory.ID] = struct{}{}
		previous, existed := m.configs[directory.ID]
		m.configs[directory.ID] = directory

		if directory.AutomationDisabled {
			m.removeWatch(directory.ID)
			m.setStatus(directory.ID, AutomationStatus{})
		} else if !existed || previous.DirectoryPath != directory.DirectoryPath ||
			previous.AlbumDepth != directory.AlbumDepth || len(m.watched[directory.ID]) == 0 {
			m.removeWatch(directory.ID)
			if err := m.addTree(directory, directory.DirectoryPath); err != nil {
				m.setStatus(directory.ID, AutomationStatus{WatchError: err.Error()})
			} else {
				m.setStatus(directory.ID, AutomationStatus{Watching: true})
			}
		}
	}

	for id := range m.configs {
		if _, exists := active[id]; exists {
			continue
		}
		m.removeWatch(id)
		delete(m.configs, id)
		delete(m.pending, id)
		m.statusMu.Lock()
		delete(m.statuses, id)
		m.statusMu.Unlock()
	}
	return nil
}

func (m *directoryMonitor) handleEvent(event fsnotify.Event) {
	directory, ok := m.ownerForPath(event.Name)
	if !ok || isGeneratedPath(directory.DirectoryPath, event.Name) {
		return
	}

	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		delete(m.watched[directory.ID], filepath.Clean(event.Name))
	}
	if event.Op&fsnotify.Create == 0 {
		return
	}
	info, err := os.Stat(event.Name)
	if err != nil || !info.IsDir() {
		return
	}
	depth, inside := pathDepth(directory.DirectoryPath, event.Name)
	albumDepth := int(normalizedAlbumDepth(directory.AlbumDepth))
	if !inside || depth < 1 || depth > albumDepth {
		return
	}
	if depth < albumDepth {
		if err := m.addTree(directory, event.Name); err != nil {
			m.setStatus(directory.ID, AutomationStatus{WatchError: err.Error()})
		}
	}
	m.pending[directory.ID] = time.Now().Add(m.scanDelay)
}

func (m *directoryMonitor) dispatchDue(ctx context.Context, now time.Time) {
	for id, due := range m.pending {
		if now.Before(due) {
			continue
		}
		if directory, exists := m.configs[id]; exists && !directory.AutomationDisabled {
			if _, _, err := m.scanner.EnqueuePaths([]string{directory.DirectoryPath}); err != nil && !errors.Is(err, ErrCanceling) {
				m.setStatus(id, AutomationStatus{WatchError: err.Error()})
			}
		}
		delete(m.pending, id)
	}

	if !m.schedule.Enabled || m.schedule.NextScanAt == nil || now.Before(*m.schedule.NextScanAt) {
		return
	}
	if _, _, err := m.scanner.EnqueueAll(ctx); err != nil && !errors.Is(err, ErrCanceling) {
		slog.Warn("定时扫描加入队列失败", "error", err)
	}
	next := NextDailyScan(now)
	if err := m.scanner.db.WithContext(ctx).Model(&database.ScanSchedule{}).
		Where("id = ?", m.schedule.ID).
		Updates(map[string]any{"next_scan_at": next}).Error; err != nil {
		slog.Warn("更新下次定时扫描时间失败", "error", err)
		return
	}
	m.schedule.NextScanAt = &next
}

func (m *directoryMonitor) ownerForPath(path string) (database.AssetDirectory, bool) {
	cleaned := filepath.Clean(path)
	for _, directory := range m.configs {
		if directory.AutomationDisabled {
			continue
		}
		relative, err := filepath.Rel(directory.DirectoryPath, cleaned)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return directory, true
		}
	}
	return database.AssetDirectory{}, false
}

func (m *directoryMonitor) addTree(directory database.AssetDirectory, start string) error {
	id := directory.ID
	if m.watched[id] == nil {
		m.watched[id] = make(map[string]struct{})
	}
	root := filepath.Clean(directory.DirectoryPath)
	// A watcher on an album's parent is enough to observe the album directory
	// being created. Album directories themselves are intentionally not watched,
	// so media file writes cannot create thousands of watches or trigger scans.
	maxDepth := int(normalizedAlbumDepth(directory.AlbumDepth)) - 1
	return filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		if path != root && derivative.IsGeneratedDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		path = filepath.Clean(path)
		depth, inside := pathDepth(root, path)
		if !inside {
			return filepath.SkipDir
		}
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if _, exists := m.watched[id][path]; exists {
			if depth == maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if err := m.watcher.Add(path); err != nil {
			return err
		}
		m.watched[id][path] = struct{}{}
		if depth == maxDepth {
			return filepath.SkipDir
		}
		return nil
	})
}

func pathDepth(root, path string) (int, bool) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return 0, false
	}
	if relative == "." {
		return 0, true
	}
	return len(strings.Split(relative, string(filepath.Separator))), true
}

func (m *directoryMonitor) removeWatch(id uint) {
	for path := range m.watched[id] {
		if err := m.watcher.Remove(path); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			slog.Debug("移除目录监听失败", "path", path, "error", err)
		}
	}
	delete(m.watched, id)
}

func (m *directoryMonitor) setStatus(id uint, status AutomationStatus) {
	m.statusMu.Lock()
	m.statuses[id] = status
	m.statusMu.Unlock()
}

func isGeneratedPath(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." {
		return false
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if derivative.IsGeneratedDirectory(part) {
			return true
		}
	}
	return false
}
