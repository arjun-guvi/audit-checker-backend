package core

import (
	"strings"

	"auditApp/salesAudit/models"
)

// Comparison sections, in display order.
const (
	SectionPersonal = "personal"
	SectionCourse   = "course"
	SectionPayment  = "payment"
)

// DbFields are the values from our database the CC must agree with, keyed as CcField keys.
func DbFields(lead models.Lead) []models.ComparisonRow {
	rows := []models.ComparisonRow{
		{Section: SectionPersonal, Key: "name", Label: "Learner name", DbValue: lead.Personal.Name},
		{Section: SectionPersonal, Key: "email", Label: "Email", DbValue: lead.Personal.Email},
		{Section: SectionPersonal, Key: "phone", Label: "Phone", DbValue: lead.Personal.Phone},
		{Section: SectionCourse, Key: "product", Label: "Course", DbValue: strings.ReplaceAll(lead.Course.Product, "_", " ")},
		{Section: SectionCourse, Key: "batchName", Label: "Batch", DbValue: lead.Course.Batch.Name},
		{Section: SectionCourse, Key: "batchStartDate", Label: "Batch start date", DbValue: DisplayDate(lead.Course.Batch.StartDate)},
		{Section: SectionCourse, Key: "modeOfStudy", Label: "Mode of study", DbValue: lead.Course.ModeOfStudy},
		{Section: SectionCourse, Key: "language", Label: "Language", DbValue: FirstNonEmpty(lead.Course.Batch.Language, lead.Personal.PreferredLanguage)},
		{Section: SectionPayment, Key: "paymentType", Label: "Payment type", DbValue: lead.Payment.PaymentType},
		{Section: SectionPayment, Key: "courseFee", Label: "Course fee", DbValue: rupeesOrBlank(lead.Payment.CourseFee)},
	}
	if discount := LatestDiscount(lead); discount != nil {
		rows = append(rows, models.ComparisonRow{Section: SectionPayment, Key: "discount", Label: "Discount", DbValue: rupeesOrBlank(discount.DiscountValue)})
	}
	if booking := latestRecord(lead, models.CreditBookingAmount); booking != nil {
		rows = append(rows, models.ComparisonRow{Section: SectionPayment, Key: "downPayment", Label: "Down payment", DbValue: rupeesOrBlank(booking.Amount)})
	}
	switch PaymentMode(lead.Payment.PaymentType) {
	case "partial", "emiPartial":
		rows = append(rows, models.ComparisonRow{Section: SectionPayment, Key: "partialCategory", Label: "Partial split", DbValue: lead.Payment.PartialCategory})
	}
	if emi := LatestEmi(lead); emi != nil {
		rows = append(rows,
			models.ComparisonRow{Section: SectionPayment, Key: "emiVendor", Label: "EMI vendor", DbValue: emi.Vendor},
			models.ComparisonRow{Section: SectionPayment, Key: "loanAmount", Label: "Loan amount", DbValue: rupeesOrBlank(emi.LoanAmount)},
			models.ComparisonRow{Section: SectionPayment, Key: "emiTenure", Label: "EMI tenure (months)", DbValue: emi.TenorMonths},
		)
	}
	return rows
}

// Compare lines the CC's values up against the database. A field the CC does not mention is a
// mismatch, so the auditor looks at it.
func Compare(lead models.Lead, extract models.CcExtract) ([]models.ComparisonRow, int) {
	cc := map[string]string{}
	for _, field := range extract.Fields {
		cc[field.Key] = field.Value
	}
	rows := DbFields(lead)
	mismatches := 0
	for index := range rows {
		rows[index].CcValue = cc[rows[index].Key]
		rows[index].Match = rows[index].CcValue != "" && sameValue(rows[index].DbValue, rows[index].CcValue)
		if !rows[index].Match {
			mismatches++
		}
	}
	return rows, mismatches
}

func sameValue(a, b string) bool {
	return normalizeValue(a) == normalizeValue(b)
}

func normalizeValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "+91")
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", "₹", "", ",", "", "rs.", "")
	value = replacer.Replace(value)
	return strings.TrimSuffix(value, ".00")
}

func rupeesOrBlank(amount float64) string {
	if amount == 0 {
		return ""
	}
	return Rupees(amount)
}

// LatestDiscount is the lead's last discount request, nil when none.
func LatestDiscount(lead models.Lead) *models.Discount {
	if len(lead.Payment.Discounts) == 0 {
		return nil
	}
	return &lead.Payment.Discounts[len(lead.Payment.Discounts)-1]
}

// LatestEmi is the lead's last EMI application, nil when none.
func LatestEmi(lead models.Lead) *models.Emi {
	if len(lead.Payment.Emis) == 0 {
		return nil
	}
	return &lead.Payment.Emis[len(lead.Payment.Emis)-1]
}

func latestRecord(lead models.Lead, recordType string) *models.PaymentRecord {
	var latest *models.PaymentRecord
	for index := range lead.Payment.Records {
		if lead.Payment.Records[index].Type == recordType {
			latest = &lead.Payment.Records[index]
		}
	}
	return latest
}
