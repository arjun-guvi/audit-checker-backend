package worker

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/smtp"
	"strings"

	"auditApp/config"
	"auditApp/models"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
)

// Sales Audit sweeps run at the top of every hour (second minute hour day month weekday).
const salesAuditSweepSchedule = "0 0 * * * *"

var salesAuditEnqueuer *work.Enqueuer

// SetupSalesAuditQueue lets the HTTP server queue mails on the worker's namespace.
func SetupSalesAuditQueue(redisPool *redis.Pool, namespace string) {
	salesAuditEnqueuer = work.NewEnqueuer(namespace, redisPool)
}

// EnqueueSalesAuditMail hands a logged alert to the mail job. It is a variable so tests can
// replace it.
var EnqueueSalesAuditMail = func(program, alertID string) error {
	if salesAuditEnqueuer == nil {
		return errors.New("sales audit mail queue is not set up")
	}
	_, err := salesAuditEnqueuer.Enqueue(models.SALES_AUDIT_SEND_MAIL_JOB, work.Q{"program": program, "alertId": alertID})
	return err
}

// registerSalesAuditJobs schedules the sweeps and registers the Sales Audit job handlers.
func registerSalesAuditJobs(pool *work.WorkerPool, redisPool *redis.Pool, namespace string) {
	SetupSalesAuditQueue(redisPool, namespace)

	pool.PeriodicallyEnqueue(salesAuditSweepSchedule, models.SALES_AUDIT_ESCALATION_SWEEP_JOB)
	pool.PeriodicallyEnqueue(salesAuditSweepSchedule, models.SALES_AUDIT_RECHECK_REMINDER_SWEEP_JOB)

	pool.Job(models.SALES_AUDIT_ESCALATION_SWEEP_JOB, EscalationSweepJob)
	pool.Job(models.SALES_AUDIT_RECHECK_REMINDER_SWEEP_JOB, RecheckReminderSweepJob)
	pool.Job(models.SALES_AUDIT_SEND_MAIL_JOB, SendMailJob)
}

func EscalationSweepJob(job *work.Job) error {
	sent, err := RunEscalationSweep(context.Background(), config.SalesAuditProgram)
	log.Printf("salesAudit: escalation sweep sent %d mail(s)", sent)
	return err
}

func RecheckReminderSweepJob(job *work.Job) error {
	sent, err := RunRecheckReminderSweep(context.Background(), config.SalesAuditProgram)
	log.Printf("salesAudit: recheck reminder sweep sent %d mail(s)", sent)
	return err
}

func SendMailJob(job *work.Job) error {
	return SendSalesAuditMail(context.Background(), job.ArgString("program"), job.ArgString("alertId"))
}

// RunEscalationSweep mails every lead that has been over 24h in SAP with an unverified or
// mismatched payment and was not mailed in the last 24h. Returns how many mails were sent.
func RunEscalationSweep(ctx context.Context, program string) (int, error) {
	leads, err := FindLeads(ctx, program)
	if err != nil {
		return 0, err
	}
	now := Now().Unix()
	sent := 0
	for _, lead := range leads {
		if !EscalationDue(lead, now) {
			continue
		}
		if _, err := Escalate(ctx, program, SystemUser, lead, models.TriggerAuto); err != nil {
			return sent, fmt.Errorf("escalating lead %s: %w", lead.ID, err)
		}
		sent++
	}
	return sent, nil
}

// RunRecheckReminderSweep re-mails the BDA and BDM about every recheck still open 24h after it
// was raised or last reminded. Returns how many reminders were sent.
func RunRecheckReminderSweep(ctx context.Context, program string) (int, error) {
	rechecks, err := FindRechecks(ctx, program, "", models.RecheckOpen)
	if err != nil {
		return 0, err
	}
	now := Now().Unix()
	sent := 0
	for _, recheck := range rechecks {
		if !RecheckReminderDue(recheck, now) {
			continue
		}
		lead, _, err := FindLead(ctx, program, recheck.LeadID)
		if err != nil {
			log.Printf("salesAudit: recheck %s: lead %s: %v", recheck.ID, recheck.LeadID, err)
			continue
		}
		mail, err := SendAlert(ctx, program, SystemUser, lead.ID, models.AlertRecheckReminder,
			models.TriggerAuto, BdaAndBdmRecipients(lead), RecheckReminderSubject(lead.StudentFullName))
		if err != nil {
			return sent, err
		}
		if err := setRecheckReminder(ctx, program, recheck.ID, mail); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// SendSalesAuditMail delivers one logged alert. Without SMTP settings the mail stays recorded
// (the UI shows it) and is marked skipped.
func SendSalesAuditMail(ctx context.Context, program, alertID string) error {
	alert, err := FindAlert(ctx, program, alertID)
	if IsNotFound(err) {
		log.Printf("salesAudit: mail %s not found, dropping", alertID)
		return nil
	}
	if err != nil {
		return err
	}
	if !smtpConfigured() {
		log.Printf("salesAudit: SMTP not configured, not sending %q to %v", alert.Subject, alert.To)
		return SetAlertDelivery(ctx, program, alertID, models.DeliverySkipped)
	}
	if len(alert.To) == 0 {
		return SetAlertDelivery(ctx, program, alertID, models.DeliverySkipped)
	}
	if err := deliverMail(alert.To, alert.Subject, mailBody(alert)); err != nil {
		if markErr := SetAlertDelivery(ctx, program, alertID, models.DeliveryFailed); markErr != nil {
			log.Printf("salesAudit: marking mail %s failed: %v", alertID, markErr)
		}
		return fmt.Errorf("sending mail %s: %w", alertID, err)
	}
	return SetAlertDelivery(ctx, program, alertID, models.DeliverySent)
}

func smtpConfigured() bool {
	return config.SMTPHost != "" && config.SMTPUsername != "" && config.SMTPPassword != "" && config.SMTPFrom != ""
}

// deliverMail sends one HTML mail over SMTP. It is a variable so tests can replace it.
var deliverMail = func(to []string, subject, htmlBody string) error {
	message := "From: " + config.SMTPFrom + "\r\n" +
		"To: " + strings.Join(to, ",") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n" +
		htmlBody
	auth := smtp.PlainAuth("", config.SMTPUsername, config.SMTPPassword, config.SMTPHost)
	return smtp.SendMail(config.SMTPHost+":"+config.SMTPPort, auth, config.SMTPFrom, to, []byte(message))
}

// mailBody picks the body by alert kind: learner mails differ from the internal alerts.
func mailBody(alert models.Alert) string {
	if alert.Kind == models.AlertPaymentVerificationPending {
		return paymentVerificationMailBody(alert)
	}
	return salesAuditMailBody(alert.Subject)
}

func salesAuditMailBody(subject string) string {
	return fmt.Sprintf(
		`<p>%s</p><p>Open the Sales Audit portal to see the details and act on it.</p>`+
			`<p style="color:#5E7087;font-size:12px">Automated mail from Zen Sales Audit.</p>`,
		html.EscapeString(subject))
}
