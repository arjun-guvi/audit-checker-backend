package actions

import (
	"context"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// LeadListParams is GET /leads: the lead filters plus filters on the leads' rechecks.
type LeadListParams struct {
	Query models.LeadQuery
	// Mine narrows an auditor or TL to their own leads (My Leads).
	Mine              bool
	RecheckRaised     models.Range
	RecheckClosed     models.Range
	RecheckCategories []string
	RecheckStatus     string
	AwaitingReaudit   bool
}

func filtersRechecks(p LeadListParams) bool {
	return core.IsSet(p.RecheckRaised) || core.IsSet(p.RecheckClosed) || len(p.RecheckCategories) > 0 ||
		p.RecheckStatus != "" || p.AwaitingReaudit
}

// scopeOf is what the member may see.
func scopeOf(ctx context.Context, member models.Member, mine bool) (models.Scope, []string, error) {
	team, err := TeamOf(ctx, member)
	if err != nil {
		return models.Scope{}, nil, err
	}
	return core.ScopeFor(member, team, mine), team, nil
}

// ListLeads is the All Leads / My Leads page for auditors and the leads list for BDAs and BDMs.
func ListLeads(ctx context.Context, member models.Member, params LeadListParams) (models.Page[models.Lead], error) {
	scope, _, err := scopeOf(ctx, member, params.Mine)
	if err != nil {
		return models.Page[models.Lead]{}, err
	}
	query := params.Query
	query.Scope = scope
	if filtersRechecks(params) {
		rechecks, err := store.FindRechecks(ctx, member.Program, models.RecheckQuery{
			Scope: scope, Status: params.RecheckStatus, Categories: params.RecheckCategories,
			Raised: params.RecheckRaised, ClosedIn: params.RecheckClosed, AwaitingReaudit: params.AwaitingReaudit,
		})
		if err != nil {
			return models.Page[models.Lead]{}, err
		}
		query.LeadIDs = []string{}
		seen := map[string]bool{}
		for _, recheck := range rechecks {
			if !seen[recheck.LeadID] {
				seen[recheck.LeadID] = true
				query.LeadIDs = append(query.LeadIDs, recheck.LeadID)
			}
		}
	}
	if query.PageSize <= 0 || query.PageSize > 200 {
		query.PageSize = 50
	}
	query.Page = max(query.Page, 1)
	leads, total, err := store.FindLeads(ctx, member.Program, query)
	return models.Page[models.Lead]{Items: leads, Total: total, Page: query.Page, PageSize: query.PageSize}, err
}

// loadLead fetches a lead the member may see.
func loadLead(ctx context.Context, member models.Member, leadID string) (models.Lead, []string, error) {
	lead, err := store.FindLead(ctx, member.Program, leadID)
	if err != nil {
		return lead, nil, orNotFound(err, "Lead")
	}
	scope, team, err := scopeOf(ctx, member, false)
	if err != nil {
		return lead, nil, err
	}
	if !core.LeadInScope(scope, lead) {
		return lead, nil, notFound("Lead")
	}
	return lead, team, nil
}

// LeadDetail is GET /leads/:leadId.
func LeadDetail(ctx context.Context, member models.Member, leadID string) (models.LeadDetail, error) {
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return models.LeadDetail{}, err
	}
	rechecks, err := store.FindRechecks(ctx, member.Program, models.RecheckQuery{LeadIDs: []string{lead.ID}})
	if err != nil {
		return models.LeadDetail{}, err
	}
	audits, err := store.FindAudits(ctx, member.Program, models.AuditQuery{LeadID: lead.ID})
	if err != nil {
		return models.LeadDetail{}, err
	}
	return models.LeadDetail{Lead: lead, Rechecks: rechecks, Audits: audits, Actions: core.ActionsFor(member, lead)}, nil
}

// Timeline is GET /leads/:leadId/timeline: every step, oldest first.
func Timeline(ctx context.Context, member models.Member, leadID string) ([]models.Event, error) {
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return nil, err
	}
	return store.FindEvents(ctx, member.Program, lead.ID)
}

// refreshWorkflow recomputes the lead's recheck summary and status and saves the workflow.
func refreshWorkflow(ctx context.Context, lead models.Lead) (models.Lead, error) {
	rechecks, err := store.FindRechecks(ctx, lead.Program, models.RecheckQuery{LeadIDs: []string{lead.ID}})
	if err != nil {
		return lead, err
	}
	lead.RecheckSummary = core.SummarizeRechecks(rechecks)
	lead.Audit = core.ReconcileStatus(lead.Audit, lead.Assignment != nil, lead.RecheckSummary)
	return lead, store.UpdateLeadWorkflow(ctx, lead)
}

// markReaudited stamps the lead's closed rechecks that were awaiting a new audit.
func markReaudited(ctx context.Context, lead models.Lead, at int64) error {
	rechecks, err := store.FindRechecks(ctx, lead.Program, models.RecheckQuery{LeadIDs: []string{lead.ID}, AwaitingReaudit: true})
	if err != nil {
		return err
	}
	for _, recheck := range rechecks {
		recheck.ReauditedAt = at
		if err := store.ReplaceRecheck(ctx, recheck); err != nil {
			return err
		}
	}
	return nil
}
