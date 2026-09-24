package models

// Sales Audit response shapes, matching what the frontend (src/salesAudit) reads.

// Credit is the latest financial record of one credit type; nil when unpaid.
type Credit struct {
	Amount      string `json:"amount"`
	Verified    string `json:"verified"`
	PaymentDate string `json:"paymentDate"`
}

type Credits struct {
	BookingAmount    *Credit `json:"bookingAmount"`
	Part1            *Credit `json:"part1"`
	RemainingBalance *Credit `json:"remainingBalance"`
}

type EmiDetails struct {
	LoanAmount string `json:"loanAmount"`
	MonthlyEmi string `json:"monthlyEmi"`
	Roi        string `json:"roi"`
	DueDate    string `json:"dueDate"`
}

// Lead is one enrolment with everything the Leads, BDA and Overview pages need.
type Lead struct {
	ID                     string      `json:"id"`
	StudentFullName        string      `json:"studentFullName"`
	Email                  string      `json:"email"`
	PrimaryPhone           string      `json:"primaryPhone"`
	Course                 string      `json:"course"`
	CourseValue            string      `json:"courseValue"`
	DiscountGiven          string      `json:"discountGiven"`
	PaymentType            string      `json:"paymentType"`
	PartialSplitUpCategory string      `json:"partialSplitUpCategory"`
	TotalPaid              string      `json:"totalPaid"`
	BalanceAmount          string      `json:"balanceAmount"`
	SaleOwner              string      `json:"saleOwner"`
	SaleOwnerManager       string      `json:"saleOwnerManager"`
	EmiStatus              string      `json:"emiStatus"`
	EmiDetails             *EmiDetails `json:"emiDetails,omitempty"`
	FinancialDetailsTypes  []string    `json:"financialDetailsTypes"`
	ConfirmationCallLink   string      `json:"confirmationCallLink"`
	SapEnteredAt           int64       `json:"sapEnteredAt"`
	CcUploadedAt           *int64      `json:"ccUploadedAt"`
	Credits                Credits     `json:"credits"`
	Escalation             *Mail       `json:"escalation"`
	CcResponse             *CcResponse `json:"ccResponse"`
	Audit                  *Audit      `json:"audit"`
}

type LeadsResponse struct {
	Leads              []Lead `json:"leads"`
	MailsSentThisSweep int    `json:"mailsSentThisSweep"`
}

type LeadSummary struct {
	ID                   string      `json:"id"`
	StudentFullName      string      `json:"studentFullName"`
	Course               string      `json:"course"`
	SaleOwner            string      `json:"saleOwner"`
	SaleOwnerManager     string      `json:"saleOwnerManager"`
	ConfirmationCallLink string      `json:"confirmationCallLink"`
	CcResponse           *CcResponse `json:"ccResponse"`
}

type Payment struct {
	ID            string `json:"id"`
	LeadID        string `json:"leadId"`
	PaymentType   string `json:"paymentType"`
	ModeOfPayment string `json:"modeOfPayment"`
	Amount        string `json:"amount"`
	PaymentDate   string `json:"paymentDate"`
	Verified      string `json:"verified"`
	Course        string `json:"course"`
	CourseValue   string `json:"courseValue"`
	Type          string `json:"type"`
	AddedAt       *int64 `json:"addedAt"`
	VerifiedAt    *int64 `json:"verifiedAt"`
}

// Source is one side of the audit comparison (Zoho, CC mail or EMI vendor). Empty fields are
// omitted so the frontend treats them as "not in this source".
type Source struct {
	Personal      *Personal      `bson:"personal,omitempty" json:"personal,omitempty"`
	CourseDetails *CourseDetails `bson:"courseDetails,omitempty" json:"courseDetails,omitempty"`
	Payment       *PaymentInfo   `bson:"payment,omitempty" json:"payment,omitempty"`
}

type Personal struct {
	LearnerName   string `bson:"learnerName,omitempty" json:"learnerName,omitempty"`
	Email         string `bson:"email,omitempty" json:"email,omitempty"`
	ContactNumber string `bson:"contactNumber,omitempty" json:"contactNumber,omitempty"`
}

type CourseDetails struct {
	CourseName string `bson:"courseName,omitempty" json:"courseName,omitempty"`
	Duration   string `bson:"duration,omitempty" json:"duration,omitempty"`
	StartDate  string `bson:"startDate,omitempty" json:"startDate,omitempty"`
	Time       string `bson:"time,omitempty" json:"time,omitempty"`
	Mode       string `bson:"mode,omitempty" json:"mode,omitempty"`
	Medium     string `bson:"medium,omitempty" json:"medium,omitempty"`
}

type PaymentInfo struct {
	TotalFee     string        `bson:"totalFee,omitempty" json:"totalFee,omitempty"`
	DownPayment  string        `bson:"downPayment,omitempty" json:"downPayment,omitempty"`
	Installments []Installment `bson:"installments,omitempty" json:"installments,omitempty"`
	Emi          *EmiTerms     `bson:"emi,omitempty" json:"emi,omitempty"`
}

type Installment struct {
	Percentage string `bson:"percentage,omitempty" json:"percentage,omitempty"`
	DueDate    string `bson:"dueDate,omitempty" json:"dueDate,omitempty"`
	Amount     string `bson:"amount,omitempty" json:"amount,omitempty"`
}

type EmiTerms struct {
	LoanAmount      string `bson:"loanAmount,omitempty" json:"loanAmount,omitempty"`
	MonthlyEmi      string `bson:"monthlyEmi,omitempty" json:"monthlyEmi,omitempty"`
	Roi             string `bson:"roi,omitempty" json:"roi,omitempty"`
	DueDate         string `bson:"dueDate,omitempty" json:"dueDate,omitempty"`
	Tenure          string `bson:"tenure,omitempty" json:"tenure,omitempty"`
	DisbursalStatus string `bson:"disbursalStatus,omitempty" json:"disbursalStatus,omitempty"`
}

type AuditSources struct {
	Zoho   *Source `json:"zoho"`
	Cc     *Source `json:"cc"`
	Vendor *Source `json:"vendor"`
}

// Discount is the lead's latest discount request, for the "Discount approved" check.
type Discount struct {
	Status             string `json:"status"`
	ActualCourseFee    string `json:"actualCourseFee"`
	RequestedCourseFee string `json:"requestedCourseFee"`
	DiscountValue      string `json:"discountValue"`
	ApprovedAt         *int64 `json:"approvedAt"`
}

// LeadAudit is everything the Lead Audit Workspace compares for one lead.
type LeadAudit struct {
	Lead                   Lead         `json:"lead"`
	Sources                AuditSources `json:"sources"`
	VendorName             string       `json:"vendorName"`
	PaymentMode            string       `json:"paymentMode"`
	PartialSplitUpCategory string       `json:"partialSplitUpCategory"`
	PointsCovered          []string     `json:"pointsCovered"`
	Rechecks               []Recheck    `json:"rechecks"`
	Discount               *Discount    `json:"discount"`
}

type CcVerification struct {
	PaymentMode            string   `json:"paymentMode"`
	PartialSplitUpCategory string   `json:"partialSplitUpCategory"`
	System                 Source   `json:"system"`
	Scraped                Source   `json:"scraped"`
	PointsCovered          []string `json:"pointsCovered"`
}

type AuditHistory struct {
	Rechecks []Recheck `json:"rechecks"`
	Payments []Payment `json:"payments"`
	Alerts   []Alert   `json:"alerts"`
}
