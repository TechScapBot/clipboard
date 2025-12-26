package logger

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"clipboard-controller/model"
)

// EventLogger logs lock and tool events, and collects metrics
type EventLogger struct {
	fileManager   *LogFileManager
	logHeartbeats bool
	mu            sync.Mutex

	// Recent events buffer for debugging (circular buffer)
	recentEvents   []model.LockEventLog
	recentEventIdx int
	recentEventMu  sync.RWMutex

	// Metrics counters (reset every minute)
	locksGranted   atomic.Int64
	locksReleased  atomic.Int64
	locksExpired   atomic.Int64
	expiredTickets atomic.Int64
	failedRequests atomic.Int64

	// Aggregated metrics for averages
	totalWaitTime atomic.Int64
	totalHoldTime atomic.Int64
	waitCount     atomic.Int64
	holdCount     atomic.Int64

	// Max values tracking
	maxQueueLength atomic.Int32
	maxWaitTime    atomic.Int64
	maxHoldTime    atomic.Int64

	// Daily counters (reset at midnight)
	dailyRequests        atomic.Int64
	dailyLocksGranted    atomic.Int64
	dailyLocksReleased   atomic.Int64
	dailyLocksExpired    atomic.Int64
	dailyToolsRegistered atomic.Int64
	dailyErrors          atomic.Int64
	dailyTotalWaitTime   atomic.Int64
	dailyTotalHoldTime   atomic.Int64
	dailyWaitCount       atomic.Int64
	dailyHoldCount       atomic.Int64
	dailyMaxQueueLength  atomic.Int32
	dailyMaxWaitTime     atomic.Int64
	dailyMaxHoldTime     atomic.Int64

	// Per-tool usage tracking
	toolUsageMu sync.Mutex
	toolUsage   map[string]*toolStats
}

type toolStats struct {
	lockCount     int64
	totalWaitTime int64
	totalHoldTime int64
}

const recentEventsBufferSize = 100

// NewEventLogger creates a new event logger
func NewEventLogger(fm *LogFileManager) *EventLogger {
	return &EventLogger{
		fileManager:   fm,
		logHeartbeats: false, // Default: don't log heartbeats (too noisy)
		toolUsage:     make(map[string]*toolStats),
		recentEvents:  make([]model.LockEventLog, recentEventsBufferSize),
	}
}

// SetLogHeartbeats enables/disables heartbeat logging
func (el *EventLogger) SetLogHeartbeats(enabled bool) {
	el.logHeartbeats = enabled
}

// addToRecentEvents adds an event to the circular buffer for debugging
func (el *EventLogger) addToRecentEvents(event model.LockEventLog) {
	el.recentEventMu.Lock()
	defer el.recentEventMu.Unlock()

	el.recentEvents[el.recentEventIdx] = event
	el.recentEventIdx = (el.recentEventIdx + 1) % recentEventsBufferSize
}

// GetRecentEvents returns the most recent lock events for debugging
func (el *EventLogger) GetRecentEvents(limit int) []model.LockEventLog {
	el.recentEventMu.RLock()
	defer el.recentEventMu.RUnlock()

	if limit > recentEventsBufferSize {
		limit = recentEventsBufferSize
	}

	result := make([]model.LockEventLog, 0, limit)

	// Start from most recent and go backwards
	idx := (el.recentEventIdx - 1 + recentEventsBufferSize) % recentEventsBufferSize
	for i := 0; i < limit; i++ {
		event := el.recentEvents[idx]
		if event.Timestamp.IsZero() {
			break // Empty slot, buffer not full yet
		}
		result = append(result, event)
		idx = (idx - 1 + recentEventsBufferSize) % recentEventsBufferSize
	}

	return result
}

// LogLockRequestedWithContext logs a lock request event with request context
func (el *EventLogger) LogLockRequestedWithContext(requestID, ticketID, toolID, threadID string, queuePosition, queueLength int) {
	event := model.LockEventLog{
		Timestamp:     time.Now(),
		EventType:     model.LockEventRequested,
		RequestID:     requestID,
		TicketID:      ticketID,
		ToolID:        toolID,
		ThreadID:      threadID,
		QueuePosition: queuePosition,
		QueueLength:   queueLength,
	}

	el.updateMaxQueueLength(int32(queueLength))
	el.addToRecentEvents(event)
	go el.fileManager.WriteJSON("lock_events", event)
}

// LogLockRequested logs a lock request event (backward compatible)
func (el *EventLogger) LogLockRequested(ticketID, toolID, threadID string, queuePosition int) {
	el.LogLockRequestedWithContext("", ticketID, toolID, threadID, queuePosition, queuePosition)
}

// LogLockGrantedWithContext logs a lock granted event with request context
func (el *EventLogger) LogLockGrantedWithContext(requestID, ticketID, toolID, threadID string, waitDurationMs int64, queueLength int) {
	event := model.LockEventLog{
		Timestamp:      time.Now(),
		EventType:      model.LockEventGranted,
		RequestID:      requestID,
		TicketID:       ticketID,
		ToolID:         toolID,
		ThreadID:       threadID,
		WaitDurationMs: waitDurationMs,
		QueueLength:    queueLength,
	}

	el.locksGranted.Add(1)
	el.dailyLocksGranted.Add(1)
	el.totalWaitTime.Add(waitDurationMs)
	el.dailyTotalWaitTime.Add(waitDurationMs)
	el.waitCount.Add(1)
	el.dailyWaitCount.Add(1)
	el.updateMaxWaitTime(waitDurationMs)

	el.addToRecentEvents(event)
	go el.fileManager.WriteJSON("lock_events", event)
}

// LogLockGranted logs a lock granted event (backward compatible)
func (el *EventLogger) LogLockGranted(ticketID, toolID, threadID string, waitDurationMs int64) {
	el.LogLockGrantedWithContext("", ticketID, toolID, threadID, waitDurationMs, 0)
}

// LogLockReleasedWithContext logs a lock released event with request context
func (el *EventLogger) LogLockReleasedWithContext(requestID, ticketID, toolID, threadID string, holdDurationMs int64, queueLength int) {
	event := model.LockEventLog{
		Timestamp:      time.Now(),
		EventType:      model.LockEventReleased,
		RequestID:      requestID,
		TicketID:       ticketID,
		ToolID:         toolID,
		ThreadID:       threadID,
		HoldDurationMs: holdDurationMs,
		QueueLength:    queueLength,
	}

	el.locksReleased.Add(1)
	el.dailyLocksReleased.Add(1)
	el.totalHoldTime.Add(holdDurationMs)
	el.dailyTotalHoldTime.Add(holdDurationMs)
	el.holdCount.Add(1)
	el.dailyHoldCount.Add(1)
	el.updateMaxHoldTime(holdDurationMs)
	el.updateToolUsage(toolID, holdDurationMs, 0)

	el.addToRecentEvents(event)
	go el.fileManager.WriteJSON("lock_events", event)
}

// LogLockReleased logs a lock released event (backward compatible)
func (el *EventLogger) LogLockReleased(ticketID, toolID, threadID string, holdDurationMs int64) {
	el.LogLockReleasedWithContext("", ticketID, toolID, threadID, holdDurationMs, 0)
}

// LogLockExpired logs a lock expired event
func (el *EventLogger) LogLockExpired(ticketID, toolID, threadID, reason string, holdDurationMs int64) {
	event := model.LockEventLog{
		Timestamp:      time.Now(),
		EventType:      model.LockEventExpired,
		TicketID:       ticketID,
		ToolID:         toolID,
		ThreadID:       threadID,
		HoldDurationMs: holdDurationMs,
		Reason:         reason,
	}

	el.locksExpired.Add(1)
	el.dailyLocksExpired.Add(1)

	el.addToRecentEvents(event)
	go el.fileManager.WriteJSON("lock_events", event)
}

// LogLockExtended logs a lock extended event
func (el *EventLogger) LogLockExtended(ticketID, toolID, threadID string, extendCount int) {
	event := model.LockEventLog{
		Timestamp: time.Now(),
		EventType: model.LockEventExtended,
		TicketID:  ticketID,
		ToolID:    toolID,
		ThreadID:  threadID,
		Reason:    fmt.Sprintf("extend_%d", extendCount),
	}

	el.addToRecentEvents(event)
	go el.fileManager.WriteJSON("lock_events", event)
}

// LogTicketExpired logs a ticket TTL expiry
func (el *EventLogger) LogTicketExpired(ticketID, toolID, threadID, reason string) {
	event := model.LockEventLog{
		Timestamp: time.Now(),
		EventType: model.LockEventExpired,
		TicketID:  ticketID,
		ToolID:    toolID,
		ThreadID:  threadID,
		Reason:    reason,
	}

	el.expiredTickets.Add(1)

	el.addToRecentEvents(event)
	go el.fileManager.WriteJSON("lock_events", event)
}

// LogToolRegistered logs a tool registration event
func (el *EventLogger) LogToolRegistered(toolID string) {
	event := model.ToolEventLog{
		Timestamp: time.Now(),
		EventType: model.ToolEventRegistered,
		ToolID:    toolID,
	}

	el.dailyToolsRegistered.Add(1)

	go el.fileManager.WriteJSON("tool_events", event)
}

// LogToolHeartbeat logs a tool heartbeat event (controlled by logHeartbeats flag)
func (el *EventLogger) LogToolHeartbeat(toolID string) {
	// Skip if heartbeat logging is disabled (default)
	if !el.logHeartbeats {
		return
	}

	event := model.ToolEventLog{
		Timestamp: time.Now(),
		EventType: model.ToolEventHeartbeat,
		ToolID:    toolID,
	}

	go el.fileManager.WriteJSON("tool_events", event)
}

// LogToolOffline logs a tool going offline
func (el *EventLogger) LogToolOffline(toolID, reason string) {
	event := model.ToolEventLog{
		Timestamp: time.Now(),
		EventType: model.ToolEventOffline,
		ToolID:    toolID,
		Reason:    reason,
	}

	go el.fileManager.WriteJSON("tool_events", event)
}

// LogToolUnregistered logs a tool unregistration
func (el *EventLogger) LogToolUnregistered(toolID string) {
	event := model.ToolEventLog{
		Timestamp: time.Now(),
		EventType: model.ToolEventUnregistered,
		ToolID:    toolID,
	}

	go el.fileManager.WriteJSON("tool_events", event)
}

// IncrementRequests increments the daily request counter
func (el *EventLogger) IncrementRequests() {
	el.dailyRequests.Add(1)
}

// IncrementErrors increments the error counter
func (el *EventLogger) IncrementErrors() {
	el.failedRequests.Add(1)
	el.dailyErrors.Add(1)
}

// GetMinuteMetrics returns metrics for the last minute and resets counters
func (el *EventLogger) GetMinuteMetrics() model.SystemMetricsLog {
	avgWait := int64(0)
	avgHold := int64(0)

	waitCount := el.waitCount.Swap(0)
	holdCount := el.holdCount.Swap(0)
	totalWait := el.totalWaitTime.Swap(0)
	totalHold := el.totalHoldTime.Swap(0)

	if waitCount > 0 {
		avgWait = totalWait / waitCount
	}
	if holdCount > 0 {
		avgHold = totalHold / holdCount
	}

	metrics := model.SystemMetricsLog{
		Timestamp:           time.Now(),
		LocksGrantedLastMin: el.locksGranted.Swap(0),
		ExpiredTickets:      el.expiredTickets.Swap(0),
		FailedRequests:      el.failedRequests.Swap(0),
		AvgWaitTimeMs:       avgWait,
		AvgHoldTimeMs:       avgHold,
	}

	// Reset lock counters
	el.locksReleased.Store(0)
	el.locksExpired.Store(0)

	return metrics
}

// GetDailySummary returns the daily summary and resets counters
func (el *EventLogger) GetDailySummary() model.DailySummaryLog {
	avgWait := int64(0)
	avgHold := int64(0)

	waitCount := el.dailyWaitCount.Swap(0)
	holdCount := el.dailyHoldCount.Swap(0)
	totalWait := el.dailyTotalWaitTime.Swap(0)
	totalHold := el.dailyTotalHoldTime.Swap(0)

	if waitCount > 0 {
		avgWait = totalWait / waitCount
	}
	if holdCount > 0 {
		avgHold = totalHold / holdCount
	}

	summary := model.DailySummaryLog{
		Date:                 time.Now().Format("2006-01-02"),
		TotalRequests:        el.dailyRequests.Swap(0),
		TotalLocksGranted:    el.dailyLocksGranted.Swap(0),
		TotalLocksExpired:    el.dailyLocksExpired.Swap(0),
		TotalLocksReleased:   el.dailyLocksReleased.Swap(0),
		TotalToolsRegistered: el.dailyToolsRegistered.Swap(0),
		AvgWaitTimeMs:        avgWait,
		AvgHoldTimeMs:        avgHold,
		MaxQueueLength:       int(el.dailyMaxQueueLength.Swap(0)),
		MaxWaitTimeMs:        el.dailyMaxWaitTime.Swap(0),
		MaxHoldTimeMs:        el.dailyMaxHoldTime.Swap(0),
		ErrorCount:           el.dailyErrors.Swap(0),
		TopTools:             el.getTopTools(10),
	}

	// Reset tool usage
	el.toolUsageMu.Lock()
	el.toolUsage = make(map[string]*toolStats)
	el.toolUsageMu.Unlock()

	return summary
}

// Helper methods

func (el *EventLogger) updateMaxQueueLength(length int32) {
	for {
		old := el.maxQueueLength.Load()
		if length <= old || el.maxQueueLength.CompareAndSwap(old, length) {
			break
		}
	}
	for {
		old := el.dailyMaxQueueLength.Load()
		if length <= old || el.dailyMaxQueueLength.CompareAndSwap(old, length) {
			break
		}
	}
}

func (el *EventLogger) updateMaxWaitTime(ms int64) {
	for {
		old := el.maxWaitTime.Load()
		if ms <= old || el.maxWaitTime.CompareAndSwap(old, ms) {
			break
		}
	}
	for {
		old := el.dailyMaxWaitTime.Load()
		if ms <= old || el.dailyMaxWaitTime.CompareAndSwap(old, ms) {
			break
		}
	}
}

func (el *EventLogger) updateMaxHoldTime(ms int64) {
	for {
		old := el.maxHoldTime.Load()
		if ms <= old || el.maxHoldTime.CompareAndSwap(old, ms) {
			break
		}
	}
	for {
		old := el.dailyMaxHoldTime.Load()
		if ms <= old || el.dailyMaxHoldTime.CompareAndSwap(old, ms) {
			break
		}
	}
}

func (el *EventLogger) updateToolUsage(toolID string, holdTime, waitTime int64) {
	el.toolUsageMu.Lock()
	defer el.toolUsageMu.Unlock()

	stats, ok := el.toolUsage[toolID]
	if !ok {
		stats = &toolStats{}
		el.toolUsage[toolID] = stats
	}

	stats.lockCount++
	stats.totalHoldTime += holdTime
	stats.totalWaitTime += waitTime
}

func (el *EventLogger) getTopTools(limit int) []model.ToolUsage {
	el.toolUsageMu.Lock()
	defer el.toolUsageMu.Unlock()

	// Convert to slice for sorting
	type toolEntry struct {
		toolID string
		stats  *toolStats
	}

	entries := make([]toolEntry, 0, len(el.toolUsage))
	for id, stats := range el.toolUsage {
		entries = append(entries, toolEntry{id, stats})
	}

	// Sort by lock count descending
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].stats.lockCount > entries[i].stats.lockCount {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Take top N
	if len(entries) > limit {
		entries = entries[:limit]
	}

	// Convert to result
	result := make([]model.ToolUsage, len(entries))
	for i, e := range entries {
		avgWait := int64(0)
		avgHold := int64(0)
		if e.stats.lockCount > 0 {
			avgWait = e.stats.totalWaitTime / e.stats.lockCount
			avgHold = e.stats.totalHoldTime / e.stats.lockCount
		}
		result[i] = model.ToolUsage{
			ToolID:    e.toolID,
			LockCount: e.stats.lockCount,
			AvgWaitMs: avgWait,
			AvgHoldMs: avgHold,
		}
	}

	return result
}
