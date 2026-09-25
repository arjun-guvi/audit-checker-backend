package core

import (
	"fmt"
	"html"
	"strings"
	"time"

	"auditApp/salesAudit/models"
)

// Mail subjects and HTML bodies. Bodies are built when the mail is queued and stored on the
// alert, so delivery does not need to read the lead again.

// EscalationAfterSeconds is how long a lead may wait with an unverified or mismatched payment
// before its BDA, BDM and Accounts are mailed; RemailAfterSeconds is the gap between repeats.
const (
	EscalationAfterSeconds                 = 24 * 60 * 60
	RemailAfterSeconds                     = 24 * 60 * 60
	PaymentVerificationPendingAfterSeconds = 24 * 60 * 60
)

// PendingSince is when the lead entered Sales Action Pending: its enrolment, else CRM creation.
func PendingSince(lead models.Lead) int64 {
	return FirstNonZero(lead.EnrolledAt, lead.CrmCreatedAt)
}

// EscalationDue: needs escalation, over 24h pending, and not mailed in the last 24h.
func EscalationDue(lead models.Lead, now int64) bool {
	since := PendingSince(lead)
	return NeedsEscalation(lead) && since > 0 && now-since > EscalationAfterSeconds &&
		(lead.LastEscalationAt == 0 || now-lead.LastEscalationAt >= RemailAfterSeconds)
}

// PaymentVerificationDue: the BDM was not mailed yet and a payment has stayed unverified for over
// 24h (dated by its payment date, else the lead's pending date).
func PaymentVerificationDue(lead models.Lead, now int64) bool {
	if lead.PaymentVerificationMailedAt > 0 || lead.BdmEmail == "" {
		return false
	}
	for _, record := range lead.Payment.Records {
		if VerificationStatus(record.Verified) != "unverified" {
			continue
		}
		at := FirstNonZero(ZohoSeconds(record.PaymentDate), PendingSince(lead))
		if at > 0 && now-at > PaymentVerificationPendingAfterSeconds {
			return true
		}
	}
	return false
}

// RecheckReminderDue: open, and 24h since it was raised or last reminded.
func RecheckReminderDue(recheck models.Recheck, now int64) bool {
	if recheck.Status != models.RecheckOpen {
		return false
	}
	last := recheck.RaisedAt
	if recheck.LastReminder != nil {
		last = recheck.LastReminder.SentAt
	}
	return now-last >= RemailAfterSeconds
}

// NonEmpty drops blank values and duplicates.
func NonEmpty(values ...string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[strings.ToLower(value)] {
			seen[strings.ToLower(value)] = true
			result = append(result, value)
		}
	}
	return result
}

func leadName(lead models.Lead) string {
	return FirstNonEmpty(lead.Personal.Name, lead.Personal.Email, "Lead "+lead.ZenID)
}

// Subjects.

func EscalationSubject(lead models.Lead) string {
	return "Sales Action Pending over 24h, payment not verified: " + leadName(lead)
}

func PaymentVerificationSubject(lead models.Lead) string {
	subject := "Payment not verified for over 24h: " + leadName(lead)
	if lead.Course.Product != "" {
		subject += " (" + lead.Course.Product + ")"
	}
	return subject
}

func RecheckSubject(recheck models.Recheck) string {
	return fmt.Sprintf("Recheck %s raised (%s): %s", recheck.RecheckNo, models.RecheckCategories[recheck.Category], recheck.LeadName)
}

func RecheckReminderSubject(recheck models.Recheck) string {
	return fmt.Sprintf("Reminder: recheck %s still open after 24h: %s", recheck.RecheckNo, recheck.LeadName)
}

func RecheckClosedSubject(recheck models.Recheck) string {
	return fmt.Sprintf("Recheck %s closed, ready to audit again: %s", recheck.RecheckNo, recheck.LeadName)
}

func LeadsAssignedSubject(count int) string {
	if count == 1 {
		return "1 new lead assigned to you for audit"
	}
	return fmt.Sprintf("%d new leads assigned to you for audit", count)
}

func CcUpdatedSubject(lead models.Lead) string {
	return "CC updated, ready to verify: " + leadName(lead)
}

// Bodies.

const mailFooter = `<p style="color:#5E7087;font-size:12px">Automated mail from Zen Sales Audit.</p>`

func table(rows ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<table style="border-collapse:collapse;font-size:14px">`)
	for _, row := range rows {
		value := row[1]
		if strings.TrimSpace(value) == "" {
			value = "-"
		}
		b.WriteString(`<tr><td style="padding:4px 12px 4px 0;color:#5E7087">` + html.EscapeString(row[0]) +
			`</td><td style="padding:4px 0">` + html.EscapeString(value) + `</td></tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}

func leadRows(lead models.Lead) [][2]string {
	return [][2]string{
		{"Learner", leadName(lead)},
		{"Zen ID", lead.ZenID},
		{"Learner email", lead.Personal.Email},
		{"Course", lead.Course.Product},
		{"Payment type", lead.Payment.PaymentType},
		{"Course fee", rupeesOrBlank(lead.Payment.CourseFee)},
		{"BDA", lead.BdaEmail},
		{"BDM", lead.BdmEmail},
	}
}

func paymentsTable(records []models.PaymentRecord) string {
	if len(records) == 0 {
		return ""
	}
	cell := `<td style="padding:4px 12px 4px 0">`
	var b strings.Builder
	b.WriteString(`<table style="border-collapse:collapse;font-size:14px"><tr style="color:#5E7087">` +
		cell + `Type</td>` + cell + `Amount</td>` + cell + `Mode</td>` + cell + `Paid on</td>` + cell + `Status</td></tr>`)
	for _, record := range records {
		b.WriteString(`<tr>` + cell + html.EscapeString(record.Type) + `</td>` + cell + html.EscapeString(Rupees(record.Amount)) +
			`</td>` + cell + html.EscapeString(record.ModeOfPayment) + `</td>` + cell + html.EscapeString(DisplayDate(record.PaymentDate)) +
			`</td>` + cell + html.EscapeString(FirstNonEmpty(record.Verified, "Not verified")) + `</td></tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}

// pendingFor writes a duration in seconds as "2 days 5 hours" or "26 hours".
func pendingFor(seconds int64) string {
	hours := seconds / 3600
	if hours < 48 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d days %d hours", hours/24, hours%24)
}

// EscalationBody tells the BDA, BDM and Accounts which lead is stuck and which payments still
// need verifying.
func EscalationBody(lead models.Lead, now int64) string {
	since := PendingSince(lead)
	var b strings.Builder
	fmt.Fprintf(&b, `<p>Hi,</p><p><b>%s</b> has been in <b>Sales Action Pending</b> for <b>%s</b> (since %s IST) and has a payment that is not verified yet.</p>`,
		html.EscapeString(leadName(lead)), html.EscapeString(pendingFor(now-since)), html.EscapeString(FormatTime(since)))
	b.WriteString(table(leadRows(lead)...))
	b.WriteString(`<p><b>Payments</b></p>` + paymentsTable(UnverifiedPayments(lead)))
	b.WriteString(`<p><b>What to do</b></p><ul>` +
		`<li>BDA: share the payment receipt or UTR / transaction ID with Accounts.</li>` +
		`<li>BDM: follow up with the BDA so the lead leaves Sales Action Pending.</li>` +
		`<li>Accounts: verify the payment, or mark it as a mismatch.</li></ul>`)
	b.WriteString(`<p>This mail repeats every 24 hours until the payment is verified or the lead is audited.</p>` + mailFooter)
	return b.String()
}

// PaymentVerificationBody asks the BDM to follow up on an unverified payment.
func PaymentVerificationBody(lead models.Lead) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<p>Hi,</p><p>A payment from <b>%s</b> has not been verified by Accounts for over 24 hours. Please follow up with the BDA so it can be verified.</p>`,
		html.EscapeString(leadName(lead)))
	b.WriteString(table(leadRows(lead)...))
	if unverified := UnverifiedPayments(lead); len(unverified) > 0 {
		b.WriteString(`<p><b>Unverified payments</b></p>` + paymentsTable(unverified))
	}
	b.WriteString(`<p>Ask the BDA to share the payment receipt or UTR / transaction ID with Accounts.</p>` + mailFooter)
	return b.String()
}

// RecheckBody tells the BDA and BDM what to fix, with the recheck number and lead.
func RecheckBody(recheck models.Recheck, lead models.Lead) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<p>Hi,</p><p>The audit team raised recheck <b>%s</b> on <b>%s</b>. Please fix the issue below and close the ticket in the Sales Audit portal.</p>`,
		html.EscapeString(recheck.RecheckNo), html.EscapeString(leadName(lead)))
	b.WriteString(table(append([][2]string{
		{"Recheck ID", recheck.RecheckNo},
		{"Category", models.RecheckCategories[recheck.Category]},
		{"Comments", recheck.Comments},
		{"Raised by", FirstNonEmpty(recheck.RaisedBy.Name, recheck.RaisedBy.Email)},
		{"Raised at", FormatTime(recheck.RaisedAt) + " IST"},
	}, leadRows(lead)...)...))
	b.WriteString(mailFooter)
	return b.String()
}

func RecheckReminderBody(recheck models.Recheck) string {
	return fmt.Sprintf(`<p>Hi,</p><p>Recheck <b>%s</b> (%s) on <b>%s</b> is still open. It was raised on %s IST.</p><p>Comments: %s</p>`,
		html.EscapeString(recheck.RecheckNo), html.EscapeString(models.RecheckCategories[recheck.Category]),
		html.EscapeString(recheck.LeadName), html.EscapeString(FormatTime(recheck.RaisedAt)), html.EscapeString(recheck.Comments)) + mailFooter
}

func RecheckClosedBody(recheck models.Recheck) string {
	closedBy, note := "", ""
	if recheck.Closed != nil {
		closedBy = FirstNonEmpty(recheck.Closed.By.Name, recheck.Closed.By.Email)
		note = recheck.Closed.Note
	}
	return fmt.Sprintf(`<p>Hi,</p><p>Recheck <b>%s</b> on <b>%s</b> was closed by %s. The lead can be audited again.</p><p>Note: %s</p>`,
		html.EscapeString(recheck.RecheckNo), html.EscapeString(recheck.LeadName), html.EscapeString(closedBy),
		html.EscapeString(FirstNonEmpty(note, "-"))) + mailFooter
}

func LeadsAssignedBody(leads []models.Lead) string {
	var b strings.Builder
	b.WriteString(`<p>Hi,</p><p>These leads were assigned to you for audit:</p><ul>`)
	for _, lead := range leads {
		fmt.Fprintf(&b, `<li>%s (Zen ID %s, %s)</li>`, html.EscapeString(leadName(lead)),
			html.EscapeString(lead.ZenID), html.EscapeString(lead.Course.Product))
	}
	b.WriteString(`</ul>` + mailFooter)
	return b.String()
}

func CcUpdatedBody(lead models.Lead) string {
	return fmt.Sprintf(`<p>Hi,</p><p>The CC for <b>%s</b> (Zen ID %s) is now updated in Zoho. It can be verified now.</p>`,
		html.EscapeString(leadName(lead)), html.EscapeString(lead.ZenID)) + mailFooter
}

// TestMailBody is the body of POST /test-mail.
func TestMailBody(now time.Time) string {
	return fmt.Sprintf(`<p>This is a test mail from Zen Sales Audit, sent at %s IST.</p><p>If you are reading it, the SMTP settings work.</p>`,
		now.In(IST).Format("02-Jan-2006 03:04:05 PM")) + mailFooter
}
