---
status: in-progress
created: 2026-05-11
cycle: 1
phase: Gap Analysis
topic: multiple-state-daemons-running-concurrently
---

# Review Tracking: multiple-state-daemons-running-concurrently - Gap Analysis

## Findings

### 1. Lock fd lifetime — variable retention not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 1 — Behaviour, Placement and structure

**Details**:
The spec states "Lock is held for the lifetime of the process and released by the kernel on exit." For this guarantee to hold, the fd must be retained in a long-lived variable — if the helper returns and the fd goes out of scope without `runtime.KeepAlive` or similar, Go's runtime is free to finalize/close it, which would release the lock while the daemon is still running. The spec does not state where the fd lives (package-level var? returned and stored on a daemon struct? stored in a closure captured by the run loop?). An implementer could plausibly write a helper that opens, flocks, and returns — and silently introduce the very race the lock is meant to close.

This is Critical: getting it wrong would defeat Part 1 entirely while passing unit tests that mock `lockAcquire`.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. Lock file creation — directory existence and open-error handling unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 1 — Behaviour

**Details**:
The lock helper opens `<stateDir>/daemon.lock` before any other state-dir write. The spec covers two outcomes of `unix.Flock`: success and `EWOULDBLOCK`. It does not address:

- What happens if `<stateDir>` does not yet exist (first ever daemon startup on a fresh install). Today `WritePIDFile` happens later via `fileutil.AtomicWrite` — does the lock helper need to `MkdirAll` first, or does it rely on something earlier in the path?
- The `open(2)` syscall preceding `flock(2)` can fail for reasons other than contention: EACCES (permissions), ENOSPC (disk full), ENOENT (parent missing), EMFILE/ENFILE (fd exhaustion). Spec does not state daemon behaviour on these — should the daemon exit non-zero with a fatal log? Exit zero to avoid abnormal-termination from tmux's view? Fall back to running without a lock?
- The file mode for create — `0600` matches portal's other state files but is not stated.

An implementer would pick a default, and that default could mask future bugs (e.g. silent fall-back without the lock invariant).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. Pidfile cleanup on graceful daemon shutdown — unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 1 — Compatibility with the existing pidfile; Acceptance Criteria — Pidfile remains coherent

**Details**:
The acceptance section says "After every successful bootstrap, `daemon.pid` reflects the PID of the currently-running daemon (the lock-holder)." But if a lock-holding daemon receives SIGHUP/SIGTERM and exits gracefully (no successor yet), the pidfile is left pointing to a now-dead PID until the next bootstrap. The spec does not state whether graceful shutdown deletes the pidfile.

Current behaviour (pre-fix): pidfile is informational and is checked via `BootstrapAliveCheck` which signal-0 probes — stale-after-exit is tolerated. With the lock as the authoritative invariant, this is probably still fine. But the spec asserts "always reflects the single daemon that won the lock" — strictly, "always" is only true while that daemon runs. Worth either weakening the claim or specifying explicit cleanup on graceful exit.

This also matters for the kill barrier: between barrier-completed-the-prior-daemon-is-dead and new-daemon-writes-pidfile, the stale PID is still in the file. Whether that intermediate-window read matters depends on `BootstrapAliveCheck` semantics, which the spec assumes are unchanged.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. Loser-daemon tmux session lifecycle — what tmux does with the empty session

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Risk and Rollout — Upgrade behaviour step 5; Fix Part 1 — Behaviour (contention path)

**Details**:
When a daemon loses the lock and exits status 0 immediately, it was launched as the initial process of `_portal-saver` via `NewDetachedSessionNoCwd(name, "portal state daemon")`. With default tmux `remain-on-exit` behaviour, an exiting initial process causes tmux to close the window and (if the only window) the session.

The Upgrade behaviour section describes step 5 as "the next `portal` command observes an empty (or dead-daemon) `_portal-saver` session" — but it's unclear whether the loser-daemon path produces (a) a closed session, (b) a session with a dead pane that remain-on-exit kept around, or (c) something else. Each has different recovery semantics:

- (a) `HasSession(_portal-saver)` returns false → `BootstrapPortalSaver` falls through to `createPortalSaverWithRetry` — no kill needed, barrier not invoked.
- (b) Session exists but has no live daemon → stale-pidfile branch fires → barrier runs against the loser's (now dead) PID → returns immediately → recreates.

Both are recoverable, but the convergence test in Test Strategy ("flock loser exits cleanly, leaving empty `_portal-saver` session") presumes path (b). If real tmux defaults produce (a), the test's setup is wrong.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 5. Concurrent bootstrap invocations — barrier has no mutual exclusion

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 2 — Behaviour; Acceptance Criteria — Singleton invariant

**Details**:
The barrier protects a single bootstrap invocation against the prior daemon, but two `portal` commands started in close succession (e.g. user fires `portal open` twice in different terminals, or scripts/automation invokes portal) both enter their own `EnsurePortalSaverVersion`. They both read the same prior PID, both call KillSession (idempotent / tolerant), both poll the same dying PID, and both then call `BootstrapPortalSaver` → `createPortalSaverWithRetry` → start a new daemon.

Two new daemons race the lock. The lock catches it (one wins, one exits with WARN). But this is now a routine occurrence, not a rare contention. The "no WARN on common case" acceptance criterion may not hold for users who routinely run multiple portal commands concurrently.

The spec acknowledges Part 1 is "the floor that holds even if every other guard fails" — so functionally safe. But the acceptance criterion "No WARN line about lock contention is emitted on the common-case recycle path" is ambiguous about whether concurrent-bootstrap is considered "common case" or not.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 6. Integration test — forcing version mismatch is not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Strategy — Integration test

**Details**:
The integration test reads: "run `EnsurePortalSaverVersion` to create the saver, then run it again with a forced version mismatch to trigger the recycle." But the mechanism for forcing the mismatch is not specified. Options visible from the codebase:

- Manually write a different value to `<stateDir>/daemon.version` between calls.
- Override the `PortalSaverVersion` / `version` constant via build flag or test seam.
- Add a new test seam in `EnsurePortalSaverVersion` to inject the comparison.

Each has different implications (the first is closest to no-new-seam; the second drags in ldflag/test-build infrastructure; the third adds API surface). An implementer would need to make this call — and the choice affects what is being asserted (real comparison logic vs forced branch).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 7. `@portal-restoring` window vs 5 s barrier — interaction unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 2 — Behaviour; Risk and Rollout

**Details**:
Bootstrap step 3 sets `@portal-restoring`; step 4 (EnsureSaver) is where the new kill barrier fires; step 7 clears `@portal-restoring`. The barrier can hold step 4 for up to 5 s on timeout, extending the total `@portal-restoring` window by up to 5 s.

CLAUDE.md states the daemon's `captureAndCommit` is suppressed while `@portal-restoring` is set, and step 6 (EagerSignalHydrate) "runs while `@portal-restoring` is still set so daemon `captureAndCommit` suppression remains in force." Whether a 5 s extension affects step 5 (Restore), step 6 (EagerSignalHydrate), or downstream warning surfacing is not addressed. Likely benign — the marker is meant to be set for the whole bootstrap — but the spec's "Critical-path latency budget" section ignores the marker window entirely.

Minor — but planners reading both this spec and the bootstrap step ordering in CLAUDE.md would benefit from an explicit confirmation that 5 s under `@portal-restoring` is acceptable.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 8. Upgrade behaviour step 3 — claim about orphans being SIGHUP'd is misleading

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Risk and Rollout — Upgrade behaviour

**Details**:
Step 3 reads: "Any **orphan daemons** (the ones whose PIDs were overwritten and are not in `daemon.pid`) are children of the tmux server. They will be SIGHUP'd when the prior `_portal-saver` session is killed — but the barrier only waits for the **one** PID it captured."

This implies the orphans receive SIGHUP from killing the current `_portal-saver`. That is incorrect as written: orphans are children of the tmux server (post-double-fork from tmux's `new-session`), but they are no longer attached to any current session — their original `_portal-saver` session was already killed and recycled at some prior bootstrap. Killing the **current** `_portal-saver` session sends SIGHUP only to processes attached to **that** session, i.e. the most recent daemon, not the orphans.

The convergence story in steps 4–6 still works (orphans drain naturally as their sweeps finish on the already-cancelled context), but the mechanism described in step 3 is wrong. An implementer or reviewer relying on this could form an incorrect mental model.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 9. WARN log content — load-bearing vs not contradiction

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 1 — Behaviour; Acceptance Criteria — Observability

**Details**:
Fix Part 1 says: *"another daemon holds the lock; exiting"* (or equivalent — log content not load-bearing, presence of the log line is).

Acceptance Criteria — Observability says: "The fix emits at most **two new WARN-class log lines** across the bug surface: 1. *"another daemon holds the lock; exiting"* 2. *"prior daemon did not exit within timeout"* (or equivalent)."

The first quotes the exact text; the body of Part 1 says content isn't load-bearing. Tests in Test Strategy say "single WARN log emitted" without asserting content. An implementer asks: do tests assert the literal string, a substring, or just presence? Pick wrong and either tests break on minor wording changes, or the acceptance section is unenforceable.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 10. Lock-acquire ordering assertion — mechanism not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Strategy — Unit tests, daemon singleton lock

**Details**:
Test case: "Acquire ordering — `lockAcquire` is called before `WritePIDFile`; reverse order would allow a loser daemon to overwrite the pidfile before exiting."

The assertion mechanism is implied but not specified: would the test inject both `lockAcquire` and a `WritePIDFile`-equivalent seam, both recording into a shared call-order log? Or would it observe filesystem state (pidfile absence after a failed acquire)? The existing daemon code calls `state.WritePIDFile` directly, not through a seam — so the test either needs a new seam for WritePIDFile (adding API surface), or asserts the negative (pidfile not present after the failure path).

Minor — but the implementer needs to decide whether to add a `WritePIDFile` seam just for this assertion.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 11. Barrier behaviour on malformed PID file content

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 2 — Behaviour (step 6); Test Strategy

**Details**:
Behaviour step 6 lists "file missing, unreadable, or empty" as the skip-barrier path. The test case lists "Prior PID file unreadable/corrupted". The behaviour section does not include "corrupted" / "contains non-numeric content" — only "unreadable, or empty". `ReadPIDFile` behaviour on garbage content (e.g. partial-write, non-numeric chars) determines whether the barrier sees a parse error (covered by "unreadable") or a successful read of value 0 (not covered).

An implementer reading only the Behaviour section may miss the malformed-content path. Aligning the two lists removes the ambiguity.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 12. Lock file path — single-server vs multi-server assumption

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Part 1 — Behaviour; Test Strategy — Test independence

**Details**:
The lock file is `<stateDir>/daemon.lock` and "the singleton invariant" is stated as "exactly one `portal state daemon` process per tmux server lifetime" (Problem Statement) and "per state directory at any time" (Acceptance Criteria).

These two scopes differ. `stateDir` is determined by config path resolution (XDG / env / fallback) and is **per-user**, not per-tmux-server. A single user running two tmux servers (e.g. one on default socket, one on a named socket) would share one `stateDir` and therefore one `daemon.lock`. The second tmux server's daemon would fail-fast on the lock.

Is this the intended behaviour? It is consistent with the Acceptance Criteria's "per state directory" framing, but inconsistent with the Problem Statement's "per tmux server lifetime" framing. Either:

- The spec should reconcile the two phrasings, or
- The state directory should be keyed per tmux server (e.g. include socket path or server PID) — which is a larger change.

This affects users who run multiple tmux servers (uncommon but supported by tmux). An implementer would default to whatever stateDir resolves to today, which means one-lock-per-user — possibly correct, but the spec doesn't say "and we accept that two tmux servers share a daemon."

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
