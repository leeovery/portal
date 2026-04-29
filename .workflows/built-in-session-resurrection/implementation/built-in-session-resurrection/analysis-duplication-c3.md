---
agent: duplication
cycle: 3
findings_count: 6
---
# Duplication Analysis (Cycle 3)

## Summary

Two high-severity duplications (twin `restoreOrchestratorAdapter` definitions; `socketCommander.Run/RunRaw` re-deriving `tmux.RealCommander.Run/RunRaw`); three medium items (logger.go OpenFile triplet, `%s:%d.%d` pane-target Sprintf across 4 files, `tmux.NewClient(&tmux.RealCommander{})` repeated 7x); one low rule-of-three for `ensure state dir` error wrap.

---

## Findings

### FINDING: `restoreOrchestratorAdapter` defined twice with identical name and shape
- **Severity**: high
- **Files**: `cmd/bootstrap_production.go:48-78`, `cmd/bootstrap/phase5_integration_test.go:71-82`
- **Description**: Both files declare a type literally named `restoreOrchestratorAdapter` whose sole purpose is to wrap a `*restore.Orchestrator` so its `Restore()` satisfies the `bootstrap.Restorer` `(corrupt, err)` contract. The production version adds an orphan-FIFO sweep after Restore; the test version is a bare delegator (`return a.inner.Restore()`). Both wrap the same inner type and exist to bridge the same two interfaces. Different packages, but answering the exact same question — any change to the bridging contract (Restorer signature, marker lifecycle) must be applied to both. The test-only version is also redundant given `internal/bootstrapadapter` already houses production-shape adapters and is the natural home.
- **Recommendation**: Move the production `restoreOrchestratorAdapter` (or a sweep-free variant of it) into `internal/bootstrapadapter` next to `RestoringMarker` / `HookRegistrar` so the integration test can import it directly and delete its local twin. Keep the FIFO-sweep wrapper as a separate decorator in `cmd/` if the sweep dependency on `*state.Logger` keeps it out of `bootstrapadapter`. Either way, eliminate the duplicate type definition.

### FINDING: `socketCommander.Run`/`RunRaw` re-derive `tmux.RealCommander.Run`/`RunRaw`
- **Severity**: high
- **Files**: `internal/tmuxtest/socket.go:119-135`, `internal/tmux/tmux.go:39-58`
- **Description**: `socketCommander.Run` and `socketCommander.RunRaw` are byte-identical to `tmux.RealCommander.Run` / `RunRaw` apart from the argv prefix transformation (`socketArgs(...)`). Both build `exec.Command("tmux", ...)`, call `.Output()`, return `("", err)` on failure, then either trim+stringify (Run) or stringify verbatim (RunRaw). Adding a third command shape (e.g. `CommandContext`, env injection, capture-stderr-on-failure) requires touching all four method bodies. The doc comments on the test methods even admit this: "matching tmux.RealCommander.Run" / "matching tmux.RealCommander.RunRaw" — explicitly acknowledging the drift surface introduced by T7-7.
- **Recommendation**: Either (a) parameterise `tmux.RealCommander` with an optional argv-prefix transformer that `socketCommander` sets, or (b) add a tiny private helper in `internal/tmuxtest` (e.g. `runRaw(args []string) ([]byte, error)`) that both methods delegate to so only the trim step differs. Option (b) is the minimal-blast-radius change.

### FINDING: `os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)` repeated three times in logger.go
- **Severity**: medium
- **Files**: `internal/state/logger.go:90,179,184`
- **Description**: The exact OpenFile flag-set + mode triplet appears three times: once in `OpenLogger` (initial open) and twice in `maybeRotate` (the post-rotate-failure reopen at line 179 and the post-rotate-success reopen at line 184). Any future change to open semantics — adding `O_SYNC` for crash safety, switching mode to a const, honouring a `PORTAL_LOG_MODE` env var — must be applied in all three sites. Within-file rule-of-three breach.
- **Recommendation**: Extract a small private helper `openAppendLog(path string) (*os.File, error)` in `logger.go` that wraps the OpenFile with the canonical flags+mode. Replace all three sites.

### FINDING: `target := fmt.Sprintf("%s:%d.%d", ...)` pane-target string repeated across four files
- **Severity**: medium
- **Files**: `internal/tmux/tmux.go:591,605`, `internal/restore/session.go:108,220`, `cmd/state_daemon.go:134`
- **Description**: The literal format `"%s:%d.%d"` with `(session, window, pane)` recurs five times across four files in resurrection-introduced code. Two sites are inside `*tmux.Client` methods that immediately pass the result as `-t <target>` (`SelectPane`, `ResizePaneZoom`); two are inside `internal/restore/session.go` (hookKey:108, liveTarget:220); one is inside `captureAndCommit` (state_daemon.go:134). `internal/tmux.PaneCoord` already exists for the inverse parse, so introducing a forward helper closes a symmetry gap. (`SelectLayout` at tmux.go:577 uses the related `%s:%d` form and could share a sibling helper or be left alone.)
- **Recommendation**: Add `func PaneTarget(session string, window, pane int) string` (or a method on `PaneCoord`) in `internal/tmux/tmux.go` next to `SanitizePaneKey`'s sibling primitives. Replace all five call sites. The internal tmux.go callers (`SelectPane`, `ResizePaneZoom`) call it locally; cross-package callers (restore, daemon) import it from the tmux package alongside the parse-side `PaneCoord`.

### FINDING: `tmux.NewClient(&tmux.RealCommander{})` cmd-side construction repeated seven times
- **Severity**: medium
- **Files**: `cmd/bootstrap_production.go:116`, `cmd/state_cleanup.go:49`, `cmd/state_daemon.go:251`, `cmd/state_signal_hydrate.go:154`, `cmd/state_hydrate.go:367` (resurrection-new); `cmd/clean.go:33`, `cmd/hooks.go:41` (pre-existing)
- **Description**: The two-allocation `tmux.NewClient(&tmux.RealCommander{})` construction appears seven times in `cmd/`, five of them in resurrection-introduced files. Five new occurrences in this implementation alone clears the rule-of-three threshold. Future production-client wiring (timeouts, an env-flagged tmux binary path, telemetry) would have to be propagated to all seven sites.
- **Recommendation**: Add a small package-level helper `tmux.DefaultClient()` (or `tmux.NewProductionClient()`) returning `NewClient(&RealCommander{})`, in `internal/tmux/tmux.go` next to `NewClient`/`RealCommander`. Drop the seven open-coded constructions.

### FINDING: `fmt.Errorf("ensure state dir: %w", err)` repeated verbatim in three cmd subcommands
- **Severity**: low
- **Files**: `cmd/state_daemon.go:209`, `cmd/state_signal_hydrate.go:140`, `cmd/state_notify.go:30`
- **Description**: All three internal subcommands open with the same pattern: `dir, err := state.EnsureDir(); if err != nil { return fmt.Errorf("ensure state dir: %w", err) }`. The wrapping prefix is byte-for-byte the same across three sites. This is the threshold case for rule-of-three, but the wrap is two lines tightly co-located with each command's RunE — extraction would arguably cost more clarity than it saves.
- **Recommendation**: Defer. If a fourth `cmd/state_*` subcommand lands needing the same shape, fold all four into a single helper. For now the duplication is small enough that extraction would not pay back the indirection cost.
