// Package worker registers the Sales Audit jobs on the Redis worker pool: the Zoho import (which
// then assigns new leads), the mail sweeps and mail delivery.
package worker

import (
	"context"
	"log"
	"time"

	"auditApp/config"
	"auditApp/salesAudit/actions"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/zoho"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
)

// Schedules (second minute hour day month weekday).
const (
	importSchedule              = "0 */15 * * * *"
	sweepSchedule               = "0 0 * * * *"
	paymentVerificationSchedule = "0 */10 * * * *"
)

// SetupQueue lets the process queue jobs (mails, assignment) on the worker's namespace. Both the
// HTTP server and the worker call it.
func SetupQueue(redisPool *redis.Pool, namespace string) {
	enqueuer := work.NewEnqueuer(namespace, redisPool)
	actions.EnqueueMail = func(program, alertID string) error {
		_, err := enqueuer.Enqueue(models.JobSendMail, work.Q{"program": program, "alertId": alertID})
		return err
	}
}

// Register schedules the jobs and registers their handlers.
func Register(pool *work.WorkerPool) {
	pool.PeriodicallyEnqueue(importSchedule, models.JobZohoImport)
	pool.PeriodicallyEnqueue(sweepSchedule, models.JobEscalationSweep)
	pool.PeriodicallyEnqueue(sweepSchedule, models.JobRecheckReminderSweep)
	pool.PeriodicallyEnqueue(paymentVerificationSchedule, models.JobPaymentVerificationSweep)

	pool.Job(models.JobZohoImport, ZohoImportJob)
	pool.Job(models.JobAssignLeads, AssignLeadsJob)
	pool.Job(models.JobEscalationSweep, EscalationSweepJob)
	pool.Job(models.JobRecheckReminderSweep, RecheckReminderSweepJob)
	pool.Job(models.JobPaymentVerificationSweep, PaymentVerificationSweepJob)
	pool.Job(models.JobSendMail, SendMailJob)
}

// ZohoImportJob fetches the sync window from Zoho, imports it, then assigns unassigned leads.
func ZohoImportJob(job *work.Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	program := config.SalesAuditProgram
	from, to := zoho.Window(time.Now())
	learners, err := zoho.Fetch(ctx, from, to)
	if err != nil {
		return err
	}
	result, importErr := actions.ImportLearners(ctx, program, learners)
	log.Printf("salesAudit: Zoho import %s..%s: %+v", from, to, result)
	assigned, err := actions.AssignPending(ctx, program, core.SystemActor)
	log.Printf("salesAudit: assignment: %+v", assigned)
	if importErr != nil {
		return importErr
	}
	return err
}

func AssignLeadsJob(job *work.Job) error {
	result, err := actions.AssignPending(context.Background(), config.SalesAuditProgram, core.SystemActor)
	log.Printf("salesAudit: assignment: %+v", result)
	return err
}

func EscalationSweepJob(job *work.Job) error {
	sent, err := actions.RunEscalationSweep(context.Background(), config.SalesAuditProgram)
	log.Printf("salesAudit: escalation sweep sent %d mail(s)", sent)
	return err
}

func RecheckReminderSweepJob(job *work.Job) error {
	sent, err := actions.RunRecheckReminderSweep(context.Background(), config.SalesAuditProgram)
	log.Printf("salesAudit: recheck reminder sweep sent %d mail(s)", sent)
	return err
}

func PaymentVerificationSweepJob(job *work.Job) error {
	sent, err := actions.RunPaymentVerificationSweep(context.Background(), config.SalesAuditProgram)
	log.Printf("salesAudit: payment verification sweep sent %d mail(s)", sent)
	return err
}

func SendMailJob(job *work.Job) error {
	return actions.DeliverMail(context.Background(), job.ArgString("program"), job.ArgString("alertId"))
}
