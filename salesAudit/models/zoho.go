package models

import (
	"encoding/json"
	"strings"
)

// ZohoValue is any scalar Zoho sends, kept as text. Zoho is loose with types (an empty amount
// comes as "", a phone as a number), so every scalar is read through this.
type ZohoValue string

func (v *ZohoValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*v = ZohoValue(strings.TrimSpace(text))
		return nil
	}
	switch data[0] {
	case '{', '[', 'n': // objects, arrays and null carry no usable value
		*v = ""
	default: // numbers and booleans, as written
		*v = ZohoValue(strings.TrimSpace(string(data)))
	}
	return nil
}

// ZohoLearner is one record of the Zoho Creator learner API (see README for the shape).
type ZohoLearner struct {
	ZenID                  ZohoValue             `json:"zenId"`
	SuperleapID            ZohoValue             `json:"superleapId"`
	Name                   ZohoValue             `json:"name"`
	Email                  ZohoValue             `json:"email"`
	Phone                  ZohoValue             `json:"phone"`
	PreferredLanguage      ZohoValue             `json:"preferredLanguage"`
	SaleOwner              ZohoValue             `json:"saleOwner"`
	SaleOwnerManager       ZohoValue             `json:"saleOwnerManager"`
	SalesTeam              ZohoValue             `json:"salesTeam"`
	SalesFrom              ZohoValue             `json:"salesFrom"`
	Stage                  ZohoValue             `json:"Stage"`
	Status                 ZohoValue             `json:"status"`
	Product                ZohoValue             `json:"product"`
	ModeOfStudy            ZohoValue             `json:"modeOfStudy"`
	DateOfEnrollment       ZohoValue             `json:"dateOfEnrollment"`
	CrmLeadCreatedDate     ZohoValue             `json:"crmLeadCreatedDate"`
	OnboardingDateTime     ZohoValue             `json:"onboardingDateTime"`
	OnboardCoordinator     ZohoValue             `json:"onboardCoordinator"`
	PaymentType            ZohoValue             `json:"paymenttype"`
	PartialCategory        ZohoValue             `json:"partialCategory"`
	CourseFee              ZohoValue             `json:"courseFee"`
	TotalPaid              ZohoValue             `json:"totalPaid"`
	BalanceAmount          ZohoValue             `json:"balaceAmount"`
	PromoCode              ZohoValue             `json:"promoCode"`
	WillPayInSameMonth     ZohoValue             `json:"willLeadPayinSameMonth"`
	ZbCustomerID           ZohoValue             `json:"zbCustomerId"`
	ZbInvoiceID            ZohoValue             `json:"zbInvoiceId"`
	Source                 ZohoValue             `json:"source"`
	Medium                 ZohoValue             `json:"medium"`
	Campaign               ZohoValue             `json:"campaign"`
	Content                ZohoValue             `json:"content"`
	AffiliateID            ZohoValue             `json:"affiliateId"`
	TermsAndConditions     ZohoValue             `json:"terms&conditions"`
	ConfirmationCall       ZohoValue             `json:"confirmationCall"`
	AuditStatus            ZohoValue             `json:"auditStatus"`
	AuditCoordinator       ZohoValue             `json:"auditCoordinator"`
	AuditAssignedDateTime  ZohoValue             `json:"auditCoordinatorAssignedDateTime"`
	AuditCompletedDateTime ZohoValue             `json:"auditCompletedDateTime"`
	BatchData              ZohoBatch             `json:"batchData"`
	AdmissionDetails       map[string]ZohoValue  `json:"admissionDetails"`
	FinancialDetails       []ZohoFinancialDetail `json:"financialDetails"`
	EmiDetails             []ZohoEmi             `json:"EMIdetails"`
	CourseDiscountDetails  []ZohoDiscount        `json:"CourseDiscountDetails"`
	PartialReminders       []ZohoPartialReminder `json:"partialReminders"`
	SubscriptionReminders  []ZohoSubscription    `json:"subscriptionReminders"`
	RecheckDetails         []ZohoRecheck         `json:"recheckDetails"`
}

type ZohoBatch struct {
	BatchName ZohoValue `json:"batchName"`
	BatchType ZohoValue `json:"batchType"`
	Language  ZohoValue `json:"language"`
	StartDate ZohoValue `json:"startDate"`
	EndDate   ZohoValue `json:"endDate"`
	StartTime ZohoValue `json:"startTime"`
	Status    ZohoValue `json:"status"`
}

type ZohoFinancialDetail struct {
	RecordID         ZohoValue `json:"recordId"`
	Type             ZohoValue `json:"type"`
	Amount           ZohoValue `json:"amount"`
	ModeOfPayment    ZohoValue `json:"zbModeOfPayment"`
	UtrPaymentID     ZohoValue `json:"utrPaymentId"`
	Verified         ZohoValue `json:"verified"`
	VerifiedDate     ZohoValue `json:"verifiedDate"`
	PaymentDate      ZohoValue `json:"paymentDate"`
	ZbReceiptCreated ZohoValue `json:"zbReceiptCreated"`
}

type ZohoEmi struct {
	RecordID               ZohoValue `json:"recordId"`
	ApplicationID          ZohoValue `json:"applicationId"`
	EmiVendor              ZohoValue `json:"emiVendor"`
	EmiStatus              ZohoValue `json:"emiStatus"`
	Stage                  ZohoValue `json:"stage"`
	LoanAmount             ZohoValue `json:"loanAmount"`
	DisbursalAmount        ZohoValue `json:"disbursalAmount"`
	FirstEmiAmount         ZohoValue `json:"firstEMIamount"`
	TenorInMonth           ZohoValue `json:"tenorInMonth"`
	RoiInPercentage        ZohoValue `json:"ROIinPercentage"`
	ApplicationDate        ZohoValue `json:"applicationDate"`
	ApplicationInTheNameOf ZohoValue `json:"applicationInTheNameOf"`
}

type ZohoDiscount struct {
	RequestedCourseFee ZohoValue `json:"requestedCourseFee"`
	ActualCourseFee    ZohoValue `json:"actualCourseFee"`
	DiscountValue      ZohoValue `json:"discountValue"`
	RequestedPerson    ZohoValue `json:"requestedperson"`
	PaymentType        ZohoValue `json:"paymentType"`
	Status             ZohoValue `json:"status"`
}

type ZohoPartialReminder struct {
	RecordID             ZohoValue `json:"recordId"`
	NoOfPartial          ZohoValue `json:"noOfPartial"`
	Amount               ZohoValue `json:"amount"`
	DueDate              ZohoValue `json:"dueDate"`
	PartialPaymentStatus ZohoValue `json:"partialpaymentStatus"`
	LinkStatus           ZohoValue `json:"linkStatus"`
	PaidDateTime         ZohoValue `json:"paidDateTime"`
}

type ZohoSubscription struct {
	RecordID         ZohoValue `json:"recordId"`
	NoOfSubscription ZohoValue `json:"noOfSubscription"`
	Amount           ZohoValue `json:"amount"`
	DueDate          ZohoValue `json:"dueDate"`
	PaymentStatus    ZohoValue `json:"paymentStatus"`
}

type ZohoRecheck struct {
	SRID           ZohoValue   `json:"SRID"`
	AuditComments  ZohoValue   `json:"auditComments"`
	RequestPerson  ZohoValue   `json:"requestPerson"`
	TicketStatus   ZohoValue   `json:"ticketStatus"`
	RecheckAttempt ZohoValue   `json:"recheckattempt"`
	RecheckDate    ZohoValue   `json:"recheckDate"`
	PendingList    []ZohoValue `json:"pendingList"`
}
