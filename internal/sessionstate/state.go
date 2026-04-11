package sessionstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ahmedlag/cloadex/internal/runner"
	"github.com/Ahmedlag/cloadex/internal/workspace"
)

type Mode string

const (
	ModeChat      Mode = "chat"
	ModePlanning  Mode = "planning"
	ModeExecution Mode = "execution"
)

type Turn struct {
	Role      string    `json:"role"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type PinnedMemory struct {
	Kind      string    `json:"kind"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

type Event struct {
	Kind      string    `json:"kind"`
	Stage     string    `json:"stage,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type State struct {
	Version       int                               `json:"version"`
	RepoPath      string                            `json:"repo_path"`
	Branch        string                            `json:"branch,omitempty"`
	Mode          Mode                              `json:"mode"`
	RepoSummary   string                            `json:"repo_summary,omitempty"`
	ActiveGoal    string                            `json:"active_goal,omitempty"`
	LastPlan      string                            `json:"last_plan,omitempty"`
	LastRunID     string                            `json:"last_run_id,omitempty"`
	AgentSessions map[string]runner.SessionSnapshot `json:"agent_sessions,omitempty"`
	Turns         []Turn                            `json:"turns,omitempty"`
	Pinned        []PinnedMemory                    `json:"pinned,omitempty"`
	Events        []Event                           `json:"events,omitempty"`
	UpdatedAt     time.Time                         `json:"updated_at"`
}

const fileName = "session.json"

func LoadOrInit() (*State, error) {
	path, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		var state State
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("parse session state: %w", err)
		}
		if state.Version == 0 {
			state.Version = 1
		}
		if normalized, ok := normalizeMode(string(state.Mode)); ok {
			state.Mode = normalized
		} else {
			state.Mode = ModeChat
		}
		if state.AgentSessions == nil {
			state.AgentSessions = map[string]runner.SessionSnapshot{}
		}
		return &state, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read session state: %w", err)
	}

	cwd, _ := os.Getwd()
	state := &State{
		Version:       1,
		RepoPath:      cwd,
		Mode:          ModeChat,
		RepoSummary:   defaultRepoSummary(cwd),
		Branch:        detectBranch(cwd),
		AgentSessions: map[string]runner.SessionSnapshot{},
		UpdatedAt:     time.Now(),
	}
	return state, Save(state)
}

func Save(state *State) error {
	if state == nil {
		return nil
	}
	p, err := path()
	if err != nil {
		return err
	}
	if err := workspace.EnsurePrivateDir(filepath.Dir(p)); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session state: %w", err)
	}
	return workspace.WritePrivateFile(p, data)
}

func (s *State) RecordTurn(role string, message string) {
	if s == nil || strings.TrimSpace(message) == "" {
		return
	}
	s.Turns = append(s.Turns, Turn{
		Role:      role,
		Message:   strings.TrimSpace(message),
		CreatedAt: time.Now(),
	})
	if len(s.Turns) > 20 {
		s.Turns = s.Turns[len(s.Turns)-20:]
	}
}

func (s *State) Pin(kind string, value string) {
	if s == nil || strings.TrimSpace(value) == "" {
		return
	}
	s.Pinned = append(s.Pinned, PinnedMemory{
		Kind:      kind,
		Value:     strings.TrimSpace(value),
		CreatedAt: time.Now(),
	})
	if len(s.Pinned) > 20 {
		s.Pinned = s.Pinned[len(s.Pinned)-20:]
	}
}

func (s *State) RecordEvent(kind string, stage string, actor string, detail string) {
	if s == nil || strings.TrimSpace(detail) == "" {
		return
	}
	s.Events = append(s.Events, Event{
		Kind:      kind,
		Stage:     strings.TrimSpace(stage),
		Actor:     strings.TrimSpace(actor),
		Detail:    strings.TrimSpace(detail),
		CreatedAt: time.Now(),
	})
	if len(s.Events) > 40 {
		s.Events = s.Events[len(s.Events)-40:]
	}
}

func (s *State) SummaryForPrompt() string {
	if s == nil {
		return ""
	}
	var parts []string
	if s.RepoSummary != "" {
		parts = append(parts, "Repo summary:\n"+s.RepoSummary)
	}
	if s.Mode != "" {
		parts = append(parts, fmt.Sprintf("Session mode: %s", s.Mode))
	}
	if s.LastRunID != "" {
		parts = append(parts, fmt.Sprintf("Last run ID: %s", s.LastRunID))
	}
	return strings.Join(parts, "\n\n")
}

func ValidMode(raw string) (Mode, bool) {
	return normalizeMode(raw)
}

func path() (string, error) {
	return workspace.Path(fileName)
}

func NextMode(mode Mode) Mode {
	switch mode {
	case ModeChat:
		return ModePlanning
	case ModePlanning:
		return ModeExecution
	default:
		return ModeChat
	}
}

func normalizeMode(raw string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ModeChat), "review":
		return ModeChat, true
	case string(ModePlanning), "plan":
		return ModePlanning, true
	case string(ModeExecution), "run":
		return ModeExecution, true
	default:
		return "", false
	}
}
