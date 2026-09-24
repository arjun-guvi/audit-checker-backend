package worker

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	"auditApp/config"
	"auditApp/models"
)

// Sales Audit operations shared by the HTTP handlers and the jobs.

// Now is the clock used for every Sales Audit timestamp; tests replace it.
var Now = time.Now

// SystemUser is recorded as created.by for mails sent by scheduled jobs.
const SystemUser = "system"

// FindLeads returns every lead in the Audit stage with its credits and auditor state.
func FindLeads(ctx context.Context, program string) ([]models.Lead, error) {
	zohoLeads, err := FindAuditLeads(ctx)
	if err != nil {
		return nil, err
	}
	return buildLeads(ctx, program, zohoLeads)
}

// FindLead returns one lead (any stage) and its Zoho record; mongo.ErrNoDocuments if missing.
func FindLead(ctx context.Context, program, leadID string) (models.Lead, models.ZohoLead, error) {
	zohoLead, err := FindZohoLead(ctx, leadID)
	if err != nil {
		return models.Lead{}, zohoLead, err
	}
	leads, err := buildLeads(ctx, program, []models.ZohoLead{zohoLead})
	if err != nil {
		return models.Lead{}, zohoLead, err
	}
	return leads[0], zohoLead, nil
}

func buildLeads(ctx context.Context, program string, zohoLeads []models.ZohoLead) ([]models.Lead, error) {
	zenIDs := make([]string, 0, len(zohoLeads))
	leadIDs := make([]string, 0, len(zohoLeads))
	for _, lead := range zohoLeads {
		if lead.ZenID != "" {
			zenIDs = append(zenIDs, lead.ZenID)
		}
		leadIDs = append(leadIDs, lead.ID)
	}

	payments, err := FindPayments(ctx, zenIDs)
	if err != nil {
		return nil, err
	}
	paymentsByZenID := map[string][]models.ZohoPayment{}
	for _, payment := range payments {
		paymentsByZenID[payment.LeadZenID()] = append(paymentsByZenID[payment.LeadZenID()], payment)
	}

	emis, err := FindEmis(ctx, zenIDs)
	if err != nil {
		return nil, err
	}
	emiByZenID := latestEmiByZenID(emis)

	escalations, err := FindAlerts(ctx, program, leadIDs, models.AlertEscalation, 0)
	if err != nil {
		return nil, err
	}
	lastEscalation := map[string]models.Mail{}
	for _, alert := range escalations { // sorted by sentAt, so the last one wins
		lastEscalation[alert.LeadID] = alert.Mail
	}

	ccResponses, err := FindCcResponses(ctx, program, leadIDs)
	if err != nil {
		return nil, err
	}
	audits, err := FindAudits(ctx, program, leadIDs)
	if err != nil {
		return nil, err
	}

	leads := make([]models.Lead, 0, len(zohoLeads))
	for _, zohoLead := range zohoLeads {
		state := LeadState{}
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
		leads = append(leads, BuildLead(zohoLead, leadPayments, emi, state))
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
		currentAt, _ := ParseZohoTime(current.AddedTime)
		emiAt, _ := ParseZohoTime(emi.AddedTime)
		if emiAt >= currentAt {
			byZenID[emi.ZenID] = emi
		}
	}
	return byZenID
}

// FindLatestEmi returns the enrolment's latest EMI application, or nil.
func FindLatestEmi(ctx context.Context, zenID string) (*models.ZohoEmi, error) {
	if zenID == "" {
		return nil, nil
	}
	emis, err := FindEmis(ctx, []string{zenID})
	if err != nil {
		return nil, err
	}
	return latestEmiByZenID(emis)[zenID], nil
}

// FindLatestDiscount returns the latest discount request for the email, or nil.
func FindLatestDiscount(ctx context.Context, email string) (*models.Discount, error) {
	if email == "" {
		return nil, nil
	}
	discounts, err := FindDiscounts(ctx, email)
	if err != nil {
		return nil, err
	}
	var latest *models.ZohoDiscount
	var latestAt int64 = -1
	for index := range discounts {
		addedAt, _ := ParseZohoTime(discounts[index].AddedTime)
		if addedAt >= latestAt {
			latest, latestAt = &discounts[index], addedAt
		}
	}
	return ToDiscount(latest), nil
}

// FindSchedule reads the lead's planned installments, soonest first: partial reminders for split
// plans, subscription reminders for subscriptions.
func FindSchedule(ctx context.Context, paymentMode string, zohoLead models.ZohoLead) ([]Scheduled, error) {
	var schedule []Scheduled
	switch paymentMode {
	case "partial", "emiPartial":
		reminders, err := FindPartialReminders(ctx, zohoLead.ID)
		if err != nil {
			return nil, err
		}
		for _, reminder := range reminders {
			schedule = append(schedule, Scheduled{Amount: reminder.Amount, DueDate: firstNonEmpty(reminder.ActualDueDate, reminder.DueDate)})
		}
	case "subscription":
		if zohoLead.ZenID == "" {
			return nil, nil
		}
		reminders, err := FindSubscriptionReminders(ctx, zohoLead.ZenID)
		if err != nil {
			return nil, err
		}
		for _, reminder := range reminders {
			schedule = append(schedule, Scheduled{Amount: reminder.Amount, DueDate: firstNonEmpty(reminder.ActualDueDate, reminder.DueDate)})
		}
	}
	sort.SliceStable(schedule, func(i, j int) bool {
		a, _ := ParseZohoTime(schedule[i].DueDate)
		b, _ := ParseZohoTime(schedule[j].DueDate)
		return a < b
	})
	return schedule, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// SendAlert logs a mail and queues it for delivery. The mail is recorded even if queueing fails,
// so the UI still shows that the alert was raised.
func SendAlert(ctx context.Context, program, by, leadID, kind, trigger string, to []string, subject string) (models.Mail, error) {
	now := Now().Unix()
	mail := models.Mail{To: to, Subject: subject, Trigger: trigger, SentAt: now}
	alert := models.Alert{
		ID:       NewID(),
		Program:  program,
		LeadID:   leadID,
		Kind:     kind,
		Mail:     mail,
		Delivery: models.DeliveryQueued,
		Created:  models.Created{At: now, By: by},
	}
	if err := insertOne(ctx, models.AlertsCollection, alert); err != nil {
		return mail, err
	}
	if err := EnqueueSalesAuditMail(program, alert.ID); err != nil {
		log.Printf("salesAudit: queueing mail %s failed: %v", alert.ID, err)
		if err := SetAlertDelivery(ctx, program, alert.ID, models.DeliveryFailed); err != nil {
			log.Printf("salesAudit: marking mail %s failed: %v", alert.ID, err)
		}
	}
	return mail, nil
}

// Escalate mails the lead's BDA and Accounts that payment verification is pending.
func Escalate(ctx context.Context, program, by string, lead models.Lead, trigger string) (models.Mail, error) {
	return SendAlert(ctx, program, by, lead.ID, models.AlertEscalation, trigger,
		EscalationRecipients(lead, config.AccountsEmail), EscalationSubject(lead.StudentFullName))
}
