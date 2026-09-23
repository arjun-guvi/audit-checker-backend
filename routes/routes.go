package routes

import (
	"github.com/gin-gonic/gin"
	"auditApp/controller"
)

// SetupRoutes initializes all routes for the application
func SetupRoutes(router *gin.Engine) {
	router.GET("/health", controller.HealthCheck)
}
