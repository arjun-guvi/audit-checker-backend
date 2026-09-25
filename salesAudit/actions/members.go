package actions

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// DevTokenPrefix marks the dev shell's tokens: "dev-mock-token:<email>" signs in as that member.
// Zen's real tokens are user hashes, matched on the member's userHash.
const DevTokenPrefix = "dev-mock-token:"

// ResolveMember finds the member a token belongs to; 401 when there is none.
func ResolveMember(ctx context.Context, program, token string) (models.Member, error) {
	member, err := store.FindMemberByHash(ctx, program, token)
	if store.IsNotFound(err) {
		if email, isDev := strings.CutPrefix(token, DevTokenPrefix); isDev {
			member, err = store.FindMemberByEmail(ctx, program, strings.TrimSpace(email))
		}
	}
	if store.IsNotFound(err) {
		return member, Error{Status: http.StatusUnauthorized, Message: "You are not a Sales Audit member. Ask the auditor TL to add you."}
	}
	return member, err
}

// TeamOf is who reports to the member: a BDM's BDAs, an auditor TL's auditors.
func TeamOf(ctx context.Context, member models.Member) ([]string, error) {
	if member.Role != models.RoleBdm && member.Role != models.RoleAuditorTl {
		return nil, nil
	}
	members, err := store.FindMembers(ctx, member.Program, "")
	if err != nil {
		return nil, err
	}
	return core.TeamEmails(member.Email, members), nil
}

// CurrentUser is GET /me.
func CurrentUser(ctx context.Context, member models.Member) (models.CurrentUser, error) {
	team, err := TeamOf(ctx, member)
	if team == nil {
		team = []string{}
	}
	return models.CurrentUser{Member: member, Permissions: core.Permissions(member.Role), TeamEmails: team}, err
}

// ListMembers returns the roster, optionally one role.
func ListMembers(ctx context.Context, program, role string) ([]models.Member, error) {
	return store.FindMembers(ctx, program, role)
}

// MemberInput is the body of POST/PUT /members.
type MemberInput struct {
	UserHash     string `json:"userHash"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Region       string `json:"region"`
	ManagerEmail string `json:"managerEmail"`
	Available    *bool  `json:"available"`
}

func validateMember(input MemberInput) error {
	if !strings.Contains(input.Email, "@") || strings.TrimSpace(input.Name) == "" {
		return badRequest(errors.New("name and a valid email are required"))
	}
	valid := false
	for _, role := range models.Roles {
		valid = valid || role == input.Role
	}
	if !valid {
		return badRequest(errors.New("role must be auditorTl, auditor, bdm or bda"))
	}
	if input.Role == models.RoleAuditor && input.Region != models.RegionNorth && input.Region != models.RegionSouth {
		return badRequest(errors.New("an auditor needs a region: North or South"))
	}
	return nil
}

// CreateMember adds a member (auditor TL only).
func CreateMember(ctx context.Context, by models.Member, input MemberInput) (models.Member, error) {
	if !core.Can(by.Role, core.ActionManageMembers) {
		return models.Member{}, forbidden("Only the auditor TL can manage members")
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if err := validateMember(input); err != nil {
		return models.Member{}, err
	}
	if _, err := store.FindMemberByEmail(ctx, by.Program, input.Email); err == nil {
		return models.Member{}, badRequest(errors.New("A member with this email already exists"))
	} else if !store.IsNotFound(err) {
		return models.Member{}, err
	}
	member := models.Member{
		ID: core.NewID(), Program: by.Program, UserHash: strings.TrimSpace(input.UserHash), Email: input.Email,
		Name: strings.TrimSpace(input.Name), Role: input.Role, Region: input.Region,
		ManagerEmail: strings.ToLower(strings.TrimSpace(input.ManagerEmail)), Available: input.Available == nil || *input.Available,
		Created: models.Created{At: nowSeconds(), By: by.UserHash},
	}
	return member, store.InsertMember(ctx, member)
}

// UpdateMember edits a member (auditor TL only). An auditor may flip their own availability.
func UpdateMember(ctx context.Context, by models.Member, memberID string, input MemberInput) (models.Member, error) {
	member, err := store.FindMember(ctx, by.Program, memberID)
	if err != nil {
		return member, orNotFound(err, "Member")
	}
	selfAvailability := member.ID == by.ID && input.Available != nil && input.Role == "" && input.Email == ""
	if !core.Can(by.Role, core.ActionManageMembers) && !selfAvailability {
		return member, forbidden("Only the auditor TL can manage members")
	}
	if input.Available != nil {
		member.Available = *input.Available
	}
	if !selfAvailability {
		input.Email = strings.ToLower(strings.TrimSpace(core.FirstNonEmpty(input.Email, member.Email)))
		input.Name = core.FirstNonEmpty(input.Name, member.Name)
		input.Role = core.FirstNonEmpty(input.Role, member.Role)
		if input.Region == "" {
			input.Region = member.Region
		}
		if err := validateMember(input); err != nil {
			return member, err
		}
		member.Email, member.Name, member.Role, member.Region = input.Email, strings.TrimSpace(input.Name), input.Role, input.Region
		member.ManagerEmail = strings.ToLower(strings.TrimSpace(input.ManagerEmail))
		if input.UserHash != "" {
			member.UserHash = strings.TrimSpace(input.UserHash)
		}
	}
	return member, store.ReplaceMember(ctx, member)
}

// MemberSync is what SyncSalesMembers did.
type MemberSync struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
}

// SyncSalesMembers adds the BDAs and BDMs Zoho names on the learners (saleOwner and
// saleOwnerManager) and points each BDA's managerEmail at their BDM, which is what a BDM's team
// and leads are worked out from. Existing members of other roles are left alone. Members added
// here have no userHash, so they can't sign in until the TL links them to Zen.
func SyncSalesMembers(ctx context.Context, program string, learners []models.ZohoLearner) (MemberSync, error) {
	result := MemberSync{}
	for _, pair := range core.SalesPairsFromZoho(learners) {
		if pair.BdmEmail != "" {
			if _, err := ensureSalesMember(ctx, program, pair.BdmEmail, models.RoleBdm, "", &result); err != nil {
				return result, err
			}
		}
		if pair.BdaEmail == "" {
			continue
		}
		bda, err := ensureSalesMember(ctx, program, pair.BdaEmail, models.RoleBda, pair.BdmEmail, &result)
		if err != nil {
			return result, err
		}
		if bda.Role == models.RoleBda && pair.BdmEmail != "" && !strings.EqualFold(bda.ManagerEmail, pair.BdmEmail) {
			bda.ManagerEmail = pair.BdmEmail
			if err := store.ReplaceMember(ctx, bda); err != nil {
				return result, err
			}
			result.Updated++
		}
	}
	return result, nil
}

// ensureSalesMember returns the member with this email, adding it with the role when missing.
func ensureSalesMember(ctx context.Context, program, email, role, managerEmail string, result *MemberSync) (models.Member, error) {
	member, err := store.FindMemberByEmail(ctx, program, email)
	if !store.IsNotFound(err) {
		return member, err
	}
	member = models.Member{
		ID: core.NewID(), Program: program, Email: email, Name: email, Role: role, ManagerEmail: managerEmail,
		Available: true, Created: models.Created{At: nowSeconds(), By: models.SystemUser},
	}
	if err := store.InsertMember(ctx, member); err != nil {
		return member, err
	}
	result.Created++
	return member, nil
}
