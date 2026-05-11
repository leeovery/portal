---
status: complete
created: 2026-05-11
cycle: 1
phase: Input Review
topic: multiple-state-daemons-running-concurrently
---

# Review Tracking: multiple-state-daemons-running-concurrently - Input Review

## Findings

### 1. Topology-churn during recycle keeps surviving daemons in back-to-back-sweep regime

**Source**: investigation.md "Contributing Factors", final bullet
**Category**: Enhancement to existing topic
**Affects**: Root Cause → "Why the old daemon survives the kill signal"

**Details**:
The investigation calls out a positive-feedback element of the bug: the recycle event itself generates topology-change tmux hooks that write `save.requested`, which keeps the dirty flag set, which keeps surviving daemons sweeping back-to-back, which is precisely the regime that maximises the cancel-to-exit window after a kill.

**Proposed Addition**:
> **Recycle-induced sweep pressure.** The kill-respawn event itself generates `session-closed` and `session-created` tmux hooks. Both fire `save.requested`, which keeps the daemon's dirty flag set and forces the surviving daemon's sweep into the back-to-back regime described above. The cancel-to-exit window is therefore widest precisely on the recycle path that the fix must defend — the worst-case kill-barrier latency is structural to the recycle event, not a tail-case observation.

**Resolution**: Approved
**Notes**: Added as standalone paragraph in Root Cause section.

---

### 2. `save.requested` race between concurrent daemons is benign

**Source**: investigation.md "Blast Radius → Directly affected", final bullet
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → "Impact / Shared-state corruption"

**Details**:
The investigation explicitly notes `save.requested` is one shared file where the race is benign (loser's `remove` is a no-op via `errors.Is(err, fs.ErrNotExist)`). Recording bounds the corruption claim and pre-empts unnecessary hardening at planning time.

**Proposed Addition**:
Append to the Shared-state corruption bullet: " The `save.requested` flag is a notable exception — both daemons race to remove it on successful sweep, but the loser's remove is a benign no-op via `errors.Is(err, fs.ErrNotExist)`."

**Resolution**: Approved
**Notes**: Appended in-place to the Shared-state corruption bullet.

---

### 3. FIFO sweep paths as a potentially-affected surface

**Source**: investigation.md "Blast Radius → Potentially affected"
**Category**: New topic
**Affects**: Problem Statement → Impact, new "Potentially affected" sub-section

**Details**:
Investigation flags FIFO cleanup as "likely safe but worth confirming during fix design." Currently absent from spec.

**Proposed Addition**:
New sub-section under Problem Statement → Impact:
> ### Potentially affected (to confirm during planning)
>
> - **FIFO sweep paths.** Two daemons could in principle both call into `state` cleanup helpers concurrently. `FIFOSweeper` itself runs only in bootstrap (single-shot per process), so daemon-side FIFO interaction is read-only — investigation assessed this as "likely safe" but flagged that the fix design should explicitly confirm there is no daemon-side write path that two concurrent daemons could race on.

**Resolution**: Approved
**Notes**: Added as new sub-section in Impact.

---

### 4. Singleton invariant matters for future seams

**Source**: investigation.md "Blast Radius → Potentially affected"
**Category**: Enhancement to existing topic
**Affects**: Acceptance Criteria → Singleton invariant

**Details**:
The investigation flags forward-compatibility as a reason the singleton invariant matters beyond immediate CPU/corruption symptoms. Future seams expecting daemon-singleton semantics (e.g. a centralised hook queue) would silently break under N>1.

**Proposed Addition**:
Append a bullet to the Singleton invariant acceptance criterion:
> - **Forward-compatibility rationale.** Singleton-ness is a structural property that future seams may depend on (e.g. a centralised hook queue, a single-writer log channel). Once 2+ daemons are possible, any such seam silently breaks. The lock (Part 1) is the floor that holds this property even under code paths not yet written.

**Resolution**: Approved
**Notes**: Appended as final bullet in Singleton invariant section.

---

### 5. Saver-recycle observability gap

**Source**: investigation.md "Why It Wasn't Caught"
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → Observability

**Details**:
Investigation cites silence of the kill-respawn path as a "why it wasn't caught" factor. Spec's Observability AC says "silent success is expected" — tension between the two needs explicit resolution.

**Proposed Addition**:
Append a bullet to the Observability acceptance criterion:
> - **Recycle-path silence is preserved (and acknowledged as a trade-off).** The investigation's "Why It Wasn't Caught" called out the existing kill-respawn path's total silence — operators have no signal that a recycle happened or how often. The spec does **not** add a diagnostic/info-level log at the recycle decision point. The two WARN paths above are sufficient to surface the abnormal cases (loser contention, barrier timeout). If recurrence diagnostics become a need, that is a follow-up observability work unit, not part of this fix.

**Resolution**: Approved
**Notes**: Trade-off explicitly acknowledged; recycle-path remains silent.

---

### 6. Existing `BootstrapAliveCheck` unit tests cannot model "pidfile overwritten while prior daemon still runs"

**Source**: investigation.md "Why It Wasn't Caught"
**Category**: Enhancement to existing topic
**Affects**: Test Strategy → Integration test rationale

**Details**:
Investigation explains why seam-level tests miss this and the integration test is load-bearing. Useful planning rationale, currently implicit in the spec.

**Proposed Addition**:
Appended to the integration-test description (after the "load-bearing test" sentence):
> Crucially, the existing `BootstrapAliveCheck` seam-level unit tests cannot model the failure mode here: they fix a pidfile and probe it, but cannot model "what happens when the pidfile is overwritten while the prior daemon still runs." That asymmetry is the reason the integration test is required and is not redundant with seam-level coverage.

**Resolution**: Approved
**Notes**: Rationale added inline in Integration test section.

---

### 7. Direct observational data — `ps`/CPU correlation

**Source**: investigation.md "Manifestation"
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Impact, Severity bullet

**Details**:
Spec captures the CPU pegging but not the direct one-to-one observational correlation between daemon count and concurrent `capture-pane` children, nor the "zero daemons → 0–22% CPU" control observation that confirmed causality during investigation.

**Proposed Addition**:
Append to the Severity bullet:
> Observational confirmation: 1 s-resolution `ps` snapshots showed 4–7 concurrent `tmux capture-pane -e -p -S -` child processes (one per running daemon mid-sweep) and a one-to-one correlation between daemon count and the CPU peg. With zero daemons running, server CPU dropped to 0–22% and capture-pane processes dropped to zero — confirming daemons are the workload, not some other process.

**Resolution**: Approved
**Notes**: Added inline in Severity bullet.

---

### 8. Rejected option: "Treat `_portal-saver` session presence as the singleton signal"

**Source**: investigation.md "Fix Direction → Options Explored"
**Category**: Enhancement to existing topic
**Affects**: Fix Part 2 → rejected alternatives area

**Details**:
Spec captures 4 of 5 rejected options from the investigation; this one is missing. Preserves the pre-emptive rejection so planners don't redesign the kill-respawn protocol.

**Proposed Addition**:
New sub-section under Fix Part 2, after "Why not spin-wait inside `BootstrapPortalSaver`":
> ### Why not treat `_portal-saver` session presence as the singleton signal
>
> Considered and rejected during investigation. The idea: skip the kill+respawn entirely on dev-build mismatch and let the daemon detect version-mismatch internally and self-exit. Rejected because it would require teaching the daemon to introspect and manage its own version lifecycle — significantly larger surgery than the kill barrier + lock. The kill-respawn protocol is otherwise sound; it needs a synchronisation barrier, not a redesign.

**Resolution**: Approved
**Notes**: Added as new sub-section in Fix Part 2.

---

### 9. `history_bytes` 82 MB top-pane data point

**Source**: investigation.md "Environment / User conditions" and "Contributing Factors"
**Category**: Enhancement to existing topic
**Affects**: Risk and Rollout → "Risk surface: timeout too short for heavier scrollback"

**Details**:
Spec records 28 MB aggregate rendered text but not the 82 MB top-pane `history_bytes` figure — the upper end of the user's distribution. Relevant for "is 5 s sized for my profile?" sanity-check.

**Proposed Addition**:
Replace the risk-surface bullet wording to add the 82 MB figure inline:
> **Risk surface: timeout too short for heavier scrollback than observed.** Users with even larger scrollback than the affected user's 24-pane / ~28 MB rendered-text aggregate (top-pane `history_bytes` of 82 MB) could see the 5 s timeout fire on legitimate cold sweeps. The lock catches this: timeout firing produces a WARN line, and if the prior daemon does still hold the lock when the new one starts, the new one fails-fast and the next `portal` command recovers via tolerant-kill-and-recreate. No corruption; no user intervention required.

**Resolution**: Approved
**Notes**: 82 MB figure added inline in risk-surface bullet.

---

### 10. Companion observation: stale `; exec $SHELL` wrappers cross-referenced

**Source**: investigation.md "Notes" and Symptoms "References"
**Category**: Enhancement to existing topic
**Affects**: Out of Scope → Stale `; exec $SHELL` wrappers from hydrate helper

**Details**:
Existing Out-of-Scope item is missing (a) "~20-hour-old bootstrap" timescale and (b) explicit cross-reference to the `.workflows/killed-sessions-resurrect-on-restart/` work unit where PaneKeys recur.

**Proposed Addition**:
Replace the first sentence of the Out-of-Scope item:
> Companion observation in the investigation: 3 stale `sh -c 'portal state hydrate …; exec $SHELL'` processes were observed, traced to a ~20-hour-old bootstrap (i.e. long-stale, not recent). The trailing `; exec $SHELL` is unreachable because the wrapper is parked on the child shell after hydrate exits. PaneKeys overlap with `.workflows/killed-sessions-resurrect-on-restart/` (active, in implementation).

**Resolution**: Approved
**Notes**: Timescale and cross-reference added.

---

### 11. Single-observation reproducibility note

**Source**: investigation.md "Reproduction Steps"
**Category**: Gap/Ambiguity
**Affects**: Problem Statement → Trigger frequency

**Details**:
The spec records the accumulation rate and 10-day uptime but does not explicitly state that the entire observation is N=1 (single user / single snapshot on 2026-05-09). Important for rollout posture and for framing the fix as structurally-justified rather than empirically-justified.

**Proposed Addition**:
Append a paragraph to Problem Statement → Trigger frequency:
> **Reproducibility caveat.** The whole observation is a **single snapshot** from one user (2026-05-09). The fix is justified by structural inevitability (the bootstrap race + missing singleton lock), not by repeated observation. Any future occurrence of any kill-respawn trigger reproduces the accumulation mechanism; the spec's confidence in the fix shape derives from the code trace, not from N>1 field reports.

**Resolution**: Approved
**Notes**: Added as trailing paragraph to Trigger frequency section.

---
