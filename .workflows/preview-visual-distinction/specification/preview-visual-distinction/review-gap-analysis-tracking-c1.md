---
status: in-progress
created: 2026-05-18
cycle: 1
phase: Gap Analysis
topic: preview-visual-distinction
---

# Review Tracking: Preview Visual Distinction - Gap Analysis

## Findings

### 1. Cascade budget arithmetic vs top-edge column layout not reconciled

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Width cascade* (Algorithm shape: predicate-over-output, Tier-by-tier behaviour) and *Top edge composition* (Column layout)

**Details**:
Two sections describe related-but-not-identical mechanics and the relationship between them is implicit.

- *Width cascade* says each tier produces a candidate, measures it via `lipgloss.Width`, and returns if it fits. The implied measurement target is "the available width," but the available width for *chrome content* is not the terminal width — the column layout reserves columns 0, 1, `width − 2`, and `width − 1` for corner + 1-cell padding on each side. Chrome content therefore has a budget of `width − 4`, not `width`.
- *Top edge composition*'s Column layout describes the position of chrome content as columns `2` through `2 + chromeWidth − 1`, with filler in between, and pins the right corner at `width − 1`.

The spec never states that `composeChromeLine(width, …)`'s `width` parameter is the *inner* chrome budget (`width − 4`), nor that the cascade measures candidates against that same budget. Tier 4's description ("always fits any width ≥ 2") references `width ≥ 2` in terminal-width terms (since tier 4 produces `╭{─×(width−2)}╮`), which conflicts with tier 1-3 measurement if `width` is the chrome budget.

An implementer cannot tell from the spec whether:
- `composeChromeLine` takes the *outer* width and is responsible for the cascade against `width − 4` internally, returning chrome content only; or
- `composeChromeLine` takes the *inner* chrome budget and returns chrome content; or
- `composeChromeLine` returns the *full top-edge row* (corners + padding + filler + chrome) and the cascade measures the full row against `width`.

The resize-handler section says `composeChromeLine(msg.Width − 2, …)` — passing `width − 2` (inner width including the 1-cell padding on each side but excluding the corners), which is a third reading. The chrome-row invariant section refers to `composeChromeLine(w, …)` without disambiguating.

**Proposed Addition**:
Pin `composeChromeLine`'s `width` parameter as inner frame width (`terminalWidth − 2`). Function returns the complete top-edge row of display-cell width `width + 2`. Update Tier 4 to `╭{─ × width}╮`. Disambiguate the column-layout section.

**Resolution**: Approved
**Notes**: Logged to *Width cascade* (new *Unit of measure* subsection + updated Tier 4 + updated *Pure function* signature/docstring) and *Top edge composition* (Column layout disambiguation + *Width below threshold* threshold update).

---

### 2. Tier 2 threshold specified as "~8 display cells" — exact integer left to implementer

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Width cascade* — Tier 2 description

**Details**:
The Tier 2 spec says "Reached when the budget for the window name segment falls below a sensible minimum (**target: ~8 display cells**). Below that minimum the truncation reads as garbage rather than as a recognisable name."

The "~" makes this an approximate target. An implementer must pick an exact integer (8? 7? 10?). Tests would then need to assert against whatever integer was chosen. This is a one-line decision but the spec leaves the exact value to implementer judgement, which conflicts with the spec's general posture of pinning constants (`verboseKeymap`, `compactKeymap` are byte-exact).

A related concern: the test table fixes `width: 40` to "No `win:` segment (tier 2 dropped); verbose keymap" — this implicitly pegs the threshold, but the test value isn't documented as the canonical threshold definition.

**Proposed Addition**:
Fix the tier-2 minimum as `const minWindowNameCells = 8`. Tests assert tier-2 entry exactly at this boundary.

**Resolution**: Approved
**Notes**: Logged to *Width cascade* Tier 2.

---

### 3. Chrome recomputation on window/pane navigation not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Resize behaviour*, *Chrome line content*

**Details**:
Chrome content is dynamic-only — Window/Pane indicators and window name change as the user navigates with `]`, `[`, `⇥`. The spec describes chrome recomputation on `tea.WindowSizeMsg` (resize), but does not state that chrome must also be recomputed on the navigation key presses that change the indicator values.

Two readings are possible:
- Chrome is recomputed in `View()` on every tick from current model state (no caching) — then no special recompute is needed; the cached `m.chromeLine` field isn't even necessary.
- Chrome is cached on a model field and recomputed only on resize — then the spec must also state that navigation handlers invalidate / recompute the cached chrome.

The "Initial sizing" section implies caching: "The initial chrome string is pre-computed for the inner width." The Resize section also implies caching: "Recompute the chrome line via `composeChromeLine(msg.Width − 2, …)`." Both suggest the chrome is stored on a model field. But the navigation-triggered recompute is never mentioned.

**Proposed Addition**:
Drop chrome caching. `View()` recomputes via `composeChromeLine(m.width − 2, …)` every tick. No invalidation logic in navigation handlers.

**Resolution**: Approved
**Notes**: Logged to *Resize behaviour* (revised rule + flow) and *Initial sizing and preview-open ordering* (constructor no longer pre-computes chrome).

---

### 4. `composeChromeLine` model-field parameters left as placeholder

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Width cascade* — Pure function, *Code shape changes*, *Tests*

**Details**:
The spec writes the signature as `composeChromeLine(width int, /* model fields */) string` with a comment placeholder, and tests reference `composeChromeLine(w, …)` with an ellipsis. The implementer must infer the exact parameter list from the cascade segments: window index, window count, pane index, pane count, window name. Probably correct, but not pinned.

This becomes load-bearing in two places:
- The cascade-tier table-driven test (Surface 5) requires the test to construct calls — implementer must guess the canonical parameter order.
- The chrome-row invariant test (`strings.Count(composeChromeLine(w, …), "\n") == 0`) needs concrete inputs to compile.

Pinning the signature is a one-line decision that removes an interpretation surface.

**Proposed Addition**:
{To be discussed — fix the exact signature, e.g., `composeChromeLine(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) string`.}

**Resolution**: Pending
**Notes**:

---

### 5. Sessions-page call site file path not named

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Code shape changes* — File scope summary

**Details**:
The File scope summary table lists "Sessions page preview-open call site" without naming the file. CLAUDE.md identifies the TUI package as `internal/tui` and references `internal/tui/modal.go` as a sibling. By convention the Sessions page file is `internal/tui/pagesessions.go` (paralleling `pagepreview.go`), but the spec doesn't pin it.

A planner would have to either inspect the codebase to find the call site, or accept that the file name is "whichever file contains the `Space`-binding handler for the Sessions page." Easy to resolve, but currently absent.

**Proposed Addition**:
{To be discussed — name the exact file path for the call site.}

**Resolution**: Pending
**Notes**:

---

### 6. `previewChromeHeight` rename — possible cross-file references not enumerated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Code shape changes* — Rename `previewChromeHeight` → `previewFrameOverhead = 2`; File scope summary's "No other files are touched"

**Details**:
The spec renames `previewChromeHeight` and asserts "No other files are touched." If the constant is referenced from outside `pagepreview.go` (e.g., from a parent model that does size accounting, or from a test in another file), the rename is not a one-file change. The spec doesn't enumerate the existing call sites of the old constant.

Likely the constant is file-local, but the spec asserts a "no other files touched" bound without verifying.

**Proposed Addition**:
{To be discussed — either confirm `previewChromeHeight` is file-local (in which case state it), or list any external call sites that need updating.}

**Resolution**: Pending
**Notes**:

---

### 7. Initial-construction edge case when width/height are unknown

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Initial sizing and preview-open ordering*, *Top edge composition* — Width below 2

**Details**:
"The parent Bubble Tea model holds current terminal dimensions from program-start." If for any reason `Space` is pressed before the first `tea.WindowSizeMsg` has been processed (very unlikely in practice, but not impossible — race at startup), the constructor receives `(0, 0)`.

The spec says `composeChromeLine` returns empty string for `width < 2`, and "lipgloss handles widths it cannot render by clipping." But:
- `viewport.SetSize(0 − 2, 0 − 2)` = `SetSize(-2, -2)` — undefined behaviour for negative arguments.
- Once the real `WindowSizeMsg` arrives, the resize handler corrects everything.

The spec's stance ("widths 0 and 1 are degenerate, render whatever falls out, no error path, no panic") covers the `composeChromeLine` side but not the `viewport.SetSize` side. An implementer might reasonably guard against negative SetSize arguments or might not — the spec leaves it open.

**Proposed Addition**:
{To be discussed — clarify whether the constructor and resize handler guard `SetSize` against negative inputs, or rely on bubbles/viewport's own behaviour.}

**Resolution**: Pending
**Notes**:

---

### 8. Sessions page's current width/height — plumbing assumed but not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Initial sizing and preview-open ordering*, *Code shape changes* — Sessions call site

**Details**:
"The Sessions page's `Update` handler passes its current width / height into the constructor." This presumes the Sessions page model already has `width` / `height` fields that track current terminal dimensions. The spec doesn't state whether this is true today or requires new plumbing in the Sessions page.

If new fields are needed (the Sessions page would need its own `tea.WindowSizeMsg` handler to record dimensions), the File scope summary's claim that the only changes are in `pagepreview.go` plus the preview-open call site under-states the work.

**Proposed Addition**:
{To be discussed — confirm Sessions page already tracks width/height, or add the dimension-tracking work to the file scope summary.}

**Resolution**: Pending
**Notes**:

---

### 9. Window-name source / model field not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Chrome line content* — Segments, *Width cascade* — Tier 1

**Details**:
Tier 1 truncates "the window name." The spec doesn't say where this string comes from on `previewModel`. CLAUDE.md describes `WindowGroup` as the shape returned by `ListWindowsAndPanesInSession`, which is "consumed by the TUI scrollback preview." It is reasonable to infer the window name lives on a field of the current `WindowGroup` (selected by some index), but the spec doesn't connect cascade-tier-1's "window name" to any concrete model accessor.

This affects test fixture construction (Surface 5 says "a fixed window-name fixture" — implementer needs to know where to inject it) and the `composeChromeLine` signature (finding #4).

**Proposed Addition**:
{To be discussed — name the model field / accessor that yields the current window name.}

**Resolution**: Pending
**Notes**:

---

### 10. Adaptive color constant — naming and location not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Border colour*, *Top edge composition* — Color application, *Code shape changes* — Style sourcing

**Details**:
The color application section uses `adaptiveBlue` in the conceptual code example. The Style sourcing section says "The `AdaptiveColor` defining the border foreground is declared once in `pagepreview.go` (or a near neighbour)." This leaves both the identifier (`adaptiveBlue`? `previewBorderColor`?) and the exact location (`pagepreview.go` vs. a styles file) to implementer judgement.

Tests don't depend on the name, so this is minor — but the File scope summary doesn't list "add adaptive color constant" as a change line, even though it's a new top-level declaration.

**Proposed Addition**:
{To be discussed — pin the constant name and location, and add the constant addition to the File scope summary if appropriate.}

**Resolution**: Pending
**Notes**:

---

### 11. Cascade-tier test fixture window-name length not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Tests* — Surface 5 cascade-tier sub-test

**Details**:
The cascade-tier table at widths 200/60/40/25/15 asserts specific tier signatures. Whether width 60 hits tier 1 (truncation) or tier 2 (drop) depends on:
- The exact verbose-keymap byte width (82 cells per spec).
- The chrome-content budget at width 60 (depends on finding #1's resolution).
- The window name's display-cell width.
- The tier-2 minimum (finding #2).

Without pinning the window-name fixture used in the test, an implementer cannot tell whether the table assertions hold. The same concern applies to the width 60 row's "Window name truncated with `…` suffix" assertion — the fixture name needs to be long enough that it truncates at width 60 but doesn't get dropped.

**Proposed Addition**:
{To be discussed — specify the test fixture window name, or convert the table to assertions on tier-output shape rather than tied-to-exact-width signatures.}

**Resolution**: Pending
**Notes**:

---

### 12. SGR-reset injector — invocation site not explicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *SGR reset injection*, *Code shape changes*

**Details**:
The spec defines `injectSGRResets` and says it runs "when wrapping `viewport.View()` output for the frame composition." The File scope summary lists "Add `injectSGRResets` helper" but doesn't make the `View()` integration explicit ("Compose top edge manually; wrap viewport content with frame" — implementer must infer that "wrap" includes the SGR injector call).

This is borderline; a careful implementer reading both sections together will compose them correctly. But the View() line in the summary doesn't enumerate the SGR injector invocation.

**Proposed Addition**:
{To be discussed — make the SGR injector's invocation point in `View()` explicit in the code-shape section.}

**Resolution**: Pending
**Notes**:
