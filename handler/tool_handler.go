package handler

import (
	"errors"
	"net/http"
	"time"

	"clipboard-controller/config"
	"clipboard-controller/service"

	"github.com/gin-gonic/gin"
)

// RegisterToolHandler registers tool management endpoints
func RegisterToolHandler(router *gin.Engine, tr *service.ToolRegistry, cfg *config.Config) {
	tool := router.Group("/tool")
	{
		tool.POST("/register", registerTool(tr, cfg))
		tool.POST("/heartbeat", heartbeatTool(tr, cfg))
		tool.POST("/unregister", unregisterTool(tr))
		tool.GET("/status", getToolStatus(tr, cfg))
	}
}

// RegisterRequest represents the request body for tool registration
type RegisterRequest struct {
	ToolID string `json:"tool_id" binding:"required"`
}

// HeartbeatRequest represents the request body for heartbeat
type HeartbeatRequest struct {
	ToolID string `json:"tool_id" binding:"required"`
}

// UnregisterRequest represents the request body for unregister
type UnregisterRequest struct {
	ToolID string `json:"tool_id" binding:"required"`
}

func registerTool(tr *service.ToolRegistry, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "tool_id is required",
			})
			return
		}

		tool, err := tr.Register(req.ToolID)
		if err != nil {
			if errors.Is(err, service.ErrToolAlreadyRegistered) {
				c.JSON(http.StatusConflict, gin.H{
					"error":   "tool_already_registered",
					"message": "Tool ID đã được đăng ký và đang online",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"tool_id": tool.ToolID,
			"status":  "registered",
			"config":  cfg.GetClientConfig(),
		})
	}
}

func heartbeatTool(tr *service.ToolRegistry, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req HeartbeatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "tool_id is required",
			})
			return
		}

		tool, err := tr.Heartbeat(req.ToolID)
		if err != nil {
			if errors.Is(err, service.ErrToolNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "tool_not_found",
					"message": "Tool chưa được đăng ký",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": err.Error(),
			})
			return
		}

		deadline, _ := tr.GetHeartbeatDeadline(req.ToolID)

		c.JSON(http.StatusOK, gin.H{
			"status":                "ok",
			"next_heartbeat_before": deadline.Format(time.RFC3339),
		})

		// Set context for logging
		c.Set("tool_id", tool.ToolID)
	}
}

func unregisterTool(tr *service.ToolRegistry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req UnregisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "tool_id is required",
			})
			return
		}

		tool, err := tr.Unregister(req.ToolID)
		if err != nil {
			if errors.Is(err, service.ErrToolNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "tool_not_found",
					"message": "Tool chưa được đăng ký",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":           "unregistered",
			"released_tickets": []string{}, // Will be populated by lock manager cleanup
		})

		// Set context for logging
		c.Set("tool_id", tool.ToolID)
	}
}

func getToolStatus(tr *service.ToolRegistry, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		toolID := c.Query("tool_id")
		if toolID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "tool_id query parameter is required",
			})
			return
		}

		tool, err := tr.GetTool(toolID)
		if err != nil {
			if errors.Is(err, service.ErrToolNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "tool_not_found",
					"message": "Tool chưa được đăng ký",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": err.Error(),
			})
			return
		}

		deadline, _ := tr.GetHeartbeatDeadline(toolID)

		c.JSON(http.StatusOK, gin.H{
			"tool_id":                 tool.ToolID,
			"status":                  tool.Status,
			"registered_at":           tool.RegisteredAt.Format(time.RFC3339),
			"last_heartbeat":          tool.LastHeartbeat.Format(time.RFC3339),
			"next_heartbeat_deadline": deadline.Format(time.RFC3339),
		})
	}
}
