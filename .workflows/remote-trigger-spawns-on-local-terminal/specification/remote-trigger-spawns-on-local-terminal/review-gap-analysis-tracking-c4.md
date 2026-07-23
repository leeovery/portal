---
status: complete
created: 2026-07-23
cycle: 4
phase: Gap Analysis
topic: remote-trigger-spawns-on-local-terminal
---

# Review Tracking: remote-trigger-spawns-on-local-terminal - Gap Analysis

## Findings

### 1. "Must stay green" invariant list omits the single-client walk-failure test (`:171`)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Testing Requirements → "Existing invariants that must stay green"

**Details**:
The test file `internal/spawn/detect_inside_test.go` contains ten subtests. The Testing Requirements section accounts for nine of them — the two transforms (`:133` invert, `:196` reframe) plus the seven "must stay green" invariants (`:46`, `:65`, `:83`, `:101`, `:117`, `:151`, `:220`). The subtest at `:171` — *"it returns a transient error when a walk fails and nothing local resolves"* (single client, its walk transient-fails) — is not referenced anywhere in the spec.

Under the fix this test still passes unchanged (single client → it is the winner → its walk transient-fails → NULL + `ErrDetectTransient`), so behaviour is preserved. But the spec presents the "must stay green" list as the authoritative pinned outcome set, and this test is genuinely in that set yet unlisted. Two concrete risks from the omission:
- The reframed `:196` becomes behaviourally near-identical to `:171` (both now assert NULL + `ErrDetectTransient`). An implementer who does not see `:171` enumerated as a must-keep invariant could mistake it for a now-redundant duplicate of the reframed `:196` and delete it, silently dropping the single-client walk-failure coverage.
- The implementer is told to "identify the tests by their subtest description," so an unlisted-but-present test is easy to overlook when reconciling the file against the spec.

**Proposed Addition**:
Added a `:171` single-client-walk-failure bullet to the 'must stay green' invariants, with an explicit 'do not delete as a duplicate of the reframed :196' warning.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 2. Docstring-rewrite scope under-specified — the most directly-inverted contract lines fall outside the cited "lines 56–59"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Owned Behaviour Change: Dropped Walk-Resilience Property (the docstring-rewrite bullet)

**Details**:
The "Owned Behaviour Change" section dedicates explicit attention to ensuring the `detect_inside.go` docstring is rewritten and not left stale ("Do not leave the old contract text in place describing behaviour the code no longer has"). It scopes that rewrite to "the current lines 56–59 describing the all-clients walk and the 'one bad `ps` cannot mask a resolvable local' guarantee."

However, the docstring statements that are *semantically inverted* by the fix sit largely outside lines 56–59:
- Line 55: "NULL-filtering is the primary signal." — false after the fix (activity is now the primary cross-client signal).
- Lines 69–72 (the "Outcomes" list), especially line 72: "client_activity is used ONLY to disambiguate among host-local clients — **never as a cross-client primary signal**." This line is the exact inversion of the new rule (activity selects the winner across all clients, local and remote alike) and is the single most contradictory sentence in the docstring.

An implementer who trusts the "lines 56–59" pointer literally would perform a narrow rewrite that leaves lines 55 and 69–72 intact — leaving a docstring that actively contradicts the code, which is precisely the outcome this section exists to prevent. The section's general directive covers it, but the specific line pointer under-scopes the rewrite and points away from the most important line to change.

**Proposed Addition**:
Broadened the docstring-rewrite scope to the whole contract (~lines 49–72) and called out the two directly-inverted sentences: line 55 ('NULL-filtering is the primary signal') and lines 70–72 ('client_activity ... never as a cross-client primary signal').

**Resolution**: Approved
**Notes**: Auto-approved.

---
