---
topic: built-in-session-resurrection
cycle: 3
total_proposed: 9
---
# Analysis Tasks: built-in-session-resurrection (Cycle 3)

## Task 1: Consolidate `restoreOrchestratorAdapter` and split out FIFO sweep
status: approved
severity: high
sources: duplication, architecture

**Problem**: Two files declare a type literally named `restoreOrchestratorAdapter` whose sole job is to wrap `*restore.Orchestrator` so its `Restore()` satisfies `bootstrap.Restorer` (`cmd/bootstrap_production.go:48-78` and `cmd/bootstrap/phase5_integration_test.go:71-82`). The production version additionally runs `state.SweepOrphanFIFOs` after Restore — bolting a separate post-restore cleanup into the Restore adapter and leaving the test-side adapter unable to exercise the sweep. This conflates step 5 (Restore) with what is conceptually a separate post-restore cleanup, makes the orchestrator's eight-step doc-comment under-describe production wiring, and forces any change to the Restorer contract or marker lifecycle to be applied in two parallel adapter definitions. Naming is also asymmetric — `internal/bootstrapadapter` types use Pascal-case while cmd-side adapters use camelCase.

**Solution**: Move a sweep-free `RestoreAdapter` (Pascal-cased) into `internal/bootstrapadapter` next to `RestoringMarker` and `HookRegistrar`, so the integration test imports it directly and deletes its local twin. Keep the FIFO sweep as a separate, narrow decorator (or its own bootstrap step) in cmd/, named explicitly so the step boundary is visible. Either promote the FIFO sweep to its own `FIFOSweeper` step interface alongside `StaleCleaner`, or — if the eight-step contract is preserved — keep the sweep as a small `fifoSweepingRestorer` decorator wrapping the bootstrapadapter.RestoreAdapter and add a leading-comment note in `cmd/bootstrap/bootstrap.go` explicitly calling out that step 5 includes the orphan-FIFO sweep on the production wiring.

**Outcome**: One canonical `RestoreAdapter` type lives in `internal/bootstrapadapter`. The integration test imports it instead of redeclaring a twin. The FIFO sweep is either its own step (preferred) with its own `FIFOSweeper` interface and test surface, or a clearly-named decorator in cmd/ accompanied by a doc-comment update on the orchestrator. Naming across `internal/bootstrapadapter` and any cmd-side adapter siblings is consistent (Pascal-case for the moved adapter; cmd-side naming convention documented in `cmd/bootstrap_production.go`'s leading comment).

**Do**:
1. Add `RestoreAdapter` to `internal/bootstrapadapter/adapters.go` (or a new file in that package) wrapping `*restore.Orchestrator` so its `Restore()` returns `(corrupt bool, err error)` matching `bootstrap.Restorer`.
2. Update `cmd/bootstrap/phase5_integration_test.go` to import and use `bootstrapadapter.RestoreAdapter`; delete the local `restoreOrchestratorAdapter`.
3. In `cmd/bootstrap_production.go`, replace the local `restoreOrchestratorAdapter` with composition: build `bootstrapadapter.RestoreAdapter`, then wrap it.
4. Decide between (a) promoting the FIFO sweep to its own bootstrap step (preferred): add a `FIFOSweeper` interface to `cmd/bootstrap/bootstrap.go`, wire it in the orchestrator after Restore (or alongside StaleCleaner), update the eight-step doc-comment to nine steps, add a `FIFOSweeperAdapter` in cmd/bootstrap_production.go calling `state.SweepOrphanFIFOs`, and update `cmd/bootstrap/phase5_integration_test.go` to exercise it; or (b) keeping the sweep as a `fifoSweepingRestorer` decorator wrapping `bootstrapadapter.RestoreAdapter` and updating `cmd/bootstrap/bootstrap.go`'s leading comment to note that "step 5 (Restore) includes the orphan-FIFO sweep on the production wiring."
5. Add a leading-comment line to `cmd/bootstrap_production.go` documenting the cmd-package adapter naming convention.
6. Run `go build -o portal .` and `go test ./...`.

**Acceptance Criteria**:
- Exactly one `RestoreAdapter` type definition exists for the production-shape adapter, in `internal/bootstrapadapter`.
- `cmd/bootstrap/phase5_integration_test.go` imports the bootstrapadapter type and contains no local `restoreOrchestratorAdapter` declaration.
- The FIFO sweep is either a discrete bootstrap step (preferred) with its own interface, adapter, and orchestrator wiring, or a clearly-named decorator with an updated leading-comment in `cmd/bootstrap/bootstrap.go`.
- Naming convention rationale for cmd-side vs `internal/bootstrapadapter` adapters is explicitly documented in `cmd/bootstrap_production.go`'s leading comment.
- `go build -o portal .` and `go test ./...` pass.

**Tests**:
- Existing `cmd/bootstrap/phase5_integration_test.go` continues to pass against the imported `bootstrapadapter.RestoreAdapter`.
- If option (a) is taken: a phase-test for the new `FIFOSweeper` step exercising the sweep behaviour (success, swallowed-error, no-orphans).
- If option (b) is taken: a unit test (or amended phase test) confirming the decorator runs `state.SweepOrphanFIFOs` after the inner Restore, and propagates the inner Restore's `(corrupt, err)` unchanged.

---

## Task 2: Eliminate `socketCommander.Run`/`RunRaw` duplication of `tmux.RealCommander`
status: approved
severity: high
sources: duplication

**Problem**: `internal/tmuxtest/socket.go:119-135` defines `socketCommander.Run` and `socketCommander.RunRaw` that are byte-identical to `tmux.RealCommander.Run`/`RunRaw` (`internal/tmux/tmux.go:39-58`) apart from the `socketArgs(...)` argv-prefix transformation. Both build `exec.Command("tmux", ...)`, call `.Output()`, return `("", err)` on failure, then either trim+stringify (Run) or stringify verbatim (RunRaw). Adding a third command shape — context cancellation, env injection, capture-stderr-on-failure — requires touching all four method bodies. The existing doc-comments on the test methods explicitly acknowledge this drift surface.

**Solution**: Adopt the minimal-blast-radius option: add a small private helper in `internal/tmuxtest` (e.g. `runRaw(args []string) ([]byte, error)`) that handles the `exec.Command("tmux", ...).Output()` call and error-path. `Run` and `RunRaw` then delegate to it, differing only in the trim step.

**Outcome**: `socketCommander.Run` and `RunRaw` collapse to two-line delegations onto a shared helper. Future changes to command-execution semantics live in one place per package. The drift-acknowledgement doc-comments can be removed.

**Do**:
1. Add a private helper `func (c *socketCommander) runRaw(args []string) ([]byte, error)` in `internal/tmuxtest/socket.go`.
2. Rewrite `socketCommander.Run` to call the helper, then `strings.TrimSpace(string(out))` on success.
3. Rewrite `socketCommander.RunRaw` to call the helper, then `string(out)` on success.
4. Drop the "matching tmux.RealCommander.Run/RunRaw" drift-acknowledgement doc-comments.
5. Run `go test ./internal/tmuxtest/... ./internal/tmux/... ./cmd/...`.

**Acceptance Criteria**:
- `socketCommander.Run` and `socketCommander.RunRaw` each contain at most ~3 lines of logic, with the shared `exec.Command("tmux", ...).Output()` call factored into a private helper.
- The drift-acknowledgement doc-comments on the two methods are removed.
- All tests that use `socketCommander` still pass.

**Tests**:
- Existing tests covering `socketCommander.Run` and `RunRaw` provide regression coverage.

---

## Task 3: Fix stale `createSkeleton` doc-comment claiming send-keys arming
status: approved
severity: medium
sources: standards

**Problem**: `internal/restore/session.go:118-119` ends with "Panes are created with no initial command — they default to the user's shell — so that the arm phase can dispatch the hydrate helper via `send-keys` against live indices." This is a fossil from the pre-T7-9 design. The actual arm phase uses `respawn-pane -k` (correctly described elsewhere in the same file at lines 7, 41, 167-174, 476-477). The spec was rewritten in T8-3 to make respawn-pane canonical. This single hold-out comment misleads any reader trusting it as design intent.

**Solution**: Replace the stale `send-keys` clause with respawn-pane wording.

**Outcome**: `internal/restore/session.go:118-119`'s doc-comment correctly describes the arm mechanism, aligned with the other references in the same file and with the spec.

**Do**:
1. Open `internal/restore/session.go` and locate the doc-comment on `createSkeleton` at lines 118-119.
2. Replace "...so that the arm phase can dispatch the hydrate helper via `send-keys` against live indices." with "...so that the arm phase can dispatch the hydrate helper via `respawn-pane -k` against live indices."
3. Run `go build -o portal .` and `go test ./internal/restore/...`.

**Acceptance Criteria**:
- The `createSkeleton` doc-comment in `internal/restore/session.go` no longer references `send-keys`.
- The doc-comment references `respawn-pane -k` consistently with the rest of the file.
- `go build -o portal .` and `go test ./internal/restore/...` pass.

**Tests**:
- No new tests; this is a comment-only change.

---

## Task 4: Extract `openAppendLog` helper for OpenFile triplet in logger.go
status: approved
severity: medium
sources: duplication

**Problem**: `internal/state/logger.go:90,179,184` repeats `os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)` three times — once in `OpenLogger` and twice in `maybeRotate`. Within-file rule-of-three breach.

**Solution**: Extract a small private helper `func openAppendLog(path string) (*os.File, error)` in `logger.go`.

**Outcome**: All three sites call `openAppendLog(path)`. The OpenFile flag/mode constant lives in one place.

**Do**:
1. Add a private helper `func openAppendLog(path string) (*os.File, error) { return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) }`.
2. Replace the three call sites at lines 90, 179, and 184.
3. Run `go test ./internal/state/...`.

**Acceptance Criteria**:
- A single private `openAppendLog` helper exists in `internal/state/logger.go`.
- All three previous OpenFile call sites call the helper.
- `go test ./internal/state/...` passes.

**Tests**:
- Existing logger tests provide regression coverage.

---

## Task 5: Add `PaneTarget` helper for `%s:%d.%d` Sprintf duplication
status: approved
severity: medium
sources: duplication

**Problem**: The literal format `"%s:%d.%d"` with `(session, window, pane)` recurs five times across four files: `internal/tmux/tmux.go:591,605` (`SelectPane`, `ResizePaneZoom`), `internal/restore/session.go:108,220` (hookKey, liveTarget), and `cmd/state_daemon.go:134` (`captureAndCommit`). `internal/tmux.PaneCoord` already exists for the inverse parse.

**Solution**: Add `func PaneTarget(session string, window, pane int) string` in `internal/tmux/tmux.go` next to `SanitizePaneKey`/`PaneCoord`. Replace all five call sites.

**Outcome**: One canonical formatter for the `session:window.pane` target string.

**Do**:
1. Add `func PaneTarget(session string, window, pane int) string { return fmt.Sprintf("%s:%d.%d", session, window, pane) }` to `internal/tmux/tmux.go` near `SanitizePaneKey` / `PaneCoord`.
2. Replace the two `*tmux.Client` call sites in `internal/tmux/tmux.go:591,605`.
3. Replace the two call sites in `internal/restore/session.go:108,220`.
4. Replace the call site in `cmd/state_daemon.go:134`.
5. Leave `SelectLayout` at `tmux.go:577` (`%s:%d` form) unchanged unless a sibling helper is natural.
6. Run `go build -o portal .` and `go test ./...`.

**Acceptance Criteria**:
- A single `PaneTarget` helper exists in `internal/tmux`.
- All five call sites listed above call the helper.
- `go build -o portal .` and `go test ./...` pass.

**Tests**:
- Add a small unit test for `PaneTarget`.
- Existing tests provide regression coverage.

---

## Task 6: Add `tmux.DefaultClient()` helper for `NewClient(&RealCommander{})` repetition
status: approved
severity: medium
sources: duplication

**Problem**: `tmux.NewClient(&tmux.RealCommander{})` appears seven times in `cmd/`, five of them in resurrection-introduced files: `cmd/bootstrap_production.go:116`, `cmd/state_cleanup.go:49`, `cmd/state_daemon.go:251`, `cmd/state_signal_hydrate.go:154`, `cmd/state_hydrate.go:367`; plus `cmd/clean.go:33`, `cmd/hooks.go:41` (pre-existing).

**Solution**: Add a small package-level helper `func DefaultClient() *Client { return NewClient(&RealCommander{}) }` in `internal/tmux/tmux.go`. Replace all seven open-coded constructions.

**Outcome**: Production-client construction has one entry point.

**Do**:
1. Add `func DefaultClient() *Client { return NewClient(&RealCommander{}) }` to `internal/tmux/tmux.go`.
2. Replace the seven call sites listed above.
3. Run `go build -o portal .` and `go test ./...`.

**Acceptance Criteria**:
- `tmux.DefaultClient()` exists in `internal/tmux/tmux.go`.
- All seven previous open-coded call sites use the helper.
- `go build -o portal .` and `go test ./...` pass.

**Tests**:
- Existing tests provide regression coverage.

---

## Task 7: Delete orphaned `NoOpServer` and `NoOpRestoringMarker`
status: approved
severity: low
sources: architecture

**Problem**: `cmd/bootstrap/noop.go` exports six NoOp* step types. Four (`NoOpHooks`, `NoOpSaver`, `NoOpRestorer`, `NoOpStaleCleaner`) earn their keep. `NoOpServer` (`:21-24`) and `NoOpRestoringMarker` (`:33-41`) have zero references outside their own declarations. The spec mandates Server and RestoringMarker as fatal-on-failure (steps 1, 3, 6), so they have no legitimate degradation path. Keeping them invites tests/code to reach for "a default" that violates the bootstrap contract.

**Solution**: Delete `NoOpServer` and `NoOpRestoringMarker`. Update the file's leading comment to note that NoOp implementations exist only for the four steps the spec permits to degrade-and-continue.

**Outcome**: Only NoOp implementations for legitimately-degradable steps remain.

**Do**:
1. Delete `NoOpServer` and `NoOpRestoringMarker` from `cmd/bootstrap/noop.go`.
2. Update the file's leading comment to state explicitly: "NoOp implementations exist only for the four steps the spec permits to degrade-and-continue (Hooks, Saver, Restore, StaleCleaner). Server and RestoringMarker are fatal-on-failure and intentionally have no NoOp."
3. Run `go build -o portal .` and `go test ./...`.

**Acceptance Criteria**:
- `cmd/bootstrap/noop.go` contains only `NoOpHooks`, `NoOpSaver`, `NoOpRestorer`, `NoOpStaleCleaner`.
- The file's leading comment explains the policy.
- `go build -o portal .` and `go test ./...` pass.

**Tests**:
- Existing tests provide regression coverage.

---

## Task 8: Resolve ambiguous `warnOnPaneKeyDrift` seam between Orchestrator and SessionRestorer
status: approved
severity: low
sources: architecture

**Problem**: `Orchestrator.warnOnPaneKeyDrift` (`internal/restore/restore.go:153-167`) iterates predicted-vs-live keys and dispatches to `SessionRestorer.warnOnPaneKeyDrift` (`internal/restore/session.go:407-412`) for the per-pane WARN. Same name on adjacent types reads as recursive at the call site. Responsibility is fragmented — predicted-key computation lives on the orchestrator, but the actual `Logger.Warn` call lives on `SessionRestorer` purely so it can reach `r.Logger`. Additionally, `flattenSavedPanePositions` and `savedPanePos` (`internal/restore/session.go:372-391`) are now consumed by exactly one caller (`Orchestrator.warnOnPaneKeyDrift` in restore.go) yet remain in session.go — its package doc says "create + geometry + skeleton-markers", which saved-position flattening isn't.

**Solution**: Emit the WARN directly from `Orchestrator.warnOnPaneKeyDrift` using `o.Logger.Warn` (the orchestrator owns the drift concern post-T8-6) and delete the `SessionRestorer.warnOnPaneKeyDrift` helper. Co-locate `savedPanePos` and `flattenSavedPanePositions` with their sole consumer by moving them from `session.go` to `restore.go`.

**Outcome**: Drift-detection machinery lives entirely in `restore.go`. No two methods in the package share a name.

**Do**:
1. Modify `Orchestrator.warnOnPaneKeyDrift` to emit the per-pane WARN directly via `o.Logger.Warn(...)`.
2. Delete `SessionRestorer.warnOnPaneKeyDrift` from `internal/restore/session.go`.
3. Move `savedPanePos` and `flattenSavedPanePositions` from `session.go` to `restore.go`, adjacent to `Orchestrator.warnOnPaneKeyDrift`.
4. Search for callers (`grep`) to confirm no other references.
5. Run `go build -o portal .` and `go test ./internal/restore/...`.

**Acceptance Criteria**:
- `internal/restore/session.go` no longer contains `warnOnPaneKeyDrift`, `savedPanePos`, or `flattenSavedPanePositions`.
- `internal/restore/restore.go` contains the drift-detection machinery co-located.
- The orchestrator emits WARN logs directly via its own logger.
- `go build -o portal .` and `go test ./internal/restore/...` pass.

**Tests**:
- Existing tests covering drift detection provide regression coverage.

---

## Task 9: Align spec text with current implementation (mkfifo ordering, hydrate helper ordering, CreateFIFO semantics)
status: approved
severity: low
sources: standards

**Problem**: Three spec/implementation alignment issues:
1. Spec L1031-1038 lists "1. mkfifo" before "2. new-session" / "4. new-window" / "5. arm with respawn-pane -k" — pre-T7-9 ordering. Implementation correctly mkfifos inside `armPanes` (`internal/restore/session.go:215`). Spec L767 ("before creating the pane") is another stale fragment.
2. Spec L796-800 lists e (settle sleep) → f (lookup) → g (unset marker) → h (exec). Code does e (sleep) → unset marker → lookup-and-exec.
3. Spec L1032 says "os.Remove(path) (ignore ENOENT); syscall.Mkfifo(path, 0600)". `state.CreateFIFO` (`internal/state/fifo.go:32-41`) actually removes any pre-existing inode and chmod's post-mkfifo to defend against a tight umask.

**Solution**: Update the spec text in three small edits to match what the implementation actually does. Spec-text changes only.

**Outcome**: Spec describes the implementation's actual behaviour.

**Do**:
1. Edit Bootstrap Flow step 5's per-session sub-steps (L1031-1038): move sub-step 1 (mkfifo) so it sits between sub-step 4 (new-window/split-window) and sub-step 5 (arm with respawn-pane), or rephrase sub-step 5 to call out FIFO creation as a precondition the arm phase performs per-pane.
2. Edit L767's "before creating the pane" parenthetical to "before arming the pane".
3. Edit "Helper Behavior on Startup" L796-800: reorder to match code, or add an explicit independence acknowledgement.
4. Edit L1032's mkfifo description to "remove any existing inode (regular file, symlink, FIFO) before mkfifo, then chmod 0600 to defend against an unusually-tight umask."
5. Re-read the four edited sections together for internal consistency.

**Acceptance Criteria**:
- Spec L1031-1038 places mkfifo between pane creation and arming.
- Spec L767 says "before arming the pane".
- Spec L796-800 either matches code ordering or adds an explicit independence acknowledgement.
- Spec L1032 documents the replace-any-inode and post-mkfifo chmod behaviour.
- No code changes — `go build -o portal .` and `go test ./...` should remain unaffected.

**Tests**:
- No code tests; this is a spec-text change.
- Manual verification against `internal/restore/session.go:215`, `cmd/state_hydrate.go:171-184`, and `internal/state/fifo.go:32-41`.
