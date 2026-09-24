package store

import (
	"context"
	"sort"
	"strings"
	"sync"

	"auditApp/salesAudit/models"
)

// Memory is an in-memory Store for tests. Fill the exported Zoho slices directly.
type Memory struct {
	mu sync.Mutex

	Leads                 []models.ZohoLead
	Payments              []models.ZohoPayment
	Emis                  []models.ZohoEmi
	Discounts             []models.ZohoDiscount
	Partials              []models.ZohoPartialReminder
	Subscriptions         []models.ZohoSubscriptionReminder
	CcExtracts            []models.CcExtract
	AlertDocs             []models.Alert
	RecheckDocs           []models.Recheck
	CcResponseDocs        []models.CcResponse
	AuditDocs             []models.Audit
	DeliveryUpdates       map[string]string
	FailNextListAuditLead error
}

func NewMemory() *Memory {
	return &Memory{DeliveryUpdates: map[string]string{}}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (m *Memory) ListAuditLeads(ctx context.Context) ([]models.ZohoLead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.FailNextListAuditLead; err != nil {
		m.FailNextListAuditLead = nil
		return nil, err
	}
	leads := []models.ZohoLead{}
	for _, lead := range m.Leads {
		if lead.Stage == models.AuditStage {
			leads = append(leads, lead)
		}
	}
	return leads, nil
}

func (m *Memory) GetLead(ctx context.Context, leadID string) (models.ZohoLead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, lead := range m.Leads {
		if lead.ID == leadID {
			return lead, nil
		}
	}
	return models.ZohoLead{}, ErrNotFound
}

func (m *Memory) PaymentsForLeads(ctx context.Context, zenIDs []string) ([]models.ZohoPayment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	payments := []models.ZohoPayment{}
	for _, payment := range m.Payments {
		if contains(zenIDs, payment.LeadZenID()) {
			payments = append(payments, payment)
		}
	}
	return payments, nil
}

func (m *Memory) EmiForLeads(ctx context.Context, zenIDs []string) ([]models.ZohoEmi, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	emis := []models.ZohoEmi{}
	for _, emi := range m.Emis {
		if contains(zenIDs, emi.ZenID) {
			emis = append(emis, emi)
		}
	}
	return emis, nil
}

func (m *Memory) PartialReminders(ctx context.Context, leadID string) ([]models.ZohoPartialReminder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reminders := []models.ZohoPartialReminder{}
	for _, reminder := range m.Partials {
		if reminder.StudentID == leadID {
			reminders = append(reminders, reminder)
		}
	}
	return reminders, nil
}

func (m *Memory) SubscriptionReminders(ctx context.Context, zenID string) ([]models.ZohoSubscriptionReminder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reminders := []models.ZohoSubscriptionReminder{}
	for _, reminder := range m.Subscriptions {
		if reminder.ZenID == zenID {
			reminders = append(reminders, reminder)
		}
	}
	return reminders, nil
}

func (m *Memory) DiscountsForEmail(ctx context.Context, email string) ([]models.ZohoDiscount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	discounts := []models.ZohoDiscount{}
	for _, discount := range m.Discounts {
		if strings.EqualFold(discount.LearnerEmail, email) {
			discounts = append(discounts, discount)
		}
	}
	return discounts, nil
}

func (m *Memory) ListAlerts(ctx context.Context, program string, filter AlertFilter) ([]models.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	alerts := []models.Alert{}
	for _, alert := range m.AlertDocs {
		if alert.Program != program || alert.Deleted ||
			(filter.LeadIDs != nil && !contains(filter.LeadIDs, alert.LeadID)) ||
			(filter.Kind != "" && alert.Kind != filter.Kind) ||
			(filter.Since > 0 && alert.SentAt < filter.Since) {
			continue
		}
		alerts = append(alerts, alert)
	}
	sort.SliceStable(alerts, func(i, j int) bool { return alerts[i].SentAt < alerts[j].SentAt })
	return alerts, nil
}

func (m *Memory) GetAlert(ctx context.Context, program, alertID string) (models.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, alert := range m.AlertDocs {
		if alert.Program == program && alert.ID == alertID && !alert.Deleted {
			return alert, nil
		}
	}
	return models.Alert{}, ErrNotFound
}

func (m *Memory) InsertAlert(ctx context.Context, alert models.Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AlertDocs = append(m.AlertDocs, alert)
	return nil
}

func (m *Memory) SetAlertDelivery(ctx context.Context, program, alertID, delivery string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.AlertDocs {
		if m.AlertDocs[index].Program == program && m.AlertDocs[index].ID == alertID {
			m.AlertDocs[index].Delivery = delivery
			m.DeliveryUpdates[alertID] = delivery
		}
	}
	return nil
}

func (m *Memory) CcResponses(ctx context.Context, program string, leadIDs []string) (map[string]models.CcResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	byLead := map[string]models.CcResponse{}
	for _, response := range m.CcResponseDocs {
		if response.Program == program && !response.Deleted && (leadIDs == nil || contains(leadIDs, response.LeadID)) {
			byLead[response.LeadID] = response
		}
	}
	return byLead, nil
}

func (m *Memory) UpsertCcResponse(ctx context.Context, response models.CcResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, existing := range m.CcResponseDocs {
		if existing.Program == response.Program && existing.LeadID == response.LeadID {
			response.ID, response.Created = existing.ID, existing.Created
			m.CcResponseDocs[index] = response
			return nil
		}
	}
	m.CcResponseDocs = append(m.CcResponseDocs, response)
	return nil
}

func (m *Memory) Audits(ctx context.Context, program string, leadIDs []string) (map[string]models.Audit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	byLead := map[string]models.Audit{}
	for _, audit := range m.AuditDocs {
		if audit.Program == program && !audit.Deleted && (leadIDs == nil || contains(leadIDs, audit.LeadID)) {
			byLead[audit.LeadID] = audit
		}
	}
	return byLead, nil
}

func (m *Memory) InsertAudit(ctx context.Context, audit models.Audit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.AuditDocs {
		if existing.Program == audit.Program && existing.LeadID == audit.LeadID {
			return ErrDuplicate
		}
	}
	m.AuditDocs = append(m.AuditDocs, audit)
	return nil
}

func (m *Memory) ListRechecks(ctx context.Context, program string, filter RecheckFilter) ([]models.Recheck, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rechecks := []models.Recheck{}
	for _, recheck := range m.RecheckDocs {
		if recheck.Program != program || recheck.Deleted ||
			(filter.LeadID != "" && recheck.LeadID != filter.LeadID) ||
			(filter.Status != "" && recheck.Status != filter.Status) {
			continue
		}
		rechecks = append(rechecks, recheck)
	}
	sort.SliceStable(rechecks, func(i, j int) bool { return rechecks[i].RaisedAt > rechecks[j].RaisedAt })
	return rechecks, nil
}

func (m *Memory) GetRecheck(ctx context.Context, program, recheckID string) (models.Recheck, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, recheck := range m.RecheckDocs {
		if recheck.Program == program && recheck.ID == recheckID && !recheck.Deleted {
			return recheck, nil
		}
	}
	return models.Recheck{}, ErrNotFound
}

func (m *Memory) InsertRecheck(ctx context.Context, recheck models.Recheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RecheckDocs = append(m.RecheckDocs, recheck)
	return nil
}

func (m *Memory) ResolveRecheck(ctx context.Context, program, recheckID string, resolvedAt int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.RecheckDocs {
		if m.RecheckDocs[index].Program == program && m.RecheckDocs[index].ID == recheckID {
			m.RecheckDocs[index].Status = models.RecheckResolved
			m.RecheckDocs[index].ResolvedAt = &resolvedAt
		}
	}
	return nil
}

func (m *Memory) SetRecheckReminder(ctx context.Context, program, recheckID string, reminder models.Mail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.RecheckDocs {
		if m.RecheckDocs[index].Program == program && m.RecheckDocs[index].ID == recheckID {
			m.RecheckDocs[index].LastReminder = &reminder
		}
	}
	return nil
}

func (m *Memory) GetCcExtract(ctx context.Context, program, leadID string) (models.CcExtract, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, extract := range m.CcExtracts {
		if extract.Program == program && extract.LeadID == leadID && !extract.Deleted {
			return extract, nil
		}
	}
	return models.CcExtract{}, ErrNotFound
}
