package worker

import (
	"context"
	"errors"
	"regexp"

	"auditApp/config"
	"auditApp/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Sales Audit reads and writes. The Zoho collections are only read and carry no program field;
// the feature's own collections are always filtered by program. A missing single record comes
// back as mongo.ErrNoDocuments.

func notDeleted(filter bson.M) bson.M {
	filter["deleted"] = false
	return filter
}

func scoped(program string, filter bson.M) bson.M {
	filter["program"] = program
	return notDeleted(filter)
}

func findAll[T any](ctx context.Context, collection string, filter bson.M, opts ...*options.FindOptions) ([]T, error) {
	cursor, err := config.MongoDB.Collection(collection).Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	results := []T{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func findOne[T any](ctx context.Context, collection string, filter bson.M) (T, error) {
	var result T
	err := config.MongoDB.Collection(collection).FindOne(ctx, filter).Decode(&result)
	return result, err
}

func insertOne(ctx context.Context, collection string, document any) error {
	_, err := config.MongoDB.Collection(collection).InsertOne(ctx, document)
	return err
}

func updateOne(ctx context.Context, collection string, filter bson.M, set bson.M) error {
	_, err := config.MongoDB.Collection(collection).UpdateOne(ctx, filter, bson.M{"$set": set})
	return err
}

func upsertOne(ctx context.Context, collection string, filter bson.M, set bson.M) error {
	_, err := config.MongoDB.Collection(collection).UpdateOne(ctx, filter,
		bson.M{"$set": set}, options.Update().SetUpsert(true))
	return err
}

// Zoho (read-only)

func FindAuditLeads(ctx context.Context) ([]models.ZohoLead, error) {
	return findAll[models.ZohoLead](ctx, models.ZohoLeadsCollection, notDeleted(bson.M{"Stage": models.AuditStage}))
}

func FindZohoLead(ctx context.Context, leadID string) (models.ZohoLead, error) {
	return findOne[models.ZohoLead](ctx, models.ZohoLeadsCollection, notDeleted(bson.M{"ID": leadID}))
}

// FindPayments returns the payments of the given enrolments (All_Enrolment, or Zen_ID when the
// record has no enrolment).
func FindPayments(ctx context.Context, zenIDs []string) ([]models.ZohoPayment, error) {
	return findAll[models.ZohoPayment](ctx, models.ZohoPaymentsCollection, notDeleted(bson.M{
		"$or": []bson.M{
			{"All_Enrolment": bson.M{"$in": zenIDs}},
			{"All_Enrolment": bson.M{"$in": []any{"", nil}}, "Zen_ID": bson.M{"$in": zenIDs}},
		},
	}))
}

func FindEmis(ctx context.Context, zenIDs []string) ([]models.ZohoEmi, error) {
	return findAll[models.ZohoEmi](ctx, models.ZohoEmiCollection, notDeleted(bson.M{"Zen_ID": bson.M{"$in": zenIDs}}))
}

func FindPartialReminders(ctx context.Context, leadID string) ([]models.ZohoPartialReminder, error) {
	return findAll[models.ZohoPartialReminder](ctx, models.ZohoPartialRemindersCollection,
		notDeleted(bson.M{"Student_ID": leadID}))
}

func FindSubscriptionReminders(ctx context.Context, zenID string) ([]models.ZohoSubscriptionReminder, error) {
	return findAll[models.ZohoSubscriptionReminder](ctx, models.ZohoSubscriptionRemindersCollection,
		notDeleted(bson.M{"Zen_ID": zenID}))
}

func FindDiscounts(ctx context.Context, email string) ([]models.ZohoDiscount, error) {
	return findAll[models.ZohoDiscount](ctx, models.ZohoDiscountsCollection, notDeleted(bson.M{
		"Learner_Email_ID": bson.M{"$regex": "^" + regexp.QuoteMeta(email) + "$", "$options": "i"},
	}))
}

// Payment verification (the lead fields below are written by this feature, not by Zoho)

// FindLeadsNotMailedForPaymentVerification returns the leads with an email whose learner has not
// yet been mailed about an unverified payment.
func FindLeadsNotMailedForPaymentVerification(ctx context.Context) ([]models.ZohoLead, error) {
	return findAll[models.ZohoLead](ctx, models.ZohoLeadsCollection, notDeleted(bson.M{
		"Email":                          bson.M{"$nin": []any{"", nil}},
		"Payment_Verification_Mailed_At": bson.M{"$in": []any{0, nil}},
	}))
}

// FindPaymentsByEmail returns the payments of the given learner emails (as the Zoho import
// stores them, lower case).
func FindPaymentsByEmail(ctx context.Context, emails []string) ([]models.ZohoPayment, error) {
	return findAll[models.ZohoPayment](ctx, models.ZohoPaymentsCollection,
		notDeleted(bson.M{"Email": bson.M{"$in": emails}}))
}

func SetPaymentVerificationMailedAt(ctx context.Context, leadID string, mailedAt int64) error {
	return updateOne(ctx, models.ZohoLeadsCollection, notDeleted(bson.M{"ID": leadID}),
		bson.M{"Payment_Verification_Mailed_At": mailedAt})
}

// Alerts (mail log)

// FindAlerts returns alerts oldest first. A nil leadIDs, empty kind or zero since means "any".
func FindAlerts(ctx context.Context, program string, leadIDs []string, kind string, since int64) ([]models.Alert, error) {
	filter := bson.M{}
	if leadIDs != nil {
		filter["leadId"] = bson.M{"$in": leadIDs}
	}
	if kind != "" {
		filter["kind"] = kind
	}
	if since > 0 {
		filter["sentAt"] = bson.M{"$gte": since}
	}
	return findAll[models.Alert](ctx, models.AlertsCollection, scoped(program, filter),
		options.Find().SetSort(bson.D{{Key: "sentAt", Value: 1}}))
}

func FindAlert(ctx context.Context, program, alertID string) (models.Alert, error) {
	return findOne[models.Alert](ctx, models.AlertsCollection, scoped(program, bson.M{"id": alertID}))
}

func SetAlertDelivery(ctx context.Context, program, alertID, delivery string) error {
	return updateOne(ctx, models.AlertsCollection, scoped(program, bson.M{"id": alertID}), bson.M{"delivery": delivery})
}

// CC responses and audits, keyed by lead id

func FindCcResponses(ctx context.Context, program string, leadIDs []string) (map[string]models.CcResponse, error) {
	responses, err := findAll[models.CcResponse](ctx, models.CcResponsesCollection,
		scoped(program, bson.M{"leadId": bson.M{"$in": leadIDs}}))
	if err != nil {
		return nil, err
	}
	byLead := map[string]models.CcResponse{}
	for _, response := range responses {
		byLead[response.LeadID] = response
	}
	return byLead, nil
}

// SaveCcResponse keeps one response per lead; the first write's id and created are kept.
func SaveCcResponse(ctx context.Context, response models.CcResponse) error {
	_, err := config.MongoDB.Collection(models.CcResponsesCollection).UpdateOne(ctx,
		scoped(response.Program, bson.M{"leadId": response.LeadID}),
		bson.M{
			"$set": bson.M{
				"response":  response.Response,
				"updatedAt": response.UpdatedAt,
				"alert":     response.Alert,
			},
			"$setOnInsert": bson.M{"id": response.ID, "created": response.Created},
		},
		options.Update().SetUpsert(true))
	return err
}

func FindAudits(ctx context.Context, program string, leadIDs []string) (map[string]models.Audit, error) {
	audits, err := findAll[models.Audit](ctx, models.AuditsCollection, scoped(program, bson.M{"leadId": bson.M{"$in": leadIDs}}))
	if err != nil {
		return nil, err
	}
	byLead := map[string]models.Audit{}
	for _, audit := range audits {
		byLead[audit.LeadID] = audit
	}
	return byLead, nil
}

// InsertAudit fails with a duplicate key error (mongo.IsDuplicateKeyError) when the lead is
// already audited, through the unique {program, leadId} index.
func InsertAudit(ctx context.Context, audit models.Audit) error {
	return insertOne(ctx, models.AuditsCollection, audit)
}

// Rechecks

// FindRechecks returns rechecks newest first. An empty leadID or status means "any".
func FindRechecks(ctx context.Context, program, leadID, status string) ([]models.Recheck, error) {
	filter := bson.M{}
	if leadID != "" {
		filter["leadId"] = leadID
	}
	if status != "" {
		filter["status"] = status
	}
	return findAll[models.Recheck](ctx, models.RechecksCollection, scoped(program, filter),
		options.Find().SetSort(bson.D{{Key: "raisedAt", Value: -1}}))
}

func FindRecheck(ctx context.Context, program, recheckID string) (models.Recheck, error) {
	return findOne[models.Recheck](ctx, models.RechecksCollection, scoped(program, bson.M{"id": recheckID}))
}

func InsertRecheck(ctx context.Context, recheck models.Recheck) error {
	return insertOne(ctx, models.RechecksCollection, recheck)
}

func ResolveRecheck(ctx context.Context, program, recheckID string, resolvedAt int64) error {
	return updateOne(ctx, models.RechecksCollection, scoped(program, bson.M{"id": recheckID}),
		bson.M{"status": models.RecheckResolved, "resolvedAt": resolvedAt})
}

func setRecheckReminder(ctx context.Context, program, recheckID string, reminder models.Mail) error {
	return updateOne(ctx, models.RechecksCollection, scoped(program, bson.M{"id": recheckID}),
		bson.M{"lastReminder": reminder})
}

// CC extracts (written by the future CC parser)

func FindCcExtract(ctx context.Context, program, leadID string) (models.CcExtract, error) {
	return findOne[models.CcExtract](ctx, models.CcExtractsCollection, scoped(program, bson.M{"leadId": leadID}))
}

// IsNotFound reports whether a single-record lookup found nothing.
func IsNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}
