package execute

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cloadex-cli/cloadex/internal/plan"
	"github.com/cloadex-cli/cloadex/internal/prompt"
	"github.com/cloadex-cli/cloadex/internal/runner"
	"github.com/cloadex-cli/cloadex/internal/ui"
)

type TaskStatus string

const (
	TaskPending TaskStatus = "pending"
	TaskRunning TaskStatus = "running"
	TaskSuccess TaskStatus = "success"
	TaskFailed  TaskStatus = "failed"
	TaskSkipped TaskStatus = "skipped"
)

type TaskResult struct {
	TaskIndex             int        `json:"task_index"`
	Task                  plan.Task  `json:"task"`
	ExecutorAI            runner.AI  `json:"executor_ai"`
	Status                TaskStatus `json:"status"`
	Output                string     `json:"output,omitempty"`
	Error                 string     `json:"error,omitempty"`
	SkipReason            string     `json:"skip_reason,omitempty"`
	ExecutionPointAwarded bool       `json:"execution_point_awarded,omitempty"`
}

type ExecutionResult struct {
	Results []TaskResult `json:"results"`
	Errors  []string     `json:"errors,omitempty"`
}

type RunOptions struct {
	// SelectedTasks limits execution to the given task indices. Nil means all tasks.
	SelectedTasks map[int]bool
	// AssumeSatisfied marks dependencies outside SelectedTasks as already satisfied.
	AssumeSatisfied map[int]bool
	// OverrideAI forces every selected task to run with the same AI.
	OverrideAI *runner.AI
}

var runAI = runner.Run

// Run executes all tasks in the plan with dependency-aware scheduling.
func Run(ctx context.Context, planText string, tasks []plan.Task) *ExecutionResult {
	return RunWithOptions(ctx, planText, tasks, RunOptions{})
}

// RunWithOptions executes a selected subset of tasks with dependency-aware scheduling.
func RunWithOptions(ctx context.Context, planText string, tasks []plan.Task, opts RunOptions) *ExecutionResult {
	wsContext := prompt.WorkspaceContext()
	result := &ExecutionResult{}
	if len(tasks) == 0 {
		return result
	}

	selected := opts.SelectedTasks
	if len(selected) == 0 {
		selected = make(map[int]bool, len(tasks))
		for i := range tasks {
			selected[i] = true
		}
	}

	statusByTask := make(map[int]TaskStatus, len(tasks))
	resultsByTask := make(map[int]TaskResult, len(tasks))

	counts := map[runner.AI]int{}
	for idx := range selected {
		ai := tasks[idx].OwnerAI
		if opts.OverrideAI != nil {
			ai = *opts.OverrideAI
		}
		counts[ai]++
	}
	ais := make([]string, 0, len(counts))
	for ai, count := range counts {
		ui.PrintSystem("Executing %d task(s) with %s", count, ai)
		ais = append(ais, string(ai))
	}

	for {
		if len(resultsByTask) == len(selected) {
			break
		}

		ready := make([]int, 0)
		progressed := false
		for idx := range selected {
			if _, done := resultsByTask[idx]; done {
				continue
			}
			if reason, blocked := blockedDependency(idx, tasks, selected, statusByTask, opts.AssumeSatisfied); blocked {
				resultsByTask[idx] = TaskResult{
					TaskIndex:  idx,
					Task:       tasks[idx],
					ExecutorAI: executorAI(tasks[idx], opts.OverrideAI),
					Status:     TaskSkipped,
					SkipReason: reason,
				}
				statusByTask[idx] = TaskSkipped
				progressed = true
				continue
			}
			if dependenciesSatisfied(idx, tasks, selected, statusByTask, opts.AssumeSatisfied) {
				ready = append(ready, idx)
			}
		}

		sort.Ints(ready)
		if len(ready) == 0 {
			if progressed {
				continue
			}
			for idx := range selected {
				if _, done := resultsByTask[idx]; done {
					continue
				}
				resultsByTask[idx] = TaskResult{
					TaskIndex:  idx,
					Task:       tasks[idx],
					ExecutorAI: executorAI(tasks[idx], opts.OverrideAI),
					Status:     TaskSkipped,
					SkipReason: "SKIPPED because DEPENDS on: unresolved dependency chain",
				}
				statusByTask[idx] = TaskSkipped
			}
			break
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, idx := range ready {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				t := tasks[idx]
				ai := executorAI(t, opts.OverrideAI)

				ui.Divider()
				streamFn := streamForAI(ai)
				printForAI(ai, "Executing task %d: %s", idx+1, truncate(t.Description, 80))

				role := roleLabel(ai)
				taskPrompt := prompt.ExecuteTask(t.Description, role, wsContext, planText)
				runResult := runAI(ctx, ai, taskPrompt, func(_ runner.AI, line string) {
					streamFn(line)
				})

				taskResult := TaskResult{
					TaskIndex:  idx,
					Task:       t,
					ExecutorAI: ai,
					Output:     runResult.Output,
				}
				if runResult.Err != nil {
					taskResult.Status = TaskFailed
					taskResult.Error = runResult.Err.Error()
				} else {
					taskResult.Status = TaskSuccess
				}

				mu.Lock()
				resultsByTask[idx] = taskResult
				statusByTask[idx] = taskResult.Status
				if taskResult.Error != "" {
					result.Errors = append(result.Errors, fmt.Sprintf("%s task %d: %s", ai, idx+1, taskResult.Error))
				}
				mu.Unlock()
			}(idx)
		}
		wg.Wait()
	}

	ordered := make([]int, 0, len(resultsByTask))
	for idx := range resultsByTask {
		ordered = append(ordered, idx)
	}
	sort.Ints(ordered)
	for _, idx := range ordered {
		result.Results = append(result.Results, resultsByTask[idx])
	}

	return result
}

func executorAI(task plan.Task, override *runner.AI) runner.AI {
	if override != nil {
		return *override
	}
	return task.OwnerAI
}

func dependenciesSatisfied(idx int, tasks []plan.Task, selected map[int]bool, statusByTask map[int]TaskStatus, assumeSatisfied map[int]bool) bool {
	for _, dep := range tasks[idx].DependsOn {
		if assumeSatisfied != nil && assumeSatisfied[dep] {
			continue
		}
		if len(selected) > 0 && !selected[dep] {
			continue
		}
		if statusByTask[dep] != TaskSuccess {
			return false
		}
	}
	return true
}

func blockedDependency(idx int, tasks []plan.Task, selected map[int]bool, statusByTask map[int]TaskStatus, assumeSatisfied map[int]bool) (string, bool) {
	for _, dep := range tasks[idx].DependsOn {
		if assumeSatisfied != nil && assumeSatisfied[dep] {
			continue
		}
		if len(selected) > 0 && !selected[dep] {
			continue
		}
		if statusByTask[dep] == TaskFailed || statusByTask[dep] == TaskSkipped {
			return fmt.Sprintf("SKIPPED because DEPENDS on: %s", tasks[dep].Description), true
		}
	}
	return "", false
}

func roleLabel(ai runner.AI) string {
	switch ai {
	case runner.Claude:
		return "Developer (Claude)"
	case runner.Codex:
		return "Developer (Codex)"
	default:
		return string(ai)
	}
}

func printForAI(ai runner.AI, format string, args ...any) {
	switch ai {
	case runner.Claude:
		ui.PrintClaude(format, args...)
	case runner.Codex:
		ui.PrintCodex(format, args...)
	default:
		ui.PrintSystem(format, args...)
	}
}

func streamForAI(ai runner.AI) func(string) {
	switch ai {
	case runner.Claude:
		return ui.StreamClaude
	case runner.Codex:
		return ui.StreamCodex
	default:
		return func(line string) { ui.PrintSystem("%s", line) }
	}
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
