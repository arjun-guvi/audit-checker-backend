package actions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
	"auditApp/salesAudit/zoho"
)

// ImportResult is what one Zoho import did.
type ImportResult struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	CcUpdated int `json:"ccUpdated"`
	Rechecks  int `json:"rechecks"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

// ImportLearners upserts Zoho learners into salesAuditLeads (by zenId) with their rechecks. New
// leads take Zoho's audit coordinator and status as their starting state; known leads only get
// their Zoho fields refreshed. A CC that became available alerts the lead's auditor.
func ImportLearners(ctx context.Context, program string, learners []models.ZohoLearner) (ImportResult, error) {
	result := ImportResult{}
	var errs []error
	for _, learner := range learners {
		zenID := strings.TrimSpace(string(learner.ZenID))
		if zenID == "" {
			result.Skipped++
			continue
		}
		if err := importLearner(ctx, program, learner, &result); err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("learner %s: %w", zenID, err))
		}
	}
	return result, errors.Join(errs...)
}

func importLearner(ctx context.Context, program string, learner models.ZohoLearner, result *ImportResult) error {
	now := nowSeconds()
	existing, err := store.FindLeadByZenID(ctx, program, strings.TrimSpace(string(learner.ZenID)))
	var lead models.Lead
	switch {
	case store.IsNotFound(err):
		lead = core.NewLeadFromZoho(program, learner, now)
		if err := store.InsertLead(ctx, lead); err != nil {
			return err
		}
		result.Created++
		recordNewLeadHistory(ctx, lead, now)
	case err != nil:
		return err
	default:
		var ccBecameUpdated bool
		lead, ccBecameUpdated = core.MergeZoho(existing, learner, now)
		if err := store.UpdateLeadZoho(ctx, lead); err != nil {
			return err
		}
		result.Updated++
		if ccBecameUpdated {
			result.CcUpdated++
			announceCcUpdated(ctx, lead, now)
		}
	}

	changed, err := importRechecks(ctx, lead, learner.RecheckDetails, now)
	if err != nil {
		return err
	}
	result.Rechecks += changed
	// A new CC on a known lead (first one, or a new link): open CC rechecks raised before it are
	// fixed and wait for the BDA to close them. A lead seen for the first time has no "before".
	if existing.ID != "" && lead.Cc.Status == models.CcUpdated && lead.Cc.UpdatedAt != existing.Cc.UpdatedAt {
		if err := flagCcRechecks(ctx, lead, now); err != nil {
			return err
		}
	}
	if changed > 0 || existing.ID == "" {
		_, err = refreshWorkflow(ctx, lead)
	}
	return err
}

func recordNewLeadHistory(ctx context.Context, lead models.Lead, now int64) {
	RecordEvent(ctx, lead.Program, lead.ID, models.EventLeadImported, core.SystemActor,
		core.FirstNonZero(lead.EnrolledAt, lead.CrmCreatedAt, now), map[string]any{"stage": lead.Stage, "region": lead.Region})
	if lead.Assignment != nil {
		RecordEvent(ctx, lead.Program, lead.ID, models.EventAssigned, core.SystemActor, lead.Assignment.AssignedAt,
			map[string]any{"auditorEmail": lead.Assignment.AuditorEmail, "mode": models.AssignZoho})
	}
	if lead.Cc.Status == models.CcUpdated {
		RecordEvent(ctx, lead.Program, lead.ID, models.EventCcUpdated, core.SystemActor, now, map[string]any{"link": lead.Cc.Link})
	}
	if lead.Audit.Status == models.AuditCompleted {
		auditor := models.Actor{Email: lead.Audit.CompletedBy, Name: lead.Audit.CompletedBy, Role: models.RoleAuditor}
		RecordEvent(ctx, lead.Program, lead.ID, models.EventAuditCompleted, auditor, lead.Audit.CompletedAt,
			map[string]any{"attempt": 1, "comments": "Completed in Zoho"})
	}
}

// announceCcUpdated tells the lead's auditor the CC can now be verified.
func announceCcUpdated(ctx context.Context, lead models.Lead, now int64) {
	RecordEvent(ctx, lead.Program, lead.ID, models.EventCcUpdated, core.SystemActor, now, map[string]any{"link": lead.Cc.Link})
	auditor := core.AuditorOf(lead)
	if auditor == "" {
		return
	}
	Notify(ctx, lead.Program, []string{auditor}, models.Notification{
		Type: models.NotifyCcUpdated, LeadID: lead.ID, Title: core.CcUpdatedSubject(lead),
		Message: "The CC is updated in Zoho and can be verified now.", Created: models.Created{By: models.SystemUser},
	})
	if _, err := Mail(ctx, lead.Program, models.SystemUser, lead.ID, models.AlertCcUpdated, models.TriggerAuto,
		[]string{auditor}, core.CcUpdatedSubject(lead), core.CcUpdatedBody(lead)); err != nil {
		log.Printf("salesAudit: CC updated mail for lead %s: %v", lead.ID, err)
	}
}

// importRechecks adds Zoho rechecks the portal has not seen and closes those Zoho has closed.
// Returns how many changed.
func importRechecks(ctx context.Context, lead models.Lead, zohoRechecks []models.ZohoRecheck, now int64) (int, error) {
	changed := 0
	for _, zohoRecheck := range zohoRechecks {
		fresh, ok := core.RecheckFromZoho(lead.Program, lead, zohoRecheck, now)
		if !ok {
			continue
		}
		existing, err := store.FindRecheckByNo(ctx, lead.Program, fresh.RecheckNo)
		switch {
		case store.IsNotFound(err):
			if err := store.InsertRecheck(ctx, fresh); err != nil {
				return changed, err
			}
			changed++
			RecordEvent(ctx, lead.Program, lead.ID, models.EventRecheckRaised, fresh.RaisedBy, fresh.RaisedAt, map[string]any{
				"recheckId": fresh.ID, "recheckNo": fresh.RecheckNo, "category": fresh.Category, "comments": fresh.Comments, "source": models.SourceZoho,
			})
			if fresh.Closed != nil {
				RecordEvent(ctx, lead.Program, lead.ID, models.EventRecheckClosed, fresh.Closed.By, fresh.Closed.At, map[string]any{
					"recheckId": fresh.ID, "recheckNo": fresh.RecheckNo, "note": fresh.Closed.Note, "source": models.SourceZoho,
				})
			}
		case err != nil:
			return changed, err
		case existing.Source == models.SourceZoho && existing.Status == models.RecheckOpen && fresh.Status == models.RecheckClosed:
			existing.Status = models.RecheckClosed
			existing.Closed = &models.RecheckClosure{At: now, By: core.SystemActor, Note: "Closed in Zoho"}
			if err := store.ReplaceRecheck(ctx, existing); err != nil {
				return changed, err
			}
			changed++
			RecordEvent(ctx, lead.Program, lead.ID, models.EventRecheckClosed, core.SystemActor, now, map[string]any{
				"recheckId": existing.ID, "recheckNo": existing.RecheckNo, "note": "Closed in Zoho", "source": models.SourceZoho,
			})
		}
	}
	return changed, nil
}

// BackfillDays is how many days of Zoho learners one backfill request fetches.
const BackfillDays = 5

// BackfillChunk is what one window of a backfill did.
type BackfillChunk struct {
	From       string       `json:"from"`
	To         string       `json:"to"`
	Fetched    int          `json:"fetched"`
	Members    MemberSync   `json:"members"`
	Import     ImportResult `json:"import"`
	Assignment AssignResult `json:"assignment"`
	Error      string       `json:"error,omitempty"`
}

// BackfillResult is every window of a backfill, in order.
type BackfillResult struct {
	Chunks []BackfillChunk `json:"chunks"`
	Failed int             `json:"failed"`
}

// BackfillZoho imports the learners enrolled from..to (IST days, inclusive) BackfillDays at a
// time, oldest first: each window's BDAs and BDMs are added as members, its learners imported,
// and new leads assigned. No mails or notifications go out. A failed window is recorded and the
// rest still run.
func BackfillZoho(ctx context.Context, program string, actor models.Actor, from, to time.Time) BackfillResult {
	ctx = WithoutAlerts(ctx)
	result := BackfillResult{Chunks: []BackfillChunk{}}
	for _, window := range core.ZohoChunks(from, to, BackfillDays) {
		chunk := backfillWindow(ctx, program, actor, window[0], window[1])
		if chunk.Error != "" {
			result.Failed++
		}
		log.Printf("salesAudit: Zoho backfill %s..%s: %+v", chunk.From, chunk.To, chunk)
		result.Chunks = append(result.Chunks, chunk)
	}
	return result
}

func backfillWindow(ctx context.Context, program string, actor models.Actor, from, to string) BackfillChunk {
	chunk := BackfillChunk{From: from, To: to}
	learners, err := zoho.Fetch(ctx, from, to)
	if err != nil {
		chunk.Error = err.Error()
		return chunk
	}
	chunk.Fetched = len(learners)
	var errs []error
	chunk.Members, err = SyncSalesMembers(ctx, program, learners)
	errs = append(errs, err)
	chunk.Import, err = ImportLearners(ctx, program, learners)
	errs = append(errs, err)
	chunk.Assignment, err = AssignPending(ctx, program, actor)
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		chunk.Error = err.Error()
	}
	return chunk
}
