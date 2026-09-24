package worker

import (
	"context"
	"fmt"
	"html"
	"log"
	"strings"

	"auditApp/config"
	"auditApp/models"

	"github.com/gocraft/work"
)

// PaymentVerificationPendingAfterSeconds is how long a payment may stay unverified before the
// learner is mailed.
const PaymentVerificationPendingAfterSeconds = 24 * 60 * 60

// PaymentVerificationSweepJob is scheduled every 10 minutes in StartWorker. Its mails are
// delivered by the Sales Audit mail job (SendMailJob).
func PaymentVerificationSweepJob(job *work.Job) error {
	log.Printf("paymentVerification: sweep started")
	sent, err := RunPaymentVerificationSweep(context.Background(), config.SalesAuditProgram)
	log.Printf("paymentVerification: sweep sent %d mail(s)", sent)
	return err
}

// RunPaymentVerificationSweep mails every learner, once, whose payment has been unverified for
// over 24h, and records the time on the lead. Returns how many mails were sent.
func RunPaymentVerificationSweep(ctx context.Context, program string) (int, error) {
	leads, err := FindLeadsNotMailedForPaymentVerification(ctx)
	if err != nil {
		return 0, err
	}
	if len(leads) == 0 {
		log.Printf("paymentVerification: no leads left to mail")
		return 0, nil
	}

	emails := make([]string, 0, len(leads))
	for _, lead := range leads {
		emails = append(emails, strings.ToLower(lead.Email))
	}
	payments, err := FindPaymentsByEmail(ctx, emails)
	if err != nil {
		return 0, err
	}
	log.Printf("paymentVerification: checking %d lead(s) not yet mailed, %d payment(s)", len(leads), len(payments))
	paymentsByEmail := map[string][]models.ZohoPayment{}
	for _, payment := range payments {
		email := strings.ToLower(payment.Email)
		paymentsByEmail[email] = append(paymentsByEmail[email], payment)
	}

	now := Now().Unix()
	sent := 0
	for _, lead := range leads {
		if !PaymentVerificationDue(lead, paymentsByEmail[strings.ToLower(lead.Email)], now) {
			continue
		}
		mail, err := SendPaymentVerificationMail(ctx, program, SystemUser, lead, models.TriggerAuto)
		if err != nil {
			return sent, fmt.Errorf("mailing lead %s: %w", lead.ID, err)
		}
		if err := SetPaymentVerificationMailedAt(ctx, lead.ID, mail.SentAt); err != nil {
			return sent, fmt.Errorf("marking lead %s mailed: %w", lead.ID, err)
		}
		log.Printf("paymentVerification: mailed %s about an unverified payment", lead.Email)
		sent++
	}
	return sent, nil
}

// PaymentVerificationDue: the learner was not mailed yet and has a payment that has stayed
// unverified for over 24h. A payment is dated by when it was added, else by its payment date,
// else by when the lead was added; an undated payment is not counted.
func PaymentVerificationDue(lead models.ZohoLead, payments []models.ZohoPayment, now int64) bool {
	if lead.PaymentVerificationMailedAt > 0 {
		return false
	}
	for _, payment := range payments {
		if VerificationStatus(payment.Verified) != "unverified" {
			continue
		}
		createdAt, ok := ParseZohoTime(firstNonEmpty(payment.AddedTime, payment.PaymentDate, lead.AddedTime))
		if ok && now-createdAt > PaymentVerificationPendingAfterSeconds {
			return true
		}
	}
	return false
}

// SendPaymentVerificationMail logs and queues the "payment not verified yet" mail to the learner.
func SendPaymentVerificationMail(ctx context.Context, program, by string, lead models.ZohoLead, trigger string) (models.Mail, error) {
	return SendAlert(ctx, program, by, lead.ID, models.AlertPaymentVerificationPending, trigger,
		[]string{lead.Email}, PaymentVerificationSubject(lead.Course))
}

func PaymentVerificationSubject(course string) string {
	if course == "" {
		return "Your payment is pending verification"
	}
	return "Your payment for " + course + " is pending verification"
}

func paymentVerificationMailBody(alert models.Alert) string {
	return fmt.Sprintf(
		`<p>Hi,</p><p>%s. We received it over 24 hours ago and our accounts team is still verifying it.</p>`+
			`<p>If you have a payment receipt or UTR / transaction ID, please reply to this mail with it so we can verify it faster.</p>`+
			`<p style="color:#5E7087;font-size:12px">This is an automated mail.</p>`,
		html.EscapeString(alert.Subject))
}
