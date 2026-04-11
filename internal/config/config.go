package config

// Options holds CLI flags that affect runtime behavior.
type Options struct {
	// MaxRounds is the maximum number of debate rounds.
	MaxRounds int

	// MaxFixAttempts is the maximum number of fix-loop iterations during validation.
	// When deterministic checks fail, the AI will attempt to fix the code up to this
	// many times before giving up. 0 means no fix loop (check and report only).
	MaxFixAttempts int

	// DryRun shows the plan but skips execution and validation.
	DryRun bool

	// Yes auto-approves plans without prompting (non-interactive mode).
	Yes bool

	// Verbose enables detailed logging of AI interactions and internal state.
	Verbose bool
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		MaxRounds:      5,
		MaxFixAttempts: 2,
	}
}
