package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"auditApp/config"
	"auditApp/models"

	"github.com/gocraft/work"
	"go.mongodb.org/mongo-driver/bson"
)

// zohoValue is any scalar Zoho sends, kept as text. Zoho is loose with types (an empty amount
// comes as "", a phone as a number), and a strict string/json.Number field made the whole learner
// fail to decode, so every scalar is read through this.
type zohoValue string

func (v *zohoValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*v = zohoValue(text)
		return nil
	}
	switch data[0] {
	case '{', '[', 'n': // objects, arrays and null carry no usable value
		*v = ""
	default: // numbers and booleans, as written
		*v = zohoValue(data)
	}
	return nil
}

func (v zohoValue) String() string {
	return strings.TrimSpace(string(v))
}

type zohoLearner struct {
	PreferredLanguage     zohoValue                  `json:"preferredLanguage"`
	SaleOwnerManager      zohoValue                  `json:"saleOwnerManager"`
	FinancialDetails      []zohoFinancialDetail      `json:"financialDetails"`
	PartialReminders      []zohoPartialReminder      `json:"partialReminders"`
	SubscriptionReminders []zohoSubscriptionReminder `json:"subscriptionReminders"`
	Source                zohoValue                  `json:"source"`
	SalesTeam             zohoValue                  `json:"salesTeam"`
	BalanceAmount         zohoValue                  `json:"balaceAmount"`
	Email                 zohoValue                  `json:"email"`
	Product               zohoValue                  `json:"product"`
	PartialCategory       zohoValue                  `json:"partialCategory"`
	ModeOfStudy           zohoValue                  `json:"modeOfStudy"`
	TotalPaid             zohoValue                  `json:"totalPaid"`
	ZBCustomerID          zohoValue                  `json:"zbCustomerId"`
	DateOfEnrollment      zohoValue                  `json:"dateOfEnrollment"`
	Phone                 zohoValue                  `json:"phone"`
	CourseFee             zohoValue                  `json:"courseFee"`
	SaleOwner             zohoValue                  `json:"saleOwner"`
	Name                  zohoValue                  `json:"name"`
	CRMLeadCreatedDate    zohoValue                  `json:"crmLeadCreatedDate"`
	SuperleapID           zohoValue                  `json:"superleapId"`
	PaymentType           zohoValue                  `json:"paymenttype"`
	Status                zohoValue                  `json:"status"`
	ZenId                 zohoValue                  `json:"zenId"`
}

type zohoFinancialDetail struct {
	RecordID      zohoValue `json:"recordId"`
	Amount        zohoValue `json:"amount"`
	VerifiedDate  zohoValue `json:"verifiedDate"`
	PaymentDate   zohoValue `json:"paymentDate"`
	Type          zohoValue `json:"type"`
	Verified      zohoValue `json:"verified"`
	ModeOfPayment zohoValue `json:"zbModeOfPayment"`
	PaymentID     zohoValue `json:"utrPaymentId"`
}

type zohoPartialReminder struct {
	RecordID zohoValue `json:"recordId"`
	Amount   zohoValue `json:"amount"`
	DueDate  zohoValue `json:"dueDate"`
	Status   zohoValue `json:"partialStaus"`
}

type zohoSubscriptionReminder struct {
	RecordID             zohoValue `json:"recordId"`
	Amount               zohoValue `json:"amount"`
	DueDate              zohoValue `json:"dueDate"`
	NumberOfSubscription zohoValue `json:"noOfSubscription"`
}

func ZohoLearnerImportJob(job *work.Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	count, err := ImportZohoLearners(ctx, http.DefaultClient)

	log.Printf("zoho: imported %d learner(s)", count)

	return err
}

func ImportZohoLearners(ctx context.Context, client *http.Client) (int, error) {
	if config.ZohoAPIPublicKey == "" {
		return 0, errors.New("ZOHO_API_PUBLIC_KEY is not configured")
	}

	endpoint, err := url.Parse(config.ZohoAPIURL)
	if err != nil {
		return 0, fmt.Errorf("parse Zoho URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("publickey", config.ZohoAPIPublicKey)
	query.Set("from", config.ZohoAPIFrom)
	query.Set("to", config.ZohoAPITo)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return 0, fmt.Errorf("create Zoho request: %w", err)
	}

	response, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch Zoho learners: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, fmt.Errorf("read Zoho response: %w", err)
	}

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf(
			"Zoho returned %s: %s",
			response.Status,
			strings.TrimSpace(string(body)),
		)
	}

	var raw json.RawMessage

	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, fmt.Errorf("decode Zoho response: %w", err)
	}

	learners, err := decodeLearners(raw)
	if err != nil {
		return 0, err
	}
	// One failing learner must not stop the rest of the batch.
	imported := 0
	var errs []error
	for _, learner := range learners {
		if err := upsertZohoLearner(ctx, learner); err != nil {
			log.Printf("zoho: %v", err)
			errs = append(errs, err)
			continue
		}
		imported++
	}

	return imported, errors.Join(errs...)
}

func decodeLearners(raw json.RawMessage) ([]zohoLearner, error) {
	// ---------------------------------------------------------
	// Case 1: {"result": [...]}
	// ---------------------------------------------------------
	// The result may itself be a list, a {"data": [...]} or one learner.
	var resultEnvelope struct {
		Result json.RawMessage `json:"result"`
	}

	if err := json.Unmarshal(raw, &resultEnvelope); err == nil &&
		len(resultEnvelope.Result) > 0 && string(resultEnvelope.Result) != "null" {
		return decodeLearners(resultEnvelope.Result)
	}

	// ---------------------------------------------------------
	// Case 2: raw array [...]
	// ---------------------------------------------------------
	if len(raw) > 0 && raw[0] == '[' {
		var rawItems []json.RawMessage
		if err := json.Unmarshal(raw, &rawItems); err != nil {
			return nil, fmt.Errorf("decode Zoho learners: %w", err)
		}
		return decodeLearnerList(rawItems), nil
	}

	// ---------------------------------------------------------
	// Case 3: {"data": [...]}
	// ---------------------------------------------------------
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil &&
		len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		return decodeLearners(envelope.Data)
	}

	// ---------------------------------------------------------
	// Case 4: single learner object
	// ---------------------------------------------------------
	var learner zohoLearner
	if err := json.Unmarshal(raw, &learner); err != nil {
		return nil, fmt.Errorf("decode Zoho learner: %w", err)
	}
	if learner.Email.String() == "" {
		return nil, fmt.Errorf("decode Zoho learner: no recognizable learner fields in payload")
	}

	return []zohoLearner{learner}, nil
}

// decodeLearnerList decodes each raw element independently so that one
// malformed record (e.g. a stray {"Status":{}} entry) doesn't cause the
// entire batch to be discarded.
func decodeLearnerList(rawItems []json.RawMessage) []zohoLearner {
	learners := make([]zohoLearner, 0, len(rawItems))

	for i, item := range rawItems {
		var learner zohoLearner
		if err := json.Unmarshal(item, &learner); err != nil {
			log.Printf("zoho: skipping malformed learner record at index %d: %v", i, err)
			continue
		}
		if learner.Email.String() == "" {
			log.Printf("zoho: skipping learner record at index %d: no email", i)
			continue
		}
		learners = append(learners, learner)
	}

	return learners
}

func upsertZohoLearner(ctx context.Context, learner zohoLearner) error {
	// ---------------------------------------------------------
	// EMAIL IS THE PRIMARY LEARNER IDENTIFIER
	// ---------------------------------------------------------

	email := strings.ToLower(learner.Email.String())

	if email == "" {
		return errors.New("Zoho learner has no email")
	}

	// Keep SuperleapID as a secondary/reference field.
	// zenID := learner.SuperleapID.String()

	// ---------------------------------------------------------
	// Lead
	// ---------------------------------------------------------

	lead := bson.M{
		"ID":                        email,
		"zenId":                     learner.ZenId,
		"Stage":                     "Audit",
		"Student_Full_Name":         learner.Name.String(),
		"Email":                     email,
		"Primary_Phone":             learner.Phone.String(),
		"Course":                    learner.Product.String(),
		"Course_Value":              numberString(learner.CourseFee),
		"Payment_Type":              learner.PaymentType.String(),
		"Partial_Split_Up_Category": learner.PartialCategory.String(),
		"Total_Paid":                numberString(learner.TotalPaid),
		"Balance_Amount":            numberString(learner.BalanceAmount),
		"Sale_Owner":                learner.SaleOwner.String(),
		"Sale_Owner_s_Manager":      learner.SaleOwnerManager.String(),
		"Mode_of_Study":             learner.ModeOfStudy.String(),
		"Preferred_Language":        learner.PreferredLanguage.String(),
		"Added_Time":                learner.CRMLeadCreatedDate.String(),
		"Date_of_Enrollment":        learner.DateOfEnrollment.String(),
		"Source":                    learner.Source.String(),
		"Sales_Team":                learner.SalesTeam.String(),
		"Status":                    learner.Status.String(),
		"deleted":                   false,
	}

	// ---------------------------------------------------------
	// IMPORTANT:
	// Learner is upserted using EMAIL.
	// ---------------------------------------------------------

	if err := upsertOne(
		ctx,
		models.ZohoLeadsCollection,
		bson.M{
			"Email": email,
		},
		lead,
	); err != nil {
		return fmt.Errorf(
			"upsert learner %s: %w",
			email,
			err,
		)
	}

	// ---------------------------------------------------------
	// Payments
	// ---------------------------------------------------------

	for _, payment := range learner.FinancialDetails {
		id := numberString(payment.RecordID)

		if id == "" {
			continue
		}

		doc := bson.M{
			"ID":              id,
			"Email":           email,
			"Zen_ID":          learner.ZenId,
			"All_Enrolment":   learner.ZenId,
			"Type":            payment.Type.String(),
			"Amount":          numberString(payment.Amount),
			"Verified":        payment.Verified.String(),
			"Verified_on":     payment.VerifiedDate.String(),
			"Payment_Date":    payment.PaymentDate.String(),
			"Mode_Of_Payment": payment.ModeOfPayment.String(),
			"UTR_Payment_ID":  payment.PaymentID.String(),
			"Payment_Type":    learner.PaymentType.String(),
			"Added_Time":      payment.PaymentDate.String(),
			"deleted":         false,
		}

		// Payment still uses its own RecordID as the unique key.
		// Multiple payments can belong to the same email.

		if err := upsertOne(
			ctx,
			models.ZohoPaymentsCollection,
			bson.M{
				"ID": id,
			},
			doc,
		); err != nil {
			return fmt.Errorf(
				"upsert payment %s: %w",
				id,
				err,
			)
		}
	}

	// ---------------------------------------------------------
	// Partial reminders
	// ---------------------------------------------------------

	for _, reminder := range learner.PartialReminders {
		id := numberString(reminder.RecordID)

		if id == "" {
			continue
		}

		doc := bson.M{
			"ID":              id,
			"Email":           email,
			"Student_ID":      email,
			"Zen_ID":          learner.ZenId,
			"Amount":          numberString(reminder.Amount),
			"Due_Date":        reminder.DueDate.String(),
			"Actual_Due_Date": reminder.DueDate.String(),
			"Partial_Status":  reminder.Status.String(),
			"deleted":         false,
		}

		// Reminder still uses its own RecordID as the unique key.

		if err := upsertOne(
			ctx,
			models.ZohoPartialRemindersCollection,
			bson.M{
				"ID": id,
			},
			doc,
		); err != nil {
			return fmt.Errorf(
				"upsert partial reminder %s: %w",
				id,
				err,
			)
		}
	}

	// ---------------------------------------------------------
	// Subscription reminders
	// ---------------------------------------------------------

	for _, reminder := range learner.SubscriptionReminders {
		id := numberString(reminder.RecordID)

		if id == "" {
			continue
		}

		doc := bson.M{
			"ID":                     id,
			"Email":                  email,
			"Zen_ID":                 learner.ZenId,
			"Amount":                 numberString(reminder.Amount),
			"Due_Date":               reminder.DueDate.String(),
			"Actual_Due_Date":        reminder.DueDate.String(),
			"Number_of_Subscription": reminder.NumberOfSubscription.String(),
			"deleted":                false,
		}

		// Reminder still uses its own RecordID as the unique key.

		if err := upsertOne(
			ctx,
			models.ZohoSubscriptionRemindersCollection,
			bson.M{
				"ID": id,
			},
			doc,
		); err != nil {
			return fmt.Errorf(
				"upsert subscription reminder %s: %w",
				id,
				err,
			)
		}
	}

	return nil
}

func numberString(value zohoValue) string {
	text := value.String()
	if text == "" {
		return ""
	}

	if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
		return strconv.FormatInt(integer, 10)
	}

	return text
}
