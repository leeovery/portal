---
status: in-progress
created: 2026-05-19
cycle: 1
phase: Input Review
topic: saver-kill-respawn-loop-leaks-daemons
---

# Review Tracking: saver-kill-respawn-loop-leaks-daemons - Input Review

## Findings

### 1. `destroy-unattached=off` lifecycle clarification missing from cascade explanation

**Source**: investigation.md "Code Trace" — Lock-contention cascade section, parenthetical: *"tmux's `_portal-saver` session is destroyed because its initial pane process has exited (`destroy-unattached=off` doesn't save a session whose only pane has exited normally — that's a different lifecycle axis)."*
**Category**: Enhancement to existing topic
**Affects**: Root Cause → Defect 2 (lock-loser cascade paragraph)

**Details**:
The spec asserts that the lock-loser daemon's clean exit "destroys the just-created `_portal-saver` pane process" and triggers the `SetSessionOption` "no such session" cascade, but doesn't explain *why* the session dies given that `destroy-unattached=off` was just being set. The investigation calls out the non-obvious tmux semantic: `destroy-unattached=off` governs the detach/no-clients axis, not the "initial pane process exited normally" axis. An implementer who reads only the spec might assume the session should have survived because `destroy-unattached=off` was the target of the failing call. This matters for review confidence and for the integration test in Testing Requirements #3 ("Lock-loser daemon's pane exit destroys `_portal-saver` session") — the test author needs to understand which tmux lifecycle axis they're exercising.

**Current**:
> When the barrier gives up early, the new daemon spawns, immediately collides with the still-held `daemon.lock`, exits cleanly **without writing `daemon.pid` or `daemon.version`**, destroys the just-created `_portal-saver` pane process, and triggers the `SetSessionOption(_portal-saver, destroy-unattached, off)` "no such session" cascade.

**Proposed Addition**:
Add a clarifying sentence: the session dies because tmux destroys a session whose only pane's initial process has exited normally — this is a distinct lifecycle axis from `destroy-unattached`, which governs the detach/no-clients case. The cascade is therefore unaffected by the `destroy-unattached=off` setting that the failing `SetSessionOption` call was trying to apply.

**Resolution**: Pending
**Notes**:

---

### 2. Recycle-induced sweep pressure not carried forward

**Source**: investigation.md "Prior Work — Cross-Reference", final bullet: *"Recycle-induced sweep pressure. Kill-respawn itself emits `session-closed` and `session-created` hooks, both of which fire `save.requested` and force the surviving daemon's sweep into a back-to-back regime — widening the cancel-to-exit window precisely on the recycle path the barrier was meant to defend."*
**Category**: Enhancement to existing topic
**Affects**: Root Cause → Defect 2 (Why It Wasn't Caught) or Contributing factors

**Details**:
The investigation explicitly notes that the kill-respawn path *itself* generates extra sweep pressure on the surviving daemon (session-closed + session-created hooks both fire `save.requested`, pushing the daemon into back-to-back sweeps). This is a self-amplifying property of the bug — the recycle path widens its own cancel-to-exit window. It's relevant to Change 2's correctness story (the ctx-aware loop must remain interruptible even under back-to-back sweep pressure) and to the integration test sizing for SIGHUP responsiveness. The spec omits this dynamic entirely.

**Proposed Addition**:
Add a note (under Defect 2 root cause or Why It Wasn't Caught) that the recycle path generates additional sweep pressure on the surviving daemon via `save.requested` events fired by `session-closed` and `session-created` hooks, producing a back-to-back sweep regime that widens the cancel-to-exit window precisely on the path the barrier defends. The ctx-aware loop in Change 2 must remain interruptible under this pressure.

**Resolution**: Pending
**Notes**:

---

### 3. Comment-as-contract at `portal_saver.go` lines 232-241 not flagged for update

**Source**: investigation.md "Contributing Factors": *"The version-mismatch comment encodes the wrong invariant as intentional. Line 236-237 of portal_saver.go explicitly says ErrVersionFileAbsent counts as mismatch, 'for first-ever bootstrap or user-initiated state-dir cleanup.' The comment is a design choice that ages badly once the file proves not to be reliably present."*
**Category**: Enhancement to existing topic
**Affects**: Change 1 — Alive-check first in `EnsurePortalSaverVersion`

**Details**:
The spec's Change 1 captures the test surface that pins the bug as contract ("must be replaced") but does not flag the analogous source comment at `portal_saver.go:232-241` that also encodes the false-positive as intentional design. An implementer working from the spec alone might update logic and tests but leave the comment stating the opposite invariant, creating a future trap.

**Current**:
> `portalSaverVersionMismatch` keeps its current external shape but is **no longer the lone gate**. The alive-check classifies the situation first; the mismatch predicate is consulted only on the alive-with-readable-version branch.

**Proposed Addition**:
Note under Change 1 (or in implementation guidance) that the existing comment on `portalSaverVersionMismatch` (currently `portal_saver.go:232-241`) explicitly encodes "ErrVersionFileAbsent counts as mismatch" as intentional design and must be updated to reflect the new contract — the alive-check ordering is what captures the broader invariant, and the predicate no longer treats absence as mismatch on its own.

**Resolution**: Pending
**Notes**:

---

### 4. Defect 3 candidate-deleter table dropped from carryover

**Source**: investigation.md "Root Cause" → Defect 3 table enumerating every checked file-removal path (state_cleanup --purge, daemon save.requested removal, hydrate FIFO removals, log rotation, scrollback dedup, fifo_sweep), plus the "open sub-question" candidates list (atomic-write race in `state.WriteVersionFile`, over-eager cleanup pass in daemon tick loop, bootstrap CleanStale step, shutdown-flush behaviour in `defaultShutdownFlush`).
**Category**: Enhancement to existing topic
**Affects**: Change 3 — Debug breadcrumb on `daemon.version` writes / Out of Scope #4

**Details**:
The spec compresses this to "no production code path removes `daemon.version` individually" without preserving the enumeration. Two consequences:

1. The breadcrumb's diagnostic value depends on knowing which paths were already ruled out — when the next disappearance is investigated, having the prior negative findings inline (or referenced) saves re-tracing.
2. The "candidates to investigate" list (atomic-write race, over-eager tick-loop cleanup, CleanStale, shutdown-flush) is the natural follow-up investigation surface if Change 3's breadcrumb captures a recurrence. Losing it forces a future investigator to re-derive it.

This is borderline — the spec correctly scopes Defect 3 as instrumentation-only. But the breadcrumb's reason-to-exist is "preserve evidence for a future investigation," and the future investigation's starting set was already established. Worth carrying forward as a pointer.

**Proposed Addition**:
Under Change 3 (or in an explanatory note), preserve a pointer that the investigation enumerated production removal paths and ruled them out; if the breadcrumb captures a recurrence, the follow-up investigation should start from the candidate list (atomic-write race in `WriteVersionFile`, over-eager cleanup in the daemon tick loop, `CleanStale`, shutdown-flush behaviour) rather than re-tracing. Cite the investigation as the source.

**Resolution**: Pending
**Notes**:

---

### 5. Synthesis gap "no fresh wall-time measurement of current cold sweep" not addressed

**Source**: investigation.md "Discussion" → Synthesis agent gap #3: *"No fresh wall-time measurement of the current cold sweep — informational, not blocking. The spec may want a measurement to size any test timeouts."*
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements → Integration test #2 ("Daemon mid-tick, SIGHUP arrives")

**Details**:
The spec's integration test #2 targets "under 2s on the test fixture" for SIGHUP-to-exit latency, but doesn't source where the 2s number comes from. The investigation explicitly flags that no fresh wall-time measurement was taken and that the spec might want one to size test timeouts. As written, the 2s threshold is unanchored. Planning/implementation will either need to take a measurement first or pick a number defensively. Worth either committing to "take a measurement during implementation and size the test threshold from it" or accepting "2s is the heuristic threshold; revise if the fixture proves it too tight/loose."

**Proposed Addition**:
Add a note under Integration test #2 either committing to taking a fresh wall-time measurement of one pane's `capture-pane` invocation against a representative scrollback fixture and sizing the test threshold from that, or explicitly designating the 2s threshold as a heuristic with permission to adjust during implementation.

**Resolution**: Pending
**Notes**:

---

### 6. Related closed bugfixes mentioned but only one cross-referenced

**Source**: investigation.md "Notes" section: *"Pre-existing related closed bugfixes worth cross-referencing: `multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`. The new orphan-leak symptom may overlap with the first of these — verify whether it's a regression or a distinct root cause."*
**Category**: Enhancement to existing topic
**Affects**: Risk & Rollout → Coordination with prior bugfix

**Details**:
The spec cross-references `multiple-state-daemons-running-concurrently` thoroughly (correctly — it's the direct predecessor). It omits the other two related closed bugfixes the investigation flagged. `daemon-merge-reintroduces-dead-sessions` and `killed-sessions-resurrect-on-restart` touch adjacent daemon/restore behaviour; a planning agent should know they exist as regression-watch points (the spec already lists "structural-index merge logic" under "Not affected" in the investigation's Blast Radius — that's effectively the daemon-merge bugfix, but the link is implicit).

**Proposed Addition**:
Add `daemon-merge-reintroduces-dead-sessions` and `killed-sessions-resurrect-on-restart` to the Risk & Rollout coordination/regression-watch list, noting that they exercise adjacent daemon/restore surfaces and their tests should remain green (without claiming any of their logic is being touched).

**Resolution**: Pending
**Notes**:

---
