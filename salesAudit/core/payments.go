package core

import (
	"fmt"
	"strings"

	"auditApp/salesAudit/models"
)

// Payment rules a lead must meet before it can be audited (kept in step with the frontend's
// utils/leadStatus.js getPaymentShortfall).
const (
	// SubscriptionMinPaid includes the ₹999 registration fee.
	SubscriptionMinPaid = 15000
	EmiMinPaidShare     = 0.4
)

// PaymentMode groups a Zoho payment type: full, partial, emi, emiPartial or subscription.
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

// VerifiedAmount is the money received and verified: every "Yes" record except discount notes.
func VerifiedAmount(records []models.PaymentRecord) float64 {
	total := 0.0
	for _, record := range records {
		if record.Verified == "Yes" && record.Type != models.CreditDiscount {
			total += record.Amount
		}
	}
	return total
}

// PaymentShortfall is "" once every payment is verified and the plan's minimum is met, else why
// the lead cannot be audited yet.
func PaymentShortfall(payment models.LeadPayment) string {
	if len(payment.Records) == 0 {
		return "No payments received yet"
	}
	for _, record := range payment.Records {
		if record.Verified != "Yes" {
			return "Auditing starts once Accounts has verified every payment"
		}
	}
	paid := VerifiedAmount(payment.Records)
	fee := payment.CourseFee
	switch PaymentMode(payment.PaymentType) {
	case "full":
		if fee <= 0 || paid < fee {
			return "Full payment not received and verified"
		}
	case "subscription":
		if paid < SubscriptionMinPaid {
			return fmt.Sprintf("Subscription needs ₹%d verified (incl. ₹999 registration)", SubscriptionMinPaid)
		}
	case "emi", "emiPartial":
		if fee <= 0 || paid < fee*EmiMinPaidShare {
			return fmt.Sprintf("EMI needs %.0f%% of the course fee verified", EmiMinPaidShare*100)
		}
	}
	return ""
}

// WithPaymentReadiness fills Ready, Shortfall and VerifiedAmount.
func WithPaymentReadiness(payment models.LeadPayment) models.LeadPayment {
	payment.VerifiedAmount = VerifiedAmount(payment.Records)
	payment.Shortfall = PaymentShortfall(payment)
	payment.Ready = payment.Shortfall == ""
	return payment
}

// NeedsEscalation: the lead is not audited yet and has money paid that is not cleanly verified.
func NeedsEscalation(lead models.Lead) bool {
	if lead.Audit.Status == models.AuditCompleted {
		return false
	}
	for _, record := range lead.Payment.Records {
		if status := VerificationStatus(record.Verified); status == "unverified" || status == "mismatch" {
			return true
		}
	}
	return false
}

// UnverifiedPayments are the records Accounts still has to verify.
func UnverifiedPayments(lead models.Lead) []models.PaymentRecord {
	result := []models.PaymentRecord{}
	for _, record := range lead.Payment.Records {
		if VerificationStatus(record.Verified) != "verified" {
			result = append(result, record)
		}
	}
	return result
}
