package logger

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// BackgroundJobs manages logging-related background tasks
type BackgroundJobs struct {
	fileManager *LogFileManager
	eventLogger *EventLogger
	getMetrics  func() (activeTools int, queueLength int, currentHolder string)
	stopChan    chan struct{}
}

// NewBackgroundJobs creates a new background jobs manager
func NewBackgroundJobs(
	fm *LogFileManager,
	el *EventLogger,
	metricsProvider func() (activeTools int, queueLength int, currentHolder string),
) *BackgroundJobs {
	return &BackgroundJobs{
		fileManager: fm,
		eventLogger: el,
		getMetrics:  metricsProvider,
		stopChan:    make(chan struct{}),
	}
}

// Start starts all background jobs
func (bj *BackgroundJobs) Start(ctx context.Context) {
	log.Info().Msg("Starting logging background jobs")

	// Metrics collection every minute
	go bj.metricsCollector(ctx)

	// Daily summary at midnight
	go bj.dailySummaryGenerator(ctx)

	// Log cleanup every hour
	go bj.logCleanup(ctx)
}

// Stop stops all background jobs
func (bj *BackgroundJobs) Stop() {
	close(bj.stopChan)
}

// metricsCollector collects and logs metrics every minute
func (bj *BackgroundJobs) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-bj.stopChan:
			return
		case <-ticker.C:
			bj.collectMetrics()
		}
	}
}

func (bj *BackgroundJobs) collectMetrics() {
	// Get minute metrics from event logger
	metrics := bj.eventLogger.GetMinuteMetrics()

	// Add current state from provider
	if bj.getMetrics != nil {
		activeTools, queueLength, currentHolder := bj.getMetrics()
		metrics.ActiveTools = activeTools
		metrics.QueueLength = queueLength
		metrics.CurrentLockHolder = currentHolder
	}

	// Write to file
	if err := bj.fileManager.WriteJSON("metrics", metrics); err != nil {
		log.Warn().Err(err).Msg("Failed to write metrics log")
	}

	log.Debug().
		Int("active_tools", metrics.ActiveTools).
		Int("queue_length", metrics.QueueLength).
		Int64("locks_granted", metrics.LocksGrantedLastMin).
		Msg("Metrics collected")
}

// dailySummaryGenerator generates daily summary at midnight
func (bj *BackgroundJobs) dailySummaryGenerator(ctx context.Context) {
	for {
		// Calculate time until next midnight
		now := time.Now()
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		durationUntilMidnight := nextMidnight.Sub(now)

		select {
		case <-ctx.Done():
			return
		case <-bj.stopChan:
			return
		case <-time.After(durationUntilMidnight):
			bj.generateDailySummary()
		}
	}
}

func (bj *BackgroundJobs) generateDailySummary() {
	// Get yesterday's date for the summary
	yesterday := time.Now().Add(-1 * time.Second).Format("2006-01-02")

	summary := bj.eventLogger.GetDailySummary()
	summary.Date = yesterday

	if err := bj.fileManager.WriteSummary(summary); err != nil {
		log.Error().Err(err).Msg("Failed to write daily summary")
	} else {
		log.Info().
			Str("date", yesterday).
			Int64("total_requests", summary.TotalRequests).
			Int64("locks_granted", summary.TotalLocksGranted).
			Msg("Daily summary generated")
	}
}

// logCleanup removes old log files every hour
func (bj *BackgroundJobs) logCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run once at startup
	bj.fileManager.CleanupOldLogs()

	for {
		select {
		case <-ctx.Done():
			return
		case <-bj.stopChan:
			return
		case <-ticker.C:
			if removed, err := bj.fileManager.CleanupOldLogs(); err != nil {
				log.Warn().Err(err).Msg("Log cleanup encountered errors")
			} else if removed > 0 {
				log.Info().Int("removed", removed).Msg("Old log files cleaned up")
			}
		}
	}
}

// ForceGenerateSummary forces generation of daily summary (for testing)
func (bj *BackgroundJobs) ForceGenerateSummary() {
	summary := bj.eventLogger.GetDailySummary()
	if err := bj.fileManager.WriteSummary(summary); err != nil {
		log.Error().Err(err).Msg("Failed to write daily summary")
	}
}

// ForceCollectMetrics forces metrics collection (for testing)
func (bj *BackgroundJobs) ForceCollectMetrics() {
	bj.collectMetrics()
}
