package worker

import (
	"os"
	"os/signal"
	"syscall"

	salesAuditWorker "auditApp/salesAudit/worker"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
)

// concurrentJobs is how many jobs the worker runs at once.
const concurrentJobs = 10

type jobContext struct{}

// StartWorker runs the Redis worker until SIGINT/SIGTERM: the Sales Audit Zoho import and lead
// assignment, the mail sweeps and mail delivery.
func StartWorker(redisPool *redis.Pool, namespace string) {
	pool := work.NewWorkerPool(jobContext{}, concurrentJobs, namespace, redisPool)

	salesAuditWorker.SetupQueue(redisPool, namespace)
	salesAuditWorker.Register(pool)

	pool.Start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan

	pool.Stop()
}
