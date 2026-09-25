// Package cc reads a lead's confirmation call (PDF or call recording) into fields the audit
// compares and, for recordings, a transcript. The real reader (the Python FastAPI service, called
// from here) is out of scope for now: Extract is a mock that replays the lead's own data with a
// deterministic mismatch on some leads, so the audit screens can be built and tested.
package cc

import (
	"fmt"
	"hash/fnv"
	"strings"

	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
)

// Extract reads the lead's CC. It is a variable so the FastAPI client can replace it.
var Extract = mockExtract

// Points the CC must cover; the mock marks all but one covered on some leads.
var mandatoryPoints = []string{
	"Course name and duration",
	"Batch start date and timing",
	"Course fee and discount",
	"Payment plan and due dates",
	"Refund policy",
	"Placement assistance terms",
}

func mockExtract(lead models.Lead, now int64) models.CcExtract {
	seed := hash(lead.ZenID + lead.Cc.Link)
	fields := []models.CcField{}
	for _, row := range core.DbFields(lead) {
		fields = append(fields, models.CcField{Key: row.Key, Value: row.DbValue})
	}
	// About one lead in four gets a mismatch, on a field picked by the seed.
	if seed%4 == 0 && len(fields) > 0 {
		index := int(seed/4) % len(fields)
		fields[index].Value = mismatched(fields[index])
	}
	points := append([]string(nil), mandatoryPoints...)
	if seed%5 == 0 {
		points = points[:len(points)-1]
	}

	extract := models.CcExtract{
		ID:            core.NewID(),
		Program:       lead.Program,
		LeadID:        lead.ID,
		Link:          lead.Cc.Link,
		Type:          lead.Cc.Type,
		Fields:        fields,
		Transcript:    []models.Line{},
		PointsCovered: points,
		Mocked:        true,
		ExtractedAt:   now,
		Created:       models.Created{At: now, By: models.SystemUser},
	}
	if lead.Cc.Type == models.CcTypeRecording || lead.Cc.Type == models.CcTypeLink {
		extract.Transcript = transcript(lead, fields)
	}
	return extract
}

func mismatched(field models.CcField) string {
	switch field.Key {
	case "courseFee", "downPayment", "discount", "loanAmount":
		return core.Rupees(core.Amount(strings.TrimPrefix(field.Value, "₹")) + 5000)
	case "phone":
		return "+910000000000"
	case "emiTenure":
		return "24"
	default:
		return field.Value + " (as told on call)"
	}
}

func transcript(lead models.Lead, fields []models.CcField) []models.Line {
	value := map[string]string{}
	for _, field := range fields {
		value[field.Key] = field.Value
	}
	say := func(at, speaker, text string) models.Line { return models.Line{At: at, Speaker: speaker, Text: text} }
	name := core.FirstNonEmpty(value["name"], "the learner")
	return []models.Line{
		say("00:00", "Executive", fmt.Sprintf("Hi, am I speaking with %s? This is the confirmation call for your enrolment.", name)),
		say("00:06", "Learner", "Yes, that's me."),
		say("00:10", "Executive", fmt.Sprintf("Let me confirm your details. Your email is %s and phone number %s. Correct?", value["email"], value["phone"])),
		say("00:18", "Learner", "Yes, correct."),
		say("00:22", "Executive", fmt.Sprintf("You have enrolled for %s, batch %s, starting %s, %s, in %s.",
			value["product"], value["batchName"], value["batchStartDate"], value["modeOfStudy"], value["language"])),
		say("00:35", "Learner", "Okay."),
		say("00:38", "Executive", fmt.Sprintf("The course fee is %s with a %s payment plan.", value["courseFee"], value["paymentType"])),
		say("00:46", "Executive", "I'll also go over the refund policy and placement assistance terms."),
		say("01:30", "Learner", "Understood, thank you."),
		say("01:34", "Executive", "Thank you. You'll get the onboarding mail shortly. (Mock transcript: the transcription service is not connected yet.)"),
	}
}

func hash(value string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(value))
	return h.Sum32()
}
