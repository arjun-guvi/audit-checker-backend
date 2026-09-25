package routes

import (
	"auditApp/controller"
	salesAuditRoutes "auditApp/salesAudit/routes"
	salesAuditWorker "auditApp/salesAudit/worker"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
)

// SetupRoutes mounts the health check and the Sales Audit feature. The feature's mails are queued
// for the worker on workerNamespace.
func SetupRoutes(router *gin.Engine, redisPool *redis.Pool, workerNamespace string) {
	router.GET("/health", controller.HealthCheck)

	salesAuditWorker.SetupQueue(redisPool, workerNamespace)
	salesAuditRoutes.Register(router)
}
