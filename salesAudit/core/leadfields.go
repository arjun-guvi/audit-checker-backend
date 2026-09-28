package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"auditApp/salesAudit/models"
)

// The lead list's filter sidebar: every filterable column, its kind, and the operators it takes.
// Filters arrive as models.FieldFilter and are compiled into models.FieldCondition, which the
// Mongo store (fieldCondition) and the fake store (MatchLeadConditions) both match.

// Field kinds.
const (
	KindText   = "text"
	KindEnum   = "enum"
	KindDate   = "date"
	KindNumber = "number"
)

// Filter operators as the client sends them.
const (
	OpIs          = "is"
	OpIsNot       = "isNot"
	OpIsEmpty     = "isEmpty"
	OpIsNotEmpty  = "isNotEmpty"
	OpStartsWith  = "startsWith"
	OpEndsWith    = "endsWith"
	OpContains    = "contains"
	OpNotContains = "notContains"
	OpBefore      = "before"
	OpAfter       = "after"
	OpBetween     = "between"
	OpLessThan    = "lessThan"
	OpGreaterThan = "greaterThan"
	OpYesterday   = "yesterday"
	OpToday       = "today"
	OpTomorrow    = "tomorrow"
	OpLastWeek    = "lastWeek"
	OpThisWeek    = "thisWeek"
	OpNextWeek    = "nextWeek"
	OpLastMonth   = "lastMonth"
	OpThisMonth   = "thisMonth"
	OpNextMonth   = "nextMonth"
	OpLastYear    = "lastYear"
	OpThisYear    = "thisYear"
	OpNextYear    = "nextYear"
)

// Compiled operators (models.FieldCondition.Op). Text conditions keep the text operator.
const (
	CondIn       = "in"
	CondNotIn    = "notIn"
	CondRange    = "range"
	CondNotRange = "notRange"
)

// Operators each kind takes.
var (
	textOps   = []string{OpIs, OpIsNot, OpIsEmpty, OpIsNotEmpty, OpStartsWith, OpEndsWith, OpContains, OpNotContains}
	enumOps   = []string{OpIs, OpIsNot, OpIsEmpty, OpIsNotEmpty}
	numberOps = []string{OpIs, OpIsNot, OpIsEmpty, OpIsNotEmpty, OpLessThan, OpGreaterThan, OpBetween}
	dateOps   = []string{OpIs, OpIsNot, OpIsEmpty, OpIsNotEmpty, OpBefore, OpAfter, OpBetween,
		OpYesterday, OpToday, OpTomorrow, OpLastWeek, OpThisWeek, OpNextWeek,
		OpLastMonth, OpThisMonth, OpNextMonth, OpLastYear, OpThisYear, OpNextYear}
)

// LeadField is a filterable field of a lead: its Mongo path and how to read it in memory (Texts
// for text and enum fields, Number for dates and numbers).
type LeadField struct {
	Kind   string
	Path   string
	Texts  func(models.Lead) []string
	Number func(models.Lead) int64
}

// RecheckField is a filterable field of a lead's rechecks; a lead matches when one of its
// rechecks matches every recheck rule.
type RecheckField struct {
	Kind   string
	Path   string
	Texts  func(models.Recheck) []string
	Number func(models.Recheck) int64
}

func leadText(read func(models.Lead) string) func(models.Lead) []string {
	return func(lead models.Lead) []string { return []string{read(lead)} }
}

func recheckText(read func(models.Recheck) string) func(models.Recheck) []string {
	return func(recheck models.Recheck) []string { return []string{read(recheck)} }
}

func assignmentOf(lead models.Lead) models.Assignment {
	if lead.Assignment == nil {
		return models.Assignment{}
	}
	return *lead.Assignment
}

// LeadFields are the lead columns that can be filtered, by the key the client sends.
var LeadFields = map[string]LeadField{
	"name":               {Kind: KindText, Path: "personal.name", Texts: leadText(func(l models.Lead) string { return l.Personal.Name })},
	"zenId":              {Kind: KindText, Path: "zenId", Texts: leadText(func(l models.Lead) string { return l.ZenID })},
	"product":            {Kind: KindText, Path: "course.product", Texts: leadText(func(l models.Lead) string { return l.Course.Product })},
	"batch":              {Kind: KindText, Path: "course.batch.name", Texts: leadText(func(l models.Lead) string { return l.Course.Batch.Name })},
	"region":             {Kind: KindEnum, Path: "region", Texts: leadText(func(l models.Lead) string { return l.Region })},
	"salesFrom":          {Kind: KindText, Path: "salesFrom", Texts: leadText(func(l models.Lead) string { return l.SalesFrom })},
	"auditorEmail":       {Kind: KindText, Path: "assignment.auditorEmail", Texts: leadText(func(l models.Lead) string { return assignmentOf(l).AuditorEmail })},
	"bdaEmail":           {Kind: KindText, Path: "bdaEmail", Texts: leadText(func(l models.Lead) string { return l.BdaEmail })},
	"onboardCoordinator": {Kind: KindText, Path: "onboardCoordinator", Texts: leadText(func(l models.Lead) string { return l.OnboardCoordinator })},
	"paymentType":        {Kind: KindText, Path: "payment.paymentType", Texts: leadText(func(l models.Lead) string { return l.Payment.PaymentType })},
	"ccLink":             {Kind: KindText, Path: "cc.link", Texts: leadText(func(l models.Lead) string { return l.Cc.Link })},
	"ccStatus":           {Kind: KindEnum, Path: "cc.status", Texts: leadText(func(l models.Lead) string { return l.Cc.Status })},
	"auditStatus":        {Kind: KindEnum, Path: "audit.status", Texts: leadText(func(l models.Lead) string { return l.Audit.Status })},
	"enrolledAt":         {Kind: KindDate, Path: "enrolledAt", Number: func(l models.Lead) int64 { return l.EnrolledAt }},
	"onboardingAt":       {Kind: KindDate, Path: "course.onboardingAt", Number: func(l models.Lead) int64 { return l.Course.OnboardingAt }},
	"awaitingAt":         {Kind: KindDate, Path: "course.awaitingAt", Number: func(l models.Lead) int64 { return l.Course.AwaitingAt }},
	"assignedAt":         {Kind: KindDate, Path: "assignment.assignedAt", Number: func(l models.Lead) int64 { return assignmentOf(l).AssignedAt }},
	"completedAt":        {Kind: KindDate, Path: "audit.completedAt", Number: func(l models.Lead) int64 { return l.Audit.CompletedAt }},
}

func closedAtOf(recheck models.Recheck) int64 {
	if recheck.Closed == nil {
		return 0
	}
	return recheck.Closed.At
}

// RecheckFields are the recheck columns that can be filtered.
var RecheckFields = map[string]RecheckField{
	"recheckNo":       {Kind: KindText, Path: "recheckNo", Texts: recheckText(func(r models.Recheck) string { return r.RecheckNo })},
	"recheckComments": {Kind: KindText, Path: "comments", Texts: recheckText(func(r models.Recheck) string { return r.Comments })},
	"recheckCategory": {Kind: KindEnum, Path: "category", Texts: recheckText(func(r models.Recheck) string { return r.Category })},
	"recheckReason":   {Kind: KindEnum, Path: "reasons.category", Texts: func(r models.Recheck) []string { return ReasonCategories(ReasonsOf(r)) }},
	"recheckRaisedBy": {Kind: KindText, Path: "raisedBy.email", Texts: recheckText(func(r models.Recheck) string { return r.RaisedBy.Email })},
	"recheckStatus":   {Kind: KindEnum, Path: "status", Texts: recheckText(func(r models.Recheck) string { return r.Status })},
	"recheckAttempt":  {Kind: KindNumber, Path: "attempt", Number: func(r models.Recheck) int64 { return int64(r.Attempt) }},
	"recheckRaisedAt": {Kind: KindDate, Path: "raisedAt", Number: func(r models.Recheck) int64 { return r.RaisedAt }},
	"recheckClosedAt": {Kind: KindDate, Path: "closed.at", Number: closedAtOf},
}

// ErrBadFilters is a filters parameter that is not a JSON list of {field, op, value, value2}.
var ErrBadFilters = errors.New("filters must be a JSON list of {field, op, value, value2}")

// CompileFilters resolves the sidebar's rules at now into conditions on lead fields and on
// recheck fields. An unknown field, an operator the field's kind does not take, or a missing or
// malformed value is an error.
func CompileFilters(filters []models.FieldFilter, now time.Time) (leads, rechecks []models.FieldCondition, err error) {
	leads, rechecks = []models.FieldCondition{}, []models.FieldCondition{}
	for _, filter := range filters {
		if field, ok := LeadFields[filter.Field]; ok {
			condition, err := compile(filter, field.Kind, field.Path, now)
			if err != nil {
				return nil, nil, err
			}
			leads = append(leads, condition)
			continue
		}
		if field, ok := RecheckFields[filter.Field]; ok {
			condition, err := compile(filter, field.Kind, field.Path, now)
			if err != nil {
				return nil, nil, err
			}
			rechecks = append(rechecks, condition)
			continue
		}
		return nil, nil, fmt.Errorf("Unknown filter field %q", filter.Field)
	}
	return leads, rechecks, nil
}

func opsOf(kind string) []string {
	switch kind {
	case KindText:
		return textOps
	case KindEnum:
		return enumOps
	case KindNumber:
		return numberOps
	}
	return dateOps
}

func compile(filter models.FieldFilter, kind, path string, now time.Time) (models.FieldCondition, error) {
	condition := models.FieldCondition{Field: filter.Field, Path: path, Kind: kind}
	if !contains(opsOf(kind), filter.Op) {
		return condition, fmt.Errorf("Filter %q does not take the operator %q", filter.Field, filter.Op)
	}
	value := strings.TrimSpace(filter.Value)
	if filter.Op == OpIsEmpty || filter.Op == OpIsNotEmpty {
		condition.Op = filter.Op
		return condition, nil
	}
	switch kind {
	case KindText:
		if value == "" {
			return condition, fmt.Errorf("Filter %q needs a value", filter.Field)
		}
		condition.Op, condition.Value = filter.Op, value
		return condition, nil
	case KindEnum:
		condition.Values = splitValues(value)
		if len(condition.Values) == 0 {
			return condition, fmt.Errorf("Filter %q needs a value", filter.Field)
		}
		condition.Op = CondIn
		if filter.Op == OpIsNot {
			condition.Op = CondNotIn
		}
		return condition, nil
	case KindNumber:
		return compileNumber(condition, filter)
	}
	return compileDate(condition, filter, now)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func splitValues(value string) []string {
	values := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func withRange(condition models.FieldCondition, op string, from, to int64, hasFrom, hasTo bool) models.FieldCondition {
	condition.Op, condition.From, condition.To, condition.HasFrom, condition.HasTo = op, from, to, hasFrom, hasTo
	return condition
}

func compileNumber(condition models.FieldCondition, filter models.FieldFilter) (models.FieldCondition, error) {
	first, err := strconv.ParseInt(strings.TrimSpace(filter.Value), 10, 64)
	if err != nil {
		return condition, fmt.Errorf("Filter %q needs a whole number", filter.Field)
	}
	switch filter.Op {
	case OpIs:
		return withRange(condition, CondRange, first, first+1, true, true), nil
	case OpIsNot:
		return withRange(condition, CondNotRange, first, first+1, true, true), nil
	case OpLessThan:
		return withRange(condition, CondRange, 0, first, false, true), nil
	case OpGreaterThan:
		return withRange(condition, CondRange, first+1, 0, true, false), nil
	}
	second, err := strconv.ParseInt(strings.TrimSpace(filter.Value2), 10, 64)
	if err != nil {
		return condition, fmt.Errorf("Filter %q needs two whole numbers", filter.Field)
	}
	return withRange(condition, CondRange, min(first, second), max(first, second)+1, true, true), nil
}

// FilterDay is how date filter values are written: an IST day.
const FilterDay = "2006-01-02"

func parseDay(value string) (time.Time, bool) {
	day, err := time.ParseInLocation(FilterDay, strings.TrimSpace(value), IST)
	return day, err == nil
}

// relativeRange is the [from, to) window of a relative date operator at now, in IST. Weeks start
// on Monday.
func relativeRange(op string, now time.Time) (time.Time, time.Time, bool) {
	local := now.In(IST)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, IST)
	week := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, IST)
	year := time.Date(local.Year(), 1, 1, 0, 0, 0, 0, IST)
	switch op {
	case OpYesterday:
		return day.AddDate(0, 0, -1), day, true
	case OpToday:
		return day, day.AddDate(0, 0, 1), true
	case OpTomorrow:
		return day.AddDate(0, 0, 1), day.AddDate(0, 0, 2), true
	case OpLastWeek:
		return week.AddDate(0, 0, -7), week, true
	case OpThisWeek:
		return week, week.AddDate(0, 0, 7), true
	case OpNextWeek:
		return week.AddDate(0, 0, 7), week.AddDate(0, 0, 14), true
	case OpLastMonth:
		return month.AddDate(0, -1, 0), month, true
	case OpThisMonth:
		return month, month.AddDate(0, 1, 0), true
	case OpNextMonth:
		return month.AddDate(0, 1, 0), month.AddDate(0, 2, 0), true
	case OpLastYear:
		return year.AddDate(-1, 0, 0), year, true
	case OpThisYear:
		return year, year.AddDate(1, 0, 0), true
	case OpNextYear:
		return year.AddDate(1, 0, 0), year.AddDate(2, 0, 0), true
	}
	return time.Time{}, time.Time{}, false
}

func compileDate(condition models.FieldCondition, filter models.FieldFilter, now time.Time) (models.FieldCondition, error) {
	if from, to, ok := relativeRange(filter.Op, now); ok {
		return withRange(condition, CondRange, from.Unix(), to.Unix(), true, true), nil
	}
	first, ok := parseDay(filter.Value)
	if !ok {
		return condition, fmt.Errorf("Filter %q needs a date (YYYY-MM-DD)", filter.Field)
	}
	next := first.AddDate(0, 0, 1)
	switch filter.Op {
	case OpIs:
		return withRange(condition, CondRange, first.Unix(), next.Unix(), true, true), nil
	case OpIsNot:
		return withRange(condition, CondNotRange, first.Unix(), next.Unix(), true, true), nil
	case OpBefore:
		return withRange(condition, CondRange, 0, first.Unix(), false, true), nil
	case OpAfter:
		return withRange(condition, CondRange, next.Unix(), 0, true, false), nil
	}
	second, ok := parseDay(filter.Value2)
	if !ok {
		return condition, fmt.Errorf("Filter %q needs two dates (YYYY-MM-DD)", filter.Field)
	}
	if second.Before(first) {
		first, second = second, first
	}
	return withRange(condition, CondRange, first.Unix(), second.AddDate(0, 0, 1).Unix(), true, true), nil
}

// MatchTexts reports whether text values pass a condition, ignoring case: positive operators need
// one value to match, negative ones (is not, not contains, not in) need none to.
func MatchTexts(condition models.FieldCondition, values []string) bool {
	present := []string{}
	for _, value := range values {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			present = append(present, value)
		}
	}
	term := strings.ToLower(condition.Value)
	anyValue := func(test func(string) bool) bool {
		for _, value := range present {
			if test(value) {
				return true
			}
		}
		return false
	}
	switch condition.Op {
	case OpIsEmpty:
		return len(present) == 0
	case OpIsNotEmpty:
		return len(present) > 0
	case OpIs:
		return anyValue(func(v string) bool { return v == term })
	case OpIsNot:
		return !anyValue(func(v string) bool { return v == term })
	case OpStartsWith:
		return anyValue(func(v string) bool { return strings.HasPrefix(v, term) })
	case OpEndsWith:
		return anyValue(func(v string) bool { return strings.HasSuffix(v, term) })
	case OpContains:
		return anyValue(func(v string) bool { return strings.Contains(v, term) })
	case OpNotContains:
		return !anyValue(func(v string) bool { return strings.Contains(v, term) })
	case CondIn:
		return anyValue(func(v string) bool { return containsFold(condition.Values, v) })
	case CondNotIn:
		return !anyValue(func(v string) bool { return containsFold(condition.Values, v) })
	}
	return false
}

// MatchNumber reports whether a date (Unix seconds) or number passes a condition. 0 is empty, and
// an empty value is neither in nor out of a range.
func MatchNumber(condition models.FieldCondition, value int64) bool {
	empty := value == 0
	inRange := (!condition.HasFrom || value >= condition.From) && (!condition.HasTo || value < condition.To)
	switch condition.Op {
	case OpIsEmpty:
		return empty
	case OpIsNotEmpty:
		return !empty
	case CondRange:
		return !empty && inRange
	case CondNotRange:
		return !empty && !inRange
	}
	return false
}

// MatchLeadConditions reports whether a lead passes every lead condition.
func MatchLeadConditions(conditions []models.FieldCondition, lead models.Lead) bool {
	for _, condition := range conditions {
		field := LeadFields[condition.Field]
		if field.Number != nil {
			if !MatchNumber(condition, field.Number(lead)) {
				return false
			}
		} else if field.Texts == nil || !MatchTexts(condition, field.Texts(lead)) {
			return false
		}
	}
	return true
}

// MatchRecheckConditions reports whether a recheck passes every recheck condition.
func MatchRecheckConditions(conditions []models.FieldCondition, recheck models.Recheck) bool {
	for _, condition := range conditions {
		field := RecheckFields[condition.Field]
		if field.Number != nil {
			if !MatchNumber(condition, field.Number(recheck)) {
				return false
			}
		} else if field.Texts == nil || !MatchTexts(condition, field.Texts(recheck)) {
			return false
		}
	}
	return true
}
