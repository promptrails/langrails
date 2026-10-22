// Package openai implements decide.Decider for OpenAI's Decisions API
// (POST /v1/decisions).
package openai

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/decide"
	"github.com/promptrails/langrails/internal/httpjson"
)

const (
	defaultBaseURL = "https://api.openai.com/v1/decisions"

	// DefaultModel is the model used when the request names none.
	DefaultModel = "gpt-6-luna"
)

// Decider calls OpenAI's Decisions API.
type Decider struct {
	baseURL string
	model   string
	client  httpjson.Client
}

// Option configures the Decider.
type Option func(*Decider)

// WithBaseURL sets a custom endpoint URL (a proxy or a compatible server).
func WithBaseURL(url string) Option {
	return func(d *Decider) { d.baseURL = url }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(d *Decider) { d.client.HTTP = c }
}

// WithModel sets the model used when a request names none.
func WithModel(model string) Option {
	return func(d *Decider) { d.model = model }
}

// New creates a Decider with the given API key.
func New(apiKey string, opts ...Option) *Decider {
	d := &Decider{
		baseURL: defaultBaseURL,
		model:   DefaultModel,
		client: httpjson.Client{
			Provider:     "openai",
			HTTP:         &http.Client{Timeout: 2 * time.Minute},
			Headers:      map[string]string{"Authorization": "Bearer " + apiKey},
			ErrorMessage: httpjson.OpenAIError,
		},
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

type request struct {
	Model     string     `json:"model"`
	Input     any        `json:"input"`
	Questions []question `json:"questions"`
}

type message struct {
	Role    string `json:"role"`
	Content []part `json:"content"`
}

type part struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type question struct {
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	Instructions string   `json:"instructions,omitempty"`
	Choices      []choice `json:"choices,omitempty"`
	Levels       []level  `json:"levels,omitempty"`
}

type choice struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type level struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type response struct {
	Model   string   `json:"model"`
	Answers []answer `json:"answers"`
	Usage   struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		InputTokensDetails struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

type answer struct {
	Type          string        `json:"type"`
	Name          string        `json:"name"`
	Probability   float64       `json:"probability"`
	Choice        string        `json:"choice"`
	Score         float64       `json:"score"`
	Confidence    *float64      `json:"confidence"`
	Probabilities []probability `json:"probabilities"`
}

type probability struct {
	// Value is the option value for choice answers and the level index
	// (a number) for score answers, so it is decoded loosely.
	Value       any     `json:"value"`
	Label       string  `json:"label"`
	Probability float64 `json:"probability"`
}

// Decide sends the request to the Decisions API.
func (d *Decider) Decide(ctx context.Context, req *decide.Request) (*decide.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	body := request{Model: req.Model, Input: input(req)}
	if body.Model == "" {
		body.Model = d.model
	}
	for _, q := range req.Questions {
		body.Questions = append(body.Questions, toQuestion(q))
	}

	var resp response
	if err := d.client.Post(ctx, d.baseURL, body, &resp, nil); err != nil {
		return nil, err
	}

	byName := make(map[string]answer, len(resp.Answers))
	for _, a := range resp.Answers {
		byName[a.Name] = a
	}
	out := &decide.Response{
		Model: resp.Model,
		Usage: langrails.TokenUsage{
			PromptTokens:        resp.Usage.InputTokens,
			CompletionTokens:    resp.Usage.OutputTokens,
			TotalTokens:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
			CachedTokens:        resp.Usage.InputTokensDetails.CachedTokens,
			CacheCreationTokens: resp.Usage.InputTokensDetails.CacheWriteTokens,
		},
	}
	if out.Model == "" {
		out.Model = body.Model
	}
	for _, q := range req.Questions {
		a, ok := byName[q.Name]
		if !ok {
			return nil, fmt.Errorf("openai: no answer to question %q", q.Name)
		}
		out.Answers = append(out.Answers, fromAnswer(q, a))
	}
	return out, nil
}

// input sends plain text as a string and anything with parts as one user
// message.
func input(req *decide.Request) any {
	if len(req.Parts) == 0 {
		return req.Input
	}
	var parts []part
	if req.Input != "" {
		parts = append(parts, part{Type: "input_text", Text: req.Input})
	}
	for _, p := range req.Parts {
		if p.Type == langrails.ContentImage {
			parts = append(parts, part{Type: "input_image", ImageURL: p.ImageURL})
		} else {
			parts = append(parts, part{Type: "input_text", Text: p.Text})
		}
	}
	return []message{{Role: "user", Content: parts}}
}

func toQuestion(q decide.Question) question {
	out := question{Type: string(q.Kind), Name: q.Name, Instructions: q.Instructions}
	if q.Kind == decide.Predicate && (q.YesWhen != "" || q.NoWhen != "") {
		// The API has no criteria field for predicates, so the criteria
		// become part of the instructions.
		if q.YesWhen != "" {
			out.Instructions += "\nAnswer yes when: " + q.YesWhen
		}
		if q.NoWhen != "" {
			out.Instructions += "\nAnswer no when: " + q.NoWhen
		}
	}
	for _, o := range q.Options {
		out.Choices = append(out.Choices, choice(o))
	}
	for _, l := range q.Levels {
		out.Levels = append(out.Levels, level(l))
	}
	return out
}

func fromAnswer(q decide.Question, a answer) decide.Answer {
	if a.Type == "refusal" {
		return decide.Answer{Name: q.Name, Kind: q.Kind, Refused: true}
	}
	if q.Kind == decide.Predicate {
		return decide.PredicateAnswer(q.Name, a.Probability)
	}

	// Map the reported probabilities back to the question's order.
	got := make(map[string]float64, len(a.Probabilities))
	for i, p := range a.Probabilities {
		switch q.Kind {
		case decide.Choice:
			if v, ok := p.Value.(string); ok {
				got[v] = p.Probability
			}
		case decide.Score:
			key := p.Label
			if key == "" && i < len(q.Levels) {
				key = q.Levels[i].Label
			}
			got[key] = p.Probability
		}
	}
	var probs []decide.Probability
	for _, o := range q.Options {
		probs = append(probs, decide.Probability{Value: o.Value, Probability: got[o.Value]})
	}
	for _, l := range q.Levels {
		probs = append(probs, decide.Probability{Value: l.Label, Probability: got[l.Label]})
	}

	out := decide.DistributionAnswer(q.Name, q.Kind, probs)
	if q.Kind == decide.Choice && a.Choice != "" {
		out.Choice = a.Choice
	}
	if q.Kind == decide.Score {
		out.Score = a.Score
	}
	if a.Confidence != nil {
		out.Confidence = *a.Confidence
	}
	return out
}
