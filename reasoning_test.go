package langrails

import "testing"

func TestReasoningEffort_BudgetTokens(t *testing.T) {
	tests := []struct {
		effort ReasoningEffort
		want   int
	}{
		{ReasoningOff, 0},
		{ReasoningMinimal, 1024},
		{ReasoningLow, 4096},
		{ReasoningMedium, 8192},
		{ReasoningHigh, 16384},
		{ReasoningEffort("unknown"), 0},
	}

	for _, tc := range tests {
		t.Run(string(tc.effort), func(t *testing.T) {
			got := tc.effort.BudgetTokens()
			if got != tc.want {
				t.Errorf("BudgetTokens() = %d, want %d", got, tc.want)
			}
		})
	}
}
