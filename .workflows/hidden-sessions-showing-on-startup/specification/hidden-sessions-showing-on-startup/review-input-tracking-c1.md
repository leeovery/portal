---
status: complete
created: 2026-04-30
cycle: 1
phase: Input Review
topic: hidden-sessions-showing-on-startup
---

# Review Tracking: hidden-sessions-showing-on-startup - Input Review

## Findings

### 1. Test-bench precedent for underscore-prefixed bootstrap name

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 192-197 ("Test-bench hint")
**Category**: Enhancement to existing topic
**Affects**: Fix B — Rename Bootstrap Session To `_portal-bootstrap`

**Details**:
Investigation notes that `internal/restore/integration_test.go:280` and `cmd/bootstrap/reboot_roundtrip_test.go:236, 319` already use `_seed` / `_bootstrap` (underscore-prefixed) names for the seeding bootstrap session in tests. This is direct precedent — the test code already demonstrates the convention works; production was the outlier.

**Proposed Addition**:
Added new "Convention Precedent" subsection under Fix B citing both test paths and the convention parity argument.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 2. Contributing factor — two cleanup mechanisms colliding

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 234-237 ("Contributing Factors")
**Category**: Enhancement to existing topic
**Affects**: Root Cause 2 — Bootstrap `0` Session Never Cleaned Up

**Details**:
Investigation framing: the bootstrap session is functionally redundant the moment steps 4-5 succeed. Spec did not articulate the redundancy-after-step-4-or-5 framing.

**Proposed Addition**:
Appended a paragraph to Root Cause 2 making the redundancy-after-step-4-or-5 framing explicit and tying it to the rename rationale (why `exit-empty` reaping is acceptable).

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 3. Blast radius — tooling that scripts against `portal list`

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 263-264 ("Blast Radius — Potentially affected")
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Scope

**Details**:
Scripts written today against `portal list` may currently see `_portal-saver` / `0` and either tolerate or fail on them; after the fix, output is strictly trimmed.

**Proposed Addition**:
Appended a "Behavioural change beyond the visible UX" paragraph to Scope acknowledging the scripted-consumer angle.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 4. `ListSessionNames` is a thin wrapper — investigation asserts as fact, spec hedges

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 386-388 ("Risk Assessment")
**Category**: Enhancement to existing topic
**Affects**: Fix A → Interaction With The Capture Path

**Details**:
Investigation states factually that `ListSessionNames` is a thin wrapper around `ListSessions`. Spec hedged with "If `ListSessionNames` is implemented as a thin wrapper…". Hedge replaced with the verified fact.

**Proposed Addition**:
Rewrote the "Interaction With The Capture Path" subsection to assert the wrapper relationship as verified fact, retain the two equivalent strategies, and reference the capture-path regression guard.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 5. "Why It Wasn't Caught" — review-process gap, not just test-surface gap

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 248-250 ("Why It Wasn't Caught")
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Why It Wasn't Caught Earlier

**Details**:
Investigation includes a third point: planning's review phase scored against the explicit task list, not an end-to-end UX walk-through. Spec captured the two test-surface bullets but dropped this one.

**Proposed Addition**:
Added a third bullet to "Why It Wasn't Caught Earlier" naming the review-process gap and tying the end-to-end test mandate to closing it.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 6. Rollout / feature-flag posture

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 393-395 ("Risk Assessment — Recommended approach")
**Category**: New topic
**Affects**: New "Rollout" section

**Details**:
Investigation explicitly recommends two small targeted commits, no feature flag. Spec was silent on rollout shape.

**Proposed Addition**:
Added new "Rollout" section specifying no feature flag, two commits with their associated tests/cleanup, and that the end-to-end test ships in the same release.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 7. `bd659a3` is the only unnamed `new-session` call in production

**Source**: investigation/hidden-sessions-showing-on-startup.md line 342
**Category**: Enhancement to existing topic
**Affects**: Fix B → Behaviour Contract / Naming Constraint

**Details**:
Investigation verified `StartServer` is the sole production call site for unnamed `new-session`. Spec did not carry this verification.

**Proposed Addition**:
Added "Sole Production Caller Verified" subsection to Fix B documenting the verification and warning future contributors against re-introducing a sibling unnamed `new-session`.

**Resolution**: Approved
**Notes**: Auto-approved.

---
