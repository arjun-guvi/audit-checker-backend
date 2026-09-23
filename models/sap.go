package models

import "time"

// SAP represents the SAP audit record with all fields from the data model
type SAP struct {
	ID                                        string    `bson:"_id,omitempty" json:"id"`
	ZenID                                     string    `bson:"zen_id" json:"zen_id"`
	ProspectLeadID                            string    `bson:"Prospect_Lead_Id" json:"prospect_lead_id"`
	Email                                     string    `bson:"Email" json:"email"`
	StudentFullName                           string    `bson:"Student_Full_Name" json:"student_full_name"`
	PrimaryPhone                              string    `bson:"Primary_Phone" json:"primary_phone"`
	Course                                    string    `bson:"Course" json:"course"`
	Stage                                     string    `bson:"Stage" json:"stage"`
	Status                                    string    `bson:"status" json:"status"`
	LeadSource                                string    `bson:"Lead_Source_new" json:"lead_source"`
	SalesFrom                                  string    `bson:"Sales_From" json:"sales_from"`
	SaleOwner                                 string    `bson:"Sale_Owner" json:"sale_owner"`
	PaymentType                               string    `bson:"Payment_Type" json:"payment_type"`
	PaymentStatus                             string    `bson:"Payment_Status" json:"payment_status"`
	TotalPaid                                 string    `bson:"Total_Paid" json:"total_paid"`
	BalanceAmount                             string    `bson:"Balance_Amount" json:"balance_amount"`
	CourseValue                               string    `bson:"Course_Value" json:"course_value"`
	EMIStatus                                 string    `bson:"EMI_Status" json:"emi_status"`
	BookingFeePaidStageDateTime               string    `bson:"BookingFeePaid_Stage_Date_Time" json:"booking_fee_paid_stage_date_time"`
	PaymentLinkSharedStageDateTime            string    `bson:"Payment_Link_Shared_Stage_Date_Time" json:"payment_link_shared_stage_date_time"`
	AdmissionFormLookup                       string    `bson:"Admission_Form_Lookup" json:"admission_form_lookup"`
	FilledAdmissionForm                       string    `bson:"Filled_Admission_Form" json:"filled_admission_form"`
	PrebootAttended                           string    `bson:"Pre_bootcamp_attended" json:"preboot_attended"`
	MainbootAttendingStageDateTime            string    `bson:"Main_Boot_ATTENDING_Stage_Date_Time" json:"mainboot_attending_stage_date_time"`
	AwaitingMainBootcampStageDateTime         string    `bson:"Awaiting_Main_Bootcamp_Stage_Date_Time" json:"awaiting_main_bootcamp_stage_date_time"`
	AssignedBatch                              string    `bson:"Assigned_Batch" json:"assigned_batch"`
	PreferredLanguage                         string    `bson:"Preferred_Language" json:"preferred_language"`
	ModeOfStudy                               string    `bson:"Mode_of_Study" json:"mode_of_study"`
	PortalActivatedStatus                     string    `bson:"Portal_Activated_Status" json:"portal_activated_status"`
	BridgeCourseActivated                     string    `bson:"Bridge_course_activated" json:"bridge_course_activated"`
	BridgeCourseActivatedDate                 string    `bson:"Bridge_Course_activated_date" json:"bridge_course_activated_date"`
	ReferrerStage                             string    `bson:"Referrer_Stage" json:"referrer_stage"`
	ReferrerName                              string    `bson:"Referrer_Name" json:"referrer_name"`
	ReferrerEmail                             string    `bson:"Referrer_Email" json:"referrer_email"`
	ReferrerMobNum                            string    `bson:"Referrer_Mob_Num" json:"referrer_mob_num"`
	ReferrerAmount                            string    `bson:"Referrer_Amount" json:"referrer_amount"`
	BootcampCoordinator                       string    `bson:"Bootcamp_Coordinator" json:"bootcamp_coordinator"`
	BootcampCoordinatorStatus                 string    `bson:"Bootcamp_Coordinator_Status" json:"bootcamp_coordinator_status"`
	SalesTeamComments                         string    `bson:"Sales_Team_Comments" json:"sales_team_comments"`
	InvoiceCreated                            string    `bson:"Invoice_Created" json:"invoice_created"`
	ZBCustomerID                              string    `bson:"ZB_Customer_ID" json:"zb_customer_id"`
	ZBInvoiceID                               string    `bson:"ZB_Invoice_ID" json:"zb_invoice_id"`
	AddedTime                                 string    `bson:"Added_Time" json:"added_time"`
	ModifiedTime                              string    `bson:"Modified_Time" json:"modified_time"`
	Deleted                                   bool      `bson:"deleted" json:"deleted"`
	
	// Reminder and Email fields
	MailsToSendTo                             []string  `bson:"mails_to_send_to" json:"mails_to_send_to"`
	LastReminderSent                          *time.Time `bson:"last_reminder_sent" json:"last_reminder_sent"`
	NextReminderDue                           *time.Time `bson:"next_reminder_due" json:"next_reminder_due"`
	ReminderCount                             int       `bson:"reminder_count" json:"reminder_count"`
	ReminderIntervalMinutes                   int       `bson:"reminder_interval_minutes" json:"reminder_interval_minutes"`
	IsActive                                  bool      `bson:"is_active" json:"is_active"`
	CreatedAt                                 time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt                                 time.Time `bson:"updated_at" json:"updated_at"`
}

// SAPCollection returns the MongoDB collection for SAP records
func SAPCollection() string {
	return "sap_records"
}

// SAPReminderConfig holds reminder configuration
type SAPReminderConfig struct {
	IntervalMinutes    int      `bson:"interval_minutes" json:"interval_minutes"`
	RecipientEmails    []string `bson:"recipient_emails" json:"recipient_emails"`
	Enabled            bool     `bson:"enabled" json:"enabled"`
	MaxReminders       int      `bson:"max_reminders" json:"max_reminders"`
	EmailTemplate      string   `bson:"email_template" json:"email_template"`
	EmailSubject       string   `bson:"email_subject" json:"email_subject"`
}

// SAPReminderConfigCollection returns the MongoDB collection for reminder config
func SAPReminderConfigCollection() string {
	return "sap_reminder_config"
}
