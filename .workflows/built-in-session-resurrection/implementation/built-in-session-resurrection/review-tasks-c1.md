---
scope: built-in-session-resurrection
cycle: 1
source: review
total_proposed: 14
gate_mode: gated
---
# Review Tasks: built-in-session-resurrection (Cycle 1)

## Task 1: Implement task 5-9 — end-to-end reboot round-trip integration test
status: pending
severity: high
sources: report Required Change 1

**Problem**: `cmd/bootstrap/reboot_roundtrip_test.go` does not exist. The Phase 5 acceptance bullet (planning.md L164) calls for an end-to-end test verifying save → kill-server → restore → attach → hydrate, including ANSI scrollback fidelity and base-index drift. Without this test, the highest-value regression guard for the resurrection workflow is missing.
**Solution**: Author the integration test under `//go:build integration` with `testing.Short()` skip. If full PTY-driven attach is too fragile, fall back to invoking `state.SignalHydrate` directly (the plan documents this as acceptable). Cover at least one base-index-drift configuration (e.g., `base-index 1` while saved state used `base-index 0`).
**Outcome**: A runnable integration test that fails if save/restore/hydrate regresses on structure, layout, zoom, CWDs, environment, hook firing, or ANSI scrollback bytes.
**Do**:
1. Create `cmd/bootstrap/reboot_roundtrip_test.go` with `//go:build integration` tag and `testing.Short()` skip at the top of each test.
2. Spin up an isolated tmux server via `internal/tmuxtest` socket helpers; seed two sessions with multi-window/multi-pane structure including one zoomed pane and one ANSI-coloured scrollback fixture (e.g., `printf '\x1b[31mred\x1b[0m\n'`).
3. Register a resume hook on at least one pane via the `hooks.Store` test seam.
4. Trigger a save tick (force daemon flush or call save path directly); kill the tmux server; restart on the same socket.
5. Run the bootstrap orchestrator via the production wiring with mocked logger; then drive `state.SignalHydrate` to fire the helper exec chain.
6. Assert: session/window/pane indices and names, layout strings, zoom flag, active pane, CWDs, per-session env round-trip, hook fired exactly once, ANSI bytes survive (`tmux capture-pane -e -p` byte-compare against the seeded fixture).
7. Add a sub-test that flips `base-index` between save and restore and asserts the structural-key lookup still resolves.
**Acceptance Criteria**:
- File `cmd/bootstrap/reboot_roundtrip_test.go` exists with `//go:build integration` tag.
- Test fails-loud if any of structure / layout / zoom / CWD / env / hook firing / ANSI scrollback regresses.
- Base-index drift sub-test passes.
- `go test -tags=integration ./cmd/bootstrap/...` passes.
**Tests**:
- The task IS the test; no new sibling tests required.

## Task 2: Implement task 5-10 — `portal attach NAME` / `portal open` reattach integration test
status: pending
severity: high
sources: report Required Change 2

**Problem**: `cmd/reattach_integration_test.go` does not exist. The Phase 5 acceptance bullet (planning.md L166) requires verification that `portal attach NAME` and `portal open` resolve names that exist only in `sessions.json` (i.e., have not yet been restored as live tmux sessions). Five named test cases are absent.
**Solution**: Create the integration test file with the five planned cases. Use the `internal/tmuxtest` socket isolation pattern; seed `sessions.json` with named entries that have no corresponding live tmux session.
**Outcome**: Regression coverage that locks in the `sessions.json`-only resolution contract for `attach` and `open`.
**Do**:
1. Create `cmd/reattach_integration_test.go` with `//go:build integration` tag and `testing.Short()` skip.
2. Re-read planning.md task 5-10 to enumerate the five test cases (e.g., attach-by-name resolves to skeleton; open resolves to skeleton; unknown name errors cleanly; live session shadows saved entry; pruned saved entry no longer resolves).
3. For each case: seed `sessions.json` via `internal/state` test helpers, run the `attach` / `open` cmd via the existing test injection seam (`attachDeps`, `openDeps`), assert the expected tmux switch/attach call and exit status.
4. Use mock connectors so the test does not require a real PTY.
**Acceptance Criteria**:
- File `cmd/reattach_integration_test.go` exists with `//go:build integration` tag.
- All five enumerated cases present and passing.
- `go test -tags=integration ./cmd/...` passes.
**Tests**:
- The task IS the test; no new sibling tests required.

## Task 3: Expand task 5-8 — marker-suppression integration test with non-vacuous probe
status: pending
severity: high
sources: report Required Change 3

**Problem**: The marker-suppression integration test currently asserts an absence (no save advances during the marker window) without proving the probe path is actually exercised — making it vacuously true if the probe never fires.
**Solution**: Register a probe `set-hook -ga` before the orchestrator runs that records structural events to a tempfile during the marker window. Assert at least one event is recorded (proving the test is non-vacuous) AND that `sessions.json.saved_at` is not advanced.
**Outcome**: The marker-suppression contract is locked by a test that cannot pass when the probe is silent.
**Do**:
1. Open `cmd/bootstrap/phase5_integration_test.go` (or wherever 5-8 lives); add the `//go:build integration` tag and `testing.Short()` skip if not already present.
2. Before orchestrator startup, install a probe via `tmux set-hook -ga session-created 'run-shell "echo $(date +%s) >> <tempfile>"'` (or similar structural hook) using the test's tmux socket.
3. Run the orchestrator through the marker-set → restore → marker-clear window with at least one structural event guaranteed to fire (e.g., create a session inside the window).
4. Assert tempfile is non-empty (probe fired) AND `sessions.json.saved_at` did not advance during the window.
5. Clean up the global hook in `t.Cleanup`.
**Acceptance Criteria**:
- Test fails when probe records zero events (non-vacuity guard).
- Test fails when `sessions.json.saved_at` advances during the marker window.
- `//go:build integration` tag and `testing.Short()` skip present.
**Tests**:
- The task IS the test expansion; no new sibling tests required.

## Task 4: Expand task 3-13 — Phase 3 integration test multi-session/ANSI/marker coverage
status: pending
severity: high
sources: report Required Change 4

**Problem**: The shipped Phase 3 integration test covers only a single-session, no-ANSI, no-marker-clearance smoke. The plan acceptance at planning.md L96 calls for multi-session × multi-window × multi-pane round-trip including zoom, ANSI byte-compare, env round-trip, active-pane round-trip, and marker clearance.
**Solution**: Expand the existing Phase 3 integration test fixture and assertions to the full plan acceptance scope.
**Outcome**: Phase 3 round-trip coverage matches the plan acceptance bullet.
**Do**:
1. Locate the existing Phase 3 integration test (likely under `internal/state` or `internal/restore`).
2. Replace the single-session fixture with at least two sessions, each with two windows, each with two panes; mark one pane zoomed.
3. Seed each pane's scrollback with ANSI SGR sequences; capture the seeded bytes for later byte-compare.
4. Set per-session environment variables via `set-environment -t SESSION KEY=value`.
5. Run the save → kill → restore round-trip; then assert: structure equality, layout equality, zoom flag preserved, active pane preserved, env round-trip, ANSI bytes preserved (byte-compare), and `@portal-restoring` cleared after `signal-hydrate` + helper dump.
**Acceptance Criteria**:
- Fixture has ≥2 sessions, ≥2 windows/session, ≥2 panes/window, ≥1 zoomed pane.
- ANSI bytes byte-equal pre/post round-trip.
- Per-session env survives round-trip.
- Active-pane preserved per session.
- `@portal-restoring` marker cleared after hydrate dump.
**Tests**:
- The task IS the test expansion.

## Task 5: Fix task 6-2 logger migration in `state_migrate_rename` and `state_notify`
status: pending
severity: high
sources: report Required Change 5 (parts a + b)

**Problem**: `cmd/state_migrate_rename.go` and `cmd/state_notify.go` still call `fmt.Fprintf(os.Stderr, …)` instead of opening a `*state.Logger`. Both subsystems are invisible to `portal.log` and to `portal state status` recent-warnings scanning, contradicting the spec § Observability contract.
**Solution**: Open a `*state.Logger` in each file with the appropriate component tag; replace stderr writes with `Logger.Warn` / `Logger.Info`. Add a WARN on file-create failure in `state_notify.go` per plan.
**Outcome**: Both subsystems contribute to `portal.log` and surface in `portal state status` recent-warnings scans.
**Do**:
1. In `cmd/state_migrate_rename.go`: open `*state.Logger` with `state.ComponentHooks`; replace each `fmt.Fprintf(os.Stderr, …)` with the matching `Logger.Warn` / `Logger.Info` call; defer `logger.Close()`.
2. In `cmd/state_notify.go`: open `*state.Logger` with `state.ComponentNotify`; replace stderr writes with logger calls; on file-create failure emit `Logger.Warn`; defer `logger.Close()`.
3. Verify component constants exist; if `ComponentNotify` does not exist add it to `internal/state/logger.go`.
**Acceptance Criteria**:
- No `fmt.Fprintf(os.Stderr, …)` remains in the two files for routine reporting (only fatal pre-logger errors permitted).
- Each subsystem emits to `portal.log` under its component tag.
- File-create failure in `state_notify.go` emits a WARN line.
**Tests**:
- Unit test asserting `state_migrate_rename` emits expected component lines (use a `*state.Logger` writing to a tempfile and read back).
- Unit test asserting `state_notify` emits a WARN on a forced file-create failure (e.g., directory pre-created as a regular file).

## Task 6: Add `Debug` method to bootstrap `Logger` interface and emit step-entry DEBUG lines
status: pending
severity: high
sources: report Required Change 5 (part c)

**Problem**: The `cmd/bootstrap/bootstrap.go` `Logger` interface lacks `Debug(component, format string, args ...any)`. The spec § Observability requires "Bootstrap events at DEBUG level only", but the orchestrator cannot emit at DEBUG without the method.
**Solution**: Add `Debug` to the interface; implement it in production logger and in `noopLogger`; emit a DEBUG line on each orchestrator step entry.
**Outcome**: Bootstrap step-entry visibility at DEBUG level matches the spec contract.
**Do**:
1. Add `Debug(component, format string, args ...any)` to the `Logger` interface in `cmd/bootstrap/bootstrap.go`.
2. Implement `Debug` on `noopLogger` (no-op) and confirm `*state.Logger` already has a matching `Debug` method (add if missing).
3. In the orchestrator `Run` switch, emit `logger.Debug(state.ComponentBootstrap, "step %s: entering", stepName)` (or equivalent) at the entry of each step.
4. Update any test doubles (e.g., `mockLogger`) to satisfy the new interface method.
**Acceptance Criteria**:
- Interface change compiles across all callers.
- Each orchestrator step emits one DEBUG line on entry.
- Existing logger tests still pass; one new test asserts the DEBUG line is emitted per step.
**Tests**:
- Unit test that runs the orchestrator with a recording logger and asserts ≥1 DEBUG line per executed step.

## Task 7: Defer `logger.Close()` in `state_signal_hydrate` and `state_hydrate` RunE
status: pending
severity: medium
sources: report Required Change 5 (part d), Bug 45

**Problem**: `cmd/state_signal_hydrate.go:149` and `cmd/state_hydrate.go:360` open a `*state.Logger` but do not defer `Close()`. Production paths exec away (acceptable), but interrupted execution and tests retain leaked file descriptors.
**Solution**: Add `defer logger.Close()` immediately after each logger open in the `RunE` body.
**Outcome**: Test runs and interrupted invocations no longer leak the log fd.
**Do**:
1. Read both files; locate the logger-open call in each `RunE`.
2. Insert `defer logger.Close()` immediately after the successful open.
3. Verify `state.Logger.Close()` is idempotent / safe on a logger that has already exec'd away (it is, since the exec'd process inherits a duplicated fd that the OS closes on process replacement).
**Acceptance Criteria**:
- Both files have `defer logger.Close()` after the logger open in `RunE`.
- `go vet ./...` and `go test ./...` pass.
**Tests**:
- Unit test that invokes `RunE` (without exec) on each command and asserts the logger fd is closed on RunE return (via tempfile + `lsof`-style check or by re-opening for write).

## Task 8: Wrap permission errors in `ReadIndex` with `ErrCorruptIndex`
status: pending
severity: high
sources: report Required Change 6

**Problem**: `internal/state/index_reader.go:42` does not wrap permission read-errors with `ErrCorruptIndex`. Downstream classifiers in Phase 5 task 5-2 and Phase 6 task 6-9 use `errors.Is(err, ErrCorruptIndex)` to bucket soft warnings; permission errors currently miss this bucket and risk being escalated.
**Solution**: Wrap the read-error branch return with `ErrCorruptIndex` using `fmt.Errorf("%w: %v", ErrCorruptIndex, err)`. Add a unit test covering the permission-denied case.
**Outcome**: Permission errors classify as soft warnings via `errors.Is(err, ErrCorruptIndex)` at every consumer site.
**Do**:
1. Open `internal/state/index_reader.go`; locate line ~42 read-error branch.
2. Change `return …, err` to `return …, fmt.Errorf("%w: read sessions.json: %v", ErrCorruptIndex, err)` (preserving any existing return tuple shape).
3. Add a test in `internal/state/index_reader_test.go` that creates a `sessions.json` with mode 0000 (in a tempdir), calls `ReadIndex`, and asserts `errors.Is(err, ErrCorruptIndex)`.
4. Skip the test on Windows (chmod semantics) and when running as root (no permission denial).
**Acceptance Criteria**:
- Permission-denied read returns an error that satisfies `errors.Is(err, ErrCorruptIndex)`.
- New unit test passes locally and in CI on macOS/Linux.
- Existing `ReadIndex` tests continue to pass.
**Tests**:
- New test: `TestReadIndex_PermissionDeniedWrapsErrCorruptIndex` asserting `errors.Is`.

## Task 9: Reconcile plan body wording for task 5-2 `Restoring.Clear` failure classification
status: pending
severity: low
sources: report Required Change 7

**Problem**: Plan body for task 5-2 says `Restoring.Clear` failure is "soft + WARN", but spec § Fatal Bootstrap Errors, CLAUDE.md, and the implementing test all say fatal. Implementation correctly follows spec; the plan body wording is the outlier.
**Solution**: Edit the planning markdown for task 5-2 to state "fatal" and cite the spec section, so future reviewers do not flag this as drift.
**Outcome**: Plan documentation aligns with implementation and spec.
**Do**:
1. Open `.workflows/built-in-session-resurrection/planning/.../planning.md` (or wherever task 5-2 body lives).
2. Locate the "soft + WARN" wording for `Restoring.Clear` failure handling.
3. Change to "fatal — see spec § Fatal Bootstrap Errors and CLAUDE.md bootstrap step 6".
4. Add a one-line note that this resolves cycle-1 review finding.
**Acceptance Criteria**:
- Plan body for task 5-2 says "fatal" for `Restoring.Clear` failure.
- Reference to spec section included.
**Tests**:
- N/A (documentation-only change).

## Task 10: Quick-fix — remove stale `migrate-rename` comment in `hooks_register_test.go`
status: pending
severity: low
sources: report Quick-fix 1

**Problem**: `internal/tmux/hooks_register_test.go:545-547` references "the migrate-rename call" that no longer exists after task 7-2 path (b) deferred the hook to v2.
**Solution**: Remove or update the stale comment to reflect the current 9-hook contract.
**Outcome**: Test comment matches current behavior and stops misleading future readers.
**Do**:
1. Open `internal/tmux/hooks_register_test.go` at L545-547.
2. Delete the stale reference or replace with current accurate description.
**Acceptance Criteria**:
- Comment accurately reflects current hook registration set.
- `go test ./internal/tmux/...` passes.
**Tests**:
- N/A (comment-only change).

## Task 11: Quick-fix — restore symmetry in `internal/state/logger.go` rename-failure reopen branch
status: pending
severity: low
sources: report Quick-fix 5

**Problem**: `internal/state/logger.go:184-188` swallows a reopen error silently after rotate-rename failure, while the parallel branch emits a diagnostic. Asymmetric error handling makes a real failure invisible.
**Solution**: Add the matching diagnostic emission to the silent branch.
**Outcome**: Both rotate-rename failure paths surface diagnostics consistently.
**Do**:
1. Open `internal/state/logger.go` at L184-188.
2. Identify the parallel branch's diagnostic call (likely a `fmt.Fprintf(os.Stderr, …)` or self-log).
3. Mirror that diagnostic in the silent branch.
**Acceptance Criteria**:
- Both branches emit equivalent diagnostics on reopen failure.
- Logger tests still pass.
**Tests**:
- Add a test that forces a reopen failure on the silent path and asserts the diagnostic is emitted (mirroring the parallel-branch test if present).

## Task 12: Quick-fixes — schema/scrollback polish (version diagnostic + xxhash allocation)
status: pending
severity: low
sources: report Quick-fix 9, Quick-fix 10

**Problem**: Two small polish items: (a) `internal/state/schema.go:109` "unsupported sessions.json version" error omits the expected version, hampering diagnostics; (b) `internal/state/scrollback.go` uses `xxhash.Sum64([]byte(out))` which allocates a copy when `xxhash.Sum64String(out)` would not.
**Solution**: Add `(current: %d)` to the schema error; switch to `xxhash.Sum64String`.
**Outcome**: Better diagnostic and one less allocation per scrollback hash.
**Do**:
1. Open `internal/state/schema.go:109`; extend the error format with `(current: %d)` substituting the supported version.
2. Open `internal/state/scrollback.go`; locate `xxhash.Sum64([]byte(out))`; change to `xxhash.Sum64String(out)`.
**Acceptance Criteria**:
- Schema error string includes the supported-version number.
- Scrollback hash uses `Sum64String`.
- Existing tests pass.
**Tests**:
- Update any test that pins the schema error string verbatim.

## Task 13: Idea — relax or remove `purgeStateDir` `EvalSymlinks` rejection
status: pending
severity: medium
sources: report Idea 19, Bug 46

**Problem**: `cmd/state_cleanup.go:141-148` rejects valid resolved-path purges when intermediate path components are symlinks (e.g., `~/.config` symlinked, macOS `~/Library`). Real users on symlinked-config setups see false-positive "refusing to purge" and must clean up manually. The test suite hides this via `canonicalTempDir`.
**Solution**: Drop the `EvalSymlinks` strict-equality check, or relax it to leaf-symlink only (which `Lstat` already catches). Add a regression test using a tempdir with a symlinked intermediate component.
**Outcome**: Users with symlinked config paths can `portal state purge` without manual fallback.
**Do**:
1. Open `cmd/state_cleanup.go:141-148`; review the `EvalSymlinks` comparison.
2. Decide between (a) dropping the check entirely, relying on `Lstat` for leaf-symlink protection, or (b) restricting the check to the leaf path component only. Prefer (a) unless leaf protection is load-bearing.
3. Add a unit test creating a tempdir with a symlinked intermediate (e.g., `tmpdir/link → realdir`, then call purge on `tmpdir/link/state`) and assert success.
4. Update or remove the `canonicalTempDir` shim if no longer needed.
**Acceptance Criteria**:
- `purgeStateDir` succeeds when intermediate path components are symlinks.
- Leaf-symlink protection (if retained) still blocks purge of an entire symlinked target dir.
- New regression test passes.
**Tests**:
- New test exercising symlinked-intermediate purge.

## Task 14: Idea — add binary-missing and projects.json-absence regression tests in `cmd/clean.go`
status: pending
severity: low
sources: report Idea 27

**Problem**: The plan called for two spec-pinned regression tests in `cmd/clean.go`: (i) binary-missing must NOT be treated as a staleness signal; (ii) `projects.json` absence must NOT be treated as a staleness signal. Neither test was added.
**Solution**: Add both table-driven tests using existing `cmd/clean_test.go` patterns (or create a new sibling file).
**Outcome**: Future regressions in cleanup-staleness classification are caught at unit level.
**Do**:
1. Read `cmd/clean.go` to identify the staleness-classification call sites.
2. Add a test case where the binary path resolved by `clean` is absent on disk and assert no staleness action is taken.
3. Add a test case where `projects.json` is absent and assert no staleness action is taken.
4. Use the existing `cleanDeps` injection seam if present; otherwise mock the file-stat call directly.
**Acceptance Criteria**:
- Two new test cases added under `cmd/clean_test.go` (or sibling file).
- Both fail-loud if staleness classification regresses.
- `go test ./cmd -run TestClean` passes.
**Tests**:
- The task IS the test addition.
