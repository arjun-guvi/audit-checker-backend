package routes

import (
	"auditApp/controller"

	"github.com/gin-gonic/gin"
)

// SetupRoutes initializes all routes for the application
func SetupRoutes(router *gin.Engine) {
	// Health check
	router.GET("/health", controller.HealthCheck)

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
}
