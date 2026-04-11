package validate

import (
	"context"
	"strings"
	"testing"

	"github.com/cloadex-cli/cloadex/internal/plan"
	"github.com/cloadex-cli/cloadex/internal/runner"
)

func TestRunCheck_PassingCommand(t *testing.T) {
	r := runCheck(context.Background(), "echo", "echo hello")
	if !r.Passed {
		t.Errorf("expected pass, got fail: %s", r.Output)
	}
	if !strings.Contains(r.Output, "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", r.Output)
	}
	if r.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestRunCheck_FailingCommand(t *testing.T) {
	r := runCheck(context.Background(), "false", "false")
	if r.Passed {
		t.Error("expected fail, got pass")
	}
}

func TestRunCheck_NonexistentCommand(t *testing.T) {
	r := runCheck(context.Background(), "bad", "nonexistent_command_xyz_123")
	if r.Passed {
		t.Error("expected fail for nonexistent command")
	}
}

func TestRunCheck_CapturesStderr(t *testing.T) {
	r := runCheck(context.Background(), "stderr", "echo error >&2")
	// The command itself succeeds (exit 0)
	if !r.Passed {
		t.Error("expected pass")
	}
	if !strings.Contains(r.Output, "error") {
		t.Errorf("expected stderr captured, got: %s", r.Output)
	}
}

func TestRunCheck_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	r := runCheck(ctx, "sleep", "sleep 60")
	if r.Passed {
		t.Error("expected fail due to cancelled context")
	}
}

func TestCheckSuite_Summary(t *testing.T) {
	suite := &CheckSuite{
		Passed: false,
		Results: []CheckResult{
			{Name: "go vet", Command: "go vet ./...", Passed: true, Output: ""},
			{Name: "go test", Command: "go test ./...", Passed: false, Output: "FAIL: TestFoo"},
		},
	}

	summary := suite.Summary()
	if !strings.Contains(summary, "[PASS] go vet") {
		t.Error("expected PASS for go vet in summary")
	}
	if !strings.Contains(summary, "[FAIL] go test") {
		t.Error("expected FAIL for go test in summary")
	}
	if !strings.Contains(summary, "FAIL: TestFoo") {
		t.Error("expected failure output in summary")
	}
}

func TestCheckSuite_FailedOutput(t *testing.T) {
	suite := &CheckSuite{
		Results: []CheckResult{
			{Name: "go vet", Command: "go vet ./...", Passed: true, Output: "ok"},
			{Name: "go test", Command: "go test ./...", Passed: false, Output: "FAIL: TestFoo\nexit status 1"},
		},
	}

	failed := suite.FailedOutput()
	if strings.Contains(failed, "go vet") {
		t.Error("FailedOutput should not include passing checks")
	}
	if !strings.Contains(failed, "FAIL: TestFoo") {
		t.Error("FailedOutput should include failing check output")
	}
	if !strings.Contains(failed, "go test ./...") {
		t.Error("FailedOutput should include the command that failed")
	}
}

func TestCheckSuite_EmptyPassesByDefault(t *testing.T) {
	suite := &CheckSuite{Passed: true}
	summary := suite.Summary()
	if summary != "" {
		t.Errorf("expected empty summary for empty suite, got: %s", summary)
	}
}

func TestRunChecks_CustomCommands(t *testing.T) {
	origApprove := approveVerificationCommand
	defer func() { approveVerificationCommand = origApprove }()
	approveVerificationCommand = func(_, _ string) bool { return true }

	suite := RunChecks(context.Background(), nil, []string{"echo custom_ok", "false"}, true)

	if suite.Passed {
		t.Error("suite should fail because 'false' fails")
	}
	if len(suite.Results) < 1 {
		t.Fatal("expected at least 1 result from custom commands")
	}

	// Find the custom commands in results
	var foundPass, foundFail bool
	for _, r := range suite.Results {
		if r.Command == "echo custom_ok" && r.Passed {
			foundPass = true
		}
		if r.Command == "false" && !r.Passed {
			foundFail = true
		}
	}
	if !foundPass {
		t.Error("expected passing result for 'echo custom_ok'")
	}
	if !foundFail {
		t.Error("expected failing result for 'false'")
	}
}

func TestRunChecks_TaskVerificationCommands(t *testing.T) {
	origApprove := approveVerificationCommand
	defer func() { approveVerificationCommand = origApprove }()
	approveVerificationCommand = func(_, _ string) bool { return true }

	tasks := []plan.Task{
		{OwnerAI: runner.Claude, Description: "task 1", Verification: "echo task1_ok"},
		{OwnerAI: runner.Codex, Description: "task 2"}, // No verification
		{OwnerAI: runner.Claude, Description: "task 3", Verification: "echo task3_ok"},
	}

	suite := RunChecks(context.Background(), tasks, nil, true)

	// Should have results from auto-detect + task verifications
	var found int
	for _, r := range suite.Results {
		if r.Command == "echo task1_ok" || r.Command == "echo task3_ok" {
			found++
			if !r.Passed {
				t.Errorf("expected task verification to pass: %s", r.Command)
			}
		}
	}
	if found != 2 {
		t.Errorf("expected 2 task verification results, found %d", found)
	}
}

func TestRunChecks_Deduplication(t *testing.T) {
	origApprove := approveVerificationCommand
	defer func() { approveVerificationCommand = origApprove }()
	approveVerificationCommand = func(_, _ string) bool { return true }

	tasks := []plan.Task{
		{OwnerAI: runner.Claude, Verification: "echo dup"},
		{OwnerAI: runner.Claude, Verification: "echo dup"},
	}

	suite := RunChecks(context.Background(), tasks, []string{"echo dup"}, true)

	count := 0
	for _, r := range suite.Results {
		if r.Command == "echo dup" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected deduplication to produce 1 result for 'echo dup', got %d", count)
	}
}

func TestRunChecks_SkipsUntrustedCommandsWhenNonInteractive(t *testing.T) {
	origApprove := approveVerificationCommand
	defer func() { approveVerificationCommand = origApprove }()
	approveVerificationCommand = func(_, _ string) bool {
		t.Fatal("approval prompt should not run in non-interactive mode")
		return false
	}

	suite := RunChecks(context.Background(), nil, []string{"echo should_not_run"}, false)
	if !suite.Passed {
		t.Fatal("suite should remain passing when untrusted commands are skipped")
	}
	if len(suite.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(suite.Results))
	}
	if !suite.Results[0].Skipped {
		t.Fatal("expected command to be skipped")
	}
	if !strings.Contains(strings.ToLower(suite.Results[0].Output), "skipped") {
		t.Fatalf("expected skip explanation, got %q", suite.Results[0].Output)
	}
}

func TestRunChecks_ApprovesUntrustedCommandsInteractively(t *testing.T) {
	origApprove := approveVerificationCommand
	defer func() { approveVerificationCommand = origApprove }()

	calls := 0
	approveVerificationCommand = func(_, command string) bool {
		calls++
		return command == "echo approved"
	}

	suite := RunChecks(context.Background(), []plan.Task{
		{OwnerAI: runner.Claude, Verification: "echo approved"},
		{OwnerAI: runner.Codex, Verification: "echo denied"},
	}, nil, true)

	if calls != 2 {
		t.Fatalf("expected 2 approval prompts, got %d", calls)
	}
	var approved, denied bool
	for _, result := range suite.Results {
		switch result.Command {
		case "echo approved":
			approved = true
			if result.Skipped || !result.Passed {
				t.Fatalf("approved command should run successfully: %#v", result)
			}
		case "echo denied":
			denied = true
			if !result.Skipped {
				t.Fatalf("denied command should be skipped: %#v", result)
			}
		}
	}
	if !approved || !denied {
		t.Fatalf("missing expected results: approved=%v denied=%v", approved, denied)
	}
}
