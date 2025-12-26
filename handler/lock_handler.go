package handler

import (
	"errors"
	"net/http"
	"time"

	"clipboard-controller/config"
	"clipboard-controller/service"

	"github.com/gin-gonic/gin"
)

// RegisterLockHandler registers lock management endpoints
func RegisterLockHandler(router *gin.Engine, lm *service.LockManager, cfg *config.Config) {
	lock := router.Group("/lock")
	{
		lock.POST("/request", requestLock(lm, cfg))
		lock.GET("/check", checkLock(lm, cfg))
		lock.POST("/release", releaseLock(lm))
		lock.POST("/extend", extendLock(lm, cfg))
		lock.GET("/status", getLockStatus(lm))
	}
}

// LockRequest represents the request body for lock request
type LockRequest struct {
	ToolID   string `json:"tool_id" binding:"required"`
	ThreadID string `json:"thread_id" binding:"required"`
}

// ReleaseRequest represents the request body for lock release
type ReleaseRequest struct {
	TicketID string `json:"ticket_id" binding:"required"`
}

// ExtendRequest represents the request body for lock extend
type ExtendRequest struct {
	TicketID string `json:"ticket_id" binding:"required"`
}

func requestLock(lm *service.LockManager, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LockRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "tool_id and thread_id are required",
			})
			return
		}

		ticket, position, err := lm.RequestLock(req.ToolID, req.ThreadID)
		if err != nil {
			if errors.Is(err, service.ErrToolOffline) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "tool_offline",
					"message": "Tool không online, cần register hoặc heartbeat",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": err.Error(),
			})
			return
		}

		// Calculate ticket expiry
		ticketExpiresAt := ticket.RequestedAt.Add(time.Duration(cfg.TicketTTL) * time.Second)

		response := gin.H{
			"ticket_id":         ticket.TicketID,
			"position":          position,
			"poll_interval":     cfg.PollInterval,
			"ticket_expires_at": ticketExpiresAt.Format(time.RFC3339),
		}

		// If ticket was already granted (position 0)
		if position == 0 {
			response["status"] = "granted"
			response["expires_at"] = ticket.ExpiresAt.Format(time.RFC3339)
			response["lock_duration_ms"] = ticket.RemainingTime().Milliseconds()
		}

		c.JSON(http.StatusOK, response)

		// Set context for logging
		c.Set("tool_id", req.ToolID)
		c.Set("thread_id", req.ThreadID)
		c.Set("ticket_id", ticket.TicketID)
	}
}

func checkLock(lm *service.LockManager, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ticketID := c.Query("ticket_id")
		if ticketID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "ticket_id query parameter is required",
			})
			return
		}

		ticket, position, err := lm.CheckLock(ticketID)
		if err != nil {
			if errors.Is(err, service.ErrTicketNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "ticket_not_found",
					"message": "Ticket không tồn tại hoặc đã bị xóa",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": err.Error(),
			})
			return
		}

		response := gin.H{
			"status": string(ticket.Status),
		}

		switch ticket.Status {
		case "waiting":
			response["position"] = position
			response["estimated_wait_ms"] = lm.EstimateWaitTime(position).Milliseconds()

		case "granted":
			response["expires_at"] = ticket.ExpiresAt.Format(time.RFC3339)
			response["lock_duration_ms"] = ticket.RemainingTime().Milliseconds()

		case "expired":
			response["reason"] = getExpireReason(ticket)
		}

		c.JSON(http.StatusOK, response)

		// Set context for logging
		c.Set("ticket_id", ticketID)
		c.Set("tool_id", ticket.ToolID)
		c.Set("thread_id", ticket.ThreadID)
	}
}

func releaseLock(lm *service.LockManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ReleaseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "ticket_id is required",
			})
			return
		}

		ticket, err := lm.ReleaseLock(req.TicketID)
		if err != nil {
			if errors.Is(err, service.ErrTicketNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "ticket_not_found",
					"message": "Ticket không tồn tại hoặc đã bị xóa",
				})
				return
			}
			if errors.Is(err, service.ErrNotLockHolder) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "not_lock_holder",
					"message": "Ticket này không đang giữ lock",
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
			"status":           "released",
			"held_duration_ms": ticket.HoldDuration().Milliseconds(),
		})

		// Set context for logging
		c.Set("ticket_id", req.TicketID)
		c.Set("tool_id", ticket.ToolID)
		c.Set("thread_id", ticket.ThreadID)
	}
}

func extendLock(lm *service.LockManager, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ExtendRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "ticket_id is required",
			})
			return
		}

		ticket, err := lm.ExtendLock(req.TicketID)
		if err != nil {
			if errors.Is(err, service.ErrExtendDisabled) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "extend_disabled",
					"message": "Lock extend không được bật trong config",
				})
				return
			}
			if errors.Is(err, service.ErrTicketNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "ticket_not_found",
					"message": "Ticket không tồn tại hoặc đã bị xóa",
				})
				return
			}
			if errors.Is(err, service.ErrNotLockHolder) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "not_lock_holder",
					"message": "Ticket này không đang giữ lock",
				})
				return
			}
			if errors.Is(err, service.ErrMaxExtendReached) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "max_extend_reached",
					"message": "Đã extend tối đa số lần cho phép",
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
			"status":           "extended",
			"new_expires_at":   ticket.ExpiresAt.Format(time.RFC3339),
			"extend_count":     ticket.ExtendCount,
			"extend_remaining": cfg.LockExtendMax - ticket.ExtendCount,
		})

		// Set context for logging
		c.Set("ticket_id", req.TicketID)
		c.Set("tool_id", ticket.ToolID)
		c.Set("thread_id", ticket.ThreadID)
	}
}

func getLockStatus(lm *service.LockManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := lm.GetQueueStatus()
		c.JSON(http.StatusOK, status)
	}
}

// getExpireReason determines why a ticket expired
func getExpireReason(ticket interface{}) string {
	// This is a simplified version - in reality, we'd track the reason
	return "ticket_ttl_exceeded"
}
