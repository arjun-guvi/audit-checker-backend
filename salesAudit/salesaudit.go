// Package salesaudit wires the Sales Audit feature into the app: HTTP routes and Redis jobs.
package salesaudit

import (
	"time"

	"auditApp/config"
	"auditApp/salesAudit/controllers"
	"auditApp/salesAudit/routes"
	"auditApp/salesAudit/service"
	"auditApp/salesAudit/store"
	"auditApp/salesAudit/worker"

	"github.com/gin-gonic/gin"
	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
	"go.mongodb.org/mongo-driver/mongo"
)

// Settings read from the environment (listed in the README).
func program() string       { return config.GetEnv("SALES_AUDIT_PROGRAM", "guvi") }
func accountsEmail() string { return config.GetEnv("ACCOUNTS_EMAIL", "") }

func mailer() worker.SMTPMailer {
	return worker.SMTPMailer{
		Host:     config.GetEnv("SMTP_HOST", ""),
		Port:     config.GetEnv("SMTP_PORT", "587"),
		Username: config.GetEnv("SMTP_USERNAME", ""),
		Password: config.GetEnv("SMTP_PASSWORD", ""),
		From:     config.GetEnv("SMTP_FROM", ""),
	}
}

func newService(db *mongo.Database, redisPool *redis.Pool, namespace string) *service.Service {
	return &service.Service{
		Store:         store.NewMongo(db),
		Mail:          worker.NewQueue(namespace, redisPool),
		AccountsEmail: accountsEmail(),
		Now:           time.Now,
	}
}

// RegisterRoutes mounts /sales-audit on the engine. Mails are queued on `namespace`, which must
// match the worker pool's namespace.
func RegisterRoutes(engine *gin.Engine, db *mongo.Database, redisPool *redis.Pool, namespace string) {
	routes.Register(engine, controllers.New(newService(db, redisPool, namespace)), program())
}

// RegisterJobs adds the Sales Audit sweeps and mail job to the worker pool.
func RegisterJobs(pool *work.WorkerPool, db *mongo.Database, redisPool *redis.Pool, namespace string) {
	worker.Register(pool, newService(db, redisPool, namespace), program(), mailer())
}
