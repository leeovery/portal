---
status: complete
created: 2026-07-01
cycle: 1
phase: Input Review
topic: Session Rename Orphans Resume Hook
---

# Review Tracking: Session Rename Orphans Resume Hook - Input Review

## Findings

### 1. `captureFieldCount` bump and parser length-validation not stated

**Source**: `internal/state/capture.go:34-36` (`captureFormat` + `const captureFieldCount = 10`), cross-referenced against spec §"Cross-Reboot Persistence of `@portal-id`" step 2 (Capture)
**Category**: Enhancement to existing topic
**Affects**: §"Cross-Reboot Persistence of `@portal-id`" → "2. Capture (`internal/state/capture.go`)"

**Details**:
The spec says "Extend `captureFormat` with a session-scoped `#{@portal-id}` field and populate `Session.PortalID` from it." The actual `captureFormat` is a fixed-arity `|||`-delimited string paired with an explicit `const captureFieldCount = 10` that the row parser length-validates against. Adding a field is not free-form: `captureFieldCount` must bump to 11 and the parser's field-index reads/validation must be updated in lockstep, or the parse rejects/mis-slots every row.

**Current**:
"**2. Capture (`internal/state/capture.go`).** Extend `captureFormat` with a session-scoped `#{@portal-id}` field and populate `Session.PortalID` from it. ..."

**Proposed Addition**:
"`captureFormat` is **fixed-arity**: it is paired with `const captureFieldCount` (currently `10`) that the row parser length-validates against. Adding the field is not free-form — `captureFieldCount` must bump to `11` and the parser's field-index reads must update in lockstep, or every captured row is rejected/mis-slotted."

**Resolution**: Approved
**Notes**: Approved via auto mode. Added as a new paragraph under Capture step 2.

---

### 2. In-source `PaneTarget` "canonical hook-key formatter" invariants become stale after the fix

**Source**: `internal/tmux/tmux.go:551-558` and `internal/tmux/tmux.go:572-573`. Also investigation "Fix constraint — hook-key format is load-bearing across releases" (lines 329-333).
**Category**: Enhancement to existing topic
**Affects**: §"Hook-Key Derivation" → "Decoupling from `tmux.PaneTarget`"

**Details**:
The fix moves hook-key derivation off `PaneTarget` onto new `HookKey` / `HookKeyFormat` primitives, but `PaneTarget`'s doc-comments still assert it *is* the canonical hooks.json key formatter and that its format must never change. After the fix those comments are contradictory/misleading; leaving them invites a future caller back into name-based keying. Comment update should be an explicit deliverable and the load-bearing-format invariant understood as transferring to `HookKey`/`HookKeyFormat`.

**Current**:
"**Decoupling from `tmux.PaneTarget`.** `PaneTarget` stays exactly as-is — it remains the canonical, name-based `-t` *target* formatter, still used to address live panes (e.g. `respawn-pane`, `select-pane`). The hook key becomes a **separate concern** with its own formatter, so the change touches only hook identity, not tmux targeting."

**Proposed Addition**:
"**Deliverable — retire the stale doc-comments.** `PaneTarget`/`PaneTargetExact` today carry in-source doc-comments (`tmux.go:551-558`, `572-573`) asserting `PaneTarget` *is* the canonical `hooks.json` key formatter and that its format must never change or it orphans `hooks.json`. After the fix those comments are false and must be updated: the canonical hook-key formatter is now `HookKey` / `HookKeyFormat`, and the load-bearing "format is stable across releases — changing it silently invalidates every `hooks.json` entry" invariant **transfers to those new primitives**, it does not disappear. Leaving the old comments in place would invite a future caller back into name-based keying — re-establishing the exact drift this fix removes."

**Resolution**: Approved
**Notes**: Approved via auto mode. Added as a new paragraph in the Decoupling section.

---

### 3. "Process restart self-heals" is stated as fact but is an observed, not code-verified, assumption

**Source**: Investigation "Notes" (lines 322-328) and Code Trace lines 173-175.
**Category**: Gap/Ambiguity
**Affects**: §"Problem Statement" (the "the hook self-heals" sentence)

**Details**:
The spec's Problem Statement asserts as fact that process restart self-heals. The investigation qualifies this: only the re-registration *code path* is verified; the external trigger firing is *observed, not provable from the codebase*. A one-clause hedge keeps the spec honest about Portal-guaranteed vs external-tooling-dependent behaviour.

**Current**:
"It bites **only when the inner pane process does not restart** across the rename. If the process restarts (e.g. the external tool's own start-hook re-runs `portal hooks set` under the new name), the hook self-heals — which is why the bug hid in everyday use."

**Proposed Addition**:
"(This self-heal depends on out-of-repo tooling actually re-running `portal hooks set` on restart: Portal's re-registration path is verified, but the external trigger firing is *observed*, not guaranteed by this codebase.)"

**Resolution**: Approved
**Notes**: Approved via auto mode. Appended as a hedge clause to the Problem Statement.

---
