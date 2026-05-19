---
topic: saver-kill-respawn-loop-leaks-daemons
cycle: 1
total_proposed: 6
---
# Analysis Tasks: saver-kill-respawn-loop-leaks-daemons (Cycle 1)

## Task 1: Collapse shouldKillSaverOnVersionDecision + portalSaverVersionMismatch into a single predicate
status: pending
severity: high
sources: duplication, architecture

**Problem**: `shouldKillSaverOnVersionDecision` (portal_saver.go:334-358) and `portalSaverVersionMismatch` (portal_saver.go:390-401) independently encode the same dev/empty/read-error rules. The in-source comment admits "byte-equivalent in semantics" reimplementation. Two predicates must be kept in sync by hand; `TestPortalSaverVersionMismatch_PredicateMatrix` pins the legacy predicate so silent divergence is not auto-detected. After the Change 1/2 work `portalSaverVersionMismatch` has zero production callers — it survives only because its dedicated test re-exports it via `tmux.PortalSaverVersionMismatch` in `internal/tmux/export_test.go:18`. The two predicates differ only in their handling of `ErrVersionFileAbsent` (true vs false).

**Solution**: Make `shouldKillSaverOnVersionDecision` the single source of truth. Delete `portalSaverVersionMismatch` entirely and reframe its predicate-matrix test against `shouldKillSaverOnVersionDecision`. This eliminates the dead production helper and the parallel encoding in one step.

**Outcome**: One predicate function encodes the dev/empty/read-error rules. The "byte-equivalent" comment is gone. Future changes to dev-build version semantics cannot drift between the kill-decision and the predicate contract.

**Do**:
1. Delete `portalSaverVersionMismatch` from `/Users/leeovery/Code/portal/internal/tmux/portal_saver.go:390-401`.
2. Remove the re-export `PortalSaverVersionMismatch` from `/Users/leeovery/Code/portal/internal/tmux/export_test.go:18`.
3. In `/Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go:1957-2031`, rewrite the predicate-matrix cases to drive `shouldKillSaverOnVersionDecision` directly (export it via `export_test.go` if needed). Preserve every matrix row's semantics; for the row that previously asserted "absent file → mismatch", add an explicit assertion that under the new predicate absent-file returns `false`.
4. Remove the "byte-equivalent in semantics" comment block at `portal_saver.go:271-275`.
5. Run `go build ./...` and `go test ./internal/tmux/...`.

**Acceptance Criteria**:
- `portalSaverVersionMismatch` no longer exists in `internal/tmux/portal_saver.go`.
- `tmux.PortalSaverVersionMismatch` no longer exists in `internal/tmux/export_test.go`.
- `shouldKillSaverOnVersionDecision` is the only predicate encoding dev/empty/read-error rules.
- The reframed predicate-matrix test targets `shouldKillSaverOnVersionDecision` and includes an explicit row covering `ErrVersionFileAbsent → false`.
- No in-source comment claims "byte-equivalent" semantics between two predicates.
- `go test ./internal/tmux/...` passes.

**Tests**:
- Reframed predicate-matrix test rows: dev-version stored, dev-version current, empty stored, empty current, equal non-dev, mismatched non-dev, readErr non-nil (non-absent), readErr `ErrVersionFileAbsent`.
- Existing caller-layer tests for `shouldKillSaverOnVersionDecision` continue to pass unchanged.

---

## Task 2: Thread a real *state.Logger to the bootstrap-side defensive WriteVersionFile call
status: pending
severity: medium
sources: standards, architecture

**Problem**: Spec § Change 3 / Acceptance Criterion #9 mandates "Each `state.WriteVersionFile` call emits one DEBUG log line containing version, caller pid, and destination path", with the explicit rationale that "the bootstrap-side defensive write (Change 1) also flows through the same helper; using `ComponentDaemon` keeps a single grep anchor regardless of caller." The implementation at `/Users/leeovery/Code/portal/internal/tmux/portal_saver.go:57-59` routes the bootstrap defensive write through a wrapper that passes `nil` for the logger. Under the `Logger` nil-receiver no-op contract, the bootstrap-survived-path repair emits zero breadcrumbs. Defect 3 investigations grepping `portal.log` will see entries for the daemon's own startup write but not for the bootstrap repair, producing a misleading paper trail on precisely the surface Defect 3 was designed to instrument.

**Solution**: Add a logger sink to `internal/tmux` for `portalSaverWriteVersionFile` (mirroring `killBarrierLogger` + `SetBarrierLogger`), and wire a real `*state.Logger` from `internal/bootstrapadapter` at the same site that already calls `SetBarrierLogger`. Drop the "follow-up" framing comment.

**Outcome**: Every `state.WriteVersionFile` call site (daemon startup AND bootstrap defensive repair) emits a DEBUG breadcrumb tagged `ComponentDaemon`. A single grep against `portal.log` surfaces both callers, restoring the audit trail Change 3 was designed to install.

**Do**:
1. In `/Users/leeovery/Code/portal/internal/tmux/portal_saver.go`, introduce a package-level `versionWriterLogger *state.Logger` sink and a `SetVersionWriterLogger(*state.Logger)` setter, mirroring `killBarrierLogger` / `SetBarrierLogger`.
2. Update `portalSaverWriteVersionFile` (portal_saver.go:57-59) to pass `versionWriterLogger` instead of `nil` to `state.WriteVersionFile`.
3. In `/Users/leeovery/Code/portal/internal/bootstrapadapter/`, at the site that calls `tmux.SetBarrierLogger`, also call `tmux.SetVersionWriterLogger` with the same `*state.Logger` instance.
4. Remove or update the "does not land for this defensive call site … wiring a real logger here can be a follow-up" comment at the wrapper site.
5. Add a unit test in `internal/tmux` that calls `SetVersionWriterLogger` with a capturing logger, invokes `portalSaverWriteVersionFile`, and asserts one DEBUG breadcrumb was emitted containing version, caller pid, and destination path.

**Acceptance Criteria**:
- `internal/tmux` exposes `SetVersionWriterLogger(*state.Logger)`.
- `portalSaverWriteVersionFile` forwards the package-level logger (never `nil`) to `state.WriteVersionFile`.
- `bootstrapadapter` wires a real `*state.Logger` via `SetVersionWriterLogger` alongside `SetBarrierLogger`.
- Comments at the wrapper site no longer flag the breadcrumb gap as a follow-up.
- A unit test pins that the bootstrap defensive call produces exactly one DEBUG breadcrumb with the spec-mandated fields.

**Tests**:
- New unit test: capturing-logger asserts one DEBUG line containing version + pid + dest path is emitted when `portalSaverWriteVersionFile` runs with the sink wired.
- Existing daemon-startup breadcrumb test continues to pass.

---

## Task 3: Extract PollUntil helper to eliminate six near-identical polling loops in integration tests
status: pending
severity: medium
sources: duplication

**Problem**: Six integration-test helpers share the same `deadline := time.Now().Add(timeout); for time.Now().Before(deadline) { ...; time.Sleep(tick) }; t.Fatalf(...)` skeleton. Sites: `cmd/state_daemon_integration_test.go:576-594` (waitForDaemonAlive), `internal/tmux/portal_saver_integration_test.go:547-559` (waitForDaemonNotAlive), :566-578 (waitForSessionAbsent), :584-594 (waitForVersionFile), :672-685 (waitForLiveDaemon), :694-707 (waitForNewLiveDaemon). Loop wiring, deadline arithmetic, and `t.Fatal`-on-timeout idiom are duplicated six times.

**Solution**: Extract `PollUntil(t *testing.T, timeout, tick time.Duration, cond func() bool) bool` in `internal/tmuxtest`. Each call site collapses to a one-liner that handles its return/fatal shape locally.

**Outcome**: One canonical polling implementation. Six call sites shrink significantly. Future polling helpers don't reinvent the deadline arithmetic.

**Do**:
1. Add `PollUntil(t *testing.T, timeout, tick time.Duration, cond func() bool) bool` to `internal/tmuxtest/`. Returns `true` on condition met, `false` on timeout. Does NOT call `t.Fatal` itself — leaves fatal shape to caller.
2. Rewrite each of the six helpers to call `PollUntil` and own their own `t.Fatalf` message on timeout plus their own return-value extraction on success.
3. Preserve each helper's external signature so call sites do not change.
4. Run `go test ./...`.

**Acceptance Criteria**:
- `internal/tmuxtest.PollUntil` exists with the documented signature.
- All six listed helpers delegate their loop body to `PollUntil`.
- No `for time.Now().Before(deadline)` loop remains in the six helpers.
- Each helper's external signature and `t.Fatalf` message wording is preserved.
- `go test ./cmd/... ./internal/tmux/...` passes (skip-on-tmux-absent paths intact).

**Tests**:
- Existing integration tests covering the six helpers are the de facto regression coverage.
- Add a focused unit test for `PollUntil`: returns true when cond becomes true before timeout; returns false when timeout elapses with cond never true.

---

## Task 4: Extract StagePortalBinary helper to eliminate repeated build+PATH preamble across integration tests
status: pending
severity: medium
sources: duplication

**Problem**: Four real-tmux integration tests open with a near-identical ~8-line preamble — `t.TempDir()` for binDir, `restoretest.BuildPortalBinary` with skip-on-failure, `t.Setenv("PATH", ...)`, `exec.LookPath("portal")` with skip-on-failure. Sites: `cmd/state_daemon_integration_test.go:181-189`, `internal/tmux/portal_saver_integration_test.go:134-151`, :287-294, :427-434. Structural duplication; the cost of accidental divergence is silent test fragility.

**Solution**: Extract `StagePortalBinary(t *testing.T) string` returning the binDir; handles skip-on-build-failure, `t.Setenv("PATH", ...)`, and skip-on-LookPath-failure. Place in `restoretest` alongside `BuildPortalBinary`.

**Outcome**: One implementation of the preamble. Four call sites collapse to a single line. New integration tests inherit the correct skip semantics for free.

**Do**:
1. Add `StagePortalBinary(t *testing.T) string` to `internal/restoretest/`. Internally: `binDir := t.TempDir()`, `restoretest.BuildPortalBinary(t, binDir)` (with skip), `t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))`, `exec.LookPath("portal")` (with skip), return `binDir`.
2. Replace the four call-site preambles with `binDir := restoretest.StagePortalBinary(t)`.
3. Verify PATH composition (binDir prepended to existing PATH) is preserved.
4. Run `go test ./cmd/... ./internal/tmux/...`.

**Acceptance Criteria**:
- `restoretest.StagePortalBinary` exists and encapsulates the build + setenv + lookpath sequence with skip semantics.
- All four listed integration tests use it instead of inlining the preamble.
- Each test's skip behaviour (build fail or `portal` not on PATH) is preserved.
- `go test ./cmd/... ./internal/tmux/...` passes.

**Tests**:
- The four integration tests using the helper are the de facto regression coverage.

---

## Task 5: Extract sentinelIndex + assertNoCommit helpers for captureAndCommit unchanged-pointer tests
status: pending
severity: low
sources: duplication

**Problem**: Four tests in `/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go` (lines 880-908, 967-1010, 1056-1117, 1162-1209) construct an identical sentinelPrev fixture and apply the same post-call assertions: "PrevIndex pointer unchanged from sentinel" and "sessions.json must not exist on disk". Fixture build-up is 6-8 lines per test; assertion block another 4-6.

**Solution**: Extract `sentinelIndex(name string) *state.Index` and `assertNoCommit(t, deps, sentinel)` helpers local to the test file. The replaced variant becomes `assertCommitReplacedPrev`.

**Outcome**: Each captureAndCommit test reads as intent (which cancellation point is exercised) rather than as fixture plumbing.

**Do**:
1. In `cmd/state_daemon_run_test.go`, add `sentinelIndex(name string) *state.Index` returning a fixed-shape sentinel distinguishable by name.
2. Add `assertNoCommit(t *testing.T, deps <captureDepsType>, sentinel *state.Index)` asserting `deps.PrevIndex == sentinel` (pointer identity) and that `sessions.json` does not exist in the test's state dir.
3. Add a peer `assertCommitReplacedPrev` for the replaced-pointer case.
4. Replace the four duplicated fixture+assertion blocks with calls to the new helpers.
5. Run `go test ./cmd/ -run TestCaptureAndCommit`.

**Acceptance Criteria**:
- `sentinelIndex`, `assertNoCommit`, and `assertCommitReplacedPrev` exist in `cmd/state_daemon_run_test.go`.
- The four listed tests use the helpers and no longer inline the sentinel fixture or post-call assertions.
- All four tests preserve their existing assertion semantics.
- `go test ./cmd/...` passes.

**Tests**:
- The four existing captureAndCommit tests are the regression coverage.

---

## Task 6: Extract assertKillBeforeNew helper for kill-before-new-session order checks
status: pending
severity: low
sources: duplication

**Problem**: Four tests in `/Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go` (lines 200-215, 293-309, 668-683, 1521-1535) contain an identical ~15-line scan of `mock.Calls` that captures the first kill-session index and the first new-session index, then asserts kill precedes new. The block is mechanical and identical across copies.

**Solution**: Extract `assertKillBeforeNew(t *testing.T, calls [][]string)` and replace the four copies with a single call each.

**Outcome**: One implementation of the order-check. Future tests asserting saver kill-before-new ordering inherit it for free.

**Do**:
1. In `internal/tmux/portal_saver_test.go` (or a sibling `*_test.go` helper file in the same package), add `assertKillBeforeNew(t *testing.T, calls [][]string)` scanning `calls` for the first `kill-session` and the first `new-session` arg-set, failing if either is missing, and asserting the kill index precedes the new-session index.
2. Replace the four duplicated 15-line blocks with `assertKillBeforeNew(t, mock.Calls)`.
3. Preserve each test's existing fatal-message wording (or accept a uniform message — pick one and apply consistently).
4. Run `go test ./internal/tmux/...`.

**Acceptance Criteria**:
- `assertKillBeforeNew` exists in the `internal/tmux` test scope.
- The four listed tests use the helper and no longer scan `mock.Calls` inline for ordering.
- Each test's ordering assertion semantics are preserved.
- `go test ./internal/tmux/...` passes.

**Tests**:
- The four existing tests are the regression coverage.

---

## Discarded Findings

- "Repeated ctx.Done() default-select pattern inside captureAndCommit" (duplication, low) — three call sites only; borderline on Rule of Three and the spec structurally pins the count at three. Author flagged it as optional. Extraction savings (three 4-line blocks → three 1-line guards) do not justify the helper + per-site comment retention.
- "WriteVersionFile signature mixes data and observability concerns" (architecture, low) — fully subsumed by Task 2. Once the bootstrap call site wires a real logger, the "logger param sometimes nil" critique no longer applies; the alternative remedy (lift breadcrumb to callers) is mutually exclusive with Task 2's solution.
