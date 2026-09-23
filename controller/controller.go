package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck returns the health status of the application
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Application is healthy",
	})
}
