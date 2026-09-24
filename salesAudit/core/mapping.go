package core

import (
	"regexp"
	"sort"
	"strings"

	"auditApp/salesAudit/models"
)

// LeadState is the auditor-side state stored for one lead in the feature's own collections.
type LeadState struct {
	Escalation *models.Mail
	CcResponse *models.CcResponse
	Audit      *models.Audit
}

// BuildLead maps a LeadData record, its payments and EMI record to the frontend's lead.
func BuildLead(zoho models.ZohoLead, payments []models.ZohoPayment, emi *models.ZohoEmi, state LeadState) models.Lead {
	sapEnteredAt, _ := ParseZohoTime(zoho.AddedTime)
	lead := models.Lead{
		ID:                     zoho.ID,
		StudentFullName:        strings.TrimSpace(zoho.StudentFullName),
		Email:                  strings.TrimSpace(zoho.Email),
		PrimaryPhone:           strings.TrimSpace(zoho.PrimaryPhone),
		Course:                 zoho.Course,
		CourseValue:            zoho.CourseValue,
		DiscountGiven:          zoho.DiscountGiven,
		PaymentType:            zoho.PaymentType,
		PartialSplitUpCategory: zoho.PartialSplitUpCategory,
		TotalPaid:              zoho.TotalPaid,
		BalanceAmount:          zoho.BalanceAmount,
		SaleOwner:              strings.TrimSpace(zoho.SaleOwner),
		SaleOwnerManager:       strings.TrimSpace(zoho.SaleOwnerManager),
		EmiStatus:              zoho.EMIStatus,
		FinancialDetailsTypes:  financialDetailsTypes(payments),
		ConfirmationCallLink:   strings.TrimSpace(zoho.ConfirmationCallLink),
		SapEnteredAt:           sapEnteredAt,
		CcUploadedAt:           OptionalTime(zoho.ConfirmationCallAddedDateTime),
		Credits:                LatestCredits(payments),
		Escalation:             state.Escalation,
		CcResponse:             state.CcResponse,
		Audit:                  state.Audit,
	}
	if emi != nil {
		lead.EmiDetails = &models.EmiDetails{
			LoanAmount: Rupees(emi.LoanAmount),
			MonthlyEmi: Rupees(emi.FirstEMI),
			Roi:        percent(emi.ROI),
		}
	}
	return lead
}

func SummarizeLead(lead models.Lead) models.LeadSummary {
	return models.LeadSummary{
		ID:                   lead.ID,
		StudentFullName:      lead.StudentFullName,
		Course:               lead.Course,
		SaleOwner:            lead.SaleOwner,
		SaleOwnerManager:     lead.SaleOwnerManager,
		ConfirmationCallLink: lead.ConfirmationCallLink,
		CcResponse:           lead.CcResponse,
	}
}

// SortPayments orders payments oldest first by when they were added in Zoho.
func SortPayments(payments []models.ZohoPayment) {
	sort.SliceStable(payments, func(i, j int) bool {
		a, _ := ParseZohoTime(payments[i].AddedTime)
		b, _ := ParseZohoTime(payments[j].AddedTime)
		return a < b
	})
}

// LatestCredits picks the most recently added record of each credit type.
func LatestCredits(payments []models.ZohoPayment) models.Credits {
	sorted := append([]models.ZohoPayment(nil), payments...)
	SortPayments(sorted)
	latest := map[string]*models.Credit{}
	for _, payment := range sorted {
		latest[payment.Type] = &models.Credit{
			Amount:      payment.Amount,
			Verified:    payment.Verified,
			PaymentDate: DisplayDate(payment.PaymentDate),
		}
	}
	return models.Credits{
		BookingAmount:    latest[models.CreditBookingAmount],
		Part1:            latest[models.CreditPart1],
		RemainingBalance: latest[models.CreditRemainingBalance],
	}
}

func financialDetailsTypes(payments []models.ZohoPayment) []string {
	types := []string{}
	seen := map[string]bool{}
	for _, payment := range payments {
		if payment.Type != "" && !seen[payment.Type] {
			seen[payment.Type] = true
			types = append(types, payment.Type)
		}
	}
	return types
}

func ToPayment(payment models.ZohoPayment, leadID string) models.Payment {
	return models.Payment{
		ID:            payment.ID,
		LeadID:        leadID,
		PaymentType:   payment.PaymentType,
		ModeOfPayment: payment.ModeOfPayment,
		Amount:        payment.Amount,
		PaymentDate:   DisplayDate(payment.PaymentDate),
		Verified:      payment.Verified,
		Course:        payment.EnrolmentCourse,
		CourseValue:   payment.EnrolmentCourseValue,
		Type:          payment.Type,
		AddedAt:       OptionalTime(payment.AddedTime),
		VerifiedAt:    OptionalTime(payment.VerifiedOn),
	}
}

// Scheduled is one planned installment, from PartialReminders or SubscriptionReminders.
type Scheduled struct {
	Amount  string
	DueDate string
}

var emiTenure = regexp.MustCompile(`EMI - (\d+) Month`)

// ZohoSource is the Zoho side of the audit comparison.
func ZohoSource(lead models.Lead, zoho models.ZohoLead, schedule []Scheduled) models.Source {
	payment := &models.PaymentInfo{TotalFee: Rupees(lead.CourseValue)}
	if booking := lead.Credits.BookingAmount; booking != nil {
		payment.DownPayment = Rupees(booking.Amount)
	}
	payment.Installments = installments(lead, schedule)
	if IsEmiPaymentType(lead.PaymentType) {
		payment.Emi = &models.EmiTerms{DisbursalStatus: lead.EmiStatus}
		if match := emiTenure.FindStringSubmatch(lead.PaymentType); match != nil {
			payment.Emi.Tenure = match[1] + " months"
		}
	}
	return models.Source{
		Personal: &models.Personal{
			LearnerName:   lead.StudentFullName,
			Email:         lead.Email,
			ContactNumber: lead.PrimaryPhone,
		},
		CourseDetails: &models.CourseDetails{
			CourseName: lead.Course,
			StartDate:  DisplayDate(zoho.AssignedBatchStartDate),
			Mode:       zoho.ModeOfStudy,
			Medium:     zoho.PreferredLanguage,
		},
		Payment: payment,
	}
}

// installments lines the plan up with the split category ("40-30-30"). The reminders only cover
// the parts still due, so they fill the last slots; the first slot is the initial payment
// (Credit_Part1) when the schedule is one short of the split.
func installments(lead models.Lead, schedule []Scheduled) []models.Installment {
	var splits []string
	for _, split := range strings.Split(lead.PartialSplitUpCategory, "-") {
		if split = strings.TrimSpace(split); split != "" {
			splits = append(splits, split+"%")
		}
	}
	count := max(len(splits), len(schedule))
	if count == 0 {
		return nil
	}
	result := make([]models.Installment, count)
	for index := range result {
		if index < len(splits) {
			result[index].Percentage = splits[index]
		}
	}
	offset := count - len(schedule)
	for index, item := range schedule {
		result[offset+index].DueDate = DisplayDate(item.DueDate)
		result[offset+index].Amount = Rupees(item.Amount)
	}
	if offset > 0 && lead.Credits.Part1 != nil {
		result[0].DueDate = lead.Credits.Part1.PaymentDate
		result[0].Amount = Rupees(lead.Credits.Part1.Amount)
	}
	return result
}

// VendorSource is the EMI vendor side: the loan application recorded in EmiData.
func VendorSource(emi *models.ZohoEmi) *models.Source {
	if emi == nil {
		return nil
	}
	terms := &models.EmiTerms{
		LoanAmount:      Rupees(emi.LoanAmount),
		MonthlyEmi:      Rupees(emi.FirstEMI),
		Roi:             percent(emi.ROI),
		DisbursalStatus: emi.EMIStatus,
	}
	if tenure := strings.TrimSpace(emi.TenorInMonths); tenure != "" {
		terms.Tenure = tenure + " months"
	}
	return &models.Source{Payment: &models.PaymentInfo{Emi: terms}}
}

func ToDiscount(discount *models.ZohoDiscount) *models.Discount {
	if discount == nil {
		return nil
	}
	return &models.Discount{
		Status:             discount.Status,
		ActualCourseFee:    discount.ActualCourseFee,
		RequestedCourseFee: discount.RequestedCourseFee,
		DiscountValue:      discount.DiscountValue,
		ApprovedAt:         OptionalTime(discount.ApprovedDateTime),
	}
}

// percent writes "5.00" as "5%".
func percent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimSuffix(value, "%")
	if whole, fraction, found := strings.Cut(value, "."); found {
		if fraction = strings.TrimRight(fraction, "0"); fraction == "" {
			value = whole
		} else {
			value = whole + "." + fraction
		}
	}
	return value + "%"
}
