# Decisions

Decision models answer closed questions with **probabilities instead of
text**: yes/no, one of a set of options, or a rating on a scale. They
are much faster and cheaper than a chat model and easy to threshold,
which makes them a good fit for routing, classification, moderation and
judging. Jev, OpenAI's GPT Luna (Decisions API) and Perplexity Decider
are examples.

The `decide` package gives them one interface:

```go
type Decider interface {
    Decide(ctx context.Context, req *decide.Request) (*decide.Response, error)
}
```

## Deciders

| Decider | Package | Default model | Notes |
|---|---|---|---|
| OpenAI Decisions API | `decide/openai` | `gpt-6-luna` | `POST /v1/decisions` |
| System One (TypeSafe Jev) | `decide/systemone` | `jev-latest` | `POST /v1/systemone`; `WithBaseURL` for compatible or self-hosted servers |
| Any chat provider | `decide.FromProvider` | `Request.Model` | Structured output; see the caveat below |

```go
import (
    "github.com/promptrails/langrails/decide"
    "github.com/promptrails/langrails/decide/openai"
    "github.com/promptrails/langrails/decide/systemone"
)

d := openai.New(os.Getenv("OPENAI_API_KEY"))
d := systemone.New(os.Getenv("TYPESAFE_API_KEY"))
d := systemone.New("", systemone.WithBaseURL("http://localhost:8000/v1/systemone"),
    systemone.WithModel("kev-4b"))
d := decide.FromProvider(llm.MustNew(llm.Anthropic, key)) // Request.Model picks the model
```

## Asking a question

`decide.Ask` asks a single question:

```go
a, err := decide.Ask(ctx, d, "", ticket, decide.NewChoice("queue",
    "Which team should handle this ticket?",
    decide.Option{Value: "billing", Description: "Charges, refunds, invoices"},
    decide.Option{Value: "tech", Description: "Bugs, errors, outages"},
    decide.Option{Value: "sales", Description: "Upgrades, new plans, quotes"},
))
if errors.Is(err, decide.ErrRefused) {
    // the model declined to answer
}
fmt.Println(a.Choice, a.Confidence, a.Probability("billing"))
```

## Question kinds

| Kind | Constructor | Answer |
|---|---|---|
| `decide.Predicate` | `NewPredicate(name, instructions)` | `YesProbability`; `Choice` is `decide.Yes` or `decide.No` |
| `decide.Choice` | `NewChoice(name, instructions, options...)` | `Choice` is the option value; 2–255 options |
| `decide.Score` | `NewScore(name, instructions, levels...)` | `Score` is the probability-weighted level index (0 = first level); `Choice` is the most likely level's label; 2–10 levels, lowest first |

A predicate can say what counts as yes and no with `YesWhen` / `NoWhen`.

Every answer has `Probabilities` (one entry per option, level, or
yes/no, in question order) and `Confidence`. Deciders that report their
own confidence use it; otherwise it is the probability of `Choice`. Since
`Choice` is set for every kind, routing code can treat them alike.

## Several questions at once

Questions in one request are answered independently against the same
input, in one call:

```go
resp, err := d.Decide(ctx, &decide.Request{
    Input: ticket,
    Questions: []decide.Question{
        decide.NewPredicate("spam", "Is this spam?"),
        decide.NewChoice("queue", "Which team?", options...),
        decide.NewScore("severity", "How severe is the issue?",
            decide.Level{Label: "cosmetic", Description: "No lost functionality"},
            decide.Level{Label: "degraded", Description: "Works with a workaround"},
            decide.Level{Label: "blocked", Description: "No workaround"},
        ),
    },
})
sev, _ := resp.Answer("severity")
```

When a question depends on another's answer, send separate requests.

## Images

`Request.Parts` adds text and image parts after `Input`. The OpenAI
decider accepts image URLs and data URIs; System One only accepts base64
PNG or JPEG data URIs (`langrails.ImageBase64Part`) and sets its image
opt-in header for you. Other part types fail with
`langrails.ErrUnsupportedContent`.

## Chat models as deciders

`decide.FromProvider` asks a chat model for a probability per answer
through structured output, so the same code runs on any of the 24
providers. The probabilities are the model's own estimate rather than
token probabilities: they are far less calibrated than a decision
model's, and each call costs a full completion. Use it as a fallback or
for prototyping.

## Errors

HTTP errors are `*langrails.APIError` (with `RetryAfter`), as for chat
providers. A request that fails validation (no questions, duplicate
names, too few options or levels) returns an error before anything is
sent. A question the API does not answer comes back with `Refused` set.
