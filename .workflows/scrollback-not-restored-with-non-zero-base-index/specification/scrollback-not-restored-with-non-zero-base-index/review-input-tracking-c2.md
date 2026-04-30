---
status: complete
created: 2026-04-30
cycle: 2
phase: Input Review
topic: scrollback-not-restored-with-non-zero-base-index
---

# Review Tracking: scrollback-not-restored-with-non-zero-base-index - Input Review

## Findings

### 1. Save-side is unaffected — bounds the fix scope

**Source**: investigation.md § "Manifestation" — "Save side is unaffected; per-pane scrollback files contain the expected ANSI-coloured terminal history."
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause § Observed Symptom

**Details**:
The investigation explicitly establishes that the save daemon and on-disk scrollback files are healthy — only the replay/hydrate path fails. Stating it bounds the fix and reinforces why AC 1 ("saved scrollback replayed") is verifiable.

**Proposed Addition**:
Insert sentence in Observed Symptom: "Save side is unaffected. Per-pane scrollback files on disk contain the expected ANSI-coloured terminal history; the save daemon reports success. The fix is scoped to the restore/hydrate path only — no save-side investigation is needed."

**Resolution**: Approved
**Notes**: Applied as a bolded sentence in the Observed Symptom paragraph, bounding the fix to restore/hydrate only.

---

### 2. Why the reporter's "removing base-index fixes it" observation appeared true

**Source**: investigation.md § "Why 'removing base-index makes it work' appears true (but isn't)"
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause § Observed Symptom

**Details**:
The spec asserts the bug report's base-index framing is incorrect but does not explain why the reporter's empirical observation seemed to corroborate it. Adding the explanation inoculates future readers against the same false signal.

**Proposed Addition**:
Append two sentences after the "framing is incorrect" sentence explaining that "removing base-index makes it work" tracks WARN-disappearance, not hydration-success.

**Resolution**: Approved
**Notes**: Applied — the framing-is-incorrect paragraph now also explains why the reporter's observation seemed to confirm the wrong cause.

---

### 3. Reproduction steps not captured

**Source**: investigation.md § "Reproduction Steps"
**Category**: Enhancement to existing topic
**Affects**: Problem & Root Cause § (new "Reproduction" sub-section)

**Details**:
Spec lacks a deterministic repro. Capturing the canonical 4-step repro and the "(a) leading-dot project basename + (b) non-zero base-index" decomposition disentangles the two confounding factors and helps test/manual verification.

**Proposed Addition**:
Add a new "Reproduction" subsection under Problem & Root Cause with the 4-step repro and a note on the independence of factors 1 and 2.

**Resolution**: Approved
**Notes**: Applied as a new "Reproduction" subsection; references the Testing Constraint when calling `kill-server`.

---
