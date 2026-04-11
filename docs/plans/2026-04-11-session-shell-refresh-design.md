# cloadex Session Shell Refresh

## Goal

Make the interactive CLI feel closer to Codex CLI without replacing the current
line-based REPL. The redesign should make the input area clearer, reduce
terminal noise, and present session state as a compact status shell instead of a
log-heavy interface.

## Chosen Approach

Keep the existing `internal/session` scanner loop and redesign the surrounding
presentation layer.

This avoids a full-screen TUI rewrite while still improving the three user pain
points:

- highlighted and obvious text input
- cleaner, less noisy layout
- clearer separation between status/meta output and transcript output

## UI Changes

### Compact Session Header

Replace the current multi-line plain-text session header with a compact ANSI
header that shows:

- product name
- repo basename
- git branch
- current mode
- Claude and Codex score labels

The header should feel like a status bar, not a splash screen. Repo summary text
is removed from the header to reduce noise.

### Highlighted Prompt

Replace the plain `cloadex[mode]>` prompt with a stronger prompt treatment that
uses:

- a mode chip
- a highlighted prompt marker
- consistent spacing that makes the input line visually separate from output

`/exit` remains the documented exit command. Bare `exit` and `quit` continue to
work as safety fallbacks so the CLI does not accidentally treat them as prompts.

### Softer Transcript Framing

Reduce bracket-heavy meta output and heavy section chrome by:

- toning down system/status prefixes
- making divider and phase markers lighter
- keeping Claude/Codex output distinct, but less visually crowded

The result should read more like an interactive terminal product and less like a
prefixed log stream.

## Scope

Implementation should stay mostly inside:

- `internal/ui/ui.go`
- `internal/session/session.go`
- `cmd/root.go`

Tests should cover prompt rendering and the session exit behavior.

## Non-Goals

- No full-screen TUI dependency
- No alternate input widget
- No transcript buffering or pane layout changes
- No command surface redesign
