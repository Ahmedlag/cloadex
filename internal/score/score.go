package score

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloadex-cli/cloadex/internal/runner"
)

type Entry struct {
	Plan      int `json:"plan"`
	Execution int `json:"execution"`
	Fix       int `json:"fix"`
}

func (e Entry) Total() int {
	return e.Plan + e.Execution + e.Fix
}

type Board struct {
	AIs map[string]Entry `json:"ais"`
}

const fileName = "scoreboard.json"

func Load() (*Board, error) {
	path, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Board{AIs: map[string]Entry{}}, nil
		}
		return nil, fmt.Errorf("read scoreboard: %w", err)
	}

	var board Board
	if err := json.Unmarshal(data, &board); err != nil {
		return nil, fmt.Errorf("parse scoreboard: %w", err)
	}
	if board.AIs == nil {
		board.AIs = map[string]Entry{}
	}
	return &board, nil
}

func AddPoint(ai runner.AI, category string) error {
	board, err := Load()
	if err != nil {
		return err
	}

	entry := board.AIs[string(ai)]
	switch category {
	case "plan":
		entry.Plan++
	case "execution":
		entry.Execution++
	case "fix":
		entry.Fix++
	default:
		return fmt.Errorf("unknown score category: %s", category)
	}
	board.AIs[string(ai)] = entry
	return Save(board)
}

func Save(board *Board) error {
	if board == nil {
		board = &Board{AIs: map[string]Entry{}}
	}
	if board.AIs == nil {
		board.AIs = map[string]Entry{}
	}
	path, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create scoreboard dir: %w", err)
	}
	data, err := json.MarshalIndent(board, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal scoreboard: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func Label(ai runner.AI) string {
	board, err := Load()
	if err != nil {
		return defaultName(ai)
	}
	entry := board.AIs[string(ai)]
	return fmt.Sprintf("%s [%d | P:%d E:%d F:%d]", defaultName(ai), entry.Total(), entry.Plan, entry.Execution, entry.Fix)
}

func defaultName(ai runner.AI) string {
	switch ai {
	case runner.Claude:
		return "Claude"
	case runner.Codex:
		return "Codex"
	default:
		return string(ai)
	}
}

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cloadex", fileName), nil
}
