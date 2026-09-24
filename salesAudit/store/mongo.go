package store

import (
	"context"
	"errors"
	"regexp"

	"auditApp/salesAudit/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Mongo implements Store. The Zoho collections carry no program field, so only the feature's own
// collections can be filtered by program (listed as a gap in the README).
type Mongo struct {
	db *mongo.Database
}

func NewMongo(db *mongo.Database) *Mongo {
	return &Mongo{db: db}
}

func notDeleted(filter bson.M) bson.M {
	filter["deleted"] = false
	return filter
}

func scoped(program string, filter bson.M) bson.M {
	filter["program"] = program
	return notDeleted(filter)
}

func findAll[T any](ctx context.Context, collection *mongo.Collection, filter bson.M, opts ...*options.FindOptions) ([]T, error) {
	cursor, err := collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	results := []T{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func findOne[T any](ctx context.Context, collection *mongo.Collection, filter bson.M) (T, error) {
	var result T
	err := collection.FindOne(ctx, filter).Decode(&result)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return result, ErrNotFound
	}
	return result, err
}

func (s *Mongo) c(name string) *mongo.Collection {
	return s.db.Collection(name)
}

// Zoho

func (s *Mongo) ListAuditLeads(ctx context.Context) ([]models.ZohoLead, error) {
	return findAll[models.ZohoLead](ctx, s.c(models.ZohoLeadsCollection),
		notDeleted(bson.M{"Stage": models.AuditStage}))
}

func (s *Mongo) GetLead(ctx context.Context, leadID string) (models.ZohoLead, error) {
	return findOne[models.ZohoLead](ctx, s.c(models.ZohoLeadsCollection), notDeleted(bson.M{"ID": leadID}))
}

func (s *Mongo) PaymentsForLeads(ctx context.Context, zenIDs []string) ([]models.ZohoPayment, error) {
	return findAll[models.ZohoPayment](ctx, s.c(models.ZohoPaymentsCollection), notDeleted(bson.M{
		"$or": []bson.M{
			{"All_Enrolment": bson.M{"$in": zenIDs}},
			{"All_Enrolment": bson.M{"$in": []any{"", nil}}, "Zen_ID": bson.M{"$in": zenIDs}},
		},
	}))
}

func (s *Mongo) EmiForLeads(ctx context.Context, zenIDs []string) ([]models.ZohoEmi, error) {
	return findAll[models.ZohoEmi](ctx, s.c(models.ZohoEmiCollection),
		notDeleted(bson.M{"Zen_ID": bson.M{"$in": zenIDs}}))
}

func (s *Mongo) PartialReminders(ctx context.Context, leadID string) ([]models.ZohoPartialReminder, error) {
	return findAll[models.ZohoPartialReminder](ctx, s.c(models.ZohoPartialRemindersCollection),
		notDeleted(bson.M{"Student_ID": leadID}))
}

func (s *Mongo) SubscriptionReminders(ctx context.Context, zenID string) ([]models.ZohoSubscriptionReminder, error) {
	return findAll[models.ZohoSubscriptionReminder](ctx, s.c(models.ZohoSubscriptionRemindersCollection),
		notDeleted(bson.M{"Zen_ID": zenID}))
}

func (s *Mongo) DiscountsForEmail(ctx context.Context, email string) ([]models.ZohoDiscount, error) {
	return findAll[models.ZohoDiscount](ctx, s.c(models.ZohoDiscountsCollection), notDeleted(bson.M{
		"Learner_Email_ID": bson.M{"$regex": "^" + regexp.QuoteMeta(email) + "$", "$options": "i"},
	}))
}

// Alerts

func (s *Mongo) ListAlerts(ctx context.Context, program string, filter AlertFilter) ([]models.Alert, error) {
	query := bson.M{}
	if filter.LeadIDs != nil {
		query["leadId"] = bson.M{"$in": filter.LeadIDs}
	}
	if filter.Kind != "" {
		query["kind"] = filter.Kind
	}
	if filter.Since > 0 {
		query["sentAt"] = bson.M{"$gte": filter.Since}
	}
	return findAll[models.Alert](ctx, s.c(models.AlertsCollection), scoped(program, query),
		options.Find().SetSort(bson.D{{Key: "sentAt", Value: 1}}))
}

func (s *Mongo) GetAlert(ctx context.Context, program, alertID string) (models.Alert, error) {
	return findOne[models.Alert](ctx, s.c(models.AlertsCollection), scoped(program, bson.M{"id": alertID}))
}

func (s *Mongo) InsertAlert(ctx context.Context, alert models.Alert) error {
	_, err := s.c(models.AlertsCollection).InsertOne(ctx, alert)
	return err
}

func (s *Mongo) SetAlertDelivery(ctx context.Context, program, alertID, delivery string) error {
	_, err := s.c(models.AlertsCollection).UpdateOne(ctx, scoped(program, bson.M{"id": alertID}),
		bson.M{"$set": bson.M{"delivery": delivery}})
	return err
}

// CC responses

func (s *Mongo) CcResponses(ctx context.Context, program string, leadIDs []string) (map[string]models.CcResponse, error) {
	query := bson.M{}
	if leadIDs != nil {
		query["leadId"] = bson.M{"$in": leadIDs}
	}
	responses, err := findAll[models.CcResponse](ctx, s.c(models.CcResponsesCollection), scoped(program, query))
	if err != nil {
		return nil, err
	}
	byLead := map[string]models.CcResponse{}
	for _, response := range responses {
		byLead[response.LeadID] = response
	}
	return byLead, nil
}

// UpsertCcResponse keeps one response per lead; the first write's id and created are kept.
func (s *Mongo) UpsertCcResponse(ctx context.Context, response models.CcResponse) error {
	_, err := s.c(models.CcResponsesCollection).UpdateOne(ctx,
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

// Audits

func (s *Mongo) Audits(ctx context.Context, program string, leadIDs []string) (map[string]models.Audit, error) {
	query := bson.M{}
	if leadIDs != nil {
		query["leadId"] = bson.M{"$in": leadIDs}
	}
	audits, err := findAll[models.Audit](ctx, s.c(models.AuditsCollection), scoped(program, query))
	if err != nil {
		return nil, err
	}
	byLead := map[string]models.Audit{}
	for _, audit := range audits {
		byLead[audit.LeadID] = audit
	}
	return byLead, nil
}

func (s *Mongo) InsertAudit(ctx context.Context, audit models.Audit) error {
	_, err := s.c(models.AuditsCollection).InsertOne(ctx, audit)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	return err
}

// Rechecks

func (s *Mongo) ListRechecks(ctx context.Context, program string, filter RecheckFilter) ([]models.Recheck, error) {
	query := bson.M{}
	if filter.LeadID != "" {
		query["leadId"] = filter.LeadID
	}
	if filter.Status != "" {
		query["status"] = filter.Status
	}
	return findAll[models.Recheck](ctx, s.c(models.RechecksCollection), scoped(program, query),
		options.Find().SetSort(bson.D{{Key: "raisedAt", Value: -1}}))
}

func (s *Mongo) GetRecheck(ctx context.Context, program, recheckID string) (models.Recheck, error) {
	return findOne[models.Recheck](ctx, s.c(models.RechecksCollection), scoped(program, bson.M{"id": recheckID}))
}

func (s *Mongo) InsertRecheck(ctx context.Context, recheck models.Recheck) error {
	_, err := s.c(models.RechecksCollection).InsertOne(ctx, recheck)
	return err
}

func (s *Mongo) ResolveRecheck(ctx context.Context, program, recheckID string, resolvedAt int64) error {
	_, err := s.c(models.RechecksCollection).UpdateOne(ctx, scoped(program, bson.M{"id": recheckID}),
		bson.M{"$set": bson.M{"status": models.RecheckResolved, "resolvedAt": resolvedAt}})
	return err
}

func (s *Mongo) SetRecheckReminder(ctx context.Context, program, recheckID string, reminder models.Mail) error {
	_, err := s.c(models.RechecksCollection).UpdateOne(ctx, scoped(program, bson.M{"id": recheckID}),
		bson.M{"$set": bson.M{"lastReminder": reminder}})
	return err
}

// CC extracts

func (s *Mongo) GetCcExtract(ctx context.Context, program, leadID string) (models.CcExtract, error) {
	return findOne[models.CcExtract](ctx, s.c(models.CcExtractsCollection), scoped(program, bson.M{"leadId": leadID}))
}
