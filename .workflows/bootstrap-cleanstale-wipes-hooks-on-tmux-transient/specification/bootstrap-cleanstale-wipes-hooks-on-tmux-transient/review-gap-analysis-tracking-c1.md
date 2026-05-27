---
status: complete
created: 2026-05-27
cycle: 1
phase: Gap Analysis
topic: bootstrap-cleanstale-wipes-hooks-on-tmux-transient
---

# Review Tracking: bootstrap-cleanstale-wipes-hooks-on-tmux-transient - Gap Analysis

## Findings

### 1. Promoted-parser destination left as "likely"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 2

**Resolution**: Adjusted
**Notes**: Code grounding revealed that hook entries are keyed by *structural keys* (raw `session:window.pane` from `ResolveStructuralKey`), while `parseLivePaneSet` in `cmd/bootstrap` produces *canonical paneKeys* via `state.SanitizePaneKey` for markers. The two parsing concerns are distinct — no parser promotion is required for hook cleanup. Change 2 was rewritten to instruct reusing the existing `parsePaneOutput` helper inside the repurposed `ListAllPanes` and leaving `parseLivePaneSet` in place. This corrects an inaccuracy in the original fix-direction sketch from the investigation.

---

### 2. Logger parameter in repurposed `ListAllPanes` left as `nil` placeholder

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 1

**Resolution**: Adjusted
**Notes**: Resolved by Finding #1 — the repurposed `ListAllPanes` no longer calls `parseLivePaneSet`. The sketch in Change 1 now uses `parsePaneOutput` (loggerless, same-package helper), eliminating the placeholder.

---

### 3. Format string alignment between sketch and existing parser not asserted

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 1 and Change 2

**Resolution**: Approved
**Notes**: Added "Format-string alignment" subsection to Change 1 explicitly stating that the chosen format `"#{session_name}:#{window_index}.#{pane_index}"` matches `ResolveStructuralKey`'s output, which is how hook entries are keyed in `hooks.json`. No parser-format mismatch exists.

---

### 4. `persistedHooks` count source not specified at the adapter callsites

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3

**Resolution**: Approved
**Notes**: Added explicit instruction to Change 3 specifying `hookStore.Load()` (already-public method on `*hooks.Store`) as the count source. No new API is introduced on `internal/hooks/store.go`. Noted that `portal clean` already calls `Load()` at line 65, so the bootstrap adapter alone needs to add the call.

---

### 5. `cleanStaleAdapter.CleanStale` current signature not described

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3, Test Requirements

**Resolution**: Approved
**Notes**: Added "Current adapter shape" paragraph to Change 3 describing the existing struct (lines 66-69), the existing method (lines 76-83), and the seam choice (continue consuming `*tmux.Client` directly; tests may introduce a local `AllPaneLister` matching the existing shape in `cmd/clean.go:13-15`).

---

### 6. `portal clean` logger availability not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3

**Resolution**: Approved
**Notes**: Added "Logger plumbing (`portal clean`)" paragraph specifying `openNoRotateLogger()` as the acquisition mechanism, writes into the same `portal.log` as bootstrap, with the same nil-tolerance contract. User-facing stderr output from `portal clean` is unchanged.

---

### 7. Soft-warning surfacing path for `ListAllPanesWithFormat` errors underspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 3, Acceptance Criteria, Test Requirements

**Resolution**: Approved
**Notes**: Added "Soft-warning surfacing contract" paragraph to Change 3 stating that the orchestrator's step-runner is responsible for converting the non-nil return into a `warning.Warning` (same path as step-9), and that the adapter returns the error directly without wrapping. The `portal clean` callsite handles its own surfacing via `Warn` log + nil return.

---

### 8. Return-type narrowing of `ListAllPanes` (nil vs empty slice) may affect callers

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 1

**Resolution**: Approved
**Notes**: Added "Return-value contract change" paragraph to Change 1 stating the shift from `([]string{}, nil)` to `(nil, err)` on error, confirming both audited production consumers handle this safely, and instructing test stubs to adopt the new shape.

---

### 9. Hazard-guard return value's interaction with "Debug on completion" not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 4, Acceptance Criteria #4

**Resolution**: Approved
**Notes**: Rewrote Change 4 to make mutual exclusivity structural — every invocation emits exactly one entry-point Debug line (after enumeration) and exactly one terminal line (Warn-on-error, Warn-on-guard, or Debug-on-completion). Tests assert the Debug-on-completion line is absent on the hazard-guard path. AC #4 updated to match.

---

### 10. "Same inversion shape used by Phase-4 subtests" reference is unanchored

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Requirements — Inverted Subtest, Notes

**Resolution**: Approved
**Notes**: Anchored to commit `7e33c04b` (`impl(hooks-skip-bootstrap): T1-2 — invert hooks list test, add hooks set test`) in both the Test Requirements section and the v0.5.11 Notes bullet, with a one-sentence description of the structural-preserve-flip-assert pattern.

---

### 11. `parseLivePaneSet` signature and whether it accepts a logger is implicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Specification → Change 2

**Resolution**: Adjusted
**Notes**: Resolved by Finding #1 — Change 2 no longer requires `parseLivePaneSet` movement. Its signature is therefore out of scope for this work unit.

---

### 12. Acceptance Criterion #4 wording leaves "Debug on entry" universality unclear

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria #4

**Resolution**: Approved
**Notes**: Renamed the log point from "Debug on entry" to "Debug after enumeration" throughout Change 4 and AC #4, with explicit positioning ("after the `ListAllPanes` + `Load` calls complete successfully and before the hazard-guard check"). Eliminates ambiguity about when `livePanes` is known.

---
