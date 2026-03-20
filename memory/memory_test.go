package memory

import (
	"sync"
	"testing"

	"github.com/promptrails/langrails"
)

func TestMemory_AddAndGet(t *testing.T) {
	mem := New()
	mem.AddUserMessage("Hello")
	mem.AddAssistantMessage("Hi there!")

	msgs := mem.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Hi there!" {
		t.Errorf("unexpected second message: %+v", msgs[1])
	}
}

func TestMemory_MaxMessages(t *testing.T) {
	mem := New(WithMaxMessages(3))
	mem.AddUserMessage("msg1")
	mem.AddAssistantMessage("resp1")
	mem.AddUserMessage("msg2")
	mem.AddAssistantMessage("resp2") // This should trim msg1

	msgs := mem.Messages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "resp1" {
		t.Errorf("expected first message to be 'resp1', got %q", msgs[0].Content)
	}
}

func TestMemory_MaxMessages_PreservesSystem(t *testing.T) {
	mem := New(WithMaxMessages(3))
	mem.Add(langrails.Message{Role: "system", Content: "You are helpful."})
	mem.AddUserMessage("msg1")
	mem.AddAssistantMessage("resp1")
	mem.AddUserMessage("msg2") // Should trim msg1, not system

	msgs := mem.Messages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected system message preserved, got %q", msgs[0].Role)
	}
	if msgs[1].Content != "resp1" {
		t.Errorf("expected 'resp1', got %q", msgs[1].Content)
	}
}

func TestMemory_MaxTokens(t *testing.T) {
	mem := New(WithMaxTokens(50))
	mem.AddUserMessage("short")
	mem.AddAssistantMessage("This is a much longer response that should push us over the token limit when combined with more messages")
	mem.AddUserMessage("another")

	// Should have trimmed to stay within 50 tokens
	if mem.TokenCount() > 50 {
		t.Errorf("expected token count <= 50, got %d", mem.TokenCount())
	}
}

func TestMemory_Len(t *testing.T) {
	mem := New()
	if mem.Len() != 0 {
		t.Errorf("expected 0, got %d", mem.Len())
	}
	mem.AddUserMessage("Hello")
	if mem.Len() != 1 {
		t.Errorf("expected 1, got %d", mem.Len())
	}
}

func TestMemory_Clear(t *testing.T) {
	mem := New()
	mem.AddUserMessage("Hello")
	mem.AddAssistantMessage("Hi")
	mem.Clear()

	if mem.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", mem.Len())
	}
}

func TestMemory_Last(t *testing.T) {
	mem := New()
	mem.AddUserMessage("msg1")
	mem.AddAssistantMessage("resp1")
	mem.AddUserMessage("msg2")
	mem.AddAssistantMessage("resp2")

	last := mem.Last(2)
	if len(last) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(last))
	}
	if last[0].Content != "msg2" {
		t.Errorf("expected 'msg2', got %q", last[0].Content)
	}
	if last[1].Content != "resp2" {
		t.Errorf("expected 'resp2', got %q", last[1].Content)
	}
}

func TestMemory_Last_MoreThanAvailable(t *testing.T) {
	mem := New()
	mem.AddUserMessage("msg1")

	last := mem.Last(10)
	if len(last) != 1 {
		t.Fatalf("expected 1 message, got %d", len(last))
	}
}

func TestMemory_TokenCount(t *testing.T) {
	mem := New()
	mem.AddUserMessage("Hello world")
	count := mem.TokenCount()
	if count <= 0 {
		t.Errorf("expected positive token count, got %d", count)
	}
}

func TestMemory_ReturnsCopy(t *testing.T) {
	mem := New()
	mem.AddUserMessage("Hello")

	msgs := mem.Messages()
	msgs[0].Content = "modified"

	original := mem.Messages()
	if original[0].Content != "Hello" {
		t.Error("Messages() should return a copy, not a reference")
	}
}

func TestMemory_ConcurrentAccess(t *testing.T) {
	mem := New(WithMaxMessages(100))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mem.AddUserMessage("concurrent message")
			_ = mem.Messages()
			_ = mem.Len()
			_ = mem.TokenCount()
		}()
	}
	wg.Wait()

	if mem.Len() > 100 {
		t.Errorf("expected <= 100 messages, got %d", mem.Len())
	}
}

func TestEstimateTokens(t *testing.T) {
	// ~4 chars per token
	tokens := estimateTokens("Hello world!") // 12 chars → ~4 tokens
	if tokens < 2 || tokens > 6 {
		t.Errorf("unexpected token estimate: %d", tokens)
	}
}
