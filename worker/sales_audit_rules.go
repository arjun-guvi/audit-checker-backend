package worker

import (
	"fmt"
	"strings"

	"auditApp/models"
)

// Business rules, kept in step with the frontend (utils/leadStatus.js, mocks/mailSimulator.js).

// EscalationAfterSeconds is how long a lead may sit in Sales Action Pending with an unverified or
// mismatched payment before the BDA, BDM and Accounts are mailed.
const EscalationAfterSeconds = 24 * 60 * 60

// RemailAfterSeconds is the gap between repeat escalation and recheck reminder mails.
const RemailAfterSeconds = 24 * 60 * 60

// VerificationStatus maps a financial record's Verified value.
func VerificationStatus(verified string) string {
	switch verified {
	case "Yes":
		return "verified"
	case "Mismatch":
		return "mismatch"
	case "Refund":
		return "refund"
	default:
		return "unverified"
	}
}

func creditStatus(credit *models.Credit) string {
	if credit == nil {
		return "notPaid"
	}
	return VerificationStatus(credit.Verified)
}

// NeedsEscalation: the lead is still pending and has money paid that is not cleanly verified.
// Leads that have paid nothing are not escalated (nothing for Accounts to verify).
func NeedsEscalation(lead models.Lead) bool {
	if lead.Audit != nil {
		return false
	}
	for _, credit := range []*models.Credit{
		lead.Credits.BookingAmount, lead.Credits.Part1, lead.Credits.RemainingBalance,
	} {
		if status := creditStatus(credit); status == "unverified" || status == "mismatch" {
			return true
		}
	}
	return false
}

func IsSapOverdue(lead models.Lead, now int64) bool {
	return lead.SapEnteredAt > 0 && now-lead.SapEnteredAt > EscalationAfterSeconds
}

// EscalationDue: needs escalation, over 24h in SAP, and not mailed in the last 24h.
func EscalationDue(lead models.Lead, now int64) bool {
	return NeedsEscalation(lead) &&
		IsSapOverdue(lead, now) &&
		(lead.Escalation == nil || now-lead.Escalation.SentAt >= RemailAfterSeconds)
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

// PaymentMode maps a lead's payment type to the CC comparison's payment mode.
func PaymentMode(paymentType string) string {
	switch {
	case paymentType == "EMI + Partial Payment":
		return "emiPartial"
	case strings.HasPrefix(paymentType, "EMI"):
		return "emi"
	case paymentType == "Direct - Partial Payment":
		return "partial"
	case paymentType == "Subscription":
		return "subscription"
	default:
		return "full"
	}
}

func IsEmiPaymentType(paymentType string) bool {
	return strings.Contains(paymentType, "EMI")
}

// Mail recipients.

func EscalationRecipients(lead models.Lead, accountsEmail string) []string {
	return nonEmpty(ContactEmail(lead.SaleOwner), ContactEmail(lead.SaleOwnerManager), accountsEmail)
}

func BdaAndBdmRecipients(lead models.Lead) []string {
	return nonEmpty(ContactEmail(lead.SaleOwner), ContactEmail(lead.SaleOwnerManager))
}

func BdaRecipients(lead models.Lead) []string {
	return nonEmpty(ContactEmail(lead.SaleOwner))
}

func nonEmpty(values ...string) []string {
	result := []string{}
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

// Mail subjects, as the frontend shows them.

func EscalationSubject(name string) string {
	return "Sales Action Pending over 24h, payment not verified: " + name
}

func RecheckSubject(category, name string) string {
	return fmt.Sprintf("Recheck raised (%s): %s", models.RecheckCategories[category], name)
}

func RecheckReminderSubject(name string) string {
	return "Reminder: recheck still open after 24h: " + name
}

func CcNotSentSubject(name string) string {
	return "CC mail not sent: " + name
}
