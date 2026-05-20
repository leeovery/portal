---
status: in-progress
created: 2026-05-20
cycle: 1
phase: Gap Analysis
topic: esc-after-preview-hides-session-list
---

# Review Tracking: esc-after-preview-hides-session-list - Gap Analysis

## Findings

### 1. `WithInsideTmux` returned cmd has no scheduling path

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Approach > Secondary sweep — `Model.WithInsideTmux`

**Details**:
The spec prescribes changing `WithInsideTmux` from `func (Model) WithInsideTmux(...) Model` to `(*Model, tea.Cmd)` "or analogous" and "updating the call site to batch the cmd". The single call site is `cmd/open.go:360`, which runs at TUI construction time — **before** `tea.NewProgram(m).Run()` is invoked. There is no `tea.Cmd` dispatcher available at that point; the cmd cannot be "batched" in the usual sense.

Plausible options the spec leaves unstated:
- Stash the cmd on the Model and emit it from `Init()` alongside the existing Init cmds.
- Emit the cmd via a synthetic startup message the top-level Update handles.
- Document that the cmd is `nil` at construction (no sessions populated yet → `SetItems` against empty filter → `nil`), and propagate it for shape-consistency only, accepting it will be discarded at the construction call site with a comment explaining why.

Without guidance, an implementer would have to make this design call. The spec already notes "Currently safe (construction-time, no filter possible)" — which makes the third option viable but unspoken.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. Rename-refresh and external-bail variants lack new test coverage

**Source**: Specification analysis
**Affects**: Test Coverage; Acceptance Criteria #2 and #6

**Category**: Gap/Ambiguity

**Details**:
Acceptance Criteria #2 names four resolved variants: preview-dismiss, kill-via-`x`, rename-via-`r`, and `previewAttachBailMsg`. The Test Coverage section only specifies:
- A `VisibleItems()` assertion on the existing preview-Esc test.
- One **new** test for the kill-refresh-under-filter scenario.

No new test is prescribed for rename-refresh under filter, nor for the externally-killed-during-preview bail. Both go through `applySessions` and would be silently covered by the fix mechanically, but acceptance criterion #2 explicitly enumerates them as resolved variants — leaving the implementer to decide whether to add tests, or whether the single kill test is considered sufficient evidence by transitive argument.

Recommend either (a) requiring tests for both additional variants, or (b) explicitly stating that one representative latent-variant test is sufficient and the other variants are covered by code review alone.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. ProjectsLoadedMsg fix path lacks a test

**Source**: Specification analysis
**Affects**: Fix Approach > Secondary sweep; Test Coverage; Acceptance Criteria #5

**Category**: Gap/Ambiguity

**Details**:
Acceptance Criteria #5 requires `ProjectsLoadedMsg` handler to propagate the `SetItems` cmd, but the Test Coverage section adds no test against the Projects page. The spec notes the path is "Currently safe (handler runs before any project filter can be committed)" — which suggests no live test is necessary, but doesn't say so explicitly. Implementer must decide whether to add a test for shape-consistency, mirror the kill-refresh-under-filter shape against the Projects page, or skip.

If skipped intentionally (the path is unreachable today), the spec should state that and warn against adding a contrived test that can't actually exercise the broken state.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. Sibling mutator audit outcome — where to record

**Source**: Specification analysis
**Affects**: Scope; Acceptance Criteria #5

**Category**: Gap/Ambiguity

**Details**:
The spec mandates an audit of `SetItem`, `InsertItem`, `RemoveItem` against `m.sessionList` / `m.projectList`, and says "if none exist, record that the audit ran clean". The recording surface is unspecified — PR description? A comment in the spec? An entry in Working Notes? A code comment near `applySessions`?

A planner cannot turn "record the audit outcome" into a concrete task without knowing the artifact. Recommend naming the destination (e.g., "noted in PR description and in this specification's Working Notes").

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 5. Kill-refresh test mechanics underspecified

**Source**: Specification analysis
**Affects**: Test Coverage > "Cover the latent variant"

**Category**: Gap/Ambiguity

**Details**:
The new test description reads:

> 2. Triggers the `x` kill-confirm modal flow against a filtered row.

The `x` flow involves a kill-confirm modal — the test must press `x`, then confirm (`y`/`Enter`?), then await the resulting `SessionsMsg` round-trip. The spec doesn't specify:
- Whether to drive through the actual modal keystrokes or shortcut to `killAndRefresh` via a synthetic message.
- Which `SessionKiller` seam to wire (real vs mock) and what it should return.
- Whether the assertion is on `VisibleItems()` length, names, or both — and which helper to use (the spec references `visibleSessionNames` only for the existing test).

Existing test patterns in the package likely give clear precedent, but the spec doesn't anchor to one — leaving the implementer to design the harness shape.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 6. `previewAttachBailMsg` handler — propagation site not enumerated in Fix Approach

**Source**: Specification analysis
**Affects**: Fix Approach; Root Cause § Execution path

**Category**: Gap/Ambiguity

**Details**:
The Root Cause section names `previewAttachBailMsg` (model.go:975-993) as one of four paths reaching `applySessions`. Acceptance Criteria #2 lists it as a variant that must be resolved. But the Fix Approach > Primary change > Update both call sites only enumerates:
- `SessionsMsg` handler (model.go:893-918)
- `previewSessionsRefreshedMsg` handler (model.go:1011-1023)

The `previewAttachBailMsg` handler currently calls `m.exitPreviewToSessions(...)` which returns the `refreshSessionsAfterPreviewCmd` — so the bail path goes through `previewSessionsRefreshedMsg`, meaning it's covered transitively by the second call site update. This is plausibly intentional but not stated. An implementer reading the spec may wonder why the bail handler isn't enumerated and look for a missed change.

A one-line note ("`previewAttachBailMsg` reaches `applySessions` via the same `previewSessionsRefreshedMsg` path; no separate change needed") would close the loop.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 7. `SessionsMsg` handler currently-returned cmd — batching specifics

**Source**: Specification analysis
**Affects**: Fix Approach > Primary change > Update both call sites

**Category**: Gap/Ambiguity

**Details**:
The spec says "batch the returned cmd into whatever the handler already returns" for the `SessionsMsg` handler. It does not specify whether the existing handler already returns a non-nil cmd (and so `tea.Batch` is needed) or returns `nil` (and so the new cmd is the sole return). The implementer must read model.go:893-918 to find out — which is fine, but a one-word characterization in the spec ("currently returns `nil` — return the new cmd directly" vs "currently returns X — `tea.Batch(existing, new)`") would make this a mechanical edit instead of a discovery task.

This is minor — the implementer will read the handler anyway — but the asymmetry with the `previewSessionsRefreshedMsg` line ("handler currently returns `nil`") is conspicuous.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
