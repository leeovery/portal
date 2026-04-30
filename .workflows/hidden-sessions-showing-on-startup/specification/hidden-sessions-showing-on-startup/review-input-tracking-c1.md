---
status: in-progress
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
**Affects**: Fix B — Rename Bootstrap Session To `_portal-bootstrap` (specifically the "Why Rename Instead Of Kill" or naming rationale subsection)

**Details**:
Investigation notes that `internal/restore/integration_test.go:280` and `cmd/bootstrap/reboot_roundtrip_test.go:236, 319` already use `_seed` / `_bootstrap` (underscore-prefixed) names for the seeding bootstrap session in tests. This is direct precedent for the chosen convention — the test code already demonstrates the pattern works; production was the outlier. Strengthens the rename rationale and shows the convention is not novel.

**Current**:
The spec's "Why Rename Instead Of Kill" subsection lists three alternatives but does not cite the existing test-bench precedent that already follows the underscore-prefix convention.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 2. Contributing factor — two cleanup mechanisms colliding

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 234-237 ("Contributing Factors")
**Category**: Enhancement to existing topic
**Affects**: Root Cause 2 — Bootstrap `0` Session Never Cleaned Up (or "Why The Two Causes Surface Together")

**Details**:
Investigation frames a specific contributing factor: "Bootstrap step 4 creates `_portal-saver`, step 5 creates user sessions via Restore. The original `0` session has no role to play once steps 4-5 complete, but no step removes it." The spec describes the lack of cleanup but does not articulate that the bootstrap session is *functionally redundant* the moment steps 4-5 succeed — it's only the keep-alive anchor *until* something else exists. This framing matters for the rename rationale (it justifies why tmux's `exit-empty on` reaping is acceptable: the session has no purpose once real sessions exist).

**Current**:
Root Cause 2 explains the deliberate change in commit `bd659a3` and notes "Bootstrap step 5 (`Restore`) does not target the `0` session, and no other bootstrap step or cleanup mechanism removes it. The session lingers indefinitely." It does not call out the redundancy-after-step-4-or-5 framing.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 3. Blast radius — tooling that scripts against `portal list`

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 263-264 ("Blast Radius — Potentially affected")
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Scope, or Fix A → Behaviour Contract

**Details**:
Investigation flags a potential affected surface beyond the visible UX: "Any tooling that scripts against `portal list` and assumes only user sessions appear." This is a behavioural contract implication — scripts written today against `portal list` may currently see `_portal-saver` / `0` and either tolerate or fail on them; after the fix, output is strictly trimmed. The change is benign but unannounced. Spec's "Scope" section limits impact to "cosmetic / UX clutter" and does not acknowledge the scripted-consumer angle.

**Current**:
"Severity: low (cosmetic / UX clutter, no data loss)." in Scope. No mention of script consumers of `portal list`.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 4. `ListSessionNames` is a thin wrapper — investigation asserts as fact, spec hedges

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 386-388 ("Risk Assessment")
**Category**: Enhancement to existing topic
**Affects**: Fix A → Interaction With The Capture Path

**Details**:
Investigation states factually: "The capture path uses `ListSessionNames` (a thin wrapper around `ListSessions`) — adding an underscore filter to `ListSessions` would also filter the capture caller, but `internal/state` already applies its own `keepSessionNames` filter on top, so the result is unchanged." The spec's "Interaction With The Capture Path" hedges with "If `ListSessionNames` is implemented as a thin wrapper around `ListSessions` and the new filter would change its output…" — implementation/planning needs to know the investigation already verified the wrapper relationship and the double-filter equivalence. Hedging here may produce unnecessary planning work.

**Current**:
> If `ListSessionNames` is implemented as a thin wrapper around `ListSessions` and the new filter would change its output, the implementation MUST preserve current behaviour for the capture path. Two acceptable implementations:
> 1. Apply the underscore filter at the post-processing layer in `ListSessions`, and have `ListSessionNames` call the lower-level raw enumeration directly (bypassing the filter), OR
> 2. Apply the filter only in `ListSessions` because the capture path already filters `_*` sessions on top via `keepSessionNames` — double-filtering produces the same result.
>
> The implementation chooses one and documents which.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 5. "Why It Wasn't Caught" — review-process gap, not just test-surface gap

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 248-250 ("Why It Wasn't Caught")
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Why It Wasn't Caught Earlier

**Details**:
Investigation includes a third point under "Why It Wasn't Caught": "Manual QA during the resurrection feature would have caught both, but the planning's 'review' phase scored against the explicit task list, not against an end-to-end UX walk-through." The spec captures the two test-surface bullets but drops this one. It matters because the proposed end-to-end test (Test Requirements → "End-To-End — No `_*` Sessions Visible Post-Bootstrap") is precisely the regression guard for this review-process gap — naming it explicitly closes the loop.

**Current**:
The spec's "Why It Wasn't Caught Earlier" lists only two reasons (state-capture-test surface, StartServer-test surface). The review-phase / UX-walkthrough point is absent.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 6. Rollout / feature-flag posture

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 393-395 ("Risk Assessment — Recommended approach")
**Category**: New topic
**Affects**: Could be a new "Rollout" subsection or appended to Test Requirements / Out Of Scope

**Details**:
Investigation explicitly recommends: "Regular bugfix. Two small targeted commits (filter + rename), each with its own test. No feature flag needed — the change is observable but strictly improves UX." The spec is silent on rollout shape, commit shape, and feature-flag stance. For a planning agent, knowing "two commits, no flag" prevents speculative complexity (e.g. gating the rename behind an env var, or shipping A and B in separate releases).

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---

### 7. `bd659a3` is the only unnamed `new-session` call in production

**Source**: investigation/hidden-sessions-showing-on-startup.md line 342
**Category**: Enhancement to existing topic
**Affects**: Fix B → Behaviour Contract or Lifecycle After The Rename

**Details**:
Investigation notes "It is the only unnamed `new-session` call in the production codebase." This single fact verifies the safety of the rename: there is no other code path that would still produce a `0` session after Fix B lands. Spec does not carry this verification. Worth recording so future contributors don't re-introduce a sibling unnamed `new-session` and re-create the bug.

**Current**:
Fix B's Behaviour Contract describes the change to `StartServer` but does not assert that `StartServer` is the sole production caller of unnamed `new-session`.

**Proposed Addition**:


**Resolution**: Pending
**Notes**:

---
