# Specification: Slow Open Empty Previews And Zombie Sessions

## Specification

## Problem Statement

This bugfix addresses three user-visible symptoms produced by a single underlying defect: Portal's daemon-singleton invariant is not enforced end-to-end. The same broken-singleton state surfaces as three different downstream effects.

**Symptoms:**

1. **Slow `portal open` (5–8 s)** — Every invocation pays a 5 s timeout before the TUI renders. Caused by the bootstrap kill-barrier in `killSaverAndWaitForDaemon` polling for the recorded `daemon.pid` to exit after `tmux kill-session _portal-saver`; when the recorded daemon is not the saver pane's process, the kill is structurally unreachable and the barrier always times out at its 5 s limit. `portal open` is expected to be sub-second.

2. **Empty session previews** — Pressing `Space` on any session in the picker shows "no saved content" even though the scrollback exists inside tmux. Caused by competing daemons each running `gcOrphanScrollback` against the same state directory with divergent indexes — the scrollback directory oscillates between 0 and 1 `.bin` file as each daemon's commit deletes files referenced only by the other's view. Expected: the highlighted session's captured scrollback renders in the preview pane.

3. **Killed sessions resurrect** — Sessions removed via `K` in the picker (or via the user's `Option-Q` tmux shortcut) reappear on the next `portal open` and persist indefinitely. Caused by multiple daemons independently committing `sessions.json` every tick — the legitimate daemon's post-kill commit (without the dead session) is overwritten seconds later by a competing daemon whose stale `prev` state still includes it. Restore on next bootstrap reconstructs the dead session as a skeleton pane. Expected: `K` removes the session permanently.

## Scope

Bundle all seven fix components (A–G, defined below) into a single bugfix work unit. Each independently closes a real defect or latent fragility; the user has explicitly chosen defence-in-depth over a minimum-viable patch. The framing is "fix Portal so this type of thing never happens" — A+B+G handle the consequences and the known triggers, C closes the underlying *mechanism* (the inode-replacement gap that lets divergent daemons coexist) so unforeseen future triggers cannot recreate the same bug class, and D bounds orphan lifetime to one tick *between* bootstraps so the daemon is polite about its own existence even when no `portal` invocation runs.

## Out of Scope

- **Re-architecting the saver/daemon ownership model.** The current "saver pane process IS the daemon" model is retained; this bugfix hardens the surrounding invariants rather than replacing them.
- **Replacing `flock` with an alternative locking primitive.** Component C tightens the existing `flock`+inode contract rather than swapping primitives. The "flock `sessions.json` itself" alternative was ruled out during investigation synthesis because `fileutil.AtomicWrite0600` replaces sessions.json's inode on every Commit, which would itself break flock semantics.
- **Migrating away from per-tick `sessions.json` rewrites.** The commit + GC pipeline shape is unchanged; only per-session error tolerance and cross-daemon coexistence are hardened.

## Root Cause

Portal's daemon-singleton contract is not enforced end-to-end. Three independent assumptions in the surrounding code, each unverified at runtime, can be violated simultaneously to produce the observed state:

1. **`daemon.lock` excludes per-inode, not per-path.** `state.AcquireDaemonLock` (`internal/state/daemon_lock.go:55-77`) opens whatever inode `daemon.lock` currently resolves to and `flock`s it. There is no cross-check that the inode it locked is still the inode at the path. If `daemon.lock` is unlinked + recreated between two daemon spawns (by any external cause — older code path, manual `rm`, leaked test scaffolding), the two daemons end up `flock`-ing different inodes and the singleton invariant is silently broken. On the reporter's install, three concurrent daemons each held `flock` on a different `daemon.lock` inode (171463046, 171582571, 170216314).

2. **The kill-barrier can only reach daemons bound to the saver pane process.** `killSaverAndWaitForDaemon` in `internal/tmux/portal_saver.go:212-248` polls the recorded `daemon.pid` for death after issuing `tmux kill-session _portal-saver`. If the recorded PID is alive but not the saver pane's process (orphan from a prior bootstrap, leaked test daemon with a different parent tmux server, etc.), the kill is structurally unreachable — the polled process never exits and the barrier times out at 5 s. No SIGTERM/SIGKILL escalation is attempted.

3. **`CaptureStructure` aborts the whole tick on any per-session error.** `internal/state/capture.go:86-90` returns immediately when `ShowEnvironment` fails for any single session. The downstream `captureAndCommit` (`cmd/state_daemon.go:132-207`) then returns before writing scrollback or calling `Commit` — a single bad session at the alphabetical head poisons capture for every later session in the same tick. Latent since commit `7dc990be4` (2026-04-27), present in every v0.5.x release. The per-pane loop in `captureAndCommit:185-192` correctly logs and continues; the per-session loop in `CaptureStructure` is missing the same defensive pattern.

When these are violated together, multiple daemons concurrently write `sessions.json` and execute destructive scrollback GC against the same state directory. `gcOrphanScrollback` (`internal/state/commit.go:102-138`) deletes any `.bin` not referenced by the just-committed index — and trusts whatever index the calling daemon produced, with no cross-check against any other daemon's view. With multiple daemons each committing different views every ~1–2 s, `.bin` files are constantly being deleted and rewritten, and `sessions.json` flips between divergent session lists.

**Trigger on this install:** A test-fixture tmux server at `/tmp/test_hook_debug2/s` is still alive from the prior evening. A test binary at `/private/tmp/portalbin/portal` was launched against this socket and is still running. It inherited `XDG_CONFIG_HOME` from the user's environment because no test isolated it, so its daemon writes to the user's real state directory while enumerating sessions from the test-fixture tmux server (a single session "A"). This is the trigger but not the *cause* — the underlying defects above allow this trigger (and any unforeseen future equivalent) to produce the observed end-state.

### Symptom → mechanism mapping

- **Slow open** → kill-barrier polling an unreachable orphan PID for the full 5 s window.
- **Empty previews** → `gcOrphanScrollback` race between divergent daemons deleting each other's `.bin` writes; further amplified by the `CaptureStructure` abort-on-error path when any single session enumeration fails.
- **Zombie sessions** → competing daemon overwrites the legitimate daemon's post-kill `sessions.json` with stale `prev` state; Restore on next bootstrap reconstructs the dead session.

## Component A — Kill-Barrier Escalation

**Goal:** Make the bootstrap kill-barrier deterministically reach any prior daemon, regardless of whether the daemon is the saver pane process.

**Current behaviour** (`internal/tmux/portal_saver.go:212-248` `killSaverAndWaitForDaemon`):
1. Read `priorPID` from the kill-barrier file.
2. If `priorPID` is not alive: `tmux kill-session _portal-saver`; return.
3. Else: `tmux kill-session _portal-saver`; poll `killBarrierIsAlive(priorPID)` every 50 ms for up to 5 s; return after process death or timeout.

If `priorPID` is alive but not the saver pane's process (orphan with a different parent tmux server), `tmux kill-session` cannot reach it. The barrier polls for an exit that never happens, times out at 5 s, and proceeds.

**New behaviour:**

1. Existing steps 1–3 run unchanged.
2. **Post-poll escalation:** if `priorPID` is still alive after the 5 s session-kill poll:
   1. **Identity-check the PID.** Verify the process at `priorPID` is a `portal state daemon` — accept only if executable name is `portal` AND argv contains `state daemon`. Implementation uses `ps -o comm=,args= -p <pid>` (macOS-compatible; portable across Linux). If the check fails (PID recycled to an unrelated process, or process gone since the last poll), treat as success and return.
   2. **Send SIGKILL directly to `priorPID`.** Do NOT send SIGTERM first.
   3. Poll `killBarrierIsAlive(priorPID)` for a bounded short window (1 s).
   4. If still alive after the SIGKILL poll, log WARN under `ComponentBootstrap` and proceed — bootstrap is best-effort at this stage.

**Why SIGKILL, not SIGTERM-with-marker:**

The daemon's signal handler at `cmd/state_daemon.go:340-345` runs `defaultShutdownFlush` → `captureAndCommit` → one final destructive GC cycle on shutdown. For an orphan being deliberately killed *because its view of state is divergent*, that final flush is exactly the destructive operation we're escaping from. SIGKILL bypasses the handler entirely — no chance of one more destructive commit on the way out.

The "SIGTERM with skip-final-flush marker" alternative would require plumbing a marker through to `defaultShutdownFlush` and auditing that no future addition to the shutdown handler can fire a write. SIGKILL achieves the same guarantee structurally with no maintenance burden.

The legitimate daemon's normal saver-kill path is **unchanged**: `tmux kill-session _portal-saver` SIGHUPs the saver pane process, its handler runs, the final flush is correct because that daemon's view is correct.

**Identity-check rationale:**

Direct signalling introduces PID-recycle risk that `tmux kill-session` did not. Between the kill-barrier writing `priorPID` and bootstrap escalating to SIGKILL, the OS could recycle the PID to an unrelated process. The identity check refuses to signal anything that isn't recognisably a `portal state daemon`.

**Acceptance criteria:**

- A leaked orphan daemon (parent ≠ saver pane process; `tmux kill-session` cannot reach it) is dead within ~6 s of bootstrap entering `killSaverAndWaitForDaemon` (5 s session-kill poll + 1 s SIGKILL poll).
- The bootstrap kill-barrier no longer adds a 5 s ceiling to `portal open` when an orphan is present — under steady-state-with-orphan, total bootstrap time is reduced by ~5 s.
- Identity check prevents signalling an unrelated process if `priorPID` has been recycled.
- No final-flush GC cycle runs on orphans being escalation-killed (verified by observing scrollback dir across an escalation event — no new `.bin` writes from the killed daemon).
- The legitimate daemon's normal shutdown path is unaffected — SIGHUP from `tmux kill-session` still triggers `defaultShutdownFlush` as before.

**Files affected:** `internal/tmux/portal_saver.go` (`killSaverAndWaitForDaemon`). May introduce a small helper in `internal/state/` or a new package for the identity-check / signal primitive depending on testability needs.

## Component B — Bootstrap-Time Orphan Sweep

**Goal:** During every bootstrap, proactively detect and kill any `portal state daemon` process that isn't the saver pane's process. Composes with Component A but closes the gap earlier in the bootstrap sequence — orphan daemons stop writing to the state directory *before* `EnsureSaver` runs, so the new saver's daemon doesn't compete with an existing one for the lock or the state dir.

**Current behaviour:** No orphan sweep exists. Orphan daemons are only addressed indirectly through the kill-barrier's poll-and-wait on `priorPID`, which (per Component A) is the kill-barrier's single recorded PID, not the full pgrep set.

**New bootstrap step: `SweepOrphanDaemons`.** Inserted as a new step between `Set @portal-restoring` (current step 3) and `EnsureSaver` (current step 4). All steps from `EnsureSaver` onward shift up by one (EnsureSaver → 5, Restore → 6, etc.).

**Behaviour:**

1. Enumerate candidate orphan PIDs: `pgrep -x 'portal state daemon'` (the `-x` matches the exact command name; portable across macOS/Linux).
2. Build the legitimate set:
   - The pane process PID of `_portal-saver`'s only pane, if `_portal-saver` exists (via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`).
   - Empty set if `_portal-saver` does not exist (fresh server, post-server-restart, etc.).
3. For each candidate PID NOT in the legitimate set:
   1. **Identity-check** (same primitive as Component A): accept only if executable is `portal` and argv contains `state daemon`. Skip if the check fails.
   2. **SIGKILL** the PID. Do NOT send SIGTERM first (same reasoning as Component A — orphan view is untrusted, no final flush).
   3. Log INFO under `ComponentBootstrap`: `"sweep: killed orphan daemon pid=%d"`.
4. Return. Step is **best-effort**; any `pgrep` / `ps` / `kill` error is logged WARN and swallowed. Never escalates to a fatal abort.

**Why this composes with Component A and is not redundant:**

- Component A handles the *single* daemon the kill-barrier knows about (`priorPID` from the kill-barrier file). It cannot handle multiple orphans because the barrier only records one PID.
- Component B sweeps the *full* pgrep set. On the reporter's install (three concurrent daemons), B kills the two orphans the barrier doesn't know about and A handles the recorded one — together they make the post-bootstrap state singleton.
- B runs before `EnsureSaver` so the new saver-pane daemon's first tick is uncontested. A runs *inside* the new `EnsureSaver` flow as part of the kill-barrier escalation.

**Concurrency note:** B is non-atomic — a new `portal state daemon` could in principle appear between the `pgrep` and the `kill` step. In practice, the only legitimate spawner of `portal state daemon` is the saver pane process via `EnsureSaver`, which has not yet run at this bootstrap step. Out-of-band spawns (manual `portal state daemon` invocation, test fixture starting between the two calls) are rare and B is best-effort anyway — the next bootstrap will sweep them.

**Acceptance criteria:**

- Given N concurrent `portal state daemon` processes where N-1 are orphans (parent ≠ saver pane process; or no saver session exists), bootstrap step `SweepOrphanDaemons` kills N-1 of them. Verified by `pgrep -xc 'portal state daemon'` returning 1 (the legitimate saver-pane daemon) after the step completes.
- Given only the legitimate saver-pane daemon, the sweep sends zero signals. Verified by audit log: no `"sweep: killed orphan daemon"` entries on a clean-state bootstrap.
- Identity check prevents signalling an unrelated process if the PID has been recycled.
- Step is best-effort: any underlying error (pgrep failure, kill failure) logs WARN and does not abort bootstrap.
- Step ordering is documented in `CLAUDE.md` bootstrap section to match the new sequence.

**Files affected:** `cmd/bootstrap/` (new step + orchestrator wiring), `internal/bootstrapadapter/` (production adapter for pgrep + identity-check + kill seam), `CLAUDE.md` (bootstrap step ordering documentation).

## Component C — Stabilise the `daemon.lock` Singleton Against Inode Replacement

**Goal:** Close the inode-replacement gap so the daemon-singleton invariant cannot be silently broken when `daemon.lock`'s path is unlinked + recreated between two daemon spawns.

**Current behaviour** (`internal/state/daemon_lock.go:55-77` `AcquireDaemonLock`):
1. `os.OpenFile(daemon.lock, O_RDWR|O_CREATE, 0o600)` — opens whatever inode is at the path.
2. `flock(LOCK_EX|LOCK_NB)` on that fd.
3. Set `FD_CLOEXEC`. Return.

`flock` excludes per-**inode**, not per-path. If two daemons end up with fds to different inodes for the same path (because the file was unlinked + recreated between their opens), both `flock`s succeed and both daemons run.

**New behaviour:** Augment `AcquireDaemonLock` with two cross-checks that use the already-existing `daemon.pid` file and an inode invariant.

1. **Pre-acquire daemon.pid liveness check.** Before opening `daemon.lock`:
   1. Read `daemon.pid` via `state.ReadPIDFile`. If absent: skip; proceed.
   2. If present: check the recorded PID is alive AND identity-checks as a `portal state daemon` (same primitive as Component A — `ps -o comm=,args= -p <pid>`; accept only when executable is `portal` and argv contains `state daemon`).
   3. If both checks pass: return `ErrDaemonLockHeld`. Another `portal state daemon` already owns the singleton, regardless of whatever inode `daemon.lock` currently resolves to.
   4. If the recorded PID is dead or doesn't identity-check: proceed to step 2.
2. **Existing open + flock** (steps 1–3 of current behaviour) run unchanged.
3. **Post-flock inode cross-check.** After `flock` succeeds:
   1. `fstat` the fd to get `fd_inode`.
   2. `stat` the path to get `path_inode`.
   3. If `fd_inode != path_inode`: the file was replaced between our open and our flock. Release the flock (close the fd) and retry the whole acquire (steps 1–3). Bounded to **3 retries** with a 10 ms sleep between attempts. On persistent mismatch after the bound, return a wrapped error (treated as fatal misconfiguration — caller logs WARN and exits).
   4. If `fd_inode == path_inode`: lock acquired, proceed.
4. **Post-acquire daemon.pid write.** After successful acquire (and after the existing FD_CLOEXEC step), the caller writes `daemon.pid` atomically with the current process's PID via `state.WritePIDFile` (existing helper). The acquire helper itself does not write the PID file — that remains the daemon's responsibility, preserving the current call-site contract — but the **daemon must write `daemon.pid` before exiting `main`'s lock-acquire path**, so that any subsequent acquirer's pre-check sees an identity-checkable recorded PID.

**Why this closes the bug class:**

- The pre-check makes `daemon.pid` (a stable file whose content we control) authoritative for singleton membership, sidestepping `flock`'s per-inode limitation. Even if `daemon.lock` has been unlinked + recreated 100 times, what matters is whether `daemon.pid` references a live identity-checkable daemon.
- The inode cross-check absorbs the small race window where a third party replaces the file between our `open` and our `flock`. Bounded retry handles transient turbulence (e.g., another daemon coming up and aborting cleanly); persistent mismatch indicates a stuck-broken state that should fail loudly.
- The identity check on the recorded PID prevents a recycled-PID coincidence from blocking legitimate succession (e.g., shell pid coincidentally matches the prior daemon's PID).

**Composition with Components A and B:**

- A+B ensure that by the time the new saver-pane daemon calls `AcquireDaemonLock`, no other `portal state daemon` is alive — so the pre-check sees a dead recorded PID and proceeds.
- C is the structural defence: if A and B both somehow miss an orphan (unforeseen future trigger), the pre-check still refuses to acquire, and the loser exits cleanly via the existing `ErrDaemonLockHeld` path. Worst case becomes "saver pane process exits 0 with a WARN; bootstrap proceeds without a healthy daemon", which is degraded but not destructive — the existing `EnsureSaver` flow already surfaces a `SaverDownWarning` for that state.

**Acceptance criteria:**

- **Pre-check refuses on live recorded daemon.** Given a live identity-checkable `portal state daemon` referenced by `daemon.pid`, `AcquireDaemonLock` returns `ErrDaemonLockHeld` without opening `daemon.lock`. Verified via unit test with a real subprocess as the "live" daemon.
- **Pre-check ignores stale daemon.pid.** Given a `daemon.pid` whose recorded PID is dead, `AcquireDaemonLock` proceeds. Verified via unit test (write daemon.pid with a known-dead PID; assert acquire succeeds).
- **Pre-check ignores wrong-identity PID.** Given a `daemon.pid` whose recorded PID is alive but identity-check fails (e.g., the PID was recycled to `sleep`), `AcquireDaemonLock` proceeds. Verified via unit test (stub identity-check seam to return false).
- **Inode-mismatch retry.** Stub the post-flock inode comparison to return mismatch for the first attempt then match: `AcquireDaemonLock` succeeds on the second attempt. Verified via unit test through the existing `lockAcquire` seam plus a new stat seam.
- **Inode-mismatch retry bound.** Stub mismatch for all attempts: `AcquireDaemonLock` returns a wrapped error after 3 attempts, with bounded total delay (<100 ms). Verified via unit test.
- **No regression in EWOULDBLOCK path.** A second `AcquireDaemonLock` against the same `daemon.lock` with the original holder still alive returns `ErrDaemonLockHeld` (either via pre-check, or via the existing EWOULDBLOCK path if daemon.pid is missing). Verified via existing daemon-lock integration test.

**Files affected:** `internal/state/daemon_lock.go` (augment `AcquireDaemonLock`), `internal/state/daemon_state.go` (no changes to `WritePIDFile`/`ReadPIDFile` — used as-is), tests in `internal/state/daemon_lock_test.go`. New test seams may be added: identity-check function pointer and stat function pointer.

## Component D — Daemon Self-Supervision Against the Saver Session

**Goal:** Bound orphan-daemon lifetime to ~3–4 seconds even when no `portal` invocation runs. A and B sweep orphans at bootstrap time; D makes the daemon self-eject when its connection to `_portal-saver` no longer holds, without needing an external sweep.

**Current behaviour:** The daemon ticks forever after acquiring `daemon.lock` until it receives SIGHUP (from `tmux kill-session _portal-saver`) or a context cancellation. There is no per-tick check that the daemon is still bound to a live saver pane.

**New behaviour:** Add a per-tick "saver-membership self-check" to the daemon's main loop in `cmd/state_daemon.go`. The check runs **before** the existing `captureAndCommit`, so a failing check exits before any commit/GC writes.

**Self-check sequence:**

1. **Query saver existence:** `tmux has-session -t _portal-saver`. Treat any error (not just "session not found") as "absent" for this tick — tmux command failures are evidence the daemon's view is unreliable.
2. **If absent:** increment the in-process consecutive-absence counter.
3. **If present:** query the saver pane's pid via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`. If the result errors or yields a pid that doesn't match `os.Getpid()`: increment the counter. If the pane pid matches `os.Getpid()`: reset the counter to 0 (this daemon is still legitimately the saver pane process).
4. **If counter ≥ N (see hysteresis below):**
   1. Log INFO under `ComponentDaemon`: `"self-supervision: saver-membership lost for N consecutive ticks, exiting"`.
   2. **Skip the final flush.** Exit immediately via `os.Exit(0)` (bypassing any deferred shutdown handler) so the divergent-view daemon does NOT execute one more `captureAndCommit` / `gcOrphanScrollback` cycle on its way out — same reasoning as Component A's straight-to-SIGKILL choice.
5. **If counter < N:** continue to the existing tick body (`captureAndCommit`).

**Hysteresis N:** **3 consecutive ticks.** Rationale:

- The legitimate daemon never observes a transient "saver absent" condition. The bootstrap path that kills `_portal-saver` SIGHUPs the saver pane process — i.e., the legitimate daemon itself — so the OLD legitimate daemon stops ticking before its next self-check. The NEW legitimate daemon spawned by the recreated saver only starts ticking AFTER the saver exists. There is no in-between window where a legitimate daemon would see absence.
- The only realistic source of false-positive absence is transient tmux command failure (mid-tick `has-session` returning an unexpected error during, e.g., a heavy tmux server moment). N=3 absorbs this without significantly extending orphan lifetime.
- With the daemon's current ~1 s tick interval, N=3 caps orphan lifetime at ~3–4 s of additional drift after the saver-membership condition first fails — well inside the user's "bound to one tick *between* bootstraps" target framing.
- N=1 was considered but rejected: a single tmux-command hiccup would unnecessarily kill the legitimate daemon mid-session (extremely rare but possible).

If implementation measurement during the planning phase reveals real-world transient durations longer than ~3 ticks, N can be increased — but the spec target is "single-digit ticks", not "tens of ticks".

**Why this composes with A, B, and C:**

- A+B run only at bootstrap. Between invocations (the user closes their laptop, comes back hours later), orphans accumulate freely under the current design — the reporter's install had a 13-hour orphan-lifetime from yesterday 21:39 to detection today 10:39. D shrinks the inter-bootstrap orphan window from "hours" to "~3 seconds".
- C makes the lock-acquire path refuse the singleton on observable divergence. D makes the lock-holder self-eject when its membership becomes invalid post-acquire. Together they enforce the singleton invariant at both ends of a daemon's lifetime.

**Acceptance criteria:**

- **Self-eject on absent saver.** Spawn `portal state daemon` against a tmux server that has no `_portal-saver` session. The daemon exits within (N + 1) tick intervals. Verified by integration test.
- **Self-eject on saver pane pid mismatch.** Spawn the daemon, then externally replace the `_portal-saver` pane process (e.g., `respawn-pane` to a different process). Daemon exits within (N + 1) tick intervals. Verified by integration test.
- **No false-positive exit on legitimate transient.** Stub the saver-existence check to return "absent" for k < N consecutive ticks then "present": daemon does NOT exit, counter resets. Verified by unit test through a `saverMembershipProbe` seam.
- **No final flush on self-eject.** After the daemon self-ejects, the scrollback directory shows no new `.bin` writes from the killed daemon's PID. Verified by integration test that monitors scrollback writes around the eject event.
- **Skipped check on first tick is benign.** The legitimate daemon, ticking for the first time inside a freshly-created `_portal-saver`, passes the self-check on tick 1 (pane pid matches its pid). Verified by integration test in the existing daemon-saver suite.

**Files affected:** `cmd/state_daemon.go` (insert self-check before `captureAndCommit`), `internal/tmux/` (may add a small `SaverPanePID(name) (int, error)` helper for testability), tests in `cmd/state_daemon_test.go` plus integration coverage.

---

## Working Notes
