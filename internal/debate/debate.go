package debate

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloadex-cli/cloadex/internal/prompt"
	"github.com/cloadex-cli/cloadex/internal/runner"
	"github.com/cloadex-cli/cloadex/internal/ui"
)

const MaxRounds = 5

type DebateResult struct {
	FinalPlan   string
	History     string
	Rounds      int
	ConvergedBy runner.AI
}

// Run executes the debate between Claude and Codex.
// Returns the converged plan and full debate history.
func Run(ctx context.Context, userPrompt string, maxRounds int) (*DebateResult, error) {
	if maxRounds <= 0 {
		maxRounds = MaxRounds
	}

	wsContext := prompt.WorkspaceContext()
	systemPrompt := prompt.DebateSystem(userPrompt, wsContext)

	var history strings.Builder
	var lastResponse string
	var convergedPlan string

	// Claude goes first
	currentAI := runner.Claude
	otherAI := runner.Codex

	for round := 1; round <= maxRounds; round++ {
		ui.Divider()
		ui.PrintSystem("Round %d/%d — %s's turn", round, maxRounds, currentAI)

		var fullPrompt string
		if round == 1 {
			fullPrompt = systemPrompt
		} else {
			fullPrompt = systemPrompt + "\n\n" + prompt.DebateRound(
				history.String(),
				string(otherAI),
				lastResponse,
			)
		}

		streamFn := streamForAI(currentAI)
		result := runner.Run(ctx, currentAI, fullPrompt, func(ai runner.AI, line string) {
			streamFn(line)
		})

		if result.Err != nil {
			return nil, fmt.Errorf("round %d (%s): %w", round, currentAI, result.Err)
		}

		history.WriteString(fmt.Sprintf("\n--- %s (Round %d) ---\n%s\n", currentAI, round, result.Output))
		lastResponse = result.Output

		// Check for convergence
		if strings.HasPrefix(strings.TrimSpace(result.Output), "CONVERGED:") {
			convergedPlan = strings.TrimPrefix(strings.TrimSpace(result.Output), "CONVERGED:")
			convergedPlan = strings.TrimSpace(convergedPlan)
			ui.PrintSuccess("Convergence reached in round %d!", round)
			return &DebateResult{
				FinalPlan:   convergedPlan,
				History:     history.String(),
				Rounds:      round,
				ConvergedBy: currentAI,
			}, nil
		}

		// Swap AIs
		currentAI, otherAI = otherAI, currentAI
	}

	// If no explicit convergence, ask Claude to summarize the best plan
	ui.PrintSystem("Max rounds reached — extracting best plan...")
	summaryPrompt := prompt.PlanSummary(history.String())
	result := runner.RunQuiet(ctx, runner.Claude, summaryPrompt)
	if result.Err != nil {
		return nil, fmt.Errorf("plan extraction: %w", result.Err)
	}

	return &DebateResult{
		FinalPlan:   result.Output,
		History:     history.String(),
		Rounds:      maxRounds,
		ConvergedBy: "",
	}, nil
}

func streamForAI(ai runner.AI) func(string) {
	if ai == runner.Claude {
		return ui.StreamClaude
	}
	return ui.StreamCodex
}
