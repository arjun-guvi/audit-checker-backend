// Package service combines the store and core rules into the operations shared by the HTTP
// handlers and the background jobs.
package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// MailQueue hands a logged alert to the mail-sending job.
type MailQueue interface {
	EnqueueMail(program, alertID string) error
}

type Service struct {
	Store         store.Store
	Mail          MailQueue
	AccountsEmail string
	Now           func() time.Time
}

func (s *Service) NowSeconds() int64 {
	return s.Now().Unix()
}

// AuditLeads returns every lead in the Audit stage with its credits and auditor state.
func (s *Service) AuditLeads(ctx context.Context, program string) ([]models.Lead, error) {
	zohoLeads, err := s.Store.ListAuditLeads(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildLeads(ctx, program, zohoLeads)
}

// Lead returns one lead (any stage) and its Zoho record.
func (s *Service) Lead(ctx context.Context, program, leadID string) (models.Lead, models.ZohoLead, error) {
	zohoLead, err := s.Store.GetLead(ctx, leadID)
	if err != nil {
		return models.Lead{}, zohoLead, err
	}
	leads, err := s.buildLeads(ctx, program, []models.ZohoLead{zohoLead})
	if err != nil {
		return models.Lead{}, zohoLead, err
	}
	return leads[0], zohoLead, nil
}

func (s *Service) buildLeads(ctx context.Context, program string, zohoLeads []models.ZohoLead) ([]models.Lead, error) {
	zenIDs := make([]string, 0, len(zohoLeads))
	leadIDs := make([]string, 0, len(zohoLeads))
	for _, lead := range zohoLeads {
		if lead.ZenID != "" {
			zenIDs = append(zenIDs, lead.ZenID)
		}
		leadIDs = append(leadIDs, lead.ID)
	}

	payments, err := s.Store.PaymentsForLeads(ctx, zenIDs)
	if err != nil {
		return nil, err
	}
	paymentsByZenID := map[string][]models.ZohoPayment{}
	for _, payment := range payments {
		paymentsByZenID[payment.LeadZenID()] = append(paymentsByZenID[payment.LeadZenID()], payment)
	}

	emis, err := s.Store.EmiForLeads(ctx, zenIDs)
	if err != nil {
		return nil, err
	}
	emiByZenID := latestEmiByZenID(emis)

	escalations, err := s.Store.ListAlerts(ctx, program, store.AlertFilter{LeadIDs: leadIDs, Kind: models.AlertEscalation})
	if err != nil {
		return nil, err
	}
	lastEscalation := map[string]models.Mail{}
	for _, alert := range escalations { // sorted by sentAt, so the last one wins
		lastEscalation[alert.LeadID] = alert.Mail
	}

	ccResponses, err := s.Store.CcResponses(ctx, program, leadIDs)
	if err != nil {
		return nil, err
	}
	audits, err := s.Store.Audits(ctx, program, leadIDs)
	if err != nil {
		return nil, err
	}

	leads := make([]models.Lead, 0, len(zohoLeads))
	for _, zohoLead := range zohoLeads {
		state := core.LeadState{}
		if mail, ok := lastEscalation[zohoLead.ID]; ok {
			state.Escalation = &mail
		}
		if response, ok := ccResponses[zohoLead.ID]; ok {
			state.CcResponse = &response
		}
		if audit, ok := audits[zohoLead.ID]; ok {
			state.Audit = &audit
		}
		var leadPayments []models.ZohoPayment
		var emi *models.ZohoEmi
		if zohoLead.ZenID != "" {
			leadPayments = paymentsByZenID[zohoLead.ZenID]
			emi = emiByZenID[zohoLead.ZenID]
		}
		leads = append(leads, core.BuildLead(zohoLead, leadPayments, emi, state))
	}
	return leads, nil
}

// latestEmiByZenID keeps the most recently added EMI application per enrolment.
func latestEmiByZenID(emis []models.ZohoEmi) map[string]*models.ZohoEmi {
	byZenID := map[string]*models.ZohoEmi{}
	for index := range emis {
		emi := &emis[index]
		current, ok := byZenID[emi.ZenID]
		if !ok {
			byZenID[emi.ZenID] = emi
			continue
		}
		currentAt, _ := core.ParseZohoTime(current.AddedTime)
		emiAt, _ := core.ParseZohoTime(emi.AddedTime)
		if emiAt >= currentAt {
			byZenID[emi.ZenID] = emi
		}
	}
	return byZenID
}

// LatestEmi returns the lead's latest EMI application, or nil.
func (s *Service) LatestEmi(ctx context.Context, zenID string) (*models.ZohoEmi, error) {
	if zenID == "" {
		return nil, nil
	}
	emis, err := s.Store.EmiForLeads(ctx, []string{zenID})
	if err != nil {
		return nil, err
	}
	return latestEmiByZenID(emis)[zenID], nil
}

// SendAlert logs a mail and queues it for delivery. The mail is recorded even if queueing fails,
// so the UI still shows that the alert was raised.
func (s *Service) SendAlert(ctx context.Context, program, by, leadID, kind, trigger string, to []string, subject string) (models.Mail, error) {
	now := s.NowSeconds()
	mail := models.Mail{To: to, Subject: subject, Trigger: trigger, SentAt: now}
	alert := models.Alert{
		ID:       core.NewID(),
		Program:  program,
		LeadID:   leadID,
		Kind:     kind,
		Mail:     mail,
		Delivery: models.DeliveryQueued,
		Created:  models.Created{At: now, By: by},
	}
	if err := s.Store.InsertAlert(ctx, alert); err != nil {
		return mail, err
	}
	if err := s.Mail.EnqueueMail(program, alert.ID); err != nil {
		log.Printf("salesAudit: queueing mail %s failed: %v", alert.ID, err)
		if err := s.Store.SetAlertDelivery(ctx, program, alert.ID, models.DeliveryFailed); err != nil {
			log.Printf("salesAudit: marking mail %s failed: %v", alert.ID, err)
		}
	}
	return mail, nil
}

// Escalate mails the lead's BDA and Accounts that payment verification is pending.
func (s *Service) Escalate(ctx context.Context, program, by string, lead models.Lead, trigger string) (models.Mail, error) {
	return s.SendAlert(ctx, program, by, lead.ID, models.AlertEscalation, trigger,
		core.EscalationRecipients(lead, s.AccountsEmail), core.EscalationSubject(lead.StudentFullName))
}

// SystemUser is recorded as created.by for mails sent by scheduled jobs.
const SystemUser = "system"

// RunEscalationSweep mails every lead that has been over 24h in SAP with an unverified or
// mismatched payment and was not mailed in the last 24h. Returns how many mails were sent.
func (s *Service) RunEscalationSweep(ctx context.Context, program string) (int, error) {
	leads, err := s.AuditLeads(ctx, program)
	if err != nil {
		return 0, err
	}
	now := s.NowSeconds()
	sent := 0
	for _, lead := range leads {
		if !core.EscalationDue(lead, now) {
			continue
		}
		if _, err := s.Escalate(ctx, program, SystemUser, lead, models.TriggerAuto); err != nil {
			return sent, fmt.Errorf("escalating lead %s: %w", lead.ID, err)
		}
		sent++
	}
	return sent, nil
}

// RunRecheckReminderSweep re-mails the BDA and BDM about every recheck still open 24h after it
// was raised or last reminded. Returns how many reminders were sent.
func (s *Service) RunRecheckReminderSweep(ctx context.Context, program string) (int, error) {
	rechecks, err := s.Store.ListRechecks(ctx, program, store.RecheckFilter{Status: models.RecheckOpen})
	if err != nil {
		return 0, err
	}
	now := s.NowSeconds()
	sent := 0
	for _, recheck := range rechecks {
		if !core.RecheckReminderDue(recheck, now) {
			continue
		}
		lead, _, err := s.Lead(ctx, program, recheck.LeadID)
		if err != nil {
			log.Printf("salesAudit: recheck %s: lead %s: %v", recheck.ID, recheck.LeadID, err)
			continue
		}
		mail, err := s.SendAlert(ctx, program, SystemUser, lead.ID, models.AlertRecheckReminder,
			models.TriggerAuto, core.BdaAndBdmRecipients(lead), core.RecheckReminderSubject(lead.StudentFullName))
		if err != nil {
			return sent, err
		}
		if err := s.Store.SetRecheckReminder(ctx, program, recheck.ID, mail); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}
