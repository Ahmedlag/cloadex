package session

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestExitTerminatesLoop(t *testing.T) {
	in := strings.NewReader("/exit\n")
	var out bytes.Buffer

	s := &Session{In: in, Out: &out}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "Goodbye!") {
		t.Error("expected goodbye message on /exit")
	}
}

func TestEOFTerminatesLoop(t *testing.T) {
	in := strings.NewReader("") // immediate EOF
	var out bytes.Buffer

	s := &Session{In: in, Out: &out}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHelpPrintsCommands(t *testing.T) {
	in := strings.NewReader("/help\n/exit\n")
	var out bytes.Buffer

	s := &Session{In: in, Out: &out}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	for _, want := range []string{"/help", "/exit", "/resume", "/mode", "/score", "/agents", "/diff"} {
		if !strings.Contains(output, want) {
			t.Errorf("help output should mention %q", want)
		}
	}
}

func TestEmptyInputIsSkipped(t *testing.T) {
	in := strings.NewReader("\n   \n\n/exit\n")
	var out bytes.Buffer

	called := false
	s := &Session{
		In:  in,
		Out: &out,
		OnPrompt: func(prompt string) error {
			called = true
			return nil
		},
	}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("OnPrompt should not be called for empty/whitespace input")
	}
}

func TestPromptDelegation(t *testing.T) {
	in := strings.NewReader("build a login page\n/exit\n")
	var out bytes.Buffer

	var received string
	s := &Session{
		In:  in,
		Out: &out,
		OnPrompt: func(prompt string) error {
			received = prompt
			return nil
		},
	}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "build a login page" {
		t.Errorf("OnPrompt got %q, want %q", received, "build a login page")
	}
}

func TestMultiplePrompts(t *testing.T) {
	in := strings.NewReader("first prompt\nsecond prompt\n/exit\n")
	var out bytes.Buffer

	var prompts []string
	s := &Session{
		In:  in,
		Out: &out,
		OnPrompt: func(prompt string) error {
			prompts = append(prompts, prompt)
			return nil
		},
	}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(prompts))
	}
	if prompts[0] != "first prompt" || prompts[1] != "second prompt" {
		t.Errorf("prompts = %v", prompts)
	}
}

func TestPromptHandlerError(t *testing.T) {
	in := strings.NewReader("bad prompt\n/exit\n")
	var out bytes.Buffer

	s := &Session{
		In:  in,
		Out: &out,
		OnPrompt: func(prompt string) error {
			return errors.New("something broke")
		},
	}
	// Session should continue after handler errors, not abort.
	if err := s.Run(); err != nil {
		t.Fatalf("session should not abort on handler error: %v", err)
	}
	if !strings.Contains(out.String(), "Error: something broke") {
		t.Error("expected error message in output")
	}
}

func TestUnknownSlashCommand(t *testing.T) {
	in := strings.NewReader("/foobar\n/exit\n")
	var out bytes.Buffer

	s := &Session{In: in, Out: &out}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Unknown command: /foobar") {
		t.Error("expected unknown command message")
	}
}

func TestCustomSlashCommand(t *testing.T) {
	in := strings.NewReader("/score\n/exit\n")
	var out bytes.Buffer

	var got SlashCommand
	s := &Session{
		In:  in,
		Out: &out,
		OnCommand: func(command SlashCommand, args string) error {
			got = command
			return nil
		},
	}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CmdScore {
		t.Fatalf("got %q, want %q", got, CmdScore)
	}
}

func TestResumeCommand(t *testing.T) {
	in := strings.NewReader("/resume\n/exit\n")
	var out bytes.Buffer

	resumed := false
	s := &Session{
		In:  in,
		Out: &out,
		OnResume: func() error {
			resumed = true
			return nil
		},
	}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resumed {
		t.Error("expected OnResume to be called")
	}
}

func TestResumeCommandNoHandler(t *testing.T) {
	in := strings.NewReader("/resume\n/exit\n")
	var out bytes.Buffer

	s := &Session{In: in, Out: &out}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "No resume handler configured") {
		t.Error("expected fallback message when OnResume is nil")
	}
}

func TestResumeCommandError(t *testing.T) {
	in := strings.NewReader("/resume\n/exit\n")
	var out bytes.Buffer

	s := &Session{
		In:  in,
		Out: &out,
		OnResume: func() error {
			return errors.New("no incomplete runs")
		},
	}
	if err := s.Run(); err != nil {
		t.Fatalf("session should not abort on resume error: %v", err)
	}
	if !strings.Contains(out.String(), "Resume error: no incomplete runs") {
		t.Error("expected resume error message in output")
	}
}

func TestPromptShownEachIteration(t *testing.T) {
	in := strings.NewReader("/exit\n")
	var out bytes.Buffer

	s := &Session{In: in, Out: &out}
	_ = s.Run()

	if !strings.Contains(out.String(), "cloadex> ") {
		t.Error("expected prompt marker 'cloadex> ' in output")
	}
}

func TestPromptRenderer(t *testing.T) {
	in := strings.NewReader("/exit\n")
	var out bytes.Buffer

	s := &Session{
		In:  in,
		Out: &out,
		PromptRenderer: func() string {
			return "cloadex[chat]> "
		},
	}
	_ = s.Run()
	if !strings.Contains(out.String(), "cloadex[chat]> ") {
		t.Fatal("expected rendered prompt")
	}
}

func TestNilOnPromptDoesNotPanic(t *testing.T) {
	in := strings.NewReader("some input\n/exit\n")
	var out bytes.Buffer

	s := &Session{In: in, Out: &out, OnPrompt: nil}
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should silently ignore prompts when no handler is set.
}

func TestSlashCommandConstants(t *testing.T) {
	if CmdExit != "/exit" {
		t.Errorf("CmdExit = %q, want /exit", CmdExit)
	}
	if CmdHelp != "/help" {
		t.Errorf("CmdHelp = %q, want /help", CmdHelp)
	}
	if CmdResume != "/resume" {
		t.Errorf("CmdResume = %q, want /resume", CmdResume)
	}
	if CmdMode != "/mode" {
		t.Errorf("CmdMode = %q, want /mode", CmdMode)
	}
	if CmdScore != "/score" {
		t.Errorf("CmdScore = %q, want /score", CmdScore)
	}
}

func TestSlashCommandCaseExact(t *testing.T) {
	// /EXIT should not be recognized as /exit — commands are case-sensitive.
	in := strings.NewReader("/EXIT\n/exit\n")
	var out bytes.Buffer

	s := &Session{In: in, Out: &out}
	_ = s.Run()

	if !strings.Contains(out.String(), "Unknown command: /EXIT") {
		t.Error("uppercase /EXIT should be treated as unknown command")
	}
}

func TestInputTrimming(t *testing.T) {
	in := strings.NewReader("  padded prompt  \n/exit\n")
	var out bytes.Buffer

	var received string
	s := &Session{
		In:  in,
		Out: &out,
		OnPrompt: func(prompt string) error {
			received = prompt
			return nil
		},
	}
	_ = s.Run()

	if received != "padded prompt" {
		t.Errorf("expected trimmed prompt, got %q", received)
	}
}
