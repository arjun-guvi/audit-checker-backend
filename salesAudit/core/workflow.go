package core

import (
	"errors"
	"strings"

	"auditApp/salesAudit/models"
)

// Roles, permissions and the audit status machine.

// Things a member may do.
const (
	ActionViewAllLeads  = "viewAllLeads"
	ActionAudit         = "audit"
	ActionRaiseRecheck  = "raiseRecheck"
	ActionCloseRecheck  = "closeRecheck"
	ActionReassign      = "reassign"
	ActionTakeUp        = "takeUp"
	ActionManageMembers = "manageMembers"
	ActionTeamDashboard = "teamDashboard"
	ActionBdaDashboard  = "bdaDashboard"
	ActionViewCcStatus  = "viewCcStatus"
	ActionRunAssignment = "runAssignment"
	ActionSendReminder  = "sendReminder"
)

var roleActions = map[string][]string{
	models.RoleAuditorTl: {ActionViewAllLeads, ActionAudit, ActionRaiseRecheck, ActionCloseRecheck,
		ActionReassign, ActionManageMembers, ActionTeamDashboard, ActionViewCcStatus, ActionRunAssignment, ActionSendReminder},
	models.RoleAuditor: {ActionViewAllLeads, ActionAudit, ActionRaiseRecheck, ActionCloseRecheck,
		ActionReassign, ActionTakeUp, ActionViewCcStatus, ActionSendReminder},
	models.RoleBdm: {ActionCloseRecheck, ActionBdaDashboard},
	models.RoleBda: {ActionCloseRecheck, ActionBdaDashboard},
}

// Can reports whether a role may take an action.
func Can(role, action string) bool {
	for _, allowed := range roleActions[role] {
		if allowed == action {
			return true
		}
	}
	return false
}

// Permissions lists every action with whether the role may take it (for GET /me).
func Permissions(role string) map[string]bool {
	result := map[string]bool{}
	for _, actions := range roleActions {
		for _, action := range actions {
			result[action] = Can(role, action)
		}
	}
	return result
}

// IsAuditRole is true for auditors and auditor TLs.
func IsAuditRole(role string) bool {
	return role == models.RoleAuditor || role == models.RoleAuditorTl
}

// ActorOf is the member as an event or ticket actor.
func ActorOf(member models.Member) models.Actor {
	return models.Actor{Email: member.Email, Name: member.Name, Role: member.Role}
}

// SystemActor is the actor of jobs and the Zoho import.
var SystemActor = models.Actor{Email: models.SystemUser, Name: "System", Role: models.RoleSystem}

// TeamEmails is the emails of the members who report to the manager.
func TeamEmails(managerEmail string, members []models.Member) []string {
	emails := []string{}
	for _, member := range members {
		if strings.EqualFold(member.ManagerEmail, managerEmail) {
			emails = append(emails, member.Email)
		}
	}
	return emails
}

// ScopeFor is what the member may see. mine narrows an auditor or TL to their own leads.
func ScopeFor(member models.Member, team []string, mine bool) models.Scope {
	switch member.Role {
	case models.RoleAuditorTl, models.RoleAuditor:
		if mine {
			return models.Scope{AuditorEmail: member.Email}
		}
		return models.Scope{}
	case models.RoleBdm:
		return models.Scope{BdmEmail: member.Email, BdaEmails: team}
	case models.RoleBda:
		return models.Scope{BdaEmails: []string{member.Email}}
	}
	return models.Scope{None: true}
}

// InScope reports whether a lead-like row (auditor, BDA, BDM) falls in a scope.
func InScope(scope models.Scope, auditorEmail, bdaEmail, bdmEmail string) bool {
	if scope.None {
		return false
	}
	if scope.AuditorEmail != "" && !strings.EqualFold(scope.AuditorEmail, auditorEmail) {
		return false
	}
	if scope.BdmEmail != "" {
		return strings.EqualFold(scope.BdmEmail, bdmEmail) || containsFold(scope.BdaEmails, bdaEmail)
	}
	if scope.BdaEmails != nil {
		return containsFold(scope.BdaEmails, bdaEmail)
	}
	return true
}

func containsFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}

// AuditorOf is the lead's assigned auditor email, "" when unassigned.
func AuditorOf(lead models.Lead) string {
	if lead.Assignment == nil {
		return ""
	}
	return lead.Assignment.AuditorEmail
}

// LeadInScope reports whether the member's scope covers the lead.
func LeadInScope(scope models.Scope, lead models.Lead) bool {
	return InScope(scope, AuditorOf(lead), lead.BdaEmail, lead.BdmEmail)
}

// Errors the workflow returns; the handlers send them as 400/403.
var (
	ErrForbidden           = errors.New("You are not allowed to do this")
	ErrNotAssigned         = errors.New("Only the lead's auditor or the auditor TL can audit this lead")
	ErrAlreadyCompleted    = errors.New("This lead's audit is already completed")
	ErrRecheckOpen         = errors.New("Close the open recheck before completing the audit")
	ErrPaymentNotReady     = errors.New("This lead cannot be audited yet")
	ErrRecheckNotOpen      = errors.New("This recheck is already closed")
	ErrUnknownCategory     = errors.New("Unknown recheck category")
	ErrCommentsRequired    = errors.New("Comments are required")
	ErrChecklistIncomplete = errors.New("Tick every checklist item before completing the audit")
	ErrNotAnAuditor        = errors.New("That member is not an available auditor")
)

// CanWorkOn: the lead's assigned auditor or any auditor TL may audit it.
func CanWorkOn(member models.Member, lead models.Lead) bool {
	if member.Role == models.RoleAuditorTl {
		return true
	}
	return member.Role == models.RoleAuditor && strings.EqualFold(AuditorOf(lead), member.Email)
}

// ActionsFor says which buttons the member gets on the lead.
func ActionsFor(member models.Member, lead models.Lead) models.Actions {
	actions := models.Actions{}
	works := CanWorkOn(member, lead)
	status := lead.Audit.Status
	auditable := status == models.AuditPending || status == models.AuditRecheckClosed
	actions.CanAudit = IsAuditRole(member.Role)
	actions.CanRaiseRecheck = works && status != models.AuditCompleted && status != models.AuditUnassigned
	actions.CanComplete = works && auditable && lead.Payment.Ready
	actions.CanReaudit = works && status == models.AuditRecheckClosed
	actions.CanReassign = Can(member.Role, ActionReassign) && status != models.AuditCompleted
	actions.CanTakeUp = member.Role == models.RoleAuditor && status != models.AuditCompleted &&
		!strings.EqualFold(AuditorOf(lead), member.Email)
	switch {
	case !IsAuditRole(member.Role):
	case !works:
		actions.BlockedReason = "This lead is assigned to another auditor"
	case status == models.AuditUnassigned:
		actions.BlockedReason = "This lead is not assigned yet"
	case status == models.AuditCompleted:
		actions.BlockedReason = "The audit is completed"
	case status == models.AuditRecheckOpen:
		actions.BlockedReason = "A recheck is open; it can be audited again once it is closed"
	case !lead.Payment.Ready:
		actions.BlockedReason = lead.Payment.Shortfall
	}
	return actions
}

// CheckComplete validates completing the audit.
func CheckComplete(member models.Member, lead models.Lead) error {
	switch {
	case !CanWorkOn(member, lead):
		return ErrNotAssigned
	case lead.Audit.Status == models.AuditCompleted:
		return ErrAlreadyCompleted
	case lead.Audit.Status == models.AuditRecheckOpen || lead.RecheckSummary.Open > 0:
		return ErrRecheckOpen
	case lead.Audit.Status == models.AuditUnassigned:
		return ErrNotAssigned
	case !lead.Payment.Ready:
		return errors.New(ErrPaymentNotReady.Error() + ": " + lead.Payment.Shortfall)
	}
	return nil
}

// CheckRaiseRecheck validates raising a recheck.
func CheckRaiseRecheck(member models.Member, lead models.Lead, category, comments string) error {
	switch {
	case !CanWorkOn(member, lead):
		return ErrNotAssigned
	case lead.Audit.Status == models.AuditCompleted:
		return ErrAlreadyCompleted
	case lead.Audit.Status == models.AuditUnassigned:
		return ErrNotAssigned
	}
	if _, known := models.RecheckCategories[category]; !known {
		return ErrUnknownCategory
	}
	if strings.TrimSpace(comments) == "" {
		return ErrCommentsRequired
	}
	return nil
}

// CanCloseRecheck: the lead's BDA or BDM, a BDM whose BDA it is, any auditor or auditor TL.
func CanCloseRecheck(member models.Member, team []string, recheck models.Recheck) bool {
	switch member.Role {
	case models.RoleAuditor, models.RoleAuditorTl:
		return true
	case models.RoleBda:
		return strings.EqualFold(recheck.BdaEmail, member.Email)
	case models.RoleBdm:
		return strings.EqualFold(recheck.BdmEmail, member.Email) || containsFold(team, recheck.BdaEmail)
	}
	return false
}

// SummarizeRechecks recomputes a lead's recheck counters.
func SummarizeRechecks(rechecks []models.Recheck) models.RecheckSummary {
	summary := models.RecheckSummary{}
	for _, recheck := range rechecks {
		summary.Total++
		if recheck.Status == models.RecheckOpen {
			summary.Open++
		}
		summary.LastRaisedAt = max(summary.LastRaisedAt, recheck.RaisedAt)
		if recheck.Closed != nil {
			summary.LastClosedAt = max(summary.LastClosedAt, recheck.Closed.At)
		}
	}
	return summary
}

// ReconcileStatus keeps the audit status in step with the lead's rechecks and assignment.
func ReconcileStatus(audit models.AuditState, assigned bool, summary models.RecheckSummary) models.AuditState {
	switch {
	case audit.Status == models.AuditCompleted:
	case summary.Open > 0:
		audit.Status = models.AuditRecheckOpen
	case audit.Status == models.AuditRecheckOpen:
		audit.Status = models.AuditRecheckClosed
	case !assigned:
		audit.Status = models.AuditUnassigned
	case audit.Status == models.AuditUnassigned || audit.Status == "":
		audit.Status = models.AuditPending
	}
	return audit
}

// Checklist is the audit checklist the auditor ticks before completing.
func Checklist() []models.ChecklistItem {
	return []models.ChecklistItem{
		{Key: "personalDetails", Label: "Personal details (name, email, phone) match the CC"},
		{Key: "courseDetails", Label: "Course, batch and mode of study match the CC"},
		{Key: "courseFee", Label: "Course fee and discount are approved and match the CC"},
		{Key: "paymentsVerified", Label: "Every payment is verified in Zoho Books"},
		{Key: "paymentPlan", Label: "Payment plan / EMI terms match what the learner agreed to"},
		{Key: "admissionForm", Label: "Admission form is complete"},
		{Key: "termsAccepted", Label: "Terms & conditions are accepted"},
		{Key: "ccPointsCovered", Label: "Every mandatory point was covered in the CC"},
	}
}

// ValidateChecklist checks every template item is ticked and returns the stored checklist.
func ValidateChecklist(ticked []models.ChecklistItem) ([]models.ChecklistItem, error) {
	checked := map[string]bool{}
	for _, item := range ticked {
		checked[item.Key] = item.Checked
	}
	result := Checklist()
	for index := range result {
		if !checked[result[index].Key] {
			return nil, ErrChecklistIncomplete
		}
		result[index].Checked = true
	}
	return result, nil
}
