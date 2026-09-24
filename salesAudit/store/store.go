package store

import (
	"context"
	"errors"

	"auditApp/salesAudit/models"
)

// ErrNotFound is returned when a single record does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when an insert breaks a unique index (e.g. a second audit for a lead).
var ErrDuplicate = errors.New("duplicate")

// AlertFilter narrows ListAlerts; zero values mean "any".
type AlertFilter struct {
	LeadIDs []string
	Kind    string
	Since   int64
}

// RecheckFilter narrows ListRechecks; zero values mean "any".
type RecheckFilter struct {
	LeadID string
	Status string
}

// Store is everything the Sales Audit feature reads and writes. The Zoho methods read the
// imported collections; the rest are the feature's own collections and are scoped by program.
type Store interface {
	// Zoho (read-only)
	ListAuditLeads(ctx context.Context) ([]models.ZohoLead, error)
	GetLead(ctx context.Context, leadID string) (models.ZohoLead, error)
	PaymentsForLeads(ctx context.Context, zenIDs []string) ([]models.ZohoPayment, error)
	EmiForLeads(ctx context.Context, zenIDs []string) ([]models.ZohoEmi, error)
	PartialReminders(ctx context.Context, leadID string) ([]models.ZohoPartialReminder, error)
	SubscriptionReminders(ctx context.Context, zenID string) ([]models.ZohoSubscriptionReminder, error)
	DiscountsForEmail(ctx context.Context, email string) ([]models.ZohoDiscount, error)

	// Alerts (mail log)
	ListAlerts(ctx context.Context, program string, filter AlertFilter) ([]models.Alert, error)
	GetAlert(ctx context.Context, program, alertID string) (models.Alert, error)
	InsertAlert(ctx context.Context, alert models.Alert) error
	SetAlertDelivery(ctx context.Context, program, alertID, delivery string) error

	// CC responses
	CcResponses(ctx context.Context, program string, leadIDs []string) (map[string]models.CcResponse, error)
	UpsertCcResponse(ctx context.Context, response models.CcResponse) error

	// Audits
	Audits(ctx context.Context, program string, leadIDs []string) (map[string]models.Audit, error)
	InsertAudit(ctx context.Context, audit models.Audit) error

	// Rechecks
	ListRechecks(ctx context.Context, program string, filter RecheckFilter) ([]models.Recheck, error)
	GetRecheck(ctx context.Context, program, recheckID string) (models.Recheck, error)
	InsertRecheck(ctx context.Context, recheck models.Recheck) error
	ResolveRecheck(ctx context.Context, program, recheckID string, resolvedAt int64) error
	SetRecheckReminder(ctx context.Context, program, recheckID string, reminder models.Mail) error

	// CC extracts
	GetCcExtract(ctx context.Context, program, leadID string) (models.CcExtract, error)
}
