package sessionstate

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Ahmedlag/cloadex/internal/workspace"
)

func defaultRepoSummary(cwd string) string {
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return ""
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".cloadex" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 12 {
		names = names[:12]
	}
	if len(names) == 0 {
		return ""
	}
	return "Top-level files:\n- " + strings.Join(names, "\n- ")
}

func detectBranch(cwd string) string {
	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func SessionFilePath() string {
	path, err := workspace.Path(fileName)
	if err == nil {
		return path
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, workspace.DirName, fileName)
}
