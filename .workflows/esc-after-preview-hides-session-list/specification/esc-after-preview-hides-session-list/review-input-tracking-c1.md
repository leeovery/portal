---
status: in-progress
created: 2026-05-20
cycle: 1
phase: Input Review
topic: esc-after-preview-hides-session-list
---

# Review Tracking: esc-after-preview-hides-session-list - Input Review

## Findings

### 1. Sibling bubbles/list APIs share the same lossy-cmd trap

**Source**: `investigation/esc-after-preview-hides-session-list.md` → "Contributing Factors" (lines 118-120): "The list's `SetItems` API has the well-known 'returns a cmd you must propagate' shape; an analogous trap exists for `SetItem`, `InsertItem`, `RemoveItem` (lines 421, 435, 449 in bubbles list) — anywhere these are called without forwarding the cmd, a filtered list will go blank."
**Category**: Enhancement to existing topic
**Affects**: Scope (sweep), Fix Approach (secondary sweep), Acceptance Criteria

**Details**:
The investigation explicitly flags that the same lossy-plumbing class extends beyond `SetItems` to its sibling mutators (`SetItem`, `InsertItem`, `RemoveItem`). The specification's secondary sweep is scoped only to remaining `SetItems` discard sites in `model.go`. If the codebase also calls any of `SetItem`/`InsertItem`/`RemoveItem` against `m.sessionList` or `m.projectList`, those would exhibit the identical blank-list-under-filter failure and are not addressed by the spec's planned sweep. At minimum, the spec should either (a) extend the sweep to verify these sibling APIs aren't used in discard form, or (b) explicitly document that the sweep is `SetItems`-only and why.

**Current** (Scope → In scope, third bullet):
> Sweep of the remaining `SetItems` discard sites in `internal/tui/model.go` (`Model.WithInsideTmux`, `ProjectsLoadedMsg` handler). These are currently safe because they run before any filter is applied, but the lossy plumbing shape is identical and would break if a filter could be applied at those points in the future. Fixing them in the same pass closes the class of bug.

**Proposed Addition**:
_Pending discussion_ — decide whether to broaden the sweep to `SetItem`/`InsertItem`/`RemoveItem` audit, or scope it explicitly to `SetItems` only.

**Resolution**: Pending
**Notes**:

---

### 2. Likely-regression provenance from prior preview work

**Source**: `investigation/esc-after-preview-hides-session-list.md` → "Notes" (line 188): "Related recently-completed work: `session-scrollback-preview`, `enter-attaches-from-preview`, `preview-visual-distinction`, `preview-keymap-discoverability`, `space-dismisses-preview` — preview pathway has had multiple iterations; this bug may be a regression introduced by one of them."
**Category**: Enhancement to existing topic
**Affects**: Why It Wasn't Caught (or a new Notes/Provenance section)

**Details**:
The investigation observes the bug is likely a regression introduced during one of the recent preview-pathway work units. The "Contributing Factors" section also notes that "The preview-dismiss refresh path (added in `enter-attaches-from-preview` for the externally-killed-session case) is the first realistic scenario in which `applySessions` can be called against a filtered list — the original `SessionsMsg`-only use was filter-naive." This regression-origin context is absent from the spec. It is useful framing for reviewers — explains why a recently-merged code area produced the bug, and reinforces the test-coverage rationale (the wrong-axis miss was introduced together with the regressing path).

**Current** ("Why It Wasn't Caught" section — three bullets covering the wrong-axis assertion, the unwired SessionLister, and the missing compile-time signal).

**Proposed Addition**:
_Pending discussion_ — likely a short note in "Why It Wasn't Caught" (or a new short subsection) attributing the regressing call path to `enter-attaches-from-preview` and noting the original `SessionsMsg`-only `applySessions` usage was filter-naive.

**Resolution**: Pending
**Notes**:

---

### 3. Initial mis-scoping of `applySessions` call sites

**Source**: `investigation/esc-after-preview-hides-session-list.md` → "Root Cause" (lines 113-114): "the `SessionsMsg` `applySessions` call site (`model.go:893-897`) is **not** boot-only as initially scoped — it also fires from `killAndRefresh` ... and `renameAndRefresh` ... which can run while a filter is applied (`x` and `r` keys are accepted on the Sessions page mid-filter)."
**Category**: Enhancement to existing topic
**Affects**: Root Cause

**Details**:
The investigation documents that the `SessionsMsg`/`applySessions` site was initially scoped as boot-only and that this scoping was wrong — `x` and `r` keys are accepted on the Sessions page mid-filter, which is what makes `killAndRefresh` and `renameAndRefresh` legitimate latent-variant call paths. The spec correctly enumerates the latent variants but loses the explanatory bit about why those paths matter (the keymap permits `x`/`r` while a filter is applied). This is a small but useful piece of evidence that supports the "latent variants" claim concretely rather than as an assertion.

**Current** (Root Cause, second paragraph):
> The preview-dismiss path is the most prominently affected because `previewSessionsRefreshedMsg` always fires after a `Space` keystroke on the Sessions page, where a filter may be applied. The same `applySessions` call site is reached from `killAndRefresh` (`model.go:1517-1525`), `renameAndRefresh` (`model.go:1571-1579`), and the `previewAttachBailMsg` handler (`model.go:975-993`) — all of which can run while a filter is applied. Those paths share the same blank-list / lost-filter outcome.

**Proposed Addition**:
_Pending discussion_ — add a short clause noting that `x` and `r` keystrokes are accepted on the Sessions page even when a committed filter is applied, which is what makes `killAndRefresh` / `renameAndRefresh` reachable from a filtered list.

**Resolution**: Pending
**Notes**:

---
