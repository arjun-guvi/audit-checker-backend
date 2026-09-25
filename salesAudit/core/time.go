package core

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// IST is the zone Zoho dates are read in and date presets are computed in.
var IST = time.FixedZone("IST", 5*60*60+30*60)

// Zoho exports dates in several layouts; all are read as IST unless they carry a zone.
var zohoLayouts = []string{
	"2006-01-02 15:04:05.0",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"02-Jan-2006 15:04:05",
	"02-Jan-2006",
	"01/02/06 03:04:05 PM",
	"01/02/2006 03:04:05 PM",
}

// ParseZohoTime turns a Zoho date string into Unix seconds; ok is false when it is empty or
// unreadable.
func ParseZohoTime(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.Unix(), true
	}
	for _, layout := range zohoLayouts {
		if parsed, err := time.ParseInLocation(layout, value, IST); err == nil {
			return parsed.Unix(), true
		}
	}
	return 0, false
}

// ZohoSeconds is ParseZohoTime with 0 for no date.
func ZohoSeconds(value string) int64 {
	seconds, _ := ParseZohoTime(value)
	return seconds
}

// DisplayDate normalises a Zoho date to DD-Mon-YYYY; unreadable values are returned as they are.
func DisplayDate(value string) string {
	seconds, ok := ParseZohoTime(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return time.Unix(seconds, 0).In(IST).Format("02-Jan-2006")
}

// FormatTime writes Unix seconds as "02-Jan-2006 03:04 PM" IST.
func FormatTime(seconds int64) string {
	if seconds == 0 {
		return ""
	}
	return time.Unix(seconds, 0).In(IST).Format("02-Jan-2006 03:04 PM")
}

// DayKey is the IST calendar day of Unix seconds, as 2006-01-02.
func DayKey(seconds int64) string {
	return time.Unix(seconds, 0).In(IST).Format("2006-01-02")
}

// Amount reads a Zoho number ("73800", "73800.00", "") as a float; 0 when empty or unreadable.
func Amount(value string) float64 {
	value = strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), "₹")
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return number
}

// Rupees formats an amount as "₹73800" (or "₹73800.5").
func Rupees(amount float64) string {
	return "₹" + strconv.FormatFloat(amount, 'f', -1, 64)
}

// NewID returns a random (version 4) UUID.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RecheckNo formats a portal recheck's sequence number: RC-000123.
func RecheckNo(sequence int64) string {
	return fmt.Sprintf("RC-%06d", sequence)
}

// NormalizeEmail lower-cases and trims an email; Zoho sometimes sends "Name - email".
func NormalizeEmail(value string) string {
	parts := strings.Split(value, " - ")
	return strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
}

// FirstNonEmpty returns the first value that is not blank.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
