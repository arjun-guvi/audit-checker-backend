package worker

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"

	"auditApp/salesAudit/models"
	"auditApp/salesAudit/service"
	"auditApp/salesAudit/store"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
)

// Job names.
const (
	JobEscalationSweep      = "salesAudit_sap_escalation_sweep"
	JobRecheckReminderSweep = "salesAudit_recheck_reminder_sweep"
	JobSendMail             = "salesAudit_send_mail"
)

// Sweeps run at the top of every hour (second minute hour day month weekday).
const sweepSchedule = "0 0 * * * *"

// Queue enqueues mail jobs; it is the service's MailQueue in the HTTP server.
type Queue struct {
	enqueuer *work.Enqueuer
}

func NewQueue(namespace string, pool *redis.Pool) *Queue {
	return &Queue{enqueuer: work.NewEnqueuer(namespace, pool)}
}

func (q *Queue) EnqueueMail(program, alertID string) error {
	_, err := q.enqueuer.Enqueue(JobSendMail, work.Q{"program": program, "alertId": alertID})
	return err
}

// Register schedules the sweeps and registers every Sales Audit job handler on the pool.
func Register(pool *work.WorkerPool, svc *service.Service, program string, mailer Mailer) {
	pool.PeriodicallyEnqueue(sweepSchedule, JobEscalationSweep)
	pool.PeriodicallyEnqueue(sweepSchedule, JobRecheckReminderSweep)

	pool.Job(JobEscalationSweep, func(job *work.Job) error {
		sent, err := svc.RunEscalationSweep(context.Background(), program)
		log.Printf("salesAudit: escalation sweep sent %d mail(s)", sent)
		return err
	})
	pool.Job(JobRecheckReminderSweep, func(job *work.Job) error {
		sent, err := svc.RunRecheckReminderSweep(context.Background(), program)
		log.Printf("salesAudit: recheck reminder sweep sent %d mail(s)", sent)
		return err
	})
	pool.Job(JobSendMail, func(job *work.Job) error {
		return SendMail(context.Background(), svc.Store, mailer, job.ArgString("program"), job.ArgString("alertId"))
	})
}

// SendMail delivers one logged alert. Without SMTP settings the mail stays recorded (the UI
// shows it) and is marked skipped.
func SendMail(ctx context.Context, s store.Store, mailer Mailer, program, alertID string) error {
	alert, err := s.GetAlert(ctx, program, alertID)
	if errors.Is(err, store.ErrNotFound) {
		log.Printf("salesAudit: mail %s not found, dropping", alertID)
		return nil
	}
	if err != nil {
		return err
	}
	if !mailer.Configured() {
		log.Printf("salesAudit: SMTP not configured, not sending %q to %v", alert.Subject, alert.To)
		return s.SetAlertDelivery(ctx, program, alertID, models.DeliverySkipped)
	}
	if len(alert.To) == 0 {
		return s.SetAlertDelivery(ctx, program, alertID, models.DeliverySkipped)
	}
	if err := mailer.Send(alert.To, alert.Subject, mailBody(alert.Subject)); err != nil {
		if markErr := s.SetAlertDelivery(ctx, program, alertID, models.DeliveryFailed); markErr != nil {
			log.Printf("salesAudit: marking mail %s failed: %v", alertID, markErr)
		}
		return fmt.Errorf("sending mail %s: %w", alertID, err)
	}
	return s.SetAlertDelivery(ctx, program, alertID, models.DeliverySent)
}

func mailBody(subject string) string {
	return fmt.Sprintf(
		`<p>%s</p><p>Open the Sales Audit portal to see the details and act on it.</p>`+
			`<p style="color:#5E7087;font-size:12px">Automated mail from Zen Sales Audit.</p>`,
		html.EscapeString(subject))
}
