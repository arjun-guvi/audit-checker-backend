package models

// Sales Audit feature: collection names, permissions and enums.

// Collections owned by the feature. Every document has id, program, created and deleted.
const (
	LeadsCollection         = "salesAuditLeads"
	MembersCollection       = "salesAuditMembers"
	AuditsCollection        = "salesAuditAudits"
	RechecksCollection      = "salesAuditRechecks"
	EventsCollection        = "salesAuditEvents"
	NotificationsCollection = "salesAuditNotifications"
	CountersCollection      = "salesAuditCounters"
	AlertsCollection        = "salesAuditAlerts"
	CcExtractsCollection    = "salesAuditCcExtracts"
)

// Permissions declared on every route.
const (
	PermissionView = "salesAudit.view"
	PermissionEdit = "salesAudit.edit"
)

// Member roles.
const (
	RoleAuditorTl = "auditorTl"
	RoleAuditor   = "auditor"
	RoleBdm       = "bdm"
	RoleBda       = "bda"
	// RoleSystem is the actor role of scheduled jobs and the Zoho import.
	RoleSystem = "system"
)

// Roles lists the member roles a member may hold.
var Roles = []string{RoleAuditorTl, RoleAuditor, RoleBdm, RoleBda}

// Regions leads come in by (Zoho salesTeam) and auditors cover.
const (
	RegionNorth = "North"
	RegionSouth = "South"
)

// SystemUser is created.by and the actor email for scheduled jobs and the Zoho import.
const SystemUser = "system"

// Audit status of a lead:
// unassigned → pending → completed, or → recheckOpen → recheckClosed → (re-audit) completed | recheckOpen.
const (
	AuditUnassigned    = "unassigned"
	AuditPending       = "pending"
	AuditRecheckOpen   = "recheckOpen"
	AuditRecheckClosed = "recheckClosed"
	AuditCompleted     = "completed"
)

// Outcome of one audit attempt.
const (
	OutcomeCompleted     = "completed"
	OutcomeRecheckRaised = "recheckRaised"
)

// How a lead reached its auditor.
const (
	AssignAuto   = "auto"
	AssignManual = "manual"
	AssignTakeUp = "takeUp"
	AssignZoho   = "zoho"
)

// Recheck categories, keyed as the API sends them, with their labels.
const (
	CategoryCcPending        = "ccPending"
	CategoryPayment          = "payment"
	CategoryEmi              = "emi"
	CategoryApproval         = "approval"
	CategoryMissedPointsInCc = "missedPointsInCc"
	CategoryDownPayment      = "downPayment"
)

var RecheckCategories = map[string]string{
	CategoryCcPending:        "CC Pending",
	CategoryPayment:          "Payment",
	CategoryEmi:              "EMI",
	CategoryApproval:         "Approval",
	CategoryMissedPointsInCc: "Missed points in CC",
	CategoryDownPayment:      "Down Payment",
}

// Recheck ticket status.
const (
	RecheckOpen   = "open"
	RecheckClosed = "closed"
)

// Where a recheck came from.
const (
	SourcePortal = "portal"
	SourceZoho   = "zoho"
)

// CC status: updated once Superleap → Zoho has the CC link.
const (
	CcPending = "pending"
	CcUpdated = "updated"
)

// CC source type, from the link.
const (
	CcTypePdf       = "pdf"
	CcTypeRecording = "recording"
	CcTypeLink      = "link"
)

// Timeline event types.
const (
	EventLeadImported   = "leadImported"
	EventAssigned       = "assigned"
	EventReassigned     = "reassigned"
	EventTakenUp        = "takenUp"
	EventCcUpdated      = "ccUpdated"
	EventAuditCompleted = "auditCompleted"
	EventRecheckRaised  = "recheckRaised"
	EventRecheckClosed  = "recheckClosed"
)

// In-app notification types.
const (
	NotifyLeadsAssigned  = "leadsAssigned"
	NotifyLeadReassigned = "leadReassigned"
	NotifyRecheckRaised  = "recheckRaised"
	NotifyRecheckClosed  = "recheckClosed"
	NotifyCcUpdated      = "ccUpdated"
	// NotifyCcTicketOpen: a CC recheck's CC was updated but the ticket is still open.
	NotifyCcTicketOpen = "ccTicketOpen"
)

// Alert (mail log) kinds.
const (
	AlertEscalation                 = "escalation"
	AlertRecheck                    = "recheck"
	AlertRecheckReminder            = "recheckReminder"
	AlertRecheckClosed              = "recheckClosed"
	AlertLeadsAssigned              = "leadsAssigned"
	AlertCcUpdated                  = "ccUpdated"
	AlertPaymentVerificationPending = "paymentVerificationPending"
	AlertCcTicketOpen               = "ccTicketOpen"
)

const (
	TriggerAuto   = "auto"
	TriggerManual = "manual"
)

// Delivery states of a logged mail.
const (
	DeliveryQueued  = "queued"
	DeliverySent    = "sent"
	DeliverySkipped = "skipped"
	DeliveryFailed  = "failed"
)

// Zoho financial record types.
const (
	CreditBookingAmount    = "Credit_Booking_Amount"
	CreditPart1            = "Credit_Part1"
	CreditRemainingBalance = "Credit_RemainingBalance"
	CreditDiscount         = "Discount"
)

// Worker job names.
const (
	JobZohoImport               = "salesAudit_zoho_import"
	JobAssignLeads              = "salesAudit_assign_leads"
	JobEscalationSweep          = "salesAudit_escalation_sweep"
	JobRecheckReminderSweep     = "salesAudit_recheck_reminder_sweep"
	JobPaymentVerificationSweep = "salesAudit_payment_verification_sweep"
	JobSendMail                 = "salesAudit_send_mail"
)
