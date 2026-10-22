package openai

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/decide"
)

func TestDecide(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{
			"answers": [
				{"type": "score", "name": "sev", "score": 1.4,
				 "probabilities": [{"value": 0, "label": "low", "probability": 0.2}, {"value": 1, "label": "high", "probability": 0.8}],
				 "confidence": 0.6},
				{"type": "choice", "name": "queue", "choice": "tech",
				 "probabilities": [{"value": "tech", "probability": 0.9}, {"value": "billing", "probability": 0.1}],
				 "confidence": 0.88},
				{"type": "predicate", "name": "spam", "probability": 0.05},
				{"type": "refusal", "name": "nsfw"}
			],
			"usage": {"input_tokens": 120, "input_tokens_details": {"cached_tokens": 100, "cache_write_tokens": 0}}
		}`))
	}))
	defer srv.Close()

	d := New("key", WithBaseURL(srv.URL))
	resp, err := d.Decide(context.Background(), &decide.Request{
		Input: "Export fails in Safari",
		Questions: []decide.Question{
			{Name: "spam", Kind: decide.Predicate, Instructions: "Spam?", YesWhen: "ads"},
			decide.NewChoice("queue", "Team?", decide.Option{Value: "billing"}, decide.Option{Value: "tech", Description: "bugs"}),
			decide.NewScore("sev", "Severity?", decide.Level{Label: "low"}, decide.Level{Label: "high"}),
			decide.NewPredicate("nsfw", "NSFW?"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got["model"] != DefaultModel || got["input"] != "Export fails in Safari" {
		t.Errorf("request = %v", got)
	}
	qs := got["questions"].([]any)
	spam := qs[0].(map[string]any)
	if spam["type"] != "predicate" || spam["instructions"] != "Spam?\nAnswer yes when: ads" {
		t.Errorf("spam question = %v", spam)
	}
	choices := qs[1].(map[string]any)["choices"].([]any)
	if choices[1].(map[string]any)["description"] != "bugs" {
		t.Errorf("choices = %v", choices)
	}
	if levels := qs[2].(map[string]any)["levels"].([]any); len(levels) != 2 {
		t.Errorf("levels = %v", levels)
	}

	// Answers come back in question order, not the API's order.
	names := []string{}
	for _, a := range resp.Answers {
		names = append(names, a.Name)
	}
	if want := []string{"spam", "queue", "sev", "nsfw"}; len(names) != 4 || names[0] != want[0] || names[2] != want[2] {
		t.Errorf("answer order = %v", names)
	}
	if a := resp.Answers[0]; a.Choice != decide.No || a.YesProbability != 0.05 {
		t.Errorf("spam = %+v", a)
	}
	if a := resp.Answers[1]; a.Choice != "tech" || a.Confidence != 0.88 || a.Probabilities[0].Value != "billing" {
		t.Errorf("queue = %+v", a)
	}
	if a := resp.Answers[2]; a.Choice != "high" || a.Score != 1.4 || math.Abs(a.Probability("low")-0.2) > 1e-9 {
		t.Errorf("sev = %+v", a)
	}
	if !resp.Answers[3].Refused {
		t.Errorf("nsfw = %+v", resp.Answers[3])
	}
	if resp.Usage.PromptTokens != 120 || resp.Usage.CachedTokens != 100 || resp.Model != DefaultModel {
		t.Errorf("usage = %+v, model = %q", resp.Usage, resp.Model)
	}
}

func TestDecideImages(t *testing.T) {
	var got struct {
		Input []message `json:"input"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"answers": [{"type": "predicate", "name": "q", "probability": 1}]}`))
	}))
	defer srv.Close()

	_, err := New("k", WithBaseURL(srv.URL)).Decide(context.Background(), &decide.Request{
		Input:     "Is the box damaged?",
		Parts:     []langrails.ContentPart{langrails.ImageURLPart("https://x/y.png")},
		Questions: []decide.Question{decide.NewPredicate("q", "")},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := got.Input[0].Content
	if got.Input[0].Role != "user" || c[0].Type != "input_text" || c[1].Type != "input_image" || c[1].ImageURL != "https://x/y.png" {
		t.Errorf("input = %+v", got.Input)
	}
}

func TestDecideErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": {"message": "slow down"}}`))
	}))
	defer srv.Close()

	_, err := New("k", WithBaseURL(srv.URL)).Decide(context.Background(),
		&decide.Request{Questions: []decide.Question{decide.NewPredicate("q", "")}})
	var apiErr *langrails.APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "slow down" || !apiErr.IsRateLimitError() || apiErr.RetryAfter == 0 {
		t.Fatalf("err = %v", err)
	}

	if _, err := New("k").Decide(context.Background(), &decide.Request{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDecideMissingAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers": []}`))
	}))
	defer srv.Close()
	_, err := New("k", WithBaseURL(srv.URL), WithModel("m")).Decide(context.Background(),
		&decide.Request{Questions: []decide.Question{decide.NewPredicate("q", "")}})
	if err == nil {
		t.Fatal("expected error for a missing answer")
	}
}
