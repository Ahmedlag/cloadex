package score

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloadex-cli/cloadex/internal/runner"
)

func TestAddPointAndLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := AddPoint(runner.Claude, "plan"); err != nil {
		t.Fatalf("AddPoint plan: %v", err)
	}
	if err := AddPoint(runner.Claude, "execution"); err != nil {
		t.Fatalf("AddPoint execution: %v", err)
	}
	if err := AddPoint(runner.Codex, "fix"); err != nil {
		t.Fatalf("AddPoint fix: %v", err)
	}

	board, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if board.AIs["claude"].Total() != 2 {
		t.Fatalf("claude total = %d, want 2", board.AIs["claude"].Total())
	}
	if got := Label(runner.Claude); got != "Claude [2 | P:1 E:1 F:0]" {
		t.Fatalf("Label(claude) = %q", got)
	}
	if got := Label(runner.Codex); got != "Codex [1 | P:0 E:0 F:1]" {
		t.Fatalf("Label(codex) = %q", got)
	}

	if _, err := os.Stat(filepath.Join(home, ".cloadex", "scoreboard.json")); err != nil {
		t.Fatalf("scoreboard file not created: %v", err)
	}
}
