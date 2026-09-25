// Package zoho fetches learners from the Zoho Creator learner API (ZOHO_API_URL) with resty.
package zoho

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"auditApp/config"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"

	"github.com/go-resty/resty/v2"
)

var client = resty.New().SetTimeout(2 * time.Minute)

// Fetch returns the learners enrolled in the sync window. It is a variable so tests and the
// dev import can replace it.
var Fetch = fetch

// Window is the date range to fetch: ZOHO_API_FROM / ZOHO_API_TO when both are set, else the last
// ZOHO_SYNC_LOOKBACK_DAYS days (default 3) up to today, in Zoho's DD-Mon-YYYY.
func Window(now time.Time) (from, to string) {
	fromEnv, toEnv := config.GetEnv("ZOHO_API_FROM", ""), config.GetEnv("ZOHO_API_TO", "")
	if fromEnv != "" && toEnv != "" {
		return fromEnv, toEnv
	}
	days, err := strconv.Atoi(config.GetEnv("ZOHO_SYNC_LOOKBACK_DAYS", "3"))
	if err != nil || days < 1 {
		days = 3
	}
	today := now.In(core.IST)
	return today.AddDate(0, 0, -days).Format(core.ZohoDay), today.Format(core.ZohoDay)
}

func fetch(ctx context.Context, from, to string) ([]models.ZohoLearner, error) {
	if config.ZohoAPIPublicKey == "" {
		return nil, errors.New("ZOHO_API_PUBLIC_KEY is not configured")
	}
	response, err := client.R().SetContext(ctx).
		SetQueryParams(map[string]string{"publickey": config.ZohoAPIPublicKey, "from": from, "to": to}).
		Get(config.ZohoAPIURL)
	if err != nil {
		return nil, fmt.Errorf("fetch Zoho learners: %w", err)
	}
	if response.IsError() {
		return nil, fmt.Errorf("Zoho returned %s", response.Status())
	}
	return Decode(response.Body())
}

// Decode reads a Zoho response: {"result": [...]}, {"data": [...]}, a bare list or one learner.
// A malformed record is skipped rather than failing the batch.
func Decode(body []byte) ([]models.ZohoLearner, error) {
	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode Zoho response: %w", err)
	}
	return decode(raw)
}

func decode(raw json.RawMessage) ([]models.ZohoLearner, error) {
	if len(raw) > 0 && raw[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("decode Zoho learners: %w", err)
		}
		learners := make([]models.ZohoLearner, 0, len(items))
		for index, item := range items {
			var learner models.ZohoLearner
			if err := json.Unmarshal(item, &learner); err != nil {
				log.Printf("zoho: skipping malformed learner at index %d: %v", index, err)
				continue
			}
			learners = append(learners, learner)
		}
		return learners, nil
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil {
		for _, inner := range []json.RawMessage{envelope.Result, envelope.Data} {
			if len(inner) > 0 && string(inner) != "null" {
				return decode(inner)
			}
		}
	}
	var learner models.ZohoLearner
	if err := json.Unmarshal(raw, &learner); err != nil || learner.ZenID == "" {
		return nil, errors.New("decode Zoho learner: no recognizable learner fields in payload")
	}
	return []models.ZohoLearner{learner}, nil
}
