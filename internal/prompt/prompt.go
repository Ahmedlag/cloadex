package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func WorkspaceContext() string {
	cwd, _ := os.Getwd()
	files := listTopFiles(cwd, 3)
	gitBranch := detectGitBranch()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Working directory: %s\n", cwd))
	if gitBranch != "" {
		sb.WriteString(fmt.Sprintf("Git branch: %s\n", gitBranch))
	}
	if len(files) > 0 {
		sb.WriteString("Key files:\n")
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	return sb.String()
}

// detectGitBranch uses git rev-parse to reliably get the current branch name.
func detectGitBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func listTopFiles(dir string, depth int) []string {
	var files []string
	if depth <= 0 {
		return files
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}
		files = append(files, name)
		if e.IsDir() && depth > 1 {
			sub, _ := os.ReadDir(filepath.Join(dir, name))
			for _, s := range sub {
				if !strings.HasPrefix(s.Name(), ".") {
					files = append(files, filepath.Join(name, s.Name()))
				}
			}
		}
	}
	if len(files) > 30 {
		files = files[:30]
	}
	return files
}

func DebateSystem(userPrompt string, wsContext string) string {
	return fmt.Sprintf(`You are participating in a collaborative debate with another AI to solve a software engineering task.

WORKSPACE CONTEXT:
%s

USER'S REQUEST:
%s

RULES:
1. Propose a clear, actionable plan to solve the user's request.
2. When you see the other AI's proposal, critique it constructively — identify gaps, improvements, or alternative approaches.
3. Build on good ideas from the other AI. Don't disagree for the sake of it.
4. Focus on architecture, file structure, key decisions, and implementation approach.
5. After discussion, converge toward the best combined approach.
6. Keep responses focused and concise — no filler.
7. Do not assume a specific language, framework, or project type — base your plan on the workspace context.
8. When you believe you've reached agreement, start your response with "CONVERGED:" followed by the final agreed plan as a JSON object with this schema:
   {"overview": "summary", "tasks": [{"owner_ai": "claude"|"codex", "description": "...", "scope": ["files"], "depends_on": [indices], "verification": "command"}]}`, wsContext, userPrompt)
}

func DebateRound(history string, otherAI string, otherResponse string) string {
	return fmt.Sprintf(`Here is the discussion so far:

%s

%s's latest response:
%s

Now provide your response. Either critique and improve, or if you agree with the approach, start with "CONVERGED:" followed by the final plan.`, history, otherAI, otherResponse)
}

func PlanSummary(debateHistory string) string {
	return fmt.Sprintf(`Based on this debate between two AIs, extract the final agreed-upon plan.

DEBATE:
%s

Output the plan as a JSON object with this schema:

{
  "overview": "1-2 sentence summary of what will be built/changed",
  "tasks": [
    {
      "owner_ai": "claude" or "codex",
      "description": "what this task accomplishes",
      "scope": ["path/to/file1", "path/to/dir/"],
      "depends_on": [],
      "verification": "optional shell command to verify correctness"
    }
  ]
}

Guidelines for task assignment:
- Assign each task to the AI best suited for it: "claude" for complex reasoning, architecture, multi-file refactoring, and tasks requiring deep context; "codex" for well-scoped, independent edits like scaffolding, boilerplate, configuration, or single-file changes.
- Include file paths in "scope" when known.
- Use "depends_on" (0-based task indices) when one task must finish before another starts.
- Add a "verification" command when there is a natural way to check the task (e.g. "go test ./...", "pytest tests/", "npm run lint", "cargo check").
- Tasks should be language-agnostic — do not assume a specific framework or project type.

If JSON is not possible, fall back to structured markdown with "## Claude Tasks" and "## Codex Tasks" section headers.

Output ONLY the plan, no preamble.`, debateHistory)
}

func ExecuteTask(task string, role string, wsContext string, plan string) string {
	return fmt.Sprintf(`You are executing a specific task as part of a larger implementation plan.

WORKSPACE CONTEXT:
%s

FULL PLAN:
%s

YOUR ROLE: %s

YOUR TASK:
%s

INSTRUCTIONS:
1. Implement ONLY your assigned task
2. Write clean, production-quality code
3. Create or modify files as needed
4. Do not implement tasks assigned to the other AI
5. Be thorough but focused`, wsContext, plan, role, task)
}

func ValidateImplementation(wsContext string, plan string) string {
	return fmt.Sprintf(`Review the code that was just written in this workspace for correctness.

WORKSPACE CONTEXT:
%s

IMPLEMENTATION PLAN THAT WAS EXECUTED:
%s

YOUR TASK:
1. Check that all components are properly connected — imports, exports, function signatures, and data flow
2. Look for missing references, broken dependencies, or interface mismatches
3. Identify type errors, undefined variables, or incorrect API usage
4. Verify the implementation matches the plan's intent
5. Report issues found, or confirm everything looks correct

Be concise. List issues as bullet points, or say "All clear" if no issues found.`, wsContext, plan)
}

func DiagnoseFailure(wsContext string, plan string, issue string) string {
	return fmt.Sprintf(`You are diagnosing a failed implementation branch in a collaborative coding workflow.

WORKSPACE CONTEXT:
%s

PLAN:
%s

ISSUE TO ANALYZE:
%s

Return ONLY valid JSON with this schema:
{
  "task_indices": [0],
  "cause": "brief root cause analysis",
  "fix_summary": "brief fix plan limited to the failed task and its dependent branch"
}

Rules:
- Pick the smallest task index set that explains the failure.
- Keep the fix scoped to the failed task and its downstream dependents.
- Do not include markdown fences or any prose outside the JSON object.`, wsContext, plan, issue)
}

func MiniDebate(wsContext string, plan string, issue string, firstAI string, firstProposal string, secondAI string, secondProposal string) string {
	return fmt.Sprintf(`You are in a short tie-break debate about a failed implementation branch.

WORKSPACE CONTEXT:
%s

PLAN:
%s

ISSUE:
%s

%s PROPOSAL:
%s

%s PROPOSAL:
%s

Return ONLY valid JSON with this schema:
{
  "task_indices": [0],
  "cause": "brief root cause analysis",
  "fix_summary": "brief revised fix plan"
}

Update your proposal after considering the other AI's position.
Keep the fix scoped to the failed task and its dependent branch.`, wsContext, plan, issue, firstAI, firstProposal, secondAI, secondProposal)
}

func ApplyScopedFix(wsContext string, plan string, issue string, fixSummary string, branchDescriptions []string) string {
	return fmt.Sprintf(`Apply the selected fix plan to the workspace.

WORKSPACE CONTEXT:
%s

PLAN:
%s

ISSUE:
%s

SELECTED FIX PLAN:
%s

FAILED BRANCH TASKS:
%s

Instructions:
1. Implement the selected fix plan.
2. Stay scoped to the failed task and its dependent branch.
3. Do not rewrite unrelated parts of the repository.
4. Make the minimal changes needed to resolve the issue.`, wsContext, plan, issue, fixSummary, strings.Join(branchDescriptions, "\n- "))
}

func ObserverCheckpoint(wsContext string, stage string, detail string) string {
	return fmt.Sprintf(`You are passively supervising another AI in a collaborative coding runtime.

WORKSPACE CONTEXT:
%s

CHECKPOINT STAGE:
%s

CHECKPOINT DETAIL:
%s

Return ONLY valid JSON with this schema:
{
  "verdict": "continue" | "warn" | "interrupt",
  "reason": "brief explanation"
}

Choose "interrupt" for clear drift, suspicious design issues, risky contradictions, or cases where the next step should change.
Choose "warn" for moderate concern that should be surfaced but does not require immediate control transfer.
Choose "continue" if the current direction looks fine.`, wsContext, stage, detail)
}

func FinalReview(wsContext string, plan string, validationResult string, checkSummary string) string {
	checksSection := ""
	if checkSummary != "" {
		checksSection = fmt.Sprintf("\nDETERMINISTIC CHECK RESULTS:\n%s\n", checkSummary)
	}
	return fmt.Sprintf(`Perform a final review of the implementation.

WORKSPACE CONTEXT:
%s

PLAN:
%s
%sAI VALIDATION RESULT:
%s

YOUR TASK:
1. Review the overall implementation quality
2. Check that all planned tasks were completed
3. Consider the deterministic check results (if any) — failures are critical
4. Identify any remaining issues or improvements
5. Provide a brief summary of what was built

Be concise. End with a clear status: COMPLETE, NEEDS_FIXES, or INCOMPLETE.`, wsContext, plan, checksSection, validationResult)
}

// FixFailures generates a prompt asking an AI to fix deterministic check failures.
func FixFailures(wsContext string, plan string, failedOutput string, attempt int, maxAttempts int) string {
	return fmt.Sprintf(`Deterministic verification checks have failed. Fix the issues.

WORKSPACE CONTEXT:
%s

ORIGINAL PLAN:
%s

FIX ATTEMPT: %d of %d

FAILED CHECKS AND THEIR OUTPUT:
%s

INSTRUCTIONS:
1. Analyze each failure carefully — read the error messages and identify root causes
2. Fix the code to make all checks pass
3. Do NOT change test expectations unless the tests themselves are wrong
4. Do NOT disable or skip checks
5. Make minimal, targeted fixes — do not refactor unrelated code
6. If a build or compilation fails, fix the syntax/type errors first
7. Be thorough — address ALL failures, not just the first one`, wsContext, plan, attempt, maxAttempts, failedOutput)
}
