package core_test

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
)

func auditor(email, region string) models.Member {
	return models.Member{Email: email, Role: models.RoleAuditor, Region: region, Available: true}
}

func leadsIn(region string, count int, prefix string) []models.Lead {
	leads := []models.Lead{}
	for index := range count {
		leads = append(leads, models.Lead{ID: prefix + string(rune('A'+index%26)) + string(rune('a'+index/26)), Region: region})
	}
	return leads
}

func TestAssignLeadsSplitsByRegionAndBalances(t *testing.T) {
	leads := append(leadsIn(models.RegionNorth, 20, "n"), leadsIn(models.RegionSouth, 80, "s")...)
	auditors := []models.Member{
		auditor("north@example.com", models.RegionNorth),
		auditor("south1@example.com", models.RegionSouth),
		auditor("south2@example.com", models.RegionSouth),
	}
	plan := core.AssignLeads(leads, auditors, map[string]int{}, rand.New(rand.NewSource(1)))

	counts := map[string]int{}
	for _, email := range plan {
		counts[email]++
	}
	if len(plan) != 100 || counts["north@example.com"] != 20 || counts["south1@example.com"] != 40 || counts["south2@example.com"] != 40 {
		t.Fatalf("want 20 / 40 / 40, got %v (assigned %d)", counts, len(plan))
	}
	for _, lead := range leads {
		if lead.Region == models.RegionNorth && plan[lead.ID] != "north@example.com" {
			t.Fatalf("north lead %s went to %s", lead.ID, plan[lead.ID])
		}
	}
}

func TestAssignLeadsSkipsUnavailableAndUncoveredRegions(t *testing.T) {
	away := auditor("away@example.com", models.RegionSouth)
	away.Available = false
	plan := core.AssignLeads(
		append(leadsIn(models.RegionSouth, 4, "s"), models.Lead{ID: "x", Region: ""}),
		[]models.Member{away, auditor("busy@example.com", models.RegionSouth), auditor("idle@example.com", models.RegionSouth)},
		map[string]int{"busy@example.com": 10},
		rand.New(rand.NewSource(1)),
	)
	for id, email := range plan {
		if email != "idle@example.com" {
			t.Fatalf("lead %s went to %s; the idle auditor should take all four (busy has 10 open)", id, email)
		}
	}
	if len(plan) != 4 {
		t.Fatalf("the lead with no region should stay unassigned, got %d assigned", len(plan))
	}
}

func TestPresetRange(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, core.IST) // a Friday
	lastMonth, _ := core.PresetRange(core.PresetLastMonth, now)
	if time.Unix(lastMonth.From, 0).In(core.IST).Format("2006-01-02") != "2026-08-01" ||
		time.Unix(lastMonth.To, 0).In(core.IST).Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("last month: %v", lastMonth)
	}
	lastWeek, _ := core.PresetRange(core.PresetLastWeek, now)
	if time.Unix(lastWeek.From, 0).In(core.IST).Format("2006-01-02") != "2026-09-14" {
		t.Fatalf("last week should start on Monday 14 Sep, got %v", time.Unix(lastWeek.From, 0).In(core.IST))
	}
	if _, _, err := core.ParseRange("someday", "", "", now); err == nil {
		t.Fatal("an unknown preset should fail")
	}
}

func TestPaymentShortfall(t *testing.T) {
	verified := func(amount float64) models.PaymentRecord {
		return models.PaymentRecord{Type: models.CreditPart1, Amount: amount, Verified: "Yes"}
	}
	cases := []struct {
		name    string
		payment models.LeadPayment
		ready   bool
	}{
		{"nothing paid", models.LeadPayment{PaymentType: "Direct - Partial Payment"}, false},
		{"unverified", models.LeadPayment{PaymentType: "Direct - Partial Payment", Records: []models.PaymentRecord{{Amount: 999}}}, false},
		{"partial, all verified", models.LeadPayment{PaymentType: "Direct - Partial Payment", CourseFee: 70000, Records: []models.PaymentRecord{verified(999)}}, true},
		{"full, short", models.LeadPayment{PaymentType: "Direct - Full Payment", CourseFee: 70000, Records: []models.PaymentRecord{verified(69000)}}, false},
		{"full, paid", models.LeadPayment{PaymentType: "Direct - Full Payment", CourseFee: 70000, Records: []models.PaymentRecord{verified(70000)}}, true},
		{"emi under 40%", models.LeadPayment{PaymentType: "EMI - 12 Month", CourseFee: 100000, Records: []models.PaymentRecord{verified(39000)}}, false},
		{"emi 40%", models.LeadPayment{PaymentType: "EMI - 12 Month", CourseFee: 100000, Records: []models.PaymentRecord{verified(40000)}}, true},
		{"subscription short", models.LeadPayment{PaymentType: "Subscription", Records: []models.PaymentRecord{verified(14999)}}, false},
		{"subscription ok", models.LeadPayment{PaymentType: "Subscription", Records: []models.PaymentRecord{verified(15000)}}, true},
	}
	for _, tc := range cases {
		if got := core.WithPaymentReadiness(tc.payment).Ready; got != tc.ready {
			t.Errorf("%s: ready = %v, want %v (%s)", tc.name, got, tc.ready, core.PaymentShortfall(tc.payment))
		}
	}
}

func TestStatusMachine(t *testing.T) {
	audit := core.ReconcileStatus(models.AuditState{Status: models.AuditUnassigned}, true, models.RecheckSummary{})
	if audit.Status != models.AuditPending {
		t.Fatalf("assigned lead should be pending, got %s", audit.Status)
	}
	audit = core.ReconcileStatus(audit, true, models.RecheckSummary{Open: 1})
	if audit.Status != models.AuditRecheckOpen {
		t.Fatalf("open recheck: got %s", audit.Status)
	}
	audit = core.ReconcileStatus(audit, true, models.RecheckSummary{})
	if audit.Status != models.AuditRecheckClosed {
		t.Fatalf("closed recheck: got %s", audit.Status)
	}
	completed := core.ReconcileStatus(models.AuditState{Status: models.AuditCompleted}, true, models.RecheckSummary{Open: 1})
	if completed.Status != models.AuditCompleted {
		t.Fatal("a completed audit stays completed")
	}
}

func TestValidateChecklist(t *testing.T) {
	ticked := []models.ChecklistItem{}
	for _, item := range core.Checklist() {
		ticked = append(ticked, models.ChecklistItem{Key: item.Key, Checked: true})
	}
	if _, err := core.ValidateChecklist(ticked); err != nil {
		t.Fatalf("all ticked: %v", err)
	}
	ticked[2].Checked = false
	if _, err := core.ValidateChecklist(ticked); err == nil {
		t.Fatal("an unticked item should fail")
	}
}

func TestScopes(t *testing.T) {
	lead := models.Lead{BdaEmail: "bda1@example.com", BdmEmail: "bdm@example.com", Assignment: &models.Assignment{AuditorEmail: "aud@example.com"}}
	bdm := models.Member{Email: "other-bdm@example.com", Role: models.RoleBdm}
	if core.LeadInScope(core.ScopeFor(bdm, []string{"bda2@example.com"}, false), lead) {
		t.Fatal("a BDM must not see another team's lead")
	}
	if !core.LeadInScope(core.ScopeFor(bdm, []string{"bda1@example.com"}, false), lead) {
		t.Fatal("a BDM sees their BDAs' leads")
	}
	if core.LeadInScope(core.ScopeFor(models.Member{Email: "bda2@example.com", Role: models.RoleBda}, nil, false), lead) {
		t.Fatal("a BDA sees only their own leads")
	}
	if !core.LeadInScope(core.ScopeFor(models.Member{Email: "x@example.com", Role: models.RoleAuditor}, nil, false), lead) {
		t.Fatal("auditors see every lead in All Leads")
	}
	if core.LeadInScope(core.ScopeFor(models.Member{Email: "x@example.com", Role: models.RoleAuditor}, nil, true), lead) {
		t.Fatal("My Leads shows only the auditor's own")
	}
}

const zohoSample = `{"result":[{"zenId":"900001","superleapId":"SL1","name":"Test  Learner","email":"Learner@Example.com",
"phone":"+910000000001","salesTeam":"South","saleOwner":"bda@example.com","saleOwnerManager":"bdm@example.com",
"paymenttype":"Direct - Partial Payment","courseFee":70000,"auditStatus":"Recheck Pending","auditCoordinator":"aud@example.com",
"confirmationCall":"https://drive.google.com/file/d/abc123/view?usp=sharing","terms&conditions":"Yes",
"financialDetails":[{"recordId":1,"type":"Credit_Booking_Amount","amount":999,"verified":"Yes"}],
"recheckDetails":[{"SRID":"SR-1","ticketStatus":"Open","pendingList":["Missed Points on CC"],"recheckDate":"2026-07-02 15:51:26.0","recheckattempt":1}],
"batchData":{"batchName":"B1"},"admissionDetails":{"12thPercentage":80}}]}`

func TestZohoMapping(t *testing.T) {
	var envelope struct {
		Result []models.ZohoLearner `json:"result"`
	}
	if err := json.Unmarshal([]byte(zohoSample), &envelope); err != nil {
		t.Fatal(err)
	}
	learner := envelope.Result[0]
	lead := core.NewLeadFromZoho("p", learner, 1000)
	if lead.Region != models.RegionSouth || lead.Personal.Name != "Test Learner" || lead.Personal.Email != "learner@example.com" {
		t.Fatalf("personal/region: %+v", lead)
	}
	if lead.Assignment == nil || lead.Assignment.AuditorEmail != "aud@example.com" || lead.Audit.Status != models.AuditRecheckOpen {
		t.Fatalf("workflow from Zoho: %+v %+v", lead.Assignment, lead.Audit)
	}
	if lead.Cc.Status != models.CcUpdated || lead.Cc.Type != models.CcTypePdf || !lead.Payment.Ready || !lead.TermsAccepted {
		t.Fatalf("cc/payment: %+v %+v", lead.Cc, lead.Payment)
	}
	if lead.Admission["12thPercentage"] != "80" {
		t.Fatalf("admission: %v", lead.Admission)
	}
	recheck, ok := core.RecheckFromZoho("p", lead, learner.RecheckDetails[0], 1000)
	if !ok || recheck.Category != models.CategoryMissedPointsInCc || recheck.Source != models.SourceZoho || recheck.RecheckNo != "SR-1" {
		t.Fatalf("recheck: %+v", recheck)
	}

	// A later import keeps the portal's workflow and flags the CC becoming available.
	existing := lead
	existing.Cc = models.LeadCc{Status: models.CcPending}
	existing.Audit.Status = models.AuditCompleted
	merged, became := core.MergeZoho(existing, learner, 2000)
	if !became || merged.Audit.Status != models.AuditCompleted || merged.Cc.UpdatedAt != 2000 {
		t.Fatalf("merge: became=%v %+v %+v", became, merged.Audit, merged.Cc)
	}
}

func TestCompareFlagsMismatches(t *testing.T) {
	lead := models.Lead{Personal: models.LeadPersonal{Name: "A B", Email: "a@example.com", Phone: "+910000000001"},
		Payment: models.LeadPayment{PaymentType: "Direct - Full Payment", CourseFee: 70000}}
	extract := models.CcExtract{Fields: []models.CcField{
		{Key: "name", Value: "a b"}, {Key: "email", Value: "a@example.com"}, {Key: "phone", Value: "0000000001"},
		{Key: "paymentType", Value: "Direct - Full Payment"}, {Key: "courseFee", Value: "₹75000"},
	}}
	rows, mismatches := core.Compare(lead, extract)
	byKey := map[string]models.ComparisonRow{}
	for _, row := range rows {
		byKey[row.Key] = row
	}
	if !byKey["name"].Match || !byKey["phone"].Match || byKey["courseFee"].Match {
		t.Fatalf("rows: %+v", rows)
	}
	if mismatches == 0 {
		t.Fatal("the fee mismatch and missing course fields should count")
	}
}
