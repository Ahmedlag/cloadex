package plan

import (
	"strings"
	"testing"

	"github.com/cloadex-cli/cloadex/internal/runner"
)

func TestParseJSON_StructuredPlan(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTasks int
		wantOK    bool
	}{
		{
			name: "valid JSON plan",
			input: `{
				"overview": "Build auth system",
				"tasks": [
					{"owner_ai": "claude", "description": "implement backend", "scope": ["internal/auth/"]},
					{"owner_ai": "codex", "description": "build login form", "scope": ["web/login.tsx"]}
				]
			}`,
			wantTasks: 2,
			wantOK:    true,
		},
		{
			name:      "JSON embedded in prose",
			input:     `Here is the plan:\n` + `{"overview":"do stuff","tasks":[{"owner_ai":"claude","description":"task 1"}]}` + `\nEnd of plan.`,
			wantTasks: 1,
			wantOK:    true,
		},
		{
			name:      "empty tasks array",
			input:     `{"overview":"nothing","tasks":[]}`,
			wantTasks: 0,
			wantOK:    false,
		},
		{
			name:      "no JSON at all",
			input:     "This is plain text with no JSON.",
			wantTasks: 0,
			wantOK:    false,
		},
		{
			name:      "malformed JSON",
			input:     `{"overview": "broken", "tasks": [{"owner_ai": "claude"`,
			wantTasks: 0,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := parseJSON(tt.input)
			if ok != tt.wantOK {
				t.Errorf("parseJSON() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && len(p.Tasks) != tt.wantTasks {
				t.Errorf("parseJSON() got %d tasks, want %d", len(p.Tasks), tt.wantTasks)
			}
		})
	}
}

func TestParseJSON_NormalizesAI(t *testing.T) {
	input := `{"overview":"x","tasks":[
		{"owner_ai":"Claude","description":"a"},
		{"owner_ai":"CODEX","description":"b"},
		{"owner_ai":"unknown","description":"c"}
	]}`

	p, ok := parseJSON(input)
	if !ok {
		t.Fatal("expected parseJSON to succeed")
	}

	want := []runner.AI{runner.Claude, runner.Codex, runner.Claude}
	for i, task := range p.Tasks {
		if task.OwnerAI != want[i] {
			t.Errorf("task %d: OwnerAI = %q, want %q", i, task.OwnerAI, want[i])
		}
	}
}

func TestParseMarkdown(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTasks  int
		wantClaude int
		wantCodex  int
	}{
		{
			name: "standard markdown with headers",
			input: `This is the plan overview.

## Claude Tasks:
- Implement the auth middleware
- Write database migrations

## Codex Tasks:
- Build the login page
- Style the dashboard`,
			wantTasks:  4,
			wantClaude: 2,
			wantCodex:  2,
		},
		{
			name: "bold labels with AI names",
			input: `Overview here.

**Claude Tasks:**
1. Set up the API routes
2. Add rate limiting

**Codex Tasks:**
- Create form component`,
			wantTasks:  3,
			wantClaude: 2,
			wantCodex:  1,
		},
		{
			name: "logic and UI/UX task labels",
			input: `The plan:

## Logic Tasks:
- Parse config files

## UI/UX Tasks:
- Render settings page`,
			wantTasks:  2,
			wantClaude: 1,
			wantCodex:  1,
		},
		{
			name:       "no recognizable structure — fallback",
			input:      "Just do the thing please.",
			wantTasks:  2, // fallback creates one per AI
			wantClaude: 1,
			wantCodex:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parseMarkdown(tt.input)
			if len(p.Tasks) != tt.wantTasks {
				t.Errorf("got %d tasks, want %d", len(p.Tasks), tt.wantTasks)
			}
			claude := TasksForAI(p.Tasks, runner.Claude)
			codex := TasksForAI(p.Tasks, runner.Codex)
			if len(claude) != tt.wantClaude {
				t.Errorf("claude tasks = %d, want %d", len(claude), tt.wantClaude)
			}
			if len(codex) != tt.wantCodex {
				t.Errorf("codex tasks = %d, want %d", len(codex), tt.wantCodex)
			}
		})
	}
}

func TestParseMarkdown_Overview(t *testing.T) {
	input := `# Plan

This is the overview sentence.

## Claude Tasks:
- Do something`

	p := parseMarkdown(input)
	if p.Overview != "This is the overview sentence." {
		t.Errorf("overview = %q, want %q", p.Overview, "This is the overview sentence.")
	}
}

func TestParsePlan_PrefersJSON(t *testing.T) {
	// When input contains valid JSON, ParsePlan should use it
	input := `Here's the plan:
{"overview":"JSON plan","tasks":[{"owner_ai":"claude","description":"task from JSON"}]}

## Codex Tasks:
- this should be ignored`

	p := ParsePlan(input)
	if len(p.Tasks) != 1 {
		t.Fatalf("expected 1 task from JSON, got %d", len(p.Tasks))
	}
	if p.Tasks[0].Description != "task from JSON" {
		t.Errorf("got description %q, want %q", p.Tasks[0].Description, "task from JSON")
	}
}

func TestIsTaskLine(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"- a bullet", true},
		{"* asterisk bullet", true},
		{"1. numbered", true},
		{"9. high number", true},
		{"not a task", false},
		{"## heading", false},
		{"", false},
		{"-- double dash", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isTaskLine(tt.input); got != tt.want {
				t.Errorf("isTaskLine(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripBullet(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"- hello", "hello"},
		{"* world", "world"},
		{"1. numbered task", "numbered task"},
		{"3. third", "third"},
		{"plain text", "plain text"},
		{"-  extra space", "extra space"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripBullet(tt.input); got != tt.want {
				t.Errorf("stripBullet(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsHeadingOrLabel(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"# Heading", true},
		{"## Sub heading", true},
		{"**Bold label**", true},
		{"Tasks:", true},
		{"plain text", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isHeadingOrLabel(tt.input); got != tt.want {
				t.Errorf("isHeadingOrLabel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s    string
		subs []string
		want bool
	}{
		{"claude tasks", []string{"claude", "codex"}, true},
		{"codex work", []string{"claude", "codex"}, true},
		{"other stuff", []string{"claude", "codex"}, false},
		{"", []string{"a"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := containsAny(tt.s, tt.subs...); got != tt.want {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.subs, got, tt.want)
			}
		})
	}
}

func TestNormalizeAI(t *testing.T) {
	tests := []struct {
		input runner.AI
		want  runner.AI
	}{
		{"claude", runner.Claude},
		{"Claude", runner.Claude},
		{"CLAUDE", runner.Claude},
		{"codex", runner.Codex},
		{"Codex", runner.Codex},
		{"CODEX", runner.Codex},
		{"gpt", runner.Claude}, // unknown defaults to claude
		{"", runner.Claude},    // empty defaults to claude
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			if got := normalizeAI(tt.input); got != tt.want {
				t.Errorf("normalizeAI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTasksForAI(t *testing.T) {
	tasks := []Task{
		{OwnerAI: runner.Claude, Description: "c1"},
		{OwnerAI: runner.Codex, Description: "x1"},
		{OwnerAI: runner.Claude, Description: "c2"},
		{OwnerAI: runner.Codex, Description: "x2"},
		{OwnerAI: runner.Claude, Description: "c3"},
	}

	claude := TasksForAI(tasks, runner.Claude)
	codex := TasksForAI(tasks, runner.Codex)

	if len(claude) != 3 {
		t.Errorf("claude tasks = %d, want 3", len(claude))
	}
	if len(codex) != 2 {
		t.Errorf("codex tasks = %d, want 2", len(codex))
	}
}

func TestTasksForAI_Empty(t *testing.T) {
	result := TasksForAI(nil, runner.Claude)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestParseMarkdown_FallbackForUnrecognizedLabels(t *testing.T) {
	// Labels like "Backend Tasks:" and "Frontend Tasks:" don't contain
	// "claude" or "codex", so the parser falls back to one task per AI.
	input := `Overview here.

**Backend Tasks:**
1. Set up the API routes
2. Add rate limiting

**Frontend Tasks:**
- Create form component`

	p := parseMarkdown(input)
	if len(p.Tasks) != 2 {
		t.Errorf("got %d tasks, want 2 (fallback)", len(p.Tasks))
	}
	// Fallback creates one codex + one claude task
	claude := TasksForAI(p.Tasks, runner.Claude)
	codex := TasksForAI(p.Tasks, runner.Codex)
	if len(claude) != 1 {
		t.Errorf("claude tasks = %d, want 1", len(claude))
	}
	if len(codex) != 1 {
		t.Errorf("codex tasks = %d, want 1", len(codex))
	}
}

func TestParseMarkdown_MixedCaseHeaders(t *testing.T) {
	input := `Plan overview.

## CLAUDE tasks:
- Backend work

## CODEX tasks:
- Frontend work`

	p := parseMarkdown(input)
	claude := TasksForAI(p.Tasks, runner.Claude)
	codex := TasksForAI(p.Tasks, runner.Codex)
	if len(claude) != 1 {
		t.Errorf("claude tasks = %d, want 1", len(claude))
	}
	if len(codex) != 1 {
		t.Errorf("codex tasks = %d, want 1", len(codex))
	}
}

func TestParseJSON_NestedBraces(t *testing.T) {
	// JSON with nested objects in description
	input := `{"overview":"test","tasks":[{"owner_ai":"claude","description":"handle {curly} braces"}]}`
	p, ok := parseJSON(input)
	if !ok {
		t.Fatal("expected parseJSON to succeed with nested braces")
	}
	if len(p.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(p.Tasks))
	}
	if !strings.Contains(p.Tasks[0].Description, "{curly}") {
		t.Errorf("description = %q, expected to contain {curly}", p.Tasks[0].Description)
	}
}

func TestParsePlan_FallsBackToMarkdown(t *testing.T) {
	input := `## Claude Tasks:
- Do backend work

## Codex Tasks:
- Do frontend work`

	p := ParsePlan(input)
	if len(p.Tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(p.Tasks))
	}
}

func TestParsePlan_FullJSON_WithDependencies(t *testing.T) {
	input := `{
		"overview": "Full stack auth",
		"tasks": [
			{
				"owner_ai": "claude",
				"description": "implement JWT auth",
				"scope": ["internal/auth/"],
				"depends_on": [],
				"verification": "go test ./internal/auth/..."
			},
			{
				"owner_ai": "codex",
				"description": "build login UI",
				"scope": ["web/login.tsx"],
				"depends_on": [0],
				"verification": "npm test"
			}
		]
	}`

	p := ParsePlan(input)
	if p.Overview != "Full stack auth" {
		t.Errorf("overview = %q", p.Overview)
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(p.Tasks))
	}
	if len(p.Tasks[1].DependsOn) != 1 || p.Tasks[1].DependsOn[0] != 0 {
		t.Errorf("task 1 depends_on = %v, want [0]", p.Tasks[1].DependsOn)
	}
	if p.Tasks[0].Verification != "go test ./internal/auth/..." {
		t.Errorf("task 0 verification = %q", p.Tasks[0].Verification)
	}
}
