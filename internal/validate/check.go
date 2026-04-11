package validate

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Ahmedlag/cloadex/internal/plan"
	"github.com/Ahmedlag/cloadex/internal/ui"
)

// CheckResult holds the outcome of a single deterministic check command.
type CheckResult struct {
	Name     string
	Command  string
	Passed   bool
	Skipped  bool
	Output   string
	Duration time.Duration
}

// CheckSuite holds results of all deterministic checks run against the workspace.
type CheckSuite struct {
	Results []CheckResult
	Passed  bool
}

// Summary returns a human-readable summary of all check results.
func (cs *CheckSuite) Summary() string {
	var sb strings.Builder
	for _, r := range cs.Results {
		status := "PASS"
		if r.Skipped {
			status = "SKIP"
		} else if !r.Passed {
			status = "FAIL"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s (%s, %s)\n", status, r.Name, r.Command, r.Duration.Round(time.Millisecond)))
		if !r.Passed && r.Output != "" {
			// Include truncated output for failures
			out := r.Output
			if len(out) > 2000 {
				out = out[:2000] + "\n... (truncated)"
			}
			sb.WriteString(out)
			if !strings.HasSuffix(out, "\n") {
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

// FailedOutput returns only the output from failed checks, suitable for feeding into a fix prompt.
func (cs *CheckSuite) FailedOutput() string {
	var sb strings.Builder
	for _, r := range cs.Results {
		if r.Passed {
			continue
		}
		sb.WriteString(fmt.Sprintf("FAILED: %s\nCommand: %s\nOutput:\n%s\n\n", r.Name, r.Command, r.Output))
	}
	return sb.String()
}

// RunChecks executes deterministic verification commands for the workspace.
// It runs:
//  1. Auto-detected repo checks (go vet, go build, etc. based on project files)
//  2. Task-level verification commands from the plan
//  3. Any custom commands provided
func RunChecks(ctx context.Context, tasks []plan.Task, customCmds []string, interactive bool) *CheckSuite {
	suite := &CheckSuite{Passed: true}

	// Collect all commands to run: auto-detected + task verifications + custom
	type namedCmd struct {
		name    string
		cmd     string
		trusted bool
	}
	var commands []namedCmd

	// Auto-detect repo-level checks
	for _, c := range detectRepoChecks() {
		commands = append(commands, namedCmd{name: c.name, cmd: c.cmd, trusted: true})
	}

	// Task-level verification commands
	for i, t := range tasks {
		if t.Verification != "" {
			name := fmt.Sprintf("task %d verification", i)
			commands = append(commands, namedCmd{name: name, cmd: t.Verification})
		}
	}

	// Custom user-provided commands
	for _, c := range customCmds {
		if c != "" {
			commands = append(commands, namedCmd{name: "custom", cmd: c})
		}
	}

	// Deduplicate commands (same command string)
	seen := map[string]bool{}
	var deduped []namedCmd
	for _, c := range commands {
		if !seen[c.cmd] {
			seen[c.cmd] = true
			deduped = append(deduped, c)
		}
	}

	if len(deduped) == 0 {
		ui.PrintVerbose("No deterministic checks to run")
		return suite
	}

	ui.PrintSystem("Running %d verification check(s)...", len(deduped))

	for _, nc := range deduped {
		var r CheckResult
		if nc.trusted {
			r = runCheck(ctx, nc.name, nc.cmd)
		} else {
			r = runUntrustedCheck(ctx, nc.name, nc.cmd, interactive)
		}
		suite.Results = append(suite.Results, r)
		if !r.Passed && !r.Skipped {
			suite.Passed = false
		}

		status := ui.SuccessColor + "PASS" + ui.Reset
		if r.Skipped {
			status = ui.UserColor + "SKIP" + ui.Reset
		} else if !r.Passed {
			status = ui.ErrorColor + "FAIL" + ui.Reset
		}
		ui.PrintSystem("  [%s] %s (%s)", status, nc.name, r.Duration.Round(time.Millisecond))
	}

	return suite
}

var approveVerificationCommand = func(name, command string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		ui.PrintSystem("Approve verification command for %s?", name)
		fmt.Printf("  %s\n", command)
		fmt.Printf("  %s[y]%s Run  %s[n]%s Skip\n\n  > ", ui.SuccessColor, ui.Reset, ui.ErrorColor, ui.Reset)
		input, _ := reader.ReadString('\n')
		switch strings.TrimSpace(strings.ToLower(input)) {
		case "y", "yes":
			return true
		case "n", "no", "":
			return false
		default:
			ui.PrintError("Invalid choice. Please enter y or n.")
		}
	}
}

// runCheck executes a single shell command and captures the result.
func runCheck(ctx context.Context, name, command string) CheckResult {
	start := time.Now()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	return CheckResult{
		Name:     name,
		Command:  command,
		Passed:   err == nil,
		Output:   strings.TrimSpace(output),
		Duration: duration,
	}
}

func runUntrustedCheck(ctx context.Context, name, command string, interactive bool) CheckResult {
	start := time.Now()
	if !interactive {
		return CheckResult{
			Name:     name,
			Command:  command,
			Passed:   true,
			Skipped:  true,
			Output:   "Skipped untrusted verification command in non-interactive mode.",
			Duration: time.Since(start),
		}
	}
	if !approveVerificationCommand(name, command) {
		return CheckResult{
			Name:     name,
			Command:  command,
			Passed:   true,
			Skipped:  true,
			Output:   "Skipped untrusted verification command by user choice.",
			Duration: time.Since(start),
		}
	}
	return runCheck(ctx, name, command)
}

type repoCheck struct {
	name string
	cmd  string
}

// detectRepoChecks looks at the workspace to determine which built-in checks apply.
func detectRepoChecks() []repoCheck {
	var checks []repoCheck

	// Go project
	if fileExists("go.mod") {
		checks = append(checks, repoCheck{name: "go vet", cmd: "go vet ./..."})
		checks = append(checks, repoCheck{name: "go build", cmd: "go build ./..."})
		if hasGoTestFiles() {
			checks = append(checks, repoCheck{name: "go test", cmd: "go test ./..."})
		}
	}

	// Node.js project
	if fileExists("package.json") {
		if fileExists("node_modules/.bin/tsc") {
			checks = append(checks, repoCheck{name: "tsc", cmd: "npx tsc --noEmit"})
		}
		if fileExists("node_modules/.bin/eslint") {
			checks = append(checks, repoCheck{name: "eslint", cmd: "npx eslint ."})
		}
	}

	// Python project
	if fileExists("pyproject.toml") || fileExists("setup.py") || fileExists("requirements.txt") {
		if cmdExists("pytest") {
			checks = append(checks, repoCheck{name: "pytest", cmd: "pytest"})
		}
		if cmdExists("mypy") {
			checks = append(checks, repoCheck{name: "mypy", cmd: "mypy ."})
		}
	}

	// Rust project
	if fileExists("Cargo.toml") {
		checks = append(checks, repoCheck{name: "cargo check", cmd: "cargo check"})
		checks = append(checks, repoCheck{name: "cargo test", cmd: "cargo test"})
	}

	return checks
}

func fileExists(path string) bool {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("[ -f %q ] || [ -d %q ]", path, path))
	return cmd.Run() == nil
}

func hasGoTestFiles() bool {
	cmd := exec.Command("sh", "-c", "find . -name '*_test.go' -not -path './vendor/*' | head -1")
	out, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
