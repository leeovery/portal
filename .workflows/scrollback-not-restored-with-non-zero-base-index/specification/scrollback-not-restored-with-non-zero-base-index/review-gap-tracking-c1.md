---
status: in-progress
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
The spec mandates that `RegisterPortalHooks` "evict any hook entry whose command contains `portal state signal-hydrate` but does not contain the `--` separator before installing the fixed entry," but does not specify:

- Which tmux package API performs the eviction (the CLAUDE.md context mentions `UnsetGlobalHookAt` and `ShowGlobalHooks` exist — should the migration use these, or a new helper?).
- Whether eviction is by index, by command-substring match, or by some other addressing scheme.
- The exact ordering: scan-then-evict-then-install vs. evict-during-scan.

A planner would have to either read source to discover the existing hook-store API or guess the implementation shape. State the intended API entry points and ordering explicitly.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 2. Hook event scope for migration scan not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 1 → "One-shot bootstrap migration"

**Details**:
The Problem section refers to `client-attached` / `client-session-changed` as hook events, but the migration text only says "evict any hook entry whose command contains `portal state signal-hydrate`" — it does not enumerate which hook events the scan iterates over. If the broken hook was registered against multiple events, missing one would leave a broken entry behind. A planner needs an explicit list of hook events to inspect (or a clear statement that the scan covers all events `RegisterPortalHooks` writes to).

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 3. Existing dedupe substring not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 1 → "One-shot bootstrap migration"

**Details**:
The spec says the dedupe substring "must be tightened to `portal state signal-hydrate --`" but does not identify the current dedupe substring or where it lives in the codebase. Without that anchor, an implementer cannot confirm they are editing the right constant, and a reviewer cannot confirm the migration distinguishes correctly between the broken and fixed shapes. State the current substring (or file:line reference for it) so the change is unambiguous.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 4. Acceptance criterion 3 — "subsequent bootstraps are no-ops" not observable

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → item 3

**Details**:
"Subsequent bootstraps are no-ops" is a behavioural assertion but the spec does not define how this is verified. Options include: count of hook entries before/after equal, no eviction-log line emitted, no tmux mutation observed by a test fixture. Without a definition, the criterion is hard to test rigorously. Clarify whether this is meant as a runtime invariant (asserted via test fixture) or simply a design property.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 5. Acceptance criterion 2 — manual vs. automated verification ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria → item 2

**Details**:
Item 2 is labelled "Manual verification" but the Testing Requirements section also includes a cobra-level argv parse test (item 1) that appears to cover the same behaviour. It is unclear whether AC 2 must be exercised as a manual repro step (e.g. captured in the PR description) or whether passing the unit test in TR 1 satisfies it. Clarify so the implementer knows whether a manual repro artefact is required for sign-off.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 6. Migration test setup mechanism unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements → item 4

**Details**:
The migration test must verify eviction of a pre-existing broken hook, but the spec does not say how the test arranges that pre-existing state — via a real tmux fixture (`internal/tmuxtest`), a mocked `Commander`, or by directly seeding the hook store. Each approach has different fidelity vs. cost trade-offs. State the intended approach (or leave it explicit that the implementer chooses, with rationale documented in the PR).

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 7. Operator visibility of migration not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 1 → "One-shot bootstrap migration"

**Details**:
The migration silently rewrites a tmux hook on first bootstrap after upgrade. The spec does not say whether this should produce a log line in `portal.log` (e.g. INFO-level "evicted broken signal-hydrate hook") or be entirely silent. Logging the migration once would help operators correlate first-bootstrap behaviour after upgrade; silence avoids noise. State the intended behaviour.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 8. Deletion list — unverified-consumer instruction missing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope → Part 2

**Details**:
Part 2 lists symbols to delete and asserts e.g. "`flattenSavedPanePositions` — only consumer was `warnOnPaneKeyDrift`" and "`readIndexOption` (if unused after removal)". This is correct in spirit but a planner should be told to verify no other consumers exist before deletion (especially in tests, exported API surface, or future-staged code). A one-line instruction — "verify each symbol has zero remaining references before removal; if any are found, surface them for review" — would prevent silent breakage from a missed reference.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 9. Reboot round-trip test — execution environment not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements → item 2

**Details**:
TR 2 says to "extend `cmd/bootstrap/reboot_roundtrip_test.go` (or add a sibling integration test)" but does not state whether the new test must run against a real tmux server (via `internal/tmuxtest`) or can use mocks. The Testing Constraint section addresses socket isolation when a real server is used, but does not mandate which mode the new test uses. Given the bug is specifically about `run-shell` argv resolution by tmux, a real-tmux fixture seems essential for fidelity — state this explicitly so the test cannot regress to a mock-only shape that wouldn't catch the bug.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---
