// Package controllers holds the Gin handlers of /sales-audit. Each reads the request, calls one
// action, and answers {status, data} or {status, message}.
package controllers

import (
	"errors"
	"io"
	"net/http"
	netmail "net/mail"
	"strconv"
	"strings"
	"time"

	"auditApp/salesAudit/actions"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/zoho"

	"github.com/gin-gonic/gin"
)

// Query helpers

func queryInt(c *gin.Context, key string) int {
	value, _ := strconv.Atoi(c.Query(key))
	return value
}

func queryList(c *gin.Context, key string) []string {
	result := []string{}
	for _, value := range strings.Split(c.Query(key), ",") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

// queryRange reads <name>In=preset or <name>From/<name>To.
func queryRange(c *gin.Context, name string) (models.Range, error) {
	span, _, err := core.ParseRange(c.Query(name+"In"), c.Query(name+"From"), c.Query(name+"To"), actions.Now())
	return span, err
}

func queryRanges(c *gin.Context, names ...string) (map[string]models.Range, bool) {
	result := map[string]models.Range{}
	for _, name := range names {
		span, err := queryRange(c, name)
		if err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return nil, false
		}
		result[name] = span
	}
	return result, true
}

// Me and members

// GetMe: GET /me
func GetMe(c *gin.Context) {
	user, err := actions.CurrentUser(c, member(c))
	reply(c, user, err)
}

// GetMembers: GET /members?role=
func GetMembers(c *gin.Context) {
	members, err := actions.ListMembers(c, program(c), c.Query("role"))
	reply(c, members, err)
}

// CreateMember: POST /members
func CreateMember(c *gin.Context) {
	var body actions.MemberInput
	if !bind(c, &body) {
		return
	}
	created, err := actions.CreateMember(c, member(c), body)
	reply(c, created, err)
}

// UpdateMember: PUT /members/:memberId
func UpdateMember(c *gin.Context) {
	var body actions.MemberInput
	if !bind(c, &body) {
		return
	}
	updated, err := actions.UpdateMember(c, member(c), c.Param("memberId"), body)
	reply(c, updated, err)
}

// Leads

// GetLeads: GET /leads. Filters: scope=mine|all, auditStatus (comma list), region, auditorEmail,
// bdaEmail, ccStatus, search, unassigned, page, pageSize; date filters completed*, recheckRaised*,
// recheckClosed* (…In=today|thisWeek|lastWeek|thisMonth|lastMonth, or …From/…To Unix seconds);
// recheckCategory, recheckStatus, awaitingReaudit.
func GetLeads(c *gin.Context) {
	ranges, ok := queryRanges(c, "completed", "recheckRaised", "recheckClosed")
	if !ok {
		return
	}
	params := actions.LeadListParams{
		Query: models.LeadQuery{
			AuditStatuses: queryList(c, "auditStatus"),
			Region:        c.Query("region"),
			AuditorEmail:  c.Query("auditorEmail"),
			BdaEmail:      c.Query("bdaEmail"),
			CcStatus:      c.Query("ccStatus"),
			Search:        c.Query("search"),
			Unassigned:    c.Query("unassigned") == "true",
			Completed:     ranges["completed"],
			Page:          queryInt(c, "page"),
			PageSize:      queryInt(c, "pageSize"),
		},
		Mine:            c.Query("scope") == "mine",
		RecheckRaised:   ranges["recheckRaised"],
		RecheckClosed:   ranges["recheckClosed"],
		RecheckCategory: c.Query("recheckCategory"),
		RecheckStatus:   c.Query("recheckStatus"),
		AwaitingReaudit: c.Query("awaitingReaudit") == "true",
	}
	page, err := actions.ListLeads(c, member(c), params)
	reply(c, page, err)
}

// AssignLeads: POST /leads/assign. Runs the region assignment now (auditor TL).
func AssignLeads(c *gin.Context) {
	who := member(c)
	if !core.Can(who.Role, core.ActionRunAssignment) {
		respondError(c, http.StatusForbidden, "Only the auditor TL can run the assignment")
		return
	}
	result, err := actions.AssignPending(c, who.Program, core.ActorOf(who))
	reply(c, result, err)
}

// GetLead: GET /leads/:leadId
func GetLead(c *gin.Context) {
	detail, err := actions.LeadDetail(c, member(c), c.Param("leadId"))
	reply(c, detail, err)
}

// GetTimeline: GET /leads/:leadId/timeline
func GetTimeline(c *gin.Context) {
	events, err := actions.Timeline(c, member(c), c.Param("leadId"))
	reply(c, events, err)
}

// GetAuditView: GET /leads/:leadId/audit
func GetAuditView(c *gin.Context) {
	view, err := actions.AuditView(c, member(c), c.Param("leadId"))
	reply(c, view, err)
}

// GetCcVerification: GET /leads/:leadId/cc-verification
func GetCcVerification(c *gin.Context) {
	verification, err := actions.CcVerification(c, member(c), c.Param("leadId"))
	reply(c, verification, err)
}

// GetLeadAlerts: GET /leads/:leadId/alerts
func GetLeadAlerts(c *gin.Context) {
	alerts, err := actions.ListAlerts(c, member(c), c.Param("leadId"), 0)
	reply(c, alerts, err)
}

// CompleteAudit: POST /leads/:leadId/complete-audit {checklist: [{key, checked}], comments}
func CompleteAudit(c *gin.Context) {
	var body struct {
		Checklist []models.ChecklistItem `json:"checklist"`
		Comments  string                 `json:"comments"`
	}
	if !bind(c, &body) {
		return
	}
	audit, err := actions.CompleteAudit(c, member(c), c.Param("leadId"), body.Checklist, body.Comments)
	reply(c, audit, err)
}

// ReassignLead: POST /leads/:leadId/reassign {auditorEmail}
func ReassignLead(c *gin.Context) {
	var body struct {
		AuditorEmail string `json:"auditorEmail"`
	}
	if !bind(c, &body) {
		return
	}
	lead, err := actions.Reassign(c, member(c), c.Param("leadId"), body.AuditorEmail)
	reply(c, lead, err)
}

// TakeUpLead: POST /leads/:leadId/take-up
func TakeUpLead(c *gin.Context) {
	lead, err := actions.TakeUp(c, member(c), c.Param("leadId"))
	reply(c, lead, err)
}

// SendReminder: POST /leads/:leadId/send-reminder
func SendReminder(c *gin.Context) {
	lead, err := actions.SendReminder(c, member(c), c.Param("leadId"))
	reply(c, lead, err)
}

// Rechecks

// GetRechecks: GET /rechecks. Filters: scope=mine|all, status=open|closed,
// view=raisedNotClosed|closedAuditPending|closed, category, auditorEmail, bdaEmail, leadId,
// raised*, closed* date filters (…In presets or …From/…To).
func GetRechecks(c *gin.Context) {
	ranges, ok := queryRanges(c, "raised", "closed")
	if !ok {
		return
	}
	query := models.RecheckQuery{
		Status: c.Query("status"), Category: c.Query("category"),
		AuditorEmail: c.Query("auditorEmail"), BdaEmail: c.Query("bdaEmail"),
		Raised: ranges["raised"], ClosedIn: ranges["closed"],
	}
	if leadID := c.Query("leadId"); leadID != "" {
		query.LeadIDs = []string{leadID}
	}
	switch c.Query("view") {
	case "raisedNotClosed":
		query.Status = models.RecheckOpen
	case "closedAuditPending":
		query.AwaitingReaudit = true
	case "closed":
		query.Status = models.RecheckClosed
	case "", "all":
	default:
		respondError(c, http.StatusBadRequest, "view must be raisedNotClosed, closedAuditPending or closed")
		return
	}
	who := member(c)
	mine := c.Query("scope") == "mine" || (who.Role == models.RoleAuditor && c.Query("scope") == "")
	rechecks, err := actions.ListRechecks(c, who, actions.RecheckListParams{Query: query, Mine: mine})
	reply(c, rechecks, err)
}

// RaiseRecheck: POST /rechecks {leadId, category, comments}
func RaiseRecheck(c *gin.Context) {
	var body struct {
		LeadID   string `json:"leadId"`
		Category string `json:"category"`
		Comments string `json:"comments"`
	}
	if !bind(c, &body) {
		return
	}
	recheck, err := actions.RaiseRecheck(c, member(c), body.LeadID, body.Category, body.Comments)
	reply(c, recheck, err)
}

// CloseRecheck: POST /rechecks/:recheckId/close {note}
func CloseRecheck(c *gin.Context) {
	var body struct {
		Note string `json:"note"`
	}
	if !bind(c, &body) {
		return
	}
	recheck, err := actions.CloseRecheck(c, member(c), c.Param("recheckId"), body.Note)
	reply(c, recheck, err)
}

// GetCcStatus: GET /rechecks/cc-status?status=updated|pending&scope=mine|all&search=&page=
func GetCcStatus(c *gin.Context) {
	who := member(c)
	if !core.Can(who.Role, core.ActionViewCcStatus) {
		respondError(c, http.StatusForbidden, "Only auditors can see the CC status")
		return
	}
	status := c.Query("status")
	if status != "" && status != models.CcUpdated && status != models.CcPending {
		respondError(c, http.StatusBadRequest, "status must be updated or pending")
		return
	}
	page, err := actions.ListLeads(c, who, actions.LeadListParams{
		Query: models.LeadQuery{CcStatus: status, Search: c.Query("search"), Page: queryInt(c, "page"), PageSize: queryInt(c, "pageSize"),
			AuditStatuses: []string{models.AuditPending, models.AuditRecheckOpen, models.AuditRecheckClosed, models.AuditUnassigned}},
		Mine: c.Query("scope") == "mine",
	})
	reply(c, page, err)
}

// Dashboards

// GetTeamDashboard: GET /dashboard/auditor-team?auditorEmail=&periodIn=|periodFrom=&periodTo=
func GetTeamDashboard(c *gin.Context) {
	span, err := queryRange(c, "period")
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	dashboard, err := actions.TeamDashboard(c, member(c), c.Query("auditorEmail"), span)
	reply(c, dashboard, err)
}

// GetBdaDashboard: GET /dashboard/bda?bdaEmail=&periodIn=|periodFrom=&periodTo=
func GetBdaDashboard(c *gin.Context) {
	span, err := queryRange(c, "period")
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	dashboard, err := actions.BdaDashboard(c, member(c), c.Query("bdaEmail"), span)
	reply(c, dashboard, err)
}

// Notifications

// GetNotifications: GET /notifications?unread=true
func GetNotifications(c *gin.Context) {
	list, err := actions.ListNotifications(c, member(c), c.Query("unread") == "true")
	reply(c, list, err)
}

// ReadNotification: POST /notifications/:notificationId/read
func ReadNotification(c *gin.Context) {
	reply(c, gin.H{"read": true}, actions.ReadNotification(c, member(c), c.Param("notificationId")))
}

// ReadAllNotifications: POST /notifications/read-all
func ReadAllNotifications(c *gin.Context) {
	reply(c, gin.H{"read": true}, actions.ReadAllNotifications(c, member(c)))
}

// Operations (auditor TL)

func requireTl(c *gin.Context) (models.Member, bool) {
	who := member(c)
	if who.Role != models.RoleAuditorTl {
		respondError(c, http.StatusForbidden, "Only the auditor TL can do this")
		return who, false
	}
	return who, true
}

// ImportZoho: POST /zoho/import. With a body, imports that Zoho response (the same JSON the
// Zoho API returns); without one, fetches the sync window from Zoho. Then assigns new leads.
func ImportZoho(c *gin.Context) {
	who, ok := requireTl(c)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 32<<20))
	if err != nil {
		respondError(c, http.StatusBadRequest, "Could not read the request body")
		return
	}
	var learners []models.ZohoLearner
	if len(strings.TrimSpace(string(body))) > 0 {
		learners, err = zoho.Decode(body)
		if err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		from, to := zoho.Window(time.Now())
		if learners, err = zoho.Fetch(c, from, to); err != nil {
			respondError(c, http.StatusBadGateway, err.Error())
			return
		}
	}
	imported, importErr := actions.ImportLearners(c, who.Program, learners)
	assigned, err := actions.AssignPending(c, who.Program, core.ActorOf(who))
	if err == nil {
		err = importErr
	}
	reply(c, gin.H{"import": imported, "assignment": assigned}, err)
}

// RunPaymentVerificationSweep: POST /payment-verification/run-sweep (auditor TL). Runs the sweep
// the worker runs every 10 minutes.
func RunPaymentVerificationSweep(c *gin.Context) {
	who, ok := requireTl(c)
	if !ok {
		return
	}
	sent, err := actions.RunPaymentVerificationSweep(c, who.Program)
	reply(c, gin.H{"sent": sent}, err)
}

// GetAlerts: GET /alerts?since=<unix seconds>. The mail log (auditors).
func GetAlerts(c *gin.Context) {
	since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
	alerts, err := actions.ListAlerts(c, member(c), "", since)
	reply(c, alerts, err)
}

// SendTestMail: POST /test-mail {"to": "someone@example.com"}. Sends one mail straight over SMTP.
func SendTestMail(c *gin.Context) {
	if _, ok := requireTl(c); !ok {
		return
	}
	var body struct {
		To string `json:"to"`
	}
	if !bind(c, &body) {
		return
	}
	address, err := netmail.ParseAddress(strings.TrimSpace(body.To))
	if err != nil {
		respondError(c, http.StatusBadRequest, "to must be a valid email address")
		return
	}
	err = actions.SendTestMail(address.Address)
	switch {
	case errors.Is(err, actions.ErrSMTPNotConfigured):
		respondError(c, http.StatusServiceUnavailable, err.Error())
	case err != nil:
		respondError(c, http.StatusBadGateway, "Sending failed: "+err.Error())
	default:
		respondOK(c, gin.H{"sent": true, "to": address.Address})
	}
}
