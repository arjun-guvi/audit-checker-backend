package actions

import (
	"context"
	"errors"
	"fmt"
	"log"

	"auditApp/config"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// Scheduled mail sweeps, and the manual escalation reminder.

// RunEscalationSweep mails the BDA, BDM and Accounts about every lead over 24h in Sales Action
// Pending with an unverified or mismatched payment, at most once a day. Returns mails sent.
func RunEscalationSweep(ctx context.Context, program string) (int, error) {
	leads, _, err := store.FindLeads(ctx, program, models.LeadQuery{})
	if err != nil {
		return 0, err
	}
	now := nowSeconds()
	sent := 0
	for _, lead := range leads {
		if !core.EscalationDue(lead, now) {
			continue
		}
		if err := escalate(ctx, program, models.SystemUser, lead, models.TriggerAuto); err != nil {
			return sent, fmt.Errorf("escalating lead %s: %w", lead.ID, err)
		}
		sent++
	}
	return sent, nil
}

func escalate(ctx context.Context, program, by string, lead models.Lead, trigger string) error {
	mail, err := Mail(ctx, program, by, lead.ID, models.AlertEscalation, trigger,
		[]string{lead.BdaEmail, lead.BdmEmail, config.AccountsEmail}, core.EscalationSubject(lead), core.EscalationBody(lead, nowSeconds()))
	if err != nil {
		return err
	}
	return store.SetLeadMailMarks(ctx, program, lead.ID, lead.PaymentVerificationMailedAt, mail.SentAt)
}

// SendReminder is POST /leads/:leadId/send-reminder: the escalation mail, sent now by an auditor.
func SendReminder(ctx context.Context, member models.Member, leadID string) (models.Lead, error) {
	if !core.Can(member.Role, core.ActionSendReminder) {
		return models.Lead{}, forbidden("Only auditors can send reminders")
	}
	lead, _, err := loadLead(ctx, member, leadID)
	if err != nil {
		return lead, err
	}
	if !core.NeedsEscalation(lead) {
		return lead, badRequest(errors.New("This lead has no unverified or mismatched payment to escalate"))
	}
	return lead, escalate(ctx, member.Program, member.UserHash, lead, models.TriggerManual)
}

// RunRecheckReminderSweep re-mails the BDA and BDM about every recheck still open 24h after it was
// raised or last reminded. Returns reminders sent.
func RunRecheckReminderSweep(ctx context.Context, program string) (int, error) {
	rechecks, err := store.FindRechecks(ctx, program, models.RecheckQuery{Status: models.RecheckOpen})
	if err != nil {
		return 0, err
	}
	now := nowSeconds()
	sent := 0
	for _, recheck := range rechecks {
		if !core.RecheckReminderDue(recheck, now) {
			continue
		}
		mail, err := Mail(ctx, program, models.SystemUser, recheck.LeadID, models.AlertRecheckReminder, models.TriggerAuto,
			[]string{recheck.BdaEmail, recheck.BdmEmail}, core.RecheckReminderSubject(recheck), core.RecheckReminderBody(recheck))
		if err != nil {
			return sent, err
		}
		recheck.LastReminder = &mail
		if err := store.ReplaceRecheck(ctx, recheck); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// RunPaymentVerificationSweep mails the BDM, once per lead, about a payment unverified for over
// 24h. Returns mails sent.
func RunPaymentVerificationSweep(ctx context.Context, program string) (int, error) {
	leads, _, err := store.FindLeads(ctx, program, models.LeadQuery{})
	if err != nil {
		return 0, err
	}
	now := nowSeconds()
	sent := 0
	for _, lead := range leads {
		if !core.PaymentVerificationDue(lead, now) {
			continue
		}
		mail, err := Mail(ctx, program, models.SystemUser, lead.ID, models.AlertPaymentVerificationPending, models.TriggerAuto,
			[]string{lead.BdmEmail}, core.PaymentVerificationSubject(lead), core.PaymentVerificationBody(lead))
		if err != nil {
			return sent, fmt.Errorf("mailing lead %s: %w", lead.ID, err)
		}
		if err := store.SetLeadMailMarks(ctx, program, lead.ID, mail.SentAt, lead.LastEscalationAt); err != nil {
			return sent, err
		}
		log.Printf("salesAudit: mailed the BDM of lead %s about an unverified payment", lead.ID)
		sent++
	}
	return sent, nil
}

// ListAlerts is the mail log of a lead (or every lead when leadID is empty) since a time.
func ListAlerts(ctx context.Context, member models.Member, leadID string, since int64) ([]models.Alert, error) {
	if leadID != "" {
		if _, _, err := loadLead(ctx, member, leadID); err != nil {
			return nil, err
		}
		return store.FindAlerts(ctx, member.Program, []string{leadID}, "", since)
	}
	if !core.IsAuditRole(member.Role) {
		return nil, forbidden("Only auditors can see the mail log")
	}
	return store.FindAlerts(ctx, member.Program, nil, "", since)
}
