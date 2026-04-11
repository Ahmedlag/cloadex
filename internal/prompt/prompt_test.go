package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListTopFiles(t *testing.T) {
	// Create a temp directory structure
	dir := t.TempDir()

	// Create files and dirs
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0644)
	os.Mkdir(filepath.Join(dir, "cmd"), 0755)
	os.WriteFile(filepath.Join(dir, "cmd", "root.go"), []byte("package cmd"), 0644)
	os.WriteFile(filepath.Join(dir, "cmd", ".secret"), []byte("secret"), 0644)
	os.Mkdir(filepath.Join(dir, "node_modules"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg.js"), []byte(""), 0644)
	os.Mkdir(filepath.Join(dir, "vendor"), 0755)
	os.WriteFile(filepath.Join(dir, "vendor", "lib.go"), []byte(""), 0644)

	files := listTopFiles(dir, 2)

	// Should include top-level visible files and dirs
	has := func(name string) bool {
		for _, f := range files {
			if f == name {
				return true
			}
		}
		return false
	}

	if !has("main.go") {
		t.Error("expected main.go")
	}
	if !has("go.mod") {
		t.Error("expected go.mod")
	}
	if !has("cmd") {
		t.Error("expected cmd/")
	}
	if !has(filepath.Join("cmd", "root.go")) {
		t.Error("expected cmd/root.go")
	}

	// Should exclude hidden files, node_modules, vendor
	if has(".hidden") {
		t.Error("should exclude .hidden")
	}
	if has("node_modules") {
		t.Error("should exclude node_modules")
	}
	if has("vendor") {
		t.Error("should exclude vendor")
	}
	// Should exclude hidden files inside subdirs
	if has(filepath.Join("cmd", ".secret")) {
		t.Error("should exclude cmd/.secret")
	}
}

func TestListTopFiles_DepthZero(t *testing.T) {
	files := listTopFiles(t.TempDir(), 0)
	if len(files) != 0 {
		t.Errorf("depth 0 should return empty, got %v", files)
	}
}

func TestListTopFiles_MaxFiles(t *testing.T) {
	dir := t.TempDir()
	// Create 40 files
	for i := 0; i < 40; i++ {
		os.WriteFile(filepath.Join(dir, strings.ReplaceAll("file_NN.go", "NN", strings.Repeat("a", i+1))), []byte(""), 0644)
	}
	files := listTopFiles(dir, 1)
	if len(files) > 30 {
		t.Errorf("should cap at 30, got %d", len(files))
	}
}

func TestDebateSystem(t *testing.T) {
	result := DebateSystem("build a login page", "Working directory: /app\nGit branch: main\n")

	if !strings.Contains(result, "build a login page") {
		t.Error("should contain user prompt")
	}
	if !strings.Contains(result, "Working directory: /app") {
		t.Error("should contain workspace context")
	}
	if !strings.Contains(result, "CONVERGED:") {
		t.Error("should contain convergence instruction")
	}
}

func TestDebateRound(t *testing.T) {
	result := DebateRound("round 1 history", "codex", "I propose we use React")

	if !strings.Contains(result, "round 1 history") {
		t.Error("should contain history")
	}
	if !strings.Contains(result, "codex") {
		t.Error("should contain other AI name")
	}
	if !strings.Contains(result, "I propose we use React") {
		t.Error("should contain other AI's response")
	}
	if !strings.Contains(result, "CONVERGED:") {
		t.Error("should mention convergence")
	}
}

func TestPlanSummary(t *testing.T) {
	result := PlanSummary("debate content here")

	if !strings.Contains(result, "debate content here") {
		t.Error("should contain debate history")
	}
	if !strings.Contains(result, "owner_ai") {
		t.Error("should describe JSON schema")
	}
	if !strings.Contains(result, "depends_on") {
		t.Error("should describe dependency field")
	}
}

func TestExecuteTask(t *testing.T) {
	result := ExecuteTask(
		"implement JWT auth",
		"Backend/Logic Developer (Claude)",
		"Working directory: /app",
		"Full plan text here",
	)

	if !strings.Contains(result, "implement JWT auth") {
		t.Error("should contain task description")
	}
	if !strings.Contains(result, "Backend/Logic Developer (Claude)") {
		t.Error("should contain role")
	}
	if !strings.Contains(result, "Working directory: /app") {
		t.Error("should contain workspace context")
	}
	if !strings.Contains(result, "Full plan text here") {
		t.Error("should contain plan")
	}
}

func TestValidateImplementation(t *testing.T) {
	result := ValidateImplementation("ctx", "plan")
	if !strings.Contains(result, "ctx") || !strings.Contains(result, "plan") {
		t.Error("should embed both context and plan")
	}
	if !strings.Contains(result, "All clear") {
		t.Error("should mention the expected output format")
	}
}

func TestFinalReview(t *testing.T) {
	result := FinalReview("ctx", "plan", "validation output", "")
	if !strings.Contains(result, "validation output") {
		t.Error("should embed validation result")
	}
	if !strings.Contains(result, "COMPLETE") {
		t.Error("should mention expected status values")
	}
}

func TestFinalReview_WithCheckSummary(t *testing.T) {
	result := FinalReview("ctx", "plan", "validation", "[PASS] go vet\n[FAIL] go test")
	if !strings.Contains(result, "[PASS] go vet") {
		t.Error("should embed check summary")
	}
	if !strings.Contains(result, "[FAIL] go test") {
		t.Error("should embed check failures")
	}
}

func TestFixFailures(t *testing.T) {
	result := FixFailures("ctx", "plan", "FAIL: TestAuth", 1, 3)
	if !strings.Contains(result, "FAIL: TestAuth") {
		t.Error("should embed failed output")
	}
	if !strings.Contains(result, "1") {
		t.Error("should embed attempt number")
	}
}

func TestWorkspaceContext_ContainsWorkingDir(t *testing.T) {
	result := WorkspaceContext()
	if !strings.Contains(result, "Working directory:") {
		t.Error("should contain working directory")
	}
}

func TestListTopFiles_NonexistentDir(t *testing.T) {
	files := listTopFiles("/nonexistent/path/xyz", 2)
	if len(files) != 0 {
		t.Errorf("expected empty for nonexistent dir, got %v", files)
	}
}
