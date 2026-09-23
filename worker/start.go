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
	var ctx context

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

	// Register job handlers
	pool.Job(models.HELLO_WORLD_JOB, HelloWorld)

	pool.Start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan

	pool.Stop()
}
