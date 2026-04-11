package validate

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/cloadex-cli/cloadex/internal/execute"
	"github.com/cloadex-cli/cloadex/internal/persist"
	"github.com/cloadex-cli/cloadex/internal/plan"
	"github.com/cloadex-cli/cloadex/internal/prompt"
	"github.com/cloadex-cli/cloadex/internal/runner"
	"github.com/cloadex-cli/cloadex/internal/ui"
)

// ValidationResult holds the full outcome of validation including checks and AI review.
type ValidationResult struct {
	CheckSuite      *CheckSuite
	ReviewReport    string
	FinalReport     string
	Status          string // COMPLETE, NEEDS_FIXES, INCOMPLETE, WAITING_INPUT
	FixAttempts     int
	Execution       *execute.ExecutionResult
	PendingDecision *persist.PendingDecision
	ResolvedFixAI   runner.AI
}

// Options controls validation behavior.
type Options struct {
	MaxFixAttempts  int
	Tasks           []plan.Task
	CustomChecks    []string
	Execution       *execute.ExecutionResult
	Interactive     bool
	PendingDecision *persist.PendingDecision
}

type issue struct {
	Details string
}

var runAI = runner.Run

// Run performs deterministic checks, AI review, and the scoped fix loop.
func Run(ctx context.Context, planText string, opts Options) (*ValidationResult, error) {
	wsContext := prompt.WorkspaceContext()
	result := &ValidationResult{
		Execution: opts.Execution,
	}
	if result.Execution == nil {
		result.Execution = &execute.ExecutionResult{}
	}

	for attempt := 0; ; attempt++ {
		ui.Divider()
		ui.PrintSystem("Running deterministic verification...")
		suite := RunChecks(ctx, executableTasks(result.Execution, opts.Tasks), opts.CustomChecks)
		result.CheckSuite = suite

		if suite.Passed {
			ui.PrintSuccess("All checks passed!")
		} else {
			ui.PrintError("Some checks failed:")
			fmt.Println(suite.Summary())
		}

		ui.Divider()
		ui.PrintCodex("Reviewing implementation...")
		reviewPrompt := prompt.ValidateImplementation(wsContext, planText)
		reviewResult := runAI(ctx, runner.Codex, reviewPrompt, func(ai runner.AI, line string) {
			ui.StreamCodex(line)
		})
		if reviewResult.Err != nil {
			return nil, fmt.Errorf("implementation review: %w", reviewResult.Err)
		}
		result.ReviewReport = reviewResult.Output

		issues := collectIssues(result.Execution, suite, reviewResult.Output)
		if len(issues) == 0 {
			break
		}
		if attempt >= opts.MaxFixAttempts {
			break
		}

		result.FixAttempts = attempt + 1
		ui.Divider()
		ui.PrintSystem("Fix attempt %d/%d — consulting both AIs...", result.FixAttempts, opts.MaxFixAttempts)

		decision, chosen, err := resolveFixChoice(ctx, wsContext, planText, issues[0], opts)
		if err != nil {
			return nil, err
		}
		if decision != nil {
			result.PendingDecision = decision
			result.Status = "WAITING_INPUT"
			return result, nil
		}

		if chosen == nil {
			break
		}

		result.ResolvedFixAI = chosen.AI
		branch := branchIndices(opts.Tasks, chosen.TaskIndices)
		if len(branch) == 0 {
			branch = allRunnableTaskIndices(result.Execution, opts.Tasks)
		}

		applyPrompt := prompt.ApplyScopedFix(wsContext, planText, issues[0].Details, chosen.FixSummary, branchDescriptions(opts.Tasks, branch))
		applyResult := runAI(ctx, chosen.AI, applyPrompt, func(ai runner.AI, line string) {
			if ai == runner.Claude {
				ui.StreamClaude(line)
				return
			}
			ui.StreamCodex(line)
		})
		if applyResult.Err != nil {
			ui.PrintError("Scoped fix failed: %s", applyResult.Err)
			continue
		}

		selected := make(map[int]bool, len(branch))
		assumeSatisfied := make(map[int]bool)
		for _, idx := range branch {
			selected[idx] = true
		}
		for _, idx := range branch {
			for _, dep := range opts.Tasks[idx].DependsOn {
				if !selected[dep] {
					assumeSatisfied[dep] = true
				}
			}
		}

		branchAI := chosen.AI
		branchExec := execute.RunWithOptions(ctx, planText, opts.Tasks, execute.RunOptions{
			SelectedTasks:   selected,
			AssumeSatisfied: assumeSatisfied,
			OverrideAI:      &branchAI,
		})
		result.Execution = mergeExecution(result.Execution, branchExec)
	}

	ui.Divider()
	ui.PrintClaude("Performing final review...")
	checkSummary := ""
	if result.CheckSuite != nil && len(result.CheckSuite.Results) > 0 {
		checkSummary = result.CheckSuite.Summary()
	}
	finalPrompt := prompt.FinalReview(wsContext, planText, result.ReviewReport, checkSummary)
	finalResult := runAI(ctx, runner.Claude, finalPrompt, func(ai runner.AI, line string) {
		ui.StreamClaude(line)
	})
	if finalResult.Err != nil {
		return nil, fmt.Errorf("final review: %w", finalResult.Err)
	}
	result.FinalReport = finalResult.Output
	result.Status = extractStatus(finalResult.Output)
	if result.CheckSuite != nil && !result.CheckSuite.Passed && result.Status == "COMPLETE" {
		result.Status = "NEEDS_FIXES"
	}
	if hasExecutableFailures(result.Execution) && result.Status == "COMPLETE" {
		result.Status = "NEEDS_FIXES"
	}
	return result, nil
}

func resolveFixChoice(ctx context.Context, wsContext string, planText string, current issue, opts Options) (*persist.PendingDecision, *persist.ProposalOption, error) {
	if opts.PendingDecision != nil {
		chosen, pending, err := chooseBetween(ctx, wsContext, planText, current.Details, *opts.PendingDecision, opts.Interactive)
		if err != nil {
			return nil, nil, err
		}
		return pending, chosen, nil
	}

	first, err := consultAI(ctx, runner.Claude, wsContext, planText, current.Details)
	if err != nil {
		return nil, nil, err
	}
	second, err := consultAI(ctx, runner.Codex, wsContext, planText, current.Details)
	if err != nil {
		return nil, nil, err
	}

	decision := persist.PendingDecision{
		Issue:       current.Details,
		TaskIndices: uniqueSorted(append(append([]int{}, first.TaskIndices...), second.TaskIndices...)),
		OptionOne:   first,
		OptionTwo:   second,
	}
	chosen, pending, err := chooseBetween(ctx, wsContext, planText, current.Details, decision, opts.Interactive)
	if err != nil {
		return nil, nil, err
	}
	return pending, chosen, nil
}

func chooseBetween(ctx context.Context, wsContext string, planText string, issueText string, decision persist.PendingDecision, interactive bool) (*persist.ProposalOption, *persist.PendingDecision, error) {
	if proposalsAgree(decision.OptionOne, decision.OptionTwo) {
		return &decision.OptionOne, nil, nil
	}
	if !interactive {
		return nil, &decision, nil
	}

	reader := bufio.NewReader(os.Stdin)
	current := decision
	for {
		ui.Divider()
		ui.PrintSystem("AI disagreement detected for the failed branch:")
		fmt.Println(current.Issue)
		fmt.Println()
		fmt.Printf("1. %s\n   Cause: %s\n   Fix: %s\n\n", current.OptionOne.AI, current.OptionOne.Cause, current.OptionOne.FixSummary)
		fmt.Printf("2. %s\n   Cause: %s\n   Fix: %s\n\n", current.OptionTwo.AI, current.OptionTwo.Cause, current.OptionTwo.FixSummary)
		fmt.Print("Choose 1, 2, or 3 for a short mini debate: ")

		input, _ := reader.ReadString('\n')
		switch strings.TrimSpace(input) {
		case "1":
			return &current.OptionOne, nil, nil
		case "2":
			return &current.OptionTwo, nil, nil
		case "3":
			updated, err := runMiniDebate(ctx, wsContext, planText, current)
			if err != nil {
				return nil, nil, err
			}
			current = updated
			if proposalsAgree(current.OptionOne, current.OptionTwo) {
				return &current.OptionOne, nil, nil
			}
			ui.PrintSystem("Mini debate complete. The AIs still disagree; please choose 1 or 2.")
		default:
			ui.PrintError("Invalid choice. Enter 1, 2, or 3.")
		}
	}
}

func runMiniDebate(ctx context.Context, wsContext string, planText string, decision persist.PendingDecision) (persist.PendingDecision, error) {
	firstPrompt := prompt.MiniDebate(wsContext, planText, decision.Issue, string(decision.OptionOne.AI), decision.OptionOne.FixSummary, string(decision.OptionTwo.AI), decision.OptionTwo.FixSummary)
	firstResult := runAI(ctx, decision.OptionOne.AI, firstPrompt, nil)
	if firstResult.Err != nil {
		return persist.PendingDecision{}, fmt.Errorf("mini debate (%s): %w", decision.OptionOne.AI, firstResult.Err)
	}
	secondPrompt := prompt.MiniDebate(wsContext, planText, decision.Issue, string(decision.OptionTwo.AI), decision.OptionTwo.FixSummary, string(decision.OptionOne.AI), decision.OptionOne.FixSummary)
	secondResult := runAI(ctx, decision.OptionTwo.AI, secondPrompt, nil)
	if secondResult.Err != nil {
		return persist.PendingDecision{}, fmt.Errorf("mini debate (%s): %w", decision.OptionTwo.AI, secondResult.Err)
	}

	first, err := parseProposal(decision.OptionOne.AI, firstResult.Output)
	if err != nil {
		return persist.PendingDecision{}, err
	}
	second, err := parseProposal(decision.OptionTwo.AI, secondResult.Output)
	if err != nil {
		return persist.PendingDecision{}, err
	}

	return persist.PendingDecision{
		Issue:      decision.Issue,
		OptionOne:  first,
		OptionTwo:  second,
		MiniDebate: fmt.Sprintf("%s: %s\n\n%s: %s", decision.OptionOne.AI, firstResult.Output, decision.OptionTwo.AI, secondResult.Output),
	}, nil
}

func consultAI(ctx context.Context, ai runner.AI, wsContext string, planText string, issueText string) (persist.ProposalOption, error) {
	consultPrompt := prompt.DiagnoseFailure(wsContext, planText, issueText)
	result := runAI(ctx, ai, consultPrompt, nil)
	if result.Err != nil {
		return persist.ProposalOption{}, fmt.Errorf("consult %s: %w", ai, result.Err)
	}
	return parseProposal(ai, result.Output)
}

func parseProposal(ai runner.AI, output string) (persist.ProposalOption, error) {
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start == -1 || end == -1 || end < start {
		return persist.ProposalOption{}, fmt.Errorf("parse %s proposal: missing JSON object", ai)
	}

	var parsed struct {
		TaskIndices []int  `json:"task_indices"`
		Cause       string `json:"cause"`
		FixSummary  string `json:"fix_summary"`
	}
	if err := json.Unmarshal([]byte(output[start:end+1]), &parsed); err != nil {
		return persist.ProposalOption{}, fmt.Errorf("parse %s proposal: %w", ai, err)
	}
	return persist.ProposalOption{
		AI:          ai,
		Cause:       strings.TrimSpace(parsed.Cause),
		FixSummary:  strings.TrimSpace(parsed.FixSummary),
		TaskIndices: uniqueSorted(parsed.TaskIndices),
	}, nil
}

func proposalsAgree(first persist.ProposalOption, second persist.ProposalOption) bool {
	if normalize(first.FixSummary) == "" || normalize(second.FixSummary) == "" {
		return false
	}
	return strings.Join(intStrings(first.TaskIndices), ",") == strings.Join(intStrings(second.TaskIndices), ",") &&
		normalize(first.FixSummary) == normalize(second.FixSummary)
}

func collectIssues(execResult *execute.ExecutionResult, suite *CheckSuite, review string) []issue {
	var issues []issue
	if execResult != nil {
		for _, result := range execResult.Results {
			switch result.Status {
			case execute.TaskFailed:
				issues = append(issues, issue{
					Details: fmt.Sprintf("Task %d failed: %s\n%s", result.TaskIndex, result.Task.Description, result.Error),
				})
			}
		}
	}
	if suite != nil {
		for _, r := range suite.Results {
			if r.Passed {
				continue
			}
			issues = append(issues, issue{
				Details: fmt.Sprintf("Deterministic check failed: %s\nCommand: %s\nOutput:\n%s", r.Name, r.Command, r.Output),
			})
		}
	}
	review = strings.TrimSpace(review)
	if review != "" && !strings.Contains(strings.ToLower(review), "all clear") {
		issues = append(issues, issue{Details: "AI review found a concrete issue:\n" + review})
	}
	return issues
}

func executableTasks(execResult *execute.ExecutionResult, tasks []plan.Task) []plan.Task {
	if execResult == nil || len(execResult.Results) == 0 {
		return tasks
	}
	allowed := map[int]bool{}
	for _, result := range execResult.Results {
		if result.Status == execute.TaskSkipped {
			continue
		}
		allowed[result.TaskIndex] = true
	}
	filtered := make([]plan.Task, 0, len(allowed))
	for i, task := range tasks {
		if allowed[i] {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func mergeExecution(base *execute.ExecutionResult, update *execute.ExecutionResult) *execute.ExecutionResult {
	if base == nil {
		return update
	}
	if update == nil {
		return base
	}
	results := map[int]execute.TaskResult{}
	for _, result := range base.Results {
		results[result.TaskIndex] = result
	}
	for _, result := range update.Results {
		results[result.TaskIndex] = result
	}
	ordered := make([]int, 0, len(results))
	for idx := range results {
		ordered = append(ordered, idx)
	}
	sort.Ints(ordered)

	merged := &execute.ExecutionResult{Errors: append([]string{}, base.Errors...)}
	merged.Errors = append(merged.Errors, update.Errors...)
	for _, idx := range ordered {
		merged.Results = append(merged.Results, results[idx])
	}
	return merged
}

func branchIndices(tasks []plan.Task, roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	selected := make(map[int]bool, len(roots))
	queue := append([]int{}, roots...)
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		if selected[idx] {
			continue
		}
		selected[idx] = true
		for candidate, task := range tasks {
			for _, dep := range task.DependsOn {
				if dep == idx {
					queue = append(queue, candidate)
					break
				}
			}
		}
	}
	indices := make([]int, 0, len(selected))
	for idx := range selected {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	return indices
}

func branchDescriptions(tasks []plan.Task, indices []int) []string {
	descriptions := make([]string, 0, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(tasks) {
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("- Task %d: %s", idx, tasks[idx].Description))
	}
	return descriptions
}

func allRunnableTaskIndices(execResult *execute.ExecutionResult, tasks []plan.Task) []int {
	if execResult == nil || len(execResult.Results) == 0 {
		indices := make([]int, 0, len(tasks))
		for i := range tasks {
			indices = append(indices, i)
		}
		return indices
	}
	indices := make([]int, 0, len(execResult.Results))
	for _, result := range execResult.Results {
		if result.Status != execute.TaskSkipped {
			indices = append(indices, result.TaskIndex)
		}
	}
	sort.Ints(indices)
	return indices
}

func hasExecutableFailures(execResult *execute.ExecutionResult) bool {
	if execResult == nil {
		return false
	}
	for _, result := range execResult.Results {
		if result.Status == execute.TaskFailed {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(".", "", ",", "", ":", "", ";", "", "\n", " ", "\t", " ")
	return strings.Join(strings.Fields(replacer.Replace(s)), " ")
}

func uniqueSorted(values []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func intStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}

func extractStatus(output string) string {
	upper := strings.ToUpper(output)
	if strings.Contains(upper, "WAITING_INPUT") {
		return "WAITING_INPUT"
	}
	if strings.Contains(upper, "NEEDS_FIXES") {
		return "NEEDS_FIXES"
	}
	if strings.Contains(upper, "INCOMPLETE") {
		return "INCOMPLETE"
	}
	return "COMPLETE"
}
