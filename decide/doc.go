// Package decide asks decision models closed questions: yes/no
// (Predicate), one of a set of options (Choice) or a rating on a scale
// (Score). The answers are probabilities rather than generated text, which
// makes them cheap, fast and easy to threshold.
//
// A Decider is a decision API (decide/openai for OpenAI's Decisions API,
// decide/systemone for TypeSafe's System One API and compatible servers) or
// any chat provider through FromProvider:
//
//	d := openai.New(os.Getenv("OPENAI_API_KEY"))
//	a, err := decide.Ask(ctx, d, "", ticket, decide.NewChoice("queue",
//	    "Which team should handle this ticket?",
//	    decide.Option{Value: "billing", Description: "Charges, refunds, invoices"},
//	    decide.Option{Value: "tech", Description: "Bugs and outages"},
//	))
//	fmt.Println(a.Choice, a.Confidence)
package decide
