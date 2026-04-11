package persist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloadex-cli/cloadex/internal/workspace"
)

const cloadexDir = workspace.DirName

// RunSummary holds metadata about a persisted run for listing purposes.
type RunSummary struct {
	ID        string    // Directory name (timestamp)
	Dir       string    // Full path to run directory
	Prompt    string    // First line of the prompt (truncated)
	Timestamp time.Time // Parsed from directory name
	HasPlan   bool
	HasExec   bool
	HasVal    bool
}

// RunDetail holds the full contents of a persisted run.
type RunDetail struct {
	RunSummary
	Plan       string
	Debate     string
	Execution  string
	Validation string
	FullPrompt string
}

// SaveRun persists the plan and execution output for a given run.
// Files are written to .cloadex/runs/<timestamp>/ in the current working directory.
func SaveRun(prompt, plan, debateHistory, execOutput, validationOutput string) (string, error) {
	ts := time.Now().Format("20060102-150405")
	dir, err := runDir(ts)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create run dir: %w", err)
	}

	files := map[string]string{
		"prompt.txt":    prompt,
		"plan.md":       plan,
		"debate.md":     debateHistory,
		"execution.md":  execOutput,
		"validation.md": validationOutput,
	}

	for name, content := range files {
		if content == "" {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", name, err)
		}
	}

	return dir, nil
}

// LatestPlan returns the plan text from the most recent run, if any.
func LatestPlan() (string, error) {
	runsDir, err := runsDir()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no previous runs found")
	}

	// Entries are sorted alphabetically; timestamps sort chronologically.
	latest := entries[len(entries)-1].Name()
	data, err := os.ReadFile(filepath.Join(runsDir, latest, "plan.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListRuns returns summaries of all persisted runs, most recent first.
func ListRuns() ([]RunSummary, error) {
	runsDir, err := runsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	runs := make([]RunSummary, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(runsDir, e.Name())
		s := RunSummary{
			ID:  e.Name(),
			Dir: dir,
		}

		// Parse timestamp from directory name
		if t, err := time.Parse("20060102-150405", e.Name()); err == nil {
			s.Timestamp = t
		}

		// Read first line of prompt
		if data, err := os.ReadFile(filepath.Join(dir, "prompt.txt")); err == nil {
			s.Prompt = firstLine(string(data), 80)
		}

		// Check which artifacts exist
		s.HasPlan = fileExists(filepath.Join(dir, "plan.md"))
		s.HasExec = fileExists(filepath.Join(dir, "execution.md"))
		s.HasVal = fileExists(filepath.Join(dir, "validation.md"))

		runs = append(runs, s)
	}

	return runs, nil
}

// LoadRun loads the full details of a run by its ID (timestamp directory name).
// If id is empty, loads the most recent run.
func LoadRun(id string) (*RunDetail, error) {
	if id == "" {
		runs, err := ListRuns()
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			return nil, fmt.Errorf("no previous runs found")
		}
		id = runs[0].ID
	}

	dir, err := runDir(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("run not found: %s", id)
	}

	d := &RunDetail{
		RunSummary: RunSummary{
			ID:  id,
			Dir: dir,
		},
	}

	if t, err := time.Parse("20060102-150405", id); err == nil {
		d.Timestamp = t
	}

	d.FullPrompt = readFileOr(filepath.Join(dir, "prompt.txt"))
	d.Prompt = firstLine(d.FullPrompt, 80)
	d.Plan = readFileOr(filepath.Join(dir, "plan.md"))
	d.Debate = readFileOr(filepath.Join(dir, "debate.md"))
	d.Execution = readFileOr(filepath.Join(dir, "execution.md"))
	d.Validation = readFileOr(filepath.Join(dir, "validation.md"))

	d.HasPlan = d.Plan != ""
	d.HasExec = d.Execution != ""
	d.HasVal = d.Validation != ""

	return d, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFileOr(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func firstLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > maxLen {
		s = s[:maxLen-3] + "..."
	}
	return s
}

// EnsureGitignore appends .cloadex/ to .gitignore if not already present.
func EnsureGitignore() {
	const entry = ".cloadex/"
	data, err := os.ReadFile(".gitignore")
	if err == nil {
		for _, line := range splitLines(string(data)) {
			if line == entry {
				return
			}
		}
	}

	f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	// Add newline before entry if file doesn't end with one
	if len(data) > 0 && data[len(data)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(entry + "\n")
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func runsDir() (string, error) {
	return workspace.Path("runs")
}

func runDir(id string) (string, error) {
	return workspace.Path("runs", id)
}
