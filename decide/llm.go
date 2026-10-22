package decide

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/promptrails/langrails"
)

const llmSystemPrompt = `You answer closed questions about an input. For every question, give a probability between 0 and 1 for each allowed answer, reflecting how likely it is to be correct; for a question with several answers the probabilities must add up to 1. Reply with JSON only, matching the schema.`

// llmDecider answers questions with a chat model and structured output.
type llmDecider struct {
	p langrails.Provider
}

// FromProvider returns a Decider that answers with a chat model through
// structured output, so any provider can stand in for a decision model.
// Request.Model is the chat model to use.
//
// The probabilities are the model's own estimate, not token
// probabilities: they are far less calibrated than a decision model's,
// and a call costs as much as an ordinary completion.
func FromProvider(p langrails.Provider) Decider {
	return &llmDecider{p: p}
}

func (d *llmDecider) Decide(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	schema, err := json.Marshal(llmSchema(req.Questions))
	if err != nil {
		return nil, err
	}

	parts := append([]langrails.ContentPart{langrails.TextPart(llmPrompt(req))}, req.Parts...)
	temp := 0.0
	resp, err := d.p.Complete(ctx, &langrails.CompletionRequest{
		Model:        req.Model,
		SystemPrompt: llmSystemPrompt,
		Messages:     []langrails.Message{{Role: langrails.RoleUser, ContentParts: parts}},
		OutputSchema: schema,
		Temperature:  &temp,
	})
	if err != nil {
		return nil, err
	}

	var raw map[string]struct {
		Yes           *float64           `json:"yes"`
		Probabilities map[string]float64 `json:"probabilities"`
	}
	if err := json.Unmarshal([]byte(trimFence(resp.Content)), &raw); err != nil {
		return nil, &langrails.ParseError{Content: resp.Content, Err: err}
	}

	out := &Response{Model: resp.Model, Usage: resp.Usage}
	for _, q := range req.Questions {
		r, ok := raw[q.Name]
		switch {
		case !ok:
			out.Answers = append(out.Answers, Answer{Name: q.Name, Kind: q.Kind, Refused: true})
		case q.Kind == Predicate:
			yes := 0.0
			if r.Yes != nil {
				yes = clamp01(*r.Yes)
			}
			out.Answers = append(out.Answers, PredicateAnswer(q.Name, yes))
		default:
			out.Answers = append(out.Answers, DistributionAnswer(q.Name, q.Kind, normalize(q.values(), r.Probabilities)))
		}
	}
	return out, nil
}

// values lists a Choice question's option values or a Score question's
// level labels, in order.
func (q Question) values() []string {
	var vs []string
	for _, o := range q.Options {
		vs = append(vs, o.Value)
	}
	for _, l := range q.Levels {
		vs = append(vs, l.Label)
	}
	return vs
}

func llmPrompt(req *Request) string {
	var b strings.Builder
	b.WriteString("Input:\n")
	b.WriteString(req.Input)
	b.WriteString("\n\nQuestions:\n")
	for _, q := range req.Questions {
		fmt.Fprintf(&b, "\n%q", q.Name)
		switch q.Kind {
		case Predicate:
			fmt.Fprintf(&b, " (yes/no; give the probability of yes): %s\n", q.Instructions)
			if q.YesWhen != "" {
				fmt.Fprintf(&b, "- yes: %s\n", q.YesWhen)
			}
			if q.NoWhen != "" {
				fmt.Fprintf(&b, "- no: %s\n", q.NoWhen)
			}
		case Choice:
			fmt.Fprintf(&b, " (pick one): %s\n", q.Instructions)
			for _, o := range q.Options {
				writeItem(&b, o.Value, o.Description)
			}
		case Score:
			fmt.Fprintf(&b, " (rate on this scale, lowest first): %s\n", q.Instructions)
			for _, l := range q.Levels {
				writeItem(&b, l.Label, l.Description)
			}
		}
	}
	return b.String()
}

func writeItem(b *strings.Builder, name, desc string) {
	if desc == "" {
		fmt.Fprintf(b, "- %s\n", name)
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", name, desc)
}

type schemaNode struct {
	Type                 string                 `json:"type"`
	Properties           map[string]*schemaNode `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`
	Minimum              *float64               `json:"minimum,omitempty"`
	Maximum              *float64               `json:"maximum,omitempty"`
}

func object(props map[string]*schemaNode, required []string) *schemaNode {
	no := false
	return &schemaNode{Type: "object", Properties: props, Required: required, AdditionalProperties: &no}
}

func probability() *schemaNode {
	lo, hi := 0.0, 1.0
	return &schemaNode{Type: "number", Minimum: &lo, Maximum: &hi}
}

// llmSchema describes the reply: one object per question, holding "yes"
// for a Predicate and "probabilities" keyed by value otherwise.
func llmSchema(qs []Question) *schemaNode {
	props := make(map[string]*schemaNode, len(qs))
	names := make([]string, 0, len(qs))
	for _, q := range qs {
		names = append(names, q.Name)
		if q.Kind == Predicate {
			props[q.Name] = object(map[string]*schemaNode{"yes": probability()}, []string{"yes"})
			continue
		}
		values := q.values()
		dist := make(map[string]*schemaNode, len(values))
		for _, v := range values {
			dist[v] = probability()
		}
		props[q.Name] = object(map[string]*schemaNode{"probabilities": object(dist, values)}, []string{"probabilities"})
	}
	return object(props, names)
}

// normalize returns the probabilities of values in order, scaled to sum to
// 1. Missing or negative values count as 0; all zeros become uniform.
func normalize(values []string, got map[string]float64) []Probability {
	probs := make([]Probability, len(values))
	sum := 0.0
	for i, v := range values {
		p := got[v]
		if p < 0 {
			p = 0
		}
		probs[i] = Probability{Value: v, Probability: p}
		sum += p
	}
	for i := range probs {
		if sum > 0 {
			probs[i].Probability /= sum
		} else {
			probs[i].Probability = 1 / float64(len(probs))
		}
	}
	return probs
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// trimFence removes a surrounding ```json … ``` fence, which some models
// add even with structured output.
func trimFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}
