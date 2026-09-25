package actions

import (
	"context"
	"strings"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// TeamDashboard is GET /dashboard/auditor-team: the auditors under the TL (every auditor when none
// has a manager set), optionally one auditor, in a date window.
func TeamDashboard(ctx context.Context, member models.Member, auditorEmail string, span models.Range) (models.TeamDashboard, error) {
	if !core.Can(member.Role, core.ActionTeamDashboard) {
		return models.TeamDashboard{}, forbidden("Only the auditor TL can see the team dashboard")
	}
	all, err := store.FindMembers(ctx, member.Program, models.RoleAuditor)
	if err != nil {
		return models.TeamDashboard{}, err
	}
	auditors := []models.Member{}
	for _, auditor := range all {
		if strings.EqualFold(auditor.ManagerEmail, member.Email) {
			auditors = append(auditors, auditor)
		}
	}
	if len(auditors) == 0 {
		auditors = all
	}
	if auditorEmail != "" {
		picked := []models.Member{}
		for _, auditor := range auditors {
			if strings.EqualFold(auditor.Email, auditorEmail) {
				picked = append(picked, auditor)
			}
		}
		auditors = picked
	}
	emails := []string{}
	for _, auditor := range auditors {
		emails = append(emails, auditor.Email)
	}
	leads, _, err := store.FindLeads(ctx, member.Program, models.LeadQuery{})
	if err != nil {
		return models.TeamDashboard{}, err
	}
	audits, err := store.FindAudits(ctx, member.Program, models.AuditQuery{AuditorEmails: emails, Submitted: span})
	if err != nil {
		return models.TeamDashboard{}, err
	}
	rechecks, err := store.FindRechecks(ctx, member.Program, models.RecheckQuery{Raised: span})
	if err != nil {
		return models.TeamDashboard{}, err
	}
	return core.TeamStats(auditors, leads, audits, rechecks, span), nil
}

// BdaDashboard is GET /dashboard/bda: a BDA's own figures, or a BDM's per BDA (optionally one).
func BdaDashboard(ctx context.Context, member models.Member, bdaEmail string, span models.Range) (models.BdaDashboard, error) {
	if !core.Can(member.Role, core.ActionBdaDashboard) {
		return models.BdaDashboard{}, forbidden("Only BDAs and BDMs have this dashboard")
	}
	scope, team, err := scopeOf(ctx, member, false)
	if err != nil {
		return models.BdaDashboard{}, err
	}
	members, err := store.FindMembers(ctx, member.Program, models.RoleBda)
	if err != nil {
		return models.BdaDashboard{}, err
	}
	bdas := []models.Member{}
	for _, bda := range members {
		mine := strings.EqualFold(bda.Email, member.Email) || (member.Role == models.RoleBdm && containsEmail(team, bda.Email))
		if mine && (bdaEmail == "" || strings.EqualFold(bda.Email, bdaEmail)) {
			bdas = append(bdas, bda)
		}
	}
	leadQuery := models.LeadQuery{Scope: scope, BdaEmails: core.NonEmpty(bdaEmail)}
	leads, _, err := store.FindLeads(ctx, member.Program, leadQuery)
	if err != nil {
		return models.BdaDashboard{}, err
	}
	rechecks, err := store.FindRechecks(ctx, member.Program, models.RecheckQuery{Scope: scope, BdaEmails: core.NonEmpty(bdaEmail)})
	if err != nil {
		return models.BdaDashboard{}, err
	}
	return core.BdaStatsOf(bdas, leads, rechecks, span), nil
}

func containsEmail(emails []string, email string) bool {
	for _, candidate := range emails {
		if strings.EqualFold(candidate, email) {
			return true
		}
	}
	return false
}
