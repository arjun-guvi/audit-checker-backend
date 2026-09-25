package core

import (
	"strconv"
	"strings"

	"auditApp/salesAudit/models"
)

// Mapping of Zoho learner records onto salesAuditLeads and salesAuditRechecks.

func str(value models.ZohoValue) string { return strings.TrimSpace(string(value)) }

func num(value models.ZohoValue) float64 { return Amount(string(value)) }

// RegionOf maps Zoho's salesTeam to a region; "" when it is neither North nor South.
func RegionOf(salesTeam string) string {
	switch strings.ToLower(strings.TrimSpace(salesTeam)) {
	case "north":
		return models.RegionNorth
	case "south":
		return models.RegionSouth
	}
	return ""
}

// CcTypeOf tells a PDF (a Drive file) from a call recording (an audio file or a Superleap call
// link); other links (Drive folders, Gmail threads) are opened as they are.
func CcTypeOf(link string) string {
	lower := strings.ToLower(strings.TrimSpace(link))
	switch {
	case lower == "":
		return ""
	case strings.Contains(lower, "superleap") || strings.Contains(lower, "recording") ||
		hasAnySuffix(strings.SplitN(lower, "?", 2)[0], ".mp3", ".wav", ".m4a", ".ogg", ".aac"):
		return models.CcTypeRecording
	case strings.Contains(lower, "drive.google.com/file/d/") || strings.HasSuffix(strings.SplitN(lower, "?", 2)[0], ".pdf"):
		return models.CcTypePdf
	default:
		return models.CcTypeLink
	}
}

func hasAnySuffix(value string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

// DrivePreviewURL turns a Drive file link into its embeddable preview; "" for other links.
func DrivePreviewURL(link string) string {
	_, rest, found := strings.Cut(link, "drive.google.com/file/d/")
	if !found {
		return ""
	}
	id := strings.SplitN(strings.SplitN(rest, "/", 2)[0], "?", 2)[0]
	if id == "" {
		return ""
	}
	return "https://drive.google.com/file/d/" + id + "/preview"
}

// LeadFromZoho maps the Zoho-owned fields of a learner. Workflow fields (assignment, audit,
// recheckSummary) are left empty; see NewLeadFromZoho and MergeZoho.
func LeadFromZoho(learner models.ZohoLearner) models.Lead {
	admission := map[string]string{}
	for key, value := range learner.AdmissionDetails {
		admission[key] = str(value)
	}
	link := str(learner.ConfirmationCall)
	lead := models.Lead{
		ZenID:       str(learner.ZenID),
		SuperleapID: str(learner.SuperleapID),
		Region:      RegionOf(str(learner.SalesTeam)),
		SalesFrom:   str(learner.SalesFrom),
		Stage:       str(learner.Stage),
		ZohoStatus:  str(learner.Status),
		Personal: models.LeadPersonal{
			Name:              strings.Join(strings.Fields(str(learner.Name)), " "),
			Email:             strings.ToLower(str(learner.Email)),
			Phone:             str(learner.Phone),
			PreferredLanguage: str(learner.PreferredLanguage),
		},
		Course: models.LeadCourse{
			Product:     str(learner.Product),
			ModeOfStudy: str(learner.ModeOfStudy),
			Batch: models.Batch{
				Name:      str(learner.BatchData.BatchName),
				Type:      str(learner.BatchData.BatchType),
				Language:  str(learner.BatchData.Language),
				StartDate: str(learner.BatchData.StartDate),
				EndDate:   str(learner.BatchData.EndDate),
				StartTime: str(learner.BatchData.StartTime),
				Status:    str(learner.BatchData.Status),
			},
			EnrolledOn:   str(learner.DateOfEnrollment),
			OnboardingAt: ZohoSeconds(str(learner.OnboardingDateTime)),
		},
		Payment:       paymentFromZoho(learner),
		Admission:     admission,
		TermsAccepted: strings.EqualFold(str(learner.TermsAndConditions), "Yes"),
		Marketing: models.LeadMarketing{
			Source:      str(learner.Source),
			Medium:      str(learner.Medium),
			Campaign:    str(learner.Campaign),
			Content:     str(learner.Content),
			AffiliateID: str(learner.AffiliateID),
		},
		BdaEmail:           NormalizeEmail(str(learner.SaleOwner)),
		BdmEmail:           NormalizeEmail(str(learner.SaleOwnerManager)),
		OnboardCoordinator: NormalizeEmail(str(learner.OnboardCoordinator)),
		Cc:                 models.LeadCc{Link: link, Type: CcTypeOf(link), Status: models.CcPending},
		CrmCreatedAt:       ZohoSeconds(str(learner.CrmLeadCreatedDate)),
		EnrolledAt:         ZohoSeconds(str(learner.DateOfEnrollment)),
	}
	if link != "" {
		lead.Cc.Status = models.CcUpdated
	}
	return lead
}

func paymentFromZoho(learner models.ZohoLearner) models.LeadPayment {
	payment := models.LeadPayment{
		PaymentType:      str(learner.PaymentType),
		PartialCategory:  str(learner.PartialCategory),
		CourseFee:        num(learner.CourseFee),
		TotalPaid:        num(learner.TotalPaid),
		BalanceAmount:    num(learner.BalanceAmount),
		PromoCode:        str(learner.PromoCode),
		PayInSameMonth:   str(learner.WillPayInSameMonth),
		ZbCustomerID:     str(learner.ZbCustomerID),
		ZbInvoiceID:      str(learner.ZbInvoiceID),
		Records:          []models.PaymentRecord{},
		Emis:             []models.Emi{},
		Discounts:        []models.Discount{},
		PartialReminders: []models.PartialReminder{},
		Subscriptions:    []models.Subscription{},
	}
	for _, record := range learner.FinancialDetails {
		payment.Records = append(payment.Records, models.PaymentRecord{
			RecordID:      str(record.RecordID),
			Type:          str(record.Type),
			Amount:        num(record.Amount),
			ModeOfPayment: str(record.ModeOfPayment),
			UtrPaymentID:  str(record.UtrPaymentID),
			Verified:      str(record.Verified),
			VerifiedDate:  str(record.VerifiedDate),
			PaymentDate:   str(record.PaymentDate),
			ReceiptMade:   str(record.ZbReceiptCreated),
		})
	}
	for _, emi := range learner.EmiDetails {
		payment.Emis = append(payment.Emis, models.Emi{
			RecordID:        str(emi.RecordID),
			ApplicationID:   str(emi.ApplicationID),
			Vendor:          str(emi.EmiVendor),
			Status:          str(emi.EmiStatus),
			Stage:           str(emi.Stage),
			LoanAmount:      num(emi.LoanAmount),
			DisbursalAmount: num(emi.DisbursalAmount),
			FirstEmiAmount:  num(emi.FirstEmiAmount),
			TenorMonths:     str(emi.TenorInMonth),
			RoiPercent:      str(emi.RoiInPercentage),
			ApplicationDate: str(emi.ApplicationDate),
			InTheNameOf:     str(emi.ApplicationInTheNameOf),
		})
	}
	for _, discount := range learner.CourseDiscountDetails {
		payment.Discounts = append(payment.Discounts, models.Discount{
			RequestedCourseFee: num(discount.RequestedCourseFee),
			ActualCourseFee:    num(discount.ActualCourseFee),
			DiscountValue:      num(discount.DiscountValue),
			RequestedBy:        NormalizeEmail(str(discount.RequestedPerson)),
			PaymentType:        str(discount.PaymentType),
			Status:             str(discount.Status),
		})
	}
	for _, reminder := range learner.PartialReminders {
		payment.PartialReminders = append(payment.PartialReminders, models.PartialReminder{
			RecordID:    str(reminder.RecordID),
			NoOfPartial: str(reminder.NoOfPartial),
			Amount:      num(reminder.Amount),
			DueDate:     str(reminder.DueDate),
			Status:      str(reminder.PartialPaymentStatus),
			LinkStatus:  str(reminder.LinkStatus),
			PaidAt:      str(reminder.PaidDateTime),
		})
	}
	for _, subscription := range learner.SubscriptionReminders {
		payment.Subscriptions = append(payment.Subscriptions, models.Subscription{
			RecordID:         str(subscription.RecordID),
			NoOfSubscription: str(subscription.NoOfSubscription),
			Amount:           num(subscription.Amount),
			DueDate:          str(subscription.DueDate),
			Status:           str(subscription.PaymentStatus),
		})
	}
	return WithPaymentReadiness(payment)
}

// AuditStatusFromZoho maps Zoho's auditStatus for a lead seen for the first time.
func AuditStatusFromZoho(auditStatus string, assigned bool) string {
	switch strings.ToLower(strings.TrimSpace(auditStatus)) {
	case "completed":
		return models.AuditCompleted
	case "recheck pending":
		return models.AuditRecheckOpen
	}
	if assigned {
		return models.AuditPending
	}
	return models.AuditUnassigned
}

// NewLeadFromZoho builds a lead seen for the first time, taking Zoho's audit coordinator and
// audit status as the starting workflow state.
func NewLeadFromZoho(program string, learner models.ZohoLearner, now int64) models.Lead {
	lead := LeadFromZoho(learner)
	lead.ID = NewID()
	lead.Program = program
	lead.ZohoSyncedAt = now
	lead.Created = models.Created{At: now, By: models.SystemUser}
	if lead.Cc.Status == models.CcUpdated {
		lead.Cc.UpdatedAt = now
	}
	if coordinator := NormalizeEmail(str(learner.AuditCoordinator)); coordinator != "" {
		lead.Assignment = &models.Assignment{
			AuditorEmail: coordinator,
			AssignedAt:   FirstNonZero(ZohoSeconds(str(learner.AuditAssignedDateTime)), now),
			AssignedBy:   models.SystemUser,
			Mode:         models.AssignZoho,
		}
	}
	lead.Audit.Status = AuditStatusFromZoho(str(learner.AuditStatus), lead.Assignment != nil)
	if lead.Audit.Status == models.AuditCompleted {
		lead.Audit.CompletedAt = FirstNonZero(ZohoSeconds(str(learner.AuditCompletedDateTime)), now)
		if lead.Assignment != nil {
			lead.Audit.CompletedBy = lead.Assignment.AuditorEmail
		}
		lead.Audit.LastAuditedAt = lead.Audit.CompletedAt
		lead.Audit.Attempt = 1
	}
	return lead
}

// MergeZoho refreshes an existing lead's Zoho fields and keeps the portal's workflow state.
// ccBecameUpdated reports a pending → updated CC change, for the "CC can be verified" alert.
func MergeZoho(existing models.Lead, learner models.ZohoLearner, now int64) (lead models.Lead, ccBecameUpdated bool) {
	lead = LeadFromZoho(learner)
	lead.ID = existing.ID
	lead.Program = existing.Program
	lead.Created = existing.Created
	lead.Assignment = existing.Assignment
	lead.Audit = existing.Audit
	lead.RecheckSummary = existing.RecheckSummary
	lead.PaymentVerificationMailedAt = existing.PaymentVerificationMailedAt
	lead.LastEscalationAt = existing.LastEscalationAt
	lead.ZohoSyncedAt = now
	switch {
	case lead.Cc.Status == models.CcUpdated && existing.Cc.Status != models.CcUpdated:
		lead.Cc.UpdatedAt = now
		ccBecameUpdated = true
	case lead.Cc.Status == models.CcUpdated && lead.Cc.Link != existing.Cc.Link:
		lead.Cc.UpdatedAt = now
	case lead.Cc.Status == models.CcUpdated:
		lead.Cc.UpdatedAt = existing.Cc.UpdatedAt
	}
	return lead, ccBecameUpdated
}

// CategoryFromZoho maps a recheckDetails pendingList entry to a recheck category.
func CategoryFromZoho(pending []models.ZohoValue) string {
	for _, item := range pending {
		switch strings.ToLower(str(item)) {
		case "confirmation call", "cc pending":
			return models.CategoryCcPending
		case "missed points on cc", "missed points in cc":
			return models.CategoryMissedPointsInCc
		case "payment":
			return models.CategoryPayment
		case "emi":
			return models.CategoryEmi
		case "approval":
			return models.CategoryApproval
		case "down payment":
			return models.CategoryDownPayment
		}
	}
	return models.CategoryPayment
}

// RecheckFromZoho maps one recheckDetails entry of a lead; ok is false without an SRID.
func RecheckFromZoho(program string, lead models.Lead, zoho models.ZohoRecheck, now int64) (models.Recheck, bool) {
	srid := str(zoho.SRID)
	if srid == "" {
		return models.Recheck{}, false
	}
	raisedAt := FirstNonZero(ZohoSeconds(str(zoho.RecheckDate)), now)
	attempt, _ := strconv.Atoi(str(zoho.RecheckAttempt))
	requestedBy := NormalizeEmail(str(zoho.RequestPerson))
	recheck := models.Recheck{
		ID:        NewID(),
		Program:   program,
		RecheckNo: srid,
		Source:    models.SourceZoho,
		LeadID:    lead.ID,
		LeadName:  lead.Personal.Name,
		ZenID:     lead.ZenID,
		Attempt:   attempt,
		Category:  CategoryFromZoho(zoho.PendingList),
		Comments:  str(zoho.AuditComments),
		Status:    models.RecheckOpen,
		RaisedBy:  models.Actor{Email: requestedBy, Name: requestedBy, Role: models.RoleAuditor},
		RaisedAt:  raisedAt,
		BdaEmail:  lead.BdaEmail,
		BdmEmail:  lead.BdmEmail,
		Created:   models.Created{At: now, By: models.SystemUser},
	}
	if lead.Assignment != nil {
		recheck.AuditorEmail = lead.Assignment.AuditorEmail
	}
	if strings.EqualFold(str(zoho.TicketStatus), "closed") {
		recheck.Status = models.RecheckClosed
		recheck.Closed = &models.RecheckClosure{
			At:   raisedAt,
			By:   models.Actor{Email: models.SystemUser, Name: "Zoho", Role: models.RoleSystem},
			Note: "Closed in Zoho before the portal took over",
		}
		// Zoho history: treat as already re-audited so it does not show as awaiting re-audit.
		recheck.ReauditedAt = raisedAt
	}
	return recheck, true
}

// FirstNonZero returns the first non-zero value.
func FirstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
