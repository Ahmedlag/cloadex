package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	Reset = "\033[0m"
	Bold  = "\033[1m"
	Dim   = "\033[2m"
	Muted = "\033[38;5;245m"

	// Claude = purple/violet
	ClaudeColor = "\033[38;5;141m"
	// Codex = green
	CodexColor = "\033[38;5;114m"
	// System = yellow
	SystemColor = "\033[38;5;221m"
	// User = cyan
	UserColor = "\033[38;5;117m"
	// Error = red
	ErrorColor = "\033[38;5;203m"
	// Success = bright green
	SuccessColor = "\033[38;5;156m"
	// Prompt background
	PromptBg = "\033[48;5;117m"
	// Dark foreground for prompt chip
	PromptFg = "\033[38;5;16m"
)

var (
	mu          sync.Mutex
	verbose     bool
	claudeLabel = "Claude"
	codexLabel  = "Codex"
)

// SetVerbose enables or disables verbose logging.
func SetVerbose(v bool) { verbose = v }

// SetAILabel updates the rendered label for Claude or Codex.
func SetAILabel(ai string, label string) {
	mu.Lock()
	defer mu.Unlock()
	switch ai {
	case "claude":
		claudeLabel = label
	case "codex":
		codexLabel = label
	}
}

// PrintVerbose prints a timestamped debug line only when verbose mode is on.
func PrintVerbose(format string, args ...any) {
	if !verbose {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("15:04:05.000")
	fmt.Printf("%s%s[debug %s]%s %s\n", Dim, SystemColor, ts, Reset, msg)
}

func PrintClaude(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s%s%s%s%s  %s\n", Bold, ClaudeColor, claudeLabel, Reset, Muted, "›", msg)
}

func PrintCodex(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s%s%s%s%s  %s\n", Bold, CodexColor, codexLabel, Reset, Muted, "›", msg)
}

func PrintSystem(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%scloadex%s%s  %s\n", Dim, SystemColor, Reset, Dim, msg)
}

func PrintError(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%serror%s%s    %s\n", Bold, ErrorColor, Reset, Dim, msg)
}

func PrintSuccess(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%sdone%s%s     %s\n", Bold, SuccessColor, Reset, Dim, msg)
}

func PrintUser(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%syou%s%s      %s\n", Bold, UserColor, Reset, Dim, msg)
}

func StreamClaude(line string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s%s%s%s%s%s  %s\n", Bold, ClaudeColor, claudeLabel, Reset, Muted, "›", line)
}

func StreamCodex(line string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s%s%s%s%s%s  %s\n", Bold, CodexColor, codexLabel, Reset, Muted, "›", line)
}

func Divider() {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s%s%s%s\n", Dim, Muted, strings.Repeat("─", 52), Reset)
}

func Banner() {
	fmt.Printf("%s%scloadex%s%s  Claude + Codex — better together%s\n\n", Bold, SystemColor, Reset, Dim, Reset)
}

func PhaseHeader(phase int, name string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Println()
	fmt.Printf("%s%sphase %d%s%s  %s%s%s\n", Dim, Muted, phase, Reset, Bold, SystemColor, name, Reset)
	fmt.Printf("%s%s%s%s\n\n", Dim, Muted, strings.Repeat("─", 28), Reset)
}

func SessionHeader(repo string, branch string, mode string, claude string, codex string) string {
	var top []string
	top = append(top, fmt.Sprintf("%s%scloadex%s", Bold, SystemColor, Reset))
	if repo != "" {
		top = append(top, fmt.Sprintf("%s%s%s", Bold, repo, Reset))
	}
	if branch != "" {
		top = append(top, fmt.Sprintf("%s%s%s", Dim, branch, Reset))
	}
	if mode != "" {
		top = append(top, chip(strings.ToUpper(mode)))
	}

	var bottom []string
	if claude != "" {
		bottom = append(bottom, fmt.Sprintf("%s%s", ClaudeColor, claude)+Reset)
	}
	if codex != "" {
		bottom = append(bottom, fmt.Sprintf("%s%s", CodexColor, codex)+Reset)
	}

	header := strings.Join(top, "  ")
	if len(bottom) > 0 {
		header += "\n" + strings.Join(bottom, "  ")
	}
	return header + "\n"
}

func SessionPrompt(mode string) string {
	return fmt.Sprintf("%s %s%s›%s ", chip(strings.ToUpper(mode)), Bold, UserColor, Reset)
}

func chip(text string) string {
	return fmt.Sprintf("%s%s%s %s %s", Bold, PromptBg, PromptFg, text, Reset)
}
