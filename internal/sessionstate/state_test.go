package sessionstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloadex-cli/cloadex/internal/runner"
)

func TestLoadOrInitCreatesState(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(tmp)
	defer os.Chdir(orig)

	state, err := LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit: %v", err)
	}
	if state.Mode != ModeChat {
		t.Fatalf("mode = %s, want %s", state.Mode, ModeChat)
	}
	if state.RepoPath == "" {
		t.Fatal("expected repo path")
	}
	if _, err := os.Stat(SessionFilePath()); err != nil {
		t.Fatalf("expected session file: %v", err)
	}
}

func TestSummaryForPrompt(t *testing.T) {
	state := &State{
		RepoSummary:   "repo",
		Mode:          ModeReview,
		LastRunID:     "20260411-120000",
		AgentSessions: map[string]runner.SessionSnapshot{},
	}
	state.Pin("approved_plan", "ship feature")
	state.RecordTurn("user", "fix auth")
	state.RecordEvent("observer_warn", "execution", "codex", "suspicious drift")
	summary := state.SummaryForPrompt()
	for _, want := range []string{"repo", "review", "20260411-120000"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q", want)
		}
	}
	for _, unwanted := range []string{"approved_plan", "fix auth", "observer_warn", "ship feature"} {
		if strings.Contains(summary, unwanted) {
			t.Fatalf("summary should not replay detailed history, found %q in %q", unwanted, summary)
		}
	}
}

func TestLoadOrInitInitializesAgentSessions(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(tmp)
	defer os.Chdir(orig)

	state, err := LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit: %v", err)
	}
	if state.AgentSessions == nil {
		t.Fatal("expected agent sessions map initialized")
	}
}

func TestValidMode(t *testing.T) {
	if _, ok := ValidMode("chat"); !ok {
		t.Fatal("expected chat mode to be valid")
	}
	if _, ok := ValidMode("bogus"); ok {
		t.Fatal("expected bogus mode to be invalid")
	}
}

func TestLoadOrInitMigratesLegacySessionFile(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(tmp)
	defer os.Chdir(orig)

	legacyDir := filepath.Join(".wizdo")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	legacyState := State{
		Version:       1,
		Mode:          ModeReview,
		RepoPath:      tmp,
		AgentSessions: map[string]runner.SessionSnapshot{},
	}
	data, err := json.Marshal(legacyState)
	if err != nil {
		t.Fatalf("marshal legacy state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, fileName), data, 0o644); err != nil {
		t.Fatalf("write legacy session: %v", err)
	}

	state, err := LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit: %v", err)
	}
	if state.Mode != ModeReview {
		t.Fatalf("mode = %s, want %s", state.Mode, ModeReview)
	}
	if _, err := os.Stat(filepath.Join(".cloadex", fileName)); err != nil {
		t.Fatalf("expected migrated session file: %v", err)
	}
	if _, err := os.Stat(".wizdo"); !os.IsNotExist(err) {
		t.Fatalf("expected legacy .wizdo dir removed after migration, got %v", err)
	}
}

func TestSaveUsesPrivatePermissions(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(tmp)
	defer os.Chdir(orig)

	state := &State{
		Version:       1,
		RepoPath:      tmp,
		Mode:          ModeChat,
		AgentSessions: map[string]runner.SessionSnapshot{},
	}
	if err := Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dirInfo, err := os.Stat(".cloadex")
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir perms = %#o, want 0700", dirInfo.Mode().Perm())
	}

	fileInfo, err := os.Stat(SessionFilePath())
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("session file perms = %#o, want 0600", fileInfo.Mode().Perm())
	}
}
