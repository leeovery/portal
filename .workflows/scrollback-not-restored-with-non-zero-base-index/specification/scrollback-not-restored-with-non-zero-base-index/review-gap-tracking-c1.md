---
status: complete
created: 2026-04-30
cycle: 1
phase: Gap Analysis
topic: scrollback-not-restored-with-non-zero-base-index
---

# Review Tracking: scrollback-not-restored-with-non-zero-base-index - Gap Analysis

## Findings

### 1. Eviction mechanism for migration not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 1 → "One-shot bootstrap migration"

**Details**:
The spec mandates that `RegisterPortalHooks` "evict any hook entry whose command contains `portal state signal-hydrate` but does not contain the `--` separator before installing the fixed entry," but does not specify which tmux package API performs the eviction, the addressing scheme, or the ordering.

**Proposed Addition**:
Add a "Migration mechanics (explicit)" sub-block to Part 1 covering: Eviction API (`ShowGlobalHooks`, `ParseShowHooks`, `UnsetGlobalHookAt`); ordering (scan-then-evict-then-install, highest index first); operator visibility (single INFO line per non-empty migration).

**Resolution**: Approved
**Notes**: Applied as a single bulleted "Migration mechanics" block alongside Findings 2, 3, 7.

---

### 2. Hook event scope for migration scan not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 1 → "One-shot bootstrap migration"

**Details**:
Migration must enumerate which hook events the scan iterates over. State explicitly that the scan covers all events in `hydrationTriggerEvents` (currently `client-attached` and `client-session-changed`).

**Proposed Addition**:
Bullet under "Migration mechanics": "Hook event scope: scan covers every event listed in `hydrationTriggerEvents` (currently `client-attached` and `client-session-changed` per `internal/tmux/hooks_register.go:25-28`). If the slice is later extended, the migration scan must follow it."

**Resolution**: Approved
**Notes**: Applied as part of Finding 1's combined block.

---

### 3. Existing dedupe substring not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 1 → "One-shot bootstrap migration"

**Details**:
State the current dedupe substring (`signalHydrateSubstring` at `internal/tmux/hooks_register.go:48`) so the change to `"portal state signal-hydrate --"` is unambiguous.

**Proposed Addition**:
Inline anchor: "the dedupe substring used to detect whether a hook is already present (currently `signalHydrateSubstring = "portal state signal-hydrate"` at `internal/tmux/hooks_register.go:48`) must be tightened to `"portal state signal-hydrate --"`."

**Resolution**: Approved
**Notes**: Applied inline in the existing migration paragraph.

---

### 4. Acceptance criterion 3 — "subsequent bootstraps are no-ops" not observable

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → item 3

**Details**:
Define the no-op invariant in observable terms: count of `portal state signal-hydrate` hook entries per event is exactly 1 after bootstrap and unchanged across two consecutive bootstraps.

**Proposed Addition**:
Rewrite AC 3 to include the runtime invariant and note that the migration test (TR 4) asserts it directly.

**Resolution**: Approved
**Notes**: Applied — AC 3 now defines the invariant in observable terms and binds it to TR 4.

---

### 5. Acceptance criterion 2 — manual vs. automated verification ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → item 2

**Details**:
Clarify that AC 2 is satisfied by passing the cobra-level argv parse test in TR 1; no separate manual repro artefact is required for sign-off.

**Proposed Addition**:
Rewrite AC 2 to bind it to TR 1 and remove the "manual verification" framing.

**Resolution**: Approved
**Notes**: Applied — AC 2 now references TR 1 explicitly and drops the manual-repro phrasing.

---

### 6. Migration test setup mechanism unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements → item 4

**Details**:
State the intended approach for migration test setup: real-tmux fixture vs. mocked Commander.

**Proposed Addition**:
Prefer a real-tmux socket fixture (`internal/tmuxtest`) — eviction logic depends on `show-hooks` output format and `set-hook -gu` index semantics, both of which a mock would have to re-implement.

**Resolution**: Approved
**Notes**: Applied as a "Test setup" clause on TR 4.

---

### 7. Operator visibility of migration not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 1 → "One-shot bootstrap migration"

**Details**:
Decide whether migration emits a `portal.log` line and at what level.

**Proposed Addition**:
Single INFO line on non-empty migration: `INFO | bootstrap | evicted N stale signal-hydrate hook(s) lacking '--' separator`. Silent on steady-state.

**Resolution**: Approved
**Notes**: Applied as part of Finding 1's combined block, plus a closing sentence on the migration paragraph.

---

### 8. Deletion list — unverified-consumer instruction missing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 2

**Details**:
Instruct the planner to verify each symbol has zero remaining references before removal; surface unexpected references for review.

**Proposed Addition**:
"Pre-deletion verification" paragraph appended to Part 2's deletion list.

**Resolution**: Approved
**Notes**: Applied as a new paragraph immediately after the deletion bullet list.

---

### 9. Reboot round-trip test — execution environment not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements → item 2

**Details**:
Mandate real-tmux fixture (`internal/tmuxtest`) for the reboot round-trip test — a mock-based shape would not exercise tmux's `run-shell` argv resolution.

**Proposed Addition**:
Insert mandate into TR 2 explicitly.

**Resolution**: Approved
**Notes**: Applied — TR 2 now requires a real-tmux fixture and explains why.

---
