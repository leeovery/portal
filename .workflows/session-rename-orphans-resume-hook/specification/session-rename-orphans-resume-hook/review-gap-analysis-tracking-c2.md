---
status: complete
created: 2026-07-01
cycle: 2
phase: Gap Analysis
topic: session-rename-orphans-resume-hook
---

# Review Tracking: session-rename-orphans-resume-hook - Gap Analysis

## Findings

### 1. `CreateFromDir` token generation source and failure branch left unspecified (asymmetric with QuickStart)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Overview → "Where it is stamped" (CreateFromDir bullet); cross-references Generation contract

**Details**:
The spec gives the QuickStart stamp path a complete generation story: the `<token>` is "generated in Go inside `Run` (via the injected id generator) **before** `ExecArgs` is assembled," and it states the failure branch explicitly — "A generation failure omits the `set-option` step (session still created, un-stamped → name fallback)."

The `CreateFromDir` bullet has no equivalent. It says only: "a best-effort `SetSessionOption(name, PortalIDOption, <token>)` immediately after `NewSession`." It never states (a) where `<token>` comes from — the same injected `IDGenerator` (`sc.gen`) that `PrepareSession` already calls once for the session name, or a distinct call — nor (b) what happens if that generation call errors.

This matters for implementation because the ordering differs from QuickStart. In `CreateFromDir` the session is *already created* (`NewSession` at create.go:86 succeeded) before the stamp runs (create.go:96). If the id-generation call errors at that point, an implementer must decide between two behaviours the spec does not disambiguate:
- swallow the generation error and omit the stamp (session survives un-stamped → name fallback), matching QuickStart's stated behaviour and the "best-effort stamping" risk entry; or
- return the error from `CreateFromDir` (aborting a session that already exists in tmux — an orphaned, un-returned live session).

Left as-is, an implementer would guess. The QuickStart path proves the intended answer is "swallow and omit," but because `CreateFromDir` runs the stamp *after* a successful create, the spec should say so explicitly for this path too — the token origin (reuse `sc.gen`) and the swallow-on-generation-error rule. Cycle 1's "token generation contract" and "QuickStart token origin" findings established the QuickStart branch and the fire-and-forget uniqueness semantics; neither closed the `CreateFromDir` generation-and-failure branch.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Approved
**Notes**: Approved via auto mode. CreateFromDir bullet now states the token is generated via the injected id generator (sc.gen) before the stamp, and both generation and SetSessionOption errors are swallowed (session survives un-stamped -> name fallback), consistent with QuickStart and @portal-dir; never aborts creation.

---
