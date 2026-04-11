package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloadex-cli/cloadex/internal/runner"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single line no newline",
			input: "hello",
			want:  []string{"hello"},
		},
		{
			name:  "single line with newline",
			input: "hello\n",
			want:  []string{"hello"},
		},
		{
			name:  "multiple lines",
			input: "line1\nline2\nline3",
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "multiple lines trailing newline",
			input: "a\nb\n",
			want:  []string{"a", "b"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "only newlines",
			input: "\n\n",
			want:  []string{"", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitLines(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSaveRun(t *testing.T) {
	// Run in a temp directory to avoid polluting the real workspace
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	dir, err := SaveRun(
		"build a CLI tool",
		"## Plan\n- Task 1",
		"debate history here",
		"execution output",
		"validation passed",
	)
	if err != nil {
		t.Fatalf("SaveRun error: %v", err)
	}

	if dir == "" {
		t.Fatal("expected non-empty dir path")
	}

	// Check files exist with correct content
	wantFiles := map[string]string{
		"prompt.txt":    "build a CLI tool",
		"plan.md":       "## Plan\n- Task 1",
		"debate.md":     "debate history here",
		"execution.md":  "execution output",
		"validation.md": "validation passed",
	}

	for name, wantContent := range wantFiles {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("reading %s: %v", name, err)
			continue
		}
		if string(data) != wantContent {
			t.Errorf("%s content = %q, want %q", name, string(data), wantContent)
		}
	}
}

func TestSaveRun_SkipsEmptyContent(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	dir, err := SaveRun("prompt", "plan", "", "", "")
	if err != nil {
		t.Fatalf("SaveRun error: %v", err)
	}

	// Only prompt.txt and plan.md should exist
	entries, _ := os.ReadDir(dir)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}

	if !names["prompt.txt"] {
		t.Error("expected prompt.txt")
	}
	if !names["plan.md"] {
		t.Error("expected plan.md")
	}
	if names["debate.md"] {
		t.Error("debate.md should not exist for empty content")
	}
	if names["execution.md"] {
		t.Error("execution.md should not exist for empty content")
	}
	if names["validation.md"] {
		t.Error("validation.md should not exist for empty content")
	}
}

func TestLatestPlan(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// No runs yet - should error
	_, err := LatestPlan()
	if err == nil {
		t.Error("expected error when no runs exist")
	}

	// Create two runs
	dir1, _ := SaveRun("p1", "first plan", "", "", "")
	_ = dir1

	// Create a second run with a slightly later timestamp
	// We need to ensure the second directory sorts after the first
	runsDir := filepath.Join(cloadexDir, "runs")
	secondDir := filepath.Join(runsDir, "29991231-235959")
	os.MkdirAll(secondDir, 0o755)
	os.WriteFile(filepath.Join(secondDir, "plan.md"), []byte("latest plan"), 0o644)

	plan, err := LatestPlan()
	if err != nil {
		t.Fatalf("LatestPlan error: %v", err)
	}
	if plan != "latest plan" {
		t.Errorf("LatestPlan() = %q, want %q", plan, "latest plan")
	}
}

func TestEnsureGitignore(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// No .gitignore exists - should create one
	EnsureGitignore()
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(data), ".cloadex/") {
		t.Error("expected .cloadex/ in .gitignore")
	}

	// Calling again should not duplicate the entry
	EnsureGitignore()
	data, _ = os.ReadFile(".gitignore")
	count := strings.Count(string(data), ".cloadex/")
	if count != 1 {
		t.Errorf("expected 1 occurrence of .cloadex/, got %d", count)
	}
}

func TestEnsureGitignore_ExistingFileNoTrailingNewline(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create .gitignore without trailing newline
	os.WriteFile(".gitignore", []byte("node_modules/"), 0o644)

	EnsureGitignore()
	data, _ := os.ReadFile(".gitignore")
	content := string(data)

	if !strings.Contains(content, "node_modules/") {
		t.Error("should preserve existing content")
	}
	if !strings.Contains(content, ".cloadex/") {
		t.Error("should add .cloadex/")
	}
	// Should have a newline between existing content and new entry
	if strings.Contains(content, "node_modules/.cloadex/") {
		t.Error("should add newline before .cloadex/ entry")
	}
}

func TestEnsureGitignore_AlreadyPresent(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	os.WriteFile(".gitignore", []byte(".cloadex/\n"), 0o644)

	EnsureGitignore()
	data, _ := os.ReadFile(".gitignore")
	if strings.Count(string(data), ".cloadex/") != 1 {
		t.Error("should not duplicate .cloadex/ entry")
	}
}

func TestSavePendingDecision(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	manifest, err := CreateRun("prompt")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	decision := &PendingDecision{
		Issue: "broken branch",
		OptionOne: ProposalOption{
			AI:         runner.Claude,
			Cause:      "cause",
			FixSummary: "fix",
		},
		OptionTwo: ProposalOption{
			AI:         runner.Codex,
			Cause:      "cause",
			FixSummary: "fix other",
		},
	}
	if err := SavePendingDecision(manifest.ID, decision); err != nil {
		t.Fatalf("SavePendingDecision: %v", err)
	}

	loaded, err := LoadManifest(manifest.ID)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if loaded.PendingDecision == nil {
		t.Fatal("expected pending decision")
	}
	if loaded.Status != StatusWaitingInput {
		t.Fatalf("status = %s, want %s", loaded.Status, StatusWaitingInput)
	}
}

func TestSaveExecutionState(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	manifest, err := CreateRun("prompt")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{"status": "ok"})
	if err := SaveExecutionState(manifest.ID, payload); err != nil {
		t.Fatalf("SaveExecutionState: %v", err)
	}
	data, err := LoadExecutionState(manifest.ID)
	if err != nil {
		t.Fatalf("LoadExecutionState: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("execution state = %s, want %s", string(data), string(payload))
	}
}

func TestListRunsMigratesLegacyWorkspaceDir(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	legacyRunDir := filepath.Join(".wizdo", "runs", "19990101-000000")
	if err := os.MkdirAll(legacyRunDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRunDir, "prompt.txt"), []byte("legacy prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRunDir, "plan.md"), []byte("legacy plan"), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	runs, err := ListRuns()
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "19990101-000000" {
		t.Fatalf("runs = %#v, want migrated legacy run", runs)
	}
	if _, err := os.Stat(filepath.Join(".cloadex", "runs", "19990101-000000", "plan.md")); err != nil {
		t.Fatalf("expected migrated plan under .cloadex: %v", err)
	}
	if _, err := os.Stat(".wizdo"); !os.IsNotExist(err) {
		t.Fatalf("expected legacy .wizdo dir removed after migration, got %v", err)
	}
}
