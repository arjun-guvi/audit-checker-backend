package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"auditApp/config"
	"auditApp/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// These tests use a throwaway MongoDB database, created and dropped per test. Set TEST_MONGO_URI
// (e.g. mongodb://localhost:27017) to run them; they skip otherwise.

const testProgram = "test-program"

// useTestDB points config.MongoDB at a fresh database, sets the clock to *at and records queued
// mails.
func useTestDB(t *testing.T, at *time.Time) *[]string {
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
	config.AccountsEmail = "accounts@example.com"
	Now = func() time.Time { return *at }
	queued := &[]string{}
	enqueue := EnqueueSalesAuditMail
	EnqueueSalesAuditMail = func(program, alertID string) error {
		*queued = append(*queued, alertID)
		return nil
	}
	t.Cleanup(func() {
		config.MongoDB.Drop(context.Background())
		client.Disconnect(context.Background())
		Now = time.Now
		EnqueueSalesAuditMail = enqueue
	})
	return queued
}

// insertDocs stores records as they appear in Mongo; Zoho records get deleted:false.
func insertDocs[T any](t *testing.T, collection string, zoho bool, records ...T) {
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

func TestEscalationSweepMailsOncePer24h(t *testing.T) {
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	queued := useTestDB(t, &at)
	insertDocs(t, models.ZohoLeadsCollection, true,
		models.ZohoLead{ID: "L1", ZenID: "Z1", Stage: "Audit", StudentFullName: "Overdue", SaleOwner: "BDA - bda@example.com", SaleOwnerManager: "BDM - bdm@example.com", AddedTime: "20-Sep-2026 10:00:00"},
		models.ZohoLead{ID: "L2", ZenID: "Z2", Stage: "Audit", StudentFullName: "Fresh", AddedTime: "24-Sep-2026 17:00:00"},
		models.ZohoLead{ID: "L3", ZenID: "Z3", Stage: "Audit", StudentFullName: "Clean", AddedTime: "20-Sep-2026 10:00:00"},
	)
	insertDocs(t, models.ZohoPaymentsCollection, true,
		models.ZohoPayment{AllEnrolment: "Z1", Type: models.CreditBookingAmount, Verified: ""},
		models.ZohoPayment{AllEnrolment: "Z2", Type: models.CreditBookingAmount, Verified: "Mismatch"},
		models.ZohoPayment{AllEnrolment: "Z3", Type: models.CreditBookingAmount, Verified: "Yes"},
	)
	ctx := context.Background()

	if sent, err := RunEscalationSweep(ctx, testProgram); err != nil || sent != 1 {
		t.Fatalf("first sweep sent %d (%v), want 1", sent, err)
	}
	alerts, err := FindAlerts(ctx, testProgram, nil, "", 0)
	if err != nil || len(alerts) != 1 {
		t.Fatalf("alerts = %+v (%v)", alerts, err)
	}
	if alert := alerts[0]; alert.LeadID != "L1" || alert.Trigger != models.TriggerAuto ||
		len(alert.To) != 3 || alert.To[0] != "bda@example.com" || alert.To[1] != "bdm@example.com" ||
		alert.To[2] != "accounts@example.com" || len(*queued) != 1 {
		t.Errorf("alert = %+v", alert)
	}

	at = at.Add(time.Hour)
	if sent, _ := RunEscalationSweep(ctx, testProgram); sent != 0 {
		t.Errorf("sweep an hour later sent %d, want 0", sent)
	}
	at = at.Add(24 * time.Hour)
	if sent, _ := RunEscalationSweep(ctx, testProgram); sent != 2 {
		t.Errorf("sweep a day later sent %d, want 2 (L1 again, L2 now overdue)", sent)
	}
}

func TestRecheckReminderSweep(t *testing.T) {
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	useTestDB(t, &at)
	insertDocs(t, models.ZohoLeadsCollection, true, models.ZohoLead{
		ID: "L1", Stage: "Audit", StudentFullName: "Learner",
		SaleOwner: "BDA - bda@example.com", SaleOwnerManager: "BDM - bdm@example.com",
	})
	insertDocs(t, models.RechecksCollection, false,
		models.Recheck{ID: "old", Program: testProgram, LeadID: "L1", Status: models.RecheckOpen, RaisedAt: at.Add(-25 * time.Hour).Unix()},
		models.Recheck{ID: "new", Program: testProgram, LeadID: "L1", Status: models.RecheckOpen, RaisedAt: at.Add(-2 * time.Hour).Unix()},
		models.Recheck{ID: "done", Program: testProgram, LeadID: "L1", Status: models.RecheckResolved, RaisedAt: at.Add(-50 * time.Hour).Unix()},
	)
	ctx := context.Background()

	if sent, err := RunRecheckReminderSweep(ctx, testProgram); err != nil || sent != 1 {
		t.Fatalf("sent %d (%v), want 1", sent, err)
	}
	recheck, err := FindRecheck(ctx, testProgram, "old")
	if err != nil {
		t.Fatal(err)
	}
	if reminder := recheck.LastReminder; reminder == nil ||
		reminder.Subject != "Reminder: recheck still open after 24h: Learner" || len(reminder.To) != 2 {
		t.Errorf("lastReminder = %+v", reminder)
	}
	if sent, _ := RunRecheckReminderSweep(ctx, testProgram); sent != 0 {
		t.Errorf("second sweep sent %d, want 0", sent)
	}
}

func TestSendSalesAuditMail(t *testing.T) {
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	useTestDB(t, &at)
	insertDocs(t, models.AlertsCollection, false, models.Alert{
		ID: "a1", Program: testProgram, Mail: models.Mail{To: []string{"bda@example.com"}, Subject: "Hello"},
	})
	ctx := context.Background()
	delivery := func() string {
		alert, err := config.MongoDB.Collection(models.AlertsCollection).FindOne(ctx, bson.M{"id": "a1"}).Raw()
		if err != nil {
			t.Fatal(err)
		}
		return alert.Lookup("delivery").StringValue()
	}

	smtpSettings := []*string{&config.SMTPHost, &config.SMTPUsername, &config.SMTPPassword, &config.SMTPFrom}
	deliver := deliverMail
	t.Cleanup(func() {
		deliverMail = deliver
		for _, setting := range smtpSettings {
			*setting = ""
		}
	})
	var sent []string
	var sendErr error
	deliverMail = func(to []string, subject, htmlBody string) error {
		sent = append(sent, subject)
		return sendErr
	}

	for _, setting := range smtpSettings {
		*setting = ""
	}
	if err := SendSalesAuditMail(ctx, testProgram, "a1"); err != nil || delivery() != models.DeliverySkipped {
		t.Errorf("unconfigured SMTP: err %v, delivery %q", err, delivery())
	}

	for _, setting := range smtpSettings {
		*setting = "set"
	}
	if err := SendSalesAuditMail(ctx, testProgram, "a1"); err != nil || delivery() != models.DeliverySent || len(sent) != 1 {
		t.Errorf("configured SMTP: err %v, delivery %q", err, delivery())
	}

	sendErr = errors.New("smtp down")
	if err := SendSalesAuditMail(ctx, testProgram, "a1"); err == nil || delivery() != models.DeliveryFailed {
		t.Errorf("failing SMTP: err %v, delivery %q", err, delivery())
	}

	if err := SendSalesAuditMail(ctx, testProgram, "missing"); err != nil {
		t.Errorf("missing alert should be dropped, got %v", err)
	}
}

func TestSendTestMail(t *testing.T) {
	smtpSettings := []*string{&config.SMTPHost, &config.SMTPUsername, &config.SMTPPassword, &config.SMTPFrom}
	deliver := deliverMail
	t.Cleanup(func() {
		deliverMail = deliver
		for _, setting := range smtpSettings {
			*setting = ""
		}
	})
	var sentTo []string
	var sendErr error
	deliverMail = func(to []string, subject, htmlBody string) error {
		sentTo = to
		return sendErr
	}

	if err := SendTestMail("me@example.com"); !errors.Is(err, ErrSMTPNotConfigured) || sentTo != nil {
		t.Errorf("unconfigured SMTP: err %v, sent to %v", err, sentTo)
	}
	for _, setting := range smtpSettings {
		*setting = "set"
	}
	if err := SendTestMail("me@example.com"); err != nil || len(sentTo) != 1 || sentTo[0] != "me@example.com" {
		t.Errorf("configured SMTP: err %v, sent to %v", err, sentTo)
	}
	sendErr = errors.New("535 authentication failed")
	if err := SendTestMail("me@example.com"); err != sendErr {
		t.Errorf("failing SMTP: err %v, want the SMTP error", err)
	}
}
