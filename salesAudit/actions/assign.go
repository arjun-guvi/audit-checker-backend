package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// AssignResult is what one assignment run did.
type AssignResult struct {
	Assigned   int            `json:"assigned"`
	Unassigned int            `json:"unassigned"`
	ByAuditor  map[string]int `json:"byAuditor"`
}

// AssignPending gives every unassigned lead to an available auditor of its region (see
// core.AssignLeads), then tells each auditor once, in the portal and by mail.
func AssignPending(ctx context.Context, program string, actor models.Actor) (AssignResult, error) {
	result := AssignResult{ByAuditor: map[string]int{}}
	pending, _, err := store.FindLeads(ctx, program, models.LeadQuery{Unassigned: true, AuditStatuses: []string{models.AuditUnassigned}})
	if err != nil || len(pending) == 0 {
		return result, err
	}
	auditors, err := store.FindMembers(ctx, program, models.RoleAuditor)
	if err != nil {
		return result, err
	}
	all, _, err := store.FindLeads(ctx, program, models.LeadQuery{})
	if err != nil {
		return result, err
	}
	plan := core.AssignLeads(pending, auditors, core.OpenLoad(all), Random)

	now := nowSeconds()
	given := map[string][]models.Lead{}
	for _, lead := range pending {
		auditor, ok := plan[lead.ID]
		if !ok {
			result.Unassigned++
			continue
		}
		lead.Assignment = &models.Assignment{AuditorEmail: auditor, AssignedAt: now, AssignedBy: actor.Email, Mode: models.AssignAuto}
		if _, err := refreshWorkflow(ctx, lead); err != nil {
			return result, err
		}
		RecordEvent(ctx, program, lead.ID, models.EventAssigned, actor, now, map[string]any{
			"auditorEmail": auditor, "mode": models.AssignAuto, "region": lead.Region,
		})
		given[auditor] = append(given[auditor], lead)
		result.Assigned++
		result.ByAuditor[auditor]++
	}
	for auditor, leads := range given {
		Notify(ctx, program, []string{auditor}, models.Notification{
			Type: models.NotifyLeadsAssigned, Title: core.LeadsAssignedSubject(len(leads)),
			Message: "New leads are waiting in My Leads.", Created: models.Created{By: actor.Email},
		})
		if _, err := Mail(ctx, program, actor.Email, "", models.AlertLeadsAssigned, models.TriggerAuto,
			[]string{auditor}, core.LeadsAssignedSubject(len(leads)), core.LeadsAssignedBody(leads)); err != nil {
			return result, err
		}
	}
	return result, nil
}

// Reassign is POST /leads/:leadId/reassign: moves the lead to another auditor.
func Reassign(ctx context.Context, member models.Member, leadID, auditorEmail string) (models.Lead, error) {
	if !core.Can(member.Role, core.ActionReassign) {
		return models.Lead{}, forbidden("Only auditors can reassign leads")
	}
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return lead, err
	}
	if lead.Audit.Status == models.AuditCompleted {
		return lead, badRequest(core.ErrAlreadyCompleted)
	}
	auditor, err := store.FindMemberByEmail(ctx, member.Program, strings.TrimSpace(auditorEmail))
	if store.IsNotFound(err) || (err == nil && auditor.Role != models.RoleAuditor) {
		return lead, badRequest(core.ErrNotAnAuditor)
	}
	if err != nil {
		return lead, err
	}
	return moveLead(ctx, member, lead, auditor.Email, models.AssignManual, models.EventReassigned)
}

// TakeUp is POST /leads/:leadId/take-up: an auditor takes a lead over (e.g. its auditor is away).
func TakeUp(ctx context.Context, member models.Member, leadID string) (models.Lead, error) {
	if !core.Can(member.Role, core.ActionTakeUp) {
		return models.Lead{}, forbidden("Only auditors can take up leads")
	}
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return lead, err
	}
	if lead.Audit.Status == models.AuditCompleted {
		return lead, badRequest(core.ErrAlreadyCompleted)
	}
	if strings.EqualFold(core.AuditorOf(lead), member.Email) {
		return lead, badRequest(errors.New("This lead is already yours"))
	}
	return moveLead(ctx, member, lead, member.Email, models.AssignTakeUp, models.EventTakenUp)
}

func moveLead(ctx context.Context, member models.Member, lead models.Lead, auditorEmail, mode, eventType string) (models.Lead, error) {
	now := nowSeconds()
	previous := core.AuditorOf(lead)
	lead.Assignment = &models.Assignment{AuditorEmail: strings.ToLower(auditorEmail), AssignedAt: now, AssignedBy: member.Email, Mode: mode}
	lead, err := refreshWorkflow(ctx, lead)
	if err != nil {
		return lead, err
	}
	// Open rechecks follow the lead to its new auditor.
	rechecks, err := store.FindRechecks(ctx, member.Program, models.RecheckQuery{LeadIDs: []string{lead.ID}})
	if err != nil {
		return lead, err
	}
	for _, recheck := range rechecks {
		if recheck.Status == models.RecheckOpen || recheck.ReauditedAt == 0 {
			recheck.AuditorEmail = lead.Assignment.AuditorEmail
			if err := store.ReplaceRecheck(ctx, recheck); err != nil {
				return lead, err
			}
		}
	}
	RecordEvent(ctx, member.Program, lead.ID, eventType, core.ActorOf(member), now, map[string]any{
		"from": previous, "auditorEmail": lead.Assignment.AuditorEmail, "mode": mode,
	})
	recipients := []string{lead.Assignment.AuditorEmail, previous}
	Notify(ctx, member.Program, removeEmail(recipients, member.Email), models.Notification{
		Type: models.NotifyLeadReassigned, LeadID: lead.ID,
		Title:   fmt.Sprintf("%s moved to %s", lead.Personal.Name, lead.Assignment.AuditorEmail),
		Message: fmt.Sprintf("%s moved this lead from %s.", core.FirstNonEmpty(member.Name, member.Email), core.FirstNonEmpty(previous, "no one")),
		Created: models.Created{By: member.UserHash},
	})
	return lead, nil
}

func removeEmail(emails []string, remove string) []string {
	result := []string{}
	for _, email := range emails {
		if !strings.EqualFold(email, remove) {
			result = append(result, email)
		}
	}
	return result
}
