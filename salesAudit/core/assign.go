package core

import (
	"math/rand"
	"sort"
	"strings"

	"auditApp/salesAudit/models"
)

// AssignLeads spreads unassigned leads over the available auditors of each lead's region. Each
// lead goes to the auditor with the fewest open leads (load, plus what this run already gave
// them); ties are broken at random, so 80 South leads over two idle auditors split 40/40.
// Leads whose region has no available auditor are left out. Returns lead id → auditor email.
func AssignLeads(leads []models.Lead, auditors []models.Member, load map[string]int, rnd *rand.Rand) map[string]string {
	byRegion := map[string][]string{}
	for _, auditor := range auditors {
		if auditor.Role == models.RoleAuditor && auditor.Available && !auditor.Deleted && auditor.Region != "" {
			byRegion[auditor.Region] = append(byRegion[auditor.Region], strings.ToLower(auditor.Email))
		}
	}
	counts := map[string]int{}
	for email, count := range load {
		counts[strings.ToLower(email)] = count
	}

	ordered := append([]models.Lead(nil), leads...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].CrmCreatedAt < ordered[j].CrmCreatedAt })

	result := map[string]string{}
	for _, lead := range ordered {
		candidates := byRegion[lead.Region]
		if len(candidates) == 0 {
			continue
		}
		lowest := -1
		var best []string
		for _, email := range candidates {
			switch count := counts[email]; {
			case lowest == -1 || count < lowest:
				lowest, best = count, []string{email}
			case count == lowest:
				best = append(best, email)
			}
		}
		chosen := best[rnd.Intn(len(best))]
		result[lead.ID] = chosen
		counts[chosen]++
	}
	return result
}

// OpenLoad counts each auditor's leads that are not completed.
func OpenLoad(leads []models.Lead) map[string]int {
	load := map[string]int{}
	for _, lead := range leads {
		if lead.Assignment != nil && lead.Audit.Status != models.AuditCompleted {
			load[strings.ToLower(lead.Assignment.AuditorEmail)]++
		}
	}
	return load
}
