package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"clipboard-controller/logger"
	"clipboard-controller/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestLogger creates a middleware that logs all HTTP requests
func RequestLogger(lfm *logger.LogFileManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate request ID
		requestID := uuid.New().String()
		c.Set("request_id", requestID)

		// Record start time
		startTime := time.Now()

		// Read request body for extracting IDs
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Extract tool_id, thread_id, ticket_id from body or query
		toolID := extractID(bodyBytes, c, "tool_id")
		threadID := extractID(bodyBytes, c, "thread_id")
		ticketID := extractID(bodyBytes, c, "ticket_id")

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(startTime)

		// Get error if any
		var errorMsg string
		if len(c.Errors) > 0 {
			errorMsg = c.Errors.String()
		}

		// Also check context for IDs set by handlers
		if tid, exists := c.Get("tool_id"); exists && toolID == "" {
			toolID = tid.(string)
		}
		if tid, exists := c.Get("thread_id"); exists && threadID == "" {
			threadID = tid.(string)
		}
		if tid, exists := c.Get("ticket_id"); exists && ticketID == "" {
			ticketID = tid.(string)
		}

		// Create log entry
		logEntry := model.RequestLog{
			Timestamp:  startTime,
			RequestID:  requestID,
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			ToolID:     toolID,
			ThreadID:   threadID,
			TicketID:   ticketID,
			StatusCode: c.Writer.Status(),
			DurationMs: duration.Milliseconds(),
			ClientIP:   c.ClientIP(),
			Error:      errorMsg,
		}

		// Write to log file asynchronously
		go func() {
			if err := lfm.WriteJSON("requests", logEntry); err != nil {
				// Don't use zerolog here to avoid circular logging
				// Just silently ignore errors
			}
		}()
	}
}

// extractID extracts an ID from request body or query parameters
func extractID(body []byte, c *gin.Context, key string) string {
	// Try query parameter first
	if val := c.Query(key); val != "" {
		return val
	}

	// Try to parse body as JSON
	if len(body) > 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err == nil {
			if val, ok := data[key].(string); ok {
				return val
			}
		}
	}

	return ""
}
