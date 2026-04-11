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
	fmt.Printf("%s%s[%s]%s %s\n", Bold, ClaudeColor, claudeLabel, Reset, msg)
}

func PrintCodex(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s[%s]%s  %s\n", Bold, CodexColor, codexLabel, Reset, msg)
}

func PrintSystem(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s[cloadex]%s  %s\n", Bold, SystemColor, Reset, msg)
}

func PrintError(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s[error]%s  %s\n", Bold, ErrorColor, Reset, msg)
}

func PrintSuccess(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s[done]%s   %s\n", Bold, SuccessColor, Reset, msg)
}

func PrintUser(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s[you]%s    %s\n", Bold, UserColor, Reset, msg)
}

func StreamClaude(line string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s%s[%s]%s %s\n", Bold, ClaudeColor, claudeLabel, Reset, line)
}

func StreamCodex(line string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s%s[%s]%s  %s\n", Bold, CodexColor, codexLabel, Reset, line)
}

func Divider() {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s%s%s%s\n", Dim, SystemColor, strings.Repeat("─", 60), Reset)
}

func Banner() {
	banner := `
 ██╗    ██╗██╗███████╗██████╗  ██████╗
 ██║    ██║██║╚══███╔╝██╔══██╗██╔═══██╗
 ██║ █╗ ██║██║  ███╔╝ ██║  ██║██║   ██║
 ██║███╗██║██║ ███╔╝  ██║  ██║██║   ██║
 ╚███╔███╔╝██║███████╗██████╔╝╚██████╔╝
  ╚══╝╚══╝ ╚═╝╚══════╝╚═════╝  ╚═════╝`
	fmt.Printf("%s%s%s%s\n", Bold, SystemColor, banner, Reset)
	fmt.Printf("%s%s  Claude + Codex — better together%s\n\n", Dim, SystemColor, Reset)
}

func PhaseHeader(phase int, name string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Println()
	fmt.Printf("%s%s══════ Phase %d: %s ══════%s\n\n", Bold, SystemColor, phase, name, Reset)
}
