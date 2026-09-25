// Package store is the only package that talks to MongoDB. Every operation is an exported
// function variable pointing at its Mongo implementation, so tests can swap in the in-memory
// versions from store/fake. Every query filters by program and deleted:false.
package store

import (
	"context"
	"errors"

	"auditApp/salesAudit/models"
)

// ErrNotFound is returned when a single-record lookup finds nothing.
var ErrNotFound = errors.New("not found")

// IsNotFound reports whether err means "no such record".
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// Members
var (
	FindMemberByHash  func(ctx context.Context, program, userHash string) (models.Member, error) = findMemberByHash
	FindMemberByEmail func(ctx context.Context, program, email string) (models.Member, error)    = findMemberByEmail
	FindMember        func(ctx context.Context, program, memberID string) (models.Member, error) = findMember
	FindMembers       func(ctx context.Context, program, role string) ([]models.Member, error)   = findMembers
	InsertMember      func(ctx context.Context, member models.Member) error                      = insertMember
	ReplaceMember     func(ctx context.Context, member models.Member) error                      = replaceMember
)

// Leads
var (
	FindLeads          func(ctx context.Context, program string, query models.LeadQuery) ([]models.Lead, int, error)                = findLeads
	FindLead           func(ctx context.Context, program, leadID string) (models.Lead, error)                                       = findLead
	FindLeadByZenID    func(ctx context.Context, program, zenID string) (models.Lead, error)                                        = findLeadByZenID
	InsertLead         func(ctx context.Context, lead models.Lead) error                                                            = insertLead
	UpdateLeadZoho     func(ctx context.Context, lead models.Lead) error                                                            = updateLeadZoho
	UpdateLeadWorkflow func(ctx context.Context, lead models.Lead) error                                                            = updateLeadWorkflow
	SetLeadMailMarks   func(ctx context.Context, program, leadID string, paymentVerificationMailedAt, lastEscalationAt int64) error = setLeadMailMarks
)

// Rechecks
var (
	FindRechecks    func(ctx context.Context, program string, query models.RecheckQuery) ([]models.Recheck, error) = findRechecks
	FindRecheck     func(ctx context.Context, program, recheckID string) (models.Recheck, error)                   = findRecheck
	FindRecheckByNo func(ctx context.Context, program, recheckNo string) (models.Recheck, error)                   = findRecheckByNo
	InsertRecheck   func(ctx context.Context, recheck models.Recheck) error                                        = insertRecheck
	ReplaceRecheck  func(ctx context.Context, recheck models.Recheck) error                                        = replaceRecheck
)

// Audits, events, notifications and counters
var (
	InsertAudit              func(ctx context.Context, audit models.Audit) error                                                              = insertAudit
	FindAudits               func(ctx context.Context, program string, query models.AuditQuery) ([]models.Audit, error)                       = findAudits
	InsertEvent              func(ctx context.Context, event models.Event) error                                                              = insertEvent
	FindEvents               func(ctx context.Context, program, leadID string) ([]models.Event, error)                                        = findEvents
	InsertNotification       func(ctx context.Context, notification models.Notification) error                                                = insertNotification
	FindNotifications        func(ctx context.Context, program, email string, unreadOnly bool, limit int) ([]models.Notification, int, error) = findNotifications
	MarkNotificationRead     func(ctx context.Context, program, email, notificationID string, at int64) error                                 = markNotificationRead
	MarkAllNotificationsRead func(ctx context.Context, program, email string, at int64) error                                                 = markAllNotificationsRead
	NextSequence             func(ctx context.Context, program, name string, now int64) (int64, error)                                        = nextSequence
)

// Mail log and CC extracts
var (
	InsertAlert      func(ctx context.Context, alert models.Alert) error                                                           = insertAlert
	FindAlert        func(ctx context.Context, program, alertID string) (models.Alert, error)                                      = findAlert
	FindAlerts       func(ctx context.Context, program string, leadIDs []string, kind string, since int64) ([]models.Alert, error) = findAlerts
	SetAlertDelivery func(ctx context.Context, program, alertID, delivery string) error                                            = setAlertDelivery
	FindCcExtract    func(ctx context.Context, program, leadID string) (models.CcExtract, error)                                   = findCcExtract
	SaveCcExtract    func(ctx context.Context, extract models.CcExtract) error                                                     = saveCcExtract
)
