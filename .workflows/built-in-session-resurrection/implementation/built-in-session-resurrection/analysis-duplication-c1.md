---
agent: duplication
cycle: 1
findings_count: 11
---
# Duplication Analysis (Cycle 1)

## Summary

Eleven duplication findings spanning ~250 LOC. Six are unaddressed repeats from the prior apply-cycle commit (paneKey helper, Logger nil-guards, RestoreWithMarker vs orchestrator, tmuxSocket harness, DeleteServerOption/UnsetServerOption, xdgConfigBase) — flagged but not consolidated.

---

## Findings

### FINDING: paneKey-from-FIFO helper duplicated across packages
- **Severity**: medium
- **Files**: `cmd/state_hydrate.go:69-77`, `internal/state/fifo_sweep.go:57-64`
- **Description**: `paneKeyFromFIFOPath` (cmd) and `paneKeyFromFIFOFilename` (state) both invert `state.FIFOPath`'s filename component with identical logic (`TrimSuffix .fifo`, `TrimPrefix hydrate-`). Two unexported copies in two packages of the inverse of an exported helper. Cycle 1 already flagged this as MAJOR; the apply-cycle commit did not consolidate. Future rename of the FIFO naming convention requires editing two files in lock-step or breaks round-trip silently.
- **Recommendation**: Promote a single `state.PaneKeyFromFIFOPath(fifoPath string) string` next to `state.FIFOPath` in `internal/state/paths.go` (it owns the encode side; symmetry argues it owns the decode side too). Have it accept either an absolute path or a basename — `filepath.Base` is idempotent on a basename. Replace both call sites; delete the unexported duplicates.

### FINDING: redundant `if Logger != nil` guards across new files
- **Severity**: medium
- **Files**: `internal/restore/restore.go:75,91,110,128,137,149,156`, `internal/restore/restore_marker.go:56`, `internal/restore/session.go:184,187,195,203,315,334`, `cmd/state_hydrate.go:191,245,279,304,323`, `cmd/state_signal_hydrate.go:60,68,80`, `cmd/bootstrap/bootstrap.go:154,164,178,196`
- **Description**: 26 nil-checks of `*state.Logger` fields across six files even though `internal/state/logger.go:53-54,238-240` documents `*Logger` as a valid no-op when nil. Cycle 1 flagged 22+; the count grew. Inconsistent with `internal/state/scrollback.go:52,67`, `internal/state/commit.go:53,133`, `internal/state/status.go`, and `cmd/state_cleanup.go:151,154` which all call `logger.Warn/Info/Error` directly. New contributors will copy whichever style they see first.
- **Recommendation**: Strip every `if .Logger != nil` guard whose only body is a single `Logger.Warn/Info/Error` call. Tests already exercise nil-Logger paths via `state.NopLogger()`, so coverage is unchanged.

### FINDING: bootstrap orchestrator marker discipline duplicated by `RestoreWithMarker`
- **Severity**: medium
- **Files**: `internal/restore/restore_marker.go:1-61`, `cmd/bootstrap/bootstrap.go:145-174`, `cmd/bootstrap_production.go:46-54`
- **Description**: `cmd/bootstrap/bootstrap.go` already owns the `@portal-restoring` set/clear discipline (steps 3 and 6); production wires it via `restoringMarkerAdapter`. `internal/restore/restore_marker.go` independently re-implements the same lifecycle (`SetRestoring`, `ClearRestoring`, `RestoreWithMarker`) for integration tests. The hardcoded `restoringMarker = "@portal-restoring"` literal in restore_marker.go:14 duplicates `state.RestoringMarkerName` (markers.go:15). Production now bypasses `RestoreWithMarker` entirely (`bootstrap_production.go:90-93` calls bare `inner.Restore()`); `RestoreWithMarker` exists only for `internal/restore/integration_test.go` and `restore_marker_test.go`.
- **Recommendation**: Delete `internal/restore/restore_marker.go` and `restore_marker_test.go`; rewrite the Phase-3 integration tests to drive the marker inline (or wrap with a tiny test-only helper). One owner of the marker lifecycle, one source of truth for the marker name.

### FINDING: tmuxSocket integration-test harness duplicated between two packages
- **Severity**: medium
- **Files**: `internal/restore/integration_test.go:37-161`, `cmd/bootstrap/phase5_integration_test.go:41-153`
- **Description**: ~120 lines of byte-equivalent or near-byte-equivalent scaffolding — `tmuxSocket` struct, `newTmuxSocket`, `tmuxCmd`, `run`, `tryRun`, `killServer`, `socketCommander` (with `Run`/`RunRaw`), `client()`, `waitForSession` — exists in both files. The `socketCommander` Run/RunRaw bodies are line-for-line identical (compare integration_test.go:117-137 vs phase5_integration_test.go:117-134). Drift already showing: phase5 omits the `_ = out` discard in waitForSession.
- **Recommendation**: Promote to a new `internal/tmuxtest` package (Go test helpers in non-`_test.go` files inside `internal/tmuxtest/` are importable from any package's test files). Move `tmuxSocket`, `socketCommander`, `waitForSession` there; both integration tests import the shared types.

### FINDING: real-step adapters in phase5 integration test duplicate production adapters
- **Severity**: medium
- **Files**: `cmd/bootstrap/phase5_integration_test.go:155-187`, `cmd/bootstrap_production.go:38-54,26-36`
- **Description**: phase5_integration_test.go defines `realRestoringMarker` (Set/Clear pass-through to client.SetServerOption/UnsetServerOption with literal `"@portal-restoring"`) and `realHookRegistrar` (RegisterPortalHooks pass-through). Production has the byte-equivalent `restoringMarkerAdapter` and `hookRegistrarAdapter`. The phase5 versions also use a hardcoded literal instead of `state.RestoringMarkerName`. The phase5 copies were a phase-5-cutover ergonomic that survived past the cutover.
- **Recommendation**: Extract the production adapters into a small `internal/bootstrapadapter` package (or expose them in a way the phase5 test can import); delete the test-side copies. Alternatively the phase5 integration test could move into `cmd` package and consume `cmd/bootstrap_production.go` types directly.

### FINDING: DeleteServerOption and UnsetServerOption are byte-identical
- **Severity**: low
- **Files**: `internal/tmux/tmux.go:440-462`
- **Description**: Both methods invoke `c.cmd.Run("set-option", "-su", name)` with only the wrapped error message differing. The doc-comment on `UnsetServerOption` even acknowledges the duplication ("This method exists alongside DeleteServerOption to provide a Set/Unset-named pair"). Two test functions exercise the same code path. Cycle 1 flagged this; not consolidated.
- **Recommendation**: Delete `DeleteServerOption`. Migrate the four callers to `UnsetServerOption` (the Set/Unset name pair is the better symmetry). Update `TestDeleteServerOption` callers to point at the surviving function.

### FINDING: SaverDownError + LastSaverErr + SaverDownWarning triple-encode the same event
- **Severity**: low
- **Files**: `cmd/bootstrap/bootstrap.go:80-93,108-114,150-158`, `cmd/bootstrap/errors.go:63-71`
- **Description**: Step 4 (EnsureSaver) failure is encoded three times: (1) wrapped in `*SaverDownError` and stashed on `Orchestrator.LastSaverErr`; (2) appended to `warnings` as the result of `SaverDownWarning()`; (3) logged via `Logger.Warn`. The doc-comment on `LastSaverErr` admits this is for "backwards compatibility with Phase 5 callers." No production caller reads `LastSaverErr` (grep shows only test reads). The `SaverDownError` wrapping never round-trips to a consumer that calls `errors.As` on it.
- **Recommendation**: Delete `SaverDownError` and the `LastSaverErr` field. The warning append + log line is the consumer surface. Update bootstrap_test.go:260-261 to assert the warning slice contains `SaverDownWarning()` instead of an `errors.Is` on LastSaverErr.

### FINDING: per-event hook registration loop duplicated three times
- **Severity**: low
- **Files**: `internal/tmux/hooks_register.go:109-134`
- **Description**: `RegisterPortalHooks` contains three identical `for _, event := range <slice>` loops, each calling `RegisterHookIfAbsent(c, event, <substring>, <command>)` and accumulating errors into the same `errs []error`. The three categories differ only in (slice, substring, command) triple. Adding a fourth event category requires a fourth copy. The unregister side has already factored this idea into `dedupedEventList` + `portalCommandSubstrings`.
- **Recommendation**: Extract a `type hookCategory struct { events []string; substring, command string }` and range over a `[]hookCategory{notify, signalHydrate, migrateRename}`. Three table entries replace 18 lines of repeated control flow.

### FINDING: daemon startup re-implements ReadIndex
- **Severity**: low
- **Files**: `cmd/state_daemon.go:240-248`, `internal/state/index_reader.go:36-51`
- **Description**: `cmd/state_daemon.go` loads the prior structural index by hand: `os.ReadFile(state.SessionsJSON(dir))` → `state.DecodeIndex(data)` → assign to `prevIdx`. `state.ReadIndex` (used by `internal/restore/restore.go:39`) is exactly this composition with proper `ErrCorruptIndex` classification. The daemon's ad-hoc version doesn't distinguish corruption from missing — both log "decode prior sessions.json".
- **Recommendation**: Replace the daemon's hand-rolled load with `state.ReadIndex(dir)`; map `(skip=true, err=nil)` to `prevIdx=nil` no-log, `(skip=true, err!=nil)` to `prevIdx=nil` + warn, `(skip=false, _)` to `prevIdx=&idx`. ~9 LOC → 5 LOC, with the same corrupt-index classification as restore.

### FINDING: surrounding-quote stripping inlined where helper exists
- **Severity**: low
- **Files**: `internal/state/markers.go:71-74`, `internal/tmux/hooks_parse.go:77-87`
- **Description**: `markers.go` strips surrounding double quotes from a server-option value with an inline 3-line check; `hooks_parse.go` exposes `stripMatchedOuterQuotes` which does the same job for both single and double quotes. Both parse `tmux show-*` output families. A future `show-options` value with single-quotes would survive the markers.go strip and break marker prefix logic silently.
- **Recommendation**: Hoist `stripMatchedOuterQuotes` to a shared leaf package (e.g. `internal/tmuxout`) since `internal/state` cannot import `internal/tmux`. Both call sites use it.

### FINDING: xdgConfigBase implemented twice with divergent signatures
- **Severity**: low
- **Files**: `cmd/config.go:63-72`, `internal/state/paths.go:98-110`
- **Description**: Both functions resolve `$XDG_CONFIG_HOME` with `~/.config` fallback. cmd's takes `homeDir` as arg; state's resolves it internally and returns an error. The state version's comment admits "Duplicated here because the cmd package cannot be imported by internal packages." Cycle 1 flagged — acknowledged but not consolidated. Drift could silently send Portal config and state to different roots.
- **Recommendation**: Hoist into a shared leaf package both can import (e.g. `internal/xdg`) with signature `func ConfigBase() (string, error)`. Update cmd/config.go's caller to drop the homeDir parameter.
