// Package llm asks an OpenAI-compatible chat completions API (LLM_API_URL, LLM_MODEL,
// LLM_API_KEY) for text, with resty.
package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"auditApp/config"

	"github.com/go-resty/resty/v2"
)

var client = resty.New().SetTimeout(60 * time.Second)

// ErrNotConfigured: LLM_API_URL or LLM_MODEL is empty.
var ErrNotConfigured = errors.New("LLM_API_URL and LLM_MODEL are not configured")

// Complete sends a system and a user message and returns the reply's text. It is a variable so
// tests can replace it.
var Complete = complete

// Configured reports whether the API is set up.
func Configured() bool {
	return config.LLMAPIURL != "" && config.LLMModel != ""
}

// Model is the configured model name.
func Model() string {
	return config.LLMModel
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type response struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// endpoint is LLM_API_URL + /chat/completions, unless the URL already is the full route.
func endpoint() string {
	url := strings.TrimRight(config.LLMAPIURL, "/")
	if strings.HasSuffix(url, "/chat/completions") {
		return url
	}
	return url + "/chat/completions"
}

func complete(ctx context.Context, system, user string) (string, error) {
	if !Configured() {
		return "", ErrNotConfigured
	}
	var result response
	call := client.R().SetContext(ctx).
		SetBody(request{
			Model:       config.LLMModel,
			Messages:    []message{{Role: "system", Content: system}, {Role: "user", Content: user}},
			Temperature: 0.2,
			MaxTokens:   400,
		}).
		SetResult(&result).
		SetError(&result)
	if config.LLMAPIKey != "" {
		call.SetAuthToken(config.LLMAPIKey)
	}
	reply, err := call.Post(endpoint())
	if err != nil {
		return "", fmt.Errorf("call the LLM API: %w", err)
	}
	if reply.IsError() {
		if result.Error != nil && result.Error.Message != "" {
			return "", fmt.Errorf("the LLM API returned %s: %s", reply.Status(), result.Error.Message)
		}
		return "", fmt.Errorf("the LLM API returned %s", reply.Status())
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", errors.New("the LLM API returned no text")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
