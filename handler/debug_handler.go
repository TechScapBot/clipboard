package handler

import (
	"net/http"
	"strconv"

	"clipboard-controller/logger"

	"github.com/gin-gonic/gin"
)

// DebugHandler handles debug and logging endpoints
type DebugHandler struct {
	eventLogger    *logger.EventLogger
	logFileManager *logger.LogFileManager
}

// RegisterDebugHandler registers debug endpoints
func RegisterDebugHandler(router *gin.Engine, el *logger.EventLogger, lfm *logger.LogFileManager) {
	h := &DebugHandler{
		eventLogger:    el,
		logFileManager: lfm,
	}

	debug := router.Group("/debug")
	{
		debug.GET("/logs/recent", h.GetRecentLogs)
		debug.GET("/logs/stats", h.GetLogStats)
	}
}

// GetRecentLogs returns recent lock events from memory buffer
// GET /debug/logs/recent?limit=50
func (h *DebugHandler) GetRecentLogs(c *gin.Context) {
	limit := 50 // default
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	events := h.eventLogger.GetRecentEvents(limit)

	c.JSON(http.StatusOK, gin.H{
		"count":  len(events),
		"events": events,
	})
}

// GetLogStats returns log file statistics
// GET /debug/logs/stats
func (h *DebugHandler) GetLogStats(c *gin.Context) {
	stats := h.logFileManager.GetStats()
	c.JSON(http.StatusOK, stats)
}
