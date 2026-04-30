---
status: complete
created: 2026-04-30
cycle: 1
phase: Gap Analysis
topic: hidden-sessions-showing-on-startup
---

# Review Tracking: hidden-sessions-showing-on-startup - Gap Analysis

## Findings

### 1. Fix A strategy choice left to implementer with no decision criteria

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Interaction With The Capture Path

**Details**:
Spec offered two equivalent strategies but deferred the choice to the implementer. Strategy 1 required a new lower-level method that the spec did not name or describe.

**Proposed Addition**:
Pinned to Strategy 2 (filter only in `ListSessions`; capture path's existing `keepSessionNames` double-filters as a no-op). Rejected Strategy 1 in the spec rather than deferring to implementer.

**Resolution**: Approved
**Notes**: Auto-resolved with smaller-change rationale.

---

### 2. `exit-empty` setting is unspecified — Fix B's lifecycle claims rely on it

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix B — Lifecycle After The Rename; Root Cause 2

**Details**:
Spec invoked tmux's `exit-empty on` to justify Fix B but never stated whether Portal sets it.

**Proposed Addition**:
Added an opening paragraph to "Lifecycle After The Rename" stating that Portal does not set or modify `exit-empty`, that reaping is opportunistic, and that Fix A's filter hides `_portal-bootstrap` regardless of the user's `exit-empty` configuration.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 3. `PortalSaverName` doc-comment cleanup — mandatory or optional?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Doc-Comment Cleanup — `tmux.PortalSaverName`

**Details**:
Internal inconsistency between "MUST be updated" opener and "may be tightened but its substance stands" sub-section.

**Proposed Addition**:
Re-wrote the `PortalSaverName` directive to require active review against post-fix code, with a deliberate edit OR an explicit "reviewed, no change required" commit-message acknowledgement.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 4. Pre-existing `0` session from prior installs not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Out Of Scope; Rollout

**Details**:
Users upgrading on a running tmux server keep the legacy `0` session because `StartServer` does not run.

**Proposed Addition**:
Added "Cleanup Of Pre-Existing `0` Sessions On Upgrade" subsection to Out Of Scope. Documented why auto-cleanup is unsafe (cannot distinguish leftover from user-owned) and required release-note guidance instructing users to restart their tmux server once after upgrade.

**Resolution**: Approved
**Notes**: Auto-approved with safety reasoning.

---

### 5. `cmd/list.go` empty-output behaviour unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Behaviour Contract

**Details**:
Spec did not specify what `portal list` should print when the filtered slice is empty.

**Proposed Addition**:
Added "Empty-List Behaviour" subsection to Fix A. `portal list` prints nothing on empty input — preserves existing behaviour, no message added.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 6. TUI behaviour when `filteredSessions` becomes empty

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Behaviour Contract

**Details**:
Same scenario as #5 in the TUI — picker may face empty list for the first time.

**Proposed Addition**:
Folded into "Empty-List Behaviour": verify existing rendering is acceptable; explicitly out-of-scope to add an empty-state UX in this bugfix.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 7. No regression guard for future unnamed `tmux new-session` callers

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix B — Sole Production Caller Verified

**Details**:
Spec warned future contributors but mandated no automated check.

**Proposed Addition**:
Added "Enforcement posture: treated as a code-review concern, not a mandated automated check" with explicit reasoning (e2e test still catches non-`_*` leaks via UX).

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 8. End-to-end test placement is left open

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Requirements — End-To-End — No `_*` Sessions Visible Post-Bootstrap

**Details**:
Spec said "either a bootstrap-level test or `reboot_roundtrip_test.go`".

**Proposed Addition**:
Pinned to `cmd/bootstrap/reboot_roundtrip_test.go` (real-tmux fixture path). Real fixture required because the assertion is a tmux-level invariant.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 9. `Client.ListSessions` filter — empty-result semantics not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Filter Definition

**Details**:
Empty post-filter slice — nil or empty? JSON marshalling differs.

**Proposed Addition**:
Added "Return-Value Contract" subsection to Fix A: empty (non-nil) slice. JSON serialises to `[]`, not `null`. Implementation MUST NOT return `nil`.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 10. Filter ordering relative to existing post-processing in `ListSessions`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Behaviour Contract; Interaction With The Capture Path

**Details**:
Spec said "post-processing layer" without pinning where in the chain.

**Proposed Addition**:
Added "Filter Application Order" subsection: filter runs as the final step before return, after parsing/sorting/enrichment. Contract preserved as the pipeline evolves.

**Resolution**: Approved
**Notes**: Auto-approved.

---
