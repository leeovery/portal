---
status: complete
created: 2026-05-10
cycle: 2
phase: Traceability Review
topic: killed-sessions-resurrect-on-restart
---

# Review Tracking: killed-sessions-resurrect-on-restart - Traceability

## Findings

### 1. Task 2-7 invents a planning-side supersession artifact the spec does not require [Fixed]

**Type**: Hallucinated content
**Spec Reference**: § "Fix 2 → Spec Supersession (Original Resurrection Spec)" (lines 156–163): "This change deliberately supersedes two invariants from `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`: ... The original session-resurrection spec is not modified in place; **the supersession is recorded here** as the canonical updated semantic for the timeout path."
**Plan Reference**: `phase-2-tasks.md` task `killed-sessions-resurrect-on-restart-2-7` ("Record spec supersession of built-in-session-resurrection lines 838 and 873 in this work unit's planning notes"). Also `planning.md` Phase 2 acceptance line 73: "Spec supersession recorded: original `built-in-session-resurrection` invariants at lines 838 and 873 are explicitly superseded by this phase's behaviour (no in-place edit of the original spec)."
**Change Type**: remove-task

**Details**:
The spec phrase "the supersession is recorded here" refers to the killed-sessions-resurrect-on-restart specification document itself — the supersession is already authored at lines 156–163 of that spec (verbatim quotes of the two superseded lines, plus replacement semantics for each). The spec does not request, mention, or describe any additional planning-side artifact such as `phase-2-supersession.md`.

Task 2-7 invents a new deliverable: a markdown file at `.workflows/killed-sessions-resurrect-on-restart/planning/killed-sessions-resurrect-on-restart/phase-2-supersession.md` that re-quotes the same two superseded lines, restates the replacement semantics, and adds a verification trail. This duplicates content that already lives in the spec, adds maintenance burden (two files now have to stay in sync), and was never validated as a deliverable during specification.

The spec's actual supersession recording obligation is satisfied the moment the killed-sessions spec is written and committed (which it is). No further plan-time deliverable is required.

If the planning agent's intent was discoverability (so a reader of the original built-in-session-resurrection spec who searches the repo for the quoted invariants finds the supersession note), that goal is already achieved by the spec at lines 156–163 — those lines quote the original invariants verbatim and a repo search lands on the killed-sessions spec directly.

The fix is to remove task 2-7 entirely. Phase 2's existing acceptance line 73 stays — it correctly reflects that the supersession is recorded (in the spec, where it already is), not that a new file is created.

**Current** (task 2-7, full content from `phase-2-tasks.md` lines 282–331):
> ## killed-sessions-resurrect-on-restart-2-7 | approved
>
> ### Task killed-sessions-resurrect-on-restart-2-7: Record spec supersession of built-in-session-resurrection lines 838 and 873 in this work unit's planning notes (no in-place edit of the original spec)
>
> **Problem**: This phase's behavioural changes (marker unset on timeout; hooks fire on timeout) deliberately supersede two invariants from `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`. The original spec must NOT be edited in place (per spec § "Fix 2 → Spec Supersession": "The original session-resurrection spec is not modified in place; the supersession is recorded here as the canonical updated semantic for the timeout path"). A small markdown note in this work unit's planning directory makes the supersession discoverable to future readers of the original spec without rewriting history.
>
> **Solution**: Author a single markdown file `phase-2-supersession.md` under `.workflows/killed-sessions-resurrect-on-restart/planning/killed-sessions-resurrect-on-restart/` that quotes the two original-spec lines verbatim, states the replaced semantic alongside each, and links the supersession back to AC2 and AC6 of this work unit's spec. The file is implementation deliverable for this task — task description specifies its content shape; the implementer writes it.
>
> **Outcome**: A discoverable supersession record exists in the planning directory. Anyone reading the original built-in-session-resurrection spec who searches the repo for the quoted invariants finds this note and learns that the post-Phase-2 semantics differ. The original spec file is byte-identical pre/post this task.
>
> **Do**:
> - Create `.workflows/killed-sessions-resurrect-on-restart/planning/killed-sessions-resurrect-on-restart/phase-2-supersession.md` with the following content shape:
>   - **Title**: `# Spec Supersession: built-in-session-resurrection (Phase 2)`.
>   - **One-paragraph preamble**: state that this work unit (`killed-sessions-resurrect-on-restart`) supersedes two invariants from the original `built-in-session-resurrection` specification, that the original file is intentionally not edited, and link the original path: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`.
>   - **Section "Superseded Invariant 1 (Original line 838)"**:
>     - Quote the original verbatim: `"Helper does NOT unset marker on FIFO timeout — next attach re-signals, retry happens naturally."`
>     - State the replaced semantic: `Helper unsets the @portal-skeleton-<paneKey> marker on FIFO timeout via unsetSkeletonMarkerOrLog. The original "next attach re-signals" promise was non-deliverable: the FIFO is unlinked at the same site, leaving no reader for any subsequent signal. Leaving the marker set fed Symptom C (stuck markers suppress scrollback save indefinitely).`
>     - Reference: link to AC2 and AC6 of the killed-sessions-resurrect-on-restart spec (`.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Acceptance Criteria → AC2 / AC6") and to phase-2-tasks.md tasks 2-1 (marker unset) and 2-3 (hook fires).
>   - **Section "Superseded Invariant 2 (Original line 873)"**:
>     - Quote the original verbatim: `"Resume hooks fire only from inside the hydrate helper's exec chain, at the end of successful hydration."`
>     - State the refined semantic: `Resume hooks fire from inside the hydrate helper's exec chain on any non-fatal terminal path — successful hydration, file-missing recovery, and timeout recovery. The original phrasing reflected an assumption that timeout was an exceptional condition; in practice (pre-Fix 1) it was the steady state, which made the "hooks unsafe on timeout" rationale incoherent. With Fix 1 (eager signaling) in place, timeout is genuinely rare; when it fires, the recovery path matches file-missing's already-tested behaviour.`
>     - Reference: same AC2/AC6 and tasks-2-3/2-4 links as above.
>   - **Section "Verification Trail"**:
>     - One-line entries linking to the unit tests that pin the new semantics: `cmd/state_hydrate_test.go::TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` (task 2-1), `TestHydrate_Timeout_FiresHookWhenRegistered` (task 2-3), and the integration test `cmd/bootstrap/phase2_hook_fire_integration_test.go::TestPhase2_HookFiresOnNonAttachedSession_AC2` (task 2-6).
> - Do NOT touch `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`. Verify with `git diff` that the only file added by this task is `phase-2-supersession.md`.
>
> **Acceptance Criteria**:
> - [ ] `phase-2-supersession.md` exists at `.workflows/killed-sessions-resurrect-on-restart/planning/killed-sessions-resurrect-on-restart/phase-2-supersession.md`.
> - [ ] Both original-spec lines (838 and 873) are quoted verbatim in fenced quote blocks.
> - [ ] Each superseded invariant has its replaced semantic stated alongside.
> - [ ] Each invariant's section links back to AC2 and AC6 of this work unit's specification.
> - [ ] The original file `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` is byte-identical pre/post this task — verified via `git diff`.
> - [ ] The Verification Trail section links to the three tests added by tasks 2-1, 2-3, and 2-6.
>
> **Tests**:
> - No new test cases — this task is documentation-only. The acceptance criteria above pin the substantive checks (file exists at canonical path, both invariants quoted verbatim, AC2/AC6 referenced explicitly, original spec file byte-identical).
>
> **Edge Cases**:
> - Original spec file untouched — verify with `git diff` that the only file added by this task is `phase-2-supersession.md`.
> - Supersession note links Phase 2 acceptance back to AC2 and AC6 — both must appear as explicit substrings in the note (not transitively via the spec link).
> - Lines 838 and 873 quoted verbatim — copy from the file, do not paraphrase. The replaced semantic is stated alongside each quote, not embedded in the quote block.
>
> **Context**:
> > Spec § "Fix 2 → Spec Supersession (Original Resurrection Spec)": "This change deliberately supersedes two invariants from `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`: ... The original session-resurrection spec is not modified in place; the supersession is recorded here as the canonical updated semantic for the timeout path."
>
> > Spec § "Acceptance Criteria → Spec Conformance": Phase 2's behavioural change deliberately deviates from line 873 of the original spec; this supersession note is the discoverable record of that deviation for future readers.
>
> > `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` lines 838 and 873 — the two invariants quoted verbatim in the new supersession note.
>
> **Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Fix 2 → Spec Supersession (Original Resurrection Spec)"

**Resolution**: Fixed
**Notes**: Task 2-7 removed from phase-2-tasks.md (replaced with a [Removed] note); planning.md Phase 2 task table row deleted; phase-2-tasks.md front-matter `total: 7` → `total: 6`; tick task tick-c9dbd7 removed via `tick remove --force`; manifest task_map entry deleted. The Phase 2 acceptance bullet at planning.md line 73 ("Spec supersession recorded ...") stays — it correctly reflects that the supersession is recorded in the killed-sessions spec at lines 156–163, not in a planning artifact.

---

### 2. Pre-Flight Notes contradict themselves on whether the planning agent ran the empirical reconfirmation [Fixed]

**Type**: Incomplete coverage
**Spec Reference**: § "Empirical Reconfirmation Before Implementation Starts" (lines 326–336): "**Owner**: planning agent runs the check before scoping tasks. **Result recording**: outcome is logged in the plan's pre-flight notes (or PR description for a one-PR plan)."
**Plan Reference**: `planning.md` Pre-Flight Notes section (lines 4–15).
**Change Type**: update-task

**Details**:
The spec assigns ownership of the empirical reconfirmation explicitly to the planning agent ("planning agent runs the check before scoping tasks") and requires the outcome to be logged in the pre-flight notes. The plan's Pre-Flight Notes section is internally contradictory:

1. Line 7 claims the planning agent did run the check: "the planning agent ran the kill → reopen check on current `main` before scoping tasks."
2. Line 9 then defers the outcome to the implementer: "**Outcome**: [TO BE FILLED BY THE IMPLEMENTER BEFORE PHASE 1 STARTS — one of:]"

Either the planning agent ran the check (in which case the outcome should be recorded now, before approval) or didn't (in which case the spec's owner assignment was deferred and that deferral must be acknowledged honestly). The current text claims the former but delivers the latter.

The spec's branch behaviour ("If reconfirmation shows Symptom A still reproduces on `main`, plan scope adds an explicit Symptom A regression test... and AC3 graduates from 'regression guard' to 'verified fix'") is contingent on the outcome being known *before* tasks are scoped — because the *Still reproduces* branch adds a task (`killed-sessions-resurrect-on-restart-1-9`) which would change Phase 1's task count from 8 to 9. Deferring the outcome to the implementer leaves the plan in a Schrödinger's-task-count state until Phase 1 begins, which violates the spec's scoping contract.

The fix is one of two options:

- **Option A (preferred)**: The planning agent runs the check now and records a definite outcome (Neutralised vs Still reproduces). If Still reproduces, task 1-9 is added now and Phase 1 task table updated to 9 tasks. Phase 1 acceptance for AC3 is updated accordingly.
- **Option B (acceptable fallback)**: The plan acknowledges the check was deferred to the implementer, and Phase 1 explicitly carries the conditional task count (8 if Neutralised; 9 if Still reproduces). The first sentence of Pre-Flight Notes is rewritten to say "the planning agent has deferred the kill → reopen check to the implementer (per the spec's branch-behaviour contract)" rather than claiming the check was run.

Either resolution removes the internal contradiction. The current state is misleading to a reader who takes line 7 at face value.

**Current** (planning.md lines 4–15):
> ### Empirical reconfirmation of Symptom A on `main`
>
> Per spec § "Empirical Reconfirmation Before Implementation Starts", the planning agent ran the kill → reopen check on current `main` before scoping tasks.
>
> **Outcome**: [TO BE FILLED BY THE IMPLEMENTER BEFORE PHASE 1 STARTS — one of:]
> - *Neutralised*: Symptom A does not reproduce on `main` (the companion daemon-merge live-set filter is in effect). AC3 remains a regression guard and is satisfied by existing coverage in `internal/state/capture_test.go` filter tests and `cmd/bootstrap/stale_marker_cleanup_test.go`. No additional Symptom-A-specific task is added.
> - *Still reproduces*: Symptom A reproduces on `main`. AC3 graduates to "verified fix"; an explicit regression test is added as task `killed-sessions-resurrect-on-restart-1-9` (kill → reopen → assert absent). Phase 1 acceptance is updated to include AC3 verification.
>
> **Verification command**: Boot a tmux server, `portal open` a saved session via Portal, kill the session via TUI `K`, then `portal open` again and confirm whether the killed session reappears in the list.
>
> **Relationship to fix scope**: Either branch ships Fix 1 / Fix 2 / Fix 3 unchanged — reconfirmation only affects whether a Symptom-A-specific test task is added, not whether the upstream-trigger fix proceeds.

**Proposed** (Option B — fallback honest framing; pick this if the planning agent has not actually run the check):
> ### Empirical reconfirmation of Symptom A on `main`
>
> Per spec § "Empirical Reconfirmation Before Implementation Starts", the kill → reopen check against current `main` is required before scoping tasks. The planning agent has **deferred this check to the implementer** because the planning environment lacks a real tmux + Portal cold-start fixture; the spec's branch-behaviour contract is preserved by carrying both branches as conditional plan scope below.
>
> **Required action before Phase 1 starts** — the implementer runs the verification command and records the outcome here, then applies the matching branch:
> - *Neutralised*: Symptom A does not reproduce on `main` (the companion daemon-merge live-set filter is in effect). AC3 remains a regression guard and is satisfied by existing coverage in `internal/state/capture_test.go` filter tests and `cmd/bootstrap/stale_marker_cleanup_test.go`. No additional Symptom-A-specific task is added; Phase 1 task count stays at 8.
> - *Still reproduces*: Symptom A reproduces on `main`. AC3 graduates to "verified fix"; an explicit regression test is added as task `killed-sessions-resurrect-on-restart-1-9` (kill → reopen → assert absent). Phase 1 task count increases to 9 and Phase 1 acceptance is updated to include AC3 verification.
>
> **Outcome**: [TO BE FILLED BY THE IMPLEMENTER BEFORE PHASE 1 STARTS]
>
> **Verification command**: Boot a tmux server, `portal open` a saved session via Portal, kill the session via TUI `K`, then `portal open` again and confirm whether the killed session reappears in the list.
>
> **Relationship to fix scope**: Either branch ships Fix 1 / Fix 2 / Fix 3 unchanged — reconfirmation only affects whether a Symptom-A-specific test task is added, not whether the upstream-trigger fix proceeds.

**Resolution**: Fixed
**Notes**: Option B applied — Pre-Flight Notes section in planning.md rewritten to honestly acknowledge that the planning environment lacks a real tmux + Portal cold-start fixture and the check is deferred to the implementer with explicit branch contract. Option A would have required a tmux server, unavailable here.

---

### 3. Phase 2 acceptance overstates AC6 satisfaction [Fixed]

**Type**: Incomplete coverage (mis-framing)
**Spec Reference**: § "Acceptance Criteria → Logging → AC6" (line 232): "**Verification is via the Manual Verification Protocol step 2** — observational, not a gated automated test."
**Plan Reference**: `planning.md` Phase 2 acceptance line 71: "Combined with Phase 1, the two `timeout waiting for signal` and `write fifo … no such file or directory` `WARN` lines are absent in steady-state cold-start logs (AC6 fully satisfied)."
**Change Type**: update-task

**Details**:
The Phase 2 acceptance bullet states AC6 is "fully satisfied" by Phase 2 (combined with Phase 1). The spec is explicit that AC6's verification gate is *observational only*, via Manual Verification Protocol step 2 — Phase 2's behavioural changes provide a *prerequisite* for AC6 (the warnings must stop firing) but do not *verify* it. The actual AC6 verification is owned by task 3-4 ("Execute Manual Verification Protocol on a real machine and record pre/post observations in the PR description (DoD item 3, AC6)").

The Phase 3 acceptance line 104 phrases the same relationship correctly: "Manual Verification Protocol executed on a real machine; pre-fix and post-fix observations recorded in the PR description (DoD item 3, AC6 observational gate via protocol step 2)."

The Phase 2 phrasing risks an implementer concluding AC6 is closed once Phase 2 lands and skipping the manual verification — which would leave DoD item 3 unsatisfied. The fix is a small wording correction so the bullet reflects the *behavioural prerequisite* without claiming verification.

**Current** (planning.md line 71):
> - [ ] Combined with Phase 1, the two `timeout waiting for signal` and `write fifo … no such file or directory` `WARN` lines are absent in steady-state cold-start logs (AC6 fully satisfied).

**Proposed**:
> - [ ] Combined with Phase 1, the behavioural prerequisites for AC6 are met — the two `timeout waiting for signal` and `write fifo … no such file or directory` `WARN` lines no longer fire in steady-state cold-start. AC6's observational verification gate is owned by task 3-4 (Manual Verification Protocol step 2); this phase does not close AC6 on its own.

**Resolution**: Fixed
**Notes**: planning.md Phase 2 acceptance bullet rewritten to frame the change as a behavioural prerequisite for AC6 with the observational verification gate explicitly owned by task 3-4.

---
