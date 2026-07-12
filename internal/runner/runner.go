package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type AI string

const (
	Claude AI = "claude"
	Codex  AI = "codex"
)

// Per-provider default timeouts. These apply when the parent context has no
// deadline of its own.
const (
	DefaultClaudeTimeout = 5 * time.Minute
	DefaultCodexTimeout  = 5 * time.Minute
)

// Retry defaults for transient failures.
const (
	DefaultMaxRetries  = 2
	RetryBaseDelay     = 2 * time.Second
	RetryMaxDelay      = 30 * time.Second
	RetryBackoffFactor = 2.0
)

type Result struct {
	AI     AI
	Output string
	Err    error
}

type SessionMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type SessionSnapshot struct {
	AI          AI               `json:"ai"`
	History     []SessionMessage `json:"history,omitempty"`
	Interrupted bool             `json:"interrupted,omitempty"`
}

type SessionOptions struct {
	RunOptions RunOptions
	MaxHistory int
}

type AgentSession struct {
	mu          sync.Mutex
	ai          AI
	history     []SessionMessage
	opts        SessionOptions
	interrupted bool
}

// RunError is a structured error from a provider invocation.
type RunError struct {
	AI        AI
	Retryable bool
	ExitCode  int
	Stderr    string
	Cause     error
}

func (e *RunError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("%s failed (exit %d): %s\n%s", e.AI, e.ExitCode, e.Cause, e.Stderr)
	}
	return fmt.Sprintf("%s failed (exit %d): %s", e.AI, e.ExitCode, e.Cause)
}

func (e *RunError) Unwrap() error { return e.Cause }

// IsRetryable reports whether an error from Run is worth retrying.
func IsRetryable(err error) bool {
	var re *RunError
	if errors.As(err, &re) {
		return re.Retryable
	}
	return false
}

// StreamCallback is called for each chunk of text output from the AI.
type StreamCallback func(ai AI, line string)

// RunOptions configures a single Run invocation.
type RunOptions struct {
	// MaxRetries is the number of retries for transient failures (0 = no retry).
	MaxRetries int
	// Timeout overrides the per-provider default timeout. Zero means use default.
	Timeout time.Duration
}

// DefaultRunOptions returns sensible defaults.
func DefaultRunOptions() RunOptions {
	return RunOptions{
		MaxRetries: DefaultMaxRetries,
	}
}

func DefaultSessionOptions() SessionOptions {
	return SessionOptions{
		RunOptions: DefaultRunOptions(),
		MaxHistory: 12,
	}
}

func NewSession(ai AI, snapshot *SessionSnapshot, opts SessionOptions) *AgentSession {
	if opts.MaxHistory <= 0 {
		opts = DefaultSessionOptions()
	}
	s := &AgentSession{
		ai:   ai,
		opts: opts,
	}
	if snapshot != nil {
		s.history = append(s.history, snapshot.History...)
		s.interrupted = snapshot.Interrupted
	}
	return s
}

func (s *AgentSession) Snapshot() SessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := append([]SessionMessage(nil), s.history...)
	return SessionSnapshot{
		AI:          s.ai,
		History:     history,
		Interrupted: s.interrupted,
	}
}

func (s *AgentSession) Send(ctx context.Context, prompt string, onLine StreamCallback) Result {
	s.mu.Lock()
	if s.interrupted {
		s.mu.Unlock()
		return Result{AI: s.ai, Err: fmt.Errorf("%s session interrupted; resume before sending more work", s.ai)}
	}
	history := append([]SessionMessage(nil), s.history...)
	opts := s.opts
	s.mu.Unlock()

	fullPrompt := prompt
	if contextPrompt := sessionPrompt(history, prompt); contextPrompt != "" {
		fullPrompt = contextPrompt
	}
	result := runDirectWithOptions(ctx, s.ai, fullPrompt, onLine, opts.RunOptions)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.record("user", prompt)
	if result.Err == nil && strings.TrimSpace(result.Output) != "" {
		s.record("assistant", result.Output)
	}
	return result
}

func (s *AgentSession) Interrupt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupted = true
}

func (s *AgentSession) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupted = false
}

func (s *AgentSession) record(role string, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	s.history = append(s.history, SessionMessage{
		Role:      role,
		Content:   strings.TrimSpace(content),
		CreatedAt: time.Now(),
	})
	maxHistory := s.opts.MaxHistory
	if maxHistory <= 0 {
		maxHistory = DefaultSessionOptions().MaxHistory
	}
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
}

var (
	registryMu      sync.RWMutex
	sessionRegistry = map[AI]*AgentSession{}
)

func RegisterSession(session *AgentSession) {
	if session == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	sessionRegistry[session.ai] = session
}

func UnregisterSession(ai AI) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(sessionRegistry, ai)
}

func RegisteredSession(ai AI) *AgentSession {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return sessionRegistry[ai]
}

func ClearRegisteredSessions() {
	registryMu.Lock()
	defer registryMu.Unlock()
	sessionRegistry = map[AI]*AgentSession{}
}

// Run executes a prompt against the specified AI CLI and streams output in real-time.
// Uses streaming JSON modes for both CLIs to avoid buffering.
//
// Claude: claude -p "<prompt>" --output-format stream-json --verbose
//
//	-> parses {"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
//
// Codex: codex exec "<prompt>" --json
//
//	-> parses {"type":"item.completed","item":{"text":"..."}}
//
// Retries transient failures with exponential backoff up to DefaultMaxRetries times.
func Run(ctx context.Context, ai AI, prompt string, onLine StreamCallback) Result {
	return RunWithOptions(ctx, ai, prompt, onLine, DefaultRunOptions())
}

// RunWithOptions is like Run but accepts explicit options.
func RunWithOptions(ctx context.Context, ai AI, prompt string, onLine StreamCallback, opts RunOptions) Result {
	if session := RegisteredSession(ai); session != nil {
		return session.Send(ctx, prompt, onLine)
	}
	return runDirectWithOptions(ctx, ai, prompt, onLine, opts)
}

func runDirectWithOptions(ctx context.Context, ai AI, prompt string, onLine StreamCallback, opts RunOptions) Result {
	var lastResult Result
	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := retryDelay(attempt)
			select {
			case <-ctx.Done():
				return Result{AI: ai, Err: ctx.Err()}
			case <-time.After(delay):
			}
		}

		lastResult = runOnce(ctx, ai, prompt, onLine, opts.Timeout)
		if lastResult.Err == nil || !IsRetryable(lastResult.Err) {
			return lastResult
		}
		// Transient failure — retry
	}
	return lastResult
}

// RunQuiet executes a prompt without streaming, returns the full output.
func RunQuiet(ctx context.Context, ai AI, prompt string) Result {
	return Run(ctx, ai, prompt, nil)
}

func sessionPrompt(history []SessionMessage, prompt string) string {
	if len(history) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString("Continue this ongoing cloadex AI session.\n\n")
	b.WriteString("Recent session history:\n")
	for _, message := range history {
		b.WriteString(fmt.Sprintf("- %s: %s\n", message.Role, message.Content))
	}
	b.WriteString("\nNew request:\n")
	b.WriteString(prompt)
	return b.String()
}

// runOnce performs a single invocation of the AI CLI.
func runOnce(ctx context.Context, ai AI, prompt string, onLine StreamCallback, timeout time.Duration) Result {
	// Apply per-provider timeout if the caller didn't set one and ctx has no deadline.
	if timeout == 0 {
		timeout = defaultTimeout(ai)
	}
	if timeout > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}

	cmd, err := buildCommand(ctx, ai, prompt)
	if err != nil {
		return Result{AI: ai, Err: err}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{AI: ai, Err: fmt.Errorf("stdout pipe: %w", err)}
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{AI: ai, Err: fmt.Errorf("stderr pipe: %w", err)}
	}

	if err := cmd.Start(); err != nil {
		return Result{AI: ai, Err: classifyStartError(ai, err)}
	}

	// Read stdout and stderr concurrently to avoid deadlock when either buffer fills.
	var output strings.Builder
	var errOutput strings.Builder
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 0, 256*1024), 256*1024)
		for s.Scan() {
			errOutput.WriteString(s.Text())
			errOutput.WriteString("\n")
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		text := extractText(ai, line)
		if text != "" {
			output.WriteString(text)
			if onLine != nil {
				for _, l := range strings.Split(text, "\n") {
					if l != "" {
						onLine(ai, l)
					}
				}
			}
		}
	}

	// Wait for stderr goroutine to finish before calling cmd.Wait.
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return Result{AI: ai, Err: classifyExitError(ai, err, errOutput.String(), output.String())}
	}

	return Result{AI: ai, Output: strings.TrimSpace(output.String())}
}

// buildCommand constructs the exec.Cmd for the given AI provider.
func buildCommand(ctx context.Context, ai AI, prompt string) (*exec.Cmd, error) {
	switch ai {
	case Claude:
		return exec.CommandContext(ctx, "claude", "-p", prompt,
			"--output-format", "stream-json", "--verbose"), nil
	case Codex:
		return exec.CommandContext(ctx, "codex", "exec", prompt, "--json"), nil
	default:
		return nil, &RunError{
			AI:        ai,
			Retryable: false,
			Cause:     fmt.Errorf("unknown AI provider: %s", ai),
		}
	}
}

// classifyStartError wraps a process-start error with retryability info.
func classifyStartError(ai AI, err error) *RunError {
	if errors.Is(err, exec.ErrNotFound) {
		return &RunError{AI: ai, Retryable: false, Cause: fmt.Errorf("%s CLI not found in PATH: %w", ai, err)}
	}
	// Other start errors (permission denied, resource exhaustion) are generally not retryable.
	return &RunError{AI: ai, Retryable: false, Cause: fmt.Errorf("start %s: %w", ai, err)}
}

// classifyExitError wraps a process-exit error with retryability info based on exit code and process output.
func classifyExitError(ai AI, err error, stderrText string, outputText string) *RunError {
	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	stderrText = strings.TrimSpace(stderrText)
	outputText = strings.TrimSpace(outputText)
	if knownMessage, knownCause, ok := classifyKnownFailure(ai, stderrText, outputText); ok {
		return &RunError{
			AI:        ai,
			Retryable: false,
			ExitCode:  exitCode,
			Stderr:    knownMessage,
			Cause:     errors.New(knownCause),
		}
	}

	retryable := isRetryableFailure(exitCode, stderrText+"\n"+outputText)

	return &RunError{
		AI:        ai,
		Retryable: retryable,
		ExitCode:  exitCode,
		Stderr:    stderrText,
		Cause:     err,
	}
}

func classifyKnownFailure(ai AI, stderr string, output string) (message string, cause string, ok bool) {
	combined := strings.ToLower(strings.TrimSpace(stderr + "\n" + output))
	switch ai {
	case Codex:
		if strings.Contains(combined, "invalid refresh token") ||
			strings.Contains(combined, "invalid_grant") ||
			strings.Contains(combined, "tokenrefreshfailed") {
			return "Codex authentication expired. Run `codex logout` and `codex login`, then try again.", "authentication required", true
		}
		if strings.Contains(combined, "authentication error") ||
			strings.Contains(combined, "not logged in") {
			return "Codex authentication is required. Run `codex login`, then try again.", "authentication required", true
		}
	case Claude:
		if strings.Contains(combined, ".claude/session-env") &&
			(strings.Contains(combined, "eperm") ||
				strings.Contains(combined, "operation not permitted") ||
				strings.Contains(combined, "permission denied")) {
			return "Claude could not write to ~/.claude/session-env. Fix local permissions for ~/.claude, then run `claude auth login` if needed.", "local permissions required", true
		}
		if strings.Contains(combined, "authentication_failed") ||
			strings.Contains(combined, "not logged in") ||
			strings.Contains(combined, "please run /login") ||
			strings.Contains(combined, "authentication error") {
			return "Claude authentication is required. Run `claude auth login`, then try again.", "authentication required", true
		}
	}
	return "", "", false
}

// isRetryableFailure determines if a failure is transient and worth retrying.
// Exit codes and stderr patterns that indicate transient issues:
//   - rate limiting (429-like), server errors (5xx-like), network/timeout errors
func isRetryableFailure(exitCode int, stderr string) bool {
	lower := strings.ToLower(stderr)

	// Rate limiting / overloaded
	if strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "429") ||
		strings.Contains(lower, "overloaded") {
		return true
	}

	// Server errors
	if strings.Contains(lower, "500") ||
		strings.Contains(lower, "502") ||
		strings.Contains(lower, "503") ||
		strings.Contains(lower, "internal server error") ||
		strings.Contains(lower, "bad gateway") ||
		strings.Contains(lower, "service unavailable") {
		return true
	}

	// Network / timeout
	if strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "timed out") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "network") ||
		strings.Contains(lower, "econnreset") ||
		strings.Contains(lower, "econnrefused") ||
		strings.Contains(lower, "etimedout") {
		return true
	}

	return false
}

func defaultTimeout(ai AI) time.Duration {
	switch ai {
	case Claude:
		return DefaultClaudeTimeout
	case Codex:
		return DefaultCodexTimeout
	default:
		return DefaultClaudeTimeout
	}
}

// retryDelay returns the backoff delay for the given attempt (1-based).
func retryDelay(attempt int) time.Duration {
	delay := RetryBaseDelay * time.Duration(math.Pow(RetryBackoffFactor, float64(attempt-1)))
	if delay > RetryMaxDelay {
		delay = RetryMaxDelay
	}
	return delay
}

// extractText pulls the human-readable text from a streaming JSON line.
func extractText(ai AI, line string) string {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return ""
	}

	switch ai {
	case Claude:
		return extractClaudeText(line)
	case Codex:
		return extractCodexText(line)
	}
	return ""
}

// extractClaudeText handles Claude's stream-json format.
// We care about two event types:
//   - {"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
//     -> the full or partial assistant message
//   - {"type":"result","result":"..."} -> final text result
func extractClaudeText(line string) string {
	var event struct {
		Type    string `json:"type"`
		Result  string `json:"result"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return ""
	}

	switch event.Type {
	case "assistant":
		var texts []string
		for _, c := range event.Message.Content {
			if c.Type == "text" && c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		return strings.Join(texts, "\n")
	case "result":
		// We already captured from "assistant" events, but if we somehow
		// missed content, the result field has the final text.
		return ""
	}

	return ""
}

// extractCodexText handles Codex's JSONL format.
// We care about: {"type":"item.completed","item":{"text":"..."}}
func extractCodexText(line string) string {
	var event struct {
		Type string `json:"type"`
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}

	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return ""
	}

	if event.Type == "item.completed" && event.Item.Text != "" {
		return event.Item.Text
	}

	return ""
}
