---
status: complete
created: 2026-05-22
cycle: 1
phase: Gap Analysis
topic: slow-open-empty-previews-and-zombie-sessions
---

Resolution summary: all 17 findings approved and applied to the specification in auto mode. Resolution fields below remain marked Pending in the per-item blocks — the authoritative status is the file-level `status: complete` plus the consolidated summary here. Highlights:

1. pgrep -fx with anchored regex (macOS compat); acceptance criteria updated to use `pgrep -fxc`.
2. New "Shared Primitive — Daemon Identity Check" section added with IdentifyResult contract and per-component error semantics.
3. Placeholder switched to `sh -c 'exec tail -f /dev/null'`; sleep infinity rejection rationale recorded.
4. Acceptance criteria for A/D reframed to scrollback-directory snapshot/delta comparison (no writer-PID attribution).
5. Component E total-failure guard now classifies per-session errors into natural-churn vs anomalous; natural-churn proceeds, anomalous skips commit.
6. Test staging note added to Component D specifying direct-spawn and state-dir prep.
7. daemon.pid write location pinned to "next statement after AcquireDaemonLock returns"; layered enforcement note added.
8. Bootstrap step ordering: full 11-step post-insertion list enumerated.
9. Component F: readiness barrier (2 s @ 50 ms cadence polling daemon.pid + identity-check) added as step 4.
10. ComponentDaemon constant confirmed (no new constant); code snippet updated.
11. Component D measurement-artefact acceptance criterion added (in-source comment citing measured value + 2× factor).
12. Component G mtime backstop: full fileFingerprint shape (exists/size/mtime/ctime/sha256 for ≤1MiB) specified; lstat semantics + scope.
13. Component C exit semantics: status 1 with WARN under ComponentDaemon for persistent-mismatch; ErrDaemonLockHeld retains status 0.
14. Component F + placeholder/B-sweep interaction note added.
15. Component A SIGKILL poll cadence pinned at 50 ms.
16. Tick-interval reference to stateDaemonTickInterval added.
17. daemon.version verification noted as observation-only downstream consequence of A+B.

# Review Tracking: slow-open-empty-previews-and-zombie-sessions - Gap Analysis

## Findings

### 1. `pgrep -x 'portal state daemon'` likely matches nothing on macOS

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component B (Bootstrap-Time Orphan Sweep), steps 1 and acceptance criteria

**Details**:
Component B specifies enumeration via `pgrep -x 'portal state daemon'` and claims "`-x` matches the exact command name; portable across macOS/Linux." On macOS, `pgrep -x` matches against the process `comm` (short name) by default — which for the daemon is `portal`, not `portal state daemon`. To match the full argv string an additional `-f` flag is required (`pgrep -fx 'portal state daemon'`), and even then quoting/spacing has to be exact. Without `-f`, the sweep enumerates nothing and Component B silently no-ops on macOS. The acceptance criterion "`pgrep -xc 'portal state daemon'` returns 1" would also be vacuous in the same way.

Component A's identity-check via `ps -o comm=,args= -p <pid>` is the same primitive but applied per-PID; the question is which command Component B uses to *enumerate* candidate PIDs in the first place. The enumeration source needs an unambiguous, OS-portable form (e.g., `pgrep -fx '^portal state daemon( |$)'` or `ps -axo pid=,comm=,args= | awk …`).

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 2. Shared identity-check primitive has no defined home

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Components A, B, C (and indirectly D's exit attribution)

**Details**:
The identity-check (exec name `portal` AND argv contains `state daemon`, via `ps -o comm=,args= -p <pid>`) is referenced by Component A ("may introduce a small helper in `internal/state/` or a new package"), Component B ("same primitive as Component A"), and Component C ("same primitive as Component A"). Three components depending on the same primitive without a single specified location risks divergent implementations or a re-litigation during planning. The "may … or a new package" hedge means an implementer is forced to pick — that's a design decision the spec defers without naming the trade-off.

Related: the spec doesn't specify the contract for when the identity-check itself errors (e.g., `ps` command fails vs. PID gone vs. PID present-but-different-binary). Component A says "If the check fails (PID recycled to an unrelated process, or process gone since the last poll), treat as success and return." Component B says "Skip if the check fails." Component C says "If the recorded PID is dead or doesn't identity-check: proceed." All three reach the right end-state but the underlying error taxonomy (transient ps failure vs. definitive negative answer) is conflated. A transient `ps` error during Component C's pre-check could let a second daemon acquire the lock alongside a live legitimate daemon.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 3. `sh -c 'sleep infinity'` placeholder is not portable to macOS

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component F (Saver Creation Sets destroy-unattached=off BEFORE Daemon Starts)

**Details**:
Spec claims "The placeholder choice (`sh -c 'sleep infinity'`) is portable across macOS and Linux." macOS' BSD `sleep` does not accept the `infinity` argument — it requires a numeric value. On macOS `sleep infinity` exits with `usage: sleep seconds`. The placeholder process would exit immediately, recreating exactly the race the component is meant to close (pane process exits → `destroy-unattached=on` default → session destroyed before `SetSessionOption` runs).

A portable form is something like `sh -c 'while :; do sleep 86400; done'`, `sh -c 'exec tail -f /dev/null'`, or a large numeric value (`sleep 2147483647`).

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 4. `.bin` writer attribution not defined for acceptance criteria

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component A acceptance ("no new `.bin` writes from the killed daemon"), Component D acceptance ("no new `.bin` writes from the killed daemon's PID")

**Details**:
Both A and D include acceptance criteria phrased as "no new `.bin` writes from the killed daemon" / "from the killed daemon's PID". The `.bin` files in the scrollback directory are not currently keyed by writer PID — they are keyed by paneKey. There is no metadata on the file that says which daemon process wrote it. Verifying this acceptance criterion requires an implementer to invent an attribution mechanism (e.g., monitor scrollback dir via fsnotify during the test window and correlate with the daemon PID's lifetime, or inject a test-only writer-identity into the filename).

The acceptance criterion is implementable but the *means* is ambiguous and would force the implementer to design the verification harness from scratch. The intent is clear ("the killed daemon must not write after escalation/eject"); a clearer phrasing would specify the observation harness (e.g., "scrollback directory mtimes/contents are stable across the eject event over an N-tick observation window").

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 5. Component E total-failure guard cannot distinguish anomaly from natural session churn

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component E (`CaptureStructure` Per-Session Log-and-Continue), "Total-failure guard"

**Details**:
The total-failure guard says: "if `len(keep) > 0 && len(sessions) == 0`, every individual session enumeration failed despite the pre-loop calls succeeding. This is anomalous … and should NOT produce a commit that wipes all scrollback. Return an error wrapping the per-session failure count, causing `captureAndCommit` to skip Commit + GC for this tick."

But `keep` is computed from the pre-loop tmux enumeration; between that enumeration and the per-session `ShowEnvironment` loop, sessions can legitimately be destroyed (user kills the last session via tmux just as the daemon is mid-tick). If `keep` contains a single session that the user destroys mid-loop, every `ShowEnvironment` call in the loop fails legitimately — and the guard treats that as the anomaly path. Result: a legitimate "user killed the only session" tick is treated as a tmux-broken tick and the commit is skipped, leaving the killed session's old state in `sessions.json` for at least one more tick. The intersection with the zombie-resurrection symptom this bugfix is closing is non-trivial — extending zombie state by one tick is precisely the wrong direction.

The guard's intent is right (don't wipe scrollback on a wholly-broken capture), but the heuristic "all sessions failed" mis-classifies the "all sessions gone" case as anomalous. A better discriminator would be: distinguish per-session "no such session" errors (natural churn — drop the session, commit) from per-session other errors (anomalous — skip the commit).

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 6. Component D test scenarios bypass Component C pre-check

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component D acceptance criteria ("Self-eject on absent saver", "Self-eject on saver pane pid mismatch")

**Details**:
Two acceptance criteria spawn `portal state daemon` against a tmux server with conditions that intentionally violate the saver-pane-process invariant (no `_portal-saver`; or `_portal-saver` pane process replaced). But Component C augments `AcquireDaemonLock` with a pre-acquire `daemon.pid` liveness check and a post-flock inode cross-check — for the test scenarios in D to even reach the tick loop, the daemon must successfully acquire the lock. The spec doesn't describe how the integration test stages the state directory so that C's pre-check passes (no daemon.pid? stale daemon.pid?).

Additionally, D's acceptance for "Self-eject on saver pane pid mismatch" requires externally replacing the `_portal-saver` pane process. The replacement process is not a `portal state daemon`; Component B's bootstrap sweep would not see it, but the integration test setup must guarantee no bootstrap runs between staging and the daemon's self-check. Whether this is staged via direct daemon spawn (bypassing the bootstrap orchestrator entirely) is not stated.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 7. Component C post-acquire `daemon.pid` write race window unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component C (Stabilise the `daemon.lock` Singleton), step 4 "Post-acquire daemon.pid write"

**Details**:
The spec defers the actual write of `daemon.pid` to the daemon caller ("the daemon must write `daemon.pid` before exiting `main`'s lock-acquire path"). Between `AcquireDaemonLock` returning and the caller running `WritePIDFile`, there is a window during which:

- A second `portal state daemon` invocation runs C's pre-check, finds the old `daemon.pid` (stale, pointing at the previous-incarnation PID or empty), determines it's not live, and proceeds.
- The second invocation then attempts `flock` and gets `EWOULDBLOCK` (legitimate daemon still holds it) — so the existing flock path covers this case.

The pre-check is the *primary* singleton enforcer per the spec's deviation note, but for newly-spawned daemons in the small window before the legitimate daemon writes its `daemon.pid`, the enforcement falls back to the flock EWOULDBLOCK path. The spec should explicitly state that this fallback is intentional and that the pre-check is *defence in depth*, not the sole mechanism. The current text says the pre-check is "authoritative for singleton membership", which doesn't match the actual layered behaviour.

Also, the daemon caller's exact write location ("before exiting main's lock-acquire path") is not pointed at a specific line/function in `cmd/state_daemon.go`. An implementer would have to choose, and a poor choice (e.g., writing after the first tick) re-opens the window.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 8. Bootstrap step ordering update missing for `CLAUDE.md`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component B (Bootstrap-Time Orphan Sweep), CLAUDE.md bootstrap section

**Details**:
Component B inserts `SweepOrphanDaemons` between current steps 3 (`Set @portal-restoring`) and 4 (`EnsureSaver`), and notes "Step ordering is documented in `CLAUDE.md` bootstrap section to match the new sequence." The current `CLAUDE.md` describes a 10-step orchestrator with specific dependencies between steps (e.g., step 7 "Clear @portal-restoring is fatal on failure"; step 8 "runs after Clear so it observes post-restore tmux state"). After insertion, the orchestrator has 11 steps and step indices throughout the doc shift.

The spec does not enumerate the new full step list (post-insertion) — it just says "All steps from `EnsureSaver` onward shift up by one (EnsureSaver → 5, Restore → 6, etc.)". An implementer must derive the rest. More importantly, the spec doesn't surface whether any of the existing inter-step invariants (e.g., the "Clear must precede CleanStaleMarkers" reasoning in step 7→8) are affected by inserting the new sweep step earlier. They likely are not, but an implementer would have to verify rather than read it.

Also: Component F changes saver creation ordering inside step 4/5 (EnsureSaver) but the spec does not describe whether the saver-creation sub-steps are visible at the orchestrator level or fully encapsulated in `BootstrapPortalSaver`. No CLAUDE.md update is requested for F.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 9. Component F: no readiness barrier between respawn and subsequent bootstrap steps

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component F (Saver Creation Sets destroy-unattached=off BEFORE Daemon Starts)

**Details**:
After F's new ordering — create with placeholder → `SetSessionOption destroy-unattached=off` → `respawn-pane -k 'portal state daemon'` — `BootstrapPortalSaver` returns. The orchestrator proceeds to subsequent steps (Restore, EagerSignalHydrate, etc.) and eventually returns. Subsequent steps in the same `portal open` invocation, and downstream code that depends on the daemon being up (e.g., the daemon writing initial `daemon.pid`, daemon picking up the new tmux state), assume the daemon process exists and is ticking.

The spec does not say whether respawning the daemon creates a readiness barrier (e.g., wait for `daemon.pid` to exist, or wait for `pgrep -fx 'portal state daemon'` to return ≥1, or similar) before returning from `BootstrapPortalSaver`. The current code (pre-F) launches the daemon as the initial pane command, but there's no observable barrier in the spec text. Whether the existing `BootstrapPortalSaver` already has such a barrier is unclear from the spec.

Acceptance criterion 2 ("destroy-unattached=off is set before daemon process can exit … AND the pane process is `portal state daemon`") implicitly requires the daemon to be running by the time the test inspects it; the *means* of guaranteeing that is not specified.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 10. Component E logger constant `ComponentCapture` may not exist

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component E (`CaptureStructure` Per-Session Log-and-Continue), acceptance criteria

**Details**:
Component E's code snippet uses `logger.Warn(ComponentCapture, "show environment for session", …)` and the acceptance criterion says "Every per-session skip emits a WARN log entry under `ComponentCapture` (or equivalent existing component constant)". The "(or equivalent)" hedge implies the spec author isn't sure whether `ComponentCapture` exists. If it does not, the implementer must either add a new component constant (a planning decision not captured in the spec) or pick the closest existing constant (which could differ across implementers).

The structured logger lives in `internal/state/` (per CLAUDE.md). The spec should either confirm `ComponentCapture` already exists in `internal/state/`'s logger package, or specify that a new constant is to be added and where.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 11. Component D hysteresis empirical measurement has no recorded artefact

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component D (Daemon Self-Supervision), Risk Summary

**Details**:
The Risk Summary says: "Planning phase **MUST** empirically measure the legitimate `_portal-saver` create/recreate transient duration before locking N — this is a required mitigation, not optional." But the acceptance criteria for Component D do not include any "the measurement was performed and N was justified against measured values" criterion. There is no specified artefact (e.g., a measurement memo, a comment in the source pinning the N value to the measured worst-case + 2× safety factor, an integration test that fails if N falls below measured worst-case). Without an acceptance criterion, the "MUST" in the risk summary has no enforcement surface — an implementer could lock N=3 on inspection and ship.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 12. Component G mtime-snapshot backstop edge cases unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component G (Test Isolation Contract), `portaltest.NewIsolatedStateEnv`

**Details**:
The helper registers a `t.Cleanup` that "compare[s] a pre-test snapshot of file mtimes; fail if any state file was modified during the test." Several edge cases are unspecified:

1. **Developer's state dir doesn't exist yet.** A fresh-clone developer who has never run `portal` won't have `~/.config/portal/state/`. The pre-test snapshot is empty; what constitutes a "modification" if the directory comes into existence during the test? The strictest interpretation (any new file = violation) is correct but should be stated.
2. **Which stat field?** mtime, ctime, atime, or any-change-via-checksum? mtime is the obvious choice but atime updates on read on some filesystems; ctime updates on any inode change. The spec just says "mtimes" — fine, but explicit is better than implicit.
3. **What about file *contents* changing without mtime advancing?** Same-second writes on a filesystem with second-resolution mtime won't show a delta. Edge case but possible with atomic-write patterns.
4. **Symlinks/directories.** If a test creates a symlink in the dev's state dir, or removes a file, mtime comparison may or may not catch it depending on stat semantics.

For a backstop, "any change at all" is the right answer — but specifying it explicitly prevents implementer ambiguity.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 13. Component C "wrapped error treated as fatal misconfiguration" — exit semantics unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Component C, step 3 (post-flock inode cross-check) bound

**Details**:
"On persistent mismatch after the bound, return a wrapped error (treated as fatal misconfiguration — caller logs WARN and exits)." This is ambiguous on two axes:

1. **Exit code.** What exit code does the daemon use? `0` (consistent with lock-loser clean exit), or non-zero to signal misconfiguration? Bootstrap's saver-creation flow expects certain semantics from the daemon's exit; if non-zero exits trigger a restart loop via tmux, the inode-mismatch case becomes a busy-loop.
2. **Warning surface.** "Caller logs WARN" — under what `Component*` constant? Visible only in the daemon's own log, or also surfaced as a bootstrap warning to the user via the `warning` package?

The same ambiguity applies to Component C's pre-check returning `ErrDaemonLockHeld` — that path's exit code and surface are presumably already defined by the existing lock-loser flow, but the spec should at minimum say "same exit semantics as existing `ErrDaemonLockHeld` path" for the persistent-inode-mismatch path, OR specify a different code if the behaviour should diverge.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 14. Component F: placeholder process is not a `portal state daemon` — interaction with Component B sweep

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Composition between Components B and F

**Details**:
Component F's placeholder is `sh -c 'sleep infinity'` (or whatever portable equivalent is chosen). Component B's sweep enumerates `portal state daemon` processes and SIGKILLs those not matching the saver pane pid. The placeholder is NOT a `portal state daemon`, so B's sweep does not see it — correct.

But: if a *prior* bootstrap died between F's step 1 (create with placeholder) and F's step 3 (respawn), the saver session would persist (because step 2 set `destroy-unattached=off`) with the placeholder still running. The next bootstrap's `BootstrapPortalSaver` sees `sessionPresent=true` and probably runs `BootstrapAliveCheck` to determine whether the daemon is healthy. If the alive-check inspects pane args or daemon.pid, it'll find the placeholder, not a daemon → treat as unhealthy → kill-and-recreate. This path is plausibly already handled by existing code (the alive-check exists today), but the spec doesn't trace through it.

Net: the "saver exists with placeholder still in it from prior crashed bootstrap" state is a new state introduced by F. The spec should briefly confirm `BootstrapAliveCheck` (or whichever existing alive-check primitive is invoked) handles it via the existing unhealthy-saver path.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 15. Component A SIGKILL poll cadence unspecified

**Source**: Specification analysis
**Category**: Minor
**Affects**: Component A (Kill-Barrier Escalation), step 2.iii

**Details**:
"Poll `killBarrierIsAlive(priorPID)` for a bounded short window (1 s)." The existing poll cadence (50 ms per CLAUDE.md / current code) is plausibly inherited, but the spec doesn't state it. If the implementer halves it (25 ms) or stretches it (250 ms), behaviour drifts. Minor — the choice doesn't affect correctness — but a deliberate "same 50 ms cadence as the session-kill poll" line would remove the choice.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 16. Tick interval not specified in spec

**Source**: Specification analysis
**Category**: Minor
**Affects**: Component D, "Hysteresis N" rationale and acceptance criteria

**Details**:
Component D refers to "the daemon's current ~1 s tick interval" and acceptance criteria use "within (N + 1) tick intervals". The actual tick interval value is not specified in the spec — it lives in `cmd/state_daemon.go`. An implementer reading the spec in isolation would not know what "tick interval" maps to in real time, and the acceptance criteria are correspondingly fuzzy. A one-line reference ("`stateDaemonTickInterval`, currently 1 s in `cmd/state_daemon.go`") would suffice.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---

### 17. End-State Verification ties `daemon.version` correctness to no specific component

**Source**: Specification analysis
**Category**: Minor
**Affects**: End-State Verification section

**Details**:
"`daemon.version` file content matches the running binary's version. On the reporter's install, `daemon.version` was `0.5.5` after a 0.5.6 upgrade — direct evidence that `EnsurePortalSaverVersion` was not running cleanly because the kill-barrier was timing out. Post-fix, `daemon.version` should track the running binary on every bootstrap."

This is implied to be a downstream consequence of A/B/F fixing the kill-barrier/recreate path, but no component explicitly takes ownership of "`daemon.version` correctness" as an acceptance criterion. If an implementer reads the seven components and ships, then runs end-state verification and finds `daemon.version` still stale, which component fails? The traceability is left implicit. Either: (a) attach this as an acceptance criterion to F or A explicitly, (b) explain in End-State Verification that it is observation-only / not directly tested.

**Proposed Addition**:
{To be discussed.}

**Resolution**: Pending
**Notes**:

---
