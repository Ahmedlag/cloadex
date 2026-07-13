package ui

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func TestWelcomeBoxContent(t *testing.T) {
	out := stripANSI(WelcomeBox("v0.3.0", "Claude [2]", "Codex [1]", "/tmp/repo", "main", "chat"))

	for _, want := range []string{
		"cloadex v0.3.0",
		"Claude + Codex",
		"Claude [2]",
		"Codex [1]",
		"/tmp/repo",
		"main",
		"CHAT",
		"shift+tab",
		"/help",
		"/exit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("WelcomeBox missing %q in:\n%s", want, out)
		}
	}
}

func TestWelcomeBoxBordersAligned(t *testing.T) {
	out := stripANSI(WelcomeBox("v0.3.0", "Claude", "Codex", "/tmp/repo", "main", "chat"))

	width := 0
	var boxLines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "│") || strings.HasPrefix(line, "╰") {
			boxLines = append(boxLines, line)
			if width == 0 {
				width = utf8.RuneCountInString(line)
			} else if got := utf8.RuneCountInString(line); got != width {
				t.Errorf("box line width %d, want %d: %q", got, width, line)
			}
		}
	}
	if len(boxLines) < 3 {
		t.Fatalf("expected at least 3 box lines (top, content, bottom), got %d:\n%s", len(boxLines), out)
	}
	last := boxLines[len(boxLines)-1]
	if !strings.HasPrefix(boxLines[0], "╭") || !strings.HasPrefix(last, "╰") {
		t.Errorf("box not framed with rounded corners:\n%s", out)
	}
}

func TestWelcomeBoxOmitsEmptyFields(t *testing.T) {
	out := stripANSI(WelcomeBox("dev", "Claude", "Codex", "", "", "planning"))

	if strings.Contains(out, "directory:") {
		t.Errorf("WelcomeBox should omit directory line when empty:\n%s", out)
	}
	if strings.Contains(out, "branch:") {
		t.Errorf("WelcomeBox should omit branch line when empty:\n%s", out)
	}
	if !strings.Contains(out, "PLANNING") {
		t.Errorf("WelcomeBox missing mode PLANNING:\n%s", out)
	}
}
