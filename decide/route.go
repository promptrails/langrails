package decide

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/graph"
)

// ErrNoRoute is returned by a router when an answer has no route: the
// model refused, its choice has no target, or its confidence is below the
// minimum, and no fallback is set.
var ErrNoRoute = errors.New("decide: no route for answer")

// Target is where a Router sends a request.
type Target struct {
	// Provider handles the request.
	Provider langrails.Provider

	// Model replaces the request's model when set, so a route can pick a
	// cheaper or stronger model on the same provider.
	Model string
}

// RouteOption configures Route and GraphRouter.
type RouteOption func(*routeConfig)

type routeConfig struct {
	minConfidence float64
	fallback      *Target
	fallbackNode  string
	input         func(*langrails.CompletionRequest) string
	onDecision    func(ctx context.Context, a Answer)
}

// WithMinConfidence sends answers whose Confidence is below c to the
// fallback, so only clear-cut cases take the routed path.
func WithMinConfidence(c float64) RouteOption {
	return func(r *routeConfig) { r.minConfidence = c }
}

// WithFallback sets the target for answers Route cannot route: refusals,
// choices without a target, and answers below the minimum confidence.
func WithFallback(t Target) RouteOption {
	return func(r *routeConfig) { r.fallback = &t }
}

// WithFallbackNode sets the node GraphRouter goes to for answers it cannot
// route.
func WithFallbackNode(node string) RouteOption {
	return func(r *routeConfig) { r.fallbackNode = node }
}

// WithInput sets how Route turns a request into the text the question is
// about. The default is the text of the last user message.
func WithInput(fn func(*langrails.CompletionRequest) string) RouteOption {
	return func(r *routeConfig) { r.input = fn }
}

// WithOnDecision is called with every answer, for logging and metrics.
func WithOnDecision(fn func(ctx context.Context, a Answer)) RouteOption {
	return func(r *routeConfig) { r.onDecision = fn }
}

func newRouteConfig(opts []RouteOption) *routeConfig {
	cfg := &routeConfig{input: lastUserText}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// decide asks q and returns the answer's route key, or "" when the answer
// falls back.
func (c *routeConfig) decide(ctx context.Context, d Decider, model, input string, q Question) (string, error) {
	a, err := Ask(ctx, d, model, input, q)
	if err != nil && !errors.Is(err, ErrRefused) {
		return "", err
	}
	if c.onDecision != nil {
		c.onDecision(ctx, a)
	}
	if a.Refused || a.Confidence < c.minConfidence {
		return "", nil
	}
	return a.Choice, nil
}

// Router is a langrails.Provider that asks a decision model where each
// request should go.
type Router struct {
	d      Decider
	model  string
	q      Question
	routes map[string]Target
	cfg    *routeConfig
}

// Route returns a Provider that answers q about each request (with the
// decision model named by model, "" for the decider's default) and sends
// the request to the target keyed by the answer's Choice: an option value,
// a level label, or Yes/No for a predicate.
//
// The classic use is a small-model/large-model split, where a fast
// decision model sends easy requests to a cheap model and the rest to a
// strong one:
//
//	p := decide.Route(d, "", decide.NewPredicate("hard",
//	    "Does answering this need multi-step reasoning or expert knowledge?"),
//	    map[string]decide.Target{
//	        decide.No:  {Provider: openai, Model: "gpt-6-luna"},
//	        decide.Yes: {Provider: anthropic, Model: "claude-opus-5-5"},
//	    },
//	    decide.WithMinConfidence(0.8),
//	    decide.WithFallback(decide.Target{Provider: anthropic, Model: "claude-opus-5-5"}),
//	)
//
// Because it is a Provider, it drops into agents, chains and graphs as is.
func Route(d Decider, model string, q Question, routes map[string]Target, opts ...RouteOption) *Router {
	if q.Name == "" {
		q.Name = "route"
	}
	return &Router{d: d, model: model, q: q, routes: routes, cfg: newRouteConfig(opts)}
}

// Complete routes the request and completes it on the chosen target.
func (r *Router) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	t, routed, err := r.pick(ctx, req)
	if err != nil {
		return nil, err
	}
	return t.Provider.Complete(ctx, routed)
}

// Stream routes the request and streams it from the chosen target.
func (r *Router) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	t, routed, err := r.pick(ctx, req)
	if err != nil {
		return nil, err
	}
	return t.Provider.Stream(ctx, routed)
}

func (r *Router) pick(ctx context.Context, req *langrails.CompletionRequest) (Target, *langrails.CompletionRequest, error) {
	key, err := r.cfg.decide(ctx, r.d, r.model, r.cfg.input(req), r.q)
	if err != nil {
		return Target{}, nil, err
	}
	t, ok := r.routes[key]
	if !ok || key == "" {
		if r.cfg.fallback == nil {
			return Target{}, nil, fmt.Errorf("%w %q", ErrNoRoute, key)
		}
		t = *r.cfg.fallback
	}
	if t.Model == "" || t.Model == req.Model {
		return t, req, nil
	}
	routed := *req
	routed.Model = t.Model
	return t, &routed, nil
}

// GraphRouter returns a graph router that answers q about the text input
// extracts from the state and goes to the node keyed by the answer's
// Choice in routes. With nil routes the choice itself is the node name.
// Register it with Graph.AddConditionalEdgeCtx:
//
//	g.AddConditionalEdgeCtx("intake", decide.GraphRouter(d, "", queueQuestion,
//	    func(s Ticket) string { return s.Body },
//	    map[string]string{"billing": "refund", "tech": "debug"},
//	    decide.WithMinConfidence(0.7), decide.WithFallbackNode("human"),
//	))
func GraphRouter[S any](d Decider, model string, q Question, input func(S) string, routes map[string]string, opts ...RouteOption) graph.RouterCtxFunc[S] {
	if q.Name == "" {
		q.Name = "route"
	}
	cfg := newRouteConfig(opts)
	return func(ctx context.Context, state S) (string, error) {
		key, err := cfg.decide(ctx, d, model, input(state), q)
		if err != nil {
			return "", err
		}
		node := key
		if routes != nil {
			node = routes[key]
		}
		if key == "" || node == "" {
			if cfg.fallbackNode == "" {
				return "", fmt.Errorf("%w %q", ErrNoRoute, key)
			}
			node = cfg.fallbackNode
		}
		return node, nil
	}
}

// lastUserText returns the text of the last user message.
func lastUserText(req *langrails.CompletionRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		m := req.Messages[i]
		if m.Role != langrails.RoleUser {
			continue
		}
		if len(m.ContentParts) == 0 {
			return m.Content
		}
		var texts []string
		for _, p := range m.ContentParts {
			if p.Type == langrails.ContentText {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}
