package actions

import (
	"context"
	"errors"
	"strings"

	"auditApp/salesAudit/cc"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// ccExtractOf returns the lead's CC extract, reading the CC when there is none yet or its link
// changed. A lead without a CC link has no extract.
func ccExtractOf(ctx context.Context, lead models.Lead) (models.CcExtract, bool, error) {
	if lead.Cc.Link == "" {
		return models.CcExtract{}, false, nil
	}
	extract, err := store.FindCcExtract(ctx, lead.Program, lead.ID)
	if err == nil && extract.Link == lead.Cc.Link {
		return extract, true, nil
	}
	if err != nil && !store.IsNotFound(err) {
		return extract, false, err
	}
	extract = cc.Extract(lead, nowSeconds())
	return extract, true, store.SaveCcExtract(ctx, extract)
}

func requireAuditRole(member models.Member) error {
	if !core.IsAuditRole(member.Role) {
		return forbidden("Only auditors can open the audit")
	}
	return nil
}

// AuditView is GET /leads/:leadId/audit: the database on the left, the CC on the right.
func AuditView(ctx context.Context, member models.Member, leadID string) (models.AuditView, error) {
	if err := requireAuditRole(member); err != nil {
		return models.AuditView{}, err
	}
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return models.AuditView{}, err
	}
	extract, _, err := ccExtractOf(ctx, lead)
	if err != nil {
		return models.AuditView{}, err
	}
	comparison, mismatches := core.Compare(lead, extract)
	rechecks, err := store.FindRechecks(ctx, member.Program, models.RecheckQuery{LeadIDs: []string{lead.ID}})
	if err != nil {
		return models.AuditView{}, err
	}
	points := extract.PointsCovered
	if points == nil {
		points = []string{}
	}
	return models.AuditView{
		Lead: lead, Cc: lead.Cc, Comparison: comparison, MismatchCount: mismatches, PointsCovered: points,
		Checklist: core.Checklist(), Rechecks: rechecks, Actions: core.ActionsFor(member, lead),
	}, nil
}

// CcVerification is GET /leads/:leadId/cc-verification: the record next to the PDF or transcript.
func CcVerification(ctx context.Context, member models.Member, leadID string) (models.CcVerification, error) {
	if err := requireAuditRole(member); err != nil {
		return models.CcVerification{}, err
	}
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return models.CcVerification{}, err
	}
	extract, found, err := ccExtractOf(ctx, lead)
	if err != nil {
		return models.CcVerification{}, err
	}
	if !found {
		return models.CcVerification{}, badRequest(errors.New("The CC for this lead is not updated yet"))
	}
	return models.CcVerification{Lead: lead, Cc: lead.Cc, Extract: extract, PreviewURL: core.DrivePreviewURL(lead.Cc.Link)}, nil
}

// CompleteAudit is POST /leads/:leadId/complete-audit: every checklist item ticked, comments
// given, no recheck open. Closed rechecks count as re-audited.
func CompleteAudit(ctx context.Context, member models.Member, leadID string, ticked []models.ChecklistItem, comments string) (models.Audit, error) {
	if err := requireAuditRole(member); err != nil {
		return models.Audit{}, err
	}
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return models.Audit{}, err
	}
	if err := core.CheckComplete(member, lead); err != nil {
		if errors.Is(err, core.ErrNotAssigned) {
			return models.Audit{}, forbidden(err.Error())
		}
		return models.Audit{}, badRequest(err)
	}
	checklist, err := core.ValidateChecklist(ticked)
	if err != nil {
		return models.Audit{}, badRequest(err)
	}
	if strings.TrimSpace(comments) == "" {
		return models.Audit{}, badRequest(core.ErrCommentsRequired)
	}
	extract, _, err := ccExtractOf(ctx, lead)
	if err != nil {
		return models.Audit{}, err
	}
	_, mismatches := core.Compare(lead, extract)

	now := nowSeconds()
	audit := models.Audit{
		ID: core.NewID(), Program: member.Program, LeadID: lead.ID, Attempt: lead.Audit.Attempt + 1,
		Auditor: core.ActorOf(member), Outcome: models.OutcomeCompleted, Checklist: checklist,
		Comments: strings.TrimSpace(comments), MismatchCount: mismatches, SubmittedAt: now,
		Created: models.Created{At: now, By: member.UserHash},
	}
	if err := store.InsertAudit(ctx, audit); err != nil {
		return audit, err
	}
	if err := markReaudited(ctx, lead, now); err != nil {
		return audit, err
	}
	lead.Audit = models.AuditState{
		Status: models.AuditCompleted, Attempt: audit.Attempt, LastAuditedAt: now, CompletedAt: now, CompletedBy: member.Email,
	}
	if _, err := refreshWorkflow(ctx, lead); err != nil {
		return audit, err
	}
	RecordEvent(ctx, member.Program, lead.ID, models.EventAuditCompleted, core.ActorOf(member), now, map[string]any{
		"attempt": audit.Attempt, "comments": audit.Comments, "mismatchCount": mismatches,
	})
	return audit, nil
}
