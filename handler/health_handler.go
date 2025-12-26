package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterHealthHandler registers health check endpoints
func RegisterHealthHandler(router *gin.Engine, version string, startTime *time.Time) {
	router.GET("/health", func(c *gin.Context) {
		uptime := time.Since(*startTime).Seconds()

		c.JSON(http.StatusOK, gin.H{
			"status":         "healthy",
			"uptime_seconds": int64(uptime),
			"version":        version,
		})
	})
}
