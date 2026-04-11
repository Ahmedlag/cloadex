package validate

import (
	"testing"

	"github.com/cloadex-cli/cloadex/internal/execute"
	"github.com/cloadex-cli/cloadex/internal/persist"
	"github.com/cloadex-cli/cloadex/internal/plan"
	"github.com/cloadex-cli/cloadex/internal/runner"
)

func TestExtractStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "explicit COMPLETE",
			input: "Everything looks good. Status: COMPLETE",
			want:  "COMPLETE",
		},
		{
			name:  "NEEDS_FIXES takes priority over COMPLETE",
			input: "Found issues. NEEDS_FIXES. But mostly COMPLETE.",
			want:  "NEEDS_FIXES",
		},
		{
			name:  "INCOMPLETE",
			input: "Several tasks are INCOMPLETE.",
			want:  "INCOMPLETE",
		},
		{
			name:  "NEEDS_FIXES takes priority over INCOMPLETE",
			input: "Status is NEEDS_FIXES but also INCOMPLETE",
			want:  "NEEDS_FIXES",
		},
		{
			name:  "lowercase maps to COMPLETE (not matched as fixes or incomplete)",
			input: "everything is fine, all done",
			want:  "COMPLETE",
		},
		{
			name:  "mixed case needs_fixes",
			input: "Status: Needs_Fixes found",
			want:  "NEEDS_FIXES",
		},
		{
			name:  "empty string defaults to COMPLETE",
			input: "",
			want:  "COMPLETE",
		},
		{
			name:  "waiting input status",
			input: "Status: WAITING_INPUT",
			want:  "WAITING_INPUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractStatus(tt.input); got != tt.want {
				t.Errorf("extractStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestProposalsAgree(t *testing.T) {
	first := persist.ProposalOption{
		AI:          runner.Claude,
		TaskIndices: []int{0, 2},
		FixSummary:  "Rename the helper and update callers.",
	}
	second := persist.ProposalOption{
		AI:          runner.Codex,
		TaskIndices: []int{0, 2},
		FixSummary:  "rename the helper and update callers",
	}
	if !proposalsAgree(first, second) {
		t.Fatal("expected proposals to agree")
	}
}

func TestBranchIndices(t *testing.T) {
	tasks := []plan.Task{
		{Description: "root"},
		{Description: "child", DependsOn: []int{0}},
		{Description: "grandchild", DependsOn: []int{1}},
		{Description: "independent"},
	}
	got := branchIndices(tasks, []int{0})
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("branchIndices len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("branchIndices[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestCollectIssuesIncludesExecutionFailures(t *testing.T) {
	execResult := &execute.ExecutionResult{
		Results: []execute.TaskResult{
			{TaskIndex: 0, Task: plan.Task{Description: "broken task"}, Status: execute.TaskFailed, Error: "compile error"},
		},
	}
	issues := collectIssues(execResult, nil, "All clear")
	if len(issues) != 1 {
		t.Fatalf("issues len = %d, want 1", len(issues))
	}
}
