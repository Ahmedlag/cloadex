package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	DirName       = ".cloadex"
	LegacyDirName = ".wizdo"
)

// Dir returns the repo-local metadata directory, migrating the legacy
// .wizdo directory to .cloadex when needed.
func Dir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working dir: %w", err)
	}
	return dirFrom(cwd)
}

// Path returns a path rooted under the repo-local metadata directory.
func Path(elem ...string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	parts := append([]string{dir}, elem...)
	return filepath.Join(parts...), nil
}

func dirFrom(cwd string) (string, error) {
	current := filepath.Join(cwd, DirName)
	if _, err := os.Stat(current); err == nil {
		if err := os.Chmod(current, 0o700); err != nil {
			return "", fmt.Errorf("secure workspace dir: %w", err)
		}
		return current, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat workspace dir: %w", err)
	}

	legacy := filepath.Join(cwd, LegacyDirName)
	if _, err := os.Stat(legacy); err == nil {
		if err := os.Rename(legacy, current); err != nil {
			return "", fmt.Errorf("migrate legacy workspace dir: %w", err)
		}
		if err := os.Chmod(current, 0o700); err != nil {
			return "", fmt.Errorf("secure migrated workspace dir: %w", err)
		}
		return current, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat legacy workspace dir: %w", err)
	}

	return current, nil
}

func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func WritePrivateFile(path string, data []byte) error {
	if err := EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
