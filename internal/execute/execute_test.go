package execute

import (
	"context"
	"testing"

	"github.com/Ahmedlag/cloadex/internal/plan"
	"github.com/Ahmedlag/cloadex/internal/runner"
)

func TestRoleLabel(t *testing.T) {
	tests := []struct {
		ai   runner.AI
		want string
	}{
		{runner.Claude, "Developer (Claude)"},
		{runner.Codex, "Developer (Codex)"},
		{runner.AI("gemini"), "gemini"},
	}

	for _, tt := range tests {
		t.Run(string(tt.ai), func(t *testing.T) {
			if got := roleLabel(tt.ai); got != tt.want {
				t.Errorf("roleLabel(%q) = %q, want %q", tt.ai, got, tt.want)
			}
		})
	}
}

func TestRoleLabel_AllProviders(t *testing.T) {
	// Verify unknown provider returns the raw AI name
	if got := roleLabel(runner.AI("deepseek")); got != "deepseek" {
		t.Errorf("roleLabel(deepseek) = %q, want %q", got, "deepseek")
	}
}

func TestStreamForAI(t *testing.T) {
	tests := []struct {
		ai runner.AI
	}{
		{runner.Claude},
		{runner.Codex},
		{runner.AI("unknown")},
	}

	for _, tt := range tests {
		t.Run(string(tt.ai), func(t *testing.T) {
			fn := streamForAI(tt.ai)
			if fn == nil {
				t.Fatal("streamForAI returned nil")
			}
			// Verify it doesn't panic
			fn("test")
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{
			name: "short string unchanged",
			s:    "hello",
			max:  10,
			want: "hello",
		},
		{
			name: "exact length unchanged",
			s:    "hello",
			max:  5,
			want: "hello",
		},
		{
			name: "long string truncated",
			s:    "this is a very long string that needs truncation",
			max:  20,
			want: "this is a very lo...",
		},
		{
			name: "newlines replaced with spaces",
			s:    "line 1\nline 2\nline 3",
			max:  100,
			want: "line 1 line 2 line 3",
		},
		{
			name: "newlines replaced then truncated",
			s:    "line 1\nline 2\nline 3",
			max:  12,
			want: "line 1 li...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.s, tt.max); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestRunWithOptions_RespectsDependencies(t *testing.T) {
	origRunAI := runAI
	defer func() { runAI = origRunAI }()

	calls := []string{}
	runAI = func(ctx context.Context, ai runner.AI, prompt string, onLine runner.StreamCallback) runner.Result {
		calls = append(calls, string(ai))
		return runner.Result{AI: ai, Output: "ok"}
	}

	tasks := []plan.Task{
		{OwnerAI: runner.Claude, Description: "task 0"},
		{OwnerAI: runner.Codex, Description: "task 1", DependsOn: []int{0}},
	}

	result := Run(context.Background(), "plan", tasks)
	if len(result.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(result.Results))
	}
	if result.Results[0].Status != TaskSuccess {
		t.Fatalf("task 0 status = %s, want success", result.Results[0].Status)
	}
	if result.Results[1].Status != TaskSuccess {
		t.Fatalf("task 1 status = %s, want success", result.Results[1].Status)
	}
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(calls))
	}
}

func TestRunWithOptions_SkipsBlockedDependents(t *testing.T) {
	origRunAI := runAI
	defer func() { runAI = origRunAI }()

	runAI = func(ctx context.Context, ai runner.AI, prompt string, onLine runner.StreamCallback) runner.Result {
		if ai == runner.Claude {
			return runner.Result{AI: ai, Err: assertErr("boom")}
		}
		return runner.Result{AI: ai, Output: "ok"}
	}

	tasks := []plan.Task{
		{OwnerAI: runner.Claude, Description: "root task"},
		{OwnerAI: runner.Codex, Description: "dependent task", DependsOn: []int{0}},
	}

	result := Run(context.Background(), "plan", tasks)
	if got := result.Results[0].Status; got != TaskFailed {
		t.Fatalf("task 0 status = %s, want failed", got)
	}
	if got := result.Results[1].Status; got != TaskSkipped {
		t.Fatalf("task 1 status = %s, want skipped", got)
	}
	if result.Results[1].SkipReason == "" {
		t.Fatal("expected skip reason")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
