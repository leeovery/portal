---
status: in-progress
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
The spec says "Extend `captureFormat` with a session-scoped `#{@portal-id}` field and populate `Session.PortalID` from it." The actual `captureFormat` is a fixed-arity `|||`-delimited string paired with an explicit `const captureFieldCount = 10` that the row parser length-validates against. Adding a field is not free-form: `captureFieldCount` must bump to 11 and the parser's field-index reads/validation must be updated in lockstep, or the parse rejects/mis-slots every row. This is the concrete, load-bearing edit implied by "extend captureFormat" and is easy to miss because the field-count constant lives a few lines from the format string. The spec correctly notes the token is alphanumeric so it can't contain `|||` (delimiter-safe), but does not mention the arity constant. Worth naming so the parser change isn't overlooked during planning/build.

**Current**:
"**2. Capture (`internal/state/capture.go`).** Extend `captureFormat` with a session-scoped `#{@portal-id}` field and populate `Session.PortalID` from it. `#{@portal-id}` resolves per-pane to the owning session's option value, so it is present on every pane row for that session; the parser takes it when assembling the session. A legacy/un-stamped session captures `PortalID == ""`. (The opaque token is alphanumeric, so it cannot contain the `|||` field delimiter.)"

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 2. In-source `PaneTarget` "canonical hook-key formatter" invariants become stale after the fix

**Source**: `internal/tmux/tmux.go:551-558` (PaneTarget doc: "the hook-key format stays stable across releases — changing it would silently invalidate every entry in hooks.json") and `internal/tmux/tmux.go:572-573` (PaneTargetExact doc: "PaneTarget (no prefix) remains the canonical hook-key formatter; do not mix the two — hook lookups against an `=`-prefixed key would miss"). Also investigation "Fix constraint — hook-key format is load-bearing across releases" (lines 329-333).
**Category**: Enhancement to existing topic
**Affects**: §"Hook-Key Derivation" → "Decoupling from `tmux.PaneTarget`"

**Details**:
The fix moves hook-key derivation off `PaneTarget` onto new `HookKey` / `HookKeyFormat` primitives. But `PaneTarget` currently carries two in-source doc-comment invariants that explicitly assert it *is* the canonical hooks.json key formatter and that its format must never change to avoid orphaning hooks.json. After this fix those comments are contradictory/misleading — the canonical hook-key formatter is now `HookKey`, and `PaneTarget`'s output is no longer a hooks.json key at all. The spec's "Decoupling" paragraph says PaneTarget "stays exactly as-is" as the name-based `-t` target formatter, which is true for behaviour, but leaves these two source comments stating the opposite of the new reality. An implementer following the spec's "stays as-is" wording may leave the now-false comments in place, re-establishing exactly the drift-risk the fix is trying to eliminate (a future caller trusting the comment would build a name-based hook key). Flagging so the doc-comment update at both sites is an explicit deliverable, and so the load-bearing-format invariant is understood as transferring to `HookKey`/`HookKeyFormat`, not disappearing.

**Current**:
"**Decoupling from `tmux.PaneTarget`.** `PaneTarget` stays exactly as-is — it remains the canonical, name-based `-t` *target* formatter, still used to address live panes (e.g. `respawn-pane`, `select-pane`). The hook key becomes a **separate concern** with its own formatter, so the change touches only hook identity, not tmux targeting."

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 3. "Process restart self-heals" is stated as fact but is an observed, not code-verified, assumption

**Source**: Investigation "Notes" (lines 322-328: "Assumption (not code-verified): the 'claude-restart self-heals' behaviour rests on Claude Code's SessionStart hook (outside this repo) re-running `portal hooks set` on restart. The re-registration code path is verified; the external trigger firing is observed, not provable from the codebase.") and Code Trace lines 173-175.
**Category**: Gap/Ambiguity
**Affects**: §"Problem Statement" (the "the hook self-heals" sentence)

**Details**:
The spec's Problem Statement asserts as fact: "If the process restarts (e.g. the external tool's own start-hook re-runs `portal hooks set` under the new name), the hook self-heals." The investigation deliberately qualifies this: only the re-registration *code path* is verified; the external trigger actually firing on restart is *observed, not provable from the codebase*. The self-heal is not something this fix builds, so this does not change the deliverable — but the spec presents an out-of-repo behavioural dependency with more certainty than the source supports. Since the whole "why the bug hid" narrative rests on this claim, a one-clause hedge (aligning with the investigation) keeps the spec honest about which parts are guaranteed by Portal vs. dependent on external tooling. Low priority; may be judged an acceptable simplification.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---
