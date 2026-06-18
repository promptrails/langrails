package anthropic

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
		wantType string
		wantName string
	}{
		{langrails.ToolChoiceAuto, "auto", ""},
		{langrails.ToolChoiceNone, "none", ""},
		{langrails.ToolChoiceRequired, "any", ""}, // Anthropic calls "required" → "any"
	}
	for _, tc := range tests {
		got := convertToolChoice(&langrails.ToolChoice{Mode: tc.mode})
		if got == nil || got.Type != tc.wantType {
			t.Errorf("mode %q → %+v, want type %q", tc.mode, got, tc.wantType)
		}
	}

	forced := convertToolChoice(langrails.ForceTool("get_weather"))
	if forced == nil || forced.Type != "tool" || forced.Name != "get_weather" {
		t.Errorf("forced → %+v, want type 'tool' name 'get_weather'", forced)
	}

	if convertToolChoice(&langrails.ToolChoice{Mode: "bogus"}) != nil {
		t.Error("unknown mode should convert to nil")
	}
}
