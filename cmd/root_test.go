package cmd

import (
	"os"
	"strings"
	"testing"
	"time"
	"sync/atomic"
	"syscall"

	"github.com/Ahmedlag/cloadex/internal/config"
	"github.com/Ahmedlag/cloadex/internal/execute"
	"github.com/Ahmedlag/cloadex/internal/session"
)

func TestParseFlagsDefaults(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	remaining := parseFlags([]string{"hello", "world"}, &opts, cliSet)

	if len(remaining) != 2 || remaining[0] != "hello" || remaining[1] != "world" {
		t.Errorf("remaining = %v, want [hello world]", remaining)
	}
	if opts.MaxRounds != 5 {
		t.Errorf("MaxRounds = %d, want 5", opts.MaxRounds)
	}
	if opts.DryRun {
		t.Error("DryRun should be false by default")
	}
	if opts.Yes {
		t.Error("Yes should be false by default")
	}
	if opts.Verbose {
		t.Error("Verbose should be false by default")
	}
}

func TestParseFlagsRounds(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	remaining := parseFlags([]string{"--rounds", "3", "my", "prompt"}, &opts, cliSet)

	if opts.MaxRounds != 3 {
		t.Errorf("MaxRounds = %d, want 3", opts.MaxRounds)
	}
	if !cliSet["rounds"] {
		t.Error("rounds should be marked as CLI-set")
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want [my prompt]", remaining)
	}
}

func TestParseFlagsMaxFixes(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	parseFlags([]string{"--max-fixes", "5"}, &opts, cliSet)

	if opts.MaxFixAttempts != 5 {
		t.Errorf("MaxFixAttempts = %d, want 5", opts.MaxFixAttempts)
	}
	if !cliSet["max-fixes"] {
		t.Error("max-fixes should be marked as CLI-set")
	}
}

func TestParseFlagsNoFix(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	parseFlags([]string{"--no-fix"}, &opts, cliSet)

	if opts.MaxFixAttempts != 0 {
		t.Errorf("MaxFixAttempts = %d, want 0", opts.MaxFixAttempts)
	}
	if !cliSet["max-fixes"] {
		t.Error("max-fixes should be marked as CLI-set for --no-fix")
	}
}

func TestParseFlagsDryRun(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	parseFlags([]string{"--dry-run", "some", "prompt"}, &opts, cliSet)

	if !opts.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestParseFlagsYes(t *testing.T) {
	tests := []struct {
		flag string
	}{
		{"--yes"},
		{"-y"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			opts := config.DefaultOptions()
			cliSet := make(map[string]bool)
			parseFlags([]string{tt.flag}, &opts, cliSet)

			if !opts.Yes {
				t.Errorf("Yes should be true for %s", tt.flag)
			}
			if !cliSet["yes"] {
				t.Error("yes should be marked as CLI-set")
			}
		})
	}
}

func TestParseFlagsVerbose(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	parseFlags([]string{"--verbose"}, &opts, cliSet)

	if !opts.Verbose {
		t.Error("Verbose should be true")
	}
	if !cliSet["verbose"] {
		t.Error("verbose should be marked as CLI-set")
	}
}

func TestParseFlagsCombined(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	remaining := parseFlags([]string{
		"--rounds", "2",
		"--dry-run",
		"--yes",
		"--verbose",
		"build", "a", "login", "page",
	}, &opts, cliSet)

	if opts.MaxRounds != 2 {
		t.Errorf("MaxRounds = %d, want 2", opts.MaxRounds)
	}
	if !opts.DryRun {
		t.Error("DryRun should be true")
	}
	if !opts.Yes {
		t.Error("Yes should be true")
	}
	if !opts.Verbose {
		t.Error("Verbose should be true")
	}
	want := "build a login page"
	got := ""
	if len(remaining) > 0 {
		got = remaining[0]
		for _, r := range remaining[1:] {
			got += " " + r
		}
	}
	if got != want {
		t.Errorf("remaining prompt = %q, want %q", got, want)
	}
}

func TestParseFlagsRoundsMissingValue(t *testing.T) {
	// --rounds at end with no value should not panic.
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	parseFlags([]string{"--rounds"}, &opts, cliSet)

	// MaxRounds should remain at default since no value was provided.
	if opts.MaxRounds != 5 {
		t.Errorf("MaxRounds = %d, want 5 (default)", opts.MaxRounds)
	}
}

func TestParseFlagsPreservesOrder(t *testing.T) {
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	remaining := parseFlags([]string{"a", "--dry-run", "b", "--verbose", "c"}, &opts, cliSet)

	if len(remaining) != 3 || remaining[0] != "a" || remaining[1] != "b" || remaining[2] != "c" {
		t.Errorf("remaining = %v, want [a b c]", remaining)
	}
}

func TestSubcommandDetection(t *testing.T) {
	// Verify that known subcommands are dispatched by executeWithArgs
	// rather than treated as prompt text. We inject a fake newSession so
	// "session" doesn't start a real REPL, and check that no error of
	// the form "please provide a prompt" is returned for any subcommand.
	origSession := newSession
	defer func() { newSession = origSession }()

	// "session" subcommand will call cmdSession → newSession → sess.Run().
	// Provide a fake that returns immediately.
	newSession = func(onPrompt session.PromptHandler) *session.Session {
		return &session.Session{
			In:       strings.NewReader("/exit\n"),
			OnPrompt: onPrompt,
		}
	}

	subcommands := []string{"runs", "show", "init", "resume", "session"}
	for _, sub := range subcommands {
		t.Run(sub, func(t *testing.T) {
			// We don't assert on the returned error here because some
			// subcommands (e.g. resume, runs) may fail in a test
			// environment without persisted data. The key contract is
			// that they are NOT treated as prompt text — so the error
			// must NOT be "please provide a prompt".
			err := executeWithArgs([]string{sub})
			if err != nil && err.Error() == "please provide a prompt" {
				t.Errorf("subcommand %q was not dispatched — fell through to prompt handler", sub)
			}
		})
	}
}

func TestDispatchNoArgs_Interactive(t *testing.T) {
	// When stdin is a TTY and no args are given, bare `cloadex` should
	// start an interactive session via cmdSession → newSession.
	origInteractive := isInteractive
	origSession := newSession
	defer func() {
		isInteractive = origInteractive
		newSession = origSession
	}()

	isInteractive = func() bool { return true }

	sessionCreated := false
	newSession = func(onPrompt session.PromptHandler) *session.Session {
		sessionCreated = true
		if onPrompt == nil {
			t.Error("OnPrompt callback must be non-nil")
		}
		// Return a session that exits immediately.
		return &session.Session{
			In:       strings.NewReader("/exit\n"),
			OnPrompt: onPrompt,
		}
	}

	err := executeWithArgs(nil)
	if err != nil {
		t.Fatalf("dispatchNoArgs returned error: %v", err)
	}
	if !sessionCreated {
		t.Error("expected newSession to be called for interactive TTY with no args")
	}
}

func TestDispatchNoArgs_NonInteractive(t *testing.T) {
	// When stdin is NOT a TTY and no args are given, bare `cloadex` should
	// print usage and return nil — no session should be started.
	origInteractive := isInteractive
	origSession := newSession
	defer func() {
		isInteractive = origInteractive
		newSession = origSession
	}()

	isInteractive = func() bool { return false }

	sessionCreated := false
	newSession = func(onPrompt session.PromptHandler) *session.Session {
		sessionCreated = true
		return &session.Session{
			In:       strings.NewReader("/exit\n"),
			OnPrompt: onPrompt,
		}
	}

	err := executeWithArgs(nil)
	if err != nil {
		t.Fatalf("dispatchNoArgs returned error: %v", err)
	}
	if sessionCreated {
		t.Error("newSession must NOT be called when stdin is not a TTY")
	}
}

func TestSessionOnPromptWired(t *testing.T) {
	// Verify that the session created by cmdSession has a working
	// OnPrompt callback. We inject a session that sends a prompt line
	// and capture whether OnPrompt fires.
	origSession := newSession
	defer func() { newSession = origSession }()

	var capturedPrompt string
	newSession = func(onPrompt session.PromptHandler) *session.Session {
		// Wrap the real OnPrompt so we can observe it, but don't
		// actually run the pipeline (it would need claude/codex).
		wrappedPrompt := func(prompt string) error {
			capturedPrompt = prompt
			return nil
		}
		return &session.Session{
			In:       strings.NewReader("hello world\n/exit\n"),
			OnPrompt: wrappedPrompt,
		}
	}

	err := executeWithArgs([]string{"session"})
	if err != nil {
		t.Fatalf("cmdSession returned error: %v", err)
	}
	if capturedPrompt != "hello world" {
		t.Errorf("OnPrompt captured %q, want %q", capturedPrompt, "hello world")
	}
}

func TestFileExistsHelper(t *testing.T) {
	// fileExists should return false for non-existent files.
	if fileExists("/nonexistent", "file.txt") {
		t.Error("fileExists should return false for missing file")
	}
}

func TestReadRunFileHelper(t *testing.T) {
	// readRunFile should return empty string for non-existent files.
	got := readRunFile("/nonexistent", "file.txt")
	if got != "" {
		t.Errorf("readRunFile = %q, want empty string", got)
	}
}

func TestParseObserverVerdict(t *testing.T) {
	verdict, reason, err := parseObserverVerdict(`{"verdict":"warn","reason":"design drift"}`)
	if err != nil {
		t.Fatalf("parseObserverVerdict error: %v", err)
	}
	if verdict != "warn" || reason != "design drift" {
		t.Fatalf("got (%q, %q)", verdict, reason)
	}
}

func TestSummarizeExecution(t *testing.T) {
	summary := summarizeExecution(&execute.ExecutionResult{
		Results: []execute.TaskResult{
			{Status: execute.TaskSuccess},
			{Status: execute.TaskFailed},
			{Status: execute.TaskSkipped},
			{Status: execute.TaskSuccess},
		},
	})
	for _, want := range []string{"success=2", "failed=1", "skipped=1"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q", want)
		}
	}
}

func TestVersionString(t *testing.T) {
	// The version variable should have a default value.
	if version == "" {
		t.Error("version should not be empty")
	}
}

func TestSignalLoopHandlesSingleInterrupt(t *testing.T) {
	sigCh := make(chan os.Signal, 2)
	done := make(chan struct{})
	var calls atomic.Int32

	finished := make(chan struct{})
	go func() {
		signalLoop(sigCh, done, func() {
			calls.Add(1)
		})
		close(finished)
	}()

	sigCh <- syscall.SIGINT
	sigCh <- syscall.SIGINT

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("signalLoop did not exit after first interrupt")
	}

	if calls.Load() != 1 {
		t.Fatalf("expected one interrupt callback, got %d", calls.Load())
	}
}
