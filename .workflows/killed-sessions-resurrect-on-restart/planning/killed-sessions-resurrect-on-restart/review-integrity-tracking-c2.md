---
status: in-progress
created: 2026-05-10
cycle: 2
phase: Plan Integrity Review
topic: Killed Sessions Resurrect on Restart
---

# Review Tracking: Killed Sessions Resurrect on Restart - Integrity

## Findings

### 1. Planning.md task-table Edge Cases for tasks 2-2 and 2-5 are stale relative to phase detail

**Severity**: Important
**Plan Reference**: `planning.md` Phase 2 task table — rows for `killed-sessions-resurrect-on-restart-2-2` (line 83) and `killed-sessions-resurrect-on-restart-2-5` (line 86)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:

Cycle 1's "sleep ownership consolidation" fix moved the 100 ms settle-sleep insertion into task 2-1 (which now inserts `time.Sleep(hydrateSettleSleep)` in `runHydrate`'s timeout branch and renames `TestHydrate_TimeoutDoesNotSleep100ms` → `TestHydrate_Timeout_PreservesSettleSleepBeforeExec`). Phase-2 task detail files were updated, but the planning.md task table rows for 2-2 and 2-5 still carry the pre-fix Edge Cases text and (for 2-5) the pre-fix task name. Concrete drift:

- Task 2-2 row says edge case: "100 ms settle-sleep absence still documented as deliberate". This contradicts phase-2-tasks.md task 2-2 Edge Cases line 80 which now reads: "The 100 ms settle-sleep is preserved on the exec fall-through ... `runHydrate`'s shared post-handler block owns the sleep call." The sleep is no longer absent — task 2-1 inserts it — and task 2-2's comment-only edit documents the unified recovery contract, not an "absence".
- Task 2-5 row name says: "Unit test: handleHydrateTimeout preserves the 100 ms settle-sleep absence and FIFO-unlink ordering". The actual phase-2-tasks.md task 2-5 heading (line 179) is now: "Unit test: runHydrate timeout fall-through preserves the 100 ms settle-sleep, marker-unset ordering, and FIFO-unlink tolerance". The phrasing inversion ("absence" → "preserves the sleep") is exactly the cycle-1 fix; it was applied to task detail but not the task-table summary row.

This is an Important-severity finding rather than Critical because phase detail (which the implementer reads via `tick show`) is correct; but the planning.md task-table row is the implementer's first-glance index into the phase, and divergence between summary and detail is the kind of mismatch that breeds confusion when an implementer scans the planning file before opening the detail. Per the integrity criteria's "Task Self-Containment" line — "An implementer could pick up any single task and execute it" — the row text should not contradict the task detail.

**Current** (`planning.md` Phase 2 task table — task 2-2 row, line 83):

> | killed-sessions-resurrect-on-restart-2-2 | Replace line-262 "marker stays set so the next attach re-signals" comment with one-line recovery-contract note | preserve adjacent FIFO-unlink and warn-log comments verbatim, no behavioural change in this task, 100 ms settle-sleep absence still documented as deliberate |

**Proposed** (`planning.md` Phase 2 task table — task 2-2 row):

> | killed-sessions-resurrect-on-restart-2-2 | Replace line-262 "marker stays set so the next attach re-signals" comment with one-line recovery-contract note | preserve adjacent FIFO-unlink and warn-log comments verbatim, no behavioural change in this task, comment documents that runHydrate (per task 2-1) owns the 100 ms settle-sleep before exec |

**Current** (`planning.md` Phase 2 task table — task 2-5 row, line 86):

> | killed-sessions-resurrect-on-restart-2-5 | Unit test: handleHydrateTimeout preserves the 100 ms settle-sleep absence and FIFO-unlink ordering | elapsed time on timeout handler stays well under hydrateSettleSleep, os.Remove(cfg.FIFO) still tolerates missing FIFO silently, marker-unset call ordered before exec fall-through |

**Proposed** (`planning.md` Phase 2 task table — task 2-5 row):

> | killed-sessions-resurrect-on-restart-2-5 | Unit test: runHydrate timeout fall-through preserves the 100 ms settle-sleep, marker-unset ordering, and FIFO-unlink tolerance | elapsed time on timeout handler stays well under hydrateSettleSleep (handler does not own the sleep), os.Remove(cfg.FIFO) still tolerates missing FIFO silently, marker-unset call ordered before exec fall-through |

**Resolution**: Pending
**Notes**: Affects only the planning.md task-table summary rows. Phase-2 task detail file (phase-2-tasks.md) is already correct.

---

### 2. Phase 2 acceptance line on supersession is now under-specified after task 2-7 removal

**Severity**: Minor
**Plan Reference**: `planning.md` Phase 2 Acceptance, line 74
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:

Cycle 2 traceability removed task 2-7 ("Record spec supersession of built-in-session-resurrection lines 838 and 873 in this work unit's planning notes") because the spec already records the supersession at lines 156–163. The cycle-2 traceability tracking explicitly preserved Phase 2 acceptance line 74 (the "Spec supersession recorded ..." line) on the rationale that it now reflects an existing spec property, not a new artifact to produce.

The current wording is ambiguous about that interpretation: "Spec supersession recorded: original `built-in-session-resurrection` invariants at lines 838 and 873 are explicitly superseded by **this phase's behaviour** (no in-place edit of the original spec)." A reader of the planning file in isolation could parse this as "this phase produces a supersession record" (which would fail since no task does so) rather than "this phase's behaviour is what supersedes those invariants, and the record already lives in the killed-sessions spec at lines 156–163". The implementer could be left guessing whether they need to produce something; the simplest fix is to make the acceptance line explicit that the record already lives in the spec and that this acceptance is satisfied by Phase 2's behavioural changes (tasks 2-1 and 2-3) landing.

This is Minor because the implementer working through phase-2-tasks.md tasks 2-1 (Context) and 2-3 (Context) sees the supersession references inline, and the cycle-2 traceability finding is documented in the tracking files for any reader who escalates. But for self-containment, the acceptance line should not be ambiguous about whether it gates a deliverable.

**Current** (`planning.md` Phase 2 Acceptance, line 74):

> - [ ] Spec supersession recorded: original `built-in-session-resurrection` invariants at lines 838 and 873 are explicitly superseded by this phase's behaviour (no in-place edit of the original spec).

**Proposed** (`planning.md` Phase 2 Acceptance, line 74):

> - [ ] Spec supersession is recorded in the killed-sessions spec (lines 156–163) and is satisfied by Phase 2's behavioural changes — task 2-1 supersedes "Helper does NOT unset marker on FIFO timeout" (built-in-session-resurrection spec line 838) and task 2-3 supersedes "Resume hooks fire only at the end of successful hydration" (line 873). No in-place edit of the original spec; no separate planning-side artifact required.

**Resolution**: Pending
**Notes**: The replacement makes the satisfaction path explicit (spec records exist; Phase 2 task behaviours close the loop) and removes the ambiguity that survived task 2-7's removal. The cycle-2 traceability tracking file already contains the rationale for why no separate artifact is created.

---
