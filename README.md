# cloadex

Claude + Codex — better together.

cloadex is a CLI that orchestrates [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [OpenAI Codex](https://github.com/openai/codex) into a single collaborative coding workflow. Instead of choosing one AI, cloadex makes them **debate**, **plan**, **execute**, and **validate** together inside a persistent terminal session.

## How it works

```
 Debate ──> Plan Review ──> Execution ──> Validation
                                              │
                                         (fix loop)
```

1. **Debate** — Claude and Codex alternate rounds discussing the best approach to your prompt. They challenge each other's ideas until they converge on a plan (or a configurable round limit is reached).

2. **Plan Review** — The converged plan is presented for your approval. You can approve it as-is, edit it, or reject it to restart the debate.

3. **Execution** — Tasks from the approved plan run in parallel, routed to the AI that owns each task. Both AIs work in your actual repository with full file access.

4. **Validation** — Deterministic checks run first (see below), then Codex reviews the implementation, and Claude performs a final review. If checks fail, a fix loop re-runs the failing tasks automatically (up to `--max-fixes` attempts). The final result is `COMPLETE`, `NEEDS_FIXES`, or `INCOMPLETE`.

Every run is persisted under `.cloadex/runs/<timestamp>/` with the original prompt, debate history, plan, execution output, and validation report.

### Deterministic checks

Before the AI review, cloadex auto-detects your project type and runs the appropriate verification commands:

| Project type | Checks |
|---|---|
| Go (`go.mod`) | `go vet ./...`, `go build ./...`, `go test ./...` (if test files exist) |
| Node.js (`package.json`) | `npx tsc --noEmit` (if installed), `npx eslint .` (if installed) |
| Python (`pyproject.toml` / `setup.py`) | `pytest` (if installed), `mypy .` (if installed) |
| Rust (`Cargo.toml`) | `cargo check`, `cargo test` |

Tasks in the plan can also include per-task verification commands. Failures are fed back into the fix loop.

## Requirements

- **Go 1.22+** (to build from source)
- **Claude Code CLI** — `brew install claude-code` or `npm install -g @anthropic-ai/claude-code`
- **OpenAI Codex CLI** — `npm install -g @openai/codex`

Both CLIs must be authenticated and available in your `PATH`. cloadex checks for them at startup and provides install instructions if either is missing.

## Install

### go install (recommended)

```sh
go install github.com/cloadex-cli/cloadex@latest
```

Make sure `$GOBIN` (or `$GOPATH/bin`) is on your `PATH`:

```sh
# Add to your shell profile (~/.zshrc, ~/.bashrc, etc.)
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Homebrew (macOS / Linux)

```sh
brew tap cloadex-cli/cloadex
brew install cloadex
```

> The Homebrew formula is available after the first tagged release.

### Windows

Download the latest `cloadex-windows-amd64.zip` or `cloadex-windows-arm64.zip` from GitHub Releases, extract `cloadex.exe`, and add it to your `PATH`.

PowerShell example:

```powershell
$dest = "$HOME\bin"
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Expand-Archive .\cloadex-windows-amd64.zip -DestinationPath $dest -Force
$env:Path += ";$dest"
```

### From source

```sh
git clone https://github.com/cloadex-cli/cloadex.git
cd cloadex
make build      # produces ./cloadex
make install    # installs to $GOPATH/bin (ensure it's on your PATH)
```

## Usage

Run `cloadex` in a terminal to start an interactive session. The default experience is a persistent CLI session with repo-local memory, modes, slash commands, and AI scores.

```sh
cloadex                         # start an interactive session
cloadex [options] <prompt>      # run a single prompt
cloadex <command>
```

When invoked with no arguments in an interactive terminal, cloadex launches a live session screen showing the active repo, branch, mode, and both AI score labels. The prompt reflects the active mode, for example `cloadex[chat]>`. Type `/help` for available slash commands or `/exit` to quit.

### Session Modes

| Mode | Purpose |
|---|---|
| `chat` | Default continuous session mode |
| `plan` | Plan-only mode; entering a prompt creates and reviews a plan without execution |
| `run` | Full execution mode |
| `review` | Review the current workspace and recent changes |

### Slash Commands

| Command | Description |
|---|---|
| `/mode <chat\|plan\|run\|review>` | Switch session mode |
| `/plan [prompt]` | Show the last approved plan or run a prompt in plan mode |
| `/run [prompt]` | Switch to run mode or execute a prompt immediately |
| `/review [prompt]` | Switch to review mode or review immediately |
| `/score` | Show the global AI score labels |
| `/agents` | Show AI labels and roles |
| `/diff` | Show git status / diff summary |
| `/resume` | Resume the most recent interrupted run |
| `/help` | Show session help |
| `/exit` | Exit the session |

### Options

| Flag | Default | Description |
|---|---|---|
| `--rounds N` | `5` | Maximum debate rounds |
| `--max-fixes N` | `2` | Maximum fix-loop attempts when checks fail |
| `--no-fix` | | Disable the fix loop (check and report only) |
| `--dry-run` | | Show the plan without executing it |
| `--yes`, `-y` | | Auto-approve plans (non-interactive mode) |
| `--verbose` | | Show detailed debug output |
| `--version`, `-v` | | Show version |
| `--help`, `-h` | | Show help |

### Commands

| Command | Description |
|---|---|
| `cloadex session` | Start an interactive session (same as bare `cloadex`) |
| `cloadex init` | Create `.cloadex/config.yaml` with defaults |
| `cloadex runs` | List previous runs with artifact indicators |
| `cloadex show [id]` | Show details of a specific run (default: latest) |

### Configuration

Run `cloadex init` to create a `.cloadex/config.yaml` file:

```yaml
# cloadex configuration
# CLI flags override these values.

# rounds: 5
# max-fixes: 2
# yes: false
# verbose: false
```

CLI flags always take precedence over config file values.

### Examples

```sh
# Start an interactive session — enter prompts one at a time
cloadex

# Single prompt — review and approve the plan before execution
cloadex "add rate limiting middleware to the API"

# Faster convergence with fewer debate rounds
cloadex --rounds 3 "add a dark mode toggle to the navbar"

# Preview what the AIs would do without changing any files
cloadex --dry-run "refactor the database layer to use connection pooling"

# CI/scripting — skip the interactive approval step
cloadex --yes "add input validation to the signup form"

# Disable the fix loop — just report validation results
cloadex --no-fix "migrate user table to UUID primary keys"

# Review past runs
cloadex runs
cloadex show 20260410-143022
```

## Sharing Cloadex

`cloadex` is a local CLI. Each user runs it on their own machine inside their own repository. To share it with friends:

1. Publish a tagged GitHub release such as `v0.1.0`
2. Attach the prebuilt artifacts from `dist/`
3. Tell users to install both external AI CLIs:
   - Claude Code CLI
   - OpenAI Codex CLI
4. Tell users to authenticate both CLIs locally
5. Tell users to install `cloadex` from Homebrew, `go install`, or a release archive

Every user still needs their own Claude/Codex access because `cloadex` shells out to those local tools.

## Release Process

### Local release build

```sh
make release
make checksums
```

This produces release archives in `dist/` for:

- macOS arm64
- macOS amd64
- Linux amd64
- Linux arm64
- Windows amd64
- Windows arm64

### GitHub Actions release

This repository includes [`.github/workflows/release.yml`](.github/workflows/release.yml). On a tag push like `v0.1.0`, it:

1. builds all supported macOS, Linux, and Windows artifacts
2. generates `checksums.txt`
3. publishes a GitHub Release with all archives attached

### Homebrew release

For Homebrew:

1. create a tag and push it
2. wait for the GitHub Release artifacts
3. run `make formula` after updating the release checksums locally
4. commit the updated [Formula/cloadex.rb](Formula/cloadex.rb) to your tap or formula repo

## Architecture

```
cmd/root.go               CLI entry point, flag parsing, orchestration loop
internal/
  session/session.go       Interactive session loop, slash commands, prompt rendering
  sessionstate/            Repo-local persistent CLI session state and pinned memory
  config/                  Runtime options, config file loading & merging
  debate/debate.go         Multi-round debate between Claude and Codex
  plan/plan.go             Plan parsing (JSON + markdown), presentation, approval
  execute/execute.go       Parallel task execution routed by AI owner
  validate/
    validate.go            AI review pipeline (Codex + Claude) with fix loop
    check.go               Deterministic checks: auto-detect repo type, run commands
  runner/runner.go         Provider adapter — invokes Claude/Codex CLIs, parses streaming JSON
  prompt/prompt.go         System prompts, workspace context, per-phase instructions
  ui/                      Terminal output, ANSI colors, spinners, streaming
  persist/persist.go       Run artifact storage & retrieval under .cloadex/
  score/                   Global analytics scoreboard under ~/.cloadex/
```

### Provider adapters

cloadex talks to Claude and Codex through their CLI tools, not their HTTP APIs. The runner package handles the differences:

- **Claude**: `claude -p "<prompt>" --output-format stream-json --verbose` — parses `{"type":"assistant",...}` events
- **Codex**: `codex exec "<prompt>" --json` — parses `{"type":"item.completed",...}` events

Both produce streaming JSON that is parsed line-by-line with a 1 MB buffer for large responses.

### Run artifacts

Each run saves its artifacts to `.cloadex/runs/<timestamp>/`:

```
.cloadex/runs/20260410-143022/
  prompt.txt        # Original user prompt
  debate.md         # Full debate history
  plan.md           # Approved plan
  execution.md      # Execution output per task
  validation.md     # Validation reports
```

The `.cloadex/` directory is automatically added to `.gitignore`.

## Development

```sh
make build       # Build the binary
make test        # Run all tests
make lint        # Run go vet
make clean       # Remove binary and dist/
make release     # Cross-compile for macOS/linux/windows
make checksums   # Build release + print SHA256 sums for release archives
```

Release archives are placed in `dist/`. Use `make checksums` to generate SHA256 values for both the Homebrew formula and the published release archives.

## License

MIT — see [LICENSE](LICENSE).
