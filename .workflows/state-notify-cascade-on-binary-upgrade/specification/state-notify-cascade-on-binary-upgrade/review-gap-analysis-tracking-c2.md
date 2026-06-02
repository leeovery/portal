---
status: complete
created: 2026-06-02
cycle: 2
phase: Gap Analysis
topic: state-notify-cascade-on-binary-upgrade
---

# Review Tracking: state-notify-cascade-on-binary-upgrade - Gap Analysis

## Findings

### 1. "verbatim, no-trim" contract claim contradicts the actual `ShowGlobalHooks` implementation it replaces

**Source**: Specification analysis (Solution Strategy → "Concrete mechanism")
**Category**: Gap/Ambiguity
**Affects**: § Solution Strategy → Concrete mechanism (the `ShowGlobalHooksForEvent` / "Delete `ShowGlobalHooks`" bullets)

**Priority**: Minor

**Details**:
The spec states that the new seam "preserves the removed method's contract — **verbatim, no-trim** output and the `failed to show global hooks: %w` error-wrap shape — which is what lets `ParseShowHooks` stay unchanged."

The error-wrap half is accurate. The "verbatim, no-trim" half is not: the current `Client.ShowGlobalHooks` (internal/tmux/tmux.go:769) is implemented with `c.cmd.Run("show-hooks", "-g")`, and `RealCommander.Run` is the **trimming** path (`runCommand(..., trim=true, ...)`, tmux.go:58). Only `RunRaw` is verbatim. So the method being replaced actually trims its output; its own godoc ("returned verbatim (no trimming)") is likewise inaccurate.

This matters two ways for an implementer:
- It is a factual contradiction between the spec's stated current-contract and the code being modified. An implementer told to "preserve the verbatim contract" who then reads the source (`Run`, not `RunRaw`) has to reconcile the conflict and guess intent.
- It risks a needless behavioural change: implementing `ShowGlobalHooksForEvent` with `RunRaw` "to match the spec" would diverge from the trimming behaviour the old method actually had.

The reassuring fact — which keeps this Minor rather than Important — is that `ParseShowHooks` trims every line itself (hooks_parse.go:46) and strips only matched outer quotes, so trim-vs-no-trim at the seam is **immaterial to correctness**: the parsed `Command` field, and therefore the idempotent fast-path full-body equality check, is identical under either. The spec's conclusion ("`ParseShowHooks` stays unchanged") holds regardless. The defect is purely in the descriptive claim, not in the outcome.

Suggested resolution: either (a) implement `ShowGlobalHooksForEvent` with the trimming `Run` to genuinely preserve the existing contract and reword the bullet to say "trimmed, matching the existing `ShowGlobalHooks`", or (b) keep `RunRaw` but reword to "trim behaviour is immaterial because `ParseShowHooks` trims per line." Pin one so the implementer is not left choosing.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Auto-approved. Corrected the 'Delete ShowGlobalHooks' bullet: the new seam preserves the same trimming `Run` path + `failed to show global hooks: %w` wrap (not 'verbatim, no-trim'); trim is immaterial since `ParseShowHooks` trims per line.

---
