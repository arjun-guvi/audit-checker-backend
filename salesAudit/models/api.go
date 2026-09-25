package models

// Request filters and response shapes of the API.

// Scope limits which leads and rechecks a query returns. The zero value means everything.
// When BdmEmail is set a row matches if its BDM is that email or its BDA is in BdaEmails.
type Scope struct {
	AuditorEmail string
	BdaEmails    []string
	BdmEmail     string
	// None matches nothing (a member with no team).
	None bool
}

// Range is a [From, To) window in Unix seconds; a zero bound is open.
type Range struct {
	From int64
	To   int64
}

// LeadQuery filters the leads list.
type LeadQuery struct {
	Scope         Scope
	LeadIDs       []string // nil means any; empty means none
	AuditStatuses []string
	// Each list matches any of its values; an empty list means any.
	Regions       []string
	AuditorEmails []string
	BdaEmails     []string
	CcStatuses    []string
	Search        string
	Completed     Range
	Unassigned    bool
	Page          int
	PageSize      int
}

// RecheckQuery filters rechecks.
type RecheckQuery struct {
	Scope      Scope
	LeadIDs    []string // nil means any
	Status     string
	Categories []string // any of; empty means any
	// Each list matches any of its values; an empty list means any.
	AuditorEmails []string
	BdaEmails     []string
	// Search matches the recheck number, lead name, Zen ID or comments.
	Search   string
	Raised   Range
	ClosedIn Range
	// AwaitingReaudit: closed and the lead not audited since.
	AwaitingReaudit bool
	// CcUpdatedOpen: open, and the lead's CC was updated after it was raised.
	CcUpdatedOpen bool
}

// AuditQuery filters audit attempts.
type AuditQuery struct {
	LeadID        string
	AuditorEmails []string
	Submitted     Range
}

// Page is one page of results.
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// CurrentUser is GET /me: the member and what they may do.
type CurrentUser struct {
	Member
	Permissions map[string]bool `json:"permissions"`
	TeamEmails  []string        `json:"teamEmails"`
}

// LeadDetail is GET /leads/:leadId, grouped as the lead page shows it.
type LeadDetail struct {
	Lead     Lead      `json:"lead"`
	Rechecks []Recheck `json:"rechecks"`
	Audits   []Audit   `json:"audits"`
	Actions  Actions   `json:"actions"`
}

// Actions says which buttons the signed-in member gets on a lead.
type Actions struct {
	CanAudit        bool   `json:"canAudit"`
	CanComplete     bool   `json:"canComplete"`
	CanRaiseRecheck bool   `json:"canRaiseRecheck"`
	CanReaudit      bool   `json:"canReaudit"`
	CanReassign     bool   `json:"canReassign"`
	CanTakeUp       bool   `json:"canTakeUp"`
	BlockedReason   string `json:"blockedReason"`
}

// ComparisonRow is one field of the side-by-side audit view.
type ComparisonRow struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Label   string `json:"label"`
	DbValue string `json:"dbValue"`
	CcValue string `json:"ccValue"`
	Match   bool   `json:"match"`
}

// AuditView is GET /leads/:leadId/audit.
type AuditView struct {
	Lead          Lead            `json:"lead"`
	Cc            LeadCc          `json:"cc"`
	Comparison    []ComparisonRow `json:"comparison"`
	MismatchCount int             `json:"mismatchCount"`
	PointsCovered []string        `json:"pointsCovered"`
	Checklist     []ChecklistItem `json:"checklist"`
	Rechecks      []Recheck       `json:"rechecks"`
	Actions       Actions         `json:"actions"`
}

// CcVerification is GET /leads/:leadId/cc-verification.
type CcVerification struct {
	Lead    Lead      `json:"lead"`
	Cc      LeadCc    `json:"cc"`
	Extract CcExtract `json:"extract"`
	// PreviewURL is an embeddable PDF preview when the CC is a Drive file.
	PreviewURL string `json:"previewUrl"`
}

// AuditorStats is one auditor's row on the TL dashboard.
type AuditorStats struct {
	Email          string     `json:"email"`
	Name           string     `json:"name"`
	Region         string     `json:"region"`
	Available      bool       `json:"available"`
	Assigned       int        `json:"assigned"`
	Pending        int        `json:"pending"`
	AuditsDone     int        `json:"auditsDone"`
	Completed      int        `json:"completed"`
	RechecksRaised int        `json:"rechecksRaised"`
	LastAuditAt    int64      `json:"lastAuditAt"`
	Daily          []DayCount `json:"daily"`
}

type DayCount struct {
	Date           string `json:"date"`
	AuditsDone     int    `json:"auditsDone"`
	Completed      int    `json:"completed"`
	RechecksRaised int    `json:"rechecksRaised"`
}

// TeamDashboard is GET /dashboard/auditor-team.
type TeamDashboard struct {
	Totals   AuditorStats   `json:"totals"`
	Auditors []AuditorStats `json:"auditors"`
	Recent   []Audit        `json:"recent"`
}

// BdaStats is one BDA's figures (or a BDM's total).
type BdaStats struct {
	Email          string         `json:"email"`
	Name           string         `json:"name"`
	Leads          int            `json:"leads"`
	AuditCompleted int            `json:"auditCompleted"`
	RechecksOpen   int            `json:"rechecksOpen"`
	RechecksClosed int            `json:"rechecksClosed"`
	ByCategory     map[string]int `json:"byCategory"`
}

// BdaDashboard is GET /dashboard/bda.
type BdaDashboard struct {
	Totals BdaStats   `json:"totals"`
	Bdas   []BdaStats `json:"bdas"`
}
