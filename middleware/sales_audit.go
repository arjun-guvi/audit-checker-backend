package middleware

import (
	"net/http"
	"strings"

	"auditApp/config"

	"github.com/gin-gonic/gin"
)

// SalesAuditAuth stands in for Zen's auth middleware: it accepts any `Authorization: <token>` (no
// Bearer prefix) and puts `auth` (the user hash; here the token itself) and `program` into the
// context. Zen replaces it with the real middleware on merge.
func SalesAuditAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Missing Authorization header"})
			return
		}
		c.Set("auth", token)
		c.Set("program", config.SalesAuditProgram)
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
