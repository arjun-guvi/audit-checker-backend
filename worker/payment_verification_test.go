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

func TestPaymentVerificationMail(t *testing.T) {
	lead := models.ZohoLead{
		StudentFullName: "Learner <One>", Email: "learner@example.com", Course: "Full Stack",
		SaleOwner: "Asha - bda@example.com", SaleOwnerManager: "Ravi - bdm@example.com",
	}
	if got := PaymentVerificationRecipients(lead); len(got) != 1 || got[0] != "bdm@example.com" {
		t.Errorf("recipients = %v, want only the BDM", got)
	}
	if got := PaymentVerificationRecipients(models.ZohoLead{Email: "learner@example.com"}); len(got) != 0 {
		t.Errorf("recipients without a BDM = %v, want none (never the learner)", got)
	}
	if got := PaymentVerificationSubject(lead.StudentFullName, lead.Course); got != "Payment not verified for over 24h: Learner <One> (Full Stack)" {
		t.Errorf("subject = %q", got)
	}

	body := paymentVerificationBody(lead, []models.ZohoPayment{
		{Type: "Booking Amount", Amount: "5000.00", Verified: "", PaymentDate: "22-Sep-2026"},
		{Type: "Part 1", Amount: "20000.00", Verified: "Yes"},
	})
	for _, want := range []string{"Learner &lt;One&gt;", "learner@example.com", "Asha - bda@example.com", "Booking Amount", "₹5000", "22-Sep-2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("body is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "₹20000") {
		t.Error("verified payments should not be listed")
	}
}
