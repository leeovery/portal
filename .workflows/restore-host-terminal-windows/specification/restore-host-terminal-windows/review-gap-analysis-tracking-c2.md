---
status: complete
created: 2026-07-11
cycle: 2
phase: Gap Analysis
topic: restore-host-terminal-windows
---

# Review Tracking: restore-host-terminal-windows - Gap Analysis

## Findings

### 1. Pre-flight-abort selection state is unspecified (gone session's mark + retry path)

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Burst & Partial-Failure Contract (§Stance: pre-flight + all-or-nothing); Multi-Select Mode (§Sticky selection); Design References (frame 2, pre-flight abort)

**Details**:
Cycle 1 specified the *post-pre-flight partial-failure* mark handling with precision — "unmarks the sessions whose windows opened and keeps the failed/un-acked ones marked, so a second `Enter` retries exactly the missing set." But the parallel *pre-flight abort* path leaves the gone session's mark state undetermined, and this now reads two ways:

- The abort text says "stay put in multi-select mode with the **remaining selections intact**." Read one way, "remaining" implies the gone session is **pruned** from the selection (so a second `Enter` proceeds with the survivors).
- Design frame 2 shows the gone session "**flagged** with a red `⚠` + `session gone`, other selections intact" — implying it stays **marked and shown**, not pruned. Under that reading a second `Enter` re-runs pre-flight, finds the same session still gone, and **re-aborts** — leaving the user in a loop unless they manually `m`-toggle the flagged row off. The spec never states that the user must unmark it (nor that it auto-prunes), so the retry-out path is unspecified.

Compounding this, §Sticky selection prunes a killed session's selection on the `Space`-preview round-trip and calls that "**consistent with the pre-flight rule**." But the pre-flight rule as written *flags-and-keeps* the gone session rather than pruning it — so the claimed consistency does not actually hold. The two events (preview return vs. Enter pre-flight) handle the identical situation (a marked session killed mid-picker) via different mechanisms with no stated reconciliation.

Net: a planner must decide (a) whether a pre-flight abort removes the gone session from the selection or keeps it marked, and (b) how the user retries — auto-proceed with survivors, or manual-unmark-then-Enter. These produce materially different UX for the primary failure path, and the spec's own two references point in opposite directions.

**Proposed Addition**:
Revised the pre-flight abort bullets in §Stance to specify: on abort, name the gone session(s) in the error, then **prune the gone session(s) from the selection** and keep surviving marks intact, so a second `Enter` proceeds with the survivors rather than re-aborting in a loop. Added an explicit note that this is the same prune-what's-gone rule as the sticky-selection preview round-trip — making the "consistent with the pre-flight rule" claim in §Sticky selection actually hold.

**Resolution**: Approved
**Notes**: Approved via auto. **Judgment call:** chose auto-prune (gone session dropped from selection) over keep-marked, to avoid the re-abort loop and to unify with the sticky-selection preview-prune rule. Design frame 2's red `⚠ session gone` flag is the error presentation at the moment of abort; the pruned session drops from the marked set on the next rebuild. Survivors stay marked.

---
