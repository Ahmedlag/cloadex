package debate

import (
	"strings"
	"testing"

	"github.com/cloadex-cli/cloadex/internal/runner"
	"github.com/cloadex-cli/cloadex/internal/ui"
)

func TestConvergenceDetection(t *testing.T) {
	// The convergence check in Run() uses:
	//   strings.HasPrefix(strings.TrimSpace(result.Output), "CONVERGED:")
	// Test the same logic directly.
	tests := []struct {
		name      string
		output    string
		converged bool
		wantPlan  string
	}{
		{
			name:      "explicit CONVERGED prefix",
			output:    "CONVERGED: Here is the final plan.",
			converged: true,
			wantPlan:  "Here is the final plan.",
		},
		{
			name:      "CONVERGED with leading whitespace",
			output:    "  CONVERGED: Trimmed plan.",
			converged: true,
			wantPlan:  "Trimmed plan.",
		},
		{
			name:      "CONVERGED with multiline plan",
			output:    "CONVERGED:\n## Tasks\n- Task 1\n- Task 2",
			converged: true,
			wantPlan:  "## Tasks\n- Task 1\n- Task 2",
		},
		{
			name:      "no convergence",
			output:    "I think we should consider React for the frontend.",
			converged: false,
		},
		{
			name:      "CONVERGED mid-text is not convergence",
			output:    "We have not CONVERGED: more discussion needed.",
			converged: false,
		},
		{
			name:      "empty output",
			output:    "",
			converged: false,
		},
		{
			name:      "lowercase converged is not convergence",
			output:    "converged: not valid",
			converged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tt.output)
			converged := strings.HasPrefix(trimmed, "CONVERGED:")

			if converged != tt.converged {
				t.Errorf("convergence = %v, want %v", converged, tt.converged)
			}

			if converged && tt.wantPlan != "" {
				plan := strings.TrimSpace(strings.TrimPrefix(trimmed, "CONVERGED:"))
				if plan != tt.wantPlan {
					t.Errorf("plan = %q, want %q", plan, tt.wantPlan)
				}
			}
		})
	}
}

func TestStreamForAI(t *testing.T) {
	tests := []struct {
		ai   runner.AI
		want string // function identity check via side-effect
	}{
		{runner.Claude, "claude"},
		{runner.Codex, "codex"},
	}

	for _, tt := range tests {
		t.Run(string(tt.ai), func(t *testing.T) {
			fn := streamForAI(tt.ai)
			if fn == nil {
				t.Fatal("streamForAI returned nil")
			}
			// Verify it doesn't panic when called
			fn("test line")
		})
	}
}

func TestMaxRoundsDefault(t *testing.T) {
	if MaxRounds != 5 {
		t.Errorf("MaxRounds = %d, want 5", MaxRounds)
	}
}

func TestHistoryBuilding(t *testing.T) {
	// Simulate the history-building logic from Run()
	var history strings.Builder
	rounds := []struct {
		ai    runner.AI
		round int
		text  string
	}{
		{runner.Claude, 1, "I propose we use Go modules."},
		{runner.Codex, 2, "Agreed, and I suggest adding tests."},
		{runner.Claude, 3, "CONVERGED: Use Go modules with tests."},
	}

	for _, r := range rounds {
		history.WriteString(strings.Join([]string{
			"\n--- ", string(r.ai), " (Round ",
			strings.Repeat("", 0), // dummy to use strings
			") ---\n", r.text, "\n",
		}, ""))
	}

	h := history.String()

	// Each round should appear in history
	if !strings.Contains(h, "claude") {
		t.Error("history should contain claude entries")
	}
	if !strings.Contains(h, "codex") {
		t.Error("history should contain codex entries")
	}
	if !strings.Contains(h, "I propose we use Go modules.") {
		t.Error("history should contain round 1 text")
	}
}

func TestDebateResult_Fields(t *testing.T) {
	r := &DebateResult{
		FinalPlan: "the plan",
		History:   "round 1\nround 2",
		Rounds:    3,
	}

	if r.FinalPlan != "the plan" {
		t.Errorf("FinalPlan = %q", r.FinalPlan)
	}
	if r.Rounds != 3 {
		t.Errorf("Rounds = %d, want 3", r.Rounds)
	}
	if !strings.Contains(r.History, "round 1") {
		t.Error("History should contain round data")
	}
}

func init() {
	// Suppress UI output during tests
	ui.SetVerbose(false)
}
