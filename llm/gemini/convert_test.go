package gemini

import (
	"testing"

	"github.com/promptrails/langrails"
)

func TestConvertToolChoice(t *testing.T) {
	if convertToolChoice(nil) != nil {
		t.Error("nil ToolChoice should convert to nil")
	}

	tests := []struct {
		mode     langrails.ToolChoiceMode
		wantMode string
	}{
		{langrails.ToolChoiceAuto, "AUTO"},
		{langrails.ToolChoiceNone, "NONE"},
		{langrails.ToolChoiceRequired, "ANY"},
	}
	for _, tc := range tests {
		got := convertToolChoice(&langrails.ToolChoice{Mode: tc.mode})
		if got == nil || got.FunctionCallingConfig.Mode != tc.wantMode {
			t.Errorf("mode %q → %+v, want %q", tc.mode, got, tc.wantMode)
		}
		if len(got.FunctionCallingConfig.AllowedFunctionNames) != 0 {
			t.Errorf("mode %q should not set AllowedFunctionNames", tc.mode)
		}
	}

	forced := convertToolChoice(langrails.ForceTool("get_weather"))
	if forced == nil || forced.FunctionCallingConfig.Mode != "ANY" {
		t.Fatalf("forced → %+v, want mode ANY", forced)
	}
	names := forced.FunctionCallingConfig.AllowedFunctionNames
	if len(names) != 1 || names[0] != "get_weather" {
		t.Errorf("AllowedFunctionNames = %v, want [get_weather]", names)
	}

	if convertToolChoice(&langrails.ToolChoice{Mode: "bogus"}) != nil {
		t.Error("unknown mode should convert to nil")
	}
}

func TestToolCallKey(t *testing.T) {
	if got := toolCallKey(langrails.ToolCall{ID: "c1", Name: "fn"}); got != "c1" {
		t.Errorf("with ID → %q, want %q", got, "c1")
	}
	if got := toolCallKey(langrails.ToolCall{Name: "fn"}); got != "fn" {
		t.Errorf("without ID → %q, want %q (falls back to name)", got, "fn")
	}
}
