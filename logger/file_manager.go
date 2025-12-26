package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// LogFileManager manages log file creation, rotation, and cleanup
type LogFileManager struct {
	baseDir       string
	retentionDays int
	mu            sync.Mutex
	writers       map[string]*bufferedWriter
	flushTicker   *time.Ticker
	stopChan      chan struct{}
}

type bufferedWriter struct {
	file   *os.File
	writer *bufio.Writer
	path   string
	date   string
}

// NewLogFileManager creates a new log file manager
func NewLogFileManager(baseDir string, retentionDays int) (*LogFileManager, error) {
	// Create base directory and subdirectories
	dirs := []string{
		baseDir,
		filepath.Join(baseDir, "requests"),
		filepath.Join(baseDir, "events", "lock"),
		filepath.Join(baseDir, "events", "tool"),
		filepath.Join(baseDir, "metrics"),
		filepath.Join(baseDir, "summary"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	lfm := &LogFileManager{
		baseDir:       baseDir,
		retentionDays: retentionDays,
		writers:       make(map[string]*bufferedWriter),
		flushTicker:   time.NewTicker(5 * time.Second),
		stopChan:      make(chan struct{}),
	}

	// Start background flush goroutine
	go lfm.flushLoop()

	log.Info().
		Str("base_dir", baseDir).
		Int("retention_days", retentionDays).
		Msg("Log file manager initialized")

	return lfm, nil
}

// flushLoop periodically flushes all buffered writers
func (lfm *LogFileManager) flushLoop() {
	for {
		select {
		case <-lfm.flushTicker.C:
			lfm.FlushAll()
		case <-lfm.stopChan:
			return
		}
	}
}

// getWriter returns a buffered writer for the given log type and date
func (lfm *LogFileManager) getWriter(logType string) (*bufferedWriter, error) {
	lfm.mu.Lock()
	defer lfm.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	key := logType + ":" + today

	// Check if we already have a writer for today
	if bw, ok := lfm.writers[key]; ok {
		return bw, nil
	}

	// Close old writer for this log type if exists (date changed)
	for k, bw := range lfm.writers {
		if strings.HasPrefix(k, logType+":") && k != key {
			bw.writer.Flush()
			bw.file.Close()
			delete(lfm.writers, k)
		}
	}

	// Create new file
	var path string
	switch logType {
	case "requests":
		path = filepath.Join(lfm.baseDir, "requests", today+".jsonl")
	case "lock_events":
		path = filepath.Join(lfm.baseDir, "events", "lock", today+".jsonl")
	case "tool_events":
		path = filepath.Join(lfm.baseDir, "events", "tool", today+".jsonl")
	case "metrics":
		path = filepath.Join(lfm.baseDir, "metrics", today+".jsonl")
	case "summary":
		path = filepath.Join(lfm.baseDir, "summary", today+".json")
	default:
		return nil, fmt.Errorf("unknown log type: %s", logType)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", path, err)
	}

	bw := &bufferedWriter{
		file:   file,
		writer: bufio.NewWriter(file),
		path:   path,
		date:   today,
	}

	lfm.writers[key] = bw
	return bw, nil
}

// WriteJSON writes a JSON object to the specified log type
func (lfm *LogFileManager) WriteJSON(logType string, data interface{}) error {
	bw, err := lfm.getWriter(logType)
	if err != nil {
		return err
	}

	lfm.mu.Lock()
	defer lfm.mu.Unlock()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal log data: %w", err)
	}

	if _, err := bw.writer.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write log data: %w", err)
	}

	if _, err := bw.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// WriteSummary writes the daily summary (overwrites the file)
func (lfm *LogFileManager) WriteSummary(data interface{}) error {
	today := time.Now().Format("2006-01-02")
	path := filepath.Join(lfm.baseDir, "summary", today+".json")

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal summary data: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write summary file: %w", err)
	}

	return nil
}

// FlushAll flushes all buffered writers
func (lfm *LogFileManager) FlushAll() {
	lfm.mu.Lock()
	defer lfm.mu.Unlock()

	for _, bw := range lfm.writers {
		bw.writer.Flush()
	}
}

// Close closes all open files
func (lfm *LogFileManager) Close() error {
	lfm.flushTicker.Stop()
	close(lfm.stopChan)

	lfm.mu.Lock()
	defer lfm.mu.Unlock()

	var lastErr error
	for _, bw := range lfm.writers {
		bw.writer.Flush()
		if err := bw.file.Close(); err != nil {
			lastErr = err
		}
	}

	lfm.writers = make(map[string]*bufferedWriter)
	return lastErr
}

// CleanupOldLogs removes log files older than retention days
func (lfm *LogFileManager) CleanupOldLogs() (int, error) {
	cutoff := time.Now().AddDate(0, 0, -lfm.retentionDays)
	removed := 0

	dirs := []string{
		filepath.Join(lfm.baseDir, "requests"),
		filepath.Join(lfm.baseDir, "events", "lock"),
		filepath.Join(lfm.baseDir, "events", "tool"),
		filepath.Join(lfm.baseDir, "metrics"),
		filepath.Join(lfm.baseDir, "summary"),
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Extract date from filename (e.g., "2025-12-25.jsonl")
			name := entry.Name()
			dateStr := strings.TrimSuffix(strings.TrimSuffix(name, ".jsonl"), ".json")
			fileDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue
			}

			if fileDate.Before(cutoff) {
				path := filepath.Join(dir, name)
				if err := os.Remove(path); err != nil {
					log.Warn().Err(err).Str("path", path).Msg("Failed to remove old log file")
				} else {
					removed++
					log.Debug().Str("path", path).Msg("Removed old log file")
				}
			}
		}
	}

	if removed > 0 {
		log.Info().Int("count", removed).Msg("Cleaned up old log files")
	}

	return removed, nil
}

// GetStats returns statistics about log files
func (lfm *LogFileManager) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"log_dir":    lfm.baseDir,
		"files":      make(map[string]int),
		"total_size": int64(0),
	}

	dirs := map[string]string{
		"requests": filepath.Join(lfm.baseDir, "requests"),
		"lock":     filepath.Join(lfm.baseDir, "events", "lock"),
		"tool":     filepath.Join(lfm.baseDir, "events", "tool"),
		"metrics":  filepath.Join(lfm.baseDir, "metrics"),
		"summary":  filepath.Join(lfm.baseDir, "summary"),
	}

	var totalSize int64
	filesCounts := make(map[string]int)
	var oldestDate, newestDate string

	for name, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			count++

			info, err := entry.Info()
			if err == nil {
				totalSize += info.Size()
			}

			// Track oldest/newest
			dateStr := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".jsonl"), ".json")
			if oldestDate == "" || dateStr < oldestDate {
				oldestDate = dateStr
			}
			if newestDate == "" || dateStr > newestDate {
				newestDate = dateStr
			}
		}
		filesCounts[name] = count
	}

	stats["files"] = filesCounts
	stats["total_size_mb"] = float64(totalSize) / 1024 / 1024
	stats["oldest_log"] = oldestDate
	stats["newest_log"] = newestDate

	return stats
}

// ListLogFiles returns a list of log files for a given type
func (lfm *LogFileManager) ListLogFiles(logType string) ([]string, error) {
	var dir string
	switch logType {
	case "requests":
		dir = filepath.Join(lfm.baseDir, "requests")
	case "lock_events":
		dir = filepath.Join(lfm.baseDir, "events", "lock")
	case "tool_events":
		dir = filepath.Join(lfm.baseDir, "events", "tool")
	case "metrics":
		dir = filepath.Join(lfm.baseDir, "metrics")
	case "summary":
		dir = filepath.Join(lfm.baseDir, "summary")
	default:
		return nil, fmt.Errorf("unknown log type: %s", logType)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	sort.Strings(files)
	return files, nil
}
