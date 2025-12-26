package model

import "time"

// RequestLog records every HTTP request
type RequestLog struct {
	Timestamp  time.Time `json:"timestamp"`
	RequestID  string    `json:"request_id"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	ToolID     string    `json:"tool_id,omitempty"`
	ThreadID   string    `json:"thread_id,omitempty"`
	TicketID   string    `json:"ticket_id,omitempty"`
	StatusCode int       `json:"status_code"`
	DurationMs int64     `json:"duration_ms"`
	ClientIP   string    `json:"client_ip"`
	Error      string    `json:"error,omitempty"`
}

// LockEventLog records lock lifecycle events
type LockEventLog struct {
	Timestamp      time.Time `json:"timestamp"`
	EventType      string    `json:"event_type"` // lock_requested, lock_granted, lock_released, lock_expired, lock_extended
	RequestID      string    `json:"request_id,omitempty"` // Correlation with HTTP request
	TicketID       string    `json:"ticket_id"`
	ToolID         string    `json:"tool_id"`
	ThreadID       string    `json:"thread_id"`
	QueuePosition  int       `json:"queue_position,omitempty"`
	QueueLength    int       `json:"queue_length,omitempty"` // Current queue state
	WaitDurationMs int64     `json:"wait_duration_ms,omitempty"`
	HoldDurationMs int64     `json:"hold_duration_ms,omitempty"`
	Reason         string    `json:"reason,omitempty"`
}

// Lock event types
const (
	LockEventRequested = "lock_requested"
	LockEventGranted   = "lock_granted"
	LockEventReleased  = "lock_released"
	LockEventExpired   = "lock_expired"
	LockEventExtended  = "lock_extended"
)

// ToolEventLog records tool lifecycle events
type ToolEventLog struct {
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"` // tool_registered, tool_heartbeat, tool_offline, tool_unregistered
	ToolID    string    `json:"tool_id"`
	Reason    string    `json:"reason,omitempty"`
}

// Tool event types
const (
	ToolEventRegistered   = "tool_registered"
	ToolEventHeartbeat    = "tool_heartbeat"
	ToolEventOffline      = "tool_offline"
	ToolEventUnregistered = "tool_unregistered"
)

// SystemMetricsLog records system metrics at regular intervals
type SystemMetricsLog struct {
	Timestamp           time.Time `json:"timestamp"`
	ActiveTools         int       `json:"active_tools"`
	QueueLength         int       `json:"queue_length"`
	CurrentLockHolder   string    `json:"current_lock_holder,omitempty"`
	LocksGrantedLastMin int64     `json:"locks_granted_last_min"`
	AvgWaitTimeMs       int64     `json:"avg_wait_time_ms"`
	AvgHoldTimeMs       int64     `json:"avg_hold_time_ms"`
	ExpiredTickets      int64     `json:"expired_tickets_last_min"`
	FailedRequests      int64     `json:"failed_requests_last_min"`
}

// DailySummaryLog records daily statistics
type DailySummaryLog struct {
	Date                 string      `json:"date"` // YYYY-MM-DD
	TotalRequests        int64       `json:"total_requests"`
	TotalLocksGranted    int64       `json:"total_locks_granted"`
	TotalLocksExpired    int64       `json:"total_locks_expired"`
	TotalLocksReleased   int64       `json:"total_locks_released"`
	TotalToolsRegistered int64       `json:"total_tools_registered"`
	AvgWaitTimeMs        int64       `json:"avg_wait_time_ms"`
	AvgHoldTimeMs        int64       `json:"avg_hold_time_ms"`
	MaxQueueLength       int         `json:"max_queue_length"`
	MaxWaitTimeMs        int64       `json:"max_wait_time_ms"`
	MaxHoldTimeMs        int64       `json:"max_hold_time_ms"`
	ErrorCount           int64       `json:"error_count"`
	TopTools             []ToolUsage `json:"top_tools"`
}

// ToolUsage records usage statistics for a single tool
type ToolUsage struct {
	ToolID    string `json:"tool_id"`
	LockCount int64  `json:"lock_count"`
	AvgWaitMs int64  `json:"avg_wait_ms"`
	AvgHoldMs int64  `json:"avg_hold_ms"`
}
