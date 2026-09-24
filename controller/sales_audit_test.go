package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"auditApp/config"
	"auditApp/models"
	"auditApp/routes"
	"auditApp/worker"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// These tests run the real routes against a throwaway MongoDB database, created and dropped per
// test. Set TEST_MONGO_URI (e.g. mongodb://localhost:27017) to run them; they skip otherwise.

const program = "test-program"

var now = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

// useTestDB points config.MongoDB at a fresh database and fixes the clock and mail queue.
func useTestDB(t *testing.T) *[]string {
	t.Helper()
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI not set")
	}
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connecting to %s: %v", uri, err)
	}
	config.MongoDB = client.Database(fmt.Sprintf("audit_app_test_%d", time.Now().UnixNano()))
	config.SalesAuditProgram = program
	config.AccountsEmail = "accounts@example.com"
	worker.Now = func() time.Time { return now }
	queued := &[]string{}
	worker.EnqueueSalesAuditMail = func(program, alertID string) error {
		*queued = append(*queued, alertID)
		return nil
	}
	t.Cleanup(func() {
		config.MongoDB.Drop(context.Background())
		client.Disconnect(context.Background())
		worker.Now = time.Now
	})
	return queued
}

// insert stores records as they appear in Mongo: Zoho records with deleted:false, feature
// documents as they are.
func insert[T any](t *testing.T, collection string, zoho bool, records ...T) {
	t.Helper()
	for _, record := range records {
		raw, err := bson.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		document := bson.M{}
		if err := bson.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		if zoho {
			document["deleted"] = false
		}
		if _, err := config.MongoDB.Collection(collection).InsertOne(context.Background(), document); err != nil {
			t.Fatal(err)
		}
	}
}

func find[T any](t *testing.T, collection string) []T {
	t.Helper()
	cursor, err := config.MongoDB.Collection(collection).Find(context.Background(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	results := []T{}
	if err := cursor.All(context.Background(), &results); err != nil {
		t.Fatal(err)
	}
	return results
}

// seedFixture: L1 is overdue with an unverified down payment; L2 is a fresh EMI lead with
// everything verified; L3 is in another stage (not listed, but reachable by id).
func seedFixture(t *testing.T) {
	t.Helper()
	insert(t, models.ZohoLeadsCollection, true,
		models.ZohoLead{
			ID: "L1", ZenID: "Z1", Stage: "Audit", StudentFullName: "Learner One", Email: "one@example.com",
			Course: "Course A", CourseValue: "75000", PaymentType: "Direct - Partial Payment",
			PartialSplitUpCategory: "40-30-30", SaleOwner: " Owner One - owner1@example.com",
			SaleOwnerManager: "Manager A - managera@example.com", AddedTime: "20-Sep-2026 10:00:00",
			DiscountGiven: "5000",
		},
		models.ZohoLead{
			ID: "L2", ZenID: "Z2", Stage: "Audit", StudentFullName: "Learner Two", Email: "two@example.com",
			Course: "Course B", CourseValue: "18000", PaymentType: "EMI - 10 Month", EMIStatus: "Disbursed",
			SaleOwner: "Owner Two - owner2@example.com", AddedTime: "24-Sep-2026 15:00:00",
			ConfirmationCallLink: "https://example.com/cc/2", ConfirmationCallAddedDateTime: "24-Sep-2026 16:00:00",
		},
		models.ZohoLead{ID: "L3", ZenID: "Z3", Stage: "Zen Student Program", StudentFullName: "Learner Three"},
	)
	insert(t, models.ZohoPaymentsCollection, true,
		models.ZohoPayment{ID: "P1", AllEnrolment: "Z1", Type: models.CreditBookingAmount, Amount: "999", Verified: "", PaymentDate: "2026-09-20", AddedTime: "20-Sep-2026 10:05:00"},
		models.ZohoPayment{ID: "P2", AllEnrolment: "Z1", Type: models.CreditPart1, Amount: "30000", Verified: "Yes", AddedTime: "21-Sep-2026 10:00:00", VerifiedOn: "2026-09-22T04:00:00Z"},
		models.ZohoPayment{ID: "P3", ZenID: "Z2", Type: models.CreditBookingAmount, Amount: "999", Verified: "Yes", AddedTime: "24-Sep-2026 15:10:00"},
		models.ZohoPayment{ID: "P4", AllEnrolment: "Z9", Type: models.CreditPart1, Amount: "1", AddedTime: "24-Sep-2026 15:10:00"},
	)
	insert(t, models.ZohoEmiCollection, true, models.ZohoEmi{
		ID: "E1", ZenID: "Z2", EMIVendor: "Loan Partner", LoanAmount: "18000.00", FirstEMI: "1800.00",
		ROI: "0", TenorInMonths: "10", EMIStatus: "Disbursed",
	})
	insert(t, models.ZohoPartialRemindersCollection, true,
		models.ZohoPartialReminder{StudentID: "L1", Amount: "22500.00", DueDate: "20-Nov-2026"},
		models.ZohoPartialReminder{StudentID: "L1", Amount: "30000.00", DueDate: "20-Oct-2026"},
	)
	insert(t, models.ZohoDiscountsCollection, true, models.ZohoDiscount{
		LearnerEmail: "ONE@example.com", Status: "Approved", DiscountValue: "5000", AddedTime: "19-Sep-2026 10:00:00",
	})
}

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	routes.SetupRoutes(engine, &redis.Pool{}, "test")
	return engine
}

type envelope struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func call(t *testing.T, engine *gin.Engine, method, path string, body any) (int, envelope) {
	t.Helper()
	reader := bytes.NewReader(nil)
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Authorization", "user-hash")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	var result envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("%s %s: invalid JSON %q", method, path, recorder.Body.String())
	}
	return recorder.Code, result
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	return value
}

func expect(t *testing.T, gotCode int, got envelope, wantCode int) {
	t.Helper()
	if gotCode != wantCode {
		t.Fatalf("status %d (%s), want %d", gotCode, got.Message, wantCode)
	}
	wantStatus := "success"
	if wantCode != http.StatusOK {
		wantStatus = "error"
	}
	if got.Status != wantStatus {
		t.Fatalf("body status %q, want %q", got.Status, wantStatus)
	}
}

func TestSalesAuditRequiresAuthorization(t *testing.T) {
	useTestDB(t)
	recorder := httptest.NewRecorder()
	newRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sales-audit/leads", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", recorder.Code)
	}
}

func TestGetLeads(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	insert(t, models.AlertsCollection, false,
		models.Alert{ID: "a1", Program: program, LeadID: "L1", Kind: models.AlertEscalation, Mail: models.Mail{Trigger: models.TriggerAuto, SentAt: now.Add(-10 * time.Minute).Unix()}},
		models.Alert{ID: "a2", Program: "other", LeadID: "L1", Kind: models.AlertEscalation, Mail: models.Mail{Trigger: models.TriggerAuto, SentAt: now.Unix()}},
	)
	code, body := call(t, newRouter(), http.MethodGet, "/sales-audit/leads", nil)
	expect(t, code, body, http.StatusOK)

	result := decode[models.LeadsResponse](t, body.Data)
	if len(result.Leads) != 2 {
		t.Fatalf("got %d leads, want the 2 in the Audit stage", len(result.Leads))
	}
	if result.MailsSentThisSweep != 1 {
		t.Errorf("mailsSentThisSweep = %d, want 1 (other program ignored)", result.MailsSentThisSweep)
	}
	first := result.Leads[0]
	if first.SaleOwner != "Owner One - owner1@example.com" || first.Credits.BookingAmount == nil ||
		first.Credits.Part1.Verified != "Yes" || first.Credits.RemainingBalance != nil ||
		first.Escalation == nil || first.SapEnteredAt == 0 {
		t.Errorf("lead = %+v", first)
	}
	if result.Leads[1].CcUploadedAt == nil || result.Leads[1].EmiDetails == nil {
		t.Errorf("lead 2 = %+v", result.Leads[1])
	}
}

func TestLeadSummaries(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	code, body := call(t, newRouter(), http.MethodGet, "/sales-audit/leads/summaries", nil)
	expect(t, code, body, http.StatusOK)
	if summaries := decode[[]models.LeadSummary](t, body.Data); len(summaries) != 2 || summaries[0].ID != "L1" {
		t.Errorf("summaries = %+v", summaries)
	}
}

func TestSendReminder(t *testing.T) {
	queued := useTestDB(t)
	seedFixture(t)
	engine := newRouter()

	code, body := call(t, engine, http.MethodPost, "/sales-audit/leads/L1/send-reminder", nil)
	expect(t, code, body, http.StatusOK)
	mail := decode[models.Mail](t, body.Data)
	if mail.Trigger != models.TriggerManual || len(mail.To) != 2 || mail.To[1] != "accounts@example.com" {
		t.Errorf("mail = %+v", mail)
	}
	alerts := find[models.Alert](t, models.AlertsCollection)
	if len(*queued) != 1 || len(alerts) != 1 || alerts[0].Created.By != "user-hash" || alerts[0].Program != program {
		t.Errorf("queued %v, logged %+v", *queued, alerts)
	}

	code, body = call(t, engine, http.MethodPost, "/sales-audit/leads/L2/send-reminder", nil)
	expect(t, code, body, http.StatusBadRequest)
	code, body = call(t, engine, http.MethodPost, "/sales-audit/leads/nope/send-reminder", nil)
	expect(t, code, body, http.StatusNotFound)
}

func TestCcResponse(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	engine := newRouter()

	code, body := call(t, engine, http.MethodPost, "/sales-audit/leads/L1/cc-response", map[string]string{"response": "maybe"})
	expect(t, code, body, http.StatusBadRequest)
	code, body = call(t, engine, http.MethodPost, "/sales-audit/leads/L2/cc-response", map[string]string{"response": models.CcMailNotSent})
	expect(t, code, body, http.StatusBadRequest) // CC already uploaded

	code, body = call(t, engine, http.MethodPost, "/sales-audit/leads/L1/cc-response", map[string]string{"response": models.CcMailNotSent})
	expect(t, code, body, http.StatusOK)
	response := decode[models.CcResponse](t, body.Data)
	if response.Alert == nil || response.Alert.To[0] != "owner1@example.com" {
		t.Errorf("mailNotSent should alert the BDA: %+v", response)
	}

	code, body = call(t, engine, http.MethodPost, "/sales-audit/leads/L1/cc-response", map[string]string{"response": models.CcMailSentAwaitingAck})
	expect(t, code, body, http.StatusOK)
	stored := find[models.CcResponse](t, models.CcResponsesCollection)
	if len(stored) != 1 || stored[0].Response != models.CcMailSentAwaitingAck || stored[0].Alert != nil {
		t.Errorf("responses = %+v, want one updated record", stored)
	}
}

func TestLeadAudit(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	insert(t, models.RechecksCollection, false, models.Recheck{ID: "r1", Program: program, LeadID: "L1", Status: models.RecheckOpen})
	engine := newRouter()

	code, body := call(t, engine, http.MethodGet, "/sales-audit/leads/L1/audit", nil)
	expect(t, code, body, http.StatusOK)
	audit := decode[models.LeadAudit](t, body.Data)
	if audit.PaymentMode != "partial" || audit.Sources.Cc != nil || audit.Sources.Vendor != nil || len(audit.Rechecks) != 1 {
		t.Errorf("audit = %+v", audit)
	}
	installments := audit.Sources.Zoho.Payment.Installments
	// 40-30-30: the initial payment first, then the two reminders sorted by due date.
	if len(installments) != 3 || installments[0].Amount != "₹30000" ||
		installments[1].DueDate != "20-Oct-2026" || installments[2].DueDate != "20-Nov-2026" {
		t.Errorf("installments = %+v", installments)
	}
	if audit.Discount == nil || audit.Discount.Status != "Approved" {
		t.Errorf("discount = %+v", audit.Discount)
	}

	code, body = call(t, engine, http.MethodGet, "/sales-audit/leads/L2/audit", nil)
	expect(t, code, body, http.StatusOK)
	emiAudit := decode[models.LeadAudit](t, body.Data)
	if emiAudit.VendorName != "Loan Partner" || emiAudit.Sources.Vendor == nil ||
		emiAudit.Sources.Vendor.Payment.Emi.Tenure != "10 months" || emiAudit.Sources.Zoho.Payment.Emi.Tenure != "10 months" {
		t.Errorf("emi audit = %+v", emiAudit)
	}

	code, body = call(t, engine, http.MethodGet, "/sales-audit/leads/nope/audit", nil)
	expect(t, code, body, http.StatusNotFound)
}

func TestLeadAuditUsesCcExtract(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	insert(t, models.CcExtractsCollection, false, models.CcExtract{
		ID: "x1", Program: program, LeadID: "L2", PointsCovered: []string{"fee"},
		Scraped: models.Source{Personal: &models.Personal{LearnerName: "Learner Two"}},
	})
	engine := newRouter()

	code, body := call(t, engine, http.MethodGet, "/sales-audit/leads/L2/audit", nil)
	expect(t, code, body, http.StatusOK)
	audit := decode[models.LeadAudit](t, body.Data)
	if audit.Sources.Cc == nil || len(audit.PointsCovered) != 1 {
		t.Errorf("audit = %+v", audit)
	}

	code, body = call(t, engine, http.MethodGet, "/sales-audit/students/L2/cc-verification", nil)
	expect(t, code, body, http.StatusOK)
	code, body = call(t, engine, http.MethodGet, "/sales-audit/students/L1/cc-verification", nil)
	expect(t, code, body, http.StatusNotFound)
}

func TestMarkAudited(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	engine := newRouter()

	code, body := call(t, engine, http.MethodPost, "/sales-audit/leads/L1/mark-audited", map[string]string{"overrideReason": "  CC shared on WhatsApp "})
	expect(t, code, body, http.StatusOK)
	audit := decode[models.Audit](t, body.Data)
	if audit.OverrideReason != "CC shared on WhatsApp" || audit.AuditedAt != now.Unix() {
		t.Errorf("audit = %+v", audit)
	}

	code, body = call(t, engine, http.MethodPost, "/sales-audit/leads/L1/mark-audited", nil)
	expect(t, code, body, http.StatusBadRequest)

	code, body = call(t, engine, http.MethodGet, "/sales-audit/leads", nil)
	expect(t, code, body, http.StatusOK)
	if leads := decode[models.LeadsResponse](t, body.Data).Leads; leads[0].Audit == nil {
		t.Error("audited lead should carry its audit (Awaiting)")
	}
}

func TestRechecks(t *testing.T) {
	queued := useTestDB(t)
	seedFixture(t)
	engine := newRouter()

	code, body := call(t, engine, http.MethodPost, "/sales-audit/rechecks", map[string]string{"leadId": "L1", "category": "nope", "notes": "x"})
	expect(t, code, body, http.StatusBadRequest)
	code, body = call(t, engine, http.MethodPost, "/sales-audit/rechecks", map[string]string{"leadId": "L1", "category": "payment", "notes": " "})
	expect(t, code, body, http.StatusBadRequest)
	code, body = call(t, engine, http.MethodPost, "/sales-audit/rechecks", map[string]string{"leadId": "nope", "category": "payment", "notes": "x"})
	expect(t, code, body, http.StatusNotFound)

	code, body = call(t, engine, http.MethodPost, "/sales-audit/rechecks", map[string]string{"leadId": "L1", "category": "payment", "notes": "Split differs"})
	expect(t, code, body, http.StatusOK)
	recheck := decode[models.Recheck](t, body.Data)
	if recheck.Status != models.RecheckOpen || recheck.Alert.Subject != "Recheck raised (Payment): Learner One" ||
		len(recheck.Alert.To) != 2 || len(*queued) != 1 {
		t.Errorf("recheck = %+v", recheck)
	}

	code, body = call(t, engine, http.MethodGet, "/sales-audit/rechecks", nil)
	expect(t, code, body, http.StatusOK)
	if list := decode[[]models.Recheck](t, body.Data); len(list) != 1 {
		t.Errorf("rechecks = %+v", list)
	}

	code, body = call(t, engine, http.MethodPost, "/sales-audit/rechecks/"+recheck.ID+"/resolve", nil)
	expect(t, code, body, http.StatusOK)
	if resolved := decode[models.Recheck](t, body.Data); resolved.ResolvedAt == nil || resolved.Status != models.RecheckResolved {
		t.Errorf("resolved = %+v", resolved)
	}
	code, body = call(t, engine, http.MethodPost, "/sales-audit/rechecks/"+recheck.ID+"/resolve", nil)
	expect(t, code, body, http.StatusBadRequest)
	code, body = call(t, engine, http.MethodPost, "/sales-audit/rechecks/nope/resolve", nil)
	expect(t, code, body, http.StatusNotFound)
}

func TestAuditHistory(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	insert(t, models.AlertsCollection, false,
		models.Alert{ID: "old", Program: program, LeadID: "L1", Kind: models.AlertEscalation, Mail: models.Mail{SentAt: now.AddDate(0, 0, -90).Unix()}},
		models.Alert{ID: "new", Program: program, LeadID: "L1", Kind: models.AlertEscalation, Mail: models.Mail{SentAt: now.AddDate(0, 0, -1).Unix()}},
	)
	insert(t, models.RechecksCollection, false,
		models.Recheck{ID: "r1", Program: program, LeadID: "L1", RaisedAt: now.AddDate(0, 0, -100).Unix(), Status: models.RecheckOpen})
	engine := newRouter()

	code, body := call(t, engine, http.MethodGet, "/sales-audit/audit-history", nil)
	expect(t, code, body, http.StatusOK)
	history := decode[models.AuditHistory](t, body.Data)
	if len(history.Alerts) != 1 || history.Alerts[0].ID != "new" || len(history.Rechecks) != 1 {
		t.Errorf("history alerts=%+v rechecks=%+v", history.Alerts, history.Rechecks)
	}
	if len(history.Payments) != 3 || history.Payments[0].LeadID == "" {
		t.Errorf("payments = %+v, want the 3 Audit-stage payments with lead ids", history.Payments)
	}

	code, body = call(t, engine, http.MethodGet, "/sales-audit/audit-history?from=abc", nil)
	expect(t, code, body, http.StatusBadRequest)
}

func TestStudentPages(t *testing.T) {
	useTestDB(t)
	seedFixture(t)
	engine := newRouter()

	code, body := call(t, engine, http.MethodGet, "/sales-audit/students/L3", nil)
	expect(t, code, body, http.StatusOK) // any stage is reachable by id
	code, body = call(t, engine, http.MethodGet, "/sales-audit/students/nope", nil)
	expect(t, code, body, http.StatusNotFound)

	code, body = call(t, engine, http.MethodGet, "/sales-audit/students/L1/payments", nil)
	expect(t, code, body, http.StatusOK)
	payments := decode[[]models.Payment](t, body.Data)
	if len(payments) != 2 || payments[0].ID != "P1" || payments[0].PaymentDate != "20-Sep-2026" ||
		payments[0].LeadID != "L1" || payments[1].VerifiedAt == nil {
		t.Errorf("payments = %+v", payments)
	}
}
