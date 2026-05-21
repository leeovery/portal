---
status: in-progress
created: 2026-05-21
cycle: 1
phase: Traceability Review
topic: killed-session-resurrects-within-tick-window
---

# Review Tracking: Killed Session Resurrects Within Tick Window - Traceability

## Findings

### 1. `@portal-restoring` defence under real tmux — marker-clear half of the test case is not covered

**Type**: Incomplete coverage
**Spec Reference**: § Testing Requirements > Integration Tests (Real Tmux Fixture, Required) > "`@portal-restoring` defence under real tmux. Set the marker, fire `session-closed`, assert `sessions.json` is untouched. Clear the marker, fire `session-closed` again, assert the file updates correctly."
**Plan Reference**: Phase 1, Task 1-6 (Real-tmux kill→bootstrap canonical symptom integration test), sub-test 3
**Change Type**: update-task

**Details**:
The spec calls out a specific real-tmux integration test for the `@portal-restoring` defence that has **two halves**:

1. Marker set → fire `session-closed` → assert `sessions.json` is untouched.
2. **Clear marker → fire `session-closed` again → assert `sessions.json` updates correctly.**

Task 1-6 sub-test 3 only covers half 1 (marker set, file byte-identical via `_portal-saver` kill). The marker-clear continuation is not asserted as a same-test transition — verifying the short-circuit lifts cleanly and the synchronous commit resumes once `@portal-restoring` is cleared. Task 1-5 (re-entrancy gate) and Task 1-6 sub-test 1 (canonical symptom) both exercise the marker-clear path, but neither asserts the **marker-set → marker-clear transition within the same fixture**, which is the discriminator the spec lists (it confirms the short-circuit gate is queryable per-invocation, not a static state captured at boot).

Extending Task 1-6 sub-test 3 to clear the marker and fire `session-closed` a second time is the smallest fix.

**Current**:
```markdown
- Sub-test 3 — `_portal-saver` self-kill, marker set:
  1. Bootstrap to stable state.
  2. Manually set `@portal-restoring` on the tmux server via the test's tmux client (simulating the bootstrap-step-4 timeline window).
  3. Snapshot `sessions.json` bytes.
  4. Kill `_portal-saver` via `tmux kill-session -t _portal-saver`.
  5. Wait briefly (e.g., 250ms) for the hook subprocess to run and exit.
  6. Re-read `sessions.json` bytes; assert byte-identical to the snapshot.
  7. Clear `@portal-restoring`.
```

**Proposed**:
```markdown
- Sub-test 3 — `@portal-restoring` defence end-to-end (covers spec § Testing Requirements > "`@portal-restoring` defence under real tmux"):
  1. Bootstrap to stable state with user sessions A and B and `_portal-saver` running.
  2. Manually set `@portal-restoring` on the tmux server via the test's tmux client (simulating the bootstrap-step-4 timeline window).
  3. Snapshot `sessions.json` bytes.
  4. Kill `_portal-saver` via `tmux kill-session -t _portal-saver`.
  5. Wait briefly (e.g., 250ms) for the hook subprocess to run and exit.
  6. Re-read `sessions.json` bytes; assert byte-identical to the snapshot (the marker-set short-circuit fired).
  7. Clear `@portal-restoring`.
  8. Kill session `B` via `tmux kill-session -t B` (a normal user kill now that the marker is clear).
  9. Poll `sessions.json` (two-consecutive-consistent-reads, ≤1.5s budget) and assert B is absent and A is present — verifies the marker-clear path resumes the synchronous commit on the same fixture.
```

Also extend the **Acceptance Criteria** list:

Add a new bullet after the existing sub-test 3 acceptance criterion:

```markdown
- [ ] Sub-test 3 demonstrates: after clearing `@portal-restoring` on the same fixture, a subsequent `tmux kill-session -t B` results in `sessions.json` omitting B within the bounded poll window — verifies the short-circuit gate is per-invocation, not a static state captured at boot.
```

And add a new named assertion to the **Tests** list:

```markdown
- `"after clearing @portal-restoring on the same fixture, a subsequent kill updates sessions.json correctly"` (sub-test 3 continuation)
```

**Resolution**: Fixed
**Notes**: Auto-applied verbatim — sub-test 3 extended to include the marker-clear continuation; acceptance criteria and Tests list updated.

---

### 2. `IsRestoringSet` query failure mode — plan introduces a design decision the spec does not make

**Type**: Hallucinated content
**Spec Reference**: § `@portal-restoring` Defence (describes the read, does not address query failure); § `commit-now` Failure Behaviour (addresses `CaptureStructure` / `Commit` failures, not the marker query)
**Plan Reference**: Phase 1, Task 1-3 (Add `commit-now` failure-path discipline)
**Change Type**: update-task

**Details**:
Task 1-3 commits to a design choice not made in the specification: if the `@portal-restoring` query (`IsRestoringSet`) returns an error, `commit-now` proceeds (fail-open) with the commit attempt rather than skipping.

The spec discusses three failure paths explicitly: `CaptureStructure` error, `Commit` error, and `save.requested` touch error. It does **not** address the failure mode of the marker query itself. The plan invents this decision with rationale ("we cannot prove restoration is in progress; suppressing the commit would re-open the resurrection window") and even raises a counter-argument inline ("Alternative — fail-closed and skip — would re-open the original bug on transient tmux query glitches; the spec's framing favours kill-side correctness over restoration-window safety on this narrow edge").

Per the traceability standard, content that "cannot be traced to the specification" must be removed or flagged. This is a real spec gap — the safe direction (fail-open vs fail-closed on a query that gates a no-op short-circuit) is non-obvious and has correctness implications (fail-open during a real restoration could corrupt the restore; fail-closed during a transient glitch re-opens the resurrection window).

The fix is one of:
1. Remove the fail-open commitment from Task 1-3 and add a `[needs-info]` flag for the spec phase to resolve.
2. Leave it in the plan but mark it as a documented plan-level choice that survives spec re-entry if the re-entrancy gate (Task 1-5) fails.

Proposing option 1: surface this as a spec gap rather than a silent plan-level decision.

**Current** (in Task 1-3 Do list):
```markdown
- The `IsRestoringSet` query (added in task 1-2) is also wrapped: on error, treat as "marker indeterminate" → safer default is to **proceed with the commit attempt** (we cannot prove restoration is in progress; suppressing the commit would re-open the resurrection window). If the subsequent capture/commit fails it lands in the same failure path. Document this choice in code comments. (Alternative — fail-closed and skip — would re-open the original bug on transient tmux query glitches; the spec's framing favours kill-side correctness over restoration-window safety on this narrow edge.)
```

**Current** (in Task 1-3 Acceptance Criteria):
```markdown
- [ ] When `IsRestoringSet` returns an error, `commit-now` proceeds with the commit attempt (does not silently short-circuit). If the subsequent commit fails it falls through the standard failure path.
```

**Current** (in Task 1-3 Tests):
```markdown
- `"it proceeds to commit when IsRestoringSet returns an error"`
```

**Current** (in Task 1-3 Edge Cases):
```markdown
- `IsRestoringSet` query errors: proceed (fail-open) rather than skipping; documented in code comment.
```

**Proposed** (replace the Do list bullet):
```markdown
- `[needs-info]` The `IsRestoringSet` query failure mode is not addressed by the specification. The spec's `@portal-restoring` Defence section describes the read but is silent on what happens when the read itself errors. This is a non-trivial correctness decision (fail-open during a real restoration could corrupt the restore; fail-closed during a transient tmux query glitch re-opens the resurrection window). **Resolve in spec before implementation:** does `commit-now` fail-open (proceed with commit) or fail-closed (skip + touch `save.requested`) when the marker query errors? Pending resolution, this task does not implement either branch.
```

**Proposed** (remove from Acceptance Criteria):
```markdown
(remove the "When `IsRestoringSet` returns an error" bullet — re-add after spec resolves the gap)
```

**Proposed** (remove from Tests):
```markdown
(remove `"it proceeds to commit when IsRestoringSet returns an error"` — re-add after spec resolves the gap, with the test name aligned to the resolved direction)
```

**Proposed** (replace in Edge Cases):
```markdown
- `IsRestoringSet` query errors: behaviour not specified — see `[needs-info]` in Do list. Implementation blocked on spec resolution.
```

**Resolution**: Pending
**Notes**: Plan-level resolution is acceptable if the user explicitly endorses the fail-open choice; the finding's existence is to surface that this is a decision being made, not to mandate which direction. If endorsed, the change-type becomes "no change" and the rationale already in the task suffices.

---
