---
status: in-progress
created: 2026-05-10
cycle: 1
phase: Traceability Review
topic: killed-sessions-resurrect-on-restart
---

# Review Tracking: killed-sessions-resurrect-on-restart - Traceability

## Findings

### 1. 100 ms settle-sleep on timeout path: plan contradicts spec [Fixed]

**Type**: Hallucinated content (plan asserts behaviour that contradicts spec)
**Spec Reference**: § "Fix 2: Timeout-Path Corrections in `handleHydrateTimeout`" → "Specific Changes" → item 4 (line 154): "The 100 ms settle-sleep is preserved before exec — same posture as the success path, gives tmux time to settle the post-restore state before respawn-pane chains take over."
**Plan Reference**: Phase 2 acceptance criterion "The 100 ms settle-sleep before exec is preserved." (planning.md line 50) is correct against spec; but task 2-2 and task 2-5 in `phase-2-tasks.md` describe and pin the **opposite** semantic — that the 100 ms sleep is **skipped** on the timeout path because "nothing was dumped to settle".
**Change Type**: update-task

**Details**:
The spec is unambiguous: Fix 2 → Specific Changes → item 4 states the 100 ms settle-sleep is preserved before exec on the timeout path, with rationale "gives tmux time to settle the post-restore state before respawn-pane chains take over". This rationale is independent of whether scrollback was dumped (it is about post-restore tmux settle, not PTY-parser ingestion).

Task 2-2's proposed replacement comment encodes the opposite semantic — that the sleep is "deliberately skipped — nothing was dumped to settle". Task 2-5 then pins this opposite semantic with an assertion `elapsed < 50*time.Millisecond` (i.e. handler ran without a 100 ms sleep), and footnotes the original line 264 absence as the reference invariant. Task 2-1 also describes the pre-fix handler ordering as having "Deliberately NO 100ms sleep" and preserves that absence post-fix.

The plan's phase-level acceptance criterion ("The 100 ms settle-sleep before exec is preserved") is faithful to the spec, but the underlying tasks contradict it. An implementer following these tasks would produce a timeout handler with **no** 100 ms sleep, which violates the spec.

The fix is to update tasks 2-2 and 2-5 (and the relevant note in task 2-1) so the implemented behaviour preserves the 100 ms sleep on the timeout path before exec, matching spec item 4. If the planning agent has independent evidence that the spec's item 4 is wrong (e.g. observed pre-fix code already lacks the sleep on timeout), the spec must be amended; the plan cannot silently invert spec semantics.

**Current** (task 2-2, "Solution" paragraph):
> Replace the multi-line "deliberately NO UnsetServerOption" comment with a single-line note that documents the post-task-2-1 recovery contract: "Marker unset above (recovery path matches handleHydrateFileMissing); the 100 ms settle sleep is still skipped — no scrollback was dumped, so there is no PTY-parser settle to wait on." The adjacent comments documenting the FIFO unlink (line 252-255) and the warn-log (line 258-259) stay verbatim — they describe behaviour that is unchanged.

And task 2-2 "Do" bullet:
> Edit `cmd/state_hydrate.go` lines 262-264 (the two-line "Deliberately NO UnsetServerOption" / "Deliberately NO 100ms sleep" block). Replace with a single-line comment placed immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, of approximately this shape: `// 100 ms settle sleep is deliberately skipped — nothing was dumped to settle, so there is no PTY-parser ingestion window to wait on.`

And task 2-5 "Do" bullets:
> - Time the handler call: `start := time.Now(); err := handleHydrateTimeout(cfg); elapsed := time.Since(start)`.
> - Assert `err == nil`.
> - Assert `elapsed < 50*time.Millisecond` (well under `hydrateSettleSleep = 100*time.Millisecond`). Use 50 ms not 100 ms so a sleep regression cannot squeak past with timing jitter.

**Proposed** (task 2-2, "Solution" paragraph):
> Replace the "deliberately NO UnsetServerOption — marker stays set so the next attach re-signals" comment block with a single-line recovery-contract note immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, e.g. `// Recovery path matches handleHydrateFileMissing: marker unset above, then runHydrate execs through execShellOrHookAndExit with the 100 ms settle sleep preserved (same posture as the success path — gives tmux time to settle the post-restore state before respawn-pane chains take over).` The adjacent comments documenting the FIFO unlink (line 252-255) and the warn-log (line 258-259) stay verbatim — they describe behaviour that is unchanged.

Task 2-2 "Do" bullet (proposed):
> Edit `cmd/state_hydrate.go` lines 262-264 (the two-line "Deliberately NO UnsetServerOption" / "Deliberately NO 100ms sleep" block). Replace with a single-line comment placed immediately after the `unsetSkeletonMarkerOrLog(cfg)` call inserted by task 2-1, of approximately this shape: `// Recovery path matches handleHydrateFileMissing: marker unset above; runHydrate's exec fall-through still pays the 100 ms settle sleep before exec (preserved per spec — same posture as the success path).` This task also restores the 100 ms `time.Sleep(hydrateSettleSleep)` before the exec fall-through if it is currently absent on the timeout path; if the existing code already pays the sleep elsewhere (e.g. inside `runHydrate`'s shared post-handler block), this task only updates the comment and leaves the sleep call untouched.

Task 2-5 "Do" bullets (proposed):
> - Time the handler call: `start := time.Now(); err := handleHydrateTimeout(cfg); elapsed := time.Since(start)`.
> - Assert `err == nil`.
> - Assert the marker-unset call ordered before the handler returns (recording-commander check). Do NOT assert handler elapsed < 50 ms — the 100 ms settle-sleep must be preserved per spec § "Fix 2 → Specific Changes → 4". If the sleep lives inside `runHydrate` (post-handler) rather than inside `handleHydrateTimeout`, replace the elapsed-time assertion at the handler boundary with one at the `runHydrate` boundary that asserts elapsed >= 100 ms.

Task 2-5 "Acceptance Criteria" — replace `Handler elapsed time on a missing-FIFO input is < 50 ms.` with `runHydrate timeout fall-through preserves the 100 ms settle-sleep before exec — recorded elapsed time at the runHydrate boundary is at least hydrateSettleSleep.`

Task 2-5 "Edge Cases" — replace the `Elapsed handler time on missing-FIFO must stay well under hydrateSettleSleep (100 ms) — bound at 50 ms in the assertion.` bullet with `100 ms settle sleep on the timeout path is preserved per spec — same posture as the success path. The assertion is at the runHydrate boundary, not the handler boundary, since runHydrate owns the sleep regardless of whether handleHydrateTimeout or handleHydrateFileMissing returned.`

Task 2-1 "Do" — adjust the second bullet to remove the "100 ms sleep deliberately absent" framing and replace with a one-line note that the timeout fall-through inherits the same 100 ms settle posture as the success and file-missing paths.

**Resolution**: Fixed
**Notes**: Tasks 2-1, 2-2, and 2-5 in `phase-2-tasks.md` updated to reflect that the 100 ms settle-sleep is preserved on the timeout fall-through (per spec § Fix 2 → Specific Changes → 4). Task 2-5 elapsed-time assertion moved from handler boundary (`< 50 ms`) to runHydrate boundary (`>= hydrateSettleSleep`); sibling test `TestHydrate_Timeout_PreservesSettleSleepBeforeExec` added to acceptance/tests sections.

---

### 2. AC4 has no explicit verification task

**Type**: Incomplete coverage
**Spec Reference**: § "Acceptance Criteria → Behavioural" AC4 (line 227): "Scrollback save resumes for previously-stuck-marker panes — daemon `captureAndCommit` no longer indefinitely skips any live pane." Also referenced in § "AC ↔ Fix Traceability" (AC4 → Fix 1).
**Plan Reference**: No task or phase acceptance criterion explicitly verifies AC4 end-to-end. Phase 1 acceptance criterion mentions AC1 and AC8 by name; AC4 is not listed and has no dedicated task.
**Change Type**: add-task

**Details**:
AC1's "markers cleared within 2s" assertion implies AC4 transitively — once a marker is unset, the daemon's capture loop resumes for that pane on the next tick. But the spec lists AC4 as a separate acceptance criterion with its own verification surface ("daemon `captureAndCommit` no longer indefinitely skips any live pane"). Without an explicit task, the plan cannot prove the daemon actually captures these panes — only that markers are cleared.

The cheapest fix is a small unit-or-integration test asserting that, after eager signaling clears markers for non-attached sessions, a subsequent daemon capture-tick produces a non-empty scrollback dump for those panes. This can extend the existing Phase 1 integration test (task 1-6) rather than adding a new task — but currently neither task 1-6 nor any Phase 2 task asserts the daemon's capture-resume behaviour for previously-stuck panes.

**Current**: N/A (no current task covers AC4 verification)

**Proposed** (new task at end of Phase 1, `killed-sessions-resurrect-on-restart-1-8`):

```markdown
## killed-sessions-resurrect-on-restart-1-8 | open

### Task killed-sessions-resurrect-on-restart-1-8: Integration test asserting daemon captureAndCommit resumes for previously-stuck-marker panes (AC4)

**Problem**: AC4 ("Scrollback save resumes for previously-stuck-marker panes — daemon `captureAndCommit` no longer indefinitely skips any live pane") is the user-visible verification that Symptom C is closed. AC1's marker-cleared assertion (task 1-6) implies AC4 transitively, but does not directly observe the daemon producing a scrollback dump for a previously-stuck pane. Without an explicit AC4 test, a future regression that clears markers but breaks daemon resumption (e.g. daemon caches the suppression flag) would silently re-introduce empty-scrollback-on-next-cold-start.

**Solution**: Extend the Phase 1 multi-session integration test scaffold (task 1-6) with an additional assertion phase that, after markers transition to empty, drives one daemon capture tick (via `state.RunCaptureOnce` or equivalent test seam) and asserts a non-empty scrollback `.bin` file exists for the previously-non-attached session's pane. Use the existing `state.TailScrollback` helper to read the dump and assert at least one record was captured.

**Outcome**: AC4 is verified end-to-end. Test fails on regression of: daemon refuses to capture a pane whose marker was stuck-then-cleared, or scrollback file remains empty post-eager-signal.

**Do**:
- Extend `cmd/bootstrap/eager_signal_hydrate_integration_test.go` (added in task 1-6) with a second sub-test `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4`.
- Reuse the same N=2 saved-sessions fixture and orchestrator wiring from task 1-6.
- After the marker-cleared poll completes (markers empty within 2 s), drive one daemon capture-tick by directly invoking the daemon's capture-once primitive (e.g. `state.RunCaptureOnce(client, stateDir, logger)`; if no such primitive exists, expose one as a test seam in `internal/state` for this purpose, mirroring the existing capture-loop body).
- Assert via `state.TailScrollback(state.ScrollbackPath(stateDir, betaPaneKey), 10)` that at least one record exists for the non-attached session's pane.
- The test gates on `tmuxtest.SkipIfNoTmux(t)` and the `//go:build integration` tag, consistent with task 1-6.

**Acceptance Criteria**:
- [ ] Sub-test `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4` exists in `cmd/bootstrap/eager_signal_hydrate_integration_test.go`.
- [ ] Sub-test passes against a build with eager signaling wired; fails if the daemon refuses to capture a previously-stuck pane.
- [ ] No `t.Parallel()` usage.
- [ ] Skips cleanly under `-short` and when tmux is unavailable.

**Tests**:
- `"TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4"` — drives one capture tick post-eager-signal and asserts non-empty scrollback dump for the previously-non-attached session's pane.

**Edge Cases**:
- The capture tick must run after the `@portal-restoring` window has closed (post step 7 Clear). The orchestrator's full Run handles this — the test does not need to manually toggle the marker.
- If the daemon's capture-once primitive does not yet exist, this task adds it as a thin test seam in `internal/state` (no public API beyond the seam needed for production).

**Context**:
> Spec § "Acceptance Criteria → Behavioural" AC4: "Scrollback save resumes for previously-stuck-marker panes — daemon `captureAndCommit` no longer indefinitely skips any live pane."
>
> Spec § "AC ↔ Fix Traceability": AC4 → Fix 1 (eager signaling unsets markers, daemon resumes capturing those panes).

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Acceptance Criteria → Behavioural → AC4" and "AC ↔ Fix Traceability".
```

Also update Phase 1 acceptance criteria in `planning.md` to add the AC4 line:

Current Phase 1 acceptance block (planning.md lines 13-22):
```
**Acceptance**:
- [ ] `writeFIFOSignal` and `signalHydrateRetryDelays` are relocated from `cmd` into `internal/state` with no public API surface added; `cmd/state_signal_hydrate.go` and the new bootstrap step both call into the shared package.
- [ ] `EagerHydrateSignaler` seam interface is defined in `cmd/bootstrap` with a production adapter wired in `internal/bootstrapadapter` against `state.ListSkeletonMarkers` and `state.WriteFIFOSignal`.
- [ ] The new `EagerSignalHydrate` step runs after step 5 (Restore) and before step 6 (Clear `@portal-restoring`) — verified by an orchestrator ordering test.
- [ ] Per-FIFO write failures log a soft warning of shape `WARN | hydrate | eager-signal: write fifo <path>: <error>` and continue; the step never escalates to a fatal bootstrap error.
- [ ] Zero-marker post-Restore is a no-op — no FIFO writes attempted, step returns nil.
- [ ] Multi-session integration test (real tmux, N≥2 saved sessions): `state.ListSkeletonMarkers` returns empty within a 2-second poll window after bootstrap (AC1).
- [ ] AC8 invariant preserved: daemon `captureAndCommit` suppression during the `@portal-restoring` window is intact; no race introduced between the eager step and helper-driven scrollback replay.
- [ ] `CLAUDE.md` "Server bootstrap" section updated in the same change with renumbered step list and one-paragraph `EagerSignalHydrate` description.
- [ ] All existing happy-path resurrection integration tests and companion daemon-merge fix tests remain green.
```

Add this line before "AC8 invariant preserved":
```
- [ ] AC4 verified end-to-end: a daemon capture tick post-eager-signal produces a non-empty scrollback dump for a previously-non-attached session's pane (task 1-8).
```

**Resolution**: Pending
**Notes**:

---

### 3. AC3 verification path (kill → no resurrect) is missing from plan

**Type**: Missing from plan
**Spec Reference**: § "Acceptance Criteria → Behavioural" AC3 (line 226): "A pane killed via `portal` TUI `K` (or `tmux kill-session` from inside) does not reappear on the next `portal open`. (Already neutralised on `main` by the daemon-merge live-set filter; verified post-fix as a regression guard rather than a new behaviour.)" Also referenced in § "Empirical Reconfirmation Before Implementation Starts" (lines 326-336) — explicit branch behaviour: "If reconfirmation shows Symptom A still reproduces on `main`, plan scope adds an explicit Symptom A regression test (kill → reopen → assert absent) and AC3 graduates from 'regression guard' to 'verified fix'. If reconfirmation shows Symptom A is already neutralised, AC3 remains a regression guard and no additional task is added."
**Plan Reference**: No phase or task. The plan has no pre-flight notes recording the empirical reconfirmation outcome.
**Change Type**: add-to-task (or add a pre-flight notes section in planning.md)

**Details**:
The spec explicitly designates the planning agent as the owner of the empirical reconfirmation (§ "Empirical Reconfirmation Before Implementation Starts" → "Owner: planning agent runs the check before scoping tasks. Result recording: outcome is logged in the plan's pre-flight notes (or PR description for a one-PR plan).").

The plan currently has neither (a) a pre-flight notes section recording the reconfirmation outcome, nor (b) a Symptom-A regression test task in case the reconfirmation showed Symptom A still reproduces. Without one of these, the plan cannot signal which AC3 branch was selected, and an implementer cannot tell whether AC3 is "regression guard only" or "verified fix with an explicit test".

The fix is to add a pre-flight notes section to `planning.md` recording the empirical reconfirmation outcome (which the planning agent must perform). Branch behaviour:
- If reconfirmation shows Symptom A neutralised → record outcome in pre-flight notes; AC3 remains regression guard (no new task needed; rely on existing `internal/state/capture_test.go` filter tests + `cmd/bootstrap/stale_marker_cleanup_test.go` per § "Regression Coverage to Preserve").
- If reconfirmation shows Symptom A still reproduces → record outcome in pre-flight notes AND add a new task `killed-sessions-resurrect-on-restart-1-9` (or similar) with the explicit regression test.

**Current**: No pre-flight notes section in `planning.md`.

**Proposed** — add this section at the top of `planning.md` immediately after the `# Plan: Killed Sessions Resurrect on Restart` heading and before `## Phases`:

```markdown
## Pre-Flight Notes

### Empirical reconfirmation of Symptom A on `main`

Per spec § "Empirical Reconfirmation Before Implementation Starts", the planning agent ran the kill → reopen check on current `main` before scoping tasks.

**Outcome**: [TO BE FILLED BY PLANNING AGENT — one of:]
- *Neutralised*: Symptom A does not reproduce on `main` (the companion daemon-merge live-set filter is in effect). AC3 remains a regression guard and is satisfied by existing coverage in `internal/state/capture_test.go` filter tests and `cmd/bootstrap/stale_marker_cleanup_test.go`. No additional Symptom-A-specific task is added.
- *Still reproduces*: Symptom A reproduces on `main`. AC3 graduates to "verified fix"; an explicit regression test is added as task `killed-sessions-resurrect-on-restart-1-9` (kill → reopen → assert absent). Phase 1 acceptance is updated to include AC3 verification.

**Verification command**: Boot a tmux server, `portal open` a saved session via Portal, kill the session via TUI `K`, then `portal open` again and confirm whether the killed session reappears in the list.

**Relationship to fix scope**: Either branch ships Fix 1 / Fix 2 / Fix 3 unchanged — reconfirmation only affects whether a Symptom-A-specific test task is added, not whether the upstream-trigger fix proceeds.
```

**Resolution**: Pending
**Notes**:

---

### 4. Definition of Done — Manual Verification Protocol step is not surfaced in the plan

**Type**: Missing from plan
**Spec Reference**: § "Definition of Done" (line 254): item 3 "The Manual Verification Protocol has been executed once on a real machine; pre-fix and post-fix observations recorded in the PR description (or linked)." Also § "Manual Verification Protocol" (lines 338-351) defines the 6-step protocol. AC6 (line 232) explicitly says: "Verification is via the Manual Verification Protocol step 2 — observational, not a gated automated test."
**Plan Reference**: No task references the Manual Verification Protocol. Phase 2 acceptance criterion mentions AC6 satisfaction but does not point to the protocol or define how observation is recorded.
**Change Type**: add-task

**Details**:
The Definition of Done is part of the spec's acceptance surface — without a task that names the Manual Verification Protocol as a deliverable, the implementer has no checklist for satisfying DoD item 3. AC6 is explicitly an *observational* gate (per spec) and depends on the protocol's step 2 inspection of `~/.config/portal/state/portal.log` for two specific WARN substrings. Phase 2's acceptance criterion ("Combined with Phase 1, the two `timeout waiting for signal` and `write fifo … no such file or directory` `WARN` lines are absent in steady-state cold-start logs (AC6 fully satisfied).") is a goal but does not anchor the verification mechanism (manual log inspection per protocol step 2).

The plan should include a small terminal task that:
1. References the protocol's 6 steps.
2. Records pre-fix and post-fix observations.
3. Pastes the relevant log substrings (or absence thereof) into the PR description.

This task lives at the end of Phase 3 (after all behavioural changes are merged) since the protocol verifies the integrated end-to-end behaviour.

**Current**: N/A (no task covers Manual Verification Protocol execution)

**Proposed** (new task at end of Phase 3, `killed-sessions-resurrect-on-restart-3-4`):

```markdown
## killed-sessions-resurrect-on-restart-3-4 | open

### Task killed-sessions-resurrect-on-restart-3-4: Execute Manual Verification Protocol on a real machine and record pre/post observations in the PR description (DoD item 3, AC6)

**Problem**: Spec § "Definition of Done" item 3 mandates the Manual Verification Protocol be executed once on a real machine with pre-fix and post-fix observations recorded in the PR description (or linked). AC6 ("WARN log volume drops to zero in the steady state") is also explicitly an observational gate verified via protocol step 2 — not a gated automated test. Without a deliverable task, the DoD is unsatisfied even if all automated tests pass.

**Solution**: Execute the 6-step Manual Verification Protocol from spec § "Manual Verification Protocol" (lines 338-351) on a real machine: (1) cold-start Portal so bootstrap step 5 reconstructs saved sessions; (2) inspect `~/.config/portal/state/portal.log` for the two WARN substrings; (3) inspect server options for stuck `@portal-skeleton-*` markers; (4) kill an affected session; (5) `portal open` again and confirm session stays gone; (6) verify on-resume hook ran in an affected pane. Plus the two additional Defect-D checks: `pgrep -fa "sh -c.*portal state hydrate"` returns no rows, and `exit` typed once closes a restored pane. Record each step's observation (pre-fix and post-fix) in the PR description.

**Outcome**: PR description contains a Manual Verification section with each protocol step's pre-fix and post-fix observation. AC6 is gated by the absence of the two named WARN substrings in `~/.config/portal/state/portal.log` after a clean cold-start with N≥2 saved sessions.

**Do**:
- Set up two side-by-side environments: one on `main` pre-fix, one on the integration branch post-fix. (Or run pre-fix observations once before merging Phase 1, and post-fix observations once after Phase 3 lands.)
- For each environment, walk through the 6 protocol steps and the two additional Defect-D checks. Record observations verbatim.
- Paste a structured Markdown table into the PR description with columns: `Step`, `Pre-fix observation`, `Post-fix observation`, `Pass/Fail`.
- AC6 specifically: confirm `~/.config/portal/state/portal.log` does **not** contain the substrings `WARN | hydrate | write fifo` or `WARN | hydrate | timeout waiting for signal` after a clean cold-start with N≥2 saved sessions.

**Acceptance Criteria**:
- [ ] PR description contains a "Manual Verification" section with all 6 protocol steps and the 2 additional Defect-D checks.
- [ ] Each step has a pre-fix and post-fix observation recorded.
- [ ] AC6 step 2: post-fix log contains zero occurrences of `WARN | hydrate | write fifo` and `WARN | hydrate | timeout waiting for signal`.
- [ ] AC5 additional check: post-fix `pgrep` returns zero rows; `exit` typed once closes the pane.
- [ ] Pre-fix observations were taken on a build that does not yet include Fixes 1/2/3 (e.g. `main` immediately before this work unit's branch was merged, or the integration branch with all three fixes reverted locally).

**Tests**: None — this task is observational verification per spec.

**Edge Cases**:
- N≥2 saved sessions are required to reproduce the multi-session bug behaviour pre-fix (single-saved-session users are unaffected).
- If a real machine is not available, the task can be deferred to a reviewer who has one — but DoD item 3 still requires it before merge.

**Context**:
> Spec § "Manual Verification Protocol" lines 338-351: the canonical 6-step protocol plus the two Defect-D checks.
>
> Spec § "Acceptance Criteria → Logging → AC6": "Verification is via the Manual Verification Protocol step 2 — observational, not a gated automated test."
>
> Spec § "Definition of Done" item 3: "The Manual Verification Protocol has been executed once on a real machine; pre-fix and post-fix observations recorded in the PR description (or linked)."

**Spec Reference**: `.workflows/killed-sessions-resurrect-on-restart/specification/killed-sessions-resurrect-on-restart/specification.md` § "Manual Verification Protocol", "Acceptance Criteria → AC6", "Definition of Done → item 3".
```

Also update Phase 3 acceptance criteria in `planning.md` to add the Manual Verification line. Current Phase 3 acceptance block (planning.md lines 80-87):
```
**Acceptance**:
- [ ] `buildHydrateCommand` returns the bare `portal state hydrate --fifo <fifo> --file <file> --hook-key <hookKey>` string with each value-arg shell-escaped via the existing `internal/tmux` quoting helper; no `sh -c` envelope, no `; exec $SHELL` trailer.
- [ ] `RespawnPane` interface signature is unchanged — still accepts a single command-string argument.
- [ ] Unit/snapshot test in `session_test.go` updated to assert the new bare-command shape on representative inputs.
- [ ] Inner `sh -c '<HOOK>; exec $SHELL'` constructed inside `execShellOrHookAndExit` when an on-resume hook is registered is untouched — hook-firing semantics preserved exactly.
- [ ] Integration test: `exit` typed once in a restored pane closes the pane (tmux `list-panes` shows the pane gone, not respawned with a fresh shell) — AC5.
- [ ] Integration / manual check: `pgrep -fa "sh -c.*portal state hydrate"` returns no rows for any restored pane.
- [ ] All existing happy-path resurrection integration tests remain green.
```

Add this line at the end:
```
- [ ] Manual Verification Protocol executed on a real machine; pre-fix and post-fix observations recorded in the PR description (DoD item 3, AC6 observational gate via protocol step 2).
```

**Resolution**: Pending
**Notes**:

---

### 5. CLAUDE.md update is a Definition of Done item but is only listed as Phase 1 acceptance

**Type**: Incomplete coverage
**Spec Reference**: § "Definition of Done" item 4 (line 259): "`CLAUDE.md` 'Server bootstrap' section is updated with the new step list." Also § "Bootstrap Step Numbering Update" (line 121): "The `CLAUDE.md` 'Server bootstrap' section is updated **as part of the same PR**."
**Plan Reference**: Task 1-7 covers the CLAUDE.md update. Phase 1 acceptance has the line "CLAUDE.md 'Server bootstrap' section updated in the same change with renumbered step list and one-paragraph EagerSignalHydrate description." This is fine for Phase 1, but the DoD item is not surfaced anywhere as a top-level plan-level acceptance.
**Change Type**: (none required if treated as Phase 1 acceptance only — flagged for awareness)

**Details**:
The plan's Phase 1 covers the CLAUDE.md update, which is sufficient. This finding is informational only — flagging that DoD item 4 is satisfied transitively via task 1-7 and Phase 1 acceptance. No change to the plan is required unless the user wants a top-level "Definition of Done" checklist added to `planning.md`.

If the user wants top-level DoD coverage, propose adding a `## Definition of Done` section at the bottom of `planning.md` listing each DoD item with its satisfying task/phase. This is optional polish — not a traceability gap by itself.

**Current**: No top-level Definition of Done section in `planning.md`.

**Proposed** (optional — only if user wants DoD checklist surfaced at plan level): add at bottom of `planning.md`:

```markdown
## Definition of Done

Per spec § "Definition of Done":

- [ ] All unit and integration tests in the Test Plan pass in CI — covered by Phase 1/2/3 task acceptance criteria.
- [ ] Existing tests under "Regression Coverage to Preserve" remain green — Phase 1 final acceptance criterion.
- [ ] Manual Verification Protocol has been executed once on a real machine; pre-fix and post-fix observations recorded in the PR description — task 3-4.
- [ ] `CLAUDE.md` "Server bootstrap" section is updated with the new step list — task 1-7.
- [ ] PR is reviewed and merged to `main` — out of scope for the planning artifact; tracked on the PR itself.
```

**Resolution**: Pending
**Notes**: Optional / informational only. If the user prefers minimal additions, this finding can be marked rejected with no change.

---
