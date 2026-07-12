package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestExtractClaudeText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "assistant event with text content",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
			want:  "Hello world",
		},
		{
			name:  "assistant event with multiple text blocks",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"line 1"},{"type":"text","text":"line 2"}]}}`,
			want:  "line 1\nline 2",
		},
		{
			name:  "assistant event with non-text content",
			input: `{"type":"assistant","message":{"content":[{"type":"tool_use","text":"ignored"}]}}`,
			want:  "",
		},
		{
			name:  "assistant event with mixed content",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"kept"},{"type":"tool_use","text":"dropped"}]}}`,
			want:  "kept",
		},
		{
			name:  "result event returns empty",
			input: `{"type":"result","result":"final text"}`,
			want:  "",
		},
		{
			name:  "unknown event type",
			input: `{"type":"system","message":"hello"}`,
			want:  "",
		},
		{
			name:  "empty content array",
			input: `{"type":"assistant","message":{"content":[]}}`,
			want:  "",
		},
		{
			name:  "malformed JSON",
			input: `not json at all`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractClaudeText(tt.input); got != tt.want {
				t.Errorf("extractClaudeText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractCodexText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "item.completed with text",
			input: `{"type":"item.completed","item":{"type":"message","text":"Codex output"}}`,
			want:  "Codex output",
		},
		{
			name:  "item.completed with empty text",
			input: `{"type":"item.completed","item":{"type":"message","text":""}}`,
			want:  "",
		},
		{
			name:  "different event type",
			input: `{"type":"item.started","item":{"text":"not completed"}}`,
			want:  "",
		},
		{
			name:  "malformed JSON",
			input: `{broken`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractCodexText(tt.input); got != tt.want {
				t.Errorf("extractCodexText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name  string
		ai    AI
		input string
		want  string
	}{
		{
			name:  "claude assistant event",
			ai:    Claude,
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`,
			want:  "hello",
		},
		{
			name:  "codex item.completed event",
			ai:    Codex,
			input: `{"type":"item.completed","item":{"text":"world"}}`,
			want:  "world",
		},
		{
			name:  "empty line",
			ai:    Claude,
			input: "",
			want:  "",
		},
		{
			name:  "whitespace-only line",
			ai:    Claude,
			input: "   ",
			want:  "",
		},
		{
			name:  "non-JSON line",
			ai:    Claude,
			input: "plain text output",
			want:  "",
		},
		{
			name:  "unknown AI",
			ai:    AI("gemini"),
			input: `{"type":"text","text":"ignored"}`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractText(tt.ai, tt.input); got != tt.want {
				t.Errorf("extractText(%s, %q) = %q, want %q", tt.ai, tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildCommand(t *testing.T) {
	tests := []struct {
		name    string
		ai      AI
		prompt  string
		wantErr bool
	}{
		{name: "claude", ai: Claude, prompt: "hello"},
		{name: "codex", ai: Codex, prompt: "hello"},
		{name: "unknown", ai: AI("gemini"), prompt: "hello", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := buildCommand(context.Background(), tt.ai, tt.prompt)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var re *RunError
				if !errors.As(err, &re) {
					t.Errorf("expected *RunError, got %T", err)
				} else if re.Retryable {
					t.Error("unknown provider error should not be retryable")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd == nil {
				t.Fatal("expected non-nil cmd")
			}
		})
	}
}

func TestIsRetryableFailure(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stderr   string
		want     bool
	}{
		{name: "rate limit", exitCode: 1, stderr: "Error: Rate limit exceeded", want: true},
		{name: "429", exitCode: 1, stderr: "HTTP 429 Too Many Requests", want: true},
		{name: "too many requests", exitCode: 1, stderr: "too many requests", want: true},
		{name: "overloaded", exitCode: 1, stderr: "API is overloaded", want: true},
		{name: "500 server error", exitCode: 1, stderr: "500 Internal Server Error", want: true},
		{name: "502 bad gateway", exitCode: 1, stderr: "502 Bad Gateway", want: true},
		{name: "503 service unavailable", exitCode: 1, stderr: "503 Service Unavailable", want: true},
		{name: "timeout", exitCode: 1, stderr: "request timed out", want: true},
		{name: "connection refused", exitCode: 1, stderr: "connection refused", want: true},
		{name: "connection reset", exitCode: 1, stderr: "ECONNRESET", want: true},
		{name: "network error", exitCode: 1, stderr: "network error", want: true},
		{name: "syntax error", exitCode: 1, stderr: "SyntaxError: unexpected token", want: false},
		{name: "auth error", exitCode: 1, stderr: "authentication failed", want: false},
		{name: "empty stderr", exitCode: 1, stderr: "", want: false},
		{name: "clean exit", exitCode: 0, stderr: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableFailure(tt.exitCode, tt.stderr); got != tt.want {
				t.Errorf("isRetryableFailure(%d, %q) = %v, want %v", tt.exitCode, tt.stderr, got, tt.want)
			}
		})
	}
}

func TestRunError(t *testing.T) {
	t.Run("error message with stderr", func(t *testing.T) {
		err := &RunError{
			AI:        Claude,
			ExitCode:  1,
			Stderr:    "something went wrong",
			Retryable: true,
			Cause:     fmt.Errorf("exit status 1"),
		}
		msg := err.Error()
		if msg == "" {
			t.Error("expected non-empty error message")
		}
		if !errors.Is(err, err.Cause) {
			t.Error("Unwrap should return Cause")
		}
	})

	t.Run("error message without stderr", func(t *testing.T) {
		err := &RunError{
			AI:       Codex,
			ExitCode: 2,
			Cause:    fmt.Errorf("exit status 2"),
		}
		msg := err.Error()
		if msg == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("IsRetryable via errors.As", func(t *testing.T) {
		retryable := &RunError{AI: Claude, Retryable: true, Cause: fmt.Errorf("rate limited")}
		notRetryable := &RunError{AI: Claude, Retryable: false, Cause: fmt.Errorf("auth failed")}
		plainErr := fmt.Errorf("some other error")

		if !IsRetryable(retryable) {
			t.Error("expected retryable error to be retryable")
		}
		if IsRetryable(notRetryable) {
			t.Error("expected non-retryable error to not be retryable")
		}
		if IsRetryable(plainErr) {
			t.Error("expected plain error to not be retryable")
		}
	})
}

func TestClassifyStartError(t *testing.T) {
	t.Run("ErrNotFound", func(t *testing.T) {
		err := classifyStartError(Claude, exec.ErrNotFound)
		if err.Retryable {
			t.Error("ErrNotFound should not be retryable")
		}
	})

	t.Run("other error", func(t *testing.T) {
		err := classifyStartError(Codex, fmt.Errorf("permission denied"))
		if err.Retryable {
			t.Error("permission denied should not be retryable")
		}
	})
}

func TestClassifyExitError_CodexInvalidRefreshToken(t *testing.T) {
	err := classifyExitError(Codex, errors.New("signal: killed"), `ERROR rmcp::transport::worker: worker quit with fatal: Transport channel closed, when Auth(TokenRefreshFailed("Server returned error response: invalid_grant: Invalid refresh token"))`, "")
	if err.Retryable {
		t.Fatal("auth failure should not be retryable")
	}
	if !strings.Contains(err.Error(), "codex logout") || !strings.Contains(err.Error(), "codex login") {
		t.Fatalf("expected actionable codex auth guidance, got: %s", err.Error())
	}
	if strings.Contains(strings.ToLower(err.Error()), "invalid_grant") {
		t.Fatalf("expected raw transport error to be suppressed, got: %s", err.Error())
	}
}

func TestClassifyExitError_CodexNotLoggedIn(t *testing.T) {
	err := classifyExitError(Codex, errors.New("exit status 1"), "authentication error: not logged in", "")
	if !strings.Contains(err.Error(), "codex login") {
		t.Fatalf("expected codex login guidance, got: %s", err.Error())
	}
}

func TestClassifyExitError_ClaudeNotLoggedInFromOutput(t *testing.T) {
	err := classifyExitError(Claude, errors.New("signal: killed"), "", `{"type":"assistant","message":{"content":[{"type":"text","text":"Not logged in · Please run /login"}]},"error":"authentication_failed"}`)
	if !strings.Contains(err.Error(), "claude auth login") {
		t.Fatalf("expected Claude login guidance, got: %s", err.Error())
	}
}

func TestClassifyExitError_ClaudeSessionEnvPermissions(t *testing.T) {
	err := classifyExitError(Claude, errors.New("signal: killed"), "Failed to run: EPERM: operation not permitted, mkdir '/Users/ahmed/.claude/session-env/abc'", "")
	if !strings.Contains(err.Error(), "~/.claude/session-env") {
		t.Fatalf("expected Claude permissions guidance, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "claude auth login") {
		t.Fatalf("expected auth follow-up guidance in permissions message, got: %s", err.Error())
	}
}

func TestRetryDelay(t *testing.T) {
	d1 := retryDelay(1)
	d2 := retryDelay(2)
	d3 := retryDelay(3)

	if d1 != RetryBaseDelay {
		t.Errorf("attempt 1: got %v, want %v", d1, RetryBaseDelay)
	}
	if d2 <= d1 {
		t.Errorf("attempt 2 delay (%v) should be > attempt 1 (%v)", d2, d1)
	}
	if d3 <= d2 {
		t.Errorf("attempt 3 delay (%v) should be > attempt 2 (%v)", d3, d2)
	}

	// Very high attempt should be capped
	dHigh := retryDelay(100)
	if dHigh > RetryMaxDelay {
		t.Errorf("delay should be capped at %v, got %v", RetryMaxDelay, dHigh)
	}
}

func TestDefaultTimeout(t *testing.T) {
	if got := defaultTimeout(Claude); got != DefaultClaudeTimeout {
		t.Errorf("Claude timeout = %v, want %v", got, DefaultClaudeTimeout)
	}
	if got := defaultTimeout(Codex); got != DefaultCodexTimeout {
		t.Errorf("Codex timeout = %v, want %v", got, DefaultCodexTimeout)
	}
	if got := defaultTimeout(AI("unknown")); got != DefaultClaudeTimeout {
		t.Errorf("unknown timeout = %v, want %v (fallback)", got, DefaultClaudeTimeout)
	}
}

func TestSessionPrompt(t *testing.T) {
	history := []SessionMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
	}
	got := sessionPrompt(history, "third")
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(got, want) {
			t.Fatalf("session prompt missing %q", want)
		}
	}
}

func TestAgentSessionSnapshotAndResume(t *testing.T) {
	session := NewSession(Claude, nil, DefaultSessionOptions())
	session.record("user", "prompt")
	session.record("assistant", "answer")
	session.Interrupt()
	snapshot := session.Snapshot()
	if !snapshot.Interrupted {
		t.Fatal("expected interrupted snapshot")
	}
	restored := NewSession(Claude, &snapshot, DefaultSessionOptions())
	restored.Resume()
	restoredSnapshot := restored.Snapshot()
	if restoredSnapshot.Interrupted {
		t.Fatal("expected resumed session")
	}
	if len(restoredSnapshot.History) != 2 {
		t.Fatalf("history len = %d, want 2", len(restoredSnapshot.History))
	}
}

func TestSessionRegistry(t *testing.T) {
	ClearRegisteredSessions()
	session := NewSession(Codex, nil, DefaultSessionOptions())
	RegisterSession(session)
	if got := RegisteredSession(Codex); got == nil {
		t.Fatal("expected registered session")
	}
	UnregisterSession(Codex)
	if got := RegisteredSession(Codex); got != nil {
		t.Fatal("expected unregistered session")
	}
}
