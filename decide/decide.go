package decide

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/promptrails/langrails"
)

// Kind is the type of a question.
type Kind string

// Question kinds.
const (
	// Predicate is a yes/no question. The answer is the probability of yes.
	Predicate Kind = "predicate"

	// Choice picks one of a closed set of options.
	Choice Kind = "choice"

	// Score rates the input on an ordered scale of levels.
	Score Kind = "score"
)

// Predicate answers report their outcome through Answer.Choice as one of
// these values, so routing code can treat every kind alike.
const (
	Yes = "yes"
	No  = "no"
)

// ErrRefused is returned by Ask when the model declined to answer.
var ErrRefused = errors.New("decide: model refused to answer")

// Option is one possible answer to a Choice question.
type Option struct {
	// Value identifies the option and is what Answer.Choice reports.
	Value string

	// Description tells the model when this option applies. Optional, but
	// it usually matters more than the value's name.
	Description string
}

// Level is one step of a Score scale.
type Level struct {
	// Label names the level and is what Answer.Choice reports.
	Label string

	// Description tells the model what the level means.
	Description string
}

// Question is one closed question about the input.
type Question struct {
	// Name identifies the question and its answer. Names must be unique
	// within a request.
	Name string

	// Kind is Predicate, Choice or Score.
	Kind Kind

	// Instructions is the question itself.
	Instructions string

	// Options are the possible answers of a Choice question.
	Options []Option

	// Levels is the scale of a Score question, lowest first.
	Levels []Level

	// YesWhen and NoWhen optionally describe what counts as yes and no for
	// a Predicate question.
	YesWhen, NoWhen string
}

// NewPredicate creates a yes/no question.
func NewPredicate(name, instructions string) Question {
	return Question{Name: name, Kind: Predicate, Instructions: instructions}
}

// NewChoice creates a question that picks one of options.
func NewChoice(name, instructions string, options ...Option) Question {
	return Question{Name: name, Kind: Choice, Instructions: instructions, Options: options}
}

// NewScore creates a question that rates the input on levels, lowest first.
func NewScore(name, instructions string, levels ...Level) Question {
	return Question{Name: name, Kind: Score, Instructions: instructions, Levels: levels}
}

// Request asks one or more questions about the same input. Questions are
// answered independently; when one depends on another's answer, send
// separate requests.
type Request struct {
	// Model is the decision model. Empty uses the decider's default.
	Model string

	// Input is the text every question is about.
	Input string

	// Parts optionally adds text and image parts after Input. Other part
	// types are rejected with an error wrapping
	// langrails.ErrUnsupportedContent.
	Parts []langrails.ContentPart

	// Questions are the questions to answer.
	Questions []Question
}

// Probability is the probability the model gave one option or level.
type Probability struct {
	// Value is the option value, the level label, or Yes/No.
	Value string

	// Probability is between 0 and 1.
	Probability float64
}

// Answer is the answer to one question.
type Answer struct {
	// Name is the question's name.
	Name string

	// Kind is the question's kind.
	Kind Kind

	// Refused is true when the model declined to answer. The other fields
	// are then zero.
	Refused bool

	// Choice is the most likely outcome: the option value for Choice, the
	// level label for Score, and Yes or No for Predicate.
	Choice string

	// YesProbability is the probability of yes for a Predicate question.
	YesProbability float64

	// Score is the probability-weighted level index (0 for the first
	// level) for a Score question. It can fall between levels.
	Score float64

	// Probabilities holds one entry per option, level, or Yes/No, in the
	// order the question listed them (Yes before No).
	Probabilities []Probability

	// Confidence is how sure the model is, between 0 and 1. Deciders that
	// report their own confidence use it; others use the probability of
	// Choice.
	Confidence float64
}

// Probability returns the probability of value, or 0 when the answer has
// no such value.
func (a Answer) Probability(value string) float64 {
	for _, p := range a.Probabilities {
		if p.Value == value {
			return p.Probability
		}
	}
	return 0
}

// Response holds the answers to a request.
type Response struct {
	// Answers has one entry per question, in question order.
	Answers []Answer

	// Model is the model that answered, when the decider reports it.
	Model string

	// Usage is the token usage of the request.
	Usage langrails.TokenUsage
}

// Answer returns the answer to the named question.
func (r *Response) Answer(name string) (Answer, bool) {
	for _, a := range r.Answers {
		if a.Name == name {
			return a, true
		}
	}
	return Answer{}, false
}

// Decider answers closed questions about an input with probabilities
// instead of generated text.
type Decider interface {
	Decide(ctx context.Context, req *Request) (*Response, error)
}

// Ask asks a single question about input. A refusal returns the answer
// with ErrRefused.
func Ask(ctx context.Context, d Decider, model, input string, q Question) (Answer, error) {
	if q.Name == "" {
		q.Name = "q"
	}
	resp, err := d.Decide(ctx, &Request{Model: model, Input: input, Questions: []Question{q}})
	if err != nil {
		return Answer{}, err
	}
	a, ok := resp.Answer(q.Name)
	if !ok {
		return Answer{}, fmt.Errorf("decide: no answer to %q", q.Name)
	}
	if a.Refused {
		return a, ErrRefused
	}
	return a, nil
}

// Validate checks that the request can be sent: at least one question,
// unique non-empty names, 2–255 distinct options per Choice and 2–10
// distinct levels per Score. Deciders call it before sending.
func (r *Request) Validate() error {
	if len(r.Questions) == 0 {
		return errors.New("decide: request has no questions")
	}
	seen := make(map[string]bool, len(r.Questions))
	for _, q := range r.Questions {
		if q.Name == "" {
			return errors.New("decide: question has no name")
		}
		if seen[q.Name] {
			return fmt.Errorf("decide: duplicate question name %q", q.Name)
		}
		seen[q.Name] = true
		switch q.Kind {
		case Predicate:
		case Choice:
			if len(q.Options) < 2 || len(q.Options) > 255 {
				return fmt.Errorf("decide: question %q needs 2 to 255 options, has %d", q.Name, len(q.Options))
			}
			values := make(map[string]bool, len(q.Options))
			for _, o := range q.Options {
				if o.Value == "" || values[o.Value] {
					return fmt.Errorf("decide: question %q has an empty or duplicate option %q", q.Name, o.Value)
				}
				values[o.Value] = true
			}
		case Score:
			if len(q.Levels) < 2 || len(q.Levels) > 10 {
				return fmt.Errorf("decide: question %q needs 2 to 10 levels, has %d", q.Name, len(q.Levels))
			}
			labels := make(map[string]bool, len(q.Levels))
			for _, l := range q.Levels {
				if l.Label == "" || labels[l.Label] {
					return fmt.Errorf("decide: question %q has an empty or duplicate level %q", q.Name, l.Label)
				}
				labels[l.Label] = true
			}
		default:
			return fmt.Errorf("decide: question %q has unknown kind %q", q.Name, q.Kind)
		}
	}
	for _, p := range r.Parts {
		if p.Type != langrails.ContentText && p.Type != langrails.ContentImage {
			return fmt.Errorf("decide: %s parts: %w", p.Type, langrails.ErrUnsupportedContent)
		}
	}
	return nil
}

// PredicateAnswer builds the answer to a Predicate question from the
// probability of yes. For Decider implementations.
func PredicateAnswer(name string, yes float64) Answer {
	a := Answer{
		Name:           name,
		Kind:           Predicate,
		YesProbability: yes,
		Probabilities:  []Probability{{Value: Yes, Probability: yes}, {Value: No, Probability: 1 - yes}},
		Choice:         No,
		Confidence:     1 - yes,
	}
	if yes >= 0.5 {
		a.Choice = Yes
		a.Confidence = yes
	}
	return a
}

// DistributionAnswer builds the answer to a Choice or Score question from
// probabilities in question order, filling Choice, Score (for Score) and
// Confidence from them. For Decider implementations.
func DistributionAnswer(name string, kind Kind, probs []Probability) Answer {
	a := Answer{Name: name, Kind: kind, Probabilities: probs}
	best := -1.0
	for i, p := range probs {
		if p.Probability > best {
			best = p.Probability
			a.Choice = p.Value
		}
		if kind == Score {
			a.Score += float64(i) * p.Probability
		}
	}
	a.Confidence = math.Max(best, 0)
	return a
}
