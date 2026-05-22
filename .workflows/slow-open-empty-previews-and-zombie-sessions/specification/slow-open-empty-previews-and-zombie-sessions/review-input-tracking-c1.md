---
status: complete
created: 2026-05-22
cycle: 1
phase: Input Review
topic: slow-open-empty-previews-and-zombie-sessions
---

# Review Tracking: slow-open-empty-previews-and-zombie-sessions - Input Review

## Findings

### 1. Pre-v0.5.6 zombie-session behaviour contrasted with current

**Source**: Investigation, Symptoms / Problem Description (line 16): "Pre-v0.5.6, they would briefly reappear within a 'tick window' then disappear after ~5 s; now they never disappear."
**Category**: Enhancement to existing topic
**Affects**: Problem Statement (Symptom 3)

**Details**: Regression-shape change at v0.5.6 not captured in spec.

**Resolution**: Approved
**Notes**: Appended pre-v0.5.6 contrast sentence to Symptom 3.

---

### 2. `daemon.version` mismatch observation not preserved

**Source**: Investigation, Reporter's local diagnostic observations (line 62): "`daemon.version` file content was `0.5.5`."
**Category**: Enhancement to existing topic
**Affects**: End-State Verification

**Details**: `daemon.version=0.5.5` after a 0.5.6 upgrade evidences `EnsurePortalSaverVersion` not running cleanly.

**Resolution**: Approved
**Notes**: Added End-State Verification bullet asserting `daemon.version` matches running binary post-fix.

---

### 3. Regression window framing within v0.5.x line

**Source**: Investigation, Constraints & Confirmed Context (line 74).
**Category**: Enhancement to existing topic
**Affects**: Root Cause (latency / regression attribution)

**Details**: Spec did not record the within-v0.5.x regression framing.

**Resolution**: Approved
**Notes**: Added Regression-point note paragraph to Root Cause — treats the regression-point framing as inconclusive given the multi-defect / ambient-trigger nature of the bug.

---

### 4. Hydrate WARN noise (`scrollback file not found`) as a downstream symptom

**Source**: Investigation, Symptoms / Manifestation block (line 25).
**Category**: Enhancement to existing topic
**Affects**: End-State Verification (quiet-log assertion)

**Resolution**: Approved
**Notes**: Extended the daemon-log quiet-log bullet to include the hydrate-side `scrollback file not found` warnings.

---

### 5. Component C's lock-file open mode (`O_EXCL|O_CREAT`) divergence from investigation

**Source**: Investigation, Options Explored — C (line 303).
**Category**: Gap/Ambiguity
**Affects**: Component C

**Resolution**: Approved
**Notes**: Added "Deviation from investigation" paragraph to Component C explaining why O_EXCL|O_CREAT was rejected in favour of the daemon.pid pre-check.

---

### 6. Investigation's ruled-out items not preserved in spec

**Source**: Investigation, Dead Ends / Ruled Out (lines 224-227).
**Category**: Enhancement to existing topic
**Affects**: Root Cause

**Resolution**: Approved
**Notes**: Added "Ruled Out (preserved for future reference)" subsection under Root Cause covering the TOCTOU-on-A, merge-filter regression, and ctx-cancellable-fix dead ends.

---

### 7. Upgrade-path inode-replacement as a landmine

**Source**: Investigation, Blast Radius / Potentially affected (line 255).
**Category**: Enhancement to existing topic
**Affects**: Component C acceptance criteria

**Resolution**: Approved
**Notes**: Added an upgrade-path acceptance scenario to Component C — v(N) daemon still alive when v(N+1) bootstraps.

---

### 8. Empirical measurement of N for Component D before tuning

**Source**: Investigation, Risk Assessment (line 339).
**Category**: Enhancement to existing topic
**Affects**: Risk Summary

**Resolution**: Approved
**Notes**: Promoted measurement from conditional to mandatory in Risk Summary; specified what to measure (steady-state, attach/detach, client-attached, bootstrap kill-and-recreate) and the safety factor (2×).

---

### 9. `portal clean` interaction with orphan sweep

**Source**: Investigation, Blast Radius / Directly + Potentially affected.
**Category**: Gap/Ambiguity
**Affects**: Component B scope

**Resolution**: Approved
**Notes**: Added explicit scope clarification to Component B: `portal clean` is out of scope, bootstrap-only sweep is sufficient, with rationale and the manual escape hatch referenced.

---

### 10. TUI preview adapter as the read-side of the symptom

**Source**: Investigation, Key Files (line 221).
**Category**: Enhancement to existing topic
**Affects**: Root Cause symptom mapping

**Resolution**: Approved
**Notes**: Extended the symptom-2 mapping bullet to name `internal/tui/preview_adapter.go` and `state.ScrollbackFile(stateDir, paneKey)` as the read site.

---

### 11. Component F doom-loop framing (recovery doom-loop)

**Source**: Investigation, Options Explored — F (line 309).
**Category**: Enhancement to existing topic
**Affects**: Component F Goal

**Resolution**: Skipped
**Notes**: Withdrawn — Component F's existing Goal paragraph already captures the doom-loop dynamic adequately. No spec change required.

---
