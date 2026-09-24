package routes

import (
	"auditApp/config"
	"auditApp/controller"
	"auditApp/middleware"
	"auditApp/models"
	"auditApp/worker"

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

	// Sales Audit routes; mails are queued for the worker on workerNamespace
	worker.SetupSalesAuditQueue(redisPool, workerNamespace)
	view := middleware.RequirePermission(models.PermissionView)
	edit := middleware.RequirePermission(models.PermissionEdit)
	salesAudit := router.Group("/sales-audit", middleware.SalesAuditAuth())
	{
		salesAudit.GET("/me", controller.GetCurrentUser)

		salesAudit.GET("/leads", view, controller.GetLeads)
		salesAudit.GET("/leads/summaries", view, controller.GetLeadSummaries)
		salesAudit.POST("/leads/:leadId/send-reminder", edit, controller.SendReminder)
		salesAudit.POST("/leads/:leadId/cc-response", edit, controller.UpdateCcResponse)
		salesAudit.GET("/leads/:leadId/audit", view, controller.GetLeadAudit)
		salesAudit.POST("/leads/:leadId/mark-audited", edit, controller.MarkAudited)

		salesAudit.GET("/rechecks", view, controller.GetRechecks)
		salesAudit.POST("/rechecks", edit, controller.RaiseRecheck)
		salesAudit.POST("/rechecks/:recheckId/resolve", edit, controller.ResolveRecheck)

		salesAudit.GET("/audit-history", view, controller.GetAuditHistory)

		// Runs the payment verification mail sweep now (the worker runs it every 10 minutes)
		salesAudit.POST("/payment-verification/run-sweep", edit, controller.RunPaymentVerificationSweep)

		// Sends one mail straight over SMTP to check the mail settings
		salesAudit.POST("/test-mail", edit, controller.SendTestMail)

		salesAudit.GET("/students/:studentId", view, controller.GetStudent)
		salesAudit.GET("/students/:studentId/payments", view, controller.GetStudentPayments)
		salesAudit.GET("/students/:studentId/cc-verification", view, controller.GetCcVerification)
	}
}
