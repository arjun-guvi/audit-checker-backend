package worker

import (
	"strings"
	"testing"
	"time"

	"auditApp/models"
)

func TestPaymentVerificationDue(t *testing.T) {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, ist).Unix()
	lead := models.ZohoLead{ID: "a@x.com", Email: "a@x.com", AddedTime: "20-Sep-2026 10:00:00"}
	old := "22-Sep-2026 10:00:00"    // over 24h before now
	recent := "24-Sep-2026 09:00:00" // 3h before now

	cases := []struct {
		name     string
		lead     models.ZohoLead
		payments []models.ZohoPayment
		want     bool
	}{
		{"unverified over 24h", lead, []models.ZohoPayment{{Verified: "No", AddedTime: old}}, true},
		{"unverified blank status", lead, []models.ZohoPayment{{Verified: "", AddedTime: old}}, true},
		{"unverified under 24h", lead, []models.ZohoPayment{{Verified: "No", AddedTime: recent}}, false},
		{"verified", lead, []models.ZohoPayment{{Verified: "Yes", AddedTime: old}}, false},
		{"mismatch is not unverified", lead, []models.ZohoPayment{{Verified: "Mismatch", AddedTime: old}}, false},
		{"falls back to payment date", lead, []models.ZohoPayment{{Verified: "No", PaymentDate: old}}, true},
		{"falls back to lead added time", lead, []models.ZohoPayment{{Verified: "No"}}, true},
		{"one due payment is enough", lead, []models.ZohoPayment{
			{Verified: "Yes", AddedTime: old}, {Verified: "No", AddedTime: old},
		}, true},
		{"no payments", lead, nil, false},
		{"already mailed", models.ZohoLead{Email: "a@x.com", PaymentVerificationMailedAt: now - 60},
			[]models.ZohoPayment{{Verified: "No", AddedTime: old}}, false},
	}
	for _, c := range cases {
		if got := PaymentVerificationDue(c.lead, c.payments, now); got != c.want {
			t.Errorf("%s: PaymentVerificationDue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPaymentVerificationMailBody(t *testing.T) {
	alert := models.Alert{Kind: models.AlertPaymentVerificationPending,
		Mail: models.Mail{Subject: PaymentVerificationSubject("Full Stack <Dev>")}}
	if body := mailBody(alert); body == salesAuditMailBody(alert.Subject) ||
		!strings.Contains(body, "Full Stack &lt;Dev&gt;") {
		t.Fatalf("mailBody() = %s", body)
	}
}
