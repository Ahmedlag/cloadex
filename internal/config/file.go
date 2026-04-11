package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Ahmedlag/cloadex/internal/workspace"
)

const configPath = ".cloadex/config.yaml"

// LoadFile reads .cloadex/config.yaml and merges values into the given Options.
// CLI flags always take precedence over file values.
// Only fields not already set by CLI flags are populated from the file.
func LoadFile(opts *Options, cliSet map[string]bool) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No config file is fine
		}
		return fmt.Errorf("read config: %w", err)
	}

	parsed := parseSimpleYAML(string(data))

	if v, ok := parsed["rounds"]; ok && !cliSet["rounds"] {
		if n, err := strconv.Atoi(v); err == nil {
			opts.MaxRounds = n
		}
	}
	if v, ok := parsed["max-fixes"]; ok && !cliSet["max-fixes"] {
		if n, err := strconv.Atoi(v); err == nil {
			opts.MaxFixAttempts = n
		}
	}
	if v, ok := parsed["verbose"]; ok && !cliSet["verbose"] {
		opts.Verbose = v == "true"
	}
	if v, ok := parsed["yes"]; ok && !cliSet["yes"] {
		opts.Yes = v == "true"
	}

	return nil
}

// InitConfig creates a default .cloadex/config.yaml if it doesn't exist.
func InitConfig() error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config file already exists: %s", configPath)
	}

	if err := workspace.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}

	content := `# cloadex configuration
# CLI flags override these values.

# Maximum debate rounds (default: 5)
# rounds: 5

# Maximum fix-loop attempts (default: 2)
# max-fixes: 2

# Auto-approve plans without prompting (default: false)
# yes: false

# Show detailed debug output (default: false)
# verbose: false
`
	return workspace.WritePrivateFile(path, []byte(content))
}

// parseSimpleYAML handles flat key: value YAML without a full parser dependency.
func parseSimpleYAML(s string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key != "" && val != "" {
			result[key] = val
		}
	}
	return result
}

func configFilePath() (string, error) {
	return workspace.Path("config.yaml")
}
