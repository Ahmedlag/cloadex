package config

import "testing"

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.MaxRounds != 5 {
		t.Errorf("MaxRounds = %d, want 5", opts.MaxRounds)
	}
	if opts.MaxFixAttempts != 2 {
		t.Errorf("MaxFixAttempts = %d, want 2", opts.MaxFixAttempts)
	}
	if opts.DryRun {
		t.Error("DryRun should default to false")
	}
	if opts.Yes {
		t.Error("Yes should default to false")
	}
	if opts.Verbose {
		t.Error("Verbose should default to false")
	}
}
