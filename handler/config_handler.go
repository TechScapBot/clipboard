package handler

import (
	"net/http"

	"clipboard-controller/config"

	"github.com/gin-gonic/gin"
)

// RegisterConfigHandler registers config endpoints
func RegisterConfigHandler(router *gin.Engine, cfg *config.Config) {
	router.GET("/config", getConfig(cfg))
	router.PATCH("/config", updateConfig(cfg))
}

func getConfig(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, cfg.ToMap())
	}
}

func updateConfig(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var updates map[string]interface{}
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "Invalid JSON body",
			})
			return
		}

		// Convert float64 to int for numeric values (JSON unmarshals numbers as float64)
		for key, value := range updates {
			if floatVal, ok := value.(float64); ok {
				updates[key] = int(floatVal)
			}
		}

		// Update config
		cfg.Update(updates)

		// Validate after update
		if err := cfg.Validate(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_config",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "updated",
			"config": cfg.ToMap(),
		})
	}
}
