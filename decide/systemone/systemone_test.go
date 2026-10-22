package systemone

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/decide"
)

func TestDecide(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("X-Extra") != "1" {
			t.Errorf("headers = %v", r.Header)
		}
		if r.Header.Get(imagesHeader) != "" {
			t.Error("images header sent without images")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{
			"model": "jev-1.13.0",
			"answers": {
				"spam": {"type": "noul", "noul": 0.97},
				"queue": {"type": "choice", "choice": "billing", "confidence": 0.7, "probabilities": {"billing": 0.8, "tech": 0.2}},
				"sev": {"type": "score", "score": 0.3, "confidence": 0.55, "legend": {"0": "low", "1": "high"}, "probabilities": {"0": 0.7, "1": 0.3}}
			},
			"usage": {"input_tokens": 40, "output_tokens": 3}
		}`))
	}))
	defer srv.Close()

	d := New("key", WithBaseURL(srv.URL), WithHeader("X-Extra", "1"))
	resp, err := d.Decide(context.Background(), &decide.Request{
		Input: "charged twice",
		Questions: []decide.Question{
			{Name: "spam", Kind: decide.Predicate, Instructions: "Spam?", NoWhen: "a real customer"},
			decide.NewChoice("queue", "Queue?", decide.Option{Value: "billing", Description: "money"}, decide.Option{Value: "tech"}),
			decide.NewScore("sev", "Severity?", decide.Level{Label: "low", Description: "cosmetic"}, decide.Level{Label: "high"}),
			decide.NewPredicate("lost", "?"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got["model"] != DefaultModel || got["state"] != "charged twice" {
		t.Errorf("request = %v", got)
	}
	qs := got["questions"].(map[string]any)
	spam := qs["spam"].(map[string]any)
	if spam["type"] != "noul" || spam["criteria"].(map[string]any)["false"] != "a real customer" {
		t.Errorf("spam = %v", spam)
	}
	crit := qs["queue"].(map[string]any)["criteria"].(map[string]any)
	if crit["billing"] != "money" || crit["tech"] != nil {
		t.Errorf("choice criteria = %v", crit)
	}
	levels := qs["sev"].(map[string]any)["criteria"].([]any)
	if levels[0] != "low: cosmetic" || levels[1] != "high" {
		t.Errorf("score criteria = %v", levels)
	}

	if a := resp.Answers[0]; a.Choice != decide.Yes || a.YesProbability != 0.97 {
		t.Errorf("spam = %+v", a)
	}
	if a := resp.Answers[1]; a.Choice != "billing" || a.Confidence != 0.7 || a.Probability("tech") != 0.2 {
		t.Errorf("queue = %+v", a)
	}
	if a := resp.Answers[2]; a.Choice != "low" || a.Score != 0.3 || a.Probability("high") != 0.3 || a.Confidence != 0.55 {
		t.Errorf("sev = %+v", a)
	}
	if !resp.Answers[3].Refused {
		t.Errorf("lost = %+v", resp.Answers[3])
	}
	if resp.Model != "jev-1.13.0" || resp.Usage.TotalTokens != 43 {
		t.Errorf("model = %q, usage = %+v", resp.Model, resp.Usage)
	}
}

func TestDecideImages(t *testing.T) {
	var got request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(imagesHeader) != "true" {
			t.Error("images header missing")
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"answers": {"q": {"type": "noul", "noul": 0.5}}}`))
	}))
	defer srv.Close()

	_, err := New("", WithBaseURL(srv.URL)).Decide(context.Background(), &decide.Request{
		Input:     "Damaged?",
		Parts:     []langrails.ContentPart{langrails.ImageBase64Part("AAAA", "image/png")},
		Questions: []decide.Question{decide.NewPredicate("q", "")},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := got.State.([]any)
	if len(state) != 2 || state[0] != "Damaged?" {
		t.Fatalf("state = %v", got.State)
	}
	a, ok := got.Assets[state[1].(string)]
	if !ok || a.Format != "png" || a.DataBase64 != "AAAA" {
		t.Errorf("assets = %+v", got.Assets)
	}

	_, err = New("").Decide(context.Background(), &decide.Request{
		Parts:     []langrails.ContentPart{langrails.ImageURLPart("https://x/y.png")},
		Questions: []decide.Question{decide.NewPredicate("q", "")},
	})
	if !errors.Is(err, langrails.ErrUnsupportedContent) {
		t.Errorf("remote image err = %v", err)
	}
}

func TestDecideErrorDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail": [{"loc": ["body", "state"], "msg": "field required"}]}`))
	}))
	defer srv.Close()

	_, err := New("k", WithBaseURL(srv.URL)).Decide(context.Background(),
		&decide.Request{Questions: []decide.Question{decide.NewPredicate("q", "")}})
	var apiErr *langrails.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 422 || apiErr.Message != "field required" {
		t.Fatalf("err = %v", err)
	}
}
