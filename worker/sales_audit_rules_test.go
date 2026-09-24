package worker

import (
	"testing"
	"time"

	"auditApp/models"
)

func TestParseZohoTime(t *testing.T) {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	cases := map[string]time.Time{
		"15-Sep-2026 15:22:19":     time.Date(2026, 9, 15, 15, 22, 19, 0, ist),
		"19-Sep-2026":              time.Date(2026, 9, 19, 0, 0, 0, 0, ist),
		"2026-09-24":               time.Date(2026, 9, 24, 0, 0, 0, 0, ist),
		"2026-09-24T04:01:51.725Z": time.Date(2026, 9, 24, 4, 1, 51, 0, time.UTC),
		"08/28/26 06:37:17 PM":     time.Date(2026, 8, 28, 18, 37, 17, 0, ist),
	}
	for input, want := range cases {
		got, ok := ParseZohoTime(input)
		if !ok || got != want.Unix() {
			t.Errorf("ParseZohoTime(%q) = %d, %v; want %d", input, got, ok, want.Unix())
		}
	}
	for _, input := range []string{"", "  ", "not a date"} {
		if _, ok := ParseZohoTime(input); ok {
			t.Errorf("ParseZohoTime(%q) should fail", input)
		}
	}
}

func TestFormatting(t *testing.T) {
	checks := []struct{ got, want string }{
		{DisplayDate("2026-09-24"), "24-Sep-2026"},
		{DisplayDate("02-Jun-2025"), "02-Jun-2025"},
		{DisplayDate("soon"), "soon"},
		{Rupees("18000.00"), "₹18000"},
		{Rupees("4500"), "₹4500"},
		{Rupees("7627.12"), "₹7627.12"},
		{Rupees(""), ""},
		{percent("5.00"), "5%"},
		{percent("24.50"), "24.5%"},
		{ContactEmail(" Sales Owner One - owner1@example.com"), "owner1@example.com"},
		{ContactName(" Sales Owner One - owner1@example.com"), "Sales Owner One"},
		{ContactEmail(""), ""},
	}
	for index, check := range checks {
		if check.got != check.want {
			t.Errorf("check %d: got %q, want %q", index, check.got, check.want)
		}
	}
	if id := NewID(); len(id) != 36 || id[14] != '4' {
		t.Errorf("NewID() = %q, want a v4 UUID", id)
	}
}

func TestPaymentMode(t *testing.T) {
	cases := map[string]string{
		"EMI + Partial Payment":    "emiPartial",
		"EMI - 10 Month":           "emi",
		"Direct - Partial Payment": "partial",
		"Subscription":             "subscription",
		"Direct - Full Payment":    "full",
		"":                         "full",
	}
	for input, want := range cases {
		if got := PaymentMode(input); got != want {
			t.Errorf("PaymentMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLatestCreditsPicksMostRecentPerType(t *testing.T) {
	credits := LatestCredits([]models.ZohoPayment{
		{Type: models.CreditPart1, Amount: "500", Verified: "Mismatch", AddedTime: "20-Sep-2026 10:00:00"},
		{Type: models.CreditPart1, Amount: "400", Verified: "", AddedTime: "18-Sep-2026 10:00:00"},
		{Type: models.CreditBookingAmount, Amount: "999", Verified: "Yes", PaymentDate: "2026-09-15", AddedTime: "15-Sep-2026 10:00:00"},
	})
	if credits.Part1 == nil || credits.Part1.Amount != "500" || credits.Part1.Verified != "Mismatch" {
		t.Errorf("Part1 = %+v, want the 20-Sep record", credits.Part1)
	}
	if credits.BookingAmount == nil || credits.BookingAmount.PaymentDate != "15-Sep-2026" {
		t.Errorf("BookingAmount = %+v", credits.BookingAmount)
	}
	if credits.RemainingBalance != nil {
		t.Errorf("RemainingBalance = %+v, want nil (unpaid)", credits.RemainingBalance)
	}
}

func TestEscalationRules(t *testing.T) {
	const now = int64(2_000_000_000)
	unverified := &models.Credit{Amount: "999", Verified: ""}
	verified := &models.Credit{Amount: "999", Verified: "Yes"}
	overdue := models.Lead{SapEnteredAt: now - 25*3600, Credits: models.Credits{BookingAmount: unverified}}

	if !EscalationDue(overdue, now) {
		t.Error("unverified payment over 24h in SAP should be due")
	}
	fresh := overdue
	fresh.SapEnteredAt = now - 2*3600
	if EscalationDue(fresh, now) {
		t.Error("under 24h in SAP should not be due")
	}
	paidNothing := overdue
	paidNothing.Credits = models.Credits{}
	if NeedsEscalation(paidNothing) {
		t.Error("a lead that paid nothing should not need escalation")
	}
	clean := overdue
	clean.Credits = models.Credits{BookingAmount: verified}
	if NeedsEscalation(clean) {
		t.Error("verified payments should not need escalation")
	}
	audited := overdue
	audited.Audit = &models.Audit{AuditedAt: now}
	if NeedsEscalation(audited) {
		t.Error("an audited lead should not need escalation")
	}
	mailedRecently := overdue
	mailedRecently.Escalation = &models.Mail{SentAt: now - 3600}
	if EscalationDue(mailedRecently, now) {
		t.Error("mailed in the last 24h should not be due again")
	}
	mailedLongAgo := overdue
	mailedLongAgo.Escalation = &models.Mail{SentAt: now - 25*3600}
	if !EscalationDue(mailedLongAgo, now) {
		t.Error("mailed over 24h ago should be due again")
	}
}

func TestRecheckReminderDue(t *testing.T) {
	const now = int64(2_000_000_000)
	open := models.Recheck{Status: models.RecheckOpen, RaisedAt: now - 25*3600}
	if !RecheckReminderDue(open, now) {
		t.Error("open over 24h should be due")
	}
	reminded := open
	reminded.LastReminder = &models.Mail{SentAt: now - 3600}
	if RecheckReminderDue(reminded, now) {
		t.Error("reminded an hour ago should not be due")
	}
	resolved := open
	resolved.Status = models.RecheckResolved
	if RecheckReminderDue(resolved, now) {
		t.Error("resolved rechecks are never due")
	}
}

func TestSources(t *testing.T) {
	zoho := models.ZohoLead{
		ID: "L1", StudentFullName: "Test Learner", Course: "Course A", CourseValue: "9000",
		PaymentType: "EMI - 10 Month", EMIStatus: "Disbursed", PreferredLanguage: "English",
		AssignedBatchStartDate: "09-Jul-2025",
	}
	lead := BuildLead(zoho, nil, nil, LeadState{})
	source := ZohoSource(lead, zoho, nil)
	if source.Payment.TotalFee != "₹9000" || source.Payment.Emi.Tenure != "10 months" ||
		source.CourseDetails.StartDate != "09-Jul-2025" || source.CourseDetails.Medium != "English" {
		t.Errorf("zoho source = %+v / %+v / %+v", source.Payment, source.Payment.Emi, source.CourseDetails)
	}

	vendor := VendorSource(&models.ZohoEmi{LoanAmount: "18000.00", FirstEMI: "1000.00", ROI: "5.00", TenorInMonths: "10"})
	emi := vendor.Payment.Emi
	if emi.LoanAmount != "₹18000" || emi.MonthlyEmi != "₹1000" || emi.Roi != "5%" || emi.Tenure != "10 months" {
		t.Errorf("vendor emi = %+v", emi)
	}
	if VendorSource(nil) != nil {
		t.Error("no EMI record means no vendor source")
	}

}

func TestInstallmentsAlignWithSplit(t *testing.T) {
	part1 := []models.ZohoPayment{{Type: models.CreditPart1, Amount: "30000.00", PaymentDate: "2025-04-14"}}
	partial := BuildLead(models.ZohoLead{PartialSplitUpCategory: "40-30-30"}, part1, nil, LeadState{})
	schedule := []Scheduled{{Amount: "22000.00", DueDate: "14-May-2025"}, {Amount: "22001.00", DueDate: "14-Jun-2025"}}

	got := ZohoSource(partial, models.ZohoLead{}, schedule).Payment.Installments
	want := []models.Installment{
		{Percentage: "40%", DueDate: "14-Apr-2025", Amount: "₹30000"},
		{Percentage: "30%", DueDate: "14-May-2025", Amount: "₹22000"},
		{Percentage: "30%", DueDate: "14-Jun-2025", Amount: "₹22001"},
	}
	if len(got) != len(want) {
		t.Fatalf("installments = %+v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("installment %d = %+v, want %+v", index, got[index], want[index])
		}
	}

	full := BuildLead(models.ZohoLead{PartialSplitUpCategory: "50-50"}, nil, nil, LeadState{})
	if got := ZohoSource(full, models.ZohoLead{}, schedule).Payment.Installments; got[0].Amount != "₹22000" || got[0].Percentage != "50%" {
		t.Errorf("a complete schedule should start at the first slot: %+v", got)
	}
	if got := ZohoSource(BuildLead(models.ZohoLead{}, nil, nil, LeadState{}), models.ZohoLead{}, nil).Payment.Installments; got != nil {
		t.Errorf("no split and no schedule should give no installments: %+v", got)
	}
}
