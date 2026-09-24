package worker

import (
	"context"
	"strings"

	"auditApp/models"
)

// DevTokenPrefix marks the dev shell's tokens: "dev-mock-token:<email>" names the user. Zen's real
// tokens are opaque and resolved by its auth middleware.
const DevTokenPrefix = "dev-mock-token:"

// ResolveUser stands in for Zen's user lookup. A dev token's email is a BDM when it manages a lead
// in Audit, a BDA when it owns one, and an auditor otherwise; any other token is an auditor.
func ResolveUser(ctx context.Context, token string) (models.CurrentUser, error) {
	user := models.CurrentUser{Hash: token, Name: models.AuditTeamName, Role: models.RoleAuditor}
	email, isDevToken := strings.CutPrefix(token, DevTokenPrefix)
	if !isDevToken {
		return user, nil
	}
	user.Email = strings.ToLower(strings.TrimSpace(email))
	user.Name = user.Email

	leads, err := FindAuditLeads(ctx)
	if err != nil {
		return user, err
	}
	for _, lead := range leads {
		if strings.EqualFold(ContactEmail(lead.SaleOwnerManager), user.Email) {
			user.Role, user.Name = models.RoleBdm, ContactName(lead.SaleOwnerManager)
			return user, nil
		}
	}
	for _, lead := range leads {
		if strings.EqualFold(ContactEmail(lead.SaleOwner), user.Email) {
			user.Role, user.Name = models.RoleBda, ContactName(lead.SaleOwner)
			return user, nil
		}
	}
	return user, nil
}
