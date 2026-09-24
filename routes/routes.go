package routes

import (
	"auditApp/config"
	"auditApp/controller"
	"auditApp/middleware"
	salesaudit "auditApp/salesAudit"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
)

// SetupRoutes initializes all routes for the application
func SetupRoutes(router *gin.Engine, redisPool *redis.Pool, workerNamespace string) {
	// Initialize controllers
	authController := controller.NewAuthController(config.MongoDB)
	// Health check
	router.GET("/health", controller.HealthCheck)

	// Authentication routes
	router.POST("/register", authController.Register)
	router.POST("/login", authController.Login)
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware(config.MongoDB))
	{
		protected.GET("/me", authController.Me)
	}

	// SAP routes
	sap := router.Group("/sap")
	{
		sap.GET("/list", controller.SAPList)
		sap.GET("/:id", controller.SAPGetByID)
		sap.POST("/create", controller.SAPCreate)
		sap.PUT("/edit/:id", controller.SAPEdit)
		sap.DELETE("/delete/:id", controller.SAPDelete)

		// Recipient management
		sap.POST("/:id/recipients", controller.SAPAddRecipients)

		// Reminder management
		sap.POST("/:id/trigger-reminder", controller.SAPTriggerReminder)
		sap.POST("/:id/activate", controller.SAPActivate)
		sap.POST("/:id/deactivate", controller.SAPDeactivate)
	}

	// Sales Audit feature (/sales-audit/...)
	salesaudit.RegisterRoutes(router, config.MongoDB, redisPool, workerNamespace)
}
