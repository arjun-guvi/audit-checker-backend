package worker

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// Zoho exports dates in several layouts; all are read as IST unless they carry a zone.
var zohoLayouts = []string{
	"02-Jan-2006 15:04:05",
	"02-Jan-2006",
	"2006-01-02",
	"2006-01-02 15:04:05",
	"01/02/06 03:04:05 PM",
}

var zohoLocation = time.FixedZone("IST", 5*60*60+30*60)

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
		if parsed, err := time.ParseInLocation(layout, value, zohoLocation); err == nil {
			return parsed.Unix(), true
		}
	}
	return 0, false
}

// OptionalTime is ParseZohoTime as a pointer, nil when there is no date.
func OptionalTime(value string) *int64 {
	if seconds, ok := ParseZohoTime(value); ok {
		return &seconds
	}
	return nil
}

// DisplayDate normalises a Zoho date to DD-Mon-YYYY; unreadable values are returned as they are.
func DisplayDate(value string) string {
	seconds, ok := ParseZohoTime(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return time.Unix(seconds, 0).In(zohoLocation).Format("02-Jan-2006")
}

// Contacts are stored as "Name - email".
func ContactEmail(contact string) string {
	parts := strings.Split(contact, " - ")
	return strings.TrimSpace(parts[len(parts)-1])
}

func ContactName(contact string) string {
	return strings.TrimSpace(strings.Split(contact, " - ")[0])
}

// Rupees formats an amount as the frontend and the CC mail write it: "₹18000" for "18000.00".
func Rupees(amount string) string {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return ""
	}
	amount = strings.TrimPrefix(amount, "₹")
	if whole, fraction, found := strings.Cut(amount, "."); found && strings.Trim(fraction, "0") == "" {
		amount = whole
	}
	return "₹" + amount
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
