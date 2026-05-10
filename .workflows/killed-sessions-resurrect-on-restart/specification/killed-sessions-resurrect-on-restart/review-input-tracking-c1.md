---
status: complete
created: 2026-05-10
cycle: 1
phase: Input Review
topic: killed-sessions-resurrect-on-restart
---

# Review Tracking: killed-sessions-resurrect-on-restart - Input Review

## Findings

### 1. Duplicate `client-attached` ENOENT warnings — addressed-by-side-effect of Fix 1

**Source**: Investigation "Supporting Observations" (lines 57-58) and "Contributing Factors" (lines 192-193) and "Discussion" (line 252)
**Category**: Enhancement to existing topic
**Affects**: Fix 1 → Relationship to Existing Hook-Driven Signaling

**Resolution**: Approved
**Notes**: Added to Fix 1 → Relationship to Existing Hook-Driven Signaling section.

---

### 2. `CleanStaleMarkers` cannot address timeout-stuck markers on live panes

**Source**: Investigation "Symptom C" (lines 174-178)
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Observed Symptoms (Symptom C)

**Resolution**: Approved
**Notes**: Added to Symptom C bullet in Observed Symptoms.

---

### 3. "Why It Wasn't Caught" insights useful for test plan motivation

**Source**: Investigation "Why It Wasn't Caught" (lines 196-202)
**Category**: Enhancement to existing topic
**Affects**: Test Plan → Multi-session cold-start

**Resolution**: Approved
**Notes**: Added gap-closure framing to multi-session cold-start integration test entry.

---

### 4. Single-saved-session user is unaffected — useful blast-radius framing

**Source**: Investigation "Blast Radius → Not affected" (lines 215-217)
**Category**: Enhancement to existing topic
**Affects**: Scope Boundary

**Resolution**: Approved
**Notes**: Added to Scope Boundary as a follow-up paragraph.

---

### 5. Rejected wrapper-redesign options not captured in out-of-scope

**Source**: Investigation "Options Explored" (lines 244-245)
**Category**: New topic
**Affects**: Fix Scope → What is explicitly out of scope

**Resolution**: Approved
**Notes**: Added two new bullets to "What is explicitly out of scope".

---

### 6. Helper success-path also leaves parked `sh` parent

**Source**: Investigation "Defect D" (lines 181-185)
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Observed Symptoms (Defect D)

**Resolution**: Approved
**Notes**: Tightened Defect D wording to make explicit the parked parent appears on every hydration outcome.

---

### 7. Reproduction steps from the investigation not surfaced as a verification protocol

**Source**: Investigation "Reproduction Steps" (lines 33-42)
**Category**: Gap/Ambiguity
**Affects**: Risks & Rollout

**Resolution**: Approved
**Notes**: Added a new "Manual Verification Protocol" subsection with the 6-step reproduction recast as pre-fix/post-fix checks, plus Defect-D-specific post-fix checks.
