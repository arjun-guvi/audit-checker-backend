package controller

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"auditApp/models"
	"auditApp/worker"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

// Sales Audit handlers (/sales-audit/...). Responses are {status, data} or {status, message}.

// sweepToastWindow is how far back GET /leads counts automatic escalation mails for the
// "Auto-escalation: N leads mailed" toast (the sweep job runs hourly).
const sweepToastWindow = time.Hour

// historyDefaultDays is how much history GET /audit-history returns without ?from: the Overview
// compares the last 30 days with the 30 before.
const historyDefaultDays = 60

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

func respondError(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"status": "error", "message": message})
}

func respondServerError(c *gin.Context, err error) {
	log.Printf("salesAudit: %s %s: %v", c.Request.Method, c.FullPath(), err)
	respondError(c, http.StatusInternalServerError, "Something went wrong")
}

func program(c *gin.Context) string {
	return c.MustGet("program").(string)
}

func authUser(c *gin.Context) string {
	return c.MustGet("auth").(string)
}

// loadLead fetches the lead in the URL, writing a 404/500 and returning false when it can't.
func loadLead(c *gin.Context, param string) (models.Lead, models.ZohoLead, bool) {
	lead, zohoLead, err := worker.FindLead(c, program(c), c.Param(param))
	if worker.IsNotFound(err) {
		respondError(c, http.StatusNotFound, "Lead not found")
		return lead, zohoLead, false
	}
	if err != nil {
		respondServerError(c, err)
		return lead, zohoLead, false
	}
	return lead, zohoLead, true
}

// GetLeads: GET /leads
func GetLeads(c *gin.Context) {
	leads, err := worker.FindLeads(c, program(c))
	if err != nil {
		respondServerError(c, err)
		return
	}
	recent, err := worker.FindAlerts(c, program(c), nil, models.AlertEscalation, worker.Now().Add(-sweepToastWindow).Unix())
	if err != nil {
		respondServerError(c, err)
		return
	}
	sweepMails := 0
	for _, alert := range recent {
		if alert.Trigger == models.TriggerAuto {
			sweepMails++
		}
	}
	respondOK(c, models.LeadsResponse{Leads: leads, MailsSentThisSweep: sweepMails})
}

// GetLeadSummaries: GET /leads/summaries
func GetLeadSummaries(c *gin.Context) {
	leads, err := worker.FindLeads(c, program(c))
	if err != nil {
		respondServerError(c, err)
		return
	}
	summaries := make([]models.LeadSummary, 0, len(leads))
	for _, lead := range leads {
		summaries = append(summaries, worker.SummarizeLead(lead))
	}
	respondOK(c, summaries)
}

// SendReminder: POST /leads/:leadId/send-reminder
func SendReminder(c *gin.Context) {
	lead, _, found := loadLead(c, "leadId")
	if !found {
		return
	}
	if !worker.NeedsEscalation(lead) {
		respondError(c, http.StatusBadRequest, "This lead has no unverified or mismatched payment to escalate")
		return
	}
	mail, err := worker.Escalate(c, program(c), authUser(c), lead, models.TriggerManual)
	if err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, mail)
}

// UpdateCcResponse: POST /leads/:leadId/cc-response
func UpdateCcResponse(c *gin.Context) {
	var body struct {
		Response string `json:"response"`
	}
	if err := c.ShouldBindJSON(&body); err != nil ||
		(body.Response != models.CcMailSentAwaitingAck && body.Response != models.CcMailNotSent) {
		respondError(c, http.StatusBadRequest, `response must be "mailSentAwaitingAck" or "mailNotSent"`)
		return
	}
	lead, _, found := loadLead(c, "leadId")
	if !found {
		return
	}
	if lead.ConfirmationCallLink != "" {
		respondError(c, http.StatusBadRequest, "The CC for this lead is already uploaded")
		return
	}

	now := worker.Now().Unix()
	response := models.CcResponse{
		ID:        worker.NewID(),
		Program:   program(c),
		LeadID:    lead.ID,
		Response:  body.Response,
		UpdatedAt: now,
		Created:   models.Created{At: now, By: authUser(c)},
	}
	if body.Response == models.CcMailNotSent {
		mail, err := worker.SendAlert(c, program(c), authUser(c), lead.ID, models.AlertCcNotSent,
			models.TriggerAuto, worker.BdaRecipients(lead), worker.CcNotSentSubject(lead.StudentFullName))
		if err != nil {
			respondServerError(c, err)
			return
		}
		response.Alert = &mail
	}
	if err := worker.SaveCcResponse(c, response); err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, response)
}

// GetLeadAudit: GET /leads/:leadId/audit
func GetLeadAudit(c *gin.Context) {
	lead, zohoLead, found := loadLead(c, "leadId")
	if !found {
		return
	}
	paymentMode := worker.PaymentMode(lead.PaymentType)

	schedule, err := worker.FindSchedule(c, paymentMode, zohoLead)
	if err != nil {
		respondServerError(c, err)
		return
	}
	zohoSource := worker.ZohoSource(lead, zohoLead, schedule)
	result := models.LeadAudit{
		Lead:                   lead,
		Sources:                models.AuditSources{Zoho: &zohoSource},
		PaymentMode:            paymentMode,
		PartialSplitUpCategory: lead.PartialSplitUpCategory,
		PointsCovered:          []string{},
	}

	if worker.IsEmiPaymentType(lead.PaymentType) {
		emi, err := worker.FindLatestEmi(c, zohoLead.ZenID)
		if err != nil {
			respondServerError(c, err)
			return
		}
		result.Sources.Vendor = worker.VendorSource(emi)
		if emi != nil {
			result.VendorName = emi.EMIVendor
		}
	}

	if lead.ConfirmationCallLink != "" {
		extract, err := worker.FindCcExtract(c, program(c), lead.ID)
		switch {
		case err == nil:
			result.Sources.Cc = &extract.Scraped
			result.PointsCovered = extract.PointsCovered
		case !worker.IsNotFound(err):
			respondServerError(c, err)
			return
		}
	}

	if result.Rechecks, err = worker.FindRechecks(c, program(c), lead.ID, ""); err != nil {
		respondServerError(c, err)
		return
	}
	if result.Discount, err = worker.FindLatestDiscount(c, lead.Email); err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, result)
}

// MarkAudited: POST /leads/:leadId/mark-audited
func MarkAudited(c *gin.Context) {
	var body struct {
		OverrideReason string `json:"overrideReason"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "Invalid request body")
			return
		}
	}
	lead, _, found := loadLead(c, "leadId")
	if !found {
		return
	}
	if lead.Audit != nil {
		respondError(c, http.StatusBadRequest, "This lead is already verified")
		return
	}
	now := worker.Now().Unix()
	audit := models.Audit{
		ID:             worker.NewID(),
		Program:        program(c),
		LeadID:         lead.ID,
		AuditedAt:      now,
		AuditedBy:      models.AuditTeamName,
		OverrideReason: strings.TrimSpace(body.OverrideReason),
		Created:        models.Created{At: now, By: authUser(c)},
	}
	if err := worker.InsertAudit(c, audit); mongo.IsDuplicateKeyError(err) {
		respondError(c, http.StatusBadRequest, "This lead is already verified")
		return
	} else if err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, audit)
}

// GetRechecks: GET /rechecks
func GetRechecks(c *gin.Context) {
	rechecks, err := worker.FindRechecks(c, program(c), "", "")
	if err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, rechecks)
}

// RaiseRecheck: POST /rechecks
func RaiseRecheck(c *gin.Context) {
	var body struct {
		LeadID   string `json:"leadId"`
		Category string `json:"category"`
		Notes    string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.LeadID == "" {
		respondError(c, http.StatusBadRequest, "leadId, category and notes are required")
		return
	}
	if _, known := models.RecheckCategories[body.Category]; !known {
		respondError(c, http.StatusBadRequest, "Unknown recheck category")
		return
	}
	if strings.TrimSpace(body.Notes) == "" {
		respondError(c, http.StatusBadRequest, "Notes are required")
		return
	}
	lead, _, err := worker.FindLead(c, program(c), body.LeadID)
	if worker.IsNotFound(err) {
		respondError(c, http.StatusNotFound, "Lead not found")
		return
	}
	if err != nil {
		respondServerError(c, err)
		return
	}

	mail, err := worker.SendAlert(c, program(c), authUser(c), lead.ID, models.AlertRecheck, models.TriggerAuto,
		worker.BdaAndBdmRecipients(lead), worker.RecheckSubject(body.Category, lead.StudentFullName))
	if err != nil {
		respondServerError(c, err)
		return
	}
	recheck := models.Recheck{
		ID:       worker.NewID(),
		Program:  program(c),
		LeadID:   lead.ID,
		Category: body.Category,
		Notes:    strings.TrimSpace(body.Notes),
		Status:   models.RecheckOpen,
		RaisedBy: models.AuditTeamName,
		RaisedAt: mail.SentAt,
		Alert:    mail,
		Created:  models.Created{At: mail.SentAt, By: authUser(c)},
	}
	if err := worker.InsertRecheck(c, recheck); err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, recheck)
}

// ResolveRecheck: POST /rechecks/:recheckId/resolve
func ResolveRecheck(c *gin.Context) {
	recheck, err := worker.FindRecheck(c, program(c), c.Param("recheckId"))
	if worker.IsNotFound(err) {
		respondError(c, http.StatusNotFound, "Recheck not found")
		return
	}
	if err != nil {
		respondServerError(c, err)
		return
	}
	if recheck.Status == models.RecheckResolved {
		respondError(c, http.StatusBadRequest, "This recheck is already resolved")
		return
	}
	now := worker.Now().Unix()
	if err := worker.ResolveRecheck(c, program(c), recheck.ID, now); err != nil {
		respondServerError(c, err)
		return
	}
	recheck.Status = models.RecheckResolved
	recheck.ResolvedAt = &now
	respondOK(c, recheck)
}

// GetAuditHistory: GET /audit-history?from=<unix seconds>
func GetAuditHistory(c *gin.Context) {
	from := worker.Now().AddDate(0, 0, -historyDefaultDays).Unix()
	if raw := c.Query("from"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			respondError(c, http.StatusBadRequest, "from must be Unix seconds")
			return
		}
		from = parsed
	}

	// Every recheck is returned: ones raised before `from` can still be open inside the window.
	rechecks, err := worker.FindRechecks(c, program(c), "", "")
	if err != nil {
		respondServerError(c, err)
		return
	}
	alerts, err := worker.FindAlerts(c, program(c), nil, "", from)
	if err != nil {
		respondServerError(c, err)
		return
	}
	payments, err := historyPayments(c, from)
	if err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, models.AuditHistory{Rechecks: rechecks, Payments: payments, Alerts: alerts})
}

// historyPayments returns the Audit-stage leads' payments added or verified since `from`.
func historyPayments(c *gin.Context, from int64) ([]models.Payment, error) {
	zohoLeads, err := worker.FindAuditLeads(c)
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
	zohoPayments, err := worker.FindPayments(c, zenIDs)
	if err != nil {
		return nil, err
	}
	payments := []models.Payment{}
	for _, zohoPayment := range zohoPayments {
		payment := worker.ToPayment(zohoPayment, leadIDByZenID[zohoPayment.LeadZenID()])
		if (payment.AddedAt != nil && *payment.AddedAt >= from) ||
			(payment.VerifiedAt != nil && *payment.VerifiedAt >= from) {
			payments = append(payments, payment)
		}
	}
	return payments, nil
}

// GetStudent: GET /students/:studentId
func GetStudent(c *gin.Context) {
	lead, _, found := loadLead(c, "studentId")
	if !found {
		return
	}
	respondOK(c, lead)
}

// GetStudentPayments: GET /students/:studentId/payments
func GetStudentPayments(c *gin.Context) {
	lead, zohoLead, found := loadLead(c, "studentId")
	if !found {
		return
	}
	payments := []models.Payment{}
	if zohoLead.ZenID != "" {
		zohoPayments, err := worker.FindPayments(c, []string{zohoLead.ZenID})
		if err != nil {
			respondServerError(c, err)
			return
		}
		worker.SortPayments(zohoPayments)
		for _, payment := range zohoPayments {
			payments = append(payments, worker.ToPayment(payment, lead.ID))
		}
	}
	respondOK(c, payments)
}

// GetCcVerification: GET /students/:studentId/cc-verification
func GetCcVerification(c *gin.Context) {
	lead, zohoLead, found := loadLead(c, "studentId")
	if !found {
		return
	}
	extract, err := worker.FindCcExtract(c, program(c), lead.ID)
	if worker.IsNotFound(err) {
		respondError(c, http.StatusNotFound, "No CC data has been extracted for this lead yet")
		return
	}
	if err != nil {
		respondServerError(c, err)
		return
	}
	paymentMode := worker.PaymentMode(lead.PaymentType)
	schedule, err := worker.FindSchedule(c, paymentMode, zohoLead)
	if err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, models.CcVerification{
		PaymentMode:            paymentMode,
		PartialSplitUpCategory: lead.PartialSplitUpCategory,
		System:                 worker.ZohoSource(lead, zohoLead, schedule),
		Scraped:                extract.Scraped,
		PointsCovered:          extract.PointsCovered,
	})
}

// RunPaymentVerificationSweep: POST /payment-verification/run-sweep. Runs the sweep the worker
// runs every 10 minutes, for testing the mail flow: the due learners are mailed and marked.
func RunPaymentVerificationSweep(c *gin.Context) {
	sent, err := worker.RunPaymentVerificationSweep(c, program(c))
	if err != nil {
		respondServerError(c, err)
		return
	}
	respondOK(c, gin.H{"sent": sent})
}
