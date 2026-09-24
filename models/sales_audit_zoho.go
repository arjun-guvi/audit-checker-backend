package models

// Read-only records imported from Zoho Creator. Field names are Zoho's; only the fields the Sales
// Audit feature reads are mapped. Dates are Zoho strings (see worker.ParseZohoTime).

// ZohoLead is one LeadData (All_Enrolment) record.
type ZohoLead struct {
	ID                            string `bson:"ID"`
	ZenID                         string `bson:"zen_id"`
	Stage                         string `bson:"Stage"`
	StudentFullName               string `bson:"Student_Full_Name"`
	Email                         string `bson:"Email"`
	PrimaryPhone                  string `bson:"Primary_Phone"`
	Course                        string `bson:"Course"`
	CourseValue                   string `bson:"Course_Value"`
	DiscountGiven                 string `bson:"Discount_Given"`
	PaymentType                   string `bson:"Payment_Type"`
	PartialSplitUpCategory        string `bson:"Partial_Split_Up_Category"`
	TotalPaid                     string `bson:"Total_Paid"`
	BalanceAmount                 string `bson:"Balance_Amount"`
	SaleOwner                     string `bson:"Sale_Owner"`
	SaleOwnerManager              string `bson:"Sale_Owner_s_Manager"`
	EMIStatus                     string `bson:"EMI_Status"`
	ConfirmationCallLink          string `bson:"Confirmation_Call_Link"`
	ConfirmationCallAddedDateTime string `bson:"Confirmation_Call_Added_Date_Time"`
	AssignedBatchStartDate        string `bson:"Assigned_Batch.Start_Date"`
	ModeOfStudy                   string `bson:"Mode_of_Study"`
	PreferredLanguage             string `bson:"Preferred_Language"`
	AddedTime                     string `bson:"Added_Time"`

	// PaymentVerificationMailedAt is when the learner was mailed that a payment is still
	// unverified (Unix seconds); 0 when never. Written by the payment verification sweep.
	PaymentVerificationMailedAt int64 `bson:"Payment_Verification_Mailed_At"`
}

// ZohoPayment is one paymentData (Financial_Details) record. All_Enrolment holds the lead's zen_id.
type ZohoPayment struct {
	ID                   string `bson:"ID"`
	ZenID                string `bson:"Zen_ID"`
	Email                string `bson:"Email"`
	AllEnrolment         string `bson:"All_Enrolment"`
	PaymentType          string `bson:"Payment_Type"`
	ModeOfPayment        string `bson:"Mode_Of_Payment"`
	Amount               string `bson:"Amount"`
	PaymentDate          string `bson:"Payment_Date"`
	Verified             string `bson:"Verified"`
	VerifiedOn           string `bson:"Verified_on"`
	Type                 string `bson:"Type"`
	AddedTime            string `bson:"Added_Time"`
	EnrolmentCourse      string `bson:"All_Enrolment.Course"`
	EnrolmentCourseValue string `bson:"All_Enrolment.Course_Value"`
}

// LeadZenID is the enrolment the payment belongs to.
func (p ZohoPayment) LeadZenID() string {
	if p.AllEnrolment != "" {
		return p.AllEnrolment
	}
	return p.ZenID
}

// ZohoEmi is one EmiData record: the loan application raised with the EMI vendor.
type ZohoEmi struct {
	ID            string `bson:"ID"`
	ZenID         string `bson:"Zen_ID"`
	EMIVendor     string `bson:"EMI_Vendor"`
	EMIStatus     string `bson:"EMI_Status"`
	LoanAmount    string `bson:"Loan_amount"`
	FirstEMI      string `bson:"st_EMI_Amount"`
	ROI           string `bson:"ROI_in"`
	TenorInMonths string `bson:"Tenor_In_Months"`
	AddedTime     string `bson:"Added_Time1"`
}

// ZohoDiscount is one DiscountData (course discount request) record.
type ZohoDiscount struct {
	ID                 string `bson:"ID"`
	LearnerEmail       string `bson:"Learner_Email_ID"`
	Status             string `bson:"Status"`
	ActualCourseFee    string `bson:"Actual_Course_Fee"`
	RequestedCourseFee string `bson:"Requested_Course_Fee"`
	DiscountValue      string `bson:"Discount_Value"`
	ApprovedDateTime   string `bson:"Approved_Date_Time"`
	AddedTime          string `bson:"Added_Time"`
}

// ZohoPartialReminder is one PartialReminders record (a scheduled partial payment).
// Student_ID is the lead's Zoho ID.
type ZohoPartialReminder struct {
	ID            string `bson:"ID"`
	StudentID     string `bson:"Student_ID"`
	Amount        string `bson:"Amount"`
	DueDate       string `bson:"Due_Date"`
	ActualDueDate string `bson:"Actual_Due_Date"`
}

// ZohoSubscriptionReminder is one SubscriptionReminders record (a scheduled subscription payment).
type ZohoSubscriptionReminder struct {
	ID                   string `bson:"ID"`
	ZenID                string `bson:"Zen_ID"`
	Amount               string `bson:"Amount"`
	DueDate              string `bson:"Due_Date"`
	ActualDueDate        string `bson:"Actual_Due_Date"`
	NumberOfSubscription string `bson:"Number_of_Subscription"`
}
