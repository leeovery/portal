---
topic: multiple-state-daemons-running-concurrently
cycle: 1
total_proposed: 5
---
# Analysis Tasks: multiple-state-daemons-running-concurrently (Cycle 1)

## Task 1: Restore spec-mandated pgrep server-children assertion in singleton integration test
status: pending
severity: medium
sources: standards

**Problem**: `internal/tmux/portal_saver_integration_test.go:213-222` asserts `priorPID dead && currentPID alive` instead of the spec-mandated `pgrep -P <tmux-server-pid> -f 'portal state daemon' | wc -l == 1` (spec lines 192, 327, 329). The pidfile-life check only sees PIDs recorded in `daemon.pid` and would miss a third orphan attached to the tmux server but not in the pidfile — exactly the multi-orphan accumulation shape the spec's pgrep form is designed to catch. Spec line 329 names this as "the load-bearing test for the bug."

**Solution**: Add (alongside the existing alive/dead pair) a `pgrep -P <serverPID> -f 'portal state daemon'` invocation and assert the live-process count is exactly 1. The serverPID is already captured at line 171.

**Outcome**: Integration test observes the process tree directly per spec, providing forward-compat regression-guard value against multi-orphan accumulation shapes that the current pidfile-PID check cannot model.

**Do**:
1. Open `internal/tmux/portal_saver_integration_test.go` at the assertion block (lines 213-222).
2. After the existing `priorPID dead && currentPID alive` assertions, exec `pgrep -P <serverPID> -f 'portal state daemon'` using the serverPID captured at line 171.
3. Parse the output line count (handle empty/whitespace) and assert it equals exactly 1.
4. Retain existing alive/dead assertions — they show the recycle actually displaced the prior daemon.
5. Use `t.Fatalf` on mismatch with the offending count and raw pgrep output for diagnostics.

**Acceptance Criteria**:
- Test asserts `pgrep -P <serverPID> -f 'portal state daemon'` returns exactly one PID after both `EnsurePortalSaverVersion` calls.
- Existing alive/dead pair assertions remain.
- Test fails with a diagnostic message including raw pgrep output if the count is not 1.

**Tests**:
- Existing integration test continues to pass against the current implementation.
- Hand-mutate the implementation to skip the kill-barrier and confirm the new pgrep assertion fails, demonstrating the new assertion's added regression value.

## Task 2: Replace literal "bootstrap" string with state.ComponentBootstrap constant at kill-barrier WARN site
status: pending
severity: low
sources: standards, architecture

**Problem**: `internal/tmux/portal_saver.go:177` (`killSaverAndWaitForDaemon`) emits its WARN-on-timeout line with a hard-coded literal `"bootstrap"` as the component argument. Every other production log site uses the typed `state.ComponentBootstrap` / `state.ComponentDaemon` constants. The file already imports `internal/state`. Two pre-existing occurrences in `internal/tmux/hooks_register.go:221,229` share the same fragility.

**Solution**: Reference `state.ComponentBootstrap` directly at the WARN site in `portal_saver.go:177`. Optionally clean up the two pre-existing occurrences in `hooks_register.go` for consistency.

**Outcome**: Single source of truth for the bootstrap component string; future renames stay safe across all log sites.

**Do**:
1. Open `internal/tmux/portal_saver.go` at line 177.
2. Replace the literal `"bootstrap"` argument in the WARN call with `state.ComponentBootstrap`.
3. Open `internal/tmux/hooks_register.go` at lines 221 and 229 and apply the same substitution.
4. Confirm `internal/state` is already imported in both files.

**Acceptance Criteria**:
- No literal `"bootstrap"` string remains as a log component argument in `internal/tmux/portal_saver.go` or `internal/tmux/hooks_register.go`.
- All three call sites reference `state.ComponentBootstrap`.
- `go build ./...` succeeds; existing tests pass unchanged.

**Tests**:
- Existing tests that match log output by component string continue to pass (constant value unchanged).
- No new tests required — purely a consistency refactor.

## Task 3: Extract ProjectRoot + buildPortalBinary helpers from restoretest into an untagged file for default-lane test reuse
status: pending
severity: medium
sources: duplication

**Problem**: `internal/tmux/portal_saver_integration_test.go:311-361` inlines `projectRootForSingletonTest` (16 lines) and `buildPortalBinaryForSingletonTest` (15 lines) — verbatim duplicates of `restoretest.ProjectRoot` and `restoretest.BuildPortalBinaryDir`'s `buildPortalBinaryInto` body. The duplication exists because `internal/restoretest/restoretest.go` carries `//go:build integration` and the new test runs under the default lane. The walk-up-to-`go.mod` loop and `go build -o <bin> .` invocation are structural shared concerns that will repeat the next time a default-lane test needs a real portal binary.

**Solution**: Split `internal/restoretest/restoretest.go` into a tagged file (keeping `BuildPortalBinaryDir`, `BuildPortalBinaryStable`, `PrependPATH`, `DriveSignalHydrate` under `//go:build integration`) and an untagged file (`ProjectRoot` + `buildPortalBinaryInto` + a returns-error `BuildPortalBinary(dir string) error` wrapper). Delete the inlined helpers in `portal_saver_integration_test.go` and call the shared helpers directly.

**Outcome**: One source of truth for "find the repo root, go build, return errors not fatals"; ~30 lines deleted; future default-lane integration tests can reuse the helpers.

**Do**:
1. Create `internal/restoretest/build.go` (no build tag) containing `ProjectRoot() (string, error)`, `buildPortalBinaryInto(dir string) error`, and a public `BuildPortalBinary(dir string) error` wrapper. All return wrapped errors not `t.Fatal`.
2. Edit `internal/restoretest/restoretest.go`: remove the ProjectRoot + buildPortalBinaryInto definitions but keep `BuildPortalBinaryDir`, `BuildPortalBinaryStable`, `PrependPATH`, `DriveSignalHydrate` under the existing `//go:build integration` tag, calling the un-tagged helpers.
3. Edit `internal/tmux/portal_saver_integration_test.go`: delete `projectRootForSingletonTest` (lines 311-326) and `buildPortalBinaryForSingletonTest` (lines 328-361). Replace call sites with `restoretest.ProjectRoot()` and `restoretest.BuildPortalBinary(dir)`.
4. Confirm both lanes build: `go build ./...` and `go test -tags integration ./internal/restoretest/...`.

**Acceptance Criteria**:
- `internal/restoretest/build.go` exists with no build tag and exports `ProjectRoot`, `BuildPortalBinary`.
- `internal/restoretest/restoretest.go` retains only the integration-tagged surface.
- `portal_saver_integration_test.go` no longer contains the inlined helpers; references resolve to the shared package.
- `go test ./...` passes in the default lane; `go test -tags integration ./...` passes in the integration lane.
- Net deletion of ~30 lines.

**Tests**:
- Existing `portal_saver_integration_test.go` singleton test continues to pass using the shared helpers.
- Existing integration-tagged tests in `internal/restoretest/` continue to pass.

## Task 4: Cover the ERROR-level log assertion for non-contention lock-acquire failure
status: pending
severity: low
sources: standards

**Problem**: Spec § Fix Part 1 → Lock-file create/open semantics (line 100) requires non-EWOULDBLOCK open(2)/flock failures to log an ERROR-level line describing the failure and exit non-zero. The implementation logs it (`cmd/state_daemon.go:261`) and propagates the error, but `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` (`cmd/state_daemon_test.go:570-599`) only asserts the non-zero exit and that state files were not written — it never asserts the ERROR-level log line was emitted. The WARN-on-contention sibling test does assert log presence and exactly-one-line, leaving the ERROR path with weaker coverage than spec demands.

**Solution**: Extend `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` (or add a sibling test) to set `PORTAL_LOG_LEVEL=error`, read `portal.log` post-run, and assert exactly one line contains "ERROR" and "acquire daemon lock".

**Outcome**: Spec's "loud surfacing" requirement is covered by an assertion equivalent to the WARN-on-contention test, closing the coverage asymmetry.

**Do**:
1. Open `cmd/state_daemon_test.go` and locate `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` (lines 570-599).
2. Set `PORTAL_LOG_LEVEL=error` via `t.Setenv` before invoking RunE.
3. After the non-zero exit assertion, read `portal.log` from the test stateDir.
4. Assert that exactly one line in the log contains both `"ERROR"` and `"acquire daemon lock"` (mirroring the WARN-on-contention sibling test's pattern).
5. Use clear failure messages including the log contents on mismatch.

**Acceptance Criteria**:
- Test asserts exactly one ERROR-level log line is emitted matching the spec's "acquire daemon lock" message.
- Test continues to assert non-zero exit and absence of state file writes.
- Test fails with diagnostic output if zero or multiple matching lines are present.
- Coverage symmetry with the WARN-on-contention sibling test is restored.

**Tests**:
- The extended test passes against the current implementation.
- Hand-mutate `cmd/state_daemon.go:261` to remove the ERROR log call and confirm the test fails.

## Task 5: Reset daemonLockFile package var in every cmd-package test that runs the real lock path
status: pending
severity: low
sources: architecture

**Problem**: `cmd.daemonLockFile` is a package-level `*os.File` retained for the daemon's process lifetime — load-bearing for production (`cmd/state_daemon.go:61,264`). In tests, every successful RunE path that goes through the real `state.AcquireDaemonLock` (tests that don't stub `acquireDaemonLock`) sets the var and leaves it pointing at an open fd inside a now-deleted `t.TempDir`. Only `withDaemonLockFileReset` clears the var. The lock/retain tests at `cmd/state_daemon_test.go:419, 451, 601` call it correctly, but tests at lines 47, 65, 88, 112, 151, 174, 196, 355 do not. No production-correctness risk, but the asymmetry is a maintenance trap.

**Solution**: Call `withDaemonLockFileReset` in every cmd-package test that runs the daemon's RunE through the real lock path, consistent with seam-reset discipline elsewhere.

**Outcome**: Consistent seam-reset discipline across all cmd-package daemon tests; no leaked package-var state between tests.

**Do**:
1. Open `cmd/state_daemon_test.go` and identify the tests at lines 47, 65, 88, 112, 151, 174, 196, 355 that run RunE through `state.AcquireDaemonLock` without stubbing `acquireDaemonLock`.
2. For each such test, add a `withDaemonLockFileReset(t)` call alongside other seam-reset cleanups (typically at the top of the test before invoking RunE).
3. Confirm tests at lines 419, 451, 601 already make the call (no change needed).
4. Run `go test ./cmd/...` to confirm no regressions.

**Acceptance Criteria**:
- Every cmd-package test that exercises the real `state.AcquireDaemonLock` path also calls `withDaemonLockFileReset`.
- The `daemonLockFile` package var is reliably reset between tests.
- `go test ./cmd/...` passes.
- A future test asserting on `daemonLockFile` would not observe leaked state from a prior test.

**Tests**:
- Existing test suite continues to pass with the resets added.
- No new test logic required — purely a hygiene change.
