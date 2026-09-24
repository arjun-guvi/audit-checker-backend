package models

// Sales Audit feature: constants and the documents it owns.

// Collections owned by the Sales Audit feature. The Zoho collections are read-only.
const (
	RechecksCollection    = "salesAuditRechecks"
	CcResponsesCollection = "salesAuditCcResponses"
	AuditsCollection      = "salesAuditAudits"
	AlertsCollection      = "salesAuditAlerts"
	CcExtractsCollection  = "salesAuditCcExtracts"

	ZohoLeadsCollection                 = "LeadData"
	ZohoPaymentsCollection              = "paymentData"
	ZohoEmiCollection                   = "EmiData"
	ZohoDiscountsCollection             = "DiscountData"
	ZohoPartialRemindersCollection      = "PartialReminders"
	ZohoSubscriptionRemindersCollection = "SubscriptionReminders"
)

// Permissions declared on every route.
const (
	PermissionView = "salesAudit.view"
	PermissionEdit = "salesAudit.edit"
)

// AuditStage is the LeadData Stage whose leads the auditor works on.
const AuditStage = "Audit"

// AuditTeamName is recorded as raisedBy / auditedBy until Zen provides the user's name.
const AuditTeamName = "Audit Team"

// Zoho financial record types that decide verification.
const (
	CreditBookingAmount    = "Credit_Booking_Amount"
	CreditPart1            = "Credit_Part1"
	CreditRemainingBalance = "Credit_RemainingBalance"
)

// Recheck categories, keyed as the frontend stores them, with their mail labels.
var RecheckCategories = map[string]string{
	"ccPending":        "CC Pending",
	"payment":          "Payment",
	"emi":              "EMI",
	"approval":         "Approval",
	"missedPointsInCc": "Missed points in CC",
	"downPayment":      "Down Payment",
}

const (
	RecheckOpen     = "open"
	RecheckResolved = "resolved"
)

// BDA answers for a pending CC.
const (
	CcMailSentAwaitingAck = "mailSentAwaitingAck"
	CcMailNotSent         = "mailNotSent"
)

// Alert kinds in the mail log.
const (
	AlertEscalation      = "escalation"
	AlertRecheck         = "recheck"
	AlertRecheckReminder = "recheckReminder"
	AlertCcNotSent       = "ccNotSent"
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

// Created is who created a document and when (Unix seconds, user hash).
type Created struct {
	At int64  `bson:"at" json:"at"`
	By string `bson:"by" json:"by"`
}

// Mail is an alert mail as the frontend shows it.
type Mail struct {
	To      []string `bson:"to" json:"to"`
	Subject string   `bson:"subject" json:"subject"`
	Trigger string   `bson:"trigger" json:"trigger"`
	SentAt  int64    `bson:"sentAt" json:"sentAt"`
}

// Alert is one logged mail (salesAuditAlerts).
type Alert struct {
	ID       string `bson:"id" json:"id"`
	Program  string `bson:"program" json:"-"`
	LeadID   string `bson:"leadId" json:"leadId"`
	Kind     string `bson:"kind" json:"kind"`
	Mail     `bson:",inline"`
	Delivery string  `bson:"delivery" json:"-"`
	Created  Created `bson:"created" json:"-"`
	Deleted  bool    `bson:"deleted" json:"-"`
}

// Recheck is an issue the auditor raised on a lead (salesAuditRechecks).
type Recheck struct {
	ID           string  `bson:"id" json:"id"`
	Program      string  `bson:"program" json:"-"`
	LeadID       string  `bson:"leadId" json:"leadId"`
	Category     string  `bson:"category" json:"category"`
	Notes        string  `bson:"notes" json:"notes"`
	Status       string  `bson:"status" json:"status"`
	RaisedBy     string  `bson:"raisedBy" json:"raisedBy"`
	RaisedAt     int64   `bson:"raisedAt" json:"raisedAt"`
	ResolvedAt   *int64  `bson:"resolvedAt" json:"resolvedAt"`
	Alert        Mail    `bson:"alert" json:"alert"`
	LastReminder *Mail   `bson:"lastReminder,omitempty" json:"lastReminder,omitempty"`
	Created      Created `bson:"created" json:"-"`
	Deleted      bool    `bson:"deleted" json:"-"`
}

// CcResponse is the BDA's answer for a lead whose CC is pending (salesAuditCcResponses).
type CcResponse struct {
	ID        string  `bson:"id" json:"-"`
	Program   string  `bson:"program" json:"-"`
	LeadID    string  `bson:"leadId" json:"-"`
	Response  string  `bson:"response" json:"response"`
	UpdatedAt int64   `bson:"updatedAt" json:"updatedAt"`
	Alert     *Mail   `bson:"alert" json:"alert"`
	Created   Created `bson:"created" json:"-"`
	Deleted   bool    `bson:"deleted" json:"-"`
}

// Audit is the auditor's decision that moves a lead to Awaiting (salesAuditAudits).
type Audit struct {
	ID             string  `bson:"id" json:"-"`
	Program        string  `bson:"program" json:"-"`
	LeadID         string  `bson:"leadId" json:"-"`
	AuditedAt      int64   `bson:"auditedAt" json:"auditedAt"`
	AuditedBy      string  `bson:"auditedBy" json:"auditedBy"`
	OverrideReason string  `bson:"overrideReason" json:"overrideReason"`
	Created        Created `bson:"created" json:"-"`
	Deleted        bool    `bson:"deleted" json:"-"`
}

// CcExtract is what was read from a lead's CC mail (salesAuditCcExtracts). Nothing writes it
// yet; it is where the CC parser (AI service, via the Go backend) will store its output.
type CcExtract struct {
	ID            string   `bson:"id" json:"-"`
	Program       string   `bson:"program" json:"-"`
	LeadID        string   `bson:"leadId" json:"-"`
	Scraped       Source   `bson:"scraped" json:"scraped"`
	PointsCovered []string `bson:"pointsCovered" json:"pointsCovered"`
	Created       Created  `bson:"created" json:"-"`
	Deleted       bool     `bson:"deleted" json:"-"`
}
