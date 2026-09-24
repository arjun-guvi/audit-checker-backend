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
// lead's BDM is mailed.
const PaymentVerificationPendingAfterSeconds = 24 * 60 * 60

// PaymentVerificationSweepJob is scheduled every 10 minutes in StartWorker. Its mails are
// delivered by the Sales Audit mail job (SendMailJob).
func PaymentVerificationSweepJob(job *work.Job) error {
	log.Printf("paymentVerification: sweep started")
	sent, err := RunPaymentVerificationSweep(context.Background(), config.SalesAuditProgram)
	log.Printf("paymentVerification: sweep sent %d mail(s)", sent)
	return err
}

// RunPaymentVerificationSweep mails the BDM, once per lead, about every learner whose payment has
// been unverified for over 24h, and records the time on the lead. Leads without a BDM email are
// left unmarked so they are mailed once one is set. Returns how many mails were sent.
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
		if len(PaymentVerificationRecipients(lead)) == 0 {
			log.Printf("paymentVerification: lead %s (%s) has no BDM email, not mailing", lead.ID, lead.Email)
			continue
		}
		mail, err := SendPaymentVerificationMail(ctx, program, SystemUser, lead, models.TriggerAuto)
		if err != nil {
			return sent, fmt.Errorf("mailing lead %s: %w", lead.ID, err)
		}
		if err := SetPaymentVerificationMailedAt(ctx, lead.ID, mail.SentAt); err != nil {
			return sent, fmt.Errorf("marking lead %s mailed: %w", lead.ID, err)
		}
		log.Printf("paymentVerification: mailed the BDM of %s about an unverified payment", lead.Email)
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

// SendPaymentVerificationMail logs and queues the "payment not verified yet" mail to the BDM.
func SendPaymentVerificationMail(ctx context.Context, program, by string, lead models.ZohoLead, trigger string) (models.Mail, error) {
	return SendAlert(ctx, program, by, lead.ID, models.AlertPaymentVerificationPending, trigger,
		PaymentVerificationRecipients(lead), PaymentVerificationSubject(lead.StudentFullName, lead.Course))
}

// PaymentVerificationRecipients is the lead's BDM (Sale_Owner_s_Manager). The learner is never
// mailed.
func PaymentVerificationRecipients(lead models.ZohoLead) []string {
	return nonEmpty(ContactEmail(lead.SaleOwnerManager))
}

func PaymentVerificationSubject(name, course string) string {
	subject := "Payment not verified for over 24h: " + firstNonEmpty(strings.TrimSpace(name), "learner")
	if course != "" {
		subject += " (" + course + ")"
	}
	return subject
}

// paymentVerificationMailBody is built from the lead and its payments as they are at delivery
// time; if they cannot be read the short generic body is sent instead.
func paymentVerificationMailBody(ctx context.Context, alert models.Alert) string {
	lead, err := FindZohoLead(ctx, alert.LeadID)
	if err != nil {
		log.Printf("paymentVerification: mail %s: lead %s: %v, sending the short body", alert.ID, alert.LeadID, err)
		return salesAuditMailBody(alert.Subject)
	}
	payments, err := FindPaymentsByEmail(ctx, []string{strings.ToLower(lead.Email)})
	if err != nil {
		log.Printf("paymentVerification: mail %s: payments of %s: %v", alert.ID, lead.Email, err)
	}
	return paymentVerificationBody(lead, payments)
}

func paymentVerificationBody(lead models.ZohoLead, payments []models.ZohoPayment) string {
	e := html.EscapeString
	cell := `<td style="padding:4px 12px 4px 0">`
	row := func(label, value string) string {
		if strings.TrimSpace(value) == "" {
			value = "-"
		}
		return `<tr><td style="padding:4px 12px 4px 0;color:#5E7087">` + e(label) + `</td>` + cell + e(value) + `</td></tr>`
	}

	var b strings.Builder
	b.WriteString(`<p>Hi,</p>`)
	fmt.Fprintf(&b, `<p>A payment from <b>%s</b> has not been verified by Accounts for over 24 hours. Please follow up with the BDA so it can be verified.</p>`,
		e(firstNonEmpty(strings.TrimSpace(lead.StudentFullName), lead.Email)))

	b.WriteString(`<table style="border-collapse:collapse;font-size:14px">`)
	b.WriteString(row("Learner email", lead.Email))
	b.WriteString(row("Phone", lead.PrimaryPhone))
	b.WriteString(row("Course", lead.Course))
	b.WriteString(row("Course value", Rupees(lead.CourseValue)))
	b.WriteString(row("Payment type", lead.PaymentType))
	b.WriteString(row("Stage", lead.Stage))
	b.WriteString(row("BDA", lead.SaleOwner))
	b.WriteString(`</table>`)

	unverified := []models.ZohoPayment{}
	for _, payment := range payments {
		if VerificationStatus(payment.Verified) == "unverified" {
			unverified = append(unverified, payment)
		}
	}
	if len(unverified) > 0 {
		b.WriteString(`<p><b>Unverified payments</b></p><table style="border-collapse:collapse;font-size:14px">`)
		b.WriteString(`<tr style="color:#5E7087">` + cell + `Type</td>` + cell + `Amount</td>` + cell + `Mode</td>` + cell + `Paid on</td>` + cell + `Added on</td></tr>`)
		for _, payment := range unverified {
			b.WriteString(`<tr>` + cell + e(payment.Type) + `</td>` + cell + e(Rupees(payment.Amount)) + `</td>` + cell +
				e(payment.ModeOfPayment) + `</td>` + cell + e(DisplayDate(payment.PaymentDate)) + `</td>` + cell +
				e(DisplayDate(payment.AddedTime)) + `</td></tr>`)
		}
		b.WriteString(`</table>`)
	}

	b.WriteString(`<p>Ask the BDA to share the payment receipt or UTR / transaction ID with Accounts. Open the Sales Audit portal to see the lead.</p>`)
	b.WriteString(`<p style="color:#5E7087;font-size:12px">Automated mail from Zen Sales Audit.</p>`)
	return b.String()
}
