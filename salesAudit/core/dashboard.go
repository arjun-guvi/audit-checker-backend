package core

import (
	"sort"
	"strings"

	"auditApp/salesAudit/models"
)

// TeamStats builds the auditor TL dashboard: per auditor, what is assigned now and what they did
// in the window (audits done, completed, rechecks raised, per day).
func TeamStats(auditors []models.Member, leads []models.Lead, audits []models.Audit, rechecks []models.Recheck, span models.Range) models.TeamDashboard {
	rows := map[string]*models.AuditorStats{}
	daily := map[string]map[string]*models.DayCount{}
	order := []string{}
	row := func(email string) *models.AuditorStats {
		key := strings.ToLower(email)
		if rows[key] == nil {
			rows[key] = &models.AuditorStats{Email: key, Name: key}
			daily[key] = map[string]*models.DayCount{}
			order = append(order, key)
		}
		return rows[key]
	}
	day := func(email string, at int64) *models.DayCount {
		key, date := strings.ToLower(email), DayKey(at)
		if daily[key][date] == nil {
			daily[key][date] = &models.DayCount{Date: date}
		}
		return daily[key][date]
	}
	for _, auditor := range auditors {
		stats := row(auditor.Email)
		stats.Name, stats.Region, stats.Available = auditor.Name, auditor.Region, auditor.Available
	}
	for _, lead := range leads {
		if lead.Assignment == nil || rows[strings.ToLower(lead.Assignment.AuditorEmail)] == nil {
			continue
		}
		stats := row(lead.Assignment.AuditorEmail)
		stats.Assigned++
		if lead.Audit.Status != models.AuditCompleted {
			stats.Pending++
		}
	}
	for _, audit := range audits {
		if rows[strings.ToLower(audit.Auditor.Email)] == nil || !inWindow(span, audit.SubmittedAt) {
			continue
		}
		stats := row(audit.Auditor.Email)
		stats.AuditsDone++
		stats.LastAuditAt = max(stats.LastAuditAt, audit.SubmittedAt)
		day(audit.Auditor.Email, audit.SubmittedAt).AuditsDone++
		if audit.Outcome == models.OutcomeCompleted {
			stats.Completed++
			day(audit.Auditor.Email, audit.SubmittedAt).Completed++
		}
	}
	for _, recheck := range rechecks {
		if recheck.Source != models.SourcePortal || rows[strings.ToLower(recheck.RaisedBy.Email)] == nil || !inWindow(span, recheck.RaisedAt) {
			continue
		}
		row(recheck.RaisedBy.Email).RechecksRaised++
		day(recheck.RaisedBy.Email, recheck.RaisedAt).RechecksRaised++
	}

	result := models.TeamDashboard{Auditors: []models.AuditorStats{}, Recent: []models.Audit{}}
	result.Totals.Name, result.Totals.Daily = "All auditors", []models.DayCount{}
	for _, key := range order {
		stats := rows[key]
		stats.Daily = []models.DayCount{}
		for _, count := range daily[key] {
			stats.Daily = append(stats.Daily, *count)
		}
		sort.Slice(stats.Daily, func(i, j int) bool { return stats.Daily[i].Date < stats.Daily[j].Date })
		result.Auditors = append(result.Auditors, *stats)
		result.Totals.Assigned += stats.Assigned
		result.Totals.Pending += stats.Pending
		result.Totals.AuditsDone += stats.AuditsDone
		result.Totals.Completed += stats.Completed
		result.Totals.RechecksRaised += stats.RechecksRaised
		result.Totals.LastAuditAt = max(result.Totals.LastAuditAt, stats.LastAuditAt)
	}
	for _, audit := range audits {
		if rows[strings.ToLower(audit.Auditor.Email)] != nil && inWindow(span, audit.SubmittedAt) {
			result.Recent = append(result.Recent, audit)
		}
	}
	sort.Slice(result.Recent, func(i, j int) bool { return result.Recent[i].SubmittedAt > result.Recent[j].SubmittedAt })
	if len(result.Recent) > 50 {
		result.Recent = result.Recent[:50]
	}
	return result
}

// BdaStatsOf builds the BDA / BDM dashboard: per BDA, their leads, completed audits and rechecks
// (open, closed and by category; rechecks raised in the window when one is given).
func BdaStatsOf(bdas []models.Member, leads []models.Lead, rechecks []models.Recheck, span models.Range) models.BdaDashboard {
	rows := map[string]*models.BdaStats{}
	order := []string{}
	row := func(email string) *models.BdaStats {
		key := strings.ToLower(email)
		if rows[key] == nil {
			rows[key] = &models.BdaStats{Email: key, Name: key, ByCategory: map[string]int{}}
			order = append(order, key)
		}
		return rows[key]
	}
	for _, bda := range bdas {
		row(bda.Email).Name = bda.Name
	}
	for _, lead := range leads {
		stats := row(lead.BdaEmail)
		stats.Leads++
		if lead.Audit.Status == models.AuditCompleted {
			stats.AuditCompleted++
		}
	}
	for _, recheck := range rechecks {
		if IsSet(span) && !InRange(span, recheck.RaisedAt) {
			continue
		}
		stats := row(recheck.BdaEmail)
		if recheck.Status == models.RecheckOpen {
			stats.RechecksOpen++
		} else {
			stats.RechecksClosed++
		}
		for _, reason := range ReasonsOf(recheck) {
			stats.ByCategory[reason.Category]++
		}
	}
	result := models.BdaDashboard{Bdas: []models.BdaStats{}, Totals: models.BdaStats{Name: "Total", ByCategory: map[string]int{}}}
	for _, key := range order {
		stats := rows[key]
		result.Bdas = append(result.Bdas, *stats)
		result.Totals.Leads += stats.Leads
		result.Totals.AuditCompleted += stats.AuditCompleted
		result.Totals.RechecksOpen += stats.RechecksOpen
		result.Totals.RechecksClosed += stats.RechecksClosed
		for category, count := range stats.ByCategory {
			result.Totals.ByCategory[category] += count
		}
	}
	return result
}

func inWindow(span models.Range, at int64) bool {
	return !IsSet(span) || InRange(span, at)
}
