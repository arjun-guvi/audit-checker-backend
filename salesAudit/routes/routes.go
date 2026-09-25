// Package routes mounts the Sales Audit API under /sales-audit.
package routes

import (
	"auditApp/salesAudit/controllers"
	"auditApp/salesAudit/models"

	"github.com/gin-gonic/gin"
)

// Register adds every /sales-audit route. Each declares salesAudit.view or salesAudit.edit.
func Register(engine *gin.Engine) {
	view := controllers.RequirePermission(models.PermissionView)
	edit := controllers.RequirePermission(models.PermissionEdit)
	group := engine.Group("/sales-audit", controllers.Auth(), controllers.Member())

	group.GET("/me", view, controllers.GetMe)
	group.GET("/members", view, controllers.GetMembers)
	group.POST("/members", edit, controllers.CreateMember)
	group.PUT("/members/:memberId", edit, controllers.UpdateMember)

	group.GET("/leads", view, controllers.GetLeads)
	group.POST("/leads/assign", edit, controllers.AssignLeads)
	group.GET("/leads/:leadId", view, controllers.GetLead)
	group.GET("/leads/:leadId/timeline", view, controllers.GetTimeline)
	group.GET("/leads/:leadId/audit", view, controllers.GetAuditView)
	group.GET("/leads/:leadId/cc-verification", view, controllers.GetCcVerification)
	group.GET("/leads/:leadId/alerts", view, controllers.GetLeadAlerts)
	group.POST("/leads/:leadId/complete-audit", edit, controllers.CompleteAudit)
	group.POST("/leads/:leadId/reassign", edit, controllers.ReassignLead)
	group.POST("/leads/:leadId/take-up", edit, controllers.TakeUpLead)
	group.POST("/leads/:leadId/send-reminder", edit, controllers.SendReminder)

	group.GET("/rechecks", view, controllers.GetRechecks)
	group.POST("/rechecks", edit, controllers.RaiseRecheck)
	group.GET("/rechecks/cc-status", view, controllers.GetCcStatus)
	group.POST("/rechecks/:recheckId/close", edit, controllers.CloseRecheck)

	group.GET("/dashboard/auditor-team", view, controllers.GetTeamDashboard)
	group.GET("/dashboard/auditor-team/summary", view, controllers.GetTeamDashboardSummary)
	group.GET("/dashboard/bda", view, controllers.GetBdaDashboard)
	group.GET("/dashboard/bda/summary", view, controllers.GetBdaDashboardSummary)

	group.GET("/notifications", view, controllers.GetNotifications)
	group.POST("/notifications/read-all", edit, controllers.ReadAllNotifications)
	group.POST("/notifications/:notificationId/read", edit, controllers.ReadNotification)

	group.GET("/alerts", view, controllers.GetAlerts)
	group.POST("/zoho/import", edit, controllers.ImportZoho)
	group.POST("/payment-verification/run-sweep", edit, controllers.RunPaymentVerificationSweep)
	group.POST("/test-mail", edit, controllers.SendTestMail)
}
