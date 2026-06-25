---
status: in-progress
created: 2026-06-26
cycle: 3
phase: Gap Analysis
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Gap Analysis

## Findings

### 1. `ProjectsLoadedMsg` handler described with a guard (`activePage != PageLoading`) it does not actually have

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Constraints & Invariants ("Decision always resolves on the cold route" bullet); § Fix Approach ("Ordering contract (cold route)" paragraph)

**Details**:
The spec twice describes the *existing* `ProjectsLoadedMsg` handler as calling `evaluateDefaultPage()` conditionally on the page:

- Constraints & Invariants: "**both** the `SessionsMsg` handler and the `ProjectsLoadedMsg` handler call `evaluateDefaultPage()` whenever `activePage != PageLoading`".
- Fix Approach (Ordering contract): "any other `evaluateDefaultPage()` caller in the interim window (notably a `ProjectsLoadedMsg` that has not yet arrived) hits the `!sessionsLoaded` early-return".

In `internal/tui/model.go` the `ProjectsLoadedMsg` arm calls `m.evaluateDefaultPage()` **unconditionally** (no `activePage != PageLoading` gate) at the end of the handler, immediately after `m.projectsLoaded = true`. Only the `SessionsMsg` arm carries the `if m.activePage == PageLoading { return ... }` early-return that stops it running `evaluateDefaultPage` during loading.

Why this matters for implementation:
- The spec is describing the **as-is** machinery it relies on (it explicitly leaves the latch and decision logic untouched and confines the change to the cold-route deferral). An implementer or reviewer cross-checking the handler against the spec will find the stated `activePage != PageLoading` condition is absent on `ProjectsLoadedMsg`, and could either (a) "fix" the handler to add the non-existent guard — an out-of-scope behaviour change the spec did not intend — or (b) lose confidence in the spec's accuracy and re-derive the safety argument from scratch.
- The actual safety property the fix depends on is the `!sessionsLoaded` early-return inside `evaluateDefaultPage()` itself (correctly stated elsewhere), which holds regardless of whether the caller is guarded by page. So the fix's *correctness* is unaffected; the *description of why* is what is wrong. The `activePage != PageLoading` phrasing is a misattribution of the `SessionsMsg`-only guard onto both handlers.

Suggested correction direction: re-state the invariant in terms of the real mechanism — the `ProjectsLoadedMsg` handler calls `evaluateDefaultPage()` unconditionally on every project load, and premature latching during the interim window is prevented solely by the `!sessionsLoaded` early-return inside `evaluateDefaultPage()` (not by any page guard on the `ProjectsLoadedMsg` caller). For `SessionsMsg`, the `activePage == PageLoading` early-return additionally suppresses the call while on the loading page. This keeps the two handlers' actual guard shapes distinct and accurate.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
