package plan

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Ahmedlag/cloadex/internal/runner"
	"github.com/Ahmedlag/cloadex/internal/ui"
)

type Decision int

const (
	Approve Decision = iota
	Edit
	Reject
)

// Task represents a single unit of work in an execution plan.
// The schema is provider-agnostic: any AI backend can own a task,
// and the scope/dependency/verification fields work for any language or project type.
type Task struct {
	// OwnerAI is which AI provider executes this task (e.g. "claude", "codex").
	OwnerAI runner.AI `json:"owner_ai"`

	// Description is a concise summary of what this task accomplishes.
	Description string `json:"description"`

	// Scope lists the files or directories this task will create or modify.
	Scope []string `json:"scope,omitempty"`

	// DependsOn lists indices (0-based) of tasks that must complete before this one.
	DependsOn []int `json:"depends_on,omitempty"`

	// Verification is an optional command to run after the task to check correctness
	// (e.g. "go test ./internal/auth/...", "pytest tests/test_auth.py").
	Verification string `json:"verification,omitempty"`
}

// Plan is the structured output of the debate/planning phase.
type Plan struct {
	Overview string `json:"overview"`
	Tasks    []Task `json:"tasks"`
}

// ParsePlan tries to extract a structured Plan from the debate output.
// It first attempts JSON parsing, then falls back to markdown heuristics.
func ParsePlan(text string) Plan {
	// Try to extract a JSON block from the text
	if p, ok := parseJSON(text); ok {
		return p
	}
	return parseMarkdown(text)
}

// parseJSON looks for a JSON object in the text and unmarshals it into a Plan.
func parseJSON(text string) (Plan, bool) {
	// Find the outermost JSON object that looks like a plan
	start := strings.Index(text, "{")
	if start == -1 {
		return Plan{}, false
	}

	// Try progressively larger substrings starting from each '{'
	for i := start; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		// Find matching closing brace
		depth := 0
		for j := i; j < len(text); j++ {
			switch text[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					var p Plan
					if err := json.Unmarshal([]byte(text[i:j+1]), &p); err == nil && len(p.Tasks) > 0 {
						// Validate and normalize owner_ai values
						for k := range p.Tasks {
							p.Tasks[k].OwnerAI = normalizeAI(p.Tasks[k].OwnerAI)
						}
						return p, true
					}
					break
				}
			}
		}
	}
	return Plan{}, false
}

// parseMarkdown extracts tasks from markdown-formatted plan text.
// This is the fallback when the AI doesn't produce structured JSON.
func parseMarkdown(text string) Plan {
	p := Plan{}
	lines := strings.Split(text, "\n")

	// Extract overview from the first non-empty, non-heading paragraph
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "**") {
			continue
		}
		if !isTaskLine(trimmed) {
			p.Overview = trimmed
			break
		}
	}

	currentAI := runner.AI("")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		// Detect owner sections by AI name in headings/labels
		if containsAny(lower, "claude") && isHeadingOrLabel(trimmed) {
			currentAI = runner.Claude
			continue
		}
		if containsAny(lower, "codex") && isHeadingOrLabel(trimmed) {
			currentAI = runner.Codex
			continue
		}

		// Parse task lines
		if currentAI != "" && isTaskLine(trimmed) {
			desc := stripBullet(trimmed)
			if desc != "" {
				p.Tasks = append(p.Tasks, Task{
					OwnerAI:     currentAI,
					Description: desc,
				})
			}
		}
	}

	// If no tasks were parsed with owner detection, treat the whole plan as
	// one task per AI so execution still proceeds.
	if len(p.Tasks) == 0 {
		p.Tasks = append(p.Tasks,
			Task{OwnerAI: runner.Codex, Description: text},
			Task{OwnerAI: runner.Claude, Description: text},
		)
	}

	return p
}

func isTaskLine(s string) bool {
	if strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") {
		return true
	}
	if len(s) > 2 && s[0] >= '1' && s[0] <= '9' && s[1] == '.' {
		return true
	}
	return false
}

func stripBullet(s string) string {
	if strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") {
		return strings.TrimSpace(s[2:])
	}
	if idx := strings.Index(s, ". "); idx != -1 && idx < 4 {
		return strings.TrimSpace(s[idx+2:])
	}
	return strings.TrimSpace(s)
}

func isHeadingOrLabel(s string) bool {
	return strings.HasPrefix(s, "#") || strings.HasPrefix(s, "**") || strings.HasSuffix(s, ":")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func normalizeAI(ai runner.AI) runner.AI {
	switch runner.AI(strings.ToLower(string(ai))) {
	case runner.Claude:
		return runner.Claude
	case runner.Codex:
		return runner.Codex
	default:
		// Default unknown providers to Claude
		return runner.Claude
	}
}

// TasksForAI returns tasks assigned to a specific AI provider.
func TasksForAI(tasks []Task, ai runner.AI) []Task {
	var filtered []Task
	for _, t := range tasks {
		if t.OwnerAI == ai {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// Present shows the plan to the user and asks for approval.
// If autoApprove is true, the plan is approved without prompting (--yes mode).
func Present(planText string, autoApprove bool) (Decision, string) {
	ui.Divider()
	ui.PhaseHeader(2, "PLAN REVIEW")

	fmt.Println(planText)
	fmt.Println()
	ui.Divider()

	if autoApprove {
		ui.PrintSuccess("Plan auto-approved (--yes)")
		return Approve, planText
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		ui.PrintSystem("Do you approve this plan?")
		fmt.Printf("  %s[a]%s Approve  %s[e]%s Edit  %s[r]%s Reject & restart debate\n",
			ui.SuccessColor, ui.Reset,
			ui.UserColor, ui.Reset,
			ui.ErrorColor, ui.Reset)
		fmt.Printf("\n  > ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "a", "approve", "yes", "y":
			ui.PrintSuccess("Plan approved!")
			return Approve, planText

		case "e", "edit":
			ui.PrintSystem("Enter your edits (type END on a new line when done):")
			var edits strings.Builder
			for {
				line, _ := reader.ReadString('\n')
				if strings.TrimSpace(line) == "END" {
					break
				}
				edits.WriteString(line)
			}
			editedPlan := planText + "\n\nUSER EDITS:\n" + edits.String()
			ui.PrintSuccess("Edits recorded. Proceeding with modified plan.")
			return Edit, editedPlan

		case "r", "reject", "no", "n":
			ui.PrintError("Plan rejected. Restarting debate...")
			return Reject, ""

		default:
			ui.PrintError("Invalid choice. Please enter a, e, or r.")
		}
	}
}
