package models

// Documents stored in the feature's collections. Timestamps are Unix seconds.

// Created is who created a document and when (user hash, or "system").
type Created struct {
	At int64  `bson:"at" json:"at"`
	By string `bson:"by" json:"by"`
}

// Actor is who did something, as shown in the timeline and tickets.
type Actor struct {
	Email string `bson:"email" json:"email"`
	Name  string `bson:"name" json:"name"`
	Role  string `bson:"role" json:"role"`
}

// Member is a person using the feature and their place in the team (salesAuditMembers).
// Auditors report to an auditor TL and BDAs to a BDM through managerEmail.
type Member struct {
	ID           string  `bson:"id" json:"id"`
	Program      string  `bson:"program" json:"-"`
	UserHash     string  `bson:"userHash" json:"userHash"`
	Email        string  `bson:"email" json:"email"`
	Name         string  `bson:"name" json:"name"`
	Role         string  `bson:"role" json:"role"`
	Region       string  `bson:"region" json:"region"`
	ManagerEmail string  `bson:"managerEmail" json:"managerEmail"`
	Available    bool    `bson:"available" json:"available"`
	Created      Created `bson:"created" json:"created"`
	Deleted      bool    `bson:"deleted" json:"-"`
}

// Lead is one enrolment imported from Zoho plus its audit workflow state (salesAuditLeads).
// The Zoho fields are refreshed on every import; assignment, audit and recheckSummary belong to
// the portal and are never overwritten by the import.
type Lead struct {
	ID          string `bson:"id" json:"id"`
	Program     string `bson:"program" json:"-"`
	ZenID       string `bson:"zenId" json:"zenId"`
	SuperleapID string `bson:"superleapId" json:"superleapId"`
	Region      string `bson:"region" json:"region"`
	SalesFrom   string `bson:"salesFrom" json:"salesFrom"`
	Stage       string `bson:"stage" json:"stage"`
	ZohoStatus  string `bson:"zohoStatus" json:"zohoStatus"`

	Personal      LeadPersonal      `bson:"personal" json:"personal"`
	Course        LeadCourse        `bson:"course" json:"course"`
	Payment       LeadPayment       `bson:"payment" json:"payment"`
	Admission     map[string]string `bson:"admission" json:"admission"`
	TermsAccepted bool              `bson:"termsAccepted" json:"termsAccepted"`
	Marketing     LeadMarketing     `bson:"marketing" json:"marketing"`

	BdaEmail           string `bson:"bdaEmail" json:"bdaEmail"`
	BdmEmail           string `bson:"bdmEmail" json:"bdmEmail"`
	OnboardCoordinator string `bson:"onboardCoordinator" json:"onboardCoordinator"`

	Cc LeadCc `bson:"cc" json:"cc"`

	Assignment     *Assignment    `bson:"assignment" json:"assignment"`
	Audit          AuditState     `bson:"audit" json:"audit"`
	RecheckSummary RecheckSummary `bson:"recheckSummary" json:"recheckSummary"`

	CrmCreatedAt int64 `bson:"crmCreatedAt" json:"crmCreatedAt"`
	EnrolledAt   int64 `bson:"enrolledAt" json:"enrolledAt"`
	ZohoSyncedAt int64 `bson:"zohoSyncedAt" json:"zohoSyncedAt"`

	// PaymentVerificationMailedAt is when the BDM was mailed about an unverified payment; 0 when never.
	PaymentVerificationMailedAt int64 `bson:"paymentVerificationMailedAt" json:"-"`
	// LastEscalationAt is when the escalation mail last went out; 0 when never.
	LastEscalationAt int64 `bson:"lastEscalationAt" json:"lastEscalationAt"`

	Created Created `bson:"created" json:"-"`
	Deleted bool    `bson:"deleted" json:"-"`
}

type LeadPersonal struct {
	Name              string `bson:"name" json:"name"`
	Email             string `bson:"email" json:"email"`
	Phone             string `bson:"phone" json:"phone"`
	PreferredLanguage string `bson:"preferredLanguage" json:"preferredLanguage"`
}

type LeadCourse struct {
	Product      string `bson:"product" json:"product"`
	ModeOfStudy  string `bson:"modeOfStudy" json:"modeOfStudy"`
	Batch        Batch  `bson:"batch" json:"batch"`
	EnrolledOn   string `bson:"enrolledOn" json:"enrolledOn"`
	OnboardingAt int64  `bson:"onboardingAt" json:"onboardingAt"`
}

type Batch struct {
	Name      string `bson:"name" json:"name"`
	Type      string `bson:"type" json:"type"`
	Language  string `bson:"language" json:"language"`
	StartDate string `bson:"startDate" json:"startDate"`
	EndDate   string `bson:"endDate" json:"endDate"`
	StartTime string `bson:"startTime" json:"startTime"`
	Status    string `bson:"status" json:"status"`
}

type LeadPayment struct {
	PaymentType     string  `bson:"paymentType" json:"paymentType"`
	PartialCategory string  `bson:"partialCategory" json:"partialCategory"`
	CourseFee       float64 `bson:"courseFee" json:"courseFee"`
	TotalPaid       float64 `bson:"totalPaid" json:"totalPaid"`
	BalanceAmount   float64 `bson:"balanceAmount" json:"balanceAmount"`
	PromoCode       string  `bson:"promoCode" json:"promoCode"`
	PayInSameMonth  string  `bson:"payInSameMonth" json:"payInSameMonth"`
	ZbCustomerID    string  `bson:"zbCustomerId" json:"zbCustomerId"`
	ZbInvoiceID     string  `bson:"zbInvoiceId" json:"zbInvoiceId"`

	// Ready is true once every payment is verified and the plan's minimum is met (see
	// core.PaymentShortfall); Shortfall says why not.
	Ready          bool    `bson:"ready" json:"ready"`
	Shortfall      string  `bson:"shortfall" json:"shortfall"`
	VerifiedAmount float64 `bson:"verifiedAmount" json:"verifiedAmount"`

	Records          []PaymentRecord   `bson:"records" json:"records"`
	Emis             []Emi             `bson:"emis" json:"emis"`
	Discounts        []Discount        `bson:"discounts" json:"discounts"`
	PartialReminders []PartialReminder `bson:"partialReminders" json:"partialReminders"`
	Subscriptions    []Subscription    `bson:"subscriptions" json:"subscriptions"`
}

// PaymentRecord is one Zoho financialDetails record.
type PaymentRecord struct {
	RecordID      string  `bson:"recordId" json:"recordId"`
	Type          string  `bson:"type" json:"type"`
	Amount        float64 `bson:"amount" json:"amount"`
	ModeOfPayment string  `bson:"modeOfPayment" json:"modeOfPayment"`
	UtrPaymentID  string  `bson:"utrPaymentId" json:"utrPaymentId"`
	Verified      string  `bson:"verified" json:"verified"`
	VerifiedDate  string  `bson:"verifiedDate" json:"verifiedDate"`
	PaymentDate   string  `bson:"paymentDate" json:"paymentDate"`
	ReceiptMade   string  `bson:"receiptMade" json:"receiptMade"`
}

// Emi is one EMI loan application.
type Emi struct {
	RecordID        string  `bson:"recordId" json:"recordId"`
	ApplicationID   string  `bson:"applicationId" json:"applicationId"`
	Vendor          string  `bson:"vendor" json:"vendor"`
	Status          string  `bson:"status" json:"status"`
	Stage           string  `bson:"stage" json:"stage"`
	LoanAmount      float64 `bson:"loanAmount" json:"loanAmount"`
	DisbursalAmount float64 `bson:"disbursalAmount" json:"disbursalAmount"`
	FirstEmiAmount  float64 `bson:"firstEmiAmount" json:"firstEmiAmount"`
	TenorMonths     string  `bson:"tenorMonths" json:"tenorMonths"`
	RoiPercent      string  `bson:"roiPercent" json:"roiPercent"`
	ApplicationDate string  `bson:"applicationDate" json:"applicationDate"`
	InTheNameOf     string  `bson:"inTheNameOf" json:"inTheNameOf"`
}

// Discount is one course discount request.
type Discount struct {
	RequestedCourseFee float64 `bson:"requestedCourseFee" json:"requestedCourseFee"`
	ActualCourseFee    float64 `bson:"actualCourseFee" json:"actualCourseFee"`
	DiscountValue      float64 `bson:"discountValue" json:"discountValue"`
	RequestedBy        string  `bson:"requestedBy" json:"requestedBy"`
	PaymentType        string  `bson:"paymentType" json:"paymentType"`
	Status             string  `bson:"status" json:"status"`
}

// PartialReminder is one scheduled partial payment.
type PartialReminder struct {
	RecordID    string  `bson:"recordId" json:"recordId"`
	NoOfPartial string  `bson:"noOfPartial" json:"noOfPartial"`
	Amount      float64 `bson:"amount" json:"amount"`
	DueDate     string  `bson:"dueDate" json:"dueDate"`
	Status      string  `bson:"status" json:"status"`
	LinkStatus  string  `bson:"linkStatus" json:"linkStatus"`
	PaidAt      string  `bson:"paidAt" json:"paidAt"`
}

// Subscription is one scheduled subscription payment.
type Subscription struct {
	RecordID         string  `bson:"recordId" json:"recordId"`
	NoOfSubscription string  `bson:"noOfSubscription" json:"noOfSubscription"`
	Amount           float64 `bson:"amount" json:"amount"`
	DueDate          string  `bson:"dueDate" json:"dueDate"`
	Status           string  `bson:"status" json:"status"`
}

type LeadMarketing struct {
	Source      string `bson:"source" json:"source"`
	Medium      string `bson:"medium" json:"medium"`
	Campaign    string `bson:"campaign" json:"campaign"`
	Content     string `bson:"content" json:"content"`
	AffiliateID string `bson:"affiliateId" json:"affiliateId"`
}

// LeadCc is the confirmation call / confirmation PDF link Superleap pushes to Zoho.
type LeadCc struct {
	Link      string `bson:"link" json:"link"`
	Type      string `bson:"type" json:"type"`
	Status    string `bson:"status" json:"status"`
	UpdatedAt int64  `bson:"updatedAt" json:"updatedAt"`
}

type Assignment struct {
	AuditorEmail string `bson:"auditorEmail" json:"auditorEmail"`
	AssignedAt   int64  `bson:"assignedAt" json:"assignedAt"`
	AssignedBy   string `bson:"assignedBy" json:"assignedBy"`
	Mode         string `bson:"mode" json:"mode"`
}

type AuditState struct {
	Status        string `bson:"status" json:"status"`
	Attempt       int    `bson:"attempt" json:"attempt"`
	LastAuditedAt int64  `bson:"lastAuditedAt" json:"lastAuditedAt"`
	CompletedAt   int64  `bson:"completedAt" json:"completedAt"`
	CompletedBy   string `bson:"completedBy" json:"completedBy"`
}

type RecheckSummary struct {
	Open         int   `bson:"open" json:"open"`
	Total        int   `bson:"total" json:"total"`
	LastRaisedAt int64 `bson:"lastRaisedAt" json:"lastRaisedAt"`
	LastClosedAt int64 `bson:"lastClosedAt" json:"lastClosedAt"`
}

// Audit is one audit attempt on a lead (salesAuditAudits): completed, or ended by a recheck.
type Audit struct {
	ID            string          `bson:"id" json:"id"`
	Program       string          `bson:"program" json:"-"`
	LeadID        string          `bson:"leadId" json:"leadId"`
	Attempt       int             `bson:"attempt" json:"attempt"`
	Auditor       Actor           `bson:"auditor" json:"auditor"`
	Outcome       string          `bson:"outcome" json:"outcome"`
	Checklist     []ChecklistItem `bson:"checklist" json:"checklist"`
	Comments      string          `bson:"comments" json:"comments"`
	MismatchCount int             `bson:"mismatchCount" json:"mismatchCount"`
	RecheckID     string          `bson:"recheckId" json:"recheckId"`
	SubmittedAt   int64           `bson:"submittedAt" json:"submittedAt"`
	Created       Created         `bson:"created" json:"-"`
	Deleted       bool            `bson:"deleted" json:"-"`
}

type ChecklistItem struct {
	Key     string `bson:"key" json:"key"`
	Label   string `bson:"label" json:"label"`
	Checked bool   `bson:"checked" json:"checked"`
}

// Recheck is a ticket raised on a lead (salesAuditRechecks). Zoho's recheckDetails are imported
// as rechecks with source "zoho" and their SRID as recheckNo.
type Recheck struct {
	ID        string `bson:"id" json:"id"`
	Program   string `bson:"program" json:"-"`
	RecheckNo string `bson:"recheckNo" json:"recheckNo"`
	Source    string `bson:"source" json:"source"`
	LeadID    string `bson:"leadId" json:"leadId"`
	LeadName  string `bson:"leadName" json:"leadName"`
	ZenID     string `bson:"zenId" json:"zenId"`
	AuditID   string `bson:"auditId" json:"auditId"`
	Attempt   int    `bson:"attempt" json:"attempt"`
	// Reasons are what the BDA must fix, one category and comment each. Category (the first
	// reason's) and Comments (every reason in one line) are kept for lists, mails and rechecks
	// stored before there could be several reasons; use core.ReasonsOf to read them.
	Category     string          `bson:"category" json:"category"`
	Comments     string          `bson:"comments" json:"comments"`
	Reasons      []RecheckReason `bson:"reasons" json:"reasons"`
	Status       string          `bson:"status" json:"status"`
	RaisedBy     Actor           `bson:"raisedBy" json:"raisedBy"`
	RaisedAt     int64           `bson:"raisedAt" json:"raisedAt"`
	BdaEmail     string          `bson:"bdaEmail" json:"bdaEmail"`
	BdmEmail     string          `bson:"bdmEmail" json:"bdmEmail"`
	AuditorEmail string          `bson:"auditorEmail" json:"auditorEmail"`
	Closed       *RecheckClosure `bson:"closed" json:"closed"`
	ReauditedAt  int64           `bson:"reauditedAt" json:"reauditedAt"`
	Alert        *Mail           `bson:"alert" json:"alert"`
	LastReminder *Mail           `bson:"lastReminder" json:"lastReminder"`
	Created      Created         `bson:"created" json:"-"`
	Deleted      bool            `bson:"deleted" json:"-"`
}

// RecheckReason is one thing to fix on a recheck.
type RecheckReason struct {
	Category string `bson:"category" json:"category"`
	Comments string `bson:"comments" json:"comments"`
}

type RecheckClosure struct {
	At   int64  `bson:"at" json:"at"`
	By   Actor  `bson:"by" json:"by"`
	Note string `bson:"note" json:"note"`
}

// Event is one step in a lead's timeline (salesAuditEvents).
type Event struct {
	ID      string         `bson:"id" json:"id"`
	Program string         `bson:"program" json:"-"`
	LeadID  string         `bson:"leadId" json:"leadId"`
	Type    string         `bson:"type" json:"type"`
	Actor   Actor          `bson:"actor" json:"actor"`
	At      int64          `bson:"at" json:"at"`
	Data    map[string]any `bson:"data" json:"data"`
	Created Created        `bson:"created" json:"-"`
	Deleted bool           `bson:"deleted" json:"-"`
}

// Notification is an in-app alert for one member (salesAuditNotifications).
type Notification struct {
	ID             string  `bson:"id" json:"id"`
	Program        string  `bson:"program" json:"-"`
	RecipientEmail string  `bson:"recipientEmail" json:"recipientEmail"`
	Type           string  `bson:"type" json:"type"`
	LeadID         string  `bson:"leadId" json:"leadId"`
	RecheckID      string  `bson:"recheckId" json:"recheckId"`
	Title          string  `bson:"title" json:"title"`
	Message        string  `bson:"message" json:"message"`
	Read           bool    `bson:"read" json:"read"`
	ReadAt         int64   `bson:"readAt" json:"readAt"`
	Created        Created `bson:"created" json:"created"`
	Deleted        bool    `bson:"deleted" json:"-"`
}

// Counter is a named sequence (salesAuditCounters), used for recheck numbers.
type Counter struct {
	ID      string  `bson:"id" json:"-"`
	Program string  `bson:"program" json:"-"`
	Name    string  `bson:"name" json:"-"`
	Value   int64   `bson:"value" json:"-"`
	Created Created `bson:"created" json:"-"`
	Deleted bool    `bson:"deleted" json:"-"`
}

// Mail is a logged mail as the frontend shows it.
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
	Body     string  `bson:"body" json:"-"`
	Delivery string  `bson:"delivery" json:"delivery"`
	Created  Created `bson:"created" json:"-"`
	Deleted  bool    `bson:"deleted" json:"-"`
}

// CcExtract is what was read from a lead's CC (salesAuditCcExtracts): fields for the comparison
// and, for call recordings, the transcript. Written by the cc package.
type CcExtract struct {
	ID            string    `bson:"id" json:"id"`
	Program       string    `bson:"program" json:"-"`
	LeadID        string    `bson:"leadId" json:"leadId"`
	Link          string    `bson:"link" json:"link"`
	Type          string    `bson:"type" json:"type"`
	Fields        []CcField `bson:"fields" json:"fields"`
	Transcript    []Line    `bson:"transcript" json:"transcript"`
	PointsCovered []string  `bson:"pointsCovered" json:"pointsCovered"`
	Mocked        bool      `bson:"mocked" json:"mocked"`
	ExtractedAt   int64     `bson:"extractedAt" json:"extractedAt"`
	Created       Created   `bson:"created" json:"-"`
	Deleted       bool      `bson:"deleted" json:"-"`
}

// CcField is one value read from the CC, keyed as the comparison rows are.
type CcField struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// Line is one transcript line.
type Line struct {
	Speaker string `bson:"speaker" json:"speaker"`
	At      string `bson:"at" json:"at"`
	Text    string `bson:"text" json:"text"`
}
