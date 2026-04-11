# cloadex Mode Policy and Input Design

## Goal

Introduce three explicit interactive modes with real behavioral guardrails and a
keyboard-first way to switch between them.

## Modes

### Chat

- read-only discussion
- repository inspection
- question answering
- no code or file modification

### Planning

- read-only planning and discovery
- ask product, architecture, and technical shaping questions
- produce or refine plans
- no code or file modification

### Execution

- full workflow
- can discuss, plan, and modify code

## Enforcement

Mode is policy, not just a label. The command layer must route prompts
differently based on mode:

- `Chat` never enters the execution pipeline
- `Planning` never executes tasks or writes code
- `Execution` is the only mode that can run the full debate/execute/validate
  pipeline

## Input

Keep the current CLI non-TUI, but replace the scanner-based interactive reader
with a minimal key-aware line editor for real terminals.

Required support:

- normal text entry
- backspace
- enter to submit
- `Shift-Tab` to cycle:
  `chat -> planning -> execution -> chat`

Fallback:

- `/mode <name>` remains available
- non-interactive/test readers continue to use the line-scanner path

## UI

- session header stays compact
- prompt shows all three modes, with the active one highlighted
- mode switching should be immediately visible in the prompt
- blocked actions in read-only modes should produce explicit guidance

## Scope

- `internal/session`
- `internal/sessionstate`
- `internal/ui`
- `internal/prompt`
- `cmd/root.go`
