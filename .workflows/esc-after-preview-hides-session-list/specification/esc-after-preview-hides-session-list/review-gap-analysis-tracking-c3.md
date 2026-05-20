---
status: in-progress
created: 2026-05-20
cycle: 3
phase: Gap Analysis
topic: esc-after-preview-hides-session-list
---

# Review Tracking: esc-after-preview-hides-session-list - Gap Analysis

## Findings

### 1. Prescribed `VisibleItems()` assertion will fail without draining the propagated `filterItems` cmd

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Coverage > "Lock in the fix at the wrong-axis miss site"; Test Coverage > "Cover the latent variant"; Acceptance Criteria #6

**Details**:
The fix mechanism is asynchronous: `SetItems` against a `FilterApplied` list synchronously nils `filteredItems` and returns a `filterItems` `tea.Cmd`. That cmd runs asynchronously, emits `FilterMatchesMsg`, and the bubbles list's own `Update` consumes the message to repopulate `filteredItems`. Only after that round-trip does `VisibleItems()` return the filtered slice.

The existing test harness `pressSpaceThenEscWithRefresh` (`internal/tui/pagepreview_refetch_test.go:76-112`) round-trips the refresh message once (`got3.Update(refreshMsg)` at line 106) and discards the returned cmd (`updated4, _ := got3.Update(refreshMsg)`). After the fix, that discarded cmd is the propagated `filterItems` cmd — the one whose `FilterMatchesMsg` must be fed back through `Update` for `VisibleItems()` to return non-empty.

Consequence: the prescribed augmentation to `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` — "assert `visibleSessionNames(got)` equals the expected filtered slice" — will fail on a correctly-fixed implementation unless the helper is also extended to drain the propagated cmd and feed its `FilterMatchesMsg` back through `Update`. Same applies to the prescribed kill-refresh test, which routes through `applySessions` via `SessionsMsg` and produces the same async `FilterMatchesMsg`.

The cursor-index assertion added by cycle 2 is less impacted because `reanchorSessionCursor` early-returns when `VisibleItems()` is empty (line 762-764 in model.go), leaving the bubbles list's pre-existing internal cursor index intact — on the primary path that index already points at the highlighted row, so the assertion may pass without the drain. But the visibility assertion definitively requires the drain.

The spec is silent on this. An implementer reading the Test Coverage section will write the assertion as prescribed, see it fail, and have to reverse-engineer the bubbles/list filter pipeline to diagnose. Worse, an implementer might mis-diagnose and conclude the fix is wrong.

Recommend the spec either (a) prescribe extending `pressSpaceThenEscWithRefresh` (and any analogous helper used by the kill-refresh test) to drain the propagated cmd and round-trip its emitted `FilterMatchesMsg` through `Update`, or (b) explicitly note that the assertion shape must account for the async refilter and point at the harness adjustment site.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
