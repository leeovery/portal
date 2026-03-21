# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build -o portal .                    # build binary
go test ./...                           # run all tests
go test ./cmd -run TestAttachCommand    # run single test
go test ./internal/tmux/...             # run package tests
go test -v ./cmd -run TestOpen          # verbose single test
```

No linter config exists — use standard Go conventions. No code generation.

Tests **must not** use `t.Parallel()` — the cmd package injects mocks via package-level mutable state (`bootstrapDeps`, `openDeps`, `attachDeps`, etc.) and cleans up with `t.Cleanup()`.

## Architecture

Portal is a CLI for interactive tmux session management, built with **Cobra** (CLI) + **Bubble Tea** (TUI) + **Lipgloss** (styling).

### Command flow

`main.go` → `cmd/root.go` → subcommands. Root's `PersistentPreRunE` handles tmux server bootstrap (auto-starts server if not running) and injects a shared `tmux.Client` + `serverStarted` flag into `cmd.Context()`.

**Key command:** `cmd/open.go` — the main entry point (`portal open` / `x`). When given a path argument, resolves and creates/attaches a session directly. With no args, launches the TUI picker.

### Resolution chain (cmd/open.go → internal/resolver/)

Path arguments go through: direct path detection → alias lookup → zoxide query → TUI fallback with filter text.

### Session connection (two modes)

- **Outside tmux (bare shell):** `AttachConnector` uses `syscall.Exec` to hand off the process to `tmux attach-session -A` (atomic, replaces the portal process)
- **Inside tmux:** `SwitchConnector` uses `tmux switch-client` (two-step: create detached session, then switch)

### Internal packages

| Package | Role |
|---------|------|
| `tmux` | Wraps tmux CLI via `Commander` interface (`RealCommander` → `os/exec`). Client methods: ListSessions, NewSession, HasSession, SwitchClient, EnsureServer, WaitForSessions |
| `session` | Session creation pipeline: git root resolution → project persistence → name generation (`{project}-{nanoid}`) → tmux session creation. `QuickStart` for atomic create-or-attach |
| `resolver` | Path resolution chain with interface-based DI (AliasLookup, ZoxideQuerier, DirValidator) |
| `tui` | Bubble Tea model with page state machine: Loading → Sessions → Projects → FileBrowser |
| `project` | JSON-backed store (`~/.config/portal/projects.json`) with atomic writes |
| `alias` | Flat key=value file store (`~/.config/portal/aliases`) |
| `browser` | Directory listing with symlink detection |
| `fuzzy` | Substring-based fuzzy matching/filtering |

### DI / testing pattern

All external dependencies use small interfaces (1-3 methods). Commands expose package-level `*Deps` structs (e.g., `bootstrapDeps`, `openDeps`) — tests set these to mock implementations and restore via `t.Cleanup()`. Integration tests in `cmd/root_integration_test.go` build the binary and test via subprocess execution.

### Server bootstrap

`PersistentPreRunE` calls `EnsureServer()` for commands needing tmux (all except version, init, help, alias, clean). If the server was just started, TUI shows a loading page; CLI commands call `bootstrapWait()` which prints to stderr and polls for session restoration (1–6s window).

## Release

Uses goreleaser (`.goreleaser.yaml`). Version injected via ldflags: `-X github.com/leeovery/portal/cmd.version`. Tagged releases trigger GitHub Actions → homebrew tap update.
