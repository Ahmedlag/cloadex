# cloadex CLI Runtime Design

## Goal

Turn `cloadex` from a batch-style debate/execution wrapper into a persistent CLI runtime that feels closer to Codex CLI and Claude Code while preserving the product's dual-agent workflow.

## Interaction Model

- Default to a persistent `chat` session.
- Support slash-command control surfaces: `/plan`, `/run`, `/review`, `/mode`, `/score`, `/agents`, `/diff`, `/resume`, `/help`, `/exit`.
- Show a lightweight startup screen with repo path, branch, current mode, and AI scores.
- Keep one shared run context plus AI-local working memory.

## Session State

Store repo-local session state in `.cloadex/session.json`:

- active mode
- repo summary
- active goal
- last approved plan
- last run id
- rolling recent turns
- pinned memory items such as approved plans, risks, and user decisions

Store global analytics in `~/.cloadex/scoreboard.json`.

## Runtime Behavior

- `chat`: default continuous mode
- `plan`: plan-only execution
- `run`: full execution mode
- `review`: workspace review mode

Safe actions run autonomously. Destructive or materially branching actions still stop for user input.

The second AI remains passive by default and becomes active at planning, review, failures, and detected risk checkpoints.

## Incremental Rollout

1. Session shell and persistent state
2. Live runtime control surfaces
3. Shared session memory wired into execution prompts
4. Unified planning, execution, and review on the same runtime loop

## Risks

- A true long-lived tool loop still depends on how the external Claude and Codex CLIs expose continuity and tool usage.
- Repo-local session continuity is implemented before deeper AI-local state handoff.
- Review mode is useful now, but narrower than the eventual fully unified runtime.
