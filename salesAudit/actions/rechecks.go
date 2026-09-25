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

// RecheckListParams is GET /rechecks.
type RecheckListParams struct {
	Query models.RecheckQuery
	// Mine narrows an auditor to rechecks on their own leads; auditors see only theirs by default.
	Mine bool
}

// ListRechecks returns the tickets the member may see.
func ListRechecks(ctx context.Context, member models.Member, params RecheckListParams) ([]models.Recheck, error) {
	scope, _, err := scopeOf(ctx, member, params.Mine)
	if err != nil {
		return nil, err
	}
	query := params.Query
	query.Scope = scope
	return store.FindRechecks(ctx, member.Program, query)
}

// RaiseRecheck is POST /rechecks: ends the current audit attempt with a recheck listing every
// reason to fix, alerts the BDA and BDM in the portal and by mail (with the recheck number and the
// lead).
func RaiseRecheck(ctx context.Context, member models.Member, leadID string, reasons []models.RecheckReason) (models.Recheck, error) {
	if !core.Can(member.Role, core.ActionRaiseRecheck) {
		return models.Recheck{}, forbidden("Only auditors can raise a recheck")
	}
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return models.Recheck{}, err
	}
	cleaned := make([]models.RecheckReason, len(reasons))
	for i, reason := range reasons {
		cleaned[i] = models.RecheckReason{Category: reason.Category, Comments: strings.TrimSpace(reason.Comments)}
	}
	reasons = cleaned
	if err := core.CheckRaiseRecheck(member, lead, reasons); err != nil {
		if errors.Is(err, core.ErrNotAssigned) {
			return models.Recheck{}, forbidden(err.Error())
		}
		return models.Recheck{}, badRequest(err)
	}
	extract, _, err := ccExtractOf(ctx, lead)
	if err != nil {
		return models.Recheck{}, err
	}
	_, mismatches := core.Compare(lead, extract)

	now := nowSeconds()
	sequence, err := store.NextSequence(ctx, member.Program, "recheckNo", now)
	if err != nil {
		return models.Recheck{}, err
	}
	attempt := lead.Audit.Attempt + 1
	recheck := models.Recheck{
		ID: core.NewID(), Program: member.Program, RecheckNo: core.RecheckNo(sequence), Source: models.SourcePortal,
		LeadID: lead.ID, LeadName: lead.Personal.Name, ZenID: lead.ZenID, Attempt: attempt,
		Category: reasons[0].Category, Comments: core.SummarizeReasons(reasons), Reasons: reasons, Status: models.RecheckOpen,
		RaisedBy: core.ActorOf(member), RaisedAt: now, BdaEmail: lead.BdaEmail, BdmEmail: lead.BdmEmail,
		AuditorEmail: core.AuditorOf(lead), Created: models.Created{At: now, By: member.UserHash},
	}
	audit := models.Audit{
		ID: core.NewID(), Program: member.Program, LeadID: lead.ID, Attempt: attempt, Auditor: core.ActorOf(member),
		Outcome: models.OutcomeRecheckRaised, Checklist: []models.ChecklistItem{}, Comments: recheck.Comments,
		MismatchCount: mismatches, RecheckID: recheck.ID, SubmittedAt: now, Created: models.Created{At: now, By: member.UserHash},
	}
	recheck.AuditID = audit.ID

	mail, err := Mail(ctx, member.Program, member.UserHash, lead.ID, models.AlertRecheck, models.TriggerAuto,
		[]string{lead.BdaEmail, lead.BdmEmail}, core.RecheckSubject(recheck), core.RecheckBody(recheck, lead))
	if err != nil {
		return recheck, err
	}
	recheck.Alert = &mail
	if err := store.InsertRecheck(ctx, recheck); err != nil {
		return recheck, err
	}
	if err := store.InsertAudit(ctx, audit); err != nil {
		return recheck, err
	}
	if err := markReaudited(ctx, lead, now); err != nil {
		return recheck, err
	}
	lead.Audit.Attempt, lead.Audit.LastAuditedAt, lead.Audit.Status = attempt, now, models.AuditRecheckOpen
	if _, err := refreshWorkflow(ctx, lead); err != nil {
		return recheck, err
	}
	RecordEvent(ctx, member.Program, lead.ID, models.EventRecheckRaised, core.ActorOf(member), now, map[string]any{
		"recheckId": recheck.ID, "recheckNo": recheck.RecheckNo, "category": recheck.Category, "comments": recheck.Comments,
		"reasons": reasons, "attempt": attempt,
	})
	Notify(ctx, member.Program, []string{lead.BdaEmail, lead.BdmEmail}, models.Notification{
		Type: models.NotifyRecheckRaised, LeadID: lead.ID, RecheckID: recheck.ID,
		Title:   fmt.Sprintf("Recheck %s raised: %s", recheck.RecheckNo, core.ReasonLabels(reasons)),
		Message: fmt.Sprintf("%s on %s: %s", core.FirstNonEmpty(member.Name, member.Email), lead.Personal.Name, recheck.Comments),
		Created: models.Created{By: member.UserHash},
	})
	return recheck, nil
}

// CloseRecheck is POST /rechecks/:recheckId/close, by the lead's BDA or BDM or an auditor. The
// lead can then be audited again; its auditor is told.
func CloseRecheck(ctx context.Context, member models.Member, recheckID, note string) (models.Recheck, error) {
	recheck, err := store.FindRecheck(ctx, member.Program, recheckID)
	if err != nil {
		return recheck, orNotFound(err, "Recheck")
	}
	team, err := TeamOf(ctx, member)
	if err != nil {
		return recheck, err
	}
	if !core.CanCloseRecheck(member, team, recheck) {
		return recheck, forbidden("Only the lead's BDA, BDM or an auditor can close this recheck")
	}
	if recheck.Status != models.RecheckOpen {
		return recheck, badRequest(core.ErrRecheckNotOpen)
	}
	if strings.TrimSpace(note) == "" {
		return recheck, badRequest(errors.New("Say what was fixed"))
	}
	now := nowSeconds()
	recheck.Status = models.RecheckClosed
	recheck.Closed = &models.RecheckClosure{At: now, By: core.ActorOf(member), Note: strings.TrimSpace(note)}
	if err := store.ReplaceRecheck(ctx, recheck); err != nil {
		return recheck, err
	}
	lead, err := store.FindLead(ctx, member.Program, recheck.LeadID)
	if err != nil {
		return recheck, orNotFound(err, "Lead")
	}
	if lead, err = refreshWorkflow(ctx, lead); err != nil {
		return recheck, err
	}
	RecordEvent(ctx, member.Program, lead.ID, models.EventRecheckClosed, core.ActorOf(member), now, map[string]any{
		"recheckId": recheck.ID, "recheckNo": recheck.RecheckNo, "category": recheck.Category, "note": recheck.Closed.Note,
	})
	auditor := core.FirstNonEmpty(core.AuditorOf(lead), recheck.AuditorEmail, recheck.RaisedBy.Email)
	Notify(ctx, member.Program, []string{auditor}, models.Notification{
		Type: models.NotifyRecheckClosed, LeadID: lead.ID, RecheckID: recheck.ID,
		Title:   fmt.Sprintf("Recheck %s closed, audit again", recheck.RecheckNo),
		Message: fmt.Sprintf("%s closed it on %s: %s", core.FirstNonEmpty(member.Name, member.Email), lead.Personal.Name, recheck.Closed.Note),
		Created: models.Created{By: member.UserHash},
	})
	if strings.Contains(auditor, "@") {
		if _, err := Mail(ctx, member.Program, member.UserHash, lead.ID, models.AlertRecheckClosed, models.TriggerAuto,
			[]string{auditor}, core.RecheckClosedSubject(recheck), core.RecheckClosedBody(recheck)); err != nil {
			return recheck, err
		}
	}
	return recheck, nil
}

// flagCcRechecks marks the lead's open CC rechecks raised before its latest CC update, and asks
// the BDA and BDM to close them.
func flagCcRechecks(ctx context.Context, lead models.Lead, now int64) error {
	rechecks, err := store.FindRechecks(ctx, lead.Program, models.RecheckQuery{LeadIDs: []string{lead.ID}, Status: models.RecheckOpen})
	if err != nil {
		return err
	}
	for _, recheck := range rechecks {
		if !core.CcUpdatedSince(recheck, lead) {
			continue
		}
		recheck.CcUpdatedAt = lead.Cc.UpdatedAt
		recheck.CcCloseAlert = nil
		if err := alertCcTicketOpen(ctx, &recheck, now); err != nil {
			return err
		}
	}
	return nil
}

// alertCcTicketOpen tells the BDA and BDM, in the portal and by mail, that the CC is updated but
// the ticket is still open, with how long ago the CC was updated; then saves the recheck.
func alertCcTicketOpen(ctx context.Context, recheck *models.Recheck, now int64) error {
	elapsed := core.FormatElapsed(now - recheck.CcUpdatedAt)
	Notify(ctx, recheck.Program, []string{recheck.BdaEmail, recheck.BdmEmail}, models.Notification{
		Type: models.NotifyCcTicketOpen, LeadID: recheck.LeadID, RecheckID: recheck.ID,
		Title:   fmt.Sprintf("Close recheck %s: CC updated %s ago", recheck.RecheckNo, elapsed),
		Message: fmt.Sprintf("The CC for %s is updated but the ticket is still open. Close it so the lead can be audited again.", recheck.LeadName),
		Created: models.Created{By: models.SystemUser},
	})
	mail, err := Mail(ctx, recheck.Program, models.SystemUser, recheck.LeadID, models.AlertCcTicketOpen, models.TriggerAuto,
		[]string{recheck.BdaEmail, recheck.BdmEmail}, core.CcCloseAlertSubject(*recheck, now), core.CcCloseAlertBody(*recheck, now))
	if err != nil {
		return err
	}
	recheck.CcCloseAlert = &mail
	return store.ReplaceRecheck(ctx, *recheck)
}
