package model

import (
	"time"
)

// ToolStatus represents the status of a tool
type ToolStatus string

const (
	ToolStatusOnline  ToolStatus = "online"
	ToolStatusOffline ToolStatus = "offline"
)

// Tool represents a registered automation tool (BAS, Go+Rod, etc.)
type Tool struct {
	ToolID        string     `json:"tool_id"`
	RegisteredAt  time.Time  `json:"registered_at"`
	LastHeartbeat time.Time  `json:"last_heartbeat"`
	Status        ToolStatus `json:"status"`
}

// NewTool creates a new Tool with online status
func NewTool(toolID string) *Tool {
	now := time.Now()
	return &Tool{
		ToolID:        toolID,
		RegisteredAt:  now,
		LastHeartbeat: now,
		Status:        ToolStatusOnline,
	}
}

// IsOnline returns true if the tool is online
func (t *Tool) IsOnline() bool {
	return t.Status == ToolStatusOnline
}

// UpdateHeartbeat updates the last heartbeat time
func (t *Tool) UpdateHeartbeat() {
	t.LastHeartbeat = time.Now()
	t.Status = ToolStatusOnline
}

// MarkOffline marks the tool as offline
func (t *Tool) MarkOffline() {
	t.Status = ToolStatusOffline
}

// IsHeartbeatExpired checks if the heartbeat has expired
func (t *Tool) IsHeartbeatExpired(timeout time.Duration) bool {
	return time.Since(t.LastHeartbeat) > timeout
}

// ToJSON returns a map representation for JSON response
func (t *Tool) ToJSON() map[string]interface{} {
	return map[string]interface{}{
		"tool_id":        t.ToolID,
		"registered_at":  t.RegisteredAt,
		"last_heartbeat": t.LastHeartbeat,
		"status":         t.Status,
	}
}
