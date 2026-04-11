package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloadex-cli/cloadex/internal/runner"
	"github.com/cloadex-cli/cloadex/internal/workspace"
)

// RunStatus represents the current phase of a run.
type RunStatus string

const (
	StatusDebating     RunStatus = "debating"
	StatusApproved     RunStatus = "approved"
	StatusExecuting    RunStatus = "executing"
	StatusValidating   RunStatus = "validating"
	StatusDone         RunStatus = "done"
	StatusInterrupted  RunStatus = "interrupted"
	StatusWaitingInput RunStatus = "waiting_input"
)

// RunManifest is the persistent state of a run, stored as status.json.
type RunManifest struct {
	ID              string           `json:"id"`
	Prompt          string           `json:"prompt"`
	Status          RunStatus        `json:"status"`
	PendingDecision *PendingDecision `json:"pending_decision,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type PendingDecision struct {
	Issue       string         `json:"issue"`
	TaskIndices []int          `json:"task_indices,omitempty"`
	OptionOne   ProposalOption `json:"option_one"`
	OptionTwo   ProposalOption `json:"option_two"`
	MiniDebate  string         `json:"mini_debate,omitempty"`
}

type ProposalOption struct {
	AI          runner.AI `json:"ai"`
	Cause       string    `json:"cause"`
	FixSummary  string    `json:"fix_summary"`
	TaskIndices []int     `json:"task_indices,omitempty"`
}

// CreateRun initialises a new run directory with a status.json manifest.
// Returns the manifest with the generated run ID.
func CreateRun(prompt string) (*RunManifest, error) {
	id := time.Now().Format("20060102-150405")
	dir, err := runDir(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}

	// Write prompt file (for backwards compat with ListRuns/LoadRun).
	if err := os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(prompt), 0o644); err != nil {
		return nil, fmt.Errorf("write prompt: %w", err)
	}

	m := &RunManifest{
		ID:        id,
		Prompt:    prompt,
		Status:    StatusDebating,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := writeManifest(dir, m); err != nil {
		return nil, err
	}
	return m, nil
}

// UpdateStatus sets the status field of an existing run's manifest.
func UpdateStatus(id string, status RunStatus) error {
	dir, err := runDir(id)
	if err != nil {
		return err
	}
	m, err := LoadManifest(id)
	if err != nil {
		return err
	}
	m.Status = status
	m.UpdatedAt = time.Now()
	return writeManifest(dir, m)
}

func SavePendingDecision(id string, decision *PendingDecision) error {
	m, err := LoadManifest(id)
	if err != nil {
		return err
	}
	m.PendingDecision = decision
	if decision != nil {
		m.Status = StatusWaitingInput
	}
	m.UpdatedAt = time.Now()
	dir, err := runDir(id)
	if err != nil {
		return err
	}
	return writeManifest(dir, m)
}

func ClearPendingDecision(id string) error {
	m, err := LoadManifest(id)
	if err != nil {
		return err
	}
	m.PendingDecision = nil
	m.UpdatedAt = time.Now()
	dir, err := runDir(id)
	if err != nil {
		return err
	}
	return writeManifest(dir, m)
}

// SavePlanArtifacts writes plan.md and debate.md into the run directory.
func SavePlanArtifacts(id, planText, debateHistory string) error {
	dir, err := runDir(id)
	if err != nil {
		return err
	}
	if planText != "" {
		if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte(planText), 0o644); err != nil {
			return fmt.Errorf("write plan: %w", err)
		}
	}
	if debateHistory != "" {
		if err := os.WriteFile(filepath.Join(dir, "debate.md"), []byte(debateHistory), 0o644); err != nil {
			return fmt.Errorf("write debate: %w", err)
		}
	}
	return nil
}

// SaveExecutionArtifact writes execution.md into the run directory.
func SaveExecutionArtifact(id, execOutput string) error {
	if execOutput == "" {
		return nil
	}
	dir, err := runDir(id)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "execution.md"), []byte(execOutput), 0o644)
}

func SaveExecutionState(id string, data []byte) error {
	dir, err := runDir(id)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "execution-state.json"), data, 0o644)
}

func LoadExecutionState(id string) ([]byte, error) {
	dir, err := runDir(id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, "execution-state.json"))
}

// SaveValidationArtifact writes validation.md into the run directory.
func SaveValidationArtifact(id, valOutput string) error {
	if valOutput == "" {
		return nil
	}
	dir, err := runDir(id)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "validation.md"), []byte(valOutput), 0o644)
}

// LoadManifest reads the status.json for a given run ID.
func LoadManifest(id string) (*RunManifest, error) {
	path, err := workspace.Path("runs", id, "status.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", id, err)
	}
	var m RunManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", id, err)
	}
	return &m, nil
}

// LatestIncompleteRun returns the most recent run whose status is neither
// "done" nor empty. Returns nil (no error) if no incomplete run exists.
func LatestIncompleteRun() (*RunManifest, error) {
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

	// Entries are sorted alphabetically; timestamps sort chronologically.
	// Walk backwards to find the most recent incomplete run.
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !e.IsDir() {
			continue
		}
		m, err := LoadManifest(e.Name())
		if err != nil {
			continue // skip runs without a manifest
		}
		if m.Status != StatusDone {
			return m, nil
		}
	}
	return nil, nil
}

// RunDir returns the filesystem path for a run ID.
func RunDir(id string) string {
	dir, err := runDir(id)
	if err != nil {
		return filepath.Join(cloadexDir, "runs", id)
	}
	return dir
}

func writeManifest(dir string, m *RunManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644)
}
