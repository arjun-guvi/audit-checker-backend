package controllers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"auditApp/config"
	"auditApp/salesAudit/actions"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/routes"
	"auditApp/salesAudit/store/fake"

	"github.com/gin-gonic/gin"
)

// Handler tests over the real routes with the in-memory fake store: no database needed.

const program = "test-program"

var now = time.Date(2026, 9, 24, 12, 0, 0, 0, core.IST)

type env struct {
	t      *testing.T
	engine *gin.Engine
	data   *fake.Data
	mails  *[]string
}

func setup(t *testing.T) env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	data := fake.Install(t)
	config.SalesAuditProgram = program
	actions.Now = func() time.Time { return now }
	actions.Random = rand.New(rand.NewSource(1))
	mails := &[]string{}
	actions.EnqueueMail = func(_, alertID string) error { *mails = append(*mails, alertID); return nil }
	t.Cleanup(func() { actions.Now = time.Now })

	member := func(email, role, region, manager string) models.Member {
		return models.Member{ID: core.NewID(), Program: program, UserHash: "hash-" + email, Email: email, Name: email,
			Role: role, Region: region, ManagerEmail: manager, Available: true}
	}
	data.Members = []models.Member{
		member("tl@example.com", models.RoleAuditorTl, "", ""),
		member("north@example.com", models.RoleAuditor, models.RegionNorth, "tl@example.com"),
		member("south1@example.com", models.RoleAuditor, models.RegionSouth, "tl@example.com"),
		member("south2@example.com", models.RoleAuditor, models.RegionSouth, "tl@example.com"),
		member("bdm@example.com", models.RoleBdm, "", ""),
		member("bda@example.com", models.RoleBda, "", "bdm@example.com"),
		member("otherbda@example.com", models.RoleBda, "", "otherbdm@example.com"),
	}
	engine := gin.New()
	routes.Register(engine)
	return env{t: t, engine: engine, data: data, mails: mails}
}

// call makes a request as the member with that email and decodes data into out (when given).
func call(e env, as, method, path string, body any, out any) int {
	e.t.Helper()
	var reader *bytes.Reader
	switch value := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(value))
	default:
		raw, _ := json.Marshal(value)
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, "/sales-audit"+path, reader)
	if as != "" {
		request.Header.Set("Authorization", "hash-"+as)
	}
	recorder := httptest.NewRecorder()
	e.engine.ServeHTTP(recorder, request)
	var envelope struct {
		Status  string          `json:"status"`
		Data    json.RawMessage `json:"data"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		e.t.Fatalf("%s %s: bad JSON %q", method, path, recorder.Body.String())
	}
	if recorder.Code == http.StatusOK && out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			e.t.Fatalf("%s %s: decoding data: %v", method, path, err)
		}
	}
	return recorder.Code
}

// zohoLearners builds a fake Zoho response: north and south leads, all payments verified.
func zohoLearners(north, south int) string {
	learners := []string{}
	for index := range north + south {
		team := "South"
		if index < north {
			team = "North"
		}
		learners = append(learners, fmt.Sprintf(`{"zenId":"%d","name":"Learner %d","email":"learner%d@example.com",
			"phone":"+9100000000%02d","salesTeam":"%s","saleOwner":"bda@example.com","saleOwnerManager":"bdm@example.com",
			"paymenttype":"Direct - Partial Payment","courseFee":70000,"product":"Zen_Course","dateOfEnrollment":"2026-09-20",
			"confirmationCall":"https://drive.google.com/file/d/file%d/view","terms&conditions":"Yes",
			"financialDetails":[{"recordId":%d,"type":"Credit_Booking_Amount","amount":999,"verified":"Yes","paymentDate":"2026-09-20"}],
			"batchData":{"batchName":"B1","startDate":"2026-10-01"}}`, 1000+index, index, index, index, team, index, 5000+index))
	}
	return `{"result":[` + strings.Join(learners, ",") + `]}`
}

func TestAuthAndMembership(t *testing.T) {
	e := setup(t)
	if code := call(e, "", http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code := call(e, "stranger@example.com", http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("unknown member: %d", code)
	}
	var me models.CurrentUser
	if code := call(e, "bdm@example.com", http.MethodGet, "/me", nil, &me); code != http.StatusOK || me.Role != models.RoleBdm ||
		len(me.TeamEmails) != 1 || me.TeamEmails[0] != "bda@example.com" {
		t.Fatalf("me: %d %+v", code, me)
	}
}

func TestImportAssignsByRegion(t *testing.T) {
	e := setup(t)
	if code := call(e, "south1@example.com", http.MethodPost, "/zoho/import", zohoLearners(2, 4), nil); code != http.StatusForbidden {
		t.Fatalf("only the TL imports: %d", code)
	}
	var result struct {
		Import     actions.ImportResult `json:"import"`
		Assignment actions.AssignResult `json:"assignment"`
	}
	if code := call(e, "tl@example.com", http.MethodPost, "/zoho/import", zohoLearners(2, 4), &result); code != http.StatusOK {
		t.Fatalf("import: %d", code)
	}
	if result.Import.Created != 6 || result.Assignment.Assigned != 6 ||
		result.Assignment.ByAuditor["north@example.com"] != 2 ||
		result.Assignment.ByAuditor["south1@example.com"] != 2 || result.Assignment.ByAuditor["south2@example.com"] != 2 {
		t.Fatalf("result: %+v", result)
	}
	var mine models.Page[models.Lead]
	call(e, "north@example.com", http.MethodGet, "/leads?scope=mine", nil, &mine)
	if mine.Total != 2 {
		t.Fatalf("north's My Leads: %d", mine.Total)
	}
	var notifications actions.NotificationList
	call(e, "north@example.com", http.MethodGet, "/notifications", nil, &notifications)
	if notifications.Unread != 1 {
		t.Fatalf("north should get one digest notification, got %+v", notifications)
	}
	// Importing again changes nothing about the assignment.
	call(e, "tl@example.com", http.MethodPost, "/zoho/import", zohoLearners(2, 4), &result)
	if result.Import.Updated != 6 || result.Assignment.Assigned != 0 {
		t.Fatalf("re-import: %+v", result)
	}
}

func TestAuditRecheckReauditLoop(t *testing.T) {
	e := setup(t)
	call(e, "tl@example.com", http.MethodPost, "/zoho/import", zohoLearners(1, 0), nil)
	lead := e.data.Leads[0]
	leadPath := "/leads/" + lead.ID

	// The audit view compares the database with the CC.
	var view models.AuditView
	if code := call(e, "north@example.com", http.MethodGet, leadPath+"/audit", nil, &view); code != http.StatusOK ||
		len(view.Comparison) == 0 || !view.Actions.CanComplete || view.Cc.Type != models.CcTypePdf {
		t.Fatalf("audit view: %d %+v", code, view.Actions)
	}
	var verification models.CcVerification
	if code := call(e, "north@example.com", http.MethodGet, leadPath+"/cc-verification", nil, &verification); code != http.StatusOK ||
		!strings.HasSuffix(verification.PreviewURL, "/preview") {
		t.Fatalf("cc verification: %d %+v", code, verification.PreviewURL)
	}

	// Only auditors raise rechecks, and only with a known category and comments.
	body := map[string]string{"leadId": lead.ID, "category": models.CategoryPayment, "comments": "UTR missing"}
	if code := call(e, "bda@example.com", http.MethodPost, "/rechecks", body, nil); code != http.StatusForbidden {
		t.Fatalf("BDA raising: %d", code)
	}
	if code := call(e, "north@example.com", http.MethodPost, "/rechecks", map[string]string{"leadId": lead.ID, "category": "nope", "comments": "x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("bad category: %d", code)
	}
	var recheck models.Recheck
	if code := call(e, "north@example.com", http.MethodPost, "/rechecks", body, &recheck); code != http.StatusOK || recheck.RecheckNo != "RC-000001" {
		t.Fatalf("raise: %d %+v", code, recheck)
	}
	if !strings.Contains(recheck.Alert.Subject, "RC-000001") || len(recheck.Alert.To) != 2 {
		t.Fatalf("the BDA and BDM mail must carry the recheck id: %+v", recheck.Alert)
	}

	// The audit cannot be completed while the recheck is open.
	checklist := []models.ChecklistItem{}
	for _, item := range core.Checklist() {
		checklist = append(checklist, models.ChecklistItem{Key: item.Key, Checked: true})
	}
	complete := map[string]any{"checklist": checklist, "comments": "All good"}
	if code := call(e, "north@example.com", http.MethodPost, leadPath+"/complete-audit", complete, nil); code != http.StatusBadRequest {
		t.Fatalf("complete with open recheck: %d", code)
	}

	// The BDA sees it as a ticket; a BDA from another team does not, and cannot close it.
	var tickets []models.Recheck
	call(e, "bda@example.com", http.MethodGet, "/rechecks?view=raisedNotClosed", nil, &tickets)
	if len(tickets) != 1 {
		t.Fatalf("BDA tickets: %d", len(tickets))
	}
	call(e, "otherbda@example.com", http.MethodGet, "/rechecks", nil, &tickets)
	if len(tickets) != 0 {
		t.Fatalf("other BDA sees %d tickets", len(tickets))
	}
	if code := call(e, "otherbda@example.com", http.MethodPost, "/rechecks/"+recheck.ID+"/close", map[string]string{"note": "x"}, nil); code != http.StatusForbidden {
		t.Fatalf("other BDA closing: %d", code)
	}
	if code := call(e, "bda@example.com", http.MethodPost, "/rechecks/"+recheck.ID+"/close", map[string]string{"note": "Shared the UTR"}, &recheck); code != http.StatusOK ||
		recheck.Closed == nil || recheck.Closed.By.Email != "bda@example.com" || recheck.Closed.By.Role != models.RoleBda {
		t.Fatalf("close: %d %+v", code, recheck.Closed)
	}

	// Closed, awaiting re-audit.
	var awaiting []models.Recheck
	call(e, "north@example.com", http.MethodGet, "/rechecks?view=closedAuditPending", nil, &awaiting)
	var detail models.LeadDetail
	call(e, "north@example.com", http.MethodGet, leadPath, nil, &detail)
	if len(awaiting) != 1 || detail.Lead.Audit.Status != models.AuditRecheckClosed || !detail.Actions.CanReaudit {
		t.Fatalf("awaiting re-audit: %d %s %+v", len(awaiting), detail.Lead.Audit.Status, detail.Actions)
	}

	// Re-audit: the checklist must be complete, then the audit completes.
	partial := map[string]any{"checklist": checklist[:3], "comments": "All good"}
	if code := call(e, "north@example.com", http.MethodPost, leadPath+"/complete-audit", partial, nil); code != http.StatusBadRequest {
		t.Fatalf("partial checklist: %d", code)
	}
	if code := call(e, "south1@example.com", http.MethodPost, leadPath+"/complete-audit", complete, nil); code != http.StatusForbidden {
		t.Fatalf("another auditor completing: %d", code)
	}
	var audit models.Audit
	if code := call(e, "north@example.com", http.MethodPost, leadPath+"/complete-audit", complete, &audit); code != http.StatusOK || audit.Attempt != 2 {
		t.Fatalf("complete: %d %+v", code, audit)
	}
	call(e, "north@example.com", http.MethodGet, "/rechecks?view=closedAuditPending", nil, &awaiting)
	if len(awaiting) != 0 {
		t.Fatal("the closed recheck is re-audited now")
	}

	// The timeline has every step, in order.
	var events []models.Event
	call(e, "bdm@example.com", http.MethodGet, leadPath+"/timeline", nil, &events)
	types := []string{}
	for _, event := range events {
		types = append(types, event.Type)
	}
	want := "leadImported,ccUpdated,assigned,recheckRaised,recheckClosed,auditCompleted"
	if strings.Join(types, ",") != want {
		t.Fatalf("timeline: %v, want %s", types, want)
	}

	// Filters: leads with a recheck closed this month, and leads completed this week.
	var page models.Page[models.Lead]
	call(e, "tl@example.com", http.MethodGet, "/leads?recheckClosedIn=thisMonth", nil, &page)
	if page.Total != 1 {
		t.Fatalf("recheck closed this month: %d", page.Total)
	}
	call(e, "tl@example.com", http.MethodGet, "/leads?completedIn=lastWeek", nil, &page)
	if page.Total != 0 {
		t.Fatalf("completed last week: %d", page.Total)
	}

	// Dashboards.
	var team models.TeamDashboard
	if code := call(e, "tl@example.com", http.MethodGet, "/dashboard/auditor-team?periodIn=thisMonth", nil, &team); code != http.StatusOK ||
		team.Totals.AuditsDone != 2 || team.Totals.Completed != 1 || team.Totals.RechecksRaised != 1 {
		t.Fatalf("team dashboard: %d %+v", code, team.Totals)
	}
	if code := call(e, "north@example.com", http.MethodGet, "/dashboard/auditor-team", nil, nil); code != http.StatusForbidden {
		t.Fatalf("auditor on TL dashboard: %d", code)
	}
	var bda models.BdaDashboard
	call(e, "bdm@example.com", http.MethodGet, "/dashboard/bda", nil, &bda)
	if bda.Totals.RechecksClosed != 1 || bda.Totals.ByCategory[models.CategoryPayment] != 1 || len(bda.Bdas) != 1 {
		t.Fatalf("bda dashboard: %+v", bda)
	}
}

func TestReassignAndTakeUp(t *testing.T) {
	e := setup(t)
	call(e, "tl@example.com", http.MethodPost, "/zoho/import", zohoLearners(0, 1), nil)
	lead := e.data.Leads[0]
	owner := lead.Assignment.AuditorEmail
	other := "south1@example.com"
	if owner == other {
		other = "south2@example.com"
	}
	var moved models.Lead
	if code := call(e, other, http.MethodPost, "/leads/"+lead.ID+"/take-up", nil, &moved); code != http.StatusOK ||
		moved.Assignment.AuditorEmail != other || moved.Assignment.Mode != models.AssignTakeUp {
		t.Fatalf("take up: %d %+v", code, moved.Assignment)
	}
	if code := call(e, "tl@example.com", http.MethodPost, "/leads/"+lead.ID+"/reassign", map[string]string{"auditorEmail": "bda@example.com"}, nil); code != http.StatusBadRequest {
		t.Fatalf("reassign to a BDA: %d", code)
	}
	if code := call(e, "tl@example.com", http.MethodPost, "/leads/"+lead.ID+"/reassign", map[string]string{"auditorEmail": owner}, &moved); code != http.StatusOK ||
		moved.Assignment.AuditorEmail != owner || moved.Assignment.Mode != models.AssignManual {
		t.Fatalf("reassign: %d %+v", code, moved.Assignment)
	}
	if code := call(e, "bda@example.com", http.MethodPost, "/leads/"+lead.ID+"/take-up", nil, nil); code != http.StatusForbidden {
		t.Fatalf("BDA take up: %d", code)
	}
}

func TestCcStatusAndLeadVisibility(t *testing.T) {
	e := setup(t)
	call(e, "tl@example.com", http.MethodPost, "/zoho/import",
		strings.Replace(zohoLearners(0, 2), `"confirmationCall":"https://drive.google.com/file/d/file1/view",`, "", 1), nil)
	var page models.Page[models.Lead]
	call(e, "tl@example.com", http.MethodGet, "/rechecks/cc-status?status=pending", nil, &page)
	if page.Total != 1 {
		t.Fatalf("pending CC: %d", page.Total)
	}
	if code := call(e, "bda@example.com", http.MethodGet, "/rechecks/cc-status", nil, nil); code != http.StatusForbidden {
		t.Fatalf("BDA on CC status: %d", code)
	}
	call(e, "otherbda@example.com", http.MethodGet, "/leads", nil, &page)
	if page.Total != 0 {
		t.Fatalf("other BDA sees %d leads", page.Total)
	}
	if code := call(e, "otherbda@example.com", http.MethodGet, "/leads/"+e.data.Leads[0].ID, nil, nil); code != http.StatusNotFound {
		t.Fatalf("other BDA opening a lead: %d", code)
	}
	if code := call(e, "tl@example.com", http.MethodGet, "/leads?completedIn=someday", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad preset: %d", code)
	}
}
