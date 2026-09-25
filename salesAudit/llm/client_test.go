package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auditApp/config"
)

func serve(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	url, model, key := config.LLMAPIURL, config.LLMModel, config.LLMAPIKey
	t.Cleanup(func() { config.LLMAPIURL, config.LLMModel, config.LLMAPIKey = url, model, key })
	config.LLMAPIURL, config.LLMModel, config.LLMAPIKey = server.URL+"/v1/", "test-model", "secret"
}

func TestCompleteSendsAChatCompletion(t *testing.T) {
	var got request
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("request: %s %s auth %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"  All quiet.  "}}]}`))
	})
	text, err := Complete(context.Background(), "be brief", "figures")
	if err != nil || text != "All quiet." {
		t.Fatalf("reply: %q %v", text, err)
	}
	if got.Model != "test-model" || len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[1].Content != "figures" {
		t.Fatalf("body: %+v", got)
	}
}

func TestCompleteReportsTheAPIError(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	if _, err := Complete(context.Background(), "s", "u"); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("error: %v", err)
	}
}

func TestCompleteNeedsConfiguration(t *testing.T) {
	url := config.LLMAPIURL
	t.Cleanup(func() { config.LLMAPIURL = url })
	config.LLMAPIURL = ""
	if _, err := Complete(context.Background(), "s", "u"); err != ErrNotConfigured {
		t.Fatalf("error: %v", err)
	}
}
