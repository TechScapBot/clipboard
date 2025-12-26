package service

import (
	"errors"
	"sync"
	"time"

	"clipboard-controller/config"
	"clipboard-controller/model"

	"github.com/rs/zerolog/log"
)

var (
	ErrToolAlreadyRegistered = errors.New("tool already registered and online")
	ErrToolNotFound          = errors.New("tool not found")
	ErrToolOffline           = errors.New("tool is offline")
)

// ToolRegistry manages registered tools
type ToolRegistry struct {
	mu          sync.RWMutex
	tools       map[string]*model.Tool
	config      *config.Config
	eventLogger EventLogger
}

// NewToolRegistry creates a new ToolRegistry
func NewToolRegistry(cfg *config.Config) *ToolRegistry {
	return &ToolRegistry{
		tools:  make(map[string]*model.Tool),
		config: cfg,
	}
}

// SetEventLogger sets the event logger for tool events
func (tr *ToolRegistry) SetEventLogger(el EventLogger) {
	tr.eventLogger = el
}

// Register registers a new tool or reactivates an offline one
func (tr *ToolRegistry) Register(toolID string) (*model.Tool, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if existing, ok := tr.tools[toolID]; ok {
		if existing.IsOnline() {
			log.Debug().
				Str("tool_id", toolID).
				Msg("Tool already registered and online")
			return nil, ErrToolAlreadyRegistered
		}

		// Reactivate offline tool
		existing.UpdateHeartbeat()
		log.Info().
			Str("tool_id", toolID).
			Msg("Tool reactivated")

		// Log event
		if tr.eventLogger != nil {
			tr.eventLogger.LogToolRegistered(toolID)
		}

		return existing, nil
	}

	// Create new tool
	tool := model.NewTool(toolID)
	tr.tools[toolID] = tool

	log.Info().
		Str("tool_id", toolID).
		Msg("Tool registered")

	// Log event
	if tr.eventLogger != nil {
		tr.eventLogger.LogToolRegistered(toolID)
	}

	return tool, nil
}

// Heartbeat updates the heartbeat for a tool
func (tr *ToolRegistry) Heartbeat(toolID string) (*model.Tool, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tool, ok := tr.tools[toolID]
	if !ok {
		log.Debug().
			Str("tool_id", toolID).
			Msg("Heartbeat for unknown tool")
		return nil, ErrToolNotFound
	}

	tool.UpdateHeartbeat()

	log.Debug().
		Str("tool_id", toolID).
		Time("last_heartbeat", tool.LastHeartbeat).
		Msg("Heartbeat updated")

	// Log event (heartbeats are frequent, only log to file)
	if tr.eventLogger != nil {
		tr.eventLogger.LogToolHeartbeat(toolID)
	}

	return tool, nil
}

// Unregister removes a tool from the registry
func (tr *ToolRegistry) Unregister(toolID string) (*model.Tool, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tool, ok := tr.tools[toolID]
	if !ok {
		return nil, ErrToolNotFound
	}

	tool.MarkOffline()

	log.Info().
		Str("tool_id", toolID).
		Msg("Tool unregistered")

	// Log event
	if tr.eventLogger != nil {
		tr.eventLogger.LogToolUnregistered(toolID)
	}

	return tool, nil
}

// GetTool returns a tool by ID
func (tr *ToolRegistry) GetTool(toolID string) (*model.Tool, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tool, ok := tr.tools[toolID]
	if !ok {
		return nil, ErrToolNotFound
	}

	return tool, nil
}

// IsOnline checks if a tool is online
func (tr *ToolRegistry) IsOnline(toolID string) bool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tool, ok := tr.tools[toolID]
	if !ok {
		return false
	}

	return tool.IsOnline()
}

// CheckAndMarkOffline checks all tools and marks expired ones as offline
// Returns list of tool IDs that were marked offline
func (tr *ToolRegistry) CheckAndMarkOffline() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	timeout := time.Duration(tr.config.HeartbeatTimeout) * time.Second
	offlineTools := make([]string, 0)

	for toolID, tool := range tr.tools {
		if tool.IsOnline() && tool.IsHeartbeatExpired(timeout) {
			tool.MarkOffline()
			offlineTools = append(offlineTools, toolID)

			log.Warn().
				Str("tool_id", toolID).
				Time("last_heartbeat", tool.LastHeartbeat).
				Dur("timeout", timeout).
				Msg("Tool marked offline due to heartbeat timeout")

			// Log event
			if tr.eventLogger != nil {
				tr.eventLogger.LogToolOffline(toolID, "heartbeat_timeout")
			}
		}
	}

	return offlineTools
}

// CountOnlineTools returns the number of online tools
func (tr *ToolRegistry) CountOnlineTools() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	count := 0
	for _, tool := range tr.tools {
		if tool.IsOnline() {
			count++
		}
	}

	return count
}

// GetAllTools returns all tools (for debug/status)
func (tr *ToolRegistry) GetAllTools() []*model.Tool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	result := make([]*model.Tool, 0, len(tr.tools))
	for _, tool := range tr.tools {
		result = append(result, tool)
	}

	return result
}

// GetHeartbeatDeadline returns the next heartbeat deadline for a tool
func (tr *ToolRegistry) GetHeartbeatDeadline(toolID string) (time.Time, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tool, ok := tr.tools[toolID]
	if !ok {
		return time.Time{}, ErrToolNotFound
	}

	timeout := time.Duration(tr.config.HeartbeatTimeout) * time.Second
	return tool.LastHeartbeat.Add(timeout), nil
}
