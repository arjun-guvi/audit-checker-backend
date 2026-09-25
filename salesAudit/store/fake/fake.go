// Package fake swaps the store's functions for in-memory versions, so handler and action tests
// run without MongoDB. Install(t) returns the Data the fake reads and writes; seed it directly.
package fake

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// Data is everything the fake store holds.
type Data struct {
	Mu            sync.Mutex
	Members       []models.Member
	Leads         []models.Lead
	Rechecks      []models.Recheck
	Audits        []models.Audit
	Events        []models.Event
	Notifications []models.Notification
	Alerts        []models.Alert
	Extracts      []models.CcExtract
	Counters      map[string]int64
}

// Install replaces every store function for the test and restores them afterwards.
func Install(t *testing.T) *Data {
	t.Helper()
	data := &Data{Counters: map[string]int64{}}
	restore := snapshot()
	t.Cleanup(restore)
	wire(data)
	return data
}

func live(program string, docProgram string, deleted bool) bool {
	return docProgram == program && !deleted
}

func find[T any](items []T, match func(T) bool) (T, int) {
	for index, item := range items {
		if match(item) {
			return item, index
		}
	}
	var zero T
	return zero, -1
}

func wire(d *Data) {
	lock := func() func() { d.Mu.Lock(); return d.Mu.Unlock }

	// Members
	store.FindMemberByHash = func(_ context.Context, program, userHash string) (models.Member, error) {
		defer lock()()
		member, index := find(d.Members, func(m models.Member) bool { return live(program, m.Program, m.Deleted) && m.UserHash == userHash })
		return member, notFoundIf(index)
	}
	store.FindMemberByEmail = func(_ context.Context, program, email string) (models.Member, error) {
		defer lock()()
		member, index := find(d.Members, func(m models.Member) bool {
			return live(program, m.Program, m.Deleted) && strings.EqualFold(m.Email, email)
		})
		return member, notFoundIf(index)
	}
	store.FindMember = func(_ context.Context, program, memberID string) (models.Member, error) {
		defer lock()()
		member, index := find(d.Members, func(m models.Member) bool { return live(program, m.Program, m.Deleted) && m.ID == memberID })
		return member, notFoundIf(index)
	}
	store.FindMembers = func(_ context.Context, program, role string) ([]models.Member, error) {
		defer lock()()
		result := []models.Member{}
		for _, member := range d.Members {
			if live(program, member.Program, member.Deleted) && (role == "" || member.Role == role) {
				result = append(result, member)
			}
		}
		return result, nil
	}
	store.InsertMember = func(_ context.Context, member models.Member) error {
		defer lock()()
		d.Members = append(d.Members, member)
		return nil
	}
	store.ReplaceMember = func(_ context.Context, member models.Member) error {
		defer lock()()
		_, index := find(d.Members, func(m models.Member) bool { return live(member.Program, m.Program, m.Deleted) && m.ID == member.ID })
		if index < 0 {
			return store.ErrNotFound
		}
		d.Members[index] = member
		return nil
	}

	// Leads
	store.FindLeads = func(_ context.Context, program string, query models.LeadQuery) ([]models.Lead, int, error) {
		defer lock()()
		result := []models.Lead{}
		for _, lead := range d.Leads {
			if live(program, lead.Program, lead.Deleted) && core.MatchLead(query, lead) {
				result = append(result, lead)
			}
		}
		sort.SliceStable(result, func(i, j int) bool {
			if result[i].EnrolledAt != result[j].EnrolledAt {
				return result[i].EnrolledAt > result[j].EnrolledAt
			}
			return result[i].CrmCreatedAt > result[j].CrmCreatedAt
		})
		total := len(result)
		if query.PageSize > 0 {
			start := min(max(query.Page-1, 0)*query.PageSize, total)
			result = result[start:min(start+query.PageSize, total)]
		}
		return result, total, nil
	}
	store.FindLead = func(_ context.Context, program, leadID string) (models.Lead, error) {
		defer lock()()
		lead, index := find(d.Leads, func(l models.Lead) bool { return live(program, l.Program, l.Deleted) && l.ID == leadID })
		return lead, notFoundIf(index)
	}
	store.FindLeadByZenID = func(_ context.Context, program, zenID string) (models.Lead, error) {
		defer lock()()
		lead, index := find(d.Leads, func(l models.Lead) bool { return live(program, l.Program, l.Deleted) && l.ZenID == zenID })
		return lead, notFoundIf(index)
	}
	store.InsertLead = func(_ context.Context, lead models.Lead) error {
		defer lock()()
		d.Leads = append(d.Leads, lead)
		return nil
	}
	updateLead := func(program, leadID string, apply func(*models.Lead)) error {
		defer lock()()
		_, index := find(d.Leads, func(l models.Lead) bool { return live(program, l.Program, l.Deleted) && l.ID == leadID })
		if index < 0 {
			return store.ErrNotFound
		}
		apply(&d.Leads[index])
		return nil
	}
	store.UpdateLeadZoho = func(_ context.Context, lead models.Lead) error {
		return updateLead(lead.Program, lead.ID, func(existing *models.Lead) {
			keep := *existing
			*existing = lead
			existing.Assignment, existing.Audit, existing.RecheckSummary = keep.Assignment, keep.Audit, keep.RecheckSummary
			existing.PaymentVerificationMailedAt, existing.LastEscalationAt = keep.PaymentVerificationMailedAt, keep.LastEscalationAt
			existing.Created = keep.Created
		})
	}
	store.UpdateLeadWorkflow = func(_ context.Context, lead models.Lead) error {
		return updateLead(lead.Program, lead.ID, func(existing *models.Lead) {
			existing.Assignment, existing.Audit, existing.RecheckSummary = lead.Assignment, lead.Audit, lead.RecheckSummary
		})
	}
	store.SetLeadMailMarks = func(_ context.Context, program, leadID string, paymentVerificationMailedAt, lastEscalationAt int64) error {
		return updateLead(program, leadID, func(existing *models.Lead) {
			existing.PaymentVerificationMailedAt, existing.LastEscalationAt = paymentVerificationMailedAt, lastEscalationAt
		})
	}

	// Rechecks
	store.FindRechecks = func(_ context.Context, program string, query models.RecheckQuery) ([]models.Recheck, error) {
		defer lock()()
		result := []models.Recheck{}
		for _, recheck := range d.Rechecks {
			if live(program, recheck.Program, recheck.Deleted) && core.MatchRecheck(query, recheck) {
				result = append(result, recheck)
			}
		}
		sort.SliceStable(result, func(i, j int) bool { return result[i].RaisedAt > result[j].RaisedAt })
		return result, nil
	}
	store.FindRecheck = func(_ context.Context, program, recheckID string) (models.Recheck, error) {
		defer lock()()
		recheck, index := find(d.Rechecks, func(r models.Recheck) bool { return live(program, r.Program, r.Deleted) && r.ID == recheckID })
		return recheck, notFoundIf(index)
	}
	store.FindRecheckByNo = func(_ context.Context, program, recheckNo string) (models.Recheck, error) {
		defer lock()()
		recheck, index := find(d.Rechecks, func(r models.Recheck) bool {
			return live(program, r.Program, r.Deleted) && r.RecheckNo == recheckNo
		})
		return recheck, notFoundIf(index)
	}
	store.InsertRecheck = func(_ context.Context, recheck models.Recheck) error {
		defer lock()()
		d.Rechecks = append(d.Rechecks, recheck)
		return nil
	}
	store.ReplaceRecheck = func(_ context.Context, recheck models.Recheck) error {
		defer lock()()
		_, index := find(d.Rechecks, func(r models.Recheck) bool { return live(recheck.Program, r.Program, r.Deleted) && r.ID == recheck.ID })
		if index < 0 {
			return store.ErrNotFound
		}
		d.Rechecks[index] = recheck
		return nil
	}

	// Audits, events, notifications, counters
	store.InsertAudit = func(_ context.Context, audit models.Audit) error {
		defer lock()()
		d.Audits = append(d.Audits, audit)
		return nil
	}
	store.FindAudits = func(_ context.Context, program string, query models.AuditQuery) ([]models.Audit, error) {
		defer lock()()
		result := []models.Audit{}
		for _, audit := range d.Audits {
			if live(program, audit.Program, audit.Deleted) && core.MatchAudit(query, audit) {
				result = append(result, audit)
			}
		}
		sort.SliceStable(result, func(i, j int) bool { return result[i].SubmittedAt > result[j].SubmittedAt })
		return result, nil
	}
	store.InsertEvent = func(_ context.Context, event models.Event) error {
		defer lock()()
		d.Events = append(d.Events, event)
		return nil
	}
	store.FindEvents = func(_ context.Context, program, leadID string) ([]models.Event, error) {
		defer lock()()
		result := []models.Event{}
		for _, event := range d.Events {
			if live(program, event.Program, event.Deleted) && event.LeadID == leadID {
				result = append(result, event)
			}
		}
		sort.SliceStable(result, func(i, j int) bool { return result[i].At < result[j].At })
		return result, nil
	}
	store.InsertNotification = func(_ context.Context, notification models.Notification) error {
		defer lock()()
		d.Notifications = append(d.Notifications, notification)
		return nil
	}
	store.FindNotifications = func(_ context.Context, program, email string, unreadOnly bool, limit int) ([]models.Notification, int, error) {
		defer lock()()
		result, unread := []models.Notification{}, 0
		for index := len(d.Notifications) - 1; index >= 0; index-- {
			notification := d.Notifications[index]
			if !live(program, notification.Program, notification.Deleted) || !strings.EqualFold(notification.RecipientEmail, email) {
				continue
			}
			if !notification.Read {
				unread++
			}
			if (!unreadOnly || !notification.Read) && (limit <= 0 || len(result) < limit) {
				result = append(result, notification)
			}
		}
		return result, unread, nil
	}
	store.MarkNotificationRead = func(_ context.Context, program, email, notificationID string, at int64) error {
		defer lock()()
		_, index := find(d.Notifications, func(n models.Notification) bool {
			return live(program, n.Program, n.Deleted) && n.ID == notificationID && strings.EqualFold(n.RecipientEmail, email)
		})
		if index < 0 {
			return store.ErrNotFound
		}
		d.Notifications[index].Read, d.Notifications[index].ReadAt = true, at
		return nil
	}
	store.MarkAllNotificationsRead = func(_ context.Context, program, email string, at int64) error {
		defer lock()()
		for index, notification := range d.Notifications {
			if live(program, notification.Program, notification.Deleted) && strings.EqualFold(notification.RecipientEmail, email) && !notification.Read {
				d.Notifications[index].Read, d.Notifications[index].ReadAt = true, at
			}
		}
		return nil
	}
	store.NextSequence = func(_ context.Context, program, name string, _ int64) (int64, error) {
		defer lock()()
		d.Counters[program+"/"+name]++
		return d.Counters[program+"/"+name], nil
	}

	// Mail log and CC extracts
	store.InsertAlert = func(_ context.Context, alert models.Alert) error {
		defer lock()()
		d.Alerts = append(d.Alerts, alert)
		return nil
	}
	store.FindAlert = func(_ context.Context, program, alertID string) (models.Alert, error) {
		defer lock()()
		alert, index := find(d.Alerts, func(a models.Alert) bool { return live(program, a.Program, a.Deleted) && a.ID == alertID })
		return alert, notFoundIf(index)
	}
	store.FindAlerts = func(_ context.Context, program string, leadIDs []string, kind string, since int64) ([]models.Alert, error) {
		defer lock()()
		result := []models.Alert{}
		for index := len(d.Alerts) - 1; index >= 0; index-- {
			alert := d.Alerts[index]
			if !live(program, alert.Program, alert.Deleted) || (kind != "" && alert.Kind != kind) || (since > 0 && alert.SentAt < since) {
				continue
			}
			if leadIDs != nil && !contains(leadIDs, alert.LeadID) {
				continue
			}
			result = append(result, alert)
		}
		return result, nil
	}
	store.SetAlertDelivery = func(_ context.Context, program, alertID, delivery string) error {
		defer lock()()
		_, index := find(d.Alerts, func(a models.Alert) bool { return live(program, a.Program, a.Deleted) && a.ID == alertID })
		if index < 0 {
			return store.ErrNotFound
		}
		d.Alerts[index].Delivery = delivery
		return nil
	}
	store.FindCcExtract = func(_ context.Context, program, leadID string) (models.CcExtract, error) {
		defer lock()()
		extract, index := find(d.Extracts, func(e models.CcExtract) bool { return live(program, e.Program, e.Deleted) && e.LeadID == leadID })
		return extract, notFoundIf(index)
	}
	store.SaveCcExtract = func(_ context.Context, extract models.CcExtract) error {
		defer lock()()
		_, index := find(d.Extracts, func(e models.CcExtract) bool {
			return live(extract.Program, e.Program, e.Deleted) && e.LeadID == extract.LeadID
		})
		if index < 0 {
			d.Extracts = append(d.Extracts, extract)
		} else {
			d.Extracts[index] = extract
		}
		return nil
	}
}

func notFoundIf(index int) error {
	if index < 0 {
		return store.ErrNotFound
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// snapshot remembers the current store functions and returns a func that puts them back.
func snapshot() func() {
	findMemberByHash, findMemberByEmail, findMember, findMembers := store.FindMemberByHash, store.FindMemberByEmail, store.FindMember, store.FindMembers
	insertMember, replaceMember := store.InsertMember, store.ReplaceMember
	findLeads, findLead, findLeadByZenID, insertLead := store.FindLeads, store.FindLead, store.FindLeadByZenID, store.InsertLead
	updateLeadZoho, updateLeadWorkflow, setLeadMailMarks := store.UpdateLeadZoho, store.UpdateLeadWorkflow, store.SetLeadMailMarks
	findRechecks, findRecheck, findRecheckByNo, insertRecheck, replaceRecheck := store.FindRechecks, store.FindRecheck, store.FindRecheckByNo, store.InsertRecheck, store.ReplaceRecheck
	insertAudit, findAudits, insertEvent, findEvents := store.InsertAudit, store.FindAudits, store.InsertEvent, store.FindEvents
	insertNotification, findNotifications := store.InsertNotification, store.FindNotifications
	markNotificationRead, markAllNotificationsRead, nextSequence := store.MarkNotificationRead, store.MarkAllNotificationsRead, store.NextSequence
	insertAlert, findAlert, findAlerts, setAlertDelivery := store.InsertAlert, store.FindAlert, store.FindAlerts, store.SetAlertDelivery
	findCcExtract, saveCcExtract := store.FindCcExtract, store.SaveCcExtract
	return func() {
		store.FindMemberByHash, store.FindMemberByEmail, store.FindMember, store.FindMembers = findMemberByHash, findMemberByEmail, findMember, findMembers
		store.InsertMember, store.ReplaceMember = insertMember, replaceMember
		store.FindLeads, store.FindLead, store.FindLeadByZenID, store.InsertLead = findLeads, findLead, findLeadByZenID, insertLead
		store.UpdateLeadZoho, store.UpdateLeadWorkflow, store.SetLeadMailMarks = updateLeadZoho, updateLeadWorkflow, setLeadMailMarks
		store.FindRechecks, store.FindRecheck, store.FindRecheckByNo, store.InsertRecheck, store.ReplaceRecheck = findRechecks, findRecheck, findRecheckByNo, insertRecheck, replaceRecheck
		store.InsertAudit, store.FindAudits, store.InsertEvent, store.FindEvents = insertAudit, findAudits, insertEvent, findEvents
		store.InsertNotification, store.FindNotifications = insertNotification, findNotifications
		store.MarkNotificationRead, store.MarkAllNotificationsRead, store.NextSequence = markNotificationRead, markAllNotificationsRead, nextSequence
		store.InsertAlert, store.FindAlert, store.FindAlerts, store.SetAlertDelivery = insertAlert, findAlert, findAlerts, setAlertDelivery
		store.FindCcExtract, store.SaveCcExtract = findCcExtract, saveCcExtract
	}
}
