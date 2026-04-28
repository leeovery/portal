---
topic: built-in-session-resurrection
cycle: 1
total_proposed: 20
---
# Analysis Tasks: built-in-session-resurrection (Cycle 1)

## Task 1: Remove obsolete README hooks paragraph that contradicts the new firing model
status: pending
severity: high
sources: standards

**Problem**: `README.md:177` still describes the deleted send-keys + `@portal-active-<pane>` hook firing model ("Hooks fire via tmux send-keys when you attach/open a session. A volatile marker prevents duplicate execution within the same boot cycle — after a reboot the markers are gone and hooks re-fire."). The very next paragraph (lines 179-184) correctly describes the new "fires only on reboot recovery via the hydrate helper" model, leaving published documentation internally contradictory. The spec's "Documentation Deliverables → Existing User-Facing Documentation Updates" mandates: "Hooks documentation … clarify that hooks fire on reboot recovery … Update any examples that assumed the old ExecuteHooks attach-time firing semantics."
**Solution**: Delete line 177 entirely. The "When hooks fire:" paragraph below already carries the correct model.
**Outcome**: README's hooks section describes only the new reboot-recovery firing semantics; no contradiction remains.
**Do**:
1. Open `README.md` and locate line 177 (the obsolete "Hooks fire via tmux send-keys…" paragraph).
2. Delete the entire obsolete paragraph.
3. Verify the surrounding section still flows: the heading, then the "When hooks fire:" paragraph at lines 179-184 (now shifted up).
4. Skim the rest of the hooks documentation for any other references to `@portal-active-<pane>`, `ExecuteHooks`, or attach-time firing; there should be none — if any survive, remove them as well.
**Acceptance Criteria**:
- `grep -n "send-keys" README.md` returns no hits in the hooks section
- `grep -n "@portal-active" README.md` returns no hits
- The hooks section reads coherently top-to-bottom with only the new firing model described
**Tests**:
- No automated tests; manual review of the rendered README diff in PR

---

## Task 2: Implement functional migrate-rename hook key migration (or remove the inert scaffolding)
status: pending
severity: high
sources: standards, architecture

**Problem**: `internal/tmux/hooks_register.go:48-52` registers `portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'` — both args expand to the **new** session name. `cmd/state_migrate_rename.go:41-80`'s `runMigrateRename` then computes `prefix = oldName + ":"` from arg1 and rewrites matched keys to `newName + ":"` from arg2. Because old == new, the function either matches nothing (the hooks.json key still carries the OLD prefix at this moment, but `prefix` is built from the new name so no match) or rewrites keys to themselves. Either way no migration occurs; bootstrap step 7's `CleanStale` then prunes the orphaned key on the next bootstrap. Visible user impact: `portal hooks set --on-resume <cmd>` followed by `tmux rename-session work work-2026` silently loses the hook. The spec ("Resume Hook Firing → Session Rename: Hook Key Migration") commits to atomic migration on rename events; it does not authorise post-v1 deferral. The hook also incurs per-rename subprocess-spawn cost for zero value and adds a third dedupe-category to the register/unregister plumbing.
**Solution**: Either (a) implement daemon-side last-seen-names tracking so the hook receives both prior and current session names and `runMigrateRename` performs a real prefix rewrite, or (b) if neither implementation is achievable inside this v1, remove the hook registration + scaffolding entirely (drop `migrateRenameEvents` / `migrateRenameCommand` / `migrateRenameSubstring` from `RegisterPortalHooks`, leave `cmd/state_migrate_rename.go` as a future endpoint) and add a v2-deferral note to the spec acknowledging the gap. Wire-and-scaffolding that satisfies traceability checks but not the contract is worse than a documented gap.
**Outcome**: Either renames atomically migrate `hooks.json` keys end-to-end (verified by an integration-style test that registers a hook, renames the session, and confirms the key under the new name), OR the inert scaffolding is gone and the spec records the v2 deferral.
**Do**:
1. Choose (a) implement or (b) remove. If (a):
   - Add daemon-side tracking of last-seen session names (e.g. an in-process map keyed by tick-time enumeration of live session names), surface a rename as a (oldName, newName) delta.
   - Replace the registered command body so the hook receives the old name as arg1 and `#{hook_session_name}` as arg2.
   - Verify `runMigrateRename` performs the rewrite for old != new.
2. If (b):
   - Remove `migrateRenameEvents`, `migrateRenameCommand`, `migrateRenameSubstring` from `RegisterPortalHooks` (`internal/tmux/hooks_register.go`).
   - Update `UnregisterPortalHooks`: the deduped union of three slices collapses back to two — confirm no events remain orphaned.
   - Keep `cmd/state_migrate_rename.go` as a future endpoint (do not delete).
   - Add a spec note recording the v2 deferral.
3. Update or add tests to match the chosen path.
**Acceptance Criteria**:
- Path (a): `portal hooks set --on-resume <cmd>` + `tmux rename-session A B` results in the hook key now living under `B:<paneKey>` in `hooks.json`; bootstrap step 7's `CleanStale` does not prune it.
- Path (b): `RegisterPortalHooks` registers two categories only; `UnregisterPortalHooks` still cleans every Portal-owned hook; the source comment in `hooks_register.go:51` admitting the no-op is gone; spec records the v2 deferral.
**Tests**:
- Path (a): integration-style test driving a real session rename and asserting the hooks.json key migrated.
- Path (b): unit test on `RegisterPortalHooks` asserting only the two surviving categories register.

---

## Task 3: Consolidate paneKey-from-FIFO helper into internal/state
status: pending
severity: medium
sources: duplication, standards, architecture

**Problem**: `paneKeyFromFIFOPath` (`cmd/state_hydrate.go:69-77`) and `paneKeyFromFIFOFilename` (`internal/state/fifo_sweep.go:57-64`) both invert `state.FIFOPath`'s filename component with identical logic (`TrimSuffix .fifo`, `TrimPrefix hydrate-`). Two unexported copies in two packages of the inverse of an exported helper. The two sites disagree subtly: the cmd version operates on `filepath.Base(fifoPath)` first, the state version takes the basename directly. A future rename of the FIFO naming convention requires editing two files in lock-step or breaks round-trip silently. Cycle 0 already flagged this as MAJOR; the apply-cycle commit did not consolidate.
**Solution**: Promote a single `state.PaneKeyFromFIFOPath(path string) string` next to `state.FIFOPath` in `internal/state/paths.go`. Have it accept either an absolute path or a basename — `filepath.Base` is idempotent on a basename. Replace both call sites; delete the unexported duplicates.
**Outcome**: One canonical inverse helper colocated with `FIFOPath`; both call sites use it; future shape changes touch one file.
**Do**:
1. In `internal/state/paths.go`, add `func PaneKeyFromFIFOPath(path string) string` that calls `filepath.Base`, then trims the `hydrate-` prefix and `.fifo` suffix.
2. Add unit tests in `internal/state/paths_test.go` covering: absolute path input, basename-only input, idempotency on input that lacks prefix/suffix.
3. Update `cmd/state_hydrate.go` call site to use `state.PaneKeyFromFIFOPath`; delete the local helper.
4. Update `internal/state/fifo_sweep.go` call site likewise.
5. `go test ./...` to confirm.
**Acceptance Criteria**:
- `grep -rn "paneKeyFromFIFO" .` returns only the new exported helper
- Both former call sites compile and pass their existing tests
- New unit tests in `internal/state/paths_test.go` cover absolute, basename, and degenerate inputs
**Tests**:
- Unit tests for `PaneKeyFromFIFOPath` (absolute path, basename, idempotency)
- Existing `cmd/state_hydrate_test.go` and `internal/state/fifo_sweep_test.go` continue to pass

---

## Task 4: Remove redundant `if Logger != nil` guards across new files
status: pending
severity: medium
sources: duplication, standards

**Problem**: 26 nil-checks of `*state.Logger` fields exist in: `internal/restore/restore.go:75,91,110,128,137,149,156`, `internal/restore/restore_marker.go:56`, `internal/restore/session.go:184,187,195,203,315,334`, `cmd/state_hydrate.go:191,245,279,304,323`, `cmd/state_signal_hydrate.go:60,68,80`, `cmd/bootstrap/bootstrap.go:154,164,178,196`. `internal/state/logger.go:53-54,238-240` documents the contract: "A nil *Logger is a valid no-op: all methods bail early." Cleanup-side and `internal/state/scrollback.go`, `commit.go`, `status.go` all call `Logger.Warn/Info/Error` directly. Codebase is internally inconsistent; new contributors will copy whichever style they see first.
**Solution**: Strip every `if .Logger != nil` guard whose only body is a single `Logger.Warn/Info/Error` call across the six listed files. Trust the contract uniformly. Tests already exercise nil-Logger paths via `state.NopLogger()`.
**Outcome**: Zero defensive nil-Logger guards in production files; logger contract uniformly trusted; codebase consistent with cleanup-side style.
**Do**:
1. For each file in the list, remove `if cfg.Logger != nil` / `if r.Logger != nil` / `if o.Logger != nil` wrappers whose entire body is a single Logger method call. Replace with the unwrapped call.
2. Where a guard wraps multiple statements (mixed Logger + non-Logger), leave it.
3. Run the full test suite to verify nil-Logger paths still pass.
**Acceptance Criteria**:
- `grep -rn "if .*Logger != nil" internal/restore cmd/state_hydrate.go cmd/state_signal_hydrate.go cmd/bootstrap/bootstrap.go` returns no hits where the body is a single Logger call
- All existing tests pass
**Tests**:
- Existing tests in `internal/restore/`, `cmd/`, and `cmd/bootstrap/` packages pass with `state.NopLogger()` injection

---

## Task 5: Delete dead-code parallel restoring-marker API in internal/restore
status: pending
severity: medium
sources: duplication, architecture

**Problem**: Two complete implementations of the `@portal-restoring` marker lifecycle exist. The bootstrap.Orchestrator (`cmd/bootstrap/bootstrap.go:145-174`) owns set/clear discipline (steps 3 and 6); production wires it via `restoringMarkerAdapter` in `cmd/bootstrap_production.go:46-54` using `state.RestoringMarkerName`. Meanwhile `internal/restore/restore_marker.go:1-61` independently re-implements the same lifecycle with its own private `const restoringMarker = "@portal-restoring"` (line 14). Production now bypasses `RestoreWithMarker` entirely (`bootstrap_production.go:90-93` calls bare `inner.Restore()`); `RestoreWithMarker` exists only for `internal/restore/integration_test.go` and `restore_marker_test.go`.
**Solution**: Delete `internal/restore/restore_marker.go` and `restore_marker_test.go`; rewrite the Phase-3 integration tests to drive the marker inline (or via a tiny test-only helper).
**Outcome**: One owner of the marker lifecycle (bootstrap.Orchestrator); one source of truth for the marker name (`state.RestoringMarkerName`); zero dead code in `internal/restore`.
**Do**:
1. Identify all callers of `RestoreWithMarker`, `SetRestoring`, `ClearRestoring` — should be only `internal/restore/integration_test.go` and `restore_marker_test.go`.
2. Rewrite the Phase-3 integration tests: call `client.SetServerOption(state.RestoringMarkerName, "1")` before `Restore()`, then `client.UnsetServerOption(state.RestoringMarkerName)` after.
3. Delete `internal/restore/restore_marker.go` and `restore_marker_test.go`.
4. Run `go test ./internal/restore/...` to confirm.
**Acceptance Criteria**:
- `internal/restore/restore_marker.go` does not exist
- `grep -rn "RestoreWithMarker\|SetRestoring\|ClearRestoring" .` returns no hits
- `grep -rn "@portal-restoring" .` returns only `state/markers.go` plus documentation
- `internal/restore/integration_test.go` continues to exercise the marker discipline
**Tests**:
- Rewritten Phase-3 integration tests using `state.RestoringMarkerName` continue to pass

---

## Task 6: Collapse DeleteServerOption and UnsetServerOption to a single method
status: pending
severity: medium
sources: duplication, standards

**Problem**: `internal/tmux/tmux.go:440-462` defines both `DeleteServerOption` and `UnsetServerOption`, both invoking `c.cmd.Run("set-option", "-su", name)`, differing only in error-message format string. The doc-comment openly admits the duplication. Two test functions exercise the same code path. Two names for the same primitive invite call sites to drift.
**Solution**: Delete `DeleteServerOption`. Migrate the four callers to `UnsetServerOption` (Set/Unset is the better symmetric pair with `SetServerOption`). Update `TestDeleteServerOption` callers to point at the surviving function.
**Outcome**: One method (`UnsetServerOption`) for the `set-option -su` primitive; symmetric naming with `SetServerOption`.
**Do**:
1. `grep -rn "DeleteServerOption" .` to enumerate callers.
2. Replace each call site with `UnsetServerOption`.
3. Delete `DeleteServerOption` from `internal/tmux/tmux.go`.
4. Update or merge `TestDeleteServerOption` cases into `TestUnsetServerOption`.
5. Update any mock/fake `Commander` implementations that tracked `DeleteServerOption` separately.
6. `go test ./...` to confirm.
**Acceptance Criteria**:
- `grep -rn "DeleteServerOption" .` returns no hits
- All former callers compile against `UnsetServerOption`
- Tests for the wire op pass
**Tests**:
- Existing `TestUnsetServerOption` (and migrated cases) exercise `set-option -su`

---

## Task 7: Promote tmuxSocket integration-test harness to shared package
status: pending
severity: medium
sources: duplication

**Problem**: ~120 lines of byte-equivalent scaffolding — `tmuxSocket` struct, `newTmuxSocket`, `tmuxCmd`, `run`, `tryRun`, `killServer`, `socketCommander`, `client()`, `waitForSession` — exists in both `internal/restore/integration_test.go:37-161` and `cmd/bootstrap/phase5_integration_test.go:41-153`. The `socketCommander` Run/RunRaw bodies are line-for-line identical. Drift already showing: phase5 omits the `_ = out` discard in `waitForSession`.
**Solution**: Promote to a new `internal/tmuxtest` package (Go test helpers in non-`_test.go` files inside `internal/tmuxtest/` are importable from any package's test files). Move `tmuxSocket`, `socketCommander`, `waitForSession` there; both integration tests import the shared types.
**Outcome**: Single tmuxSocket harness imported by both integration tests; no drift; ~120 LOC reclaimed.
**Do**:
1. Create `internal/tmuxtest/socket.go` (non-`_test.go`). Move `tmuxSocket`, `newTmuxSocket`, `tmuxCmd`, `run`, `tryRun`, `killServer`, `socketCommander`, `client`, `waitForSession` there. Export as needed.
2. Update `internal/restore/integration_test.go` to import `internal/tmuxtest`.
3. Update `cmd/bootstrap/phase5_integration_test.go` likewise; restore the `_ = out` discard.
4. Delete the duplicated scaffolding from both test files.
5. `go test ./internal/restore/... ./cmd/bootstrap/...` to confirm.
**Acceptance Criteria**:
- `internal/tmuxtest/socket.go` is the only definition of the harness types
- Both integration test files import `internal/tmuxtest`
- Both integration suites pass
**Tests**:
- Existing integration tests in both packages pass

---

## Task 8: Eliminate real-step adapter copies in phase5 integration test
status: pending
severity: medium
sources: duplication

**Problem**: `cmd/bootstrap/phase5_integration_test.go:155-187` defines `realRestoringMarker` (Set/Clear pass-through with literal `"@portal-restoring"`) and `realHookRegistrar` (RegisterPortalHooks pass-through). Production has byte-equivalent `restoringMarkerAdapter` and `hookRegistrarAdapter` in `cmd/bootstrap_production.go:38-54,26-36`. Phase5 copies use a hardcoded literal instead of `state.RestoringMarkerName`.
**Solution**: Extract the production adapters into a small `internal/bootstrapadapter` package (or expose them so the phase5 test can import them); delete the test-side copies. Alternatively move the phase5 integration test into `cmd` package.
**Outcome**: One implementation of each real-step adapter; phase5 test references `state.RestoringMarkerName` not a literal.
**Do**:
1. Choose extraction path:
   - Option A: Create `internal/bootstrapadapter/adapters.go` and move the adapters there.
   - Option B: Move `phase5_integration_test.go` into the `cmd` package so it can reference `bootstrap_production.go` types directly.
2. Delete `realRestoringMarker` and `realHookRegistrar` from the phase5 test.
3. Verify any `"@portal-restoring"` literal becomes `state.RestoringMarkerName`.
4. `go test ./cmd/...` to confirm.
**Acceptance Criteria**:
- Phase5 integration test no longer defines its own real adapters
- No hardcoded `"@portal-restoring"` literal in phase5 test code
- Test passes against a real tmux server
**Tests**:
- Phase5 integration test continues to pass

---

## Task 9: Re-query live tmux indices post-creation instead of predicting
status: pending
severity: medium
sources: standards

**Problem**: Spec ("Index Semantics and base-index / pane-base-index") states explicitly: after creating each window/pane, Portal re-queries `list-panes -t <session>` to map saved-structure position → actual live tmux index, used for skeleton markers, FIFO paths, and `--file <scrollback>`. Implementation `buildPaneInfo` (`internal/restore/session.go:114-134,350-368`) instead **predicts** live indices via `baseIdx + wi` and `paneBaseIdx + pj` from `PredictLiveIndices`, then bakes those predictions into the FIFO path and pane-creation command before tmux is even called. `ApplySkeletonMarkers` later re-queries `list-panes` and warns on drift, but markers are still set against live indices while FIFO paths and `--file` were computed from predictions. Under prediction-only, the daemon's `signal-hydrate` will compute FIFO paths from live indices that drift from the predicted-index FIFO paths the helper is blocked on — silent hydration failure.
**Solution**: After `NewSessionWithCommand`/`NewWindow`/`SplitWindow` for each pane, re-query `list-panes -t <session>:<window>` to obtain the actual live (window,pane) tuple; compute FIFO path and skeleton-marker key from those live indices. Restructure `paneInfo` into a two-phase flow: (1) collect saved-position metadata, (2) walk live `list-panes` output to assign FIFOs and skeleton markers under live indices.
**Outcome**: Restore is robust to `base-index`/`pane-base-index` drift between save and restore as the spec mandates; hydration helpers receive FIFO paths matching the live tmux state.
**Do**:
1. In `internal/restore/session.go`, restructure into two phases: phase 1 walks saved structure, creates session/window/pane, captures saved-position metadata only. Phase 2 calls `client.ListPanes(sessionName)` to fetch actual live indices; builds saved-position → live-(window,pane) map.
2. Compute FIFO path using the live indices from the map.
3. Pass the correct `--file <scrollback>` to the helper at pane-creation time (defer the helper command until phase 2, or queue the helper into the FIFO once live indices are known).
4. Set skeleton markers using live indices; remove the prediction-based set.
5. Drop or repurpose `PredictLiveIndices`.
6. Add a regression test that varies `base-index`/`pane-base-index` between save and restore.
**Acceptance Criteria**:
- `buildPaneInfo` (or its replacement) sources FIFO paths and marker keys from a live `list-panes` re-query
- A regression test driving an index drift between save and restore passes
- `signal-hydrate` paths correctly hydrate panes when live indices differ from saved
**Tests**:
- New regression test for `base-index` drift between save and restore
- Existing `internal/restore/` unit and integration tests continue to pass

---

## Task 10: Self-enforce the Restorer interface contract for corrupt-vs-soft errors
status: pending
severity: medium
sources: architecture

**Problem**: `bootstrap.Restorer` (`cmd/bootstrap/bootstrap.go:59-61`) is declared as `Restore() error`, but the orchestrator's behaviour at `cmd/bootstrap/bootstrap.go:160-188` depends on a contract the interface does not express: implementations MUST return errors wrapped with `state.ErrCorruptIndex` for unparseable sessions.json, and MUST return nil for every other soft per-session failure. `internal/restore/restore.go:38-82` honours this informal contract, so the path is unreachable today — but tomorrow a refactor or second implementation could break the assumption silently. `Run` treats any non-corrupt error from `Restore()` as fatal-ish (`fmt.Errorf("step 5 (Restore): %w", restoreErr)` propagates to cobra). The spec's "degrade locally, log, continue" principle says no per-session failure should abort.
**Solution**: Make the contract self-enforcing by either (a) narrowing `Restorer` to `Restore() (corrupt bool, err error)`, or (b) keeping the sentinel but documenting on the interface (and asserting in a contract test) that any other error class is invalid input.
**Outcome**: Orchestrator cannot be silently broken by a future Restorer implementation that returns soft errors; contract is explicit at the type or test level.
**Do**:
1. Choose (a) or (b):
   - Option A: Change `Restorer` to `Restore() (corrupt bool, err error)`. Update `internal/restore/restore.go` to return `(true, err)` only for `state.ErrCorruptIndex`, `(false, nil)` otherwise. Update orchestrator step 5 to branch on the bool.
   - Option B: Add interface doc-comment "Restore() must return either nil or an error wrapping state.ErrCorruptIndex". Add a contract test using a fake Restorer returning a non-corrupt error and assert the orchestrator panics or rejects via a guard.
2. Update orchestrator step 5 logic. The catch-all branch must not surface soft errors as command-aborting returns.
3. Add or update tests for the new contract surface.
**Acceptance Criteria**:
- The Restorer contract is documented or typed so a future impl returning an unrelated error class fails at compile time (option A) or contract-test time (option B)
- Orchestrator step 5 cannot escalate a soft per-session failure to a `PersistentPreRunE` abort
- Existing `cmd/bootstrap/bootstrap_test.go` Restorer scenarios pass
**Tests**:
- New contract test for the chosen option
- Existing orchestrator tests covering corrupt-index and clean-restore paths pass

---

## Task 11: Delete SaverDownError and LastSaverErr triple-encoding
status: pending
severity: low
sources: duplication

**Problem**: Step 4 (EnsureSaver) failure is encoded three times in `cmd/bootstrap/bootstrap.go:80-93,108-114,150-158` and `cmd/bootstrap/errors.go:63-71`: (1) wrapped in `*SaverDownError` and stashed on `Orchestrator.LastSaverErr`; (2) appended to `warnings` as `SaverDownWarning()`; (3) logged via `Logger.Warn`. The doc-comment on `LastSaverErr` admits this is for "backwards compatibility with Phase 5 callers." No production caller reads `LastSaverErr`. The `SaverDownError` wrapping never round-trips to a consumer that calls `errors.As` on it.
**Solution**: Delete `SaverDownError` and the `LastSaverErr` field. The warning append + log line is the consumer surface. Update `bootstrap_test.go:260-261` to assert the warning slice contains `SaverDownWarning()`.
**Outcome**: One encoding of EnsureSaver failure; two stale surfaces removed.
**Do**:
1. Remove the `LastSaverErr` field from `Orchestrator`.
2. Remove `SaverDownError` from `cmd/bootstrap/errors.go`.
3. Update step 4 logic to drop the wrap and the `LastSaverErr =`; keep the warning append and `Logger.Warn`.
4. Update `bootstrap_test.go:260-261` (and any other readers).
5. `go test ./cmd/bootstrap/...` to confirm.
**Acceptance Criteria**:
- `grep -rn "LastSaverErr\|SaverDownError" .` returns no hits
- Tests assert the warning slice surface only
- Step 4 still emits warning + log on failure
**Tests**:
- Updated `bootstrap_test.go` cases assert via `warnings`

---

## Task 12: Table-drive RegisterPortalHooks event categories
status: pending
severity: low
sources: duplication

**Problem**: `internal/tmux/hooks_register.go:109-134`'s `RegisterPortalHooks` contains three identical `for _, event := range <slice>` loops differing only in (slice, substring, command) triple. The unregister side has already factored this idea into `dedupedEventList` + `portalCommandSubstrings`.
**Solution**: Extract `type hookCategory struct { events []string; substring, command string }` and range over a `[]hookCategory` table.
**Outcome**: One loop driven by a table; new categories add a single entry. Note: overlaps Task 2 — if Task 2 chooses path (b), the table has two entries instead of three.
**Do**:
1. Define `type hookCategory struct { events []string; substring, command string }`.
2. Build the table from the existing constants.
3. Replace the three blocks with one outer loop over the table and one inner loop over `cat.events`.
4. Verify error accumulation behaviour preserved.
5. `go test ./internal/tmux/...` to confirm.
**Acceptance Criteria**:
- `RegisterPortalHooks` contains a single registration loop driven by a table
- Existing tests pass without modification
**Tests**:
- Existing `internal/tmux/hooks_register_test.go` continues to pass

---

## Task 13: Replace daemon's hand-rolled index load with state.ReadIndex
status: pending
severity: low
sources: duplication

**Problem**: `cmd/state_daemon.go:240-248` loads the prior structural index by hand: `os.ReadFile(state.SessionsJSON(dir))` → `state.DecodeIndex(data)` → assign to `prevIdx`. `state.ReadIndex` (`internal/state/index_reader.go:36-51`, used by `internal/restore/restore.go:39`) is exactly this composition with proper `ErrCorruptIndex` classification. The daemon's ad-hoc version doesn't distinguish corruption from missing.
**Solution**: Replace the daemon's hand-rolled load with `state.ReadIndex(dir)`; map `(skip=true, err=nil)` to `prevIdx=nil` no-log, `(skip=true, err!=nil)` to `prevIdx=nil` + warn, `(skip=false, _)` to `prevIdx=&idx`.
**Outcome**: Daemon and restore share corrupt-vs-missing classification.
**Do**:
1. Update `cmd/state_daemon.go:240-248` to call `state.ReadIndex(dir)`.
2. Map the three return shapes to the right `prevIdx` and log behaviour.
3. Remove the hand-rolled `os.ReadFile` + `DecodeIndex` lines.
4. `go test ./cmd/...` to confirm.
**Acceptance Criteria**:
- `cmd/state_daemon.go` calls `state.ReadIndex` for the prior index load
- Daemon distinguishes corrupt-index from missing-file in its logs
- Existing daemon tests pass
**Tests**:
- Existing `cmd/state_daemon_test.go` paths covering missing/corrupt prior index continue to pass

---

## Task 14: Hoist surrounding-quote stripping into a shared leaf package
status: pending
severity: low
sources: duplication

**Problem**: `internal/state/markers.go:71-74` strips surrounding double quotes inline (3 lines); `internal/tmux/hooks_parse.go:77-87` exposes `stripMatchedOuterQuotes` which handles both single and double quotes. Both parse `tmux show-*` output families. A future `show-options` value with single-quotes would survive the markers.go strip and break marker prefix logic silently.
**Solution**: Hoist `stripMatchedOuterQuotes` to a shared leaf package (e.g. `internal/tmuxout`) since `internal/state` cannot import `internal/tmux`. Both call sites use it.
**Outcome**: One quote-stripping helper handling both quote styles; no risk of single-quote bypass.
**Do**:
1. Create `internal/tmuxout/strip.go` exporting `StripMatchedOuterQuotes(s string) string`.
2. Move the body from `hooks_parse.go`'s `stripMatchedOuterQuotes`.
3. Update `markers.go:71-74` to call the shared helper.
4. Update `hooks_parse.go` to call the shared helper.
5. Add unit tests covering both quote styles, mismatched quotes, empty strings.
6. `go test ./...` to confirm.
**Acceptance Criteria**:
- `internal/tmuxout/strip.go` is the canonical helper
- `markers.go` no longer has inline quote-stripping
- Both call sites handle both `"…"` and `'…'`
**Tests**:
- Unit tests for `StripMatchedOuterQuotes` covering both quote styles, asymmetric quotes, empty input

---

## Task 15: Hoist xdgConfigBase into a shared leaf package
status: pending
severity: low
sources: duplication

**Problem**: `cmd/config.go:63-72` and `internal/state/paths.go:98-110` both resolve `$XDG_CONFIG_HOME` with `~/.config` fallback. cmd's takes `homeDir` as arg; state's resolves it internally. The state version's comment admits "Duplicated here because the cmd package cannot be imported by internal packages." Drift could silently send Portal config and state to different roots.
**Solution**: Hoist into a shared leaf package both can import (e.g. `internal/xdg`) with signature `func ConfigBase() (string, error)`. Update `cmd/config.go`'s caller to drop the homeDir parameter.
**Outcome**: One canonical `ConfigBase` resolver.
**Do**:
1. Create `internal/xdg/xdg.go` exporting `func ConfigBase() (string, error)`.
2. Update `internal/state/paths.go:98-110` to delegate.
3. Update `cmd/config.go:63-72` to use `xdg.ConfigBase`; drop homeDir parameter.
4. Update callers of the old cmd-side helper.
5. Add unit tests (env set, env unset, env empty).
6. `go test ./...` to confirm.
**Acceptance Criteria**:
- `internal/xdg/xdg.go` is the only XDG resolver
- cmd and state both call `xdg.ConfigBase`
- `grep -rn "XDG_CONFIG_HOME" .` shows only the new helper and tests
**Tests**:
- Unit tests for `xdg.ConfigBase`
- Existing `cmd/` and `internal/state/` tests pass

---

## Task 16: Move noopRunner out of production cmd/root.go
status: pending
severity: low
sources: standards

**Problem**: `cmd/root.go:113-120` defines `noopRunner`, referenced only when `bootstrapDeps != nil` (test mode), but lives in the production file. Test-only fallbacks belong in `_test.go` files.
**Solution**: Move `noopRunner` to `cmd/bootstrap_orchestrator_test.go` or a new `cmd/root_test_helpers.go`. Production code should reference `bootstrap.Runner` only via a non-nil concrete `*bootstrap.Orchestrator`.
**Outcome**: `cmd/root.go` contains no test-only types.
**Do**:
1. Cut `noopRunner` from `cmd/root.go:113-120`.
2. Paste into a `_test.go` file.
3. Verify no production code references `noopRunner`.
4. `go build ./...` and `go test ./cmd/...` to confirm.
**Acceptance Criteria**:
- `grep -rn "noopRunner" cmd/` shows hits only in `_test.go` files
- `cmd/root.go` builds without `noopRunner`
- Tests still pass
**Tests**:
- Existing `cmd/` tests that exercise `bootstrapDeps` injection continue to pass

---

## Task 17: Reconcile log rotation threshold with spec wording (1 MB vs 1 MiB)
status: pending
severity: low
sources: standards

**Problem**: Spec ("Log Rotation") says: "Simple 2-file cap at 1 MB per file." `internal/state/logger.go:43-44` defines `LogRotateThreshold = 1 * 1024 * 1024` (1 MiB) with a comment admitting reinterpretation. ~4.86% drift, non-load-bearing, but a unilateral spec edit.
**Solution**: Either change to `1_000_000` to match the spec literally, or update the spec to read "1 MiB per file".
**Outcome**: Spec wording and constant agree; no apology comment.
**Do**:
1. Choose direction (A: change constant; B: update spec).
2. Apply the chosen change; remove the apology comment.
3. `go test ./internal/state/...` to confirm rotation tests pass at the chosen threshold.
**Acceptance Criteria**:
- Spec wording and `LogRotateThreshold` agree exactly
- The apology comment is gone
- Rotation tests pass
**Tests**:
- Existing `internal/state/logger_test.go` rotation cases pass

---

## Task 18: Relocate AllPaneLister out of misnamed internal/hooks/tmux.go
status: pending
severity: low
sources: architecture

**Problem**: `internal/hooks/tmux.go:1-8` is a 7-line file declaring `AllPaneLister`, with one consumer (`cmd/clean.go`) and no implementation in this package. Filename suggests tmux integration but the file does not import `internal/tmux`. Stranding an interface in its own file muddies package boundaries.
**Solution**: Move `AllPaneLister` to `cmd/clean.go` (sole consumer), or fold it into `internal/hooks/lookup.go` alongside `LookupOnResume`.
**Outcome**: No misleadingly-named one-interface file in `internal/hooks`.
**Do**:
1. Choose target (A: cmd/clean.go; B: internal/hooks/lookup.go).
2. Delete `internal/hooks/tmux.go`.
3. Update imports if needed.
4. `go build ./...` and `go test ./...` to confirm.
**Acceptance Criteria**:
- `internal/hooks/tmux.go` does not exist
- `AllPaneLister` is declared in exactly one file
- `cmd/clean.go` builds cleanly
**Tests**:
- Existing `cmd/clean_test.go` continues to pass

---

## Task 19: Add canonical SetSkeletonMarker / UnsetSkeletonMarker helpers in internal/state
status: pending
severity: low
sources: architecture

**Problem**: `internal/state/markers.go:5-15` declares `SkeletonMarkerPrefix`, but three call sites construct the option name via `SkeletonMarkerPrefix + paneKey` and call `client.SetServerOption`/`UnsetServerOption` directly: `internal/restore/session.go:313-318`, `cmd/state_hydrate.go:189-193`, `cmd/state_hydrate.go:322-325`. The state package owns `ListSkeletonMarkers` (read side) and the prefix constant; the write side has no canonical helper. Asymmetry — read primitive in state, write primitive nowhere.
**Solution**: Add `SetSkeletonMarker(client, paneKey)` and `UnsetSkeletonMarker(client, paneKey)` helpers in `internal/state/markers.go` alongside `ListSkeletonMarkers`. Replace the three direct sites with calls into those helpers.
**Outcome**: Canonical write-side helpers colocated with `ListSkeletonMarkers`.
**Do**:
1. Define a small `ServerOptionWriter` interface in `internal/state/markers.go` (`SetServerOption(name, value string) error`; `UnsetServerOption(name string) error`).
2. Add `SetSkeletonMarker` and `UnsetSkeletonMarker` functions.
3. Update `internal/restore/session.go:313-318` to use `state.SetSkeletonMarker`.
4. Update `cmd/state_hydrate.go:189-193` and `cmd/state_hydrate.go:322-325` to use `state.UnsetSkeletonMarker`.
5. Add unit tests covering set, unset, and key escaping.
6. `go test ./...` to confirm.
**Acceptance Criteria**:
- `state.SetSkeletonMarker` and `state.UnsetSkeletonMarker` are the only writers
- The three former direct call sites use the helpers
- `grep -rn "SkeletonMarkerPrefix +" .` returns no hits outside `markers.go`
**Tests**:
- Unit tests in `internal/state/markers_test.go` for set/unset
- Existing `internal/restore/` and `cmd/state_hydrate_test.go` tests pass

---

## Task 20: If migrate-rename hook is removed (Task 2 path b), collapse the dedupe-category list
status: pending
severity: low
sources: architecture

**Problem**: Conditional on Task 2 choosing path (b). `UnregisterPortalHooks` currently deduplicates a union of three slices because the migrate-rename category was added separately. Removing migrate-rename collapses the union to two; leaving the deduped-three structure carries latent complexity.
**Solution**: After Task 2 path (b), simplify `UnregisterPortalHooks` to iterate the two surviving categories without the deduped-union scaffolding. If Task 2 chooses path (a), this task becomes obsolete.
**Outcome**: Unregister logic matches the post-Task-2 register logic; no residual three-way deduping.
**Do**:
1. Verify Task 2 was applied as path (b). If path (a), close this task as obsolete.
2. Remove `migrateRenameEvents` from the deduped slice union.
3. Simplify the dedupe logic if only two categories remain.
4. Confirm `portalCommandSubstrings` shrinks symmetrically.
5. `go test ./internal/tmux/...` to confirm unregister still cleans every Portal-owned hook.
**Acceptance Criteria**:
- (Conditional on Task 2 path b) `UnregisterPortalHooks` references only two event categories
- No reference to `migrateRenameEvents` / `migrateRenameSubstring` remains
- Unregister tests confirm every Portal-owned hook in both surviving categories is cleaned
**Tests**:
- Existing `internal/tmux/hooks_unregister_test.go` continues to pass
