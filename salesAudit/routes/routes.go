package routes

import (
	"net/http"
	"strings"

	"auditApp/salesAudit/controllers"
	"auditApp/salesAudit/models"

	"github.com/gin-gonic/gin"
)

// MockAuth stands in for Zen's auth middleware: it accepts any `Authorization: <token>` (no
// Bearer prefix) and puts `auth` (the user hash; here the token itself) and `program` into the
// context. Zen replaces it with the real middleware on merge.
func MockAuth(program string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Missing Authorization header"})
			return
		}
		c.Set("auth", token)
		c.Set("program", program)
		c.Next()
	}
}

// RequirePermission declares the permission a route needs. The mock auth has no permission data,
// so it only records it; Zen's permission middleware enforces it on merge.
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("permission", permission)
		c.Next()
	}
}

// Register mounts every Sales Audit route under /sales-audit.
func Register(engine *gin.Engine, h *controllers.Handlers, program string) {
	group := engine.Group("/sales-audit", MockAuth(program))
	view := RequirePermission(models.PermissionView)
	edit := RequirePermission(models.PermissionEdit)

	group.GET("/leads", view, h.GetLeads)
	group.GET("/leads/summaries", view, h.GetLeadSummaries)
	group.POST("/leads/:leadId/send-reminder", edit, h.SendReminder)
	group.POST("/leads/:leadId/cc-response", edit, h.UpdateCcResponse)
	group.GET("/leads/:leadId/audit", view, h.GetLeadAudit)
	group.POST("/leads/:leadId/mark-audited", edit, h.MarkAudited)

	group.GET("/rechecks", view, h.GetRechecks)
	group.POST("/rechecks", edit, h.RaiseRecheck)
	group.POST("/rechecks/:recheckId/resolve", edit, h.ResolveRecheck)

	group.GET("/audit-history", view, h.GetAuditHistory)

	group.GET("/students/:studentId", view, h.GetStudent)
	group.GET("/students/:studentId/payments", view, h.GetStudentPayments)
	group.GET("/students/:studentId/cc-verification", view, h.GetCcVerification)
}
