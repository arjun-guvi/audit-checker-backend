package store

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"auditApp/config"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Mongo implementations. The filters must agree with core.MatchLead / MatchRecheck / MatchAudit,
// which the fake store uses.

func collection(name string) *mongo.Collection {
	return config.MongoDB.Collection(name)
}

// scoped adds the tenancy and soft-delete conditions every query carries.
func scoped(program string, filter bson.M) bson.M {
	filter["program"] = program
	filter["deleted"] = false
	return filter
}

func notFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}

func findAll[T any](ctx context.Context, name string, filter bson.M, opts ...*options.FindOptions) ([]T, error) {
	cursor, err := collection(name).Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	results := []T{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func findOne[T any](ctx context.Context, name string, filter bson.M) (T, error) {
	var result T
	err := collection(name).FindOne(ctx, filter).Decode(&result)
	return result, notFound(err)
}

func insertOne(ctx context.Context, name string, document any) error {
	_, err := collection(name).InsertOne(ctx, document)
	return err
}

func updateOne(ctx context.Context, name string, filter bson.M, set bson.M) error {
	result, err := collection(name).UpdateOne(ctx, filter, bson.M{"$set": set})
	if err == nil && result.MatchedCount == 0 {
		return ErrNotFound
	}
	return err
}

func replaceOne(ctx context.Context, name string, filter bson.M, document any) error {
	result, err := collection(name).ReplaceOne(ctx, filter, document)
	if err == nil && result.MatchedCount == 0 {
		return ErrNotFound
	}
	return err
}

func lower(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strings.ToLower(value))
	}
	return result
}

// scopeFilter is the Scope as conditions on auditorField / bdaEmail / bdmEmail.
func scopeFilter(scope models.Scope, auditorField string) []bson.M {
	if scope.None {
		return []bson.M{{"id": bson.M{"$in": []string{}}}}
	}
	conditions := []bson.M{}
	if scope.AuditorEmail != "" {
		conditions = append(conditions, bson.M{auditorField: strings.ToLower(scope.AuditorEmail)})
	}
	switch {
	case scope.BdmEmail != "":
		conditions = append(conditions, bson.M{"$or": []bson.M{
			{"bdmEmail": strings.ToLower(scope.BdmEmail)},
			{"bdaEmail": bson.M{"$in": lower(scope.BdaEmails)}},
		}})
	case scope.BdaEmails != nil:
		conditions = append(conditions, bson.M{"bdaEmail": bson.M{"$in": lower(scope.BdaEmails)}})
	}
	return conditions
}

func rangeFilter(span models.Range) bson.M {
	condition := bson.M{"$gt": 0}
	if span.From != 0 {
		condition["$gte"] = span.From
	}
	if span.To != 0 {
		condition["$lt"] = span.To
	}
	return condition
}

func withAnd(program string, conditions []bson.M) bson.M {
	if len(conditions) == 0 {
		return scoped(program, bson.M{})
	}
	return scoped(program, bson.M{"$and": conditions})
}

// lowerAll lower-cases emails, which are stored lower-case.
func lowerAll(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = strings.ToLower(value)
	}
	return result
}

// Members

func findMemberByHash(ctx context.Context, program, userHash string) (models.Member, error) {
	return findOne[models.Member](ctx, models.MembersCollection, scoped(program, bson.M{"userHash": userHash}))
}

func findMemberByEmail(ctx context.Context, program, email string) (models.Member, error) {
	return findOne[models.Member](ctx, models.MembersCollection, scoped(program, bson.M{"email": strings.ToLower(email)}))
}

func findMember(ctx context.Context, program, memberID string) (models.Member, error) {
	return findOne[models.Member](ctx, models.MembersCollection, scoped(program, bson.M{"id": memberID}))
}

func findMembers(ctx context.Context, program, role string) ([]models.Member, error) {
	filter := bson.M{}
	if role != "" {
		filter["role"] = role
	}
	return findAll[models.Member](ctx, models.MembersCollection, scoped(program, filter),
		options.Find().SetSort(bson.D{{Key: "role", Value: 1}, {Key: "name", Value: 1}}))
}

func insertMember(ctx context.Context, member models.Member) error {
	return insertOne(ctx, models.MembersCollection, member)
}

func replaceMember(ctx context.Context, member models.Member) error {
	return replaceOne(ctx, models.MembersCollection, scoped(member.Program, bson.M{"id": member.ID}), member)
}

// Leads

func leadFilter(program string, query models.LeadQuery) bson.M {
	conditions := scopeFilter(query.Scope, "assignment.auditorEmail")
	if query.LeadIDs != nil {
		conditions = append(conditions, bson.M{"id": bson.M{"$in": query.LeadIDs}})
	}
	if len(query.AuditStatuses) > 0 {
		conditions = append(conditions, bson.M{"audit.status": bson.M{"$in": query.AuditStatuses}})
	}
	if len(query.Regions) > 0 {
		conditions = append(conditions, bson.M{"region": bson.M{"$in": query.Regions}})
	}
	if len(query.AuditorEmails) > 0 {
		conditions = append(conditions, bson.M{"assignment.auditorEmail": bson.M{"$in": lowerAll(query.AuditorEmails)}})
	}
	if len(query.BdaEmails) > 0 {
		conditions = append(conditions, bson.M{"bdaEmail": bson.M{"$in": lowerAll(query.BdaEmails)}})
	}
	if len(query.CcStatuses) > 0 {
		conditions = append(conditions, bson.M{"cc.status": bson.M{"$in": query.CcStatuses}})
	}
	if query.Unassigned {
		conditions = append(conditions, bson.M{"assignment": nil})
	}
	if core.IsSet(query.Completed) {
		conditions = append(conditions, bson.M{"audit.completedAt": rangeFilter(query.Completed)})
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		pattern := bson.M{"$regex": regexp.QuoteMeta(search), "$options": "i"}
		conditions = append(conditions, bson.M{"$or": []bson.M{
			{"personal.name": pattern}, {"personal.email": pattern}, {"personal.phone": pattern}, {"zenId": pattern},
		}})
	}
	return withAnd(program, conditions)
}

func findLeads(ctx context.Context, program string, query models.LeadQuery) ([]models.Lead, int, error) {
	filter := leadFilter(program, query)
	opts := options.Find().SetSort(bson.D{{Key: "enrolledAt", Value: -1}, {Key: "crmCreatedAt", Value: -1}, {Key: "id", Value: 1}})
	if query.PageSize > 0 {
		opts.SetSkip(int64(max(query.Page-1, 0) * query.PageSize)).SetLimit(int64(query.PageSize))
	}
	leads, err := findAll[models.Lead](ctx, models.LeadsCollection, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	total := len(leads)
	if query.PageSize > 0 {
		count, err := collection(models.LeadsCollection).CountDocuments(ctx, filter)
		if err != nil {
			return nil, 0, err
		}
		total = int(count)
	}
	return leads, total, nil
}

func findLead(ctx context.Context, program, leadID string) (models.Lead, error) {
	return findOne[models.Lead](ctx, models.LeadsCollection, scoped(program, bson.M{"id": leadID}))
}

func findLeadByZenID(ctx context.Context, program, zenID string) (models.Lead, error) {
	return findOne[models.Lead](ctx, models.LeadsCollection, scoped(program, bson.M{"zenId": zenID}))
}

func insertLead(ctx context.Context, lead models.Lead) error {
	return insertOne(ctx, models.LeadsCollection, lead)
}

// updateLeadZoho writes only the Zoho-owned fields, so an import never overwrites the workflow.
func updateLeadZoho(ctx context.Context, lead models.Lead) error {
	return updateOne(ctx, models.LeadsCollection, scoped(lead.Program, bson.M{"id": lead.ID}), bson.M{
		"superleapId":        lead.SuperleapID,
		"region":             lead.Region,
		"salesFrom":          lead.SalesFrom,
		"stage":              lead.Stage,
		"zohoStatus":         lead.ZohoStatus,
		"personal":           lead.Personal,
		"course":             lead.Course,
		"payment":            lead.Payment,
		"admission":          lead.Admission,
		"termsAccepted":      lead.TermsAccepted,
		"marketing":          lead.Marketing,
		"bdaEmail":           lead.BdaEmail,
		"bdmEmail":           lead.BdmEmail,
		"onboardCoordinator": lead.OnboardCoordinator,
		"cc":                 lead.Cc,
		"crmCreatedAt":       lead.CrmCreatedAt,
		"enrolledAt":         lead.EnrolledAt,
		"zohoSyncedAt":       lead.ZohoSyncedAt,
	})
}

// updateLeadWorkflow writes the portal-owned fields.
func updateLeadWorkflow(ctx context.Context, lead models.Lead) error {
	return updateOne(ctx, models.LeadsCollection, scoped(lead.Program, bson.M{"id": lead.ID}), bson.M{
		"assignment":     lead.Assignment,
		"audit":          lead.Audit,
		"recheckSummary": lead.RecheckSummary,
	})
}

func setLeadMailMarks(ctx context.Context, program, leadID string, paymentVerificationMailedAt, lastEscalationAt int64) error {
	return updateOne(ctx, models.LeadsCollection, scoped(program, bson.M{"id": leadID}), bson.M{
		"paymentVerificationMailedAt": paymentVerificationMailedAt,
		"lastEscalationAt":            lastEscalationAt,
	})
}

// Rechecks

func recheckFilter(program string, query models.RecheckQuery) bson.M {
	conditions := scopeFilter(query.Scope, "auditorEmail")
	if query.LeadIDs != nil {
		conditions = append(conditions, bson.M{"leadId": bson.M{"$in": query.LeadIDs}})
	}
	if query.Status != "" {
		conditions = append(conditions, bson.M{"status": query.Status})
	}
	if len(query.Categories) > 0 {
		// reasons.category for rechecks with several reasons; category for older ones.
		conditions = append(conditions, bson.M{"$or": bson.A{
			bson.M{"reasons.category": bson.M{"$in": query.Categories}},
			bson.M{"category": bson.M{"$in": query.Categories}},
		}})
	}
	if len(query.AuditorEmails) > 0 {
		conditions = append(conditions, bson.M{"auditorEmail": bson.M{"$in": lowerAll(query.AuditorEmails)}})
	}
	if len(query.BdaEmails) > 0 {
		conditions = append(conditions, bson.M{"bdaEmail": bson.M{"$in": lowerAll(query.BdaEmails)}})
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		pattern := bson.M{"$regex": regexp.QuoteMeta(search), "$options": "i"}
		conditions = append(conditions, bson.M{"$or": []bson.M{
			{"recheckNo": pattern}, {"leadName": pattern}, {"zenId": pattern}, {"comments": pattern},
		}})
	}
	if core.IsSet(query.Raised) {
		conditions = append(conditions, bson.M{"raisedAt": rangeFilter(query.Raised)})
	}
	if core.IsSet(query.ClosedIn) {
		conditions = append(conditions, bson.M{"closed.at": rangeFilter(query.ClosedIn)})
	}
	if query.AwaitingReaudit {
		conditions = append(conditions, bson.M{"status": models.RecheckClosed, "reauditedAt": 0})
	}
	return withAnd(program, conditions)
}

func findRechecks(ctx context.Context, program string, query models.RecheckQuery) ([]models.Recheck, error) {
	return findAll[models.Recheck](ctx, models.RechecksCollection, recheckFilter(program, query),
		options.Find().SetSort(bson.D{{Key: "raisedAt", Value: -1}}))
}

func findRecheck(ctx context.Context, program, recheckID string) (models.Recheck, error) {
	return findOne[models.Recheck](ctx, models.RechecksCollection, scoped(program, bson.M{"id": recheckID}))
}

func findRecheckByNo(ctx context.Context, program, recheckNo string) (models.Recheck, error) {
	return findOne[models.Recheck](ctx, models.RechecksCollection, scoped(program, bson.M{"recheckNo": recheckNo}))
}

func insertRecheck(ctx context.Context, recheck models.Recheck) error {
	return insertOne(ctx, models.RechecksCollection, recheck)
}

func replaceRecheck(ctx context.Context, recheck models.Recheck) error {
	return replaceOne(ctx, models.RechecksCollection, scoped(recheck.Program, bson.M{"id": recheck.ID}), recheck)
}

// Audits, events, notifications, counters

func insertAudit(ctx context.Context, audit models.Audit) error {
	return insertOne(ctx, models.AuditsCollection, audit)
}

func findAudits(ctx context.Context, program string, query models.AuditQuery) ([]models.Audit, error) {
	filter := bson.M{}
	if query.LeadID != "" {
		filter["leadId"] = query.LeadID
	}
	if query.AuditorEmails != nil {
		filter["auditor.email"] = bson.M{"$in": lower(query.AuditorEmails)}
	}
	if core.IsSet(query.Submitted) {
		filter["submittedAt"] = rangeFilter(query.Submitted)
	}
	return findAll[models.Audit](ctx, models.AuditsCollection, scoped(program, filter),
		options.Find().SetSort(bson.D{{Key: "submittedAt", Value: -1}}))
}

func insertEvent(ctx context.Context, event models.Event) error {
	return insertOne(ctx, models.EventsCollection, event)
}

func findEvents(ctx context.Context, program, leadID string) ([]models.Event, error) {
	return findAll[models.Event](ctx, models.EventsCollection, scoped(program, bson.M{"leadId": leadID}),
		options.Find().SetSort(bson.D{{Key: "at", Value: 1}}))
}

func insertNotification(ctx context.Context, notification models.Notification) error {
	return insertOne(ctx, models.NotificationsCollection, notification)
}

func findNotifications(ctx context.Context, program, email string, unreadOnly bool, limit int) ([]models.Notification, int, error) {
	filter := bson.M{"recipientEmail": strings.ToLower(email)}
	if unreadOnly {
		filter["read"] = false
	}
	opts := options.Find().SetSort(bson.D{{Key: "created.at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	notifications, err := findAll[models.Notification](ctx, models.NotificationsCollection, scoped(program, filter), opts)
	if err != nil {
		return nil, 0, err
	}
	unread, err := collection(models.NotificationsCollection).CountDocuments(ctx,
		scoped(program, bson.M{"recipientEmail": strings.ToLower(email), "read": false}))
	return notifications, int(unread), err
}

func markNotificationRead(ctx context.Context, program, email, notificationID string, at int64) error {
	return updateOne(ctx, models.NotificationsCollection,
		scoped(program, bson.M{"id": notificationID, "recipientEmail": strings.ToLower(email)}),
		bson.M{"read": true, "readAt": at})
}

func markAllNotificationsRead(ctx context.Context, program, email string, at int64) error {
	_, err := collection(models.NotificationsCollection).UpdateMany(ctx,
		scoped(program, bson.M{"recipientEmail": strings.ToLower(email), "read": false}),
		bson.M{"$set": bson.M{"read": true, "readAt": at}})
	return err
}

func nextSequence(ctx context.Context, program, name string, now int64) (int64, error) {
	var counter models.Counter
	err := collection(models.CountersCollection).FindOneAndUpdate(ctx,
		scoped(program, bson.M{"name": name}),
		bson.M{
			"$inc":         bson.M{"value": 1},
			"$setOnInsert": bson.M{"id": core.NewID(), "created": models.Created{At: now, By: models.SystemUser}},
		},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&counter)
	return counter.Value, err
}

// Mail log and CC extracts

func insertAlert(ctx context.Context, alert models.Alert) error {
	return insertOne(ctx, models.AlertsCollection, alert)
}

func findAlert(ctx context.Context, program, alertID string) (models.Alert, error) {
	return findOne[models.Alert](ctx, models.AlertsCollection, scoped(program, bson.M{"id": alertID}))
}

func findAlerts(ctx context.Context, program string, leadIDs []string, kind string, since int64) ([]models.Alert, error) {
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
		options.Find().SetSort(bson.D{{Key: "sentAt", Value: -1}}))
}

func setAlertDelivery(ctx context.Context, program, alertID, delivery string) error {
	return updateOne(ctx, models.AlertsCollection, scoped(program, bson.M{"id": alertID}), bson.M{"delivery": delivery})
}

func findCcExtract(ctx context.Context, program, leadID string) (models.CcExtract, error) {
	return findOne[models.CcExtract](ctx, models.CcExtractsCollection, scoped(program, bson.M{"leadId": leadID}))
}

// saveCcExtract keeps one extract per lead, replacing an older one.
func saveCcExtract(ctx context.Context, extract models.CcExtract) error {
	_, err := collection(models.CcExtractsCollection).ReplaceOne(ctx,
		scoped(extract.Program, bson.M{"leadId": extract.LeadID}), extract, options.Replace().SetUpsert(true))
	return err
}
