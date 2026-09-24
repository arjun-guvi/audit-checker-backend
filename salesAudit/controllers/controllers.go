package controllers

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/service"
	"auditApp/salesAudit/store"

	"github.com/gin-gonic/gin"
)

// SweepToastWindow is how far back GET /leads counts automatic escalation mails for the
// "Auto-escalation: N leads mailed" toast (the sweep job runs hourly).
const SweepToastWindow = time.Hour

// HistoryDefaultDays is how much history GET /audit-history returns without ?from: the Overview
// compares the last 30 days with the 30 before.
const HistoryDefaultDays = 60

type Handlers struct {
	svc *service.Service
}

func New(svc *service.Service) *Handlers {
	return &Handlers{svc: svc}
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

func fail(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"status": "error", "message": message})
}

func serverError(c *gin.Context, err error) {
	log.Printf("salesAudit: %s %s: %v", c.Request.Method, c.FullPath(), err)
	fail(c, http.StatusInternalServerError, "Something went wrong")
}

func program(c *gin.Context) string {
	return c.MustGet("program").(string)
}

func user(c *gin.Context) string {
	return c.MustGet("auth").(string)
}

// loadLead fetches the lead in the URL, writing a 404/500 and returning false when it can't.
func (h *Handlers) loadLead(c *gin.Context, param string) (models.Lead, models.ZohoLead, bool) {
	lead, zohoLead, err := h.svc.Lead(c, program(c), c.Param(param))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "Lead not found")
		return lead, zohoLead, false
	}
	if err != nil {
		serverError(c, err)
		return lead, zohoLead, false
	}
	return lead, zohoLead, true
}

// GetLeads: GET /leads
func (h *Handlers) GetLeads(c *gin.Context) {
	leads, err := h.svc.AuditLeads(c, program(c))
	if err != nil {
		serverError(c, err)
		return
	}
	recent, err := h.svc.Store.ListAlerts(c, program(c), store.AlertFilter{
		Kind:  models.AlertEscalation,
		Since: h.svc.Now().Add(-SweepToastWindow).Unix(),
	})
	if err != nil {
		serverError(c, err)
		return
	}
	sweepMails := 0
	for _, alert := range recent {
		if alert.Trigger == models.TriggerAuto {
			sweepMails++
		}
	}
	ok(c, models.LeadsResponse{Leads: leads, MailsSentThisSweep: sweepMails})
}

// GetLeadSummaries: GET /leads/summaries
func (h *Handlers) GetLeadSummaries(c *gin.Context) {
	leads, err := h.svc.AuditLeads(c, program(c))
	if err != nil {
		serverError(c, err)
		return
	}
	summaries := make([]models.LeadSummary, 0, len(leads))
	for _, lead := range leads {
		summaries = append(summaries, core.SummarizeLead(lead))
	}
	ok(c, summaries)
}

// SendReminder: POST /leads/:leadId/send-reminder
func (h *Handlers) SendReminder(c *gin.Context) {
	lead, _, found := h.loadLead(c, "leadId")
	if !found {
		return
	}
	if !core.NeedsEscalation(lead) {
		fail(c, http.StatusBadRequest, "This lead has no unverified or mismatched payment to escalate")
		return
	}
	mail, err := h.svc.Escalate(c, program(c), user(c), lead, models.TriggerManual)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, mail)
}

// UpdateCcResponse: POST /leads/:leadId/cc-response
func (h *Handlers) UpdateCcResponse(c *gin.Context) {
	var body struct {
		Response string `json:"response"`
	}
	if err := c.ShouldBindJSON(&body); err != nil ||
		(body.Response != models.CcMailSentAwaitingAck && body.Response != models.CcMailNotSent) {
		fail(c, http.StatusBadRequest, `response must be "mailSentAwaitingAck" or "mailNotSent"`)
		return
	}
	lead, _, found := h.loadLead(c, "leadId")
	if !found {
		return
	}
	if lead.ConfirmationCallLink != "" {
		fail(c, http.StatusBadRequest, "The CC for this lead is already uploaded")
		return
	}

	now := h.svc.NowSeconds()
	response := models.CcResponse{
		ID:        core.NewID(),
		Program:   program(c),
		LeadID:    lead.ID,
		Response:  body.Response,
		UpdatedAt: now,
		Created:   models.Created{At: now, By: user(c)},
	}
	if body.Response == models.CcMailNotSent {
		mail, err := h.svc.SendAlert(c, program(c), user(c), lead.ID, models.AlertCcNotSent,
			models.TriggerAuto, core.BdaRecipients(lead), core.CcNotSentSubject(lead.StudentFullName))
		if err != nil {
			serverError(c, err)
			return
		}
		response.Alert = &mail
	}
	if err := h.svc.Store.UpsertCcResponse(c, response); err != nil {
		serverError(c, err)
		return
	}
	ok(c, response)
}

// GetLeadAudit: GET /leads/:leadId/audit
func (h *Handlers) GetLeadAudit(c *gin.Context) {
	lead, zohoLead, found := h.loadLead(c, "leadId")
	if !found {
		return
	}
	paymentMode := core.PaymentMode(lead.PaymentType)

	schedule, err := h.schedule(c, paymentMode, zohoLead)
	if err != nil {
		serverError(c, err)
		return
	}
	emi, err := h.svc.LatestEmi(c, zohoLead.ZenID)
	if err != nil {
		serverError(c, err)
		return
	}
	zohoSource := core.ZohoSource(lead, zohoLead, schedule)
	result := models.LeadAudit{
		Lead:                   lead,
		Sources:                models.AuditSources{Zoho: &zohoSource},
		PaymentMode:            paymentMode,
		PartialSplitUpCategory: lead.PartialSplitUpCategory,
		PointsCovered:          []string{},
	}
	if core.IsEmiPaymentType(lead.PaymentType) {
		result.Sources.Vendor = core.VendorSource(emi)
		if emi != nil {
			result.VendorName = emi.EMIVendor
		}
	}

	if lead.ConfirmationCallLink != "" {
		extract, err := h.svc.Store.GetCcExtract(c, program(c), lead.ID)
		switch {
		case err == nil:
			result.Sources.Cc = &extract.Scraped
			result.PointsCovered = extract.PointsCovered
		case !errors.Is(err, store.ErrNotFound):
			serverError(c, err)
			return
		}
	}

	if result.Rechecks, err = h.svc.Store.ListRechecks(c, program(c), store.RecheckFilter{LeadID: lead.ID}); err != nil {
		serverError(c, err)
		return
	}

	if lead.Email != "" {
		discounts, err := h.svc.Store.DiscountsForEmail(c, lead.Email)
		if err != nil {
			serverError(c, err)
			return
		}
		result.Discount = core.ToDiscount(latestDiscount(discounts))
	}
	ok(c, result)
}

// schedule reads the lead's planned installments: partial reminders for split plans,
// subscription reminders for subscriptions.
func (h *Handlers) schedule(c *gin.Context, paymentMode string, zohoLead models.ZohoLead) ([]core.Scheduled, error) {
	var schedule []core.Scheduled
	switch paymentMode {
	case "partial", "emiPartial":
		reminders, err := h.svc.Store.PartialReminders(c, zohoLead.ID)
		if err != nil {
			return nil, err
		}
		for _, reminder := range reminders {
			schedule = append(schedule, core.Scheduled{Amount: reminder.Amount, DueDate: firstNonEmpty(reminder.ActualDueDate, reminder.DueDate)})
		}
	case "subscription":
		if zohoLead.ZenID == "" {
			return nil, nil
		}
		reminders, err := h.svc.Store.SubscriptionReminders(c, zohoLead.ZenID)
		if err != nil {
			return nil, err
		}
		for _, reminder := range reminders {
			schedule = append(schedule, core.Scheduled{Amount: reminder.Amount, DueDate: firstNonEmpty(reminder.ActualDueDate, reminder.DueDate)})
		}
	}
	sort.SliceStable(schedule, func(i, j int) bool {
		a, _ := core.ParseZohoTime(schedule[i].DueDate)
		b, _ := core.ParseZohoTime(schedule[j].DueDate)
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

func latestDiscount(discounts []models.ZohoDiscount) *models.ZohoDiscount {
	var latest *models.ZohoDiscount
	var latestAt int64 = -1
	for index := range discounts {
		addedAt, _ := core.ParseZohoTime(discounts[index].AddedTime)
		if addedAt >= latestAt {
			latest, latestAt = &discounts[index], addedAt
		}
	}
	return latest
}

// MarkAudited: POST /leads/:leadId/mark-audited
func (h *Handlers) MarkAudited(c *gin.Context) {
	var body struct {
		OverrideReason string `json:"overrideReason"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, http.StatusBadRequest, "Invalid request body")
			return
		}
	}
	lead, _, found := h.loadLead(c, "leadId")
	if !found {
		return
	}
	if lead.Audit != nil {
		fail(c, http.StatusBadRequest, "This lead is already verified")
		return
	}
	now := h.svc.NowSeconds()
	audit := models.Audit{
		ID:             core.NewID(),
		Program:        program(c),
		LeadID:         lead.ID,
		AuditedAt:      now,
		AuditedBy:      models.AuditTeamName,
		OverrideReason: strings.TrimSpace(body.OverrideReason),
		Created:        models.Created{At: now, By: user(c)},
	}
	if err := h.svc.Store.InsertAudit(c, audit); errors.Is(err, store.ErrDuplicate) {
		fail(c, http.StatusBadRequest, "This lead is already verified")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	ok(c, audit)
}

// GetRechecks: GET /rechecks
func (h *Handlers) GetRechecks(c *gin.Context) {
	rechecks, err := h.svc.Store.ListRechecks(c, program(c), store.RecheckFilter{})
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, rechecks)
}

// RaiseRecheck: POST /rechecks
func (h *Handlers) RaiseRecheck(c *gin.Context) {
	var body struct {
		LeadID   string `json:"leadId"`
		Category string `json:"category"`
		Notes    string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.LeadID == "" {
		fail(c, http.StatusBadRequest, "leadId, category and notes are required")
		return
	}
	if _, known := models.RecheckCategories[body.Category]; !known {
		fail(c, http.StatusBadRequest, "Unknown recheck category")
		return
	}
	if strings.TrimSpace(body.Notes) == "" {
		fail(c, http.StatusBadRequest, "Notes are required")
		return
	}
	lead, _, err := h.svc.Lead(c, program(c), body.LeadID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "Lead not found")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	mail, err := h.svc.SendAlert(c, program(c), user(c), lead.ID, models.AlertRecheck, models.TriggerAuto,
		core.BdaAndBdmRecipients(lead), core.RecheckSubject(body.Category, lead.StudentFullName))
	if err != nil {
		serverError(c, err)
		return
	}
	recheck := models.Recheck{
		ID:       core.NewID(),
		Program:  program(c),
		LeadID:   lead.ID,
		Category: body.Category,
		Notes:    strings.TrimSpace(body.Notes),
		Status:   models.RecheckOpen,
		RaisedBy: models.AuditTeamName,
		RaisedAt: mail.SentAt,
		Alert:    mail,
		Created:  models.Created{At: mail.SentAt, By: user(c)},
	}
	if err := h.svc.Store.InsertRecheck(c, recheck); err != nil {
		serverError(c, err)
		return
	}
	ok(c, recheck)
}

// ResolveRecheck: POST /rechecks/:recheckId/resolve
func (h *Handlers) ResolveRecheck(c *gin.Context) {
	recheck, err := h.svc.Store.GetRecheck(c, program(c), c.Param("recheckId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "Recheck not found")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if recheck.Status == models.RecheckResolved {
		fail(c, http.StatusBadRequest, "This recheck is already resolved")
		return
	}
	now := h.svc.NowSeconds()
	if err := h.svc.Store.ResolveRecheck(c, program(c), recheck.ID, now); err != nil {
		serverError(c, err)
		return
	}
	recheck.Status = models.RecheckResolved
	recheck.ResolvedAt = &now
	ok(c, recheck)
}

// GetAuditHistory: GET /audit-history?from=<unix seconds>
func (h *Handlers) GetAuditHistory(c *gin.Context) {
	from := h.svc.Now().AddDate(0, 0, -HistoryDefaultDays).Unix()
	if raw := c.Query("from"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			fail(c, http.StatusBadRequest, "from must be Unix seconds")
			return
		}
		from = parsed
	}

	// Every recheck is returned: ones raised before `from` can still be open inside the window.
	rechecks, err := h.svc.Store.ListRechecks(c, program(c), store.RecheckFilter{})
	if err != nil {
		serverError(c, err)
		return
	}
	alerts, err := h.svc.Store.ListAlerts(c, program(c), store.AlertFilter{Since: from})
	if err != nil {
		serverError(c, err)
		return
	}
	payments, err := h.historyPayments(c, from)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, models.AuditHistory{Rechecks: rechecks, Payments: payments, Alerts: alerts})
}

// historyPayments returns the Audit-stage leads' payments added or verified since `from`.
func (h *Handlers) historyPayments(c *gin.Context, from int64) ([]models.Payment, error) {
	zohoLeads, err := h.svc.Store.ListAuditLeads(c)
	if err != nil {
		return nil, err
	}
	leadIDByZenID := map[string]string{}
	zenIDs := []string{}
	for _, lead := range zohoLeads {
		if lead.ZenID != "" {
			leadIDByZenID[lead.ZenID] = lead.ID
			zenIDs = append(zenIDs, lead.ZenID)
		}
	}
	zohoPayments, err := h.svc.Store.PaymentsForLeads(c, zenIDs)
	if err != nil {
		return nil, err
	}
	payments := []models.Payment{}
	for _, zohoPayment := range zohoPayments {
		payment := core.ToPayment(zohoPayment, leadIDByZenID[zohoPayment.LeadZenID()])
		if (payment.AddedAt != nil && *payment.AddedAt >= from) ||
			(payment.VerifiedAt != nil && *payment.VerifiedAt >= from) {
			payments = append(payments, payment)
		}
	}
	return payments, nil
}

// GetStudent: GET /students/:studentId
func (h *Handlers) GetStudent(c *gin.Context) {
	lead, _, found := h.loadLead(c, "studentId")
	if !found {
		return
	}
	ok(c, lead)
}

// GetStudentPayments: GET /students/:studentId/payments
func (h *Handlers) GetStudentPayments(c *gin.Context) {
	lead, zohoLead, found := h.loadLead(c, "studentId")
	if !found {
		return
	}
	payments := []models.Payment{}
	if zohoLead.ZenID != "" {
		zohoPayments, err := h.svc.Store.PaymentsForLeads(c, []string{zohoLead.ZenID})
		if err != nil {
			serverError(c, err)
			return
		}
		core.SortPayments(zohoPayments)
		for _, payment := range zohoPayments {
			payments = append(payments, core.ToPayment(payment, lead.ID))
		}
	}
	ok(c, payments)
}

// GetCcVerification: GET /students/:studentId/cc-verification
func (h *Handlers) GetCcVerification(c *gin.Context) {
	lead, zohoLead, found := h.loadLead(c, "studentId")
	if !found {
		return
	}
	extract, err := h.svc.Store.GetCcExtract(c, program(c), lead.ID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "No CC data has been extracted for this lead yet")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	paymentMode := core.PaymentMode(lead.PaymentType)
	schedule, err := h.schedule(c, paymentMode, zohoLead)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, models.CcVerification{
		PaymentMode:            paymentMode,
		PartialSplitUpCategory: lead.PartialSplitUpCategory,
		System:                 core.ZohoSource(lead, zohoLead, schedule),
		Scraped:                extract.Scraped,
		PointsCovered:          extract.PointsCovered,
	})
}
