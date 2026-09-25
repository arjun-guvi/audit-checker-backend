package controllers

import (
	"log"
	"net/http"
	"strings"

	"auditApp/config"
	"auditApp/salesAudit/actions"
	"auditApp/salesAudit/models"

	"github.com/gin-gonic/gin"
)

// Auth stands in for Zen's auth middleware: it accepts any `Authorization: <token>` (no Bearer
// prefix) and puts `auth` (the user hash; here the token itself) and `program` into the context.
// Zen replaces it with the real middleware on merge.
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("Authorization"))
		if token == "" {
			respondError(c, http.StatusUnauthorized, "Missing Authorization header")
			c.Abort()
			return
		}
		c.Set("auth", token)
		c.Set("program", config.SalesAuditProgram)
		c.Next()
	}
}

// Member resolves the signed-in user to their salesAuditMembers row (role, region, manager).
func Member() gin.HandlerFunc {
	return func(c *gin.Context) {
		member, err := actions.ResolveMember(c, program(c), c.MustGet("auth").(string))
		if err != nil {
			fail(c, err)
			c.Abort()
			return
		}
		c.Set("member", member)
		c.Next()
	}
}

// RequirePermission declares the permission a route needs. The mock auth has no permission data,
// so it only records it; Zen's permission middleware enforces it on merge. Role checks are done
// in the actions.
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("permission", permission)
		c.Next()
	}
}

func program(c *gin.Context) string {
	return c.MustGet("program").(string)
}

func member(c *gin.Context) models.Member {
	return c.MustGet("member").(models.Member)
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

func respondError(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"status": "error", "message": message})
}

// fail answers with the error's status: an actions.Error as it is, anything else as a 500.
func fail(c *gin.Context, err error) {
	code, message := actions.StatusOf(err)
	if code == http.StatusInternalServerError {
		log.Printf("salesAudit: %s %s: %v", c.Request.Method, c.FullPath(), err)
	}
	respondError(c, code, message)
}

// reply sends data, or the error.
func reply(c *gin.Context, data any, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	respondOK(c, data)
}

// bind reads a JSON body, answering 400 when it can't.
func bind(c *gin.Context, body any) bool {
	if err := c.ShouldBindJSON(body); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request body")
		return false
	}
	return true
}
