package core

import (
	"strings"

	"auditApp/salesAudit/models"
)

// In-memory versions of the store's query filters (used by the fake store; the Mongo filters in
// store/ must agree with them).

// MatchLead reports whether a lead passes a lead query (paging aside).
func MatchLead(query models.LeadQuery, lead models.Lead) bool {
	if !LeadInScope(query.Scope, lead) {
		return false
	}
	if query.LeadIDs != nil && !containsFold(query.LeadIDs, lead.ID) {
		return false
	}
	if len(query.AuditStatuses) > 0 && !containsFold(query.AuditStatuses, lead.Audit.Status) {
		return false
	}
	if len(query.Regions) > 0 && !containsFold(query.Regions, lead.Region) {
		return false
	}
	if len(query.AuditorEmails) > 0 && !containsFold(query.AuditorEmails, AuditorOf(lead)) {
		return false
	}
	if len(query.BdaEmails) > 0 && !containsFold(query.BdaEmails, lead.BdaEmail) {
		return false
	}
	if len(query.CcStatuses) > 0 && !containsFold(query.CcStatuses, lead.Cc.Status) {
		return false
	}
	if query.Unassigned && lead.Assignment != nil {
		return false
	}
	if IsSet(query.Completed) && !InRange(query.Completed, lead.Audit.CompletedAt) {
		return false
	}
	if search := strings.ToLower(strings.TrimSpace(query.Search)); search != "" {
		haystack := strings.ToLower(strings.Join([]string{lead.Personal.Name, lead.Personal.Email, lead.Personal.Phone, lead.ZenID}, " "))
		if !strings.Contains(haystack, search) {
			return false
		}
	}
	return true
}

// MatchRecheck reports whether a recheck passes a recheck query.
func MatchRecheck(query models.RecheckQuery, recheck models.Recheck) bool {
	if !InScope(query.Scope, recheck.AuditorEmail, recheck.BdaEmail, recheck.BdmEmail) {
		return false
	}
	if query.LeadIDs != nil && !containsFold(query.LeadIDs, recheck.LeadID) {
		return false
	}
	if query.Status != "" && query.Status != recheck.Status {
		return false
	}
	if len(query.Categories) > 0 && !anyContained(query.Categories, ReasonCategories(ReasonsOf(recheck))) {
		return false
	}
	if len(query.AuditorEmails) > 0 && !containsFold(query.AuditorEmails, recheck.AuditorEmail) {
		return false
	}
	if len(query.BdaEmails) > 0 && !containsFold(query.BdaEmails, recheck.BdaEmail) {
		return false
	}
	if search := strings.ToLower(strings.TrimSpace(query.Search)); search != "" {
		haystack := strings.ToLower(strings.Join([]string{recheck.RecheckNo, recheck.LeadName, recheck.ZenID, recheck.Comments}, " "))
		if !strings.Contains(haystack, search) {
			return false
		}
	}
	if IsSet(query.Raised) && !InRange(query.Raised, recheck.RaisedAt) {
		return false
	}
	if IsSet(query.ClosedIn) && (recheck.Closed == nil || !InRange(query.ClosedIn, recheck.Closed.At)) {
		return false
	}
	if query.AwaitingReaudit && (recheck.Status != models.RecheckClosed || recheck.ReauditedAt != 0) {
		return false
	}
	if query.CcUpdatedOpen && (recheck.Status != models.RecheckOpen || recheck.CcUpdatedAt == 0) {
		return false
	}
	return true
}

// MatchAudit reports whether an audit attempt passes an audit query.
func MatchAudit(query models.AuditQuery, audit models.Audit) bool {
	if query.LeadID != "" && query.LeadID != audit.LeadID {
		return false
	}
	if query.AuditorEmails != nil && !containsFold(query.AuditorEmails, audit.Auditor.Email) {
		return false
	}
	return !IsSet(query.Submitted) || InRange(query.Submitted, audit.SubmittedAt)
}

// anyContained reports whether any of values is in list.
func anyContained(list, values []string) bool {
	for _, value := range values {
		if containsFold(list, value) {
			return true
		}
	}
	return false
}
