---
status: in-progress
created: 2026-05-11
cycle: 1
phase: Input Review
topic: multiple-state-daemons-running-concurrently
---

# Review Tracking: multiple-state-daemons-running-concurrently - Input Review

## Findings

### 1. Topology-churn during recycle keeps surviving daemons in back-to-back-sweep regime

**Source**: investigation.md "Contributing Factors", final bullet:
> "Topology-churn from any saver recycle. When the kill-respawn path does fire, the resulting `session-closed`/`session-created` hooks write `save.requested`, keeping the daemon's dirty flag set and pushing surviving daemons into the back-to-back-sweep regime exactly when the kill race is most live."

**Category**: Enhancement to existing topic
**Affects**: Root Cause → "Why the old daemon survives the kill signal" (and possibly the Acceptance Criteria / Risk sections)

**Details**:
The investigation calls out a positive-feedback element of the bug: the recycle event itself generates topology-change tmux hooks that write `save.requested`, which keeps the dirty flag set, which keeps surviving daemons sweeping back-to-back, which is precisely the regime that maximises the cancel-to-exit window after a kill. The spec captures the back-to-back-sweep mechanism ("The Go ticker drops missed fires...") but does not connect it to recycle-induced topology churn. This is potentially load-bearing because (a) it justifies why kills land specifically when the daemon is least able to observe them, and (b) it suggests the kill barrier's 5 s timeout sees its worst case precisely on the recycle path it covers — not coincidentally.

**Current**:
> Sweep cost is the cost driver. `internal/tmux/tmux.go:625` uses `capture-pane -e -p -S -` (unbounded scrollback). Measured 24-pane sweep: **3.9 s cold / 1.5 s warm** at the observed scrollback profile (~28 MB rendered text). The Go ticker drops missed fires, so when a sweep overruns the 1 s tick interval the next tick fires immediately on completion — daemons in this regime **never reach `ctx.Done()` between sweeps**, extending the orphan-eligibility window indefinitely after a kill.

**Proposed Addition**:
{leave blank — discuss whether this belongs in Root Cause as an additional paragraph or in Risk and Rollout as part of the worst-case latency analysis}

**Resolution**: Pending
**Notes**:

---

### 2. `save.requested` race between concurrent daemons is benign (but worth recording)

**Source**: investigation.md "Blast Radius → Directly affected", final bullet:
> "`save.requested` flag — both daemons race to remove it on successful sweep; remove on the loser's side is a benign no-op via `errors.Is(err, fs.ErrNotExist)`."

**Category**: Enhancement to existing topic
**Affects**: Problem Statement → "Impact / Shared-state corruption"

**Details**:
The spec's Impact section enumerates shared-state corruption surfaces (`sessions.json`, scrollback `.bin`, `daemon.pid`/`daemon.version`) but omits `save.requested`. The investigation explicitly notes this is one place where the race is **benign** — the loser's `remove` is a no-op via `errors.Is(err, fs.ErrNotExist)`. Recording this matters because (a) it bounds the corruption claim (not every shared file is corrupted) and (b) a planner could otherwise be tempted to harden `save.requested` removal, which the investigation has already determined is unnecessary.

**Current**:
> - **Shared-state corruption.** Multiple daemons writing the same state directory: `sessions.json` (atomic per-commit but two daemons race the read-then-commit window, producing flip-flop content across consecutive ticks), per-pane scrollback `.bin` files (`AtomicWrite` is per-call atomic but the two writers can interleave content versions), and `daemon.pid`/`daemon.version` markers become incoherent — `BootstrapAliveCheck` becomes meaningless once N > 1.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 3. FIFO sweep paths as a potentially-affected surface

**Source**: investigation.md "Blast Radius → Potentially affected":
> "FIFO sweep paths: two daemons could both call into `state` cleanup helpers concurrently. The `FIFOSweeper` runs only in bootstrap (single-shot per process), so daemon-side FIFO interaction is read-only — likely safe but worth confirming during fix design."

**Category**: New topic (or Enhancement to Impact / Out of Scope)
**Affects**: Problem Statement → Impact, or a new "Potentially affected surfaces" sub-section

**Details**:
The investigation flags this as a surface the planner should explicitly confirm during fix design. It is not in the spec at all. The investigation's own framing ("likely safe but worth confirming") makes this a planning-readiness item: either the spec records that the planner needs to confirm it, or it acknowledges the assessment and moves on. Dropping it silently leaves the planner without a pointer to a surface the investigation already triaged.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 4. Singleton invariant matters for future seams (centralised hook queue example)

**Source**: investigation.md "Blast Radius → Potentially affected":
> "Any future seam expecting daemon-singleton semantics (e.g. a centralised hook queue) would silently break."

**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Impact, or Acceptance Criteria → Singleton invariant rationale

**Details**:
The investigation flags forward-compatibility as a reason the singleton invariant matters beyond the immediate CPU/corruption symptoms. The spec's singleton-invariant acceptance criterion is purely about live process count; the investigation's framing adds a "this is a structural property other code may depend on" rationale that justifies why the lock (Part 1) is the floor rather than a redundant guard once the barrier (Part 2) is in place. This reinforces the existing "Why a lock and not `O_EXCL` pidfile create" / "How the defects compose" reasoning but is not currently said.

**Current**:
> ### Singleton invariant
>
> - At most one `portal state daemon` process exists per state directory at any time, regardless of how many bootstrap invocations have run during the tmux server lifetime.
> - Verified by: an integration test (real-tmux fixture) that runs two back-to-back `EnsurePortalSaverVersion` calls — one must trigger a recycle via version mismatch — and asserts exactly one live `portal state daemon` process when both calls complete.
> - Verified manually by: running `pgrep -P <tmux-server-pid> -f 'portal state daemon'` after repeated bootstrap invocations and observing a count of exactly 1.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 5. Saver-recycle observability gap ("kill-respawn path is silent")

**Source**: investigation.md "Why It Wasn't Caught":
> "Saver recycle is silent. The kill-respawn path in `EnsurePortalSaverVersion` and `BootstrapPortalSaver` logs nothing at the moment it decides to recycle, so an operator has no signal that a kill happened or how often. Even if multiplication were happening continuously, the visibility surface for 'daemon was killed; replacement is starting' is zero."

**Category**: Gap/Ambiguity (potentially New topic)
**Affects**: Acceptance Criteria → Observability

**Details**:
The spec's Observability acceptance criterion explicitly says "No new logs are emitted on the common-case (clean handover) path. Silent success is the expected behaviour." The investigation, in contrast, identified the **existing silence of the recycle path** as one of the reasons the bug went unnoticed. There is a tension here that the spec does not resolve: should the recycle decision itself emit a debug/info-level signal (separate from the WARN paths), so future occurrences are at least diagnosable? The investigation does not propose adding logging here, but it does name the silence as a contributing failure-to-detect. The spec should either (a) explicitly acknowledge the trade-off and decide to leave it silent, or (b) propose a low-level diagnostic log.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 6. Existing `BootstrapAliveCheck` unit-tests cannot model "pidfile overwritten while prior daemon still runs"

**Source**: investigation.md "Why It Wasn't Caught":
> "Unit-level seam tests (`BootstrapAliveCheck` is a `var` for test override) verify the alive-check **for a given pidfile** but cannot model 'what happens when the pidfile is overwritten while the prior daemon still runs.'"

**Category**: Enhancement to existing topic
**Affects**: Test Strategy → "What is NOT tested here", or the singleton-lock unit test rationale

**Details**:
The spec's test strategy adds new unit tests and an integration test, but does not record **why** the existing `BootstrapAliveCheck` unit tests would not have caught this even with their current seam-based design. The investigation captures this explicitly — the seam tests fix the pidfile and probe it; they cannot model the "overwritten while prior is alive" condition. This is useful planning context because it explains why the **integration** test (real tmux, two back-to-back calls) is load-bearing and not redundant with seam-level tests.

**Current**:
> ### What is NOT tested here
>
> - **Sweep duration at realistic scrollback scale.** The bug is latent at N=1 with sub-second sweeps and only manifests when sweep overrun combines with recycle frequency. Reproducing this in CI would require fabricating ~28 MB of scrollback per pane across 24 panes. The barrier's 5 s timeout is sized for this profile based on the investigation's field measurements; this is captured in the spec, not in a CI test.
> - **Long-uptime accumulation.** The 7-orphan snapshot accumulated over 10 days; no CI test can reproduce that timescale. The singleton invariant test plus the kill-barrier unit tests collectively cover the structural mechanism that would otherwise drive accumulation.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 7. Direct observational data — `ps`/CPU correlation

**Source**: investigation.md "Manifestation":
> "`ps` snapshots once per second showed 4–7 concurrent `tmux capture-pane -e -p -S -` child processes (one per running daemon mid-sweep)."
> "With zero daemons running, tmux server CPU dropped to 0–22% and capture-pane processes dropped to zero."

**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Impact

**Details**:
The spec's Impact section captures the CPU pegging (75–98%) and that `cmd_capture_pane_exec` dominates the sample, but does not record the direct one-to-one observational correlation between daemon count and concurrent `capture-pane` children, nor the "zero daemons → 0–22% CPU, zero capture-pane procs" control observation that confirmed causality during investigation. This is the evidence that the symptom is daemons (not some other workload), and a reader of the spec without the investigation would not see it. Worth a one-line addition.

**Current**:
> - **Severity: High.** When 2+ daemons run concurrently the tmux server pegs at 75–98% CPU. The cost is in `capture-pane -e -p -S -` (unbounded scrollback) called by each daemon's per-pane sweep — `sample` shows all time in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 8. Rejected option not captured: "Treat `_portal-saver` session presence as the singleton signal"

**Source**: investigation.md "Fix Direction → Options Explored":
> "Treat `_portal-saver` session presence as the singleton signal (don't kill at all on dev-build mismatch). Rejected — would require teaching the daemon to handle version-mismatch internally and self-exit. Larger surgery; the kill-respawn protocol is otherwise sound, just needs a synchronisation barrier."

**Category**: Enhancement to existing topic
**Affects**: Fix Part 2 → "Why not spin-wait inside `BootstrapPortalSaver`..." (rejected-alternatives area), or a new "Rejected alternatives" sub-section

**Details**:
The spec captures three of the five rejected options from the investigation (flock-alone-noisy, sync-kill-alone, spin-wait-in-bootstrap, `O_EXCL`-pidfile). It does **not** capture the "treat session presence as singleton signal / move version-mismatch handling into the daemon" rejection. This may matter to a planner who, encountering the kill-respawn protocol for the first time, is tempted to redesign it. The investigation's pre-emptive rejection saves that round trip. Per the hard rule "never re-litigate decisions," the rejected status stays — but the rejection itself was a recorded decision and warrants being preserved.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 9. Long tmux uptime + heavy scrollback as named precondition for symptom manifestation

**Source**: investigation.md "Environment / User conditions" and "Contributing Factors":
> "Long-running tmux server with high scrollback (24 panes, ~28 MB rendered text, top `history_bytes` 82 MB) — the conditions under which sweep overrun becomes the dominant regime."

**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Trigger frequency, or Risk and Rollout → "Risk surface: timeout too short for heavier scrollback than observed"

**Details**:
The spec mentions the 28 MB rendered-text profile in two places (root cause measurements, risk-surface caveat) but does not record the `history_bytes 82 MB` top-pane data point — which is the upper end of the user's distribution and is relevant for the "Risk surface: timeout too short for heavier scrollback than observed" claim in Risk and Rollout. A reader trying to validate "is 5 s sized for my profile" would want both the 28 MB aggregate and the 82 MB per-pane top.

**Current**:
> - **Risk surface: timeout too short for heavier scrollback than observed.** Users with even larger scrollback than the affected user's 24-pane / 28 MB profile could see the 5 s timeout fire on legitimate cold sweeps. The lock catches this: timeout firing produces a WARN line, and if the prior daemon does still hold the lock when the new one starts, the new one fails-fast and the next `portal` command recovers via tolerant-kill-and-recreate. No corruption; no user intervention required.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 10. Companion observation: stale `; exec $SHELL` wrappers cross-referenced to `killed-sessions-resurrect-on-restart`

**Source**: investigation.md "Notes" and Symptoms "References":
> "Companion side-observation: 3 stale `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from a ~20-hour-old bootstrap. PaneKeys recur with `killed-sessions-resurrect-on-restart`. Trailing `; exec $SHELL` is unreachable because hydrate has exited and the wrapper is parked on the child shell. Not load-causing — log as separate observation, not part of this fix."

**Category**: Enhancement to existing topic
**Affects**: Out of Scope → "Stale `; exec $SHELL` wrappers from hydrate helper"

**Details**:
The spec already has an Out-of-Scope item for this — good. However, the spec's version omits two specific details from the investigation: (a) the wrappers were from a "~20-hour-old bootstrap" (timescale relevant for understanding they're long-stale, not recent), and (b) the explicit suggestion that this "may surface in the `killed-sessions-resurrect-on-restart` work unit since PaneKeys recur there" is in the spec but the cross-reference to the related `.workflows/killed-sessions-resurrect-on-restart/` work unit (mentioned in Symptoms References) is not. Minor; worth noting for completeness.

**Current**:
> ### Out: Stale `; exec $SHELL` wrappers from hydrate helper
>
> Companion observation in the investigation: 3 stale `sh -c 'portal state hydrate …; exec $SHELL'` processes were observed from an older bootstrap. The trailing `; exec $SHELL` is unreachable because the wrapper is parked on the child shell after hydrate exits. PaneKeys overlap with `killed-sessions-resurrect-on-restart`.
>
> Why deferred: not load-causing for this bug. The wrappers are idle and do not contribute to the CPU pegging. Logged as a separate observation in the investigation Notes section.
>
> Disposition: track as a standalone observation. May surface in the `killed-sessions-resurrect-on-restart` work unit since PaneKeys recur there.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 11. Single-observation reproducibility note (date and N=1 sample)

**Source**: investigation.md "Reproduction Steps":
> "Reproducibility: Single observation so far (2026-05-09). Conditions for accumulation are structural (the bootstrap race), so any repeat of the trigger reproduces."

**Category**: Gap/Ambiguity
**Affects**: Problem Statement → Trigger frequency, or Risk and Rollout → Rollout

**Details**:
The spec records the accumulation rate ("one orphan per 34 hours of tmux uptime") and the 10-day uptime, but does not explicitly note that the **whole observation is from a single user / single snapshot on a specific date**. This is potentially load-bearing for rollout decisions: "we have one data point" is a different rollout posture than "we have a reproducible class of bug." The investigation explicitly states reproducibility is N=1 but argues from structural inevitability. The spec's risk and rollout posture implicitly assumes this but does not name it.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---
