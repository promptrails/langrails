package graph

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type hookKey struct{}

func TestHooks_ReportNodes(t *testing.T) {
	g := New[int]()
	g.AddNode("a", func(ctx context.Context, s int) (int, error) {
		if ctx.Value(hookKey{}) != "a" {
			t.Error("node did not receive the OnNodeStart context")
		}
		return s + 1, nil
	})
	g.AddNode("b", func(_ context.Context, s int) (int, error) { return s * 10, nil })
	g.SetEntryPoint("a")
	g.AddEdge("a", "b")
	g.AddEdge("b", END)

	var mu sync.Mutex
	var log []string
	hooks := Hooks[int]{
		OnNodeStart: func(ctx context.Context, node string, s int) context.Context {
			mu.Lock()
			log = append(log, "start:"+node)
			mu.Unlock()
			return context.WithValue(ctx, hookKey{}, node)
		},
		OnNodeEnd: func(ctx context.Context, node string, s int, err error, d time.Duration) {
			if ctx.Value(hookKey{}) != node || err != nil || d < 0 {
				t.Errorf("OnNodeEnd(%s): ctx=%v err=%v", node, ctx.Value(hookKey{}), err)
			}
			mu.Lock()
			log = append(log, "end:"+node)
			mu.Unlock()
		},
	}

	res, err := g.Run(context.Background(), 1, WithHooks(hooks))
	if err != nil {
		t.Fatal(err)
	}
	if res.State != 20 {
		t.Errorf("state = %d", res.State)
	}
	if got := strings.Join(log, ","); got != "start:a,end:a,start:b,end:b" {
		t.Errorf("hooks = %s", got)
	}
}

func TestHooks_FanOutBranchesAndErrors(t *testing.T) {
	g := New[int]()
	g.AddNode("split", func(_ context.Context, s int) (int, error) { return s, nil })
	g.AddNode("work", func(_ context.Context, s int) (int, error) {
		if s == 3 {
			return 0, errors.New("bad branch")
		}
		return s, nil
	})
	g.AddNode("join", func(_ context.Context, s int) (int, error) { return s, nil })
	g.SetEntryPoint("split")
	g.AddFanOut("split", func(_ context.Context, _ int) ([]Send[int], error) {
		return []Send[int]{{Node: "work", State: 1}, {Node: "work", State: 3}}, nil
	}, nil, "join")
	g.AddEdge("join", END)

	var mu sync.Mutex
	var ends []string
	_, err := g.Run(context.Background(), 0, WithHooks(Hooks[int]{
		OnNodeEnd: func(_ context.Context, node string, _ int, err error, _ time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				ends = append(ends, node+":err")
			} else {
				ends = append(ends, node)
			}
		},
	}))
	if err == nil {
		t.Fatal("expected branch error")
	}
	slices.Sort(ends)
	if got := strings.Join(ends, ","); got != "split,work,work:err" {
		t.Errorf("ends = %s", got)
	}
}
