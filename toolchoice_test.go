package langrails

import "testing"

func TestToolChoiceFactories(t *testing.T) {
	tests := []struct {
		name     string
		got      *ToolChoice
		wantMode ToolChoiceMode
		wantName string
	}{
		{"auto", AutoToolChoice(), ToolChoiceAuto, ""},
		{"none", NoToolChoice(), ToolChoiceNone, ""},
		{"required", RequiredToolChoice(), ToolChoiceRequired, ""},
		{"force", ForceTool("get_weather"), ToolChoiceTool, "get_weather"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got == nil {
				t.Fatal("factory returned nil")
			}
			if tc.got.Mode != tc.wantMode {
				t.Errorf("Mode = %q, want %q", tc.got.Mode, tc.wantMode)
			}
			if tc.got.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", tc.got.Name, tc.wantName)
			}
		})
	}
}

// ForceTool with an empty name still selects the tool mode; the provider is
// responsible for validating that a name was supplied.
func TestForceTool_EmptyName(t *testing.T) {
	tc := ForceTool("")
	if tc.Mode != ToolChoiceTool {
		t.Errorf("Mode = %q, want %q", tc.Mode, ToolChoiceTool)
	}
	if tc.Name != "" {
		t.Errorf("Name = %q, want empty", tc.Name)
	}
}
