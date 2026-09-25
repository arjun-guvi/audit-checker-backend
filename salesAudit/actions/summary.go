package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/llm"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// Dashboard summaries: the dashboard's own figures, as compact JSON, go to the configured LLM with
// instructions to write one paragraph from them and nothing else. Only staff names, emails and
// counts are sent; no learner details.

// SummaryTTL is how long a summary is reused while the figures stay the same.
const SummaryTTL = 15 * time.Minute

const summaryInstructions = `You summarise a sales-audit dashboard for %s.
Write exactly one paragraph of 3 to 5 sentences in plain English: no lists, headings, markdown or preamble.
Use only the figures in the JSON you are given. Never invent, estimate or round numbers, and never mention data that is not there.
Start with what needs attention now (%s), then the overall progress, and name the people who stand out, for better or worse.
If every figure is zero, say there was no activity in the period.`

var presetLabels = map[string]string{
	"today": "today", "thisWeek": "this week", "lastWeek": "last week", "thisMonth": "this month", "lastMonth": "last month",
}

// PeriodLabel describes a dashboard period for the summary: its preset, else its dates.
func PeriodLabel(preset string, span models.Range) string {
	if label, ok := presetLabels[preset]; ok {
		return label
	}
	if core.IsSet(span) {
		return fmt.Sprintf("from %s to %s IST", core.FormatTime(span.From), core.FormatTime(span.To))
	}
	return "all time"
}

type teamSummaryFacts struct {
	Period       string           `json:"period"`
	OnlyAuditor  string           `json:"onlyAuditor,omitempty"`
	Totals       teamSummaryRow   `json:"totals"`
	Auditors     []teamSummaryRow `json:"auditors"`
	RecentAudits map[string]int   `json:"recentAuditsByOutcome"`
}

type teamSummaryRow struct {
	Name           string `json:"name,omitempty"`
	Region         string `json:"region,omitempty"`
	Away           bool   `json:"away,omitempty"`
	Assigned       int    `json:"leadsAssigned"`
	Open           int    `json:"leadsStillOpen"`
	AuditsDone     int    `json:"auditsDoneInPeriod"`
	Completed      int    `json:"completedInPeriod"`
	RechecksRaised int    `json:"rechecksRaisedInPeriod"`
	LastAudit      string `json:"lastAudit,omitempty"`
}

func teamRow(stats models.AuditorStats) teamSummaryRow {
	return teamSummaryRow{
		Name: stats.Name, Region: stats.Region, Away: stats.Email != "" && !stats.Available,
		Assigned: stats.Assigned, Open: stats.Pending, AuditsDone: stats.AuditsDone, Completed: stats.Completed,
		RechecksRaised: stats.RechecksRaised, LastAudit: core.FormatTime(stats.LastAuditAt),
	}
}

// TeamDashboardSummary is GET /dashboard/auditor-team/summary, for the auditor TL.
func TeamDashboardSummary(ctx context.Context, member models.Member, auditorEmail string, span models.Range, period string, refresh bool) (models.DashboardSummary, error) {
	dashboard, err := TeamDashboard(ctx, member, auditorEmail, span)
	if err != nil {
		return models.DashboardSummary{}, err
	}
	facts := teamSummaryFacts{Period: period, Totals: teamRow(dashboard.Totals), RecentAudits: map[string]int{}}
	if auditorEmail != "" && len(dashboard.Auditors) == 1 {
		facts.OnlyAuditor = dashboard.Auditors[0].Name
	}
	for _, auditor := range dashboard.Auditors {
		facts.Auditors = append(facts.Auditors, teamRow(auditor))
	}
	for _, audit := range dashboard.Recent {
		facts.RecentAudits[audit.Outcome]++
	}
	return summarize(ctx, "team", member, facts, refresh, fmt.Sprintf(summaryInstructions,
		"an auditor team lead", "leads still open, auditors with a backlog or who are away, rechecks raised"))
}

type bdmSummaryFacts struct {
	Period           string          `json:"rechecksRaisedPeriod"`
	OnlyBda          string          `json:"onlyBda,omitempty"`
	Totals           bdmSummaryRow   `json:"teamTotals"`
	Bdas             []bdmSummaryRow `json:"bdas"`
	CcTicketsToClose []ccTicketFact  `json:"ccUpdatedButTicketStillOpen"`
}

type bdmSummaryRow struct {
	Name           string         `json:"name,omitempty"`
	Leads          int            `json:"leads"`
	AuditCompleted int            `json:"auditCompleted"`
	RechecksOpen   int            `json:"rechecksOpen"`
	RechecksClosed int            `json:"rechecksClosed"`
	ByCategory     map[string]int `json:"rechecksByReason"`
}

// ccTicketFact counts whole hours, so the figures (and the cached summary) only change hourly.
type ccTicketFact struct {
	RecheckNo          string `json:"recheckNo"`
	Bda                string `json:"bda"`
	HoursSinceCcUpdate int64  `json:"hoursSinceCcUpdate"`
}

func bdmRow(stats models.BdaStats) bdmSummaryRow {
	byCategory := map[string]int{}
	for category, count := range stats.ByCategory {
		byCategory[models.RecheckCategories[category]] = count
	}
	return bdmSummaryRow{
		Name: stats.Name, Leads: stats.Leads, AuditCompleted: stats.AuditCompleted,
		RechecksOpen: stats.RechecksOpen, RechecksClosed: stats.RechecksClosed, ByCategory: byCategory,
	}
}

// BdaDashboardSummary is GET /dashboard/bda/summary, for the BDM.
func BdaDashboardSummary(ctx context.Context, member models.Member, bdaEmail string, span models.Range, period string, refresh bool) (models.DashboardSummary, error) {
	if member.Role != models.RoleBdm {
		return models.DashboardSummary{}, forbidden("Only the BDM gets the team summary")
	}
	dashboard, err := BdaDashboard(ctx, member, bdaEmail, span)
	if err != nil {
		return models.DashboardSummary{}, err
	}
	scope, _, err := scopeOf(ctx, member, false)
	if err != nil {
		return models.DashboardSummary{}, err
	}
	toClose, err := store.FindRechecks(ctx, member.Program, models.RecheckQuery{
		Scope: scope, BdaEmails: core.NonEmpty(bdaEmail), CcUpdatedOpen: true,
	})
	if err != nil {
		return models.DashboardSummary{}, err
	}
	facts := bdmSummaryFacts{Period: period, Totals: bdmRow(dashboard.Totals), CcTicketsToClose: []ccTicketFact{}}
	if bdaEmail != "" && len(dashboard.Bdas) == 1 {
		facts.OnlyBda = dashboard.Bdas[0].Name
	}
	for _, bda := range dashboard.Bdas {
		facts.Bdas = append(facts.Bdas, bdmRow(bda))
	}
	now := nowSeconds()
	for _, recheck := range toClose {
		facts.CcTicketsToClose = append(facts.CcTicketsToClose, ccTicketFact{
			RecheckNo: recheck.RecheckNo, Bda: recheck.BdaEmail, HoursSinceCcUpdate: max(now-recheck.CcUpdatedAt, 0) / 3600,
		})
	}
	return summarize(ctx, "bdm", member, facts, refresh, fmt.Sprintf(summaryInstructions,
		"a sales manager (BDM) whose BDAs must fix rechecks raised by the audit team",
		"tickets whose CC was updated but are still open (these must be closed urgently), open rechecks and which BDAs have the most"))
}

type cachedSummary struct {
	summary models.DashboardSummary
	expires time.Time
}

var (
	summaryCache   = map[string]cachedSummary{}
	summaryCacheMu sync.Mutex
)

// summarize returns the cached summary for these exact figures, or asks the LLM for one. refresh
// skips the cache.
func summarize(ctx context.Context, kind string, member models.Member, facts any, refresh bool, instructions string) (models.DashboardSummary, error) {
	if !llm.Configured() {
		return models.DashboardSummary{}, Error{Status: http.StatusServiceUnavailable,
			Message: "The dashboard summary is not set up yet: set LLM_API_URL and LLM_MODEL in the backend .env"}
	}
	data, err := json.Marshal(facts)
	if err != nil {
		return models.DashboardSummary{}, err
	}
	hash := sha256.Sum256(append([]byte(kind+"\x00"+member.Program+"\x00"+strings.ToLower(member.Email)+"\x00"+llm.Model()+"\x00"), data...))
	key := hex.EncodeToString(hash[:])
	now := Now()

	summaryCacheMu.Lock()
	cached, ok := summaryCache[key]
	summaryCacheMu.Unlock()
	if ok && !refresh && now.Before(cached.expires) {
		cached.summary.Cached = true
		return cached.summary, nil
	}

	text, err := llm.Complete(ctx, instructions, "Dashboard figures (JSON):\n"+string(data))
	if err != nil {
		log.Printf("salesAudit: dashboard summary: %v", err)
		if errors.Is(err, context.Canceled) {
			return models.DashboardSummary{}, err
		}
		return models.DashboardSummary{}, Error{Status: http.StatusBadGateway, Message: "Could not write the summary right now. Try again in a moment."}
	}
	summary := models.DashboardSummary{Summary: oneParagraph(text), Model: llm.Model(), GeneratedAt: now.Unix()}

	summaryCacheMu.Lock()
	for existing, entry := range summaryCache {
		if now.After(entry.expires) {
			delete(summaryCache, existing)
		}
	}
	summaryCache[key] = cachedSummary{summary: summary, expires: now.Add(SummaryTTL)}
	summaryCacheMu.Unlock()
	return summary, nil
}

// oneParagraph joins the model's lines into one paragraph, in case it wrote several.
func oneParagraph(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
