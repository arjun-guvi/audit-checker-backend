package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"auditApp/salesAudit/models"
	"auditApp/salesAudit/service"
	"auditApp/salesAudit/store"
)

const program = "test-program"

type fakeQueue struct{ alertIDs []string }

func (q *fakeQueue) EnqueueMail(program, alertID string) error {
	q.alertIDs = append(q.alertIDs, alertID)
	return nil
}

type fakeMailer struct {
	configured bool
	err        error
	sent       []string
}

func (m *fakeMailer) Configured() bool { return m.configured }

func (m *fakeMailer) Send(to []string, subject, htmlBody string) error {
	m.sent = append(m.sent, subject)
	return m.err
}

func newService(s store.Store, at *time.Time) (*service.Service, *fakeQueue) {
	queue := &fakeQueue{}
	return &service.Service{Store: s, Mail: queue, AccountsEmail: "accounts@example.com", Now: func() time.Time { return *at }}, queue
}

func TestEscalationSweepMailsOncePer24h(t *testing.T) {
	s := store.NewMemory()
	s.Leads = []models.ZohoLead{
		{ID: "L1", ZenID: "Z1", Stage: "Audit", StudentFullName: "Overdue", SaleOwner: "BDA - bda@example.com", AddedTime: "20-Sep-2026 10:00:00"},
		{ID: "L2", ZenID: "Z2", Stage: "Audit", StudentFullName: "Fresh", AddedTime: "24-Sep-2026 17:00:00"},
		{ID: "L3", ZenID: "Z3", Stage: "Audit", StudentFullName: "Clean", AddedTime: "20-Sep-2026 10:00:00"},
	}
	s.Payments = []models.ZohoPayment{
		{AllEnrolment: "Z1", Type: models.CreditBookingAmount, Verified: ""},
		{AllEnrolment: "Z2", Type: models.CreditBookingAmount, Verified: "Mismatch"},
		{AllEnrolment: "Z3", Type: models.CreditBookingAmount, Verified: "Yes"},
	}
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	svc, queue := newService(s, &at)
	ctx := context.Background()

	if sent, err := svc.RunEscalationSweep(ctx, program); err != nil || sent != 1 {
		t.Fatalf("first sweep sent %d (%v), want 1", sent, err)
	}
	if alert := s.AlertDocs[0]; alert.LeadID != "L1" || alert.Trigger != models.TriggerAuto ||
		alert.To[0] != "bda@example.com" || alert.To[1] != "accounts@example.com" || len(queue.alertIDs) != 1 {
		t.Errorf("alert = %+v", alert)
	}

	at = at.Add(time.Hour)
	if sent, _ := svc.RunEscalationSweep(ctx, program); sent != 0 {
		t.Errorf("sweep an hour later sent %d, want 0", sent)
	}
	at = at.Add(24 * time.Hour)
	if sent, _ := svc.RunEscalationSweep(ctx, program); sent != 2 {
		t.Errorf("sweep a day later sent %d, want 2 (L1 again, L2 now overdue)", sent)
	}
}

func TestRecheckReminderSweep(t *testing.T) {
	s := store.NewMemory()
	s.Leads = []models.ZohoLead{{ID: "L1", Stage: "Audit", StudentFullName: "Learner", SaleOwner: "BDA - bda@example.com", SaleOwnerManager: "BDM - bdm@example.com"}}
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	s.RecheckDocs = []models.Recheck{
		{ID: "old", Program: program, LeadID: "L1", Status: models.RecheckOpen, RaisedAt: at.Add(-25 * time.Hour).Unix()},
		{ID: "new", Program: program, LeadID: "L1", Status: models.RecheckOpen, RaisedAt: at.Add(-2 * time.Hour).Unix()},
		{ID: "done", Program: program, LeadID: "L1", Status: models.RecheckResolved, RaisedAt: at.Add(-50 * time.Hour).Unix()},
	}
	svc, _ := newService(s, &at)

	if sent, err := svc.RunRecheckReminderSweep(context.Background(), program); err != nil || sent != 1 {
		t.Fatalf("sent %d (%v), want 1", sent, err)
	}
	reminder := s.RecheckDocs[0].LastReminder
	if reminder == nil || reminder.Subject != "Reminder: recheck still open after 24h: Learner" || len(reminder.To) != 2 {
		t.Errorf("lastReminder = %+v", reminder)
	}
	if sent, _ := svc.RunRecheckReminderSweep(context.Background(), program); sent != 0 {
		t.Errorf("second sweep sent %d, want 0", sent)
	}
}

func TestSendMail(t *testing.T) {
	s := store.NewMemory()
	s.AlertDocs = []models.Alert{{ID: "a1", Program: program, Mail: models.Mail{To: []string{"bda@example.com"}, Subject: "Hello"}}}
	ctx := context.Background()

	if err := SendMail(ctx, s, &fakeMailer{}, program, "a1"); err != nil || s.DeliveryUpdates["a1"] != models.DeliverySkipped {
		t.Errorf("unconfigured SMTP: err %v, delivery %q", err, s.DeliveryUpdates["a1"])
	}

	mailer := &fakeMailer{configured: true}
	if err := SendMail(ctx, s, mailer, program, "a1"); err != nil || s.DeliveryUpdates["a1"] != models.DeliverySent || len(mailer.sent) != 1 {
		t.Errorf("configured SMTP: err %v, delivery %q", err, s.DeliveryUpdates["a1"])
	}

	failing := &fakeMailer{configured: true, err: errors.New("smtp down")}
	if err := SendMail(ctx, s, failing, program, "a1"); err == nil || s.DeliveryUpdates["a1"] != models.DeliveryFailed {
		t.Errorf("failing SMTP: err %v, delivery %q", err, s.DeliveryUpdates["a1"])
	}

	if err := SendMail(ctx, s, mailer, program, "missing"); err != nil {
		t.Errorf("missing alert should be dropped, got %v", err)
	}
}
