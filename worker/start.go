package worker

import (
	"os"
	"os/signal"
	"syscall"

	"auditApp/models"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
)

func StartWorker(redisPool *redis.Pool, namespace string) {
	var ctx jobContext

	pool := work.NewWorkerPool(
		ctx,
		models.CONCURRENT_TASKS,
		namespace,
		redisPool,
	)

	// Cron schedulers - runs every 2 minutes
	//     *      *     *   *    *      *
	//  Second Minute Hour Day Month day(week)
	pool.PeriodicallyEnqueue("0 */2 * * * *", models.HELLO_WORLD_JOB)
	pool.PeriodicallyEnqueue("0 */15 * * * *", models.ZOHO_LEARNER_IMPORT_JOB)
	pool.PeriodicallyEnqueue("0 */10 * * * *", models.PAYMENT_VERIFICATION_SWEEP_JOB)

	// Register job handlers
	pool.Job(models.HELLO_WORLD_JOB, HelloWorld)
	pool.Job(models.ZOHO_LEARNER_IMPORT_JOB, ZohoLearnerImportJob)

	// Payment verification: mails learners whose payment is unverified for over 24h.
	// The mails are sent by the salesAudit_send_mail job registered below.
	pool.Job(models.PAYMENT_VERIFICATION_SWEEP_JOB, PaymentVerificationSweepJob)

	// Sales Audit sweeps and mail delivery
	registerSalesAuditJobs(pool, redisPool, namespace)

	pool.Start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan

	pool.Stop()
}
