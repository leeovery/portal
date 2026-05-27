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
| `tmux` | Wraps tmux CLI via `Commander` interface (`RealCommander` → `os/exec` with separate `Run`/`RunRaw` for trim vs verbatim output). Client methods cover sessions (ListSessions, NewSession, NewSessionWithCommand, NewDetachedSessionNoCwd, HasSession, KillSession, RenameSession, SwitchClient, CurrentSessionName), windows/panes (NewWindow, SplitWindow, ListPanes, ListPanesInSession, ListWindowsAndPanesInSession, ListAllPanes, ListAllPanesWithFormat, ResolveStructuralKey, SendKeys, RespawnPane, SelectLayout, SelectPane, ResizePaneZoom, CapturePane), environment (ShowEnvironment, SetSessionEnvironment), server lifecycle (ServerRunning, StartServer, EnsureServer), options (SetServerOption, SetSessionOption, GetServerOption, TryGetServerOption, UnsetServerOption, ShowAllServerOptions), and global hooks (ShowGlobalHooks, AppendGlobalHook, UnsetGlobalHookAt). Also exposes `BootstrapPortalSaver` / `EnsurePortalSaverVersion` for the `_portal-saver` session lifecycle — `EnsurePortalSaverVersion` runs the Component A kill-barrier: `kill-session` then a 5s exit poll, then identity-checked direct SIGKILL escalation against the recorded `daemon.pid` if the orphan refuses to die. `SaverPanePIDOrAbsent(*Client, sessionName) (pid int, present bool, err error)` is the tri-state helper consumed by the orphan-sweep, Component D self-supervision probe, and `_portal-saver` end-state assertions — collapses `ErrNoSuchSession` and `ErrEmptyPaneList` into `present=false` while passing other errors through. `WindowGroup` is the shape returned by `ListWindowsAndPanesInSession` — a window-grouped pane enumeration consumed by the TUI scrollback preview |
| `state` | Resurrection state model: schema, capture, atomic commit, scrollback dump/replay, FIFO plumbing, marker helpers (notably `@portal-restoring` via `IsRestoringSet`), pane-key resolution, paths, structured logger. Contains the `portal state daemon` runtime invariants (capture loop, FIFO sweep, version guard). Also exposes `AcquireDaemonLock(stateDir)` / `ErrDaemonLockHeld` — the `daemon.lock` flock-based singleton primitive enforcing N ≤ 1 daemons per state directory (lock fd is retained in a `cmd` package-level var for the daemon's lifetime). Component C strengthens this with a `daemon.pid` pre-check (identifies the recorded PID; if alive and a `portal state daemon`, refuse with `ErrDaemonLockHeld` *before* opening `daemon.lock`) plus a post-flock `fstat`/`stat` inode cross-check against the `O_EXCL|O_CREAT` open and a bounded 3-iteration retry loop on inode mismatch to absorb the unlink+recreate race. `IdentifyDaemon(pid)` is the shared identity-check primitive consumed by Components A/B/C — wraps `ps -o pid,comm,args -p <pid>` against the canonical `PortalDaemonArgvPattern` and returns one of three results (`IdentifyAlive` for a live `portal state daemon`, `IdentifyDead` for canonical pid-not-found, and a transient-error case where the caller applies component-specific policy). `PgrepPortalDaemons()` is the canonical `pgrep -fx '^portal state daemon( |$)'` enumeration — single source of truth used by bootstrap step 4 (orphan sweep) and tests; eliminates prod/test drift risk. `TailScrollback(path, n)` is a single-fd reverse-chunked read that returns at most the last n newline-terminated records of a `.bin` file with a three-shape return contract (`(bytes, nil)` / `(nil, nil)` for ENOENT-or-empty / `(nil, err)` for OS errors) — used by the TUI scrollback preview |
| `restore` | Two-phase restore engine consumed by bootstrap step 6: phase A reconstructs skeleton (sessions/windows/panes via new-session/new-window/split-window with `respawn-pane -k` swapping the default shell for the hydrate helper), phase B applies geometry (select-layout, select-pane, resize-pane -Z) and replays scrollback through per-pane FIFOs |
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
| `tmuxerr` | Leaf, dependency-free error sentinels for tmux command failures (`ErrNoSuchSession`, `ErrEmptyPaneList`, etc.). Factored out of `internal/tmux` to break a latent import cycle — consumed by `internal/tmux` (wrapping) and other internal packages that need to discriminate tmux error classes without depending on the tmux client itself |
| `portaltest` | Test-only helpers — production code must not import. Flagship `IsolateStateForTest(t) (env []string, stateDir string)` returns per-test isolated env that scrubs developer `XDG_CONFIG_HOME` and registers a fingerprint-diff cleanup backstop. Also exposes `SpawnIsolatedDaemon` / `RegisterSubprocessCleanup` (the canonical subprocess reap pattern for daemon-spawning integration tests), `SnapshotStateDir` / `DiffFingerprints` / `FormatDelta` / `Fingerprint` (state-dir fingerprinting; deltas include a `hashed-changed` label for content-only edits), `PgrepPortalDaemons` (forwards to `state.PgrepPortalDaemons`), and `ReadPortalLogSafe` (read portal.log without failing on ENOENT). `*testing.T`-first parameter on the flagship helper structurally enforces test-only consumption for most exports; the small set of exceptions (fingerprint helpers, `ReadPortalLogSafe`) rely on contributor discipline. See "Test isolation for daemon-spawning tests" below |
| `ui` | Shared Bubble Tea file-browser model (`BrowserCancelMsg`, `BrowserDirSelectedMsg`, `DirLister`) consumed by both `internal/tui` and `cmd/open.go` |
| `fileutil` | Shared utilities — `AtomicWrite` (temp file + rename) used by hooks store |
| `fuzzy` | Substring-based fuzzy matching/filtering |
| `restoretest` / `tmuxtest` / `portalbintest` / `transienttest` | Test-only helpers — production code must not import these. `restoretest` holds restore-specific scaffolding (sessions.json seeders, signal-hydrate drivers, marker-clear pollers, integration-tagged `BuildPortalBinaryDir`/`BuildPortalBinaryStable` wrappers). `tmuxtest` provides real-tmux socket fixtures. `portalbintest` provides general-purpose `go build` / PATH-stage helpers (`BuildPortalBinary`, `StagePortalBinary`, `ProjectRoot`) consumed by restore, daemon, and saver integration tests. `transienttest` provides the shared `list-panes -a` tmux-transient scaffolding (`Commander` + `FailureMode` + `PassThrough`/`FailExitNonZero`/`FailEmptyStdout`, `SocketCommander` pass-through, `SeedHooksJSON`/`HooksJSONBytes`/`ResolveHooksFilePathFromEnv` hook seeders) consumed by the bootstrap-step-11 and `portal clean` integration tests — single canonical declaration so the failure-mode contract cannot drift between the two destructive consumers of `ListAllPanes` |

### Config path resolution (cmd/config.go)

All config files (`projects.json`, `aliases`, `hooks.json`) resolve via `configFilePath`: per-file env var → `XDG_CONFIG_HOME/portal/` → `~/.config/portal/`. On first access, `migrateConfigFile` performs a one-shot move from the old macOS path (`~/Library/Application Support/portal/`) — never overwrites existing files at the new path.

### DI / testing pattern

All external dependencies use small interfaces (1-3 methods). Commands expose package-level `*Deps` structs (e.g., `bootstrapDeps`, `openDeps`, `hooksDeps`) — tests set these to mock implementations and restore via `t.Cleanup()`. Integration tests in `cmd/root_integration_test.go` build the binary and test via subprocess execution.

#### Test isolation for daemon-spawning tests

Any test that runs `portal state daemon` directly OR via `portal open` / bootstrap MUST call `portaltest.IsolateStateForTest(t *testing.T) (env []string, stateDir string)` (in `internal/portaltest/isolated_env.go`) and apply the returned env to every spawned subprocess (`cmd.Env = env`). Without this, the spawned daemon inherits the developer's `$XDG_CONFIG_HOME` and writes to the real `~/.config/portal/state/` — the slow-open / empty-previews / zombie-session incident is the canonical example of how a leaked test daemon corrupts the live install. The helper also registers a `t.Cleanup` fingerprint-diff backstop that walks the developer's state dir post-test and fails on any delta; the backstop is **defence-in-depth, not a substitute** for the env override. No lint or CI enforcement exists — the rule is contributor-discipline plus the structurally-mandatory `*testing.T` parameter (which prevents importing the helper from non-`*_test.go` code).

For daemon-spawning tests, prefer `portaltest.SpawnIsolatedDaemon` + `portaltest.RegisterSubprocessCleanup` over hand-rolling `exec.Cmd` / `cmd.Wait`. These helpers wrap the canonical SIGKILL+Wait+reap pattern that prevents zombie subprocesses and ensures the test's `*testing.T` sees deterministic teardown order — both are load-bearing on macOS where unreaped subprocesses can hold open the state dir's file descriptors past the test's `t.Cleanup` window.

### Server bootstrap

`PersistentPreRunE` runs an eleven-step `bootstrap.Orchestrator` (in `cmd/bootstrap/`) for commands needing tmux (all except version, init, help, alias, clean). Step ordering is load-bearing; "Return" is the post-step boundary, not a numbered step:

1. **EnsureServer** — start the tmux server if not running.
2. **RegisterPortalHooks** — install global tmux hooks (e.g. `client-attached` running `portal state signal-hydrate`) idempotently.
3. **Set `@portal-restoring`** — must precede saver/restore so the daemon and hydrate helpers can detect they are inside a restoration window.
4. **SweepOrphanDaemons** — best-effort `pgrep -fx '^portal state daemon( |$)'` enumeration; identity-checked SIGKILL of any candidate that is not the `_portal-saver` pane's PID. Runs before `EnsureSaver` so the new saver-pane daemon's first tick is uncontested by leftover daemons from prior server lifetimes. Composes with the kill-barrier in `EnsureSaver` (which handles the single recorded `priorPID`) to converge multi-orphan installs to a singleton in one bootstrap. Errors log at WARN under `ComponentBootstrap` and are swallowed; never escalates to a fatal abort.
5. **EnsureSaver** — bootstrap (or version-upgrade) the `_portal-saver` detached session that hosts `portal state daemon`. Best-effort; failure surfaces as a `SaverDownWarning`.
6. **Restore** — invoke `internal/restore` to reconstruct skeleton + geometry + scrollback FIFOs from the saved state. Never escalates to a fatal abort; corrupt state surfaces as a warning.
7. **EagerSignalHydrate** — best-effort write of the hydrate signal byte to every freshly-armed `@portal-skeleton-*` pane's FIFO. Production wiring uses `state.DefaultFIFOSignaler` / `state.SendHydrateSignal` — the no-seam entry point that pins `OpenFIFOForSignal` + `time.Sleep`. The lower-level `state.WriteFIFOSignal` is the seam-bearing variant used only for retry-ladder unit tests. Runs while `@portal-restoring` is still set so daemon `captureAndCommit` suppression remains in force during helper-driven scrollback replay. Eliminates the per-session signaling gap that would otherwise leave N-1 saved sessions' helpers waiting on `client-attached` events that never fire for non-attached sessions. A non-nil return logs at WARN under `ComponentBootstrap` and is swallowed; never escalates to a fatal abort.
8. **Clear `@portal-restoring`** — fatal on failure (the marker must not leak past bootstrap).
9. **CleanStaleMarkers** — best-effort cleanup of `@portal-skeleton-*` server-option markers whose paneKey is no longer represented by a live pane. Runs after Clear so it observes post-restore tmux state, and before Sweep so any stale markers protecting orphan FIFOs are unset first, allowing those FIFOs to be reclaimed in the same bootstrap.
10. **SweepOrphanFIFOs** — best-effort cleanup of orphan `hydrate-*.fifo` files whose paneKey is no longer represented by a live `@portal-skeleton-*` marker.
11. **CleanStale** — best-effort cleanup of orphaned markers / stale entries.

After step 11, the orchestrator returns the accumulated warnings; the TUI's loading page (subject to `LoadingMinDuration` = 1.2s minimum-display pad) drains them via `LoadingMinElapsedMsg`, while the bare-CLI path drains them post-bootstrap.

If the server was just started, the TUI shows the loading page until both Restore completes and the 1.2s pad has elapsed; warnings flush to stderr (with alt-screen toggle) only after dismissal so the rendered UI is not corrupted.

### Daemon self-supervision

The `portal state daemon` runs a per-tick **saver-membership self-check** before each `captureAndCommit`. On every ticker fire (1s `TickerPeriod`), the daemon asks "am I still the pane process of the live `_portal-saver` session?" via `tmux.SaverPanePIDOrAbsent` plus a `pane_pid == os.Getpid()` comparison. A failing probe (saver absent, pane pid mismatch, or transient tmux error) increments an in-process consecutive-absence counter. A passing probe resets it to 0.

When the counter reaches `selfSupervisionHysteresisTicks = 3` consecutive ticks (declared in `cmd/state_daemon.go`; clamp-floor + clamp-ceiling pinned by `TestSelfSupervisionHysteresisTicks_ClampInvariant`), the daemon:

1. Logs INFO under `ComponentDaemon`: `"self-supervision: saver-membership lost for N consecutive ticks, exiting"`.
2. Calls `os.Exit(0)` immediately — bypassing `daemonShutdownFunc` so the divergent-view daemon does **not** execute one more `captureAndCommit` / `gcOrphanScrollback` cycle on its way out (same reasoning as Component A's straight-to-SIGKILL choice).
3. Leaves `daemon.pid` stale on disk. The next acquire's Component C pre-check handles this correctly (recorded PID is dead → pre-check skips → new daemon acquires cleanly). Cleanup logic to delete `daemon.pid` before the eject would be racy against a concurrent pre-check and would invert the layered-enforcement contract — **do not add it**.

The hysteresis value justification is documented in-source as a comment block above the constant (per-scenario tick counts measured against real tmux 3.6b, 2× safety factor, clamp result, measurement date, binary version). The integration-tagged harness `cmd/state_daemon_hysteresis_measurement_test.go` re-runs the experiment to verify the safety-factor invariant whenever it executes.

### Resume hooks

Per-pane hooks are registered via `portal hooks set --on-resume "cmd"` and persisted in `hooks.json`. `portal hooks rm --on-resume` removes the entry for the current pane (resolved from `$TMUX_PANE`); pass `--pane-key <key>` as a literal pass-through to remove an entry for any structural key, bypassing the `$TMUX_PANE` requirement (enables pruning of entries whose pane no longer exists). They fire **only inside the hydrate helper's exec chain** (`portal state hydrate`), which is launched as the initial process of each restored pane via `respawn-pane -k` during bootstrap step 6. After the helper finishes scrollback replay it looks up the saved structural hook key in `hooks.json` and exec's either `sh -c '<HOOK>; exec $SHELL'` or a bare `$SHELL` — meaning hooks fire on reboot recovery, not on every detach/reattach inside the same tmux server lifetime. The structural key is preserved across base-index drift so lookups stay addressable. Stale hook entries are cleaned lazily by the daemon and explicitly via `portal clean`.

## Release

Uses goreleaser (`.goreleaser.yaml`). Version injected via ldflags: `-X github.com/leeovery/portal/cmd.version`. Tagged releases trigger GitHub Actions → homebrew tap update.
