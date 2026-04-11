package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/Ahmedlag/cloadex/internal/config"
	"github.com/Ahmedlag/cloadex/internal/debate"
	"github.com/Ahmedlag/cloadex/internal/execute"
	"github.com/Ahmedlag/cloadex/internal/persist"
	"github.com/Ahmedlag/cloadex/internal/plan"
	"github.com/Ahmedlag/cloadex/internal/prompt"
	"github.com/Ahmedlag/cloadex/internal/runner"
	"github.com/Ahmedlag/cloadex/internal/score"
	"github.com/Ahmedlag/cloadex/internal/session"
	"github.com/Ahmedlag/cloadex/internal/sessionstate"
	"github.com/Ahmedlag/cloadex/internal/ui"
	"github.com/Ahmedlag/cloadex/internal/validate"
)

// isInteractive reports whether stdin is a terminal.
// Replaced in tests with a fake.
var isInteractive = func() bool {
	return session.IsInteractive(os.Stdin)
}

// newSession creates and returns a configured *session.Session.
// Replaced in tests with a fake.
var newSession = func(onPrompt session.PromptHandler) *session.Session {
	return &session.Session{
		OnPrompt: onPrompt,
	}
}

var version = "dev"

// runContext tracks the active run's cancellation and ID for interrupt handling.
// A single runContext is shared across all prompts in a session, avoiding
// repeated signal.Notify goroutine creation.
type runContext struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	activeID string
}

// begin creates a new cancellable context for a run and returns the context
// and a setRunID callback for tracking the active run ID.
func (rc *runContext) begin() (context.Context, func(string)) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	rc.cancel = cancel
	rc.activeID = ""
	return ctx, func(id string) {
		rc.mu.Lock()
		rc.activeID = id
		rc.mu.Unlock()
	}
}

// end cancels the current context and resets state.
func (rc *runContext) end() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.cancel != nil {
		rc.cancel()
		rc.cancel = nil
	}
	rc.activeID = ""
}

// interrupt handles a signal by marking the active run as interrupted and
// cancelling the current context.
func (rc *runContext) interrupt() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.activeID != "" {
		_ = persist.UpdateStatus(rc.activeID, persist.StatusInterrupted)
	}
	if session := runner.RegisteredSession(runner.Claude); session != nil {
		session.Interrupt()
	}
	if session := runner.RegisteredSession(runner.Codex); session != nil {
		session.Interrupt()
	}
	if rc.cancel != nil {
		rc.cancel()
	}
}

// startSignalHandler starts a single goroutine that listens for SIGINT/SIGTERM
// and interrupts the given runContext. Returns a cleanup function that stops
// signal delivery and terminates the goroutine.
func startSignalHandler(rc *runContext, extraInterrupt func()) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			signal.Stop(sigCh)
			close(done)
		})
	}
	go func() {
		signalLoop(sigCh, done, func() {
			ui.PrintSystem("Interrupted — shutting down...")
			rc.interrupt()
			if extraInterrupt != nil {
				extraInterrupt()
			}
			stop()
		})
	}()
	return func() {
		stop()
	}
}

func signalLoop(sigCh <-chan os.Signal, done <-chan struct{}, onInterrupt func()) {
	select {
	case <-sigCh:
		if onInterrupt != nil {
			onInterrupt()
		}
	case <-done:
	}
}

func Execute() error {
	return executeWithArgs(os.Args[1:])
}

// executeWithArgs is the testable core of dispatch.
func executeWithArgs(args []string) error {
	if len(args) == 0 {
		return dispatchNoArgs()
	}

	// Handle top-level flags before subcommand dispatch
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Printf("cloadex %s\n", version)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return nil
	}

	// Subcommands
	switch args[0] {
	case "runs":
		return cmdRuns()
	case "show":
		id := ""
		if len(args) > 1 {
			id = args[1]
		}
		return cmdShow(id)
	case "init":
		return cmdInit()
	case "resume":
		return cmdResume()
	case "session":
		return cmdSession()
	}

	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	args = parseFlags(args, &opts, cliSet)

	// Merge config file (CLI flags take precedence)
	if err := config.LoadFile(&opts, cliSet); err != nil {
		ui.PrintError("Config: %s", err)
	}

	ui.SetVerbose(opts.Verbose)

	userPrompt := strings.Join(args, " ")
	if strings.TrimSpace(userPrompt) == "" {
		return fmt.Errorf("please provide a prompt")
	}

	rc := &runContext{}
	cleanup := startSignalHandler(rc, nil)
	defer cleanup()

	ctx, setRunID := rc.begin()
	defer rc.end()

	return run(ctx, userPrompt, opts, setRunID)
}

// dispatchNoArgs handles the bare `cloadex` invocation with no arguments.
// If stdin is a TTY, it starts an interactive session; otherwise it prints usage.
func dispatchNoArgs() error {
	if isInteractive() {
		return cmdSession()
	}
	printUsage()
	return nil
}

// cmdSession starts an interactive REPL session.
func cmdSession() error {
	rc := &runContext{}

	state, err := sessionstate.LoadOrInit()
	if err != nil {
		return err
	}
	refreshAILabels()
	agentSessions := loadAgentSessions(state)
	registerAgentSessions(agentSessions)
	defer func() {
		saveAgentSessions(state, agentSessions)
		unregisterAgentSessions()
	}()

	promptHandler := func(prompt string) error {
		resumeRegisteredAgentSessions()
		opts := config.DefaultOptions()
		cliSet := make(map[string]bool)
		if err := config.LoadFile(&opts, cliSet); err != nil {
			ui.PrintError("Config: %s", err)
		}
		ui.SetVerbose(opts.Verbose)

		ctx, setRunID := rc.begin()
		defer rc.end()
		err := runForSession(ctx, prompt, opts, setRunID, state)
		saveAgentSessions(state, agentSessions)
		return err
	}

	resumeHandler := func() error {
		resumeRegisteredAgentSessions()
		opts := config.DefaultOptions()
		cliSet := make(map[string]bool)
		if err := config.LoadFile(&opts, cliSet); err != nil {
			ui.PrintError("Config: %s", err)
		}
		ui.SetVerbose(opts.Verbose)

		ctx, setRunID := rc.begin()
		defer rc.end()

		err := doResume(ctx, opts, setRunID)
		saveAgentSessions(state, agentSessions)
		return err
	}

	sess := newSession(promptHandler)
	sess.OnResume = resumeHandler
	sess.OnCommand = func(command session.SlashCommand, args string) error {
		err := handleSessionCommand(command, args, state, rc)
		saveAgentSessions(state, agentSessions)
		return err
	}
	sess.OnCycleMode = func() error {
		state.Mode = sessionstate.NextMode(state.Mode)
		return sessionstate.Save(state)
	}
	if sess.In == nil {
		sess.In = os.Stdin
	}
	sess.PromptRenderer = func() string {
		return ui.SessionPrompt(string(state.Mode))
	}
	sess.Header = sessionHeader(state)
	cleanup := startSignalHandler(rc, func() {
		if closer, ok := sess.In.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	defer cleanup()
	return sess.Run()
}

// parseFlags extracts known flags from args, modifies opts, and returns remaining args.
// cliSet tracks which flags were explicitly set on the command line.
func parseFlags(args []string, opts *config.Options, cliSet map[string]bool) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--rounds":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.MaxRounds)
				cliSet["rounds"] = true
				i++
			}
		case "--max-fixes":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.MaxFixAttempts)
				cliSet["max-fixes"] = true
				i++
			}
		case "--no-fix":
			opts.MaxFixAttempts = 0
			cliSet["max-fixes"] = true
		case "--dry-run":
			opts.DryRun = true
		case "--yes", "-y":
			opts.Yes = true
			cliSet["yes"] = true
		case "--verbose":
			opts.Verbose = true
			cliSet["verbose"] = true
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining
}

// run executes the full cloadex pipeline, persisting state at each phase boundary.
func run(ctx context.Context, userPrompt string, opts config.Options, setRunID func(string)) error {
	return runWithState(ctx, userPrompt, opts, setRunID, nil)
}

func runForSession(ctx context.Context, userPrompt string, opts config.Options, setRunID func(string), state *sessionstate.State) error {
	switch state.Mode {
	case sessionstate.ModePlanning:
		return runPlanning(ctx, userPrompt, state)
	case sessionstate.ModeExecution:
		return runWithState(ctx, userPrompt, opts, setRunID, state)
	default:
		return runChat(ctx, userPrompt, state)
	}
}

func runChat(ctx context.Context, userPrompt string, state *sessionstate.State) error {
	refreshAILabels()
	if err := checkDependencies(); err != nil {
		return err
	}
	wsContext := prompt.WorkspaceContext()
	summary := ""
	if state != nil {
		summary = state.SummaryForPrompt()
	}
	ui.PrintSystem("CHAT mode is read-only.")
	spinner := ui.NewSpinner("CHAT thinking…")
	var spinnerOnce sync.Once
	stopSpinner := func() {
		spinnerOnce.Do(func() {
			spinner.Stop("")
		})
	}
	result := runner.Run(ctx, runner.Codex, prompt.ChatSession(wsContext, summary, userPrompt), func(ai runner.AI, line string) {
		stopSpinner()
		ui.StreamCodex(line)
	})
	stopSpinner()
	if result.Err != nil {
		return result.Err
	}
	if state != nil {
		state.ActiveGoal = userPrompt
		state.RecordTurn("user", userPrompt)
		state.RecordTurn("chat", truncateForMemory(result.Output))
		state.RecordEvent("phase_complete", "chat", "codex", "Chat response completed.")
		return sessionstate.Save(state)
	}
	return nil
}

func runPlanning(ctx context.Context, userPrompt string, state *sessionstate.State) error {
	refreshAILabels()
	if err := checkDependencies(); err != nil {
		return err
	}
	wsContext := prompt.WorkspaceContext()
	summary := ""
	if state != nil {
		summary = state.SummaryForPrompt()
	}
	ui.PrintSystem("PLANNING mode is read-only.")
	spinner := ui.NewSpinner("PLANNING thinking…")
	var spinnerOnce sync.Once
	stopSpinner := func() {
		spinnerOnce.Do(func() {
			spinner.Stop("")
		})
	}
	result := runner.Run(ctx, runner.Claude, prompt.PlanningSession(wsContext, summary, userPrompt), func(ai runner.AI, line string) {
		stopSpinner()
		ui.StreamClaude(line)
	})
	stopSpinner()
	if result.Err != nil {
		return result.Err
	}
	if state != nil {
		state.ActiveGoal = userPrompt
		state.LastPlan = result.Output
		state.RecordTurn("user", userPrompt)
		state.RecordTurn("planning", truncateForMemory(result.Output))
		state.RecordEvent("phase_complete", "planning", "claude", "Planning response completed.")
		return sessionstate.Save(state)
	}
	return nil
}

func runWithState(ctx context.Context, userPrompt string, opts config.Options, setRunID func(string), state *sessionstate.State) error {
	refreshAILabels()
	ui.Banner()
	ui.PrintSystem("Prompt: %s", userPrompt)
	ui.PrintVerbose("Max debate rounds: %d", opts.MaxRounds)
	if opts.DryRun {
		ui.PrintSystem("Mode: dry-run (plan only, no execution)")
	}
	if opts.Yes {
		ui.PrintVerbose("Auto-approve enabled (--yes)")
	}
	composedPrompt := userPrompt
	if state != nil {
		state.ActiveGoal = userPrompt
		if summary := state.SummaryForPrompt(); strings.TrimSpace(summary) != "" {
			composedPrompt = userPrompt + "\n\nSESSION CONTEXT:\n" + summary
		}
		state.RecordTurn("user", userPrompt)
		state.RecordEvent("user_prompt", "input", "user", truncateForMemory(userPrompt))
		_ = sessionstate.Save(state)
	}

	if err := checkDependencies(); err != nil {
		return err
	}

	// Create a run manifest before starting work.
	persist.EnsureGitignore()
	manifest, err := persist.CreateRun(userPrompt)
	if err != nil {
		ui.PrintVerbose("Could not create run manifest: %s", err)
		// Non-fatal: continue without persistence.
	}
	runID := ""
	if manifest != nil {
		runID = manifest.ID
		setRunID(runID)
		ui.PrintVerbose("Run ID: %s", runID)
	}

	// Phase 1: Debate
	debateResult, planDecision, err := runDebatePhase(ctx, composedPrompt, opts)
	if err != nil {
		return err
	}
	if state != nil {
		state.RecordEvent("phase_complete", "debate", "cloadex", fmt.Sprintf("Debate completed in %d rounds.", debateResult.Rounds))
		if checkpointErr := runObserverCheckpoint(ctx, state, "debate", debateResult.ConvergedBy, truncateForMemory(debateResult.FinalPlan)); checkpointErr != nil {
			ui.PrintVerbose("Observer checkpoint (debate): %s", checkpointErr)
		}
		_ = sessionstate.Save(state)
	}
	if planDecision != plan.Edit && debateResult.ConvergedBy != "" {
		_ = score.AddPoint(debateResult.ConvergedBy, "plan")
		refreshAILabels()
	}

	// Save plan artifacts immediately after approval.
	if runID != "" {
		_ = persist.SavePlanArtifacts(runID, debateResult.FinalPlan, debateResult.History)
		_ = persist.UpdateStatus(runID, persist.StatusApproved)
	}
	if state != nil {
		state.LastPlan = debateResult.FinalPlan
		state.LastRunID = runID
		if planDecision == plan.Edit {
			state.Pin("plan_decision", "user edited approved plan; automatic plan scoring disabled")
		} else {
			state.Pin("approved_plan", truncateForMemory(debateResult.FinalPlan))
		}
		state.RecordEvent("plan_approved", "plan", "cloadex", "Approved merged plan.")
		_ = sessionstate.Save(state)
	}

	// Dry-run: stop before execution.
	if opts.DryRun {
		ui.Divider()
		ui.PrintSystem("Dry-run complete — skipping execution and validation.")
		if runID != "" {
			_ = persist.UpdateStatus(runID, persist.StatusDone)
		}
		ui.PrintSystem("Run saved to %s", persist.RunDir(runID))
		if state != nil {
			state.RecordTurn("cloadex", "Created a plan without execution.")
			state.RecordEvent("dry_run_complete", "plan", "cloadex", "Created a plan without execution.")
			_ = sessionstate.Save(state)
		}
		return nil
	}

	// Phase 3: Execute
	execResult, err := runExecutePhase(ctx, debateResult, runID)
	if err != nil {
		return err
	}
	if state != nil {
		state.RecordEvent("phase_complete", "execution", "cloadex", summarizeExecution(execResult))
		if checkpointErr := runObserverCheckpoint(ctx, state, "execution", runner.Codex, summarizeExecution(execResult)); checkpointErr != nil {
			ui.PrintVerbose("Observer checkpoint (execution): %s", checkpointErr)
		}
		_ = sessionstate.Save(state)
	}

	// Phase 4: Validate
	err = runValidatePhase(ctx, debateResult, execResult, opts, runID, nil)
	if state != nil {
		if err != nil {
			state.Pin("risk", truncateForMemory(err.Error()))
			state.RecordEvent("phase_error", "validation", "cloadex", truncateForMemory(err.Error()))
		} else {
			state.RecordTurn("cloadex", fmt.Sprintf("Completed run %s.", runID))
			state.RecordEvent("phase_complete", "validation", "cloadex", "Validation completed.")
			if checkpointErr := runObserverCheckpoint(ctx, state, "validation", runner.Claude, "Validation completed without runtime error."); checkpointErr != nil {
				ui.PrintVerbose("Observer checkpoint (validation): %s", checkpointErr)
			}
		}
		_ = sessionstate.Save(state)
	}
	return err
}

// runDebatePhase runs the debate and plan-review loop. Returns the approved debate result.
func runDebatePhase(ctx context.Context, userPrompt string, opts config.Options) (*debate.DebateResult, plan.Decision, error) {
	var debateResult *debate.DebateResult
	for {
		ui.PhaseHeader(1, "DEBATE")
		ui.PrintSystem("Claude and Codex are debating your request...")

		var err error
		debateResult, err = debate.Run(ctx, userPrompt, opts.MaxRounds)
		if err != nil {
			return nil, plan.Reject, fmt.Errorf("debate failed: %w", err)
		}
		ui.PrintSuccess("Debate completed in %d round(s)", debateResult.Rounds)

		// Phase 2: Plan Review
		ui.PhaseHeader(2, "PLAN REVIEW")
		decision, approvedPlan := plan.Present(debateResult.FinalPlan, opts.Yes)

		switch decision {
		case plan.Approve:
			debateResult.FinalPlan = approvedPlan
		case plan.Edit:
			debateResult.FinalPlan = approvedPlan
		case plan.Reject:
			continue // Restart debate
		}
		return debateResult, decision, nil
	}
}

// runExecutePhase runs the execution phase and persists results.
func runExecutePhase(ctx context.Context, debateResult *debate.DebateResult, runID string) (*execute.ExecutionResult, error) {
	if runID != "" {
		_ = persist.UpdateStatus(runID, persist.StatusExecuting)
	}

	ui.PhaseHeader(3, "EXECUTION")
	parsed := plan.ParsePlan(debateResult.FinalPlan)
	if parsed.Overview != "" {
		ui.PrintSystem("Plan: %s", parsed.Overview)
	}
	ui.PrintVerbose("Parsed %d task(s) from plan", len(parsed.Tasks))

	ui.PrintSystem("Executing %d task(s)...", len(parsed.Tasks))
	execResult := execute.Run(ctx, debateResult.FinalPlan, parsed.Tasks)
	ui.PrintSuccess("Execution finished (%d task(s))", len(execResult.Results))

	if len(execResult.Errors) > 0 {
		ui.PrintError("Some tasks had errors:")
		for _, err := range execResult.Errors {
			ui.PrintError("  - %s", err)
		}
	}

	// Build execution summary
	var execSummary strings.Builder
	for _, r := range execResult.Results {
		execSummary.WriteString(fmt.Sprintf("## Task %d (%s)\nStatus: %s\n", r.TaskIndex, r.ExecutorAI, strings.ToUpper(string(r.Status))))
		if r.SkipReason != "" {
			execSummary.WriteString(r.SkipReason + "\n")
		}
		if r.Error != "" {
			execSummary.WriteString(r.Error + "\n")
		}
		if r.Output != "" {
			execSummary.WriteString(r.Output + "\n")
		}
		execSummary.WriteString("\n")
	}

	if runID != "" {
		summary := execSummary.String()
		_ = persist.SaveExecutionArtifact(runID, summary)
		if data, err := json.MarshalIndent(execResult, "", "  "); err == nil {
			_ = persist.SaveExecutionState(runID, data)
		}
	}
	return execResult, nil
}

// runValidatePhase runs validation and finalises the run.
func runValidatePhase(ctx context.Context, debateResult *debate.DebateResult, execResult *execute.ExecutionResult, opts config.Options, runID string, pending *persist.PendingDecision) error {
	if runID != "" {
		_ = persist.UpdateStatus(runID, persist.StatusValidating)
	}

	ui.PhaseHeader(4, "VALIDATION")
	parsed := plan.ParsePlan(debateResult.FinalPlan)
	ui.PrintSystem("Running checks and AI review...")
	valOpts := validate.Options{
		MaxFixAttempts:  opts.MaxFixAttempts,
		Tasks:           parsed.Tasks,
		Execution:       execResult,
		Interactive:     isInteractive(),
		PendingDecision: pending,
	}
	valResult, err := validate.Run(ctx, debateResult.FinalPlan, valOpts)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if valResult.Execution != nil {
		execResult = valResult.Execution
	}

	if valResult.Status == "WAITING_INPUT" {
		if runID != "" {
			_ = persist.SavePendingDecision(runID, valResult.PendingDecision)
			if data, err := json.MarshalIndent(execResult, "", "  "); err == nil {
				_ = persist.SaveExecutionState(runID, data)
			}
		}
		ui.PrintSystem("Validation is waiting for your decision. Resume this run to continue.")
		return nil
	}

	ui.PrintSuccess("Validation complete")

	if valResult.FixAttempts > 0 {
		ui.PrintVerbose("Fix loop ran %d attempt(s)", valResult.FixAttempts)
	}

	// Save validation artifacts and mark done.
	if runID != "" {
		_ = persist.ClearPendingDecision(runID)
		_ = persist.SaveValidationArtifact(runID, valResult.FinalReport)
		_ = persist.UpdateStatus(runID, persist.StatusDone)
		if data, err := json.MarshalIndent(execResult, "", "  "); err == nil {
			_ = persist.SaveExecutionState(runID, data)
		}
	}
	awardExecutionPoints(execResult, runID)
	if valResult.ResolvedFixAI != "" && valResult.Status == "COMPLETE" {
		_ = score.AddPoint(valResult.ResolvedFixAI, "fix")
		refreshAILabels()
	}

	// Final summary
	ui.Divider()
	ui.PrintSystem("Final Status: %s", valResult.Status)
	if runID != "" {
		ui.PrintSystem("Run saved to %s", persist.RunDir(runID))
	}
	ui.PrintSuccess("cloadex complete!")

	return nil
}

// resumeRun resumes an interrupted or incomplete run from its last completed phase.
func resumeRun(ctx context.Context, manifest *persist.RunManifest, opts config.Options, setRunID func(string)) error {
	ui.Banner()
	runID := manifest.ID
	setRunID(runID)
	ui.PrintSystem("Resuming run %s", runID)
	ui.PrintSystem("Prompt: %s", manifest.Prompt)

	if err := checkDependencies(); err != nil {
		return err
	}

	dir := persist.RunDir(runID)

	// Determine completed and pending phases based on status and artifacts.
	hasPlan := fileExists(dir, "plan.md")
	hasExec := fileExists(dir, "execution.md")
	hasVal := fileExists(dir, "validation.md")

	printPhaseStatus := func(name string, done bool) {
		if done {
			ui.PrintSuccess("  %s — completed", name)
		} else {
			ui.PrintSystem("  %s — pending", name)
		}
	}

	ui.Divider()
	ui.PrintSystem("Phase status:")
	printPhaseStatus("Debate & Plan", hasPlan)
	printPhaseStatus("Execution", hasExec)
	printPhaseStatus("Validation", hasVal)
	ui.Divider()

	// Determine where to resume. We replay from the earliest incomplete phase.
	// If the plan exists, we can skip debate entirely and reuse the saved plan.
	if !hasPlan {
		// Need to re-run the full pipeline from the start.
		ui.PrintSystem("No approved plan found — restarting from debate.")
		return run(ctx, manifest.Prompt, opts, setRunID)
	}

	// Load the saved plan to reuse.
	planText := readRunFile(dir, "plan.md")
	debateHistory := readRunFile(dir, "debate.md")
	debateResult := &debate.DebateResult{
		FinalPlan: planText,
		History:   debateHistory,
	}

	ui.PrintSuccess("Reusing approved plan from previous run.")

	if opts.DryRun {
		ui.Divider()
		ui.PrintSystem("Dry-run complete — skipping execution and validation.")
		return nil
	}

	// Resume from execution if needed.
	if !hasExec {
		ui.PrintSystem("Resuming from execution phase...")
		if _, err := runExecutePhase(ctx, debateResult, runID); err != nil {
			return err
		}
	} else {
		ui.PrintSuccess("Execution already completed — skipping.")
	}

	// Resume validation if needed.
	if !hasVal || manifest.PendingDecision != nil {
		ui.PrintSystem("Resuming from validation phase...")
		execResult, err := loadExecutionResult(runID)
		if err != nil {
			return fmt.Errorf("load execution state: %w", err)
		}
		return runValidatePhase(ctx, debateResult, execResult, opts, runID, manifest.PendingDecision)
	}

	ui.PrintSuccess("All phases already completed.")
	_ = persist.UpdateStatus(runID, persist.StatusDone)
	ui.PrintSuccess("cloadex complete!")
	return nil
}

func refreshAILabels() {
	ui.SetAILabel("claude", score.Label(runner.Claude))
	ui.SetAILabel("codex", score.Label(runner.Codex))
}

func loadAgentSessions(state *sessionstate.State) map[runner.AI]*runner.AgentSession {
	sessions := map[runner.AI]*runner.AgentSession{
		runner.Claude: runner.NewSession(runner.Claude, nil, runner.DefaultSessionOptions()),
		runner.Codex:  runner.NewSession(runner.Codex, nil, runner.DefaultSessionOptions()),
	}
	if state == nil {
		return sessions
	}
	if snapshot, ok := state.AgentSessions[string(runner.Claude)]; ok {
		sessions[runner.Claude] = runner.NewSession(runner.Claude, &snapshot, runner.DefaultSessionOptions())
	}
	if snapshot, ok := state.AgentSessions[string(runner.Codex)]; ok {
		sessions[runner.Codex] = runner.NewSession(runner.Codex, &snapshot, runner.DefaultSessionOptions())
	}
	return sessions
}

func registerAgentSessions(sessions map[runner.AI]*runner.AgentSession) {
	for _, ai := range []runner.AI{runner.Claude, runner.Codex} {
		if session := sessions[ai]; session != nil {
			runner.RegisterSession(session)
		}
	}
}

func unregisterAgentSessions() {
	runner.ClearRegisteredSessions()
}

func resumeRegisteredAgentSessions() {
	for _, ai := range []runner.AI{runner.Claude, runner.Codex} {
		if session := runner.RegisteredSession(ai); session != nil {
			session.Resume()
		}
	}
}

func saveAgentSessions(state *sessionstate.State, sessions map[runner.AI]*runner.AgentSession) {
	if state == nil {
		return
	}
	if state.AgentSessions == nil {
		state.AgentSessions = map[string]runner.SessionSnapshot{}
	}
	for _, ai := range []runner.AI{runner.Claude, runner.Codex} {
		if session := sessions[ai]; session != nil {
			state.AgentSessions[string(ai)] = session.Snapshot()
		}
	}
	_ = sessionstate.Save(state)
}

func loadExecutionResult(runID string) (*execute.ExecutionResult, error) {
	data, err := persist.LoadExecutionState(runID)
	if err != nil {
		return nil, err
	}
	var result execute.ExecutionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func awardExecutionPoints(execResult *execute.ExecutionResult, runID string) {
	if execResult == nil {
		return
	}
	changed := false
	for i := range execResult.Results {
		result := &execResult.Results[i]
		if result.Status != execute.TaskSuccess || result.ExecutionPointAwarded {
			continue
		}
		if err := score.AddPoint(result.ExecutorAI, "execution"); err == nil {
			result.ExecutionPointAwarded = true
			changed = true
		}
	}
	if changed {
		refreshAILabels()
		if runID != "" {
			if data, err := json.MarshalIndent(execResult, "", "  "); err == nil {
				_ = persist.SaveExecutionState(runID, data)
			}
		}
	}
}

func handleSessionCommand(command session.SlashCommand, args string, state *sessionstate.State, rc *runContext) error {
	switch command {
	case session.CmdMode:
		if strings.TrimSpace(args) == "" {
			ui.PrintSystem("Current mode: %s", state.Mode)
			return nil
		}
		mode, ok := sessionstate.ValidMode(args)
		if !ok {
			return fmt.Errorf("unknown mode %q", strings.TrimSpace(args))
		}
		state.Mode = mode
		if err := sessionstate.Save(state); err != nil {
			return err
		}
		ui.PrintSystem("Switched mode to %s", mode)
		return nil
	case session.CmdScore:
		refreshAILabels()
		ui.PrintSystem("%s", score.Label(runner.Claude))
		ui.PrintSystem("%s", score.Label(runner.Codex))
		return nil
	case session.CmdAgents:
		refreshAILabels()
		ui.PrintSystem("%s — primary agent, planning/review heavyweight", score.Label(runner.Claude))
		ui.PrintSystem("%s — primary agent, implementation/review heavyweight", score.Label(runner.Codex))
		ui.PrintSystem("Mode: %s", state.Mode)
		return nil
	case session.CmdDiff:
		return printGitDiffSummary()
	case session.CmdPlan:
		if strings.TrimSpace(args) == "" {
			if state.LastPlan == "" {
				ui.PrintSystem("No approved plan recorded yet.")
				return nil
			}
			ui.Divider()
			fmt.Println(state.LastPlan)
			return nil
		}
		return runCommandInSession(rc, state, args, sessionstate.ModePlanning)
	case session.CmdRun:
		if strings.TrimSpace(args) == "" {
			state.Mode = sessionstate.ModeExecution
			return sessionstate.Save(state)
		}
		return runCommandInSession(rc, state, args, sessionstate.ModeExecution)
	case session.CmdReview:
		if strings.TrimSpace(args) == "" {
			state.Mode = sessionstate.ModeChat
			return sessionstate.Save(state)
		}
		return runCommandInSession(rc, state, args, sessionstate.ModeChat)
	default:
		return fmt.Errorf("unknown command %s", command)
	}
}

func runCommandInSession(rc *runContext, state *sessionstate.State, prompt string, mode sessionstate.Mode) error {
	resumeRegisteredAgentSessions()
	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	if err := config.LoadFile(&opts, cliSet); err != nil {
		ui.PrintError("Config: %s", err)
	}
	ui.SetVerbose(opts.Verbose)

	ctx, setRunID := rc.begin()
	defer rc.end()

	prev := state.Mode
	state.Mode = mode
	defer func() {
		state.Mode = prev
		_ = sessionstate.Save(state)
	}()
	return runForSession(ctx, strings.TrimSpace(argsOrPrompt(prompt)), opts, setRunID, state)
}

func argsOrPrompt(input string) string {
	return strings.TrimSpace(input)
}

func sessionHeader(state *sessionstate.State) string {
	refreshAILabels()
	repo := ""
	if state != nil && state.RepoPath != "" {
		repo = filepath.Base(state.RepoPath)
	}
	branch := ""
	if state != nil {
		branch = state.Branch
	}
	return ui.SessionHeader(repo, branch, score.Label(runner.Claude), score.Label(runner.Codex))
}

func printGitDiffSummary() error {
	cmd := exec.Command("git", "status", "--short")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	ui.Divider()
	if len(out) == 0 {
		ui.PrintSystem("Working tree clean.")
		return nil
	}
	fmt.Print(string(out))
	return nil
}

func runReview(ctx context.Context, userPrompt string, state *sessionstate.State) error {
	refreshAILabels()
	if err := checkDependencies(); err != nil {
		return err
	}
	reviewPrompt := buildReviewPrompt(userPrompt, state)
	ui.PrintSystem("Running workspace review...")
	codexReview := runner.Run(ctx, runner.Codex, reviewPrompt, func(ai runner.AI, line string) {
		ui.StreamCodex(line)
	})
	if codexReview.Err != nil {
		return codexReview.Err
	}
	claudeReview := runner.Run(ctx, runner.Claude, prompt.FinalReview(prompt.WorkspaceContext(), state.LastPlan, codexReview.Output, ""), func(ai runner.AI, line string) {
		ui.StreamClaude(line)
	})
	if claudeReview.Err != nil {
		return claudeReview.Err
	}
	state.RecordTurn("review", truncateForMemory(claudeReview.Output))
	state.Pin("risk", truncateForMemory(codexReview.Output))
	state.RecordEvent("phase_complete", "review", "cloadex", "Review mode completed.")
	if checkpointErr := runObserverCheckpoint(ctx, state, "review", runner.Codex, truncateForMemory(codexReview.Output)); checkpointErr != nil {
		ui.PrintVerbose("Observer checkpoint (review): %s", checkpointErr)
	}
	return sessionstate.Save(state)
}

func buildReviewPrompt(userPrompt string, state *sessionstate.State) string {
	var sb strings.Builder
	sb.WriteString("Review the current workspace and recent changes.\n")
	if strings.TrimSpace(userPrompt) != "" {
		sb.WriteString("User focus:\n")
		sb.WriteString(userPrompt)
		sb.WriteString("\n\n")
	}
	if state != nil && strings.TrimSpace(state.SummaryForPrompt()) != "" {
		sb.WriteString("Session context:\n")
		sb.WriteString(state.SummaryForPrompt())
		sb.WriteString("\n\n")
	}
	sb.WriteString("Return a concise review with findings first, then a brief summary.")
	return sb.String()
}

func truncateForMemory(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 280 {
		return s
	}
	return s[:277] + "..."
}

func summarizeExecution(execResult *execute.ExecutionResult) string {
	if execResult == nil {
		return "No execution result recorded."
	}
	var success, failed, skipped int
	for _, result := range execResult.Results {
		switch result.Status {
		case execute.TaskSuccess:
			success++
		case execute.TaskFailed:
			failed++
		case execute.TaskSkipped:
			skipped++
		}
	}
	return fmt.Sprintf("Execution summary: success=%d failed=%d skipped=%d", success, failed, skipped)
}

func runObserverCheckpoint(ctx context.Context, state *sessionstate.State, stage string, activeAI runner.AI, detail string) error {
	if state == nil {
		return nil
	}
	observer := oppositeAI(activeAI)
	if observer == "" {
		return nil
	}
	checkpointPrompt := prompt.ObserverCheckpoint(prompt.WorkspaceContext(), stage, detail)
	result := runner.Run(ctx, observer, checkpointPrompt, nil)
	if result.Err != nil {
		return result.Err
	}
	verdict, reason, err := parseObserverVerdict(result.Output)
	if err != nil {
		return err
	}
	state.RecordEvent("observer_"+verdict, stage, string(observer), reason)
	switch verdict {
	case "warn":
		ui.PrintSystem("Observer notice from %s: %s", score.Label(observer), reason)
		state.Pin("risk", truncateForMemory(reason))
	case "interrupt":
		ui.PrintSystem("Observer intervention from %s: %s", score.Label(observer), reason)
		state.Pin("risk", truncateForMemory(reason))
		state.RecordTurn("observer", truncateForMemory(reason))
	}
	return nil
}

func oppositeAI(ai runner.AI) runner.AI {
	switch ai {
	case runner.Claude:
		return runner.Codex
	case runner.Codex:
		return runner.Claude
	default:
		return ""
	}
}

func parseObserverVerdict(output string) (string, string, error) {
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start == -1 || end == -1 || end < start {
		return "", "", fmt.Errorf("observer verdict missing JSON object")
	}
	var parsed struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(output[start:end+1]), &parsed); err != nil {
		return "", "", err
	}
	verdict := strings.ToLower(strings.TrimSpace(parsed.Verdict))
	switch verdict {
	case "continue", "warn", "interrupt":
	default:
		return "", "", fmt.Errorf("unknown observer verdict %q", parsed.Verdict)
	}
	return verdict, strings.TrimSpace(parsed.Reason), nil
}

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func readRunFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return string(data)
}

func checkDependencies() error {
	missing := []dependencyError{}

	if _, err := exec.LookPath("claude"); err != nil {
		missing = append(missing, dependencyError{
			name:    "claude",
			install: installClaude(),
			url:     "https://docs.anthropic.com/en/docs/claude-code",
		})
	}
	if _, err := exec.LookPath("codex"); err != nil {
		missing = append(missing, dependencyError{
			name:    "codex",
			install: "npm install -g @openai/codex",
			url:     "https://github.com/openai/codex",
		})
	}

	if len(missing) == 0 {
		return nil
	}

	ui.PrintError("Missing required CLI tool(s):")
	for _, d := range missing {
		fmt.Fprintf(os.Stderr, "\n  %s%s%s is not installed or not in PATH.\n", ui.Bold, d.name, ui.Reset)
		fmt.Fprintf(os.Stderr, "  Install:  %s\n", d.install)
		fmt.Fprintf(os.Stderr, "  Docs:     %s\n", d.url)
	}
	fmt.Fprintln(os.Stderr)

	return fmt.Errorf("install the missing tool(s) above and try again")
}

type dependencyError struct {
	name    string
	install string
	url     string
}

func installClaude() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install claude-code  (or)  npm install -g @anthropic-ai/claude-code"
	default:
		return "npm install -g @anthropic-ai/claude-code"
	}
}

// cmdRuns lists all persisted runs.
func cmdRuns() error {
	runs, err := persist.ListRuns()
	if err != nil {
		return fmt.Errorf("list runs: %w", err)
	}
	if len(runs) == 0 {
		fmt.Println("No runs found. Run cloadex with a prompt to get started.")
		return nil
	}

	fmt.Printf("%s%sRecent runs:%s\n\n", ui.Bold, ui.SystemColor, ui.Reset)
	for _, r := range runs {
		ts := r.Timestamp.Format("2006-01-02 15:04:05")
		artifacts := ""
		if r.HasPlan {
			artifacts += "P"
		} else {
			artifacts += "\u00b7"
		}
		if r.HasExec {
			artifacts += "E"
		} else {
			artifacts += "\u00b7"
		}
		if r.HasVal {
			artifacts += "V"
		} else {
			artifacts += "\u00b7"
		}
		fmt.Printf("  %s%s%s  [%s]  %s\n", ui.Dim, ts, ui.Reset, artifacts, r.Prompt)
	}
	fmt.Printf("\n%sArtifact key: P=plan E=execution V=validation%s\n", ui.Dim, ui.Reset)
	fmt.Printf("%sView details: cloadex show <YYYYMMDD-HHMMSS>%s\n", ui.Dim, ui.Reset)
	return nil
}

// cmdShow displays the details of a specific run.
func cmdShow(id string) error {
	run, err := persist.LoadRun(id)
	if err != nil {
		return err
	}

	ts := run.Timestamp.Format("2006-01-02 15:04:05")
	fmt.Printf("%s%sRun: %s%s\n", ui.Bold, ui.SystemColor, ts, ui.Reset)
	fmt.Printf("%s%sDir: %s%s\n\n", ui.Dim, ui.SystemColor, run.Dir, ui.Reset)

	fmt.Printf("%s%sPrompt:%s\n%s\n\n", ui.Bold, ui.UserColor, ui.Reset, run.FullPrompt)

	if run.Plan != "" {
		ui.Divider()
		fmt.Printf("%s%sPlan:%s\n%s\n\n", ui.Bold, ui.SystemColor, ui.Reset, run.Plan)
	}
	if run.Execution != "" {
		ui.Divider()
		fmt.Printf("%s%sExecution:%s\n%s\n\n", ui.Bold, ui.ClaudeColor, ui.Reset, run.Execution)
	}
	if run.Validation != "" {
		ui.Divider()
		fmt.Printf("%s%sValidation:%s\n%s\n", ui.Bold, ui.CodexColor, ui.Reset, run.Validation)
	}
	return nil
}

// doResume loads the latest incomplete run and resumes it. This is the shared
// logic used by both the top-level `cloadex resume` command and the session /resume.
func doResume(ctx context.Context, opts config.Options, setRunID func(string)) error {
	manifest, err := persist.LatestIncompleteRun()
	if err != nil {
		return fmt.Errorf("find incomplete run: %w", err)
	}
	if manifest == nil {
		ui.PrintSystem("No incomplete runs found. Nothing to resume.")
		return nil
	}
	return resumeRun(ctx, manifest, opts, setRunID)
}

// cmdResume finds the latest incomplete run and resumes it.
func cmdResume() error {
	// Check for --help
	for _, arg := range os.Args[2:] {
		if arg == "--help" || arg == "-h" {
			printResumeUsage()
			return nil
		}
	}

	opts := config.DefaultOptions()
	cliSet := make(map[string]bool)
	remaining := parseFlags(os.Args[2:], &opts, cliSet)
	_ = remaining

	if err := config.LoadFile(&opts, cliSet); err != nil {
		ui.PrintError("Config: %s", err)
	}
	ui.SetVerbose(opts.Verbose)

	rc := &runContext{}
	cleanup := startSignalHandler(rc, nil)
	defer cleanup()

	ctx, setRunID := rc.begin()
	defer rc.end()

	return doResume(ctx, opts, setRunID)
}

// cmdInit creates a default config file.
func cmdInit() error {
	if err := config.InitConfig(); err != nil {
		return err
	}
	persist.EnsureGitignore()
	ui.PrintSuccess("Created .cloadex/config.yaml")
	fmt.Println("  Edit this file to set default options for this project.")
	return nil
}

func printUsage() {
	ui.Banner()
	fmt.Println("Usage: cloadex                    Start an interactive session (TTY only)")
	fmt.Println("       cloadex [options] <prompt>  Run a single prompt through the pipeline")
	fmt.Println("       cloadex <command>")
	fmt.Println()
	fmt.Println("When invoked with no arguments in an interactive terminal, cloadex starts a")
	fmt.Println("live session with repo-local memory, active modes, slash commands, and AI")
	fmt.Println("score labels. Type /help inside a session for available commands, or /exit")
	fmt.Println("to quit.")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  session         Start an interactive session (same as bare 'cloadex')")
	fmt.Println("  init            Create a .cloadex/config.yaml with defaults")
	fmt.Println("  runs            List previous runs")
	fmt.Println("  show [id]       Show details of a run (default: latest)")
	fmt.Println("  resume          Resume the most recent interrupted run")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --rounds N      Maximum debate rounds (default: 5)")
	fmt.Println("  --max-fixes N   Maximum fix-loop attempts when checks fail (default: 2)")
	fmt.Println("  --no-fix        Disable fix loop (check and report only)")
	fmt.Println("  --dry-run       Show the plan without executing it")
	fmt.Println("  --yes, -y       Auto-approve plans (non-interactive mode)")
	fmt.Println("  --verbose       Show detailed debug output")
	fmt.Println("  --version       Show version")
	fmt.Println("  --help          Show this help")
	fmt.Println()
	fmt.Println("Config:")
	fmt.Println("  Options can be persisted in .cloadex/config.yaml (run 'cloadex init').")
	fmt.Println("  CLI flags always override config file values.")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  cloadex                                                    # interactive session")
	fmt.Println("  cloadex \"build a login page with email and password\"")
	fmt.Println("  cloadex --rounds 3 \"add a dark mode toggle to the navbar\"")
	fmt.Println("  cloadex --dry-run \"refactor the API routes to use middleware\"")
	fmt.Println("  cloadex --yes \"add input validation to the signup form\"")
	fmt.Println("  cloadex runs")
	fmt.Println("  cloadex show 20240115-143022")
	fmt.Println("  cloadex resume")
}

func printResumeUsage() {
	fmt.Println("Usage: cloadex resume [options]")
	fmt.Println()
	fmt.Println("Resume the most recent interrupted or incomplete run.")
	fmt.Println("Skips phases that already completed (debate, execution) and")
	fmt.Println("restarts from the earliest incomplete phase.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --max-fixes N   Maximum fix-loop attempts (default: 2)")
	fmt.Println("  --no-fix        Disable fix loop")
	fmt.Println("  --dry-run       Show the plan without executing")
	fmt.Println("  --yes, -y       Auto-approve plans")
	fmt.Println("  --verbose       Show detailed debug output")
	fmt.Println("  --help          Show this help")
}
