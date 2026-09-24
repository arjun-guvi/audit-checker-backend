package worker

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/smtp"
	"strings"
	"time"

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

// RunEscalationSweep mails the BDA, BDM and Accounts about every lead that has been over 24h in SAP with an unverified or
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
	if err := deliverMail(alert.To, alert.Subject, mailBody(ctx, program, alert)); err != nil {
		if markErr := SetAlertDelivery(ctx, program, alertID, models.DeliveryFailed); markErr != nil {
			log.Printf("salesAudit: marking mail %s failed: %v", alertID, markErr)
		}
		return fmt.Errorf("sending mail %s: %w", alertID, err)
	}
	return SetAlertDelivery(ctx, program, alertID, models.DeliverySent)
}

// ErrSMTPNotConfigured is returned by SendTestMail when an SMTP setting is missing.
var ErrSMTPNotConfigured = errors.New("SMTP is not configured: set SMTP_HOST, SMTP_USERNAME, SMTP_PASSWORD and SMTP_FROM")

// SendTestMail sends one mail straight over SMTP, skipping the queue and the mail log, so the
// SMTP settings can be checked. It returns the SMTP error as is.
func SendTestMail(to string) error {
	if !smtpConfigured() {
		return ErrSMTPNotConfigured
	}
	return deliverMail([]string{to}, "Zen Sales Audit test mail", fmt.Sprintf(
		`<p>This is a test mail from Zen Sales Audit, sent at %s IST.</p>`+
			`<p>If you are reading it, the SMTP settings work.</p>`+
			`<p style="color:#5E7087;font-size:12px">Automated mail from Zen Sales Audit.</p>`,
		Now().In(zohoLocation).Format("02-Jan-2006 03:04:05 PM")))
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

// mailBody picks the body by alert kind. The payment verification and escalation bodies is built from the lead as it is at delivery time; if the lead cannot be read
// the short generic body is sent instead.
func mailBody(ctx context.Context, program string, alert models.Alert) string {
	switch alert.Kind {
	case models.AlertPaymentVerificationPending:
		return paymentVerificationMailBody(ctx, alert)
	case models.AlertEscalation:
		lead, _, err := FindLead(ctx, program, alert.LeadID)
		if err == nil {
			return escalationMailBody(lead, Now().Unix())
		}
		log.Printf("salesAudit: mail %s: lead %s: %v, sending the short body", alert.ID, alert.LeadID, err)
	}
	return salesAuditMailBody(alert.Subject)
}

func salesAuditMailBody(subject string) string {
	return fmt.Sprintf(
		`<p>%s</p><p>Open the Sales Audit portal to see the details and act on it.</p>`+
			`<p style="color:#5E7087;font-size:12px">Automated mail from Zen Sales Audit.</p>`,
		html.EscapeString(subject))
}

// escalationMailBody tells the BDA, BDM and Accounts which lead is stuck in Sales Action Pending,
// for how long, and which payments still need verifying.
func escalationMailBody(lead models.Lead, now int64) string {
	e := html.EscapeString
	row := func(label, value string) string {
		if strings.TrimSpace(value) == "" {
			value = "-"
		}
		return `<tr><td style="padding:4px 12px 4px 0;color:#5E7087">` + e(label) + `</td><td style="padding:4px 0">` + e(value) + `</td></tr>`
	}
	creditRow := func(label string, credit *models.Credit) string {
		if credit == nil {
			return ""
		}
		status := creditStatusLabel(VerificationStatus(credit.Verified))
		return `<tr><td style="padding:4px 12px 4px 0">` + e(label) + `</td><td style="padding:4px 12px 4px 0">` + e(Rupees(credit.Amount)) +
			`</td><td style="padding:4px 12px 4px 0">` + e(DisplayDate(credit.PaymentDate)) + `</td><td style="padding:4px 0">` + e(status) + `</td></tr>`
	}

	var b strings.Builder
	b.WriteString(`<p>Hi,</p>`)
	fmt.Fprintf(&b, `<p><b>%s</b> has been in <b>Sales Action Pending</b> for <b>%s</b> (since %s IST) and has a payment that is not verified yet.</p>`,
		e(firstNonEmpty(lead.StudentFullName, "This lead")), e(pendingFor(now-lead.SapEnteredAt)),
		e(time.Unix(lead.SapEnteredAt, 0).In(zohoLocation).Format("02-Jan-2006 03:04 PM")))

	b.WriteString(`<table style="border-collapse:collapse;font-size:14px">`)
	b.WriteString(row("Learner email", lead.Email))
	b.WriteString(row("Phone", lead.PrimaryPhone))
	b.WriteString(row("Course", lead.Course))
	b.WriteString(row("Course value", Rupees(lead.CourseValue)))
	b.WriteString(row("Payment type", lead.PaymentType))
	b.WriteString(row("Total paid", Rupees(lead.TotalPaid)))
	b.WriteString(row("Balance", Rupees(lead.BalanceAmount)))
	b.WriteString(row("BDA", lead.SaleOwner))
	b.WriteString(row("BDM", lead.SaleOwnerManager))
	b.WriteString(`</table>`)

	b.WriteString(`<p><b>Payments</b></p><table style="border-collapse:collapse;font-size:14px">`)
	b.WriteString(`<tr style="color:#5E7087"><td style="padding:4px 12px 4px 0">Type</td><td style="padding:4px 12px 4px 0">Amount</td><td style="padding:4px 12px 4px 0">Paid on</td><td style="padding:4px 0">Status</td></tr>`)
	b.WriteString(creditRow("Booking amount", lead.Credits.BookingAmount))
	b.WriteString(creditRow("Part 1", lead.Credits.Part1))
	b.WriteString(creditRow("Remaining balance", lead.Credits.RemainingBalance))
	b.WriteString(`</table>`)

	b.WriteString(`<p><b>What to do</b></p><ul>` +
		`<li>BDA: share the payment receipt or UTR / transaction ID with Accounts.</li>` +
		`<li>BDM: follow up with the BDA so the lead leaves Sales Action Pending.</li>` +
		`<li>Accounts: verify the payment, or mark it as a mismatch.</li></ul>`)
	b.WriteString(`<p>Open the Sales Audit portal to see the lead and act on it. This mail repeats every 24 hours until the payment is verified or the lead is audited.</p>`)
	b.WriteString(`<p style="color:#5E7087;font-size:12px">Automated mail from Zen Sales Audit.</p>`)
	return b.String()
}

func creditStatusLabel(status string) string {
	switch status {
	case "verified":
		return "Verified"
	case "mismatch":
		return "Mismatch"
	case "refund":
		return "Refund"
	default:
		return "Not verified"
	}
}

// pendingFor writes a duration in seconds as "2 days 5 hours" or "26 hours".
func pendingFor(seconds int64) string {
	hours := seconds / 3600
	if hours < 48 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d days %d hours", hours/24, hours%24)
}
