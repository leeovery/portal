---
scope: slow-open-empty-previews-and-zombie-sessions
cycle: 1
source: review
total_proposed: 4
gate_mode: auto
---
# Review Tasks: Slow Open Empty Previews And Zombie Sessions (Cycle 1)

## Task 1: Fix TestStateDaemon_ReturnsErrorOnNonContentionLockFailure to assert WARN
status: approved
severity: high
sources: report.md Required Changes #1, report-9-7

**Problem**: T9-7 changed production log level from Error to Warn at
`cmd/state_daemon.go:213` (per Component C spec — non-contention lock
failures must not be noisier than contention sibling) but did not update
`TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` in
`cmd/state_daemon_test.go:591-650`. The test still sets
`PORTAL_LOG_LEVEL=error` and asserts an `ERROR` log line is present. Result:
`go test ./cmd -run TestStateDaemon_ReturnsErrorOnNonContentionLockFailure`
FAILS. Verified failing in review.

**Solution**: Update the test's log-level env var, log-line matchers, and
stale comments to assert WARN instead of ERROR. Production code is correct;
test was simply not updated alongside the production change.

**Outcome**: Test passes against the WARN-emitting production code. Fatal
path remains non-silent (asserts the WARN line exists) and non-noisy (asserts
exactly one matching line).

**Do**:
1. In `cmd/state_daemon_test.go`:
   - Line 594: change `t.Setenv("PORTAL_LOG_LEVEL", "error")` to
     `t.Setenv("PORTAL_LOG_LEVEL", "warn")`.
   - Line 633: change `strings.Contains(got, "ERROR")` to
     `strings.Contains(got, "WARN")`.
   - Lines 640-645: change the loop's `strings.Contains(line, "ERROR")` and
     the surrounding `ERROR`-referencing error message to `WARN`.
   - Lines 592-593 update comment: replace the "ERROR is above the default
     WARN threshold" rationale with one explaining that WARN is the production
     emission level per Component C spec (non-contention failure mirrors
     contention path severity).
   - Lines 624-627 update the spec-citation comment: replace
     "requires non-EWOULDBLOCK ... to emit an ERROR-level log line" with
     the WARN-level wording matching the post-T9-7 spec text.
   - Line 646-648 error message: replace `"ERROR line"` wording with
     `"WARN line"`.
2. Optionally rename the test function to
   `TestStateDaemon_ReturnsErrorAndLogsWarnOnNonContentionLockFailure` to make
   the level switch visible from the test name. Update any references.
3. Run `go test ./cmd -run TestStateDaemon_ReturnsError` and confirm pass.
4. Run `go test ./...` to confirm no other test references the old name or
   ERROR assertion shape.

**Acceptance Criteria**:
- `go test ./cmd -run TestStateDaemon_ReturnsErrorOnNonContentionLockFailure`
  (or renamed equivalent) passes.
- `go test ./...` clean.
- Test still asserts exactly one matching log line — the "not silent, not
  noisy" invariant is preserved.
- All ERROR-referencing comments in the test body are updated to WARN.

**Tests**:
- The test itself is what is being fixed. No new tests required.
- Verify by running the targeted test plus the package-wide suite.

---

## Task 2: Fix T6-4 scrollback-stability harness silent-pass on empty baseline and missing dir
status: approved
severity: high
sources: report.md Bugs #49 + #50, report-6-4

**Problem**: The composite scrollback-stability test in
`cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go`
silently green-lights two scenarios that are the exact Component E
regressions the test was designed to catch:

1. **Empty baseline (line 113-114)**: After bootstrap the harness has seeded
   two sessions running `while sleep 0.1; do echo "hello $RANDOM"; done`, so
   the surviving daemon's capture loop MUST produce at least one `.bin`
   file under `state/scrollback/` within the post-bootstrap buffer window.
   The current code treats an empty baseline as a "valid starting point"
   ("stays empty" invariant) — but stays-empty is precisely the
   capture-pipeline-broken signal. Plan requires FAIL with message
   `"scrollback baseline empty after first post-bootstrap tick — capture pipeline may be broken or seed activity insufficient"`.

2. **Missing scrollback dir (lines 145-150)**: `snapshotScrollbackPaths`
   treats ENOENT at the dir root as an empty path-set via
   `filepath.SkipDir`. Plan requires distinguishing ENOENT from
   empty-set and failing with `"scrollback dir does not exist"`.

Both gaps mean a future regression that breaks Component E capture would
pass this composite test silently.

**Solution**: Add a positive-baseline assertion immediately after
`baseline := snapshotScrollbackPaths(...)` requiring `len(baseline) > 0`,
and change the walker's ENOENT handling so the missing-dir case is
distinguishable from empty-set and surfaces as a failure.

**Outcome**: The test fails loudly with the plan-specified diagnostic
messages when either (a) the capture pipeline produces no output during
the post-bootstrap buffer window, or (b) `state/scrollback/` is missing
entirely. The path-set stability assertion still runs over the
non-empty baseline.

**Do**:
1. In `cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go`
   at the test body (around line 114), after the baseline snapshot:
   ```go
   baseline := snapshotScrollbackPaths(t, scrollbackDir)
   if len(baseline) == 0 {
       t.Fatalf("scrollback baseline empty after first post-bootstrap tick — capture pipeline may be broken or seed activity insufficient")
   }
   ```
2. Change `snapshotScrollbackPaths` so the caller can distinguish "dir
   missing" from "dir present but empty". Options (pick the one that
   composes best with the existing helper signature):
   - Add a bool return: `(map[string]struct{}, bool)` where the bool is
     `false` when the root dir does not exist. Caller fails with
     `"scrollback dir does not exist"` when the bool is false.
   - Or split into two helpers (`scrollbackDirExists` + the existing path
     walker) and assert existence in the test body before snapshotting.
   - Update the docstring (lines 129-138) to remove the "empty set is
     valid baseline" claim, replacing it with the new semantics.
3. Update the post-bootstrap-buffer comment block (lines 103-107) to note
   that the seeded `while sleep 0.1; do echo "hello $RANDOM"; done` work
   guarantees at least one capture tick produces output before the
   baseline snapshot, so an empty baseline IS a regression signal.
4. Run `go test ./cmd/bootstrap -run E2EScrollbackStability` (or the
   integration-tagged form per the file's build constraints) and confirm
   the happy path still passes against the working capture pipeline.

**Acceptance Criteria**:
- `len(baseline) > 0` is asserted immediately after the baseline snapshot,
  with the plan-specified error message.
- The walker (or caller) distinguishes ENOENT-at-root from empty-set and
  fails with `"scrollback dir does not exist"` on the missing-dir case.
- Test still passes against the working production capture pipeline.
- Docstring on `snapshotScrollbackPaths` no longer claims empty-set is a
  valid baseline shape.

**Tests**:
- The test being fixed is itself the regression guard. Verify the
  positive path passes with `go test` against the integration tag.
- A short follow-up unit test that calls the modified
  `snapshotScrollbackPaths` against (a) a missing dir and (b) an empty
  but existing dir, asserting the two cases are distinguishable, would
  pin the new contract — recommended but not load-bearing.

---

## Task 3: Resolve Component F spec/impl mismatch — "_portal-saver session persists after daemon exits"
status: approved
severity: medium
sources: report.md Recommendations #26 / Spec deviations Task 3-5, report-3-5

**Problem**: Component F acceptance bullet 3 in the specification literally
asserts `_portal-saver` session **persists after the daemon exits**.
Implementation (task 3-5) reframed the acceptance check to "absence of
no-such-session log noise during the lock-loser cascade" because on tmux
3.6b the session DOES disappear when the lock-loser daemon exits, even
with `destroy-unattached=off`. Spec text was not updated; the spec/impl
record carries a literal mismatch. The reframing is defensible (tmux
behaviour is not under our control without `remain-on-exit on`) but the
spec author owes a decision: (a) amend the spec to match the
log-noise-absence shape implementation actually asserts, or (b) add
`remain-on-exit on` to the saver session bootstrap so the literal spec
assertion holds.

**Solution**: Pick a path and execute. Option (a) — amend spec — is the
lower-risk default given the implementation is already shipped and the
log-noise-absence assertion is observable and meaningful. Option (b) —
add `remain-on-exit on` — keeps the spec literal but introduces a tmux
option change with its own subtle behaviour (the pane retains a dead
shell, may affect later restore semantics). Default to (a) unless there
is a reason to prefer (b).

**Outcome**: Spec text and implementation behaviour agree on Component F
bullet 3. Future readers do not encounter a documented mismatch.

**Do**:
1. Decide between option (a) (amend spec) and option (b) (add
   `remain-on-exit on`). Default to (a).
2. **If (a)**:
   - Locate Component F acceptance bullet 3 in
     `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md`.
   - Rewrite the bullet to assert: "during the lock-loser cascade, no
     `no such session: _portal-saver` (or equivalent) log lines appear in
     `portal.log`". Cite the tmux 3.6b behaviour note as rationale.
   - Add a one-paragraph spec note explaining that without
     `remain-on-exit on`, the saver session does NOT outlive its daemon
     pane process, and that future work could opt in to literal
     session-persistence if needed.
3. **If (b)**:
   - Add `set-option -t _portal-saver remain-on-exit on` (or the
     session-scoped equivalent) to the saver bootstrap path in
     `internal/tmux/` (search for `BootstrapPortalSaver` / saver-session
     creation).
   - Add an integration test that kills the daemon and asserts the
     session remains present.
   - Update the impl docs / comments at the task 3-5 assertion site to
     reference the new literal-persistence behaviour.
4. Update the task 3-5 row in the planning artefact (if mutable per the
   workflow's conventions) noting the resolution.

**Acceptance Criteria**:
- Specification Component F bullet 3 text matches what the implementation
  actually asserts.
- No outstanding "spec/impl mismatch" note remains in the review record
  for this item.
- If option (b), an integration test demonstrates the session persists
  past daemon exit and `go test ./...` clean.

**Tests**:
- If option (a) (spec amendment), no code tests required; verify by
  re-reading the spec and the implementation assertion side-by-side.
- If option (b), a new integration test that:
  - Bootstraps the saver session.
  - SIGKILLs the daemon pane PID.
  - Polls `tmux has-session -t _portal-saver` for up to 2s and asserts
    the session is still present.

---

## Task 4: Fix T7-5 stale misleading comment in TestStateDaemon_DoesNotWritePIDFileWhenLockHeld
status: approved
severity: low
sources: report.md Bugs #51, report-7-5

**Problem**: At `cmd/state_daemon_test.go:543-548` the comment in
`TestStateDaemon_DoesNotWritePIDFileWhenLockHeld` states "daemon.version IS
written when lock-held under the new ordering" — but post-T7-5,
daemon.version is NOT written on lock contention (the test still passes
because it only checks daemon.pid). The comment misleads future readers
into believing the opposite of the actual production behaviour, which
will bite during the next change in this neighbourhood.

**Solution**: Rewrite the comment to describe what actually happens on
the lock-held path post-T7-5: neither daemon.pid nor daemon.version is
written, and this test currently only spot-checks daemon.pid.

**Outcome**: The comment correctly describes post-T7-5 ordering. Future
readers can rely on it. Optional: add an explicit
`os.Stat(daemon.version)` assertion to lift the daemon.version invariant
from comment-only to test-asserted.

**Do**:
1. Read `cmd/state_daemon_test.go:543-548` and the surrounding test body
   to confirm current behaviour.
2. Rewrite the comment to state: under the post-T7-5 ordering, neither
   `daemon.pid` nor `daemon.version` is written when the daemon exits on
   lock contention. This test asserts the daemon.pid invariant; the
   daemon.version invariant is upheld structurally by the
   acquire-then-write ordering pinned by the T4-8 AST adjacency test.
3. Optionally (recommended): add a stat assertion that
   `daemon.version` likewise does not exist after the contention exit,
   mirroring the daemon.pid check. This lifts the invariant from
   structural-only to spot-checked.
4. Run `go test ./cmd -run TestStateDaemon_DoesNotWritePIDFileWhenLockHeld`
   and confirm pass.

**Acceptance Criteria**:
- Comment at `cmd/state_daemon_test.go:543-548` accurately describes
  post-T7-5 behaviour.
- Test still passes.
- If the optional daemon.version stat assertion is added, it passes.

**Tests**:
- The targeted test continues to pass.
- If the optional daemon.version assertion is added, it is exercised by
  the same test run.
