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
| `tmux` | Wraps tmux CLI via `Commander` interface (`RealCommander` → `os/exec` with separate `Run`/`RunRaw` for trim vs verbatim output). Client methods cover sessions (ListSessions, NewSession, NewSessionWithCommand, NewDetachedSessionNoCwd, HasSession, KillSession, RenameSession, SwitchClient, CurrentSessionName), windows/panes (NewWindow, SplitWindow, ListPanes, ListPanesInSession, ListWindowsAndPanesInSession, ListAllPanes, ListAllPanesWithFormat, ResolveStructuralKey, SendKeys, RespawnPane, SelectLayout, SelectPane, ResizePaneZoom, CapturePane), environment (ShowEnvironment, SetSessionEnvironment), server lifecycle (ServerRunning, StartServer, EnsureServer), options (SetServerOption, SetSessionOption, GetServerOption, TryGetServerOption, UnsetServerOption, ShowAllServerOptions), and global hooks (ShowGlobalHooks, AppendGlobalHook, UnsetGlobalHookAt). Also exposes `BootstrapPortalSaver` / `EnsurePortalSaverVersion` for the `_portal-saver` session lifecycle. `WindowGroup` is the shape returned by `ListWindowsAndPanesInSession` — a window-grouped pane enumeration consumed by the TUI scrollback preview |
| `state` | Resurrection state model: schema, capture, atomic commit, scrollback dump/replay, FIFO plumbing, marker helpers (notably `@portal-restoring` via `IsRestoringSet`), pane-key resolution, paths, structured logger. Contains the `portal state daemon` runtime invariants (capture loop, FIFO sweep, version guard). Also exposes `TailScrollback(path, n)` — a single-fd reverse-chunked read that returns at most the last n newline-terminated records of a `.bin` file with a three-shape return contract (`(bytes, nil)` / `(nil, nil)` for ENOENT-or-empty / `(nil, err)` for OS errors) — used by the TUI scrollback preview |
| `restore` | Two-phase restore engine consumed by bootstrap step 5: phase A reconstructs skeleton (sessions/windows/panes via new-session/new-window/split-window with `respawn-pane -k` swapping the default shell for the hydrate helper), phase B applies geometry (select-layout, select-pane, resize-pane -Z) and replays scrollback through per-pane FIFOs |
| `session` | Session creation pipeline: git root resolution → project persistence → name generation (`{project}-{nanoid}`) → tmux session creation. `QuickStart` for atomic create-or-attach |
| `resolver` | Path resolution chain with interface-based DI (AliasLookup, ZoxideQuerier, DirValidator) |
| `tui` | Bubble Tea model with page state machine: Loading → Sessions → Projects → FileBrowser → Preview. `LoadingMinDuration = 1.2s` enforces a minimum loading-page display window. The `pagePreview` arm (peer of `pageFileBrowser`) renders a read-only scrollback preview when `Space` is pressed on the Sessions page; the `previewModel` is constructor-injected with `TmuxEnumerator` and `ScrollbackReader` seam interfaces (production adapters wire `*tmux.Client` and the `state.TailScrollback` helper, with `stateDir` resolved once at TUI construction). The dismiss handler dispatches a sessions-list refresh on the `pagePreview → pageSessions` transition so externally-killed sessions disappear from the post-dismiss list |
| `project` | JSON-backed store (`~/.config/portal/projects.json`) with atomic writes |
| `alias` | Flat key=value file store (`~/.config/portal/aliases`) |
| `browser` | Directory listing with symlink detection |
| `hooks` | JSON-backed `Store` (`~/.config/portal/hooks.json`) holding per-pane on-resume commands keyed by structural pane key. Pure persistence — no execution. Hook firing is now driven by the hydrate helper's exec chain (`portal state hydrate`), not by the cmd layer at attach time |
| `bootstrapadapter` | Production adapters wiring concrete `*tmux.Client`, hooks store, and state package functions to the `cmd/bootstrap` Orchestrator's seam interfaces |
| `warning` | Canonical `Warning` shape and `WriteLines` stderr helper for soft bootstrap warnings. Single source of truth straddling the `cmd → tui` import boundary; `cmd/bootstrap` aliases the type and both CLI and TUI emission paths delegate to `WriteLines` so output is byte-identical |
| `xdg` | Leaf package resolving `$XDG_CONFIG_HOME` (with `~/.config` fallback). Single source of truth consumed by both `cmd/config.go` and `internal/state/paths.go` |
| `tmuxout` | Leaf, dependency-free helpers for parsing `tmux show-*` output (e.g., `StripMatchedOuterQuotes`). Imported by both `internal/tmux` and `internal/state` to avoid an import cycle |
| `ui` | Shared Bubble Tea file-browser model (`BrowserCancelMsg`, `BrowserDirSelectedMsg`, `DirLister`) consumed by both `internal/tui` and `cmd/open.go` |
| `fileutil` | Shared utilities — `AtomicWrite` (temp file + rename) used by hooks store |
| `fuzzy` | Substring-based fuzzy matching/filtering |
| `restoretest` / `tmuxtest` | Test-only helpers (shared restore drivers, real-tmux socket fixtures) — production code must not import these |

### Config path resolution (cmd/config.go)

All config files (`projects.json`, `aliases`, `hooks.json`) resolve via `configFilePath`: per-file env var → `XDG_CONFIG_HOME/portal/` → `~/.config/portal/`. On first access, `migrateConfigFile` performs a one-shot move from the old macOS path (`~/Library/Application Support/portal/`) — never overwrites existing files at the new path.

### DI / testing pattern

All external dependencies use small interfaces (1-3 methods). Commands expose package-level `*Deps` structs (e.g., `bootstrapDeps`, `openDeps`, `hooksDeps`) — tests set these to mock implementations and restore via `t.Cleanup()`. Integration tests in `cmd/root_integration_test.go` build the binary and test via subprocess execution.

### Server bootstrap

`PersistentPreRunE` runs a nine-step `bootstrap.Orchestrator` (in `cmd/bootstrap/`) for commands needing tmux (all except version, init, help, alias, clean). Step ordering is load-bearing:

1. **EnsureServer** — start the tmux server if not running.
2. **RegisterPortalHooks** — install global tmux hooks (e.g. `client-attached` running `portal state signal-hydrate`) idempotently.
3. **Set `@portal-restoring`** — must precede saver/restore so the daemon and hydrate helpers can detect they are inside a restoration window.
4. **EnsureSaver** — bootstrap (or version-upgrade) the `_portal-saver` detached session that hosts `portal state daemon`. Best-effort; failure surfaces as a `SaverDownWarning`.
5. **Restore** — invoke `internal/restore` to reconstruct skeleton + geometry + scrollback FIFOs from the saved state. Never escalates to a fatal abort; corrupt state surfaces as a warning.
6. **Clear `@portal-restoring`** — fatal on failure (the marker must not leak past bootstrap).
7. **SweepOrphanFIFOs** — best-effort cleanup of orphan `hydrate-*.fifo` files whose paneKey is no longer represented by a live `@portal-skeleton-*` marker.
8. **CleanStale** — best-effort cleanup of orphaned markers / stale entries.
9. **Return** — collect warnings; the TUI's loading page (subject to `LoadingMinDuration` = 1.2s minimum-display pad) drains them via `LoadingMinElapsedMsg`, while the bare-CLI path drains them post-bootstrap.

If the server was just started, the TUI shows the loading page until both Restore completes and the 1.2s pad has elapsed; warnings flush to stderr (with alt-screen toggle) only after dismissal so the rendered UI is not corrupted.

### Resume hooks

Per-pane hooks are registered via `portal hooks set --on-resume "cmd"` and persisted in `hooks.json`. They fire **only inside the hydrate helper's exec chain** (`portal state hydrate`), which is launched as the initial process of each restored pane via `respawn-pane -k` during bootstrap step 5. After the helper finishes scrollback replay it looks up the saved structural hook key in `hooks.json` and exec's either `sh -c '<HOOK>; exec $SHELL'` or a bare `$SHELL` — meaning hooks fire on reboot recovery, not on every detach/reattach inside the same tmux server lifetime. The structural key is preserved across base-index drift so lookups stay addressable. Stale hook entries are cleaned lazily by the daemon and explicitly via `portal clean`.

## Release

Uses goreleaser (`.goreleaser.yaml`). Version injected via ldflags: `-X github.com/leeovery/portal/cmd.version`. Tagged releases trigger GitHub Actions → homebrew tap update.
