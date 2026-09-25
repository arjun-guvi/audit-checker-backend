package core

import (
	"errors"
	"strconv"
	"time"

	"auditApp/salesAudit/models"
)

// Date presets for filters, computed in IST. Weeks start on Monday.
const (
	PresetToday     = "today"
	PresetThisWeek  = "thisWeek"
	PresetLastWeek  = "lastWeek"
	PresetThisMonth = "thisMonth"
	PresetLastMonth = "lastMonth"
)

var ErrBadRange = errors.New("Date filters take today, thisWeek, lastWeek, thisMonth, lastMonth or from/to in Unix seconds")

// PresetRange is the [from, to) window of a preset at now.
func PresetRange(preset string, now time.Time) (models.Range, bool) {
	local := now.In(IST)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, IST)
	weekStart := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, IST)
	span := func(from, to time.Time) models.Range { return models.Range{From: from.Unix(), To: to.Unix()} }
	switch preset {
	case PresetToday:
		return span(day, day.AddDate(0, 0, 1)), true
	case PresetThisWeek:
		return span(weekStart, weekStart.AddDate(0, 0, 7)), true
	case PresetLastWeek:
		return span(weekStart.AddDate(0, 0, -7), weekStart), true
	case PresetThisMonth:
		return span(monthStart, monthStart.AddDate(0, 1, 0)), true
	case PresetLastMonth:
		return span(monthStart.AddDate(0, -1, 0), monthStart), true
	}
	return models.Range{}, false
}

// ParseRange reads a date filter: a preset, or from/to Unix seconds. ok is false when neither
// was given.
func ParseRange(preset, from, to string, now time.Time) (models.Range, bool, error) {
	if preset != "" {
		span, known := PresetRange(preset, now)
		if !known {
			return span, false, ErrBadRange
		}
		return span, true, nil
	}
	if from == "" && to == "" {
		return models.Range{}, false, nil
	}
	span := models.Range{}
	var err error
	if from != "" {
		if span.From, err = strconv.ParseInt(from, 10, 64); err != nil {
			return span, false, ErrBadRange
		}
	}
	if to != "" {
		if span.To, err = strconv.ParseInt(to, 10, 64); err != nil {
			return span, false, ErrBadRange
		}
	}
	return span, true, nil
}

// InRange reports whether seconds falls in the window; an unset (zero) time never does.
func InRange(span models.Range, seconds int64) bool {
	if seconds == 0 {
		return false
	}
	return (span.From == 0 || seconds >= span.From) && (span.To == 0 || seconds < span.To)
}

// IsSet reports whether the window has a bound.
func IsSet(span models.Range) bool {
	return span.From != 0 || span.To != 0
}

// ZohoDay is how the Zoho learner API takes from/to dates.
const ZohoDay = "02-Jan-2006"

// ZohoChunks splits the IST days from..to (both inclusive) into windows of days, oldest first,
// as Zoho from/to pairs: 1-Sep..30-Sep by 5 is 01-Sep..05-Sep, 06-Sep..10-Sep, …, 26-Sep..30-Sep.
func ZohoChunks(from, to time.Time, days int) [][2]string {
	if days < 1 {
		days = 1
	}
	start := dayOf(from)
	end := dayOf(to)
	chunks := [][2]string{}
	for !start.After(end) {
		last := start.AddDate(0, 0, days-1)
		if last.After(end) {
			last = end
		}
		chunks = append(chunks, [2]string{start.Format(ZohoDay), last.Format(ZohoDay)})
		start = last.AddDate(0, 0, 1)
	}
	return chunks
}

func dayOf(value time.Time) time.Time {
	local := value.In(IST)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, IST)
}
