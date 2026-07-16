---
status: complete
created: 2026-07-16
cycle: 3
phase: Input Review
topic: Ghostty Spawn Zero Windows
---

# Review Tracking: Ghostty Spawn Zero Windows - Input Review

## Findings

### 1. Fix 4's osacompile "assumption to confirm" understates the investigation's already-demonstrated osacompile reproduction

**Source**: Investigation §Code Trace, lines 96-101 ("The diagnosing session (transcript `7d88b6b8`) reproduced this via `osacompile` against the installed Ghostty 1.3.1, exact output: … (-2741) exit=1"); also §Symptoms line 22 ("reproduced locally per discovery")

**Category**: Enhancement to existing topic
**Affects**: Fix 4 (Prevention) — Compile-check regression guard, specifically the "Assumption to confirm (during live-Mac implementation)" note (spec lines 120-121)

**Details**:
Fix 4's central mechanism is compiling the emitted script through `osacompile` and asserting a zero exit, which requires that `osacompile` resolves the `tell application "Ghostty"` terminology against the installed dictionary. The spec frames this as an open "assumption to confirm" and warns it "must not be treated as settled" because it is "the same class of scripting-tool assumption that caused the original bug."

However, the investigation already provides direct prior evidence for exactly this mechanism: the diagnosing session ran `osacompile` against installed Ghostty 1.3.1 and got the `-2741` terminology-level parse error (it reached line 2 and errored on the undefined `with properties` parameter, having already resolved the `tell application "Ghostty"` block against the app's dictionary). This is the same `osacompile` path Fix 4 proposes, already demonstrated to load/parse against the installed Ghostty terminology and to produce the exact error the guard is designed to catch.

The spec's Fix 4 never references that the observed `-2741` was produced via `osacompile` (the very tool the guard uses) — it says only "the observed `-2741` compile error." Surfacing the investigation's prior `osacompile` demonstration would partially de-risk the assumption note (the terminology-resolution half is already evidenced) while preserving the genuinely-unsettled part: whether Ghostty must be *running* (the reproduction was likely done inside a running Ghostty, so the running-vs-not-running question the spec raises remains open and correctly flagged for live confirmation).

**Current**:
> **Assumption to confirm (during live-Mac implementation).** The guard assumes `osacompile` resolves the `tell application "Ghostty"` terminology from the installed dictionary **without requiring Ghostty to be running and without launching it** (no window, no side effect). This is the same class of scripting-tool assumption that caused the original bug, so it must **not** be treated as settled: because the test is macOS+Ghostty-gated it is authored and first run on a live Mac, where the behaviour is confirmed directly. …

**Proposed Addition**:
Rewrote the "Assumption to confirm" note to credit the investigation's already-demonstrated `osacompile` reproduction (terminology resolution + no-window property evidenced, not assumed), narrow the genuinely-open question to Ghostty's *running* state, and drop the overstated "same class of assumption that caused the original bug" framing. Fallbacks (require/ensure running, or `t.Skip` on unresolved terminology) preserved.

**Resolution**: Approved
**Notes**: Verified against investigation lines 96-101 — osacompile against installed Ghostty 1.3.1 produced the `-2741` terminology error, so the resolution/no-window half is evidenced; only running-state remains for live confirmation.

---
