---
phase: 1
phase_name: Daemon-Side Singleton Lock
total: 4
---

## multiple-state-daemons-running-concurrently-1-1 | approved

### Task 1.1: Add seam-injectable flock helper for daemon.lock

**Problem**: There is no structural guarantee that at most one `portal state daemon` writes a given state directory. `state.WritePIDFile` is "last writer wins" via `fileutil.AtomicWrite`, and `BootstrapAliveCheck` is informational only — once a second daemon overwrites `daemon.pid` the prior daemon becomes an invisible orphan. We need an OS-level advisory lock primitive scoped to `<stateDir>/daemon.lock` that the daemon-startup path can call before any state-directory write, with a test seam matching the existing `daemonRunFunc` / `daemonShutdownFunc` pattern so unit tests can simulate acquire-success / acquire-fail / open-error without real kernel locks.

**Solution**: Add a small helper in `internal/state` (alongside the existing pidfile helpers in `internal/state/daemon_state.go`, or as a new sibling file `internal/state/daemon_lock.go`) that opens `<stateDir>/daemon.lock` mode 0600 and attempts `unix.Flock(fd, LOCK_EX|LOCK_NB)`. The helper accepts a `stateDir string` parameter (no hardcoded path) so each test can isolate via `t.TempDir()`. On success it sets `FD_CLOEXEC` on the returned fd and hands the fd back to the caller as an `*os.File` so caller-owned fd retention is explicit. On `EWOULDBLOCK` it returns a distinct sentinel error (`ErrDaemonLockHeld`) so callers can branch contention vs. fatal-open. Other `open(2)` errors (`EACCES`, `ENOSPC`, `ENOENT`, `EMFILE`, `ENFILE`) are wrapped and returned as plain errors. The `unix.Flock` call is seamed via a package-level `var lockAcquire = unix.Flock` so unit tests inject a fake.

**Outcome**: A self-contained, dependency-injected lock helper exists in `internal/state` that callers use to acquire an exclusive non-blocking flock on `<stateDir>/daemon.lock`, with EWOULDBLOCK distinguishable from other open/flock failures, FD_CLOEXEC asserted on the returned fd, and a test seam that lets unit tests drive every branch without touching real flock state.

**Do**:
- Create `internal/state/daemon_lock.go` (new file) in the `state` package.
- Declare exported sentinel: `var ErrDaemonLockHeld = errors.New("daemon.lock held by another process")`.
- Declare the seam: `var lockAcquire = unix.Flock` (import `golang.org/x/sys/unix`; already a transitive dependency via `golang.org/x/sys`).
- Implement `func AcquireDaemonLock(stateDir string) (*os.File, error)`:
  - Compose path via a new accessor `DaemonLock(dir string) string` added to `internal/state/paths.go` (`filepath.Join(dir, "daemon.lock")`), with a private filename const `daemonLockName = "daemon.lock"` for symmetry with neighbouring entries.
  - Open with `os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)`. Do NOT use `MkdirAll` or otherwise create `<stateDir>` — pre-existing responsibility of the caller.
  - On open error: return `nil, fmt.Errorf("open daemon.lock: %w", err)` — propagates `fs.ErrNotExist` for `<stateDir>` missing, `syscall.EACCES`, etc.
  - Call `lockAcquire(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)`.
  - On `errors.Is(err, unix.EWOULDBLOCK)` (or `syscall.EWOULDBLOCK` — match whatever sentinel `unix.Flock` returns on darwin/linux): close `f`, return `nil, ErrDaemonLockHeld`.
  - On any other flock error: close `f`, return `nil, fmt.Errorf("flock daemon.lock: %w", err)`.
  - On success: set FD_CLOEXEC via `unix.FcntlInt(f.Fd(), unix.F_SETFD, unix.FD_CLOEXEC)` (or `syscall.SetNonblock`-style fcntl equivalent). On FcntlInt failure, close `f` and return wrapped error — a lock without CLOEXEC violates the spec invariant.
  - Return `f, nil`. The returned `*os.File` is the lock fd; closing it (or letting the process exit) releases the lock.
- Add a package-level doc comment on `AcquireDaemonLock` explaining fd-retention contract: callers MUST retain the returned `*os.File` for the daemon process lifetime — closing it (or GC + finalizer) releases the lock. The helper itself sets no finalizer; the returned `*os.File` from `os.OpenFile` already has the runtime finalizer, but production wiring (task 1.2) keeps the fd in a package-level var so it cannot be GC'd while the daemon runs.

**Acceptance Criteria**:
- [ ] `AcquireDaemonLock(stateDir)` exists in package `state`, accepts a state directory parameter (no hardcoded path).
- [ ] Lock file path resolves to `<stateDir>/daemon.lock`; `DaemonLock(dir)` accessor exposed alongside `DaemonPID` / `DaemonVersion`.
- [ ] Lock file created with mode `0600`.
- [ ] Helper does NOT create `<stateDir>` (no `MkdirAll`); state-dir existence is caller's responsibility.
- [ ] On `unix.Flock` returning `EWOULDBLOCK` the helper returns the exported sentinel `ErrDaemonLockHeld`, distinguishable via `errors.Is`.
- [ ] On `open(2)` errors (`ENOENT`, `EACCES`, `ENOSPC`, `EMFILE`, `ENFILE`) the helper returns a wrapped error that does NOT match `ErrDaemonLockHeld`.
- [ ] On success the returned fd has `FD_CLOEXEC` set (verifiable via `unix.FcntlInt(fd, F_GETFD, 0) & FD_CLOEXEC != 0`).
- [ ] `unix.Flock` call is dispatched via the package-level `lockAcquire` seam so tests inject a fake.
- [ ] No new test pattern departures — seam style matches existing `daemonRunFunc` / `BootstrapAliveCheck`.
- [ ] Daemon-side FIFO-sweep paths reviewed and confirmed read-only — there is no daemon-side write path into the FIFO surface that two concurrent daemons could race on. `FIFOSweeper` is single-shot per process during bootstrap; daemon-side FIFO interaction is read-only. Confirmation recorded as a code-trace assertion in the task's implementation notes / commit message, not as a runtime test (matching spec § "Potentially affected" which framed this as a confirmation requirement, not a verification requirement).

**Tests**: (in a new file `internal/state/daemon_lock_test.go`)
- `"it returns ErrDaemonLockHeld when lockAcquire fake returns EWOULDBLOCK"` — install a fake `lockAcquire` returning `unix.EWOULDBLOCK`; assert `errors.Is(err, state.ErrDaemonLockHeld)` and that no `*os.File` is leaked (use a temp dir + verify only the lockfile remains on disk).
- `"it wraps non-EWOULDBLOCK flock errors as plain errors"` — fake returns `unix.EINVAL`; assert error is non-nil, `errors.Is(err, state.ErrDaemonLockHeld)` is false.
- `"it returns a wrapped open error when stateDir does not exist"` — call with a path that does not exist; assert error matches `fs.ErrNotExist` via `errors.Is`, not `ErrDaemonLockHeld`.
- `"it creates daemon.lock with mode 0600 on first acquire"` — call with `t.TempDir()`, real `lockAcquire` (stub returning nil), `os.Stat` the lockfile, assert `mode & 0o777 == 0o600`.
- `"it sets FD_CLOEXEC on the returned fd"` — call with `t.TempDir()`, real `lockAcquire` stub, inspect `unix.FcntlInt(fd, unix.F_GETFD, 0)` and assert `& FD_CLOEXEC != 0`.
- `"it does not create stateDir if missing"` — call with a path under `t.TempDir()` plus a non-existent sub-component; assert error is non-nil AND `os.Stat(stateDir)` returns `IsNotExist`.
- `"it accepts arbitrary stateDir parameter"` — call twice with two different `t.TempDir()` paths; assert each acquires successfully (using real flock against real fds, no fake) and the two files are independent.

**Edge Cases**:
- EWOULDBLOCK surfaces as a distinct exported sentinel so the caller's contention branch is unambiguous; the open(2) error families are NOT collapsed into the sentinel.
- FD_CLOEXEC must be asserted on the fd actually returned to the caller (not on a copy) so a future refactor that re-opens the lockfile cannot regress the invariant silently.
- The helper deliberately does not call `MkdirAll` because the spec calls out state-dir existence as a pre-existing caller responsibility (`internal/state/daemon_state.go` already relies on `EnsureDir` upstream). Re-creating the directory here would mask programmer errors in the call chain.
- The lock file mode is `0600` to match other portal state files (`daemon.pid`, `daemon.version` write via `AtomicWrite` which inherits from `os.CreateTemp`'s default; `daemon.lock` here explicitly enforces 0600 on the create flag).

**Context**:
> The fix design specifies (spec § Fix Part 1 → Behaviour, Lock-file create / open semantics, Placement and structure):
> - `unix.Flock(fd, LOCK_EX|LOCK_NB)` — exclusive, non-blocking.
> - Lock file mode `0600`; helper does not create `<stateDir>` itself.
> - `open(2)` failures OTHER than `EWOULDBLOCK` (which comes from `flock`, not `open`) are treated as fatal at the call-site level — distinct from contention.
> - The acquire call must be seamed via a package-level `var lockAcquire = unix.Flock` matching existing `daemonRunFunc` / `daemonShutdownFunc` pattern.
> - FD_CLOEXEC is load-bearing — fd must not leak to forked children.
> - `unix.Flock(LOCK_EX|LOCK_NB)` is POSIX BSD advisory locking; works on darwin (affected platform) and linux (CI). Windows is not a supported portal platform.
>
> Forward-compatibility rationale: singleton-ness is a structural property that future seams (centralised hook queue, single-writer log channel) may depend on. The lock is the floor that holds this property even under code paths not yet written.

**Spec Reference**: `.workflows/multiple-state-daemons-running-concurrently/specification/multiple-state-daemons-running-concurrently/specification.md` § Fix Part 1: Daemon-Side Singleton Lock (Behaviour, Lock-file create / open semantics, Placement and structure)

## multiple-state-daemons-running-concurrently-1-2 | approved

### Task 1.2: Wire lock acquisition into daemon startup before WritePIDFile

**Problem**: The lock helper from Task 1.1 is inert until the daemon-startup path actually invokes it. The structural invariant (N ≤ 1 daemons per state directory) only holds when `AcquireDaemonLock` runs BEFORE `state.WritePIDFile` — otherwise a loser daemon overwrites the pidfile with its own PID before discovering it lost the lock, leaving `BootstrapAliveCheck` permanently misaligned with the actual lock-holder. The lock fd must also be retained for the daemon's entire process lifetime so the kernel does not release the lock while ticks are still running.

**Solution**: In `cmd/state_daemon.go`'s `stateDaemonCmd.RunE`, insert a call to `state.AcquireDaemonLock(dir)` immediately after the existing defensive `os.Remove(state.SaveRequested(dir))` and BEFORE `state.WritePIDFile(dir, os.Getpid())`. On success, assign the returned `*os.File` to a package-level `var daemonLockFile *os.File` so the fd lives for the lifetime of the process and cannot be reclaimed by Go's GC mechanism. On `errors.Is(err, state.ErrDaemonLockHeld)`, log a single WARN line via the existing logger under `ComponentDaemon` and return `nil` from `RunE` so cobra exits status 0 (matching the spec's loser-exits-clean behaviour). On any other error, log at ERROR level and return the wrapped error so cobra exits non-zero.

**Outcome**: A daemon that wins the lock proceeds to write its pidfile, write its version file, and enter the tick loop, retaining the lock fd for the lifetime of the process. A daemon that loses the lock emits exactly one WARN-level log line and exits status 0 without overwriting `daemon.pid`. A daemon that hits an open(2) failure emits an ERROR-level line and exits non-zero. The ordering invariant (acquire before WritePIDFile) is observable: after a failed-acquire test, `daemon.pid` is byte-identical to its pre-test state (or absent on a fresh state directory).

**Do**:
- In `cmd/state_daemon.go`, add a package-level `var daemonLockFile *os.File` at the top of the file (alongside `daemonRunFunc` / `daemonShutdownFunc`). Document the load-bearing fd-retention contract in a comment: closing this fd (or letting it be GC'd) releases the kernel lock; production wiring retains it for the lifetime of the daemon process.
- In `stateDaemonCmd.RunE` (between line 224 `_ = os.Remove(state.SaveRequested(dir))` and line 226 `state.WritePIDFile(...)`):
  - Call `lockFile, err := state.AcquireDaemonLock(dir)`.
  - On `errors.Is(err, state.ErrDaemonLockHeld)`: `logger.Warn(state.ComponentDaemon, "another daemon holds the lock; exiting")` then `return nil`. Do NOT call `WritePIDFile`, `WriteVersionFile`, `SeedHashMap`, `ReadIndex`, or `daemonRunFunc`.
  - On any other non-nil error: `logger.Error(state.ComponentDaemon, "acquire daemon lock: %v", err)` then `return fmt.Errorf("acquire daemon lock: %w", err)`. RunE returning a non-nil error causes cobra to exit non-zero.
  - On nil error: assign `daemonLockFile = lockFile`. The package-level var prevents GC collection of the `*os.File`.
- Verify no `runtime.SetFinalizer` is introduced on `lockFile` anywhere in the call chain. The default `*os.File` finalizer (set by `os.OpenFile` in the stdlib) would close the fd on GC — but because we pin the file in a package-level var with a process-lifetime reachability path, the finalizer cannot fire while the daemon is running. Document this reasoning in a comment on `daemonLockFile`.
- Add a `daemonAcquireLockFunc` seam if (and only if) tests cannot replace `state.AcquireDaemonLock` directly via the `lockAcquire` seam from Task 1.1 — the spec explicitly avoids introducing new seams when an existing one suffices. Prefer driving tests through the Task 1.1 `lockAcquire` seam plus `PORTAL_STATE_DIR`.
- Do NOT introduce a `WritePIDFile` seam. The ordering assertion uses observable filesystem state (pidfile absent / unchanged after failed acquire), not seam-injection.

**Acceptance Criteria**:
- [ ] `state.AcquireDaemonLock` is called from `stateDaemonCmd.RunE` after the `os.Remove(state.SaveRequested(dir))` line and before `state.WritePIDFile`.
- [ ] On success, the returned `*os.File` is assigned to a package-level `var daemonLockFile *os.File` so it cannot be GC-collected for the lifetime of the daemon process.
- [ ] No `runtime.SetFinalizer` is added; the default `*os.File` finalizer is suppressed by package-level retention, not by any explicit `SetFinalizer(f, nil)` call (which would be a no-op safeguard but is not required).
- [ ] On `ErrDaemonLockHeld`: exactly one WARN-level log line is emitted under `ComponentDaemon`, `daemon.pid` is NOT written (or, if pre-existing, NOT overwritten), `daemon.version` is NOT written, the tick loop is NOT entered, and `RunE` returns `nil` (cobra exit status 0).
- [ ] On other lock errors (e.g. `fs.ErrNotExist`, `syscall.EACCES`): exactly one ERROR-level log line is emitted, `RunE` returns a non-nil error (cobra exit non-zero).
- [ ] The existing test `TestStateDaemon_WritesPIDFileOnStartup` and friends continue to pass — the lock seam defaults to a real `unix.Flock` that will succeed against a fresh `t.TempDir()`, so non-contention tests are unaffected.
- [ ] Tests use `t.TempDir()` via `PORTAL_STATE_DIR` for isolation; tests do not use `t.Parallel()`.
- [ ] Project `CLAUDE.md` `state` package row updated to note the `daemon.lock` singleton invariant — one short sentence in the existing row format, surfaced alongside the existing `BootstrapPortalSaver` / `IsRestoringSet` references. This addresses the spec's "to be evaluated during planning" disposition and keeps the doc + behaviour in a single commit.

**Tests**: (in `cmd/state_daemon_test.go`)
- `"it acquires the lock before WritePIDFile on the happy path"` — install a `lockAcquire` fake that records call order and returns success; install `withImmediateRun` so `daemonRunFunc` returns nil; run `runStateDaemon(t)`; assert no error, pidfile present and equals `os.Getpid()`, and (via the recorded call sequence) lockAcquire is called before any filesystem write to `daemon.pid` — alternatively assert by ordering of an injected `daemonRunFunc` callback that the file exists at that point.
- `"it exits status 0 and does not overwrite daemon.pid when the lock is held"` — pre-seed `daemon.pid` with a known sentinel content (e.g. `"99999\n"`); install a `lockAcquire` fake returning `unix.EWOULDBLOCK`; run `runStateDaemon(t)`; assert `err == nil` (cobra exit 0), assert `daemon.pid` content is byte-identical to the pre-seeded sentinel, assert no `daemon.version` was written, assert one WARN line in stderr/log output.
- `"it does not write daemon.pid when the lock is held on a fresh state directory"` — `t.TempDir()` with no pre-seeded files; lockAcquire fake returns EWOULDBLOCK; run; assert `state.ReadPIDFile(dir)` returns `state.ErrPIDFileAbsent`.
- `"it returns a non-zero error when AcquireDaemonLock fails with a non-EWOULDBLOCK error"` — lockAcquire fake returns `unix.EINVAL` (or similar non-EWOULDBLOCK); run; assert `err != nil`, assert daemon.pid was not written, assert one ERROR-level log line.
- `"it retains the lock fd in a package-level var across the daemon lifetime"` — install a `lockAcquire` fake returning success; install `daemonRunFunc` that captures `daemonLockFile` (the package var) and asserts it is non-nil before returning. Run, then after `RunE` returns assert `daemonLockFile != nil` (covers retention from outside the run function too).
- `"it emits one WARN line on lock-contention exit"` — capture log output (via the existing log-capture helpers used by `TestStateDaemon_StartupLogIncludesVersionAndPID`); lockAcquire returns EWOULDBLOCK; assert exactly one line at WARN level under `ComponentDaemon` is present; literal text not asserted (per spec: log content illustrative, presence load-bearing).

**Edge Cases**:
- Ordering is asserted via observable filesystem state — no new `WritePIDFile` seam is introduced. The pidfile-absent-or-unchanged-after-failed-acquire assertion is the load-bearing check.
- The package-level `daemonLockFile` var is the load-bearing retention mechanism. Tests that reset package state between runs (e.g. `t.Cleanup`) must NOT close `daemonLockFile` mid-test in a way that affects other tests — each test isolates via `PORTAL_STATE_DIR=t.TempDir()` so each acquires its own lockfile.
- The WARN message text is illustrative per the spec; tests assert presence + WARN level, not literal text.
- Cobra's `RunE` returning `nil` makes cobra exit status 0; returning an error makes it exit non-zero. This is the exit-status mechanism — no `os.Exit` calls are added.
- Empty state directory: `AcquireDaemonLock` is called AFTER `state.EnsureDir(...)` (line 207), so `<stateDir>` is guaranteed to exist at the acquire call site.

**Context**:
> The fix design specifies (spec § Fix Part 1 → Behaviour, Placement and structure, Acceptance Criteria):
> - Lock acquisition belongs in `cmd/state_daemon.go`, before the call to `state.WritePIDFile` at line 226.
> - Fd retention is load-bearing: the lock fd MUST be held in a variable that lives for the lifetime of the daemon process. A package-level `var` in the daemon command is explicitly suggested. The fd MUST NOT be allowed to go out of scope; if a future refactor wraps the fd in a value with a finalizer, the finalizer must not close the fd.
> - Contention path: WARN + exit 0 ensures tmux does not treat this as an abnormal session termination.
> - Open(2) errors: ERROR + non-zero exit; distinct from contention (silent corruption is the alternative; loud failure is preferred).
> - Pidfile becomes "informational and authoritative" — it always reflects the single daemon that won the lock.
>
> Observability constraint: at most two new WARN-class log lines across the bug surface. This task introduces one of them (lock contention). The other (barrier timeout) belongs to Phase 2.

**Spec Reference**: `.workflows/multiple-state-daemons-running-concurrently/specification/multiple-state-daemons-running-concurrently/specification.md` § Fix Part 1: Daemon-Side Singleton Lock (Behaviour, Placement and structure, Compatibility with the existing pidfile, Observability)

## multiple-state-daemons-running-concurrently-1-3 | approved

### Task 1.3: Regression test — kernel releases lock fd on abrupt daemon exit

**Problem**: The lock-cleanup-on-crash invariant ("a daemon that crashes — panic, SIGKILL, OS reboot — releases the lock via kernel fd cleanup; the next daemon acquires cleanly with no stale-lockfile dance") is a structural property of `unix.Flock` semantics, not of portal's own code. But a future refactor could introduce a finalizer that closes the fd prematurely, or replace `unix.Flock` with a primitive whose semantics differ on abrupt exit (e.g. lockfile-based locks that DO leave stale state). A regression test that exercises this against the REAL `unix.Flock` is the canonical guard.

**Solution**: Write a regression test in `cmd/state_daemon_test.go` (or a new sibling file) that drives two `AcquireDaemonLock` calls against the same `t.TempDir()` lockfile, where the FIRST call's fd is released via a mechanism analogous to abrupt-exit kernel cleanup — specifically, by closing the `*os.File` returned from the first acquire (which is how the kernel releases the lock when the holding process exits regardless of cause). The SECOND call must succeed against the real `unix.Flock` (no `lockAcquire` seam injection) — proving the lock state is clean post-release. Optionally extend with a subprocess-based variant that fork-execs a small helper, SIGKILLs it mid-hold, and observes a parent re-acquire succeed; the subprocess variant strengthens the assertion against OS-level cleanup but is not strictly required if `Close()` semantics are sufficient.

**Outcome**: A test in the regression suite asserts that the lock is released cleanly when the lock-holding fd is closed (the kernel's exit-time cleanup hook), and that the next acquire against the same lockfile succeeds with no manual unlink or other cleanup dance. The test exercises the real `unix.Flock` syscall on the real lockfile — it does not use the `lockAcquire` seam.

**Do**:
- Add a test `TestAcquireDaemonLock_KernelReleasesOnFDClose` in `internal/state/daemon_lock_test.go` (or `cmd/state_daemon_test.go` per the spec's location guidance — Test Strategy → Regression test specifies "alongside the singleton-lock unit tests in `cmd/state_daemon_test.go`, or as a small integration test if real-process simulation is needed"; prefer `internal/state/daemon_lock_test.go` since the helper lives there and the test exercises the helper directly without cobra wiring).
- Within the test:
  - Create a `t.TempDir()` as `stateDir`.
  - Call `f1, err := state.AcquireDaemonLock(stateDir)` — assert `err == nil`, `f1 != nil`.
  - Attempt `f2, err := state.AcquireDaemonLock(stateDir)` — assert `errors.Is(err, state.ErrDaemonLockHeld)`, `f2 == nil`. (This proves the first lock is genuinely held.)
  - Close `f1` via `f1.Close()` (simulates kernel-level fd cleanup on process exit).
  - Call `f3, err := state.AcquireDaemonLock(stateDir)` — assert `err == nil`, `f3 != nil`. (This is the load-bearing assertion: re-acquire succeeds with no manual cleanup.)
  - Close `f3` via `t.Cleanup(func() { f3.Close() })` so the test does not leak the fd.
- (Optional, defer to follow-up if subprocess fixtures are heavy) — add a SIGKILL variant: `TestAcquireDaemonLock_KernelReleasesOnSIGKILL` that fork-execs `os.Args[0]` with a magic env var sentinel telling it to acquire the lock and `select{}` forever, then SIGKILLs the child and asserts a parent-side re-acquire succeeds. This is the strongest possible assertion but is only useful if `Close()` is considered insufficient simulation. Given the spec says "abrupt exit / SIGKILL simulation" the `Close()` variant satisfies the simulation requirement — the SIGKILL variant is a strength bonus, not a correctness gate.
- Do NOT install a `lockAcquire` fake in this test — the whole point is to exercise the real `unix.Flock` syscall.
- Use `t.TempDir()` for isolation; no `t.Parallel()`.

**Acceptance Criteria**:
- [ ] Test exists and is registered under `cmd/state_daemon_test.go` or `internal/state/daemon_lock_test.go`.
- [ ] Test uses the real `unix.Flock` syscall (no `lockAcquire` seam injection).
- [ ] Test asserts re-acquire succeeds after the first fd is closed (no `os.Remove(lockfile)` dance; the test does NOT manually unlink the lockfile between acquires).
- [ ] Test asserts intermediate contention: while the first fd is held, a second `AcquireDaemonLock` returns `ErrDaemonLockHeld` (proving the first lock is genuinely active).
- [ ] Test uses `t.TempDir()` for isolation; does not use `t.Parallel()`.
- [ ] Test passes against current `unix.Flock` semantics on darwin AND linux (no platform skip required — the spec calls out both as supported).

**Tests**:
- `"it allows re-acquisition after the lock fd is closed (kernel cleanup simulation)"` — described in detail above.
- `"it rejects concurrent acquisition while the first fd is held"` — inline within the same test or as a peer test, asserts that contention is observable before the close (otherwise the close-then-reacquire assertion is meaningless because there was never a held lock to release).

**Edge Cases**:
- Closing the `*os.File` is the standard idiom for releasing a `unix.Flock`-acquired lock; the kernel maps process exit to "close all fds" so `Close()` is a faithful simulation. The spec accepts this: "release the lock via kernel fd cleanup" — Close() invokes the same cleanup path the kernel runs at exit.
- The test must NOT depend on filesystem-level state cleanup between acquires (e.g. `os.Remove(lockfile)`). The whole point of choosing `flock` over `O_EXCL` is that no stale-file dance is needed; the test verifies this property.
- Future refactor concern: if a contributor wraps the lock fd in a struct with a `finalizer` that closes the fd, the regression test still passes (because we explicitly Close()) — but the production fd-retention contract (Task 1.2) is what guards against that footgun. This test guards against the OTHER direction: someone replacing `flock` with a lockfile-based primitive whose semantics LEAK on abrupt exit.

**Context**:
> The spec calls this out as a required regression test:
>
> > **Lock cleanup on crash**
> > - A daemon that crashes (panic, SIGKILL, OS reboot) releases the lock via kernel fd cleanup. The next daemon startup acquires cleanly without a stale-lockfile dance.
> > - Verified by: a regression test that simulates abrupt exit and confirms the next acquisition succeeds.
>
> And in Test Strategy → Regression test:
>
> > **Flock loser exits cleanly, leaving empty `_portal-saver` session** — simulate two daemon startup attempts where the second loses the lock and exits status 0. Verify the next bootstrap call recovers...
>
> (The flock-loser-recovery aspect is split into Task 1.4; THIS task covers only the kernel-cleanup-on-abrupt-exit property.)

**Spec Reference**: `.workflows/multiple-state-daemons-running-concurrently/specification/multiple-state-daemons-running-concurrently/specification.md` § Acceptance Criteria → Lock cleanup on crash; Test Strategy → Regression test — flock-loser recovery (kernel-cleanup half)

## multiple-state-daemons-running-concurrently-1-4 | approved

### Task 1.4: Flock-loser recovery via tolerant-kill-and-recreate

**Problem**: When two daemons race for the lock, the loser exits status 0 as the initial process of `_portal-saver`. Default tmux behaviour (no `remain-on-exit`) then closes the window — and since the session has only one window, the session itself closes. The next bootstrap therefore typically observes `HasSession(_portal-saver) == false` and falls through to `createPortalSaverWithRetry`. If `remain-on-exit` is in effect for any reason, the session lingers with a dead pane, hits the stale-pidfile recovery branch in `BootstrapPortalSaver`, and recreates. Both convergence paths must actually work end-to-end against the real bootstrap code — otherwise the singleton lock's contention path leaks broken state instead of healing it.

**Solution**: Write a regression test that exercises the loser-recovery convergence: simulate a daemon-startup attempt that loses the lock (via the Task 1.1 `lockAcquire` seam returning EWOULDBLOCK) and exits status 0, leaving `_portal-saver` in the post-loser aftermath state; then invoke `BootstrapPortalSaver` (or `EnsurePortalSaverVersion`) and assert it converges to a healthy state via the tolerant-kill-and-recreate branch. Two sub-cases cover the two aftermath shapes: empty-session (tmux closed the window after the loser exited) and dead-pane-under-remain-on-exit. Both should call into the same recovery path. Implement at the unit-test seam level — mock the tmux client + `BootstrapAliveCheck` seam — because the load-bearing real-tmux integration test belongs to Phase 2.

**Outcome**: A unit-level regression test confirms `BootstrapPortalSaver` recovers from the post-loser aftermath state in both shapes (empty session, dead-pane session), invoking the create-session branch (empty case) or the kill+create branch (dead-pane case), without leaving the user in a broken state. The test does NOT require a real tmux process — it uses the existing `MockCommander` + `BootstrapAliveCheck` seams from `internal/tmux/portal_saver_test.go`.

**Do**:
- Add a test `TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession` to `internal/tmux/portal_saver_test.go`:
  - Use `MockCommander` + `portalSaverScript` (existing helpers in that file).
  - Aftermath: tmux has already closed the saver window/session because the loser exited. `script.hasSession` returns `(_, error)` on the first call (the "absent" shape, matching `TestBootstrapPortalSaver_CreatesOnFreshServer`).
  - `script.newSession` returns success.
  - `script.setOption` returns success.
  - `BootstrapAliveCheck` is stubbed to false (irrelevant when session absent, but spec consistency).
  - Assert: exactly 1 new-session call, exactly 1 set-option call, zero kill-session calls (no prior session to kill). The recovery converges via the create-from-fresh branch.
- Add a test `TestBootstrapPortalSaver_RecoversFromFlockLoserDeadPaneSession` to `internal/tmux/portal_saver_test.go`:
  - Aftermath: `remain-on-exit` left the session in place but with a dead pane. The session is present (`HasSession` returns true) but the daemon is dead — exactly the stale-pidfile recovery branch.
  - `script.hasSession` returns `("", nil)` (present) on the first call.
  - `BootstrapAliveCheck` is stubbed to false (the loser exited; no live daemon).
  - `script.killSession` returns success.
  - `script.newSession` returns success.
  - `script.setOption` returns success.
  - Assert: 1 kill-session call, 1 new-session call, 1 set-option call, in order kill → new (existing `TestBootstrapPortalSaver_KillsAndRecreatesWhenSessionExistsButDaemonDead` already asserts this ordering pattern; copy that approach).
- Both tests must use `stubAliveCheck(t, false)` and `shrinkRetryDelay(t)` — the existing helpers in `portal_saver_test.go`.
- No `t.Parallel()`.
- No new test seams introduced; both cases are already covered by existing seams (`MockCommander`, `BootstrapAliveCheck`).

**Acceptance Criteria**:
- [ ] Test `TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession` exists and asserts: 1 new-session call, 1 set-option call, 0 kill-session calls (the empty-session aftermath case where tmux closed the window when the loser exited).
- [ ] Test `TestBootstrapPortalSaver_RecoversFromFlockLoserDeadPaneSession` exists and asserts: 1 kill-session call, 1 new-session call, 1 set-option call, with kill ordered before new (the dead-pane aftermath case where `remain-on-exit` kept the session present with a dead pane).
- [ ] Both tests are placed in `internal/tmux/portal_saver_test.go` and use the existing `MockCommander` + `BootstrapAliveCheck` seams; no new seam introduced.
- [ ] Tests pass without launching real daemon processes or real tmux servers.
- [ ] Tests do not use `t.Parallel()`.

**Tests**:
- `"it recovers from an empty _portal-saver session left behind by a flock loser (no remain-on-exit)"` — described above; uses `script.hasSession` returning "absent", expects create-from-fresh.
- `"it recovers from a dead-pane _portal-saver session left behind by a flock loser (remain-on-exit in effect)"` — described above; uses `script.hasSession` returning "present", `BootstrapAliveCheck` returning false, expects kill + create.

**Edge Cases**:
- The two aftermath shapes (empty-session and dead-pane-session) are both valid post-loser states per the spec — the test must cover both, not just one. Picking only the common case (empty-session) would leave the `remain-on-exit` variant unguarded.
- The "next bootstrap converges" assertion is what makes this a useful regression test: it proves the recovery path is connected to the existing `BootstrapPortalSaver` code without requiring real flock contention or real daemon processes. The Phase 2 integration test (singleton invariant under real tmux) is the load-bearing end-to-end assertion; this test is the cheaper unit-level safety net.
- This task does NOT exercise the kill barrier from Phase 2. Both tests run against the current `BootstrapPortalSaver` shape (Phase 1 does not touch `internal/tmux/portal_saver.go`). When Phase 2 lands and adds the barrier into the kill paths, these tests will continue to pass because the barrier returns immediately on "no prior PID" / "PID already dead" — which is exactly the state these aftermath tests leave behind.

**Context**:
> The spec specifies (§ Fix Part 1 → Loser-daemon session aftermath):
>
> > When the loser exits status 0 as the initial process of `_portal-saver`, default tmux behaviour (no `remain-on-exit`) closes the window — and since the session has only that one window, the **session itself closes**. The next bootstrap therefore typically observes `HasSession(_portal-saver) == false` and falls through to `createPortalSaverWithRetry` (no kill barrier invoked — there is no prior session to kill). If `remain-on-exit` is in effect for any reason, the next bootstrap observes the session with a dead pane, hits the stale-pidfile recovery branch, runs the barrier (which returns immediately because the prior PID is already dead), and recreates. Both convergence paths are recoverable.
>
> And in Test Strategy → Regression test — flock-loser recovery:
>
> > Flock loser exits cleanly, leaving empty `_portal-saver` session — simulate two daemon startup attempts where the second loses the lock and exits status 0. Verify the next bootstrap call recovers via the tolerant-kill-and-recreate branch in `BootstrapPortalSaver` (the empty session is detected as having no live daemon, gets killed via the barrier, and is recreated).
>
> The spec's recovery-test phrasing mentions "via the barrier" — but Phase 1 lands before the barrier exists (Phase 2). Per the planning order rationale, Phase 1 must work without the barrier; the existing `BootstrapPortalSaver` tolerant-kill-and-recreate branch is the convergence mechanism here, and the barrier becomes the silencer (not the convergence enabler) when Phase 2 lands.

**Spec Reference**: `.workflows/multiple-state-daemons-running-concurrently/specification/multiple-state-daemons-running-concurrently/specification.md` § Fix Part 1: Daemon-Side Singleton Lock → Loser-daemon session aftermath; § Test Strategy → Regression test — flock-loser recovery
