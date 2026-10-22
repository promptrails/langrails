// Package systemone implements decide.Decider for TypeSafe's System One API
// (POST /v1/systemone), which Jev speaks, and for servers that copy its
// request shape, such as local decision-model servers. Point WithBaseURL at
// them.
package systemone

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/decide"
	"github.com/promptrails/langrails/internal/httpjson"
)

const (
	defaultBaseURL = "https://api.typesafe.ai/v1/systemone"

	// DefaultModel is the model used when the request names none.
	DefaultModel = "jev-latest"

	// imagesHeader opts a request into image assets.
	imagesHeader = "TypeSafe-Danger-Zone-Images"
)

// Decider calls a System One endpoint.
type Decider struct {
	baseURL string
	model   string
	client  httpjson.Client
}

// Option configures the Decider.
type Option func(*Decider)

// WithBaseURL sets the endpoint URL, for a compatible or self-hosted server.
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

// WithHeader adds a header to every request, for example TypeSafe's
// long-context opt-in.
func WithHeader(key, value string) Option {
	return func(d *Decider) { d.client.Headers[key] = value }
}

// New creates a Decider. apiKey is sent as a bearer token; leave it empty
// for a local server that needs none.
func New(apiKey string, opts ...Option) *Decider {
	d := &Decider{
		baseURL: defaultBaseURL,
		model:   DefaultModel,
		client: httpjson.Client{
			Provider:     "systemone",
			HTTP:         &http.Client{Timeout: 2 * time.Minute},
			Headers:      map[string]string{},
			ErrorMessage: httpjson.DetailError,
		},
	}
	if apiKey != "" {
		d.client.Headers["Authorization"] = "Bearer " + apiKey
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

type request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]question `json:"questions"`
	Assets    map[string]asset    `json:"assets,omitempty"`
}

type question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

type asset struct {
	Type       string `json:"type"`
	Format     string `json:"format"`
	DataBase64 string `json:"data_base64"`
}

type response struct {
	Model   string            `json:"model"`
	Answers map[string]answer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type answer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice"`
	Score         float64            `json:"score"`
	Confidence    *float64           `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// Decide sends the request to the System One endpoint.
func (d *Decider) Decide(ctx context.Context, req *decide.Request) (*decide.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	body := request{Model: req.Model, Questions: make(map[string]question, len(req.Questions))}
	if body.Model == "" {
		body.Model = d.model
	}
	var extra map[string]string
	state, assets, err := buildState(req)
	if err != nil {
		return nil, err
	}
	body.State, body.Assets = state, assets
	if len(assets) > 0 {
		extra = map[string]string{imagesHeader: "true"}
	}
	for _, q := range req.Questions {
		body.Questions[q.Name] = toQuestion(q)
	}

	var resp response
	if err := d.client.Post(ctx, d.baseURL, body, &resp, extra); err != nil {
		return nil, err
	}

	out := &decide.Response{
		Model: resp.Model,
		Usage: langrails.TokenUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
	for _, q := range req.Questions {
		a, ok := resp.Answers[q.Name]
		if !ok {
			// The API has no refusal type: an answer it could not give is
			// left out (and not billed).
			out.Answers = append(out.Answers, decide.Answer{Name: q.Name, Kind: q.Kind, Refused: true})
			continue
		}
		out.Answers = append(out.Answers, fromAnswer(q, a))
	}
	return out, nil
}

// buildState returns the state: the input text alone, or an array of the
// text parts and image placeholders, with the images as assets.
func buildState(req *decide.Request) (any, map[string]asset, error) {
	if len(req.Parts) == 0 {
		return req.Input, nil, nil
	}
	var state []string
	if req.Input != "" {
		state = append(state, req.Input)
	}
	assets := map[string]asset{}
	for _, p := range req.Parts {
		if p.Type == langrails.ContentText {
			state = append(state, p.Text)
			continue
		}
		a, err := toAsset(p.ImageURL)
		if err != nil {
			return nil, nil, err
		}
		id, err := placeholder()
		if err != nil {
			return nil, nil, err
		}
		assets[id] = a
		state = append(state, id)
	}
	return state, assets, nil
}

// toAsset converts a base64 data URI into an image asset. Remote URLs are
// not accepted by the API.
func toAsset(uri string) (asset, error) {
	rest, ok := strings.CutPrefix(uri, "data:")
	mediaType, data, ok2 := strings.Cut(rest, ";base64,")
	if !ok || !ok2 {
		return asset{}, fmt.Errorf("systemone: images must be base64 data URIs: %w", langrails.ErrUnsupportedContent)
	}
	switch mediaType {
	case "image/png":
		return asset{Type: "image", Format: "png", DataBase64: data}, nil
	case "image/jpeg", "image/jpg":
		return asset{Type: "image", Format: "jpeg", DataBase64: data}, nil
	}
	return asset{}, fmt.Errorf("systemone: %s images: %w", mediaType, langrails.ErrUnsupportedContent)
}

// placeholder returns a random UUID-shaped id for an asset.
func placeholder() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

func toQuestion(q decide.Question) question {
	out := question{Instructions: q.Instructions}
	switch q.Kind {
	case decide.Predicate:
		out.Type = "noul"
		if q.YesWhen != "" || q.NoWhen != "" {
			c := map[string]string{}
			if q.YesWhen != "" {
				c["true"] = q.YesWhen
			}
			if q.NoWhen != "" {
				c["false"] = q.NoWhen
			}
			out.Criteria = c
		}
	case decide.Choice:
		out.Type = "choice"
		c := make(map[string]*string, len(q.Options))
		for _, o := range q.Options {
			if o.Description == "" {
				c[o.Value] = nil
			} else {
				desc := o.Description
				c[o.Value] = &desc
			}
		}
		out.Criteria = c
	case decide.Score:
		// Levels are positional descriptions; the label leads so the model
		// sees it too.
		out.Type = "score"
		c := make([]string, len(q.Levels))
		for i, l := range q.Levels {
			c[i] = l.Label
			if l.Description != "" {
				c[i] += ": " + l.Description
			}
		}
		out.Criteria = c
	}
	return out
}

func fromAnswer(q decide.Question, a answer) decide.Answer {
	if q.Kind == decide.Predicate {
		return decide.PredicateAnswer(q.Name, a.Noul)
	}
	var probs []decide.Probability
	for _, o := range q.Options {
		probs = append(probs, decide.Probability{Value: o.Value, Probability: a.Probabilities[o.Value]})
	}
	for i, l := range q.Levels {
		// Score probabilities are keyed by level index.
		probs = append(probs, decide.Probability{Value: l.Label, Probability: a.Probabilities[strconv.Itoa(i)]})
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
