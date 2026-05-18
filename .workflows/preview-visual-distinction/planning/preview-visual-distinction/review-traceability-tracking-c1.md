---
status: complete
created: 2026-05-18
cycle: 1
phase: Traceability Review
topic: Preview Visual Distinction
---

# Review Tracking: Preview Visual Distinction - Traceability

## Findings

### 1. Task 1-8 deviates from spec on chrome content styling

**Type**: Hallucinated content
**Spec Reference**: § Top edge composition > Color application (and the "Implication for composeChromeLine's purity" sub-paragraph at the end of that section)
**Plan Reference**: Phase 1 / Task `preview-visual-distinction-1-8` — Compose painted frame in View() and initialise viewport in NewPreviewModel
**Change Type**: update-task

**Details**:
Task 1-8 explicitly takes a "pragmatic interpretation" that contradicts a validated spec decision. The spec's § Color application section is explicit and load-bearing:

> The top edge is composed as **two stylings concatenated**:
> - **Border parts** — corner glyphs, the `─` padding cells, and the `─` filler. Wrapped in `lipgloss.NewStyle().Foreground(adaptiveBlue).Render(…)` so they pick up the design colour.
> - **Chrome content** — rendered with no explicit foreground, inheriting terminal default. Chrome reads as legible terminal text against the blue-bordered surround. This matches the convention used elsewhere in the TUI for label-style strips.
>
> Final assembly at the `View()` call site, conceptually:
>
> ```
> styledBorder("╭─") + chromeContent + styledBorder(filler + "─╮")
> ```

Task 1-8's implementation tints the entire top edge — chrome content included — with `previewBorderColor` via a single `Render` call, and its in-task NOTE openly acknowledges this. The task even flags it as a "single edit point if reviewers prefer the stricter form". The traceability standard is that the plan must faithfully implement the spec; an acknowledged deviation is still a deviation. This must be corrected to render the chrome content with no explicit foreground and only the border parts in `previewBorderColor`.

The fix requires `composeChromeLine` to expose its border-parts vs chrome-content boundary so the `View()` call site can apply two-style composition. The simplest change that preserves the existing pure function's return contract (full top-edge row, display-cell width `width + 2`) is to introduce a second variant or a helper that returns the same content as a structured split — `(leftBorder, chrome, rightBorder string)` — used only at the call site. Tier 4 simply returns `("", "", "")` for chrome (or equivalently the full filler as border parts).

**Current** (task 1-8's `View()` body and the surrounding NOTE):

```go
func (m previewModel) View() string {
    // Compose chrome content for the top edge. Recomputed every tick;
    // no cached field. Pure function — no I/O.
    chrome := composeChromeLine(
        m.width-previewFrameOverhead,
        m.windowIdx, len(m.groups),
        m.paneIdx, len(m.currentGroup().Panes),
        m.currentGroup().WindowName,
    )
    // Style the body: borders on left, right, bottom (the three lipgloss-rendered
    // edges). Top border is hand-composed and concatenated above.
    borderStyle := lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder(), false, true, true, true).
        BorderForeground(previewBorderColor)
    body := borderStyle.Render(injectSGRResets(m.viewport.View()))

    // Style the hand-composed top edge so its border parts (corners + filler)
    // pick up the design colour. composeChromeLine returns a single styled
    // string today — wrapping the whole thing in Foreground would tint the
    // chrome characters too, which the spec forbids. So we apply the
    // foreground only to the border parts and leave chrome content with
    // terminal-default foreground. composeChromeLine already returns a
    // single concatenated row; to keep this task self-contained we tint
    // the whole top edge with previewBorderColor as a single Render call,
    // which is consistent with the spec's "border parts coloured" rule
    // when chromeContent is empty (tier 4) and acceptable in all other
    // tiers because chrome content does not contain SGR-reset-sensitive
    // sequences.
    // ──
    // NOTE on spec ambiguity: the spec describes the top edge as
    // "two stylings concatenated" — border parts coloured, chrome
    // content inheriting default foreground. composeChromeLine
    // currently returns a single concatenated string. Splitting the
    // styling boundary requires either composeChromeLine returning
    // a structured (border-parts, chrome, border-parts) triple or
    // this call site doing the slicing. The simpler path that
    // satisfies the user-visible acceptance criterion "all four
    // edges are coloured via previewBorderColor" is to tint the
    // entire top edge here. Chrome content rendered with the same
    // blue is still legible terminal text. If reviewers prefer the
    // strict spec interpretation this site is the single edit point.
    styledTop := lipgloss.NewStyle().Foreground(previewBorderColor).Render(chrome)

    return styledTop + "\n" + body
}
```

(Plus the related Context note in task 1-8 that flags this as a "pragmatic interpretation".)

**Proposed** (task 1-8 rewritten end-to-end to faithfully implement the spec's two-style composition):

```markdown
### Task 1-8: Compose painted frame in View() and initialise viewport in NewPreviewModel

**Problem**: After task 1-7's stub, `View()` returns the bare viewport content — no frame, no chrome, no SGR-reset injection. The build phase's user-visible payload is to render the rounded blue frame around the viewport every tick, with the chrome line riding on the top border row, the three other edges rendered by `lipgloss`, and SGR resets applied per row. Additionally, `NewPreviewModel`'s constructor must initialise the viewport with both dimensions reduced by `previewFrameOverhead` so the first `View()` call (before any `WindowSizeMsg`) renders at the correct size.

**Solution**: Rewrite `View()` in `internal/tui/pagepreview.go` to:
1. Compute the top edge as **two stylings concatenated** — border parts (corners + padding `─` + filler `─`) wrapped in `lipgloss.NewStyle().Foreground(previewBorderColor).Render(…)`, chrome content rendered with no explicit foreground inheriting terminal default. This requires a structured split between border parts and chrome content; add a sibling helper `composeChromeLineParts(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) (left, chrome, right string)` that returns the three parts so the call site can apply the two-style composition. `composeChromeLine` continues to return the assembled top edge as before for the pure-function tests.
2. Pass `viewport.View()` through `injectSGRResets`.
3. Use `lipgloss.NewStyle()` with `Border(lipgloss.RoundedBorder())`, `BorderTop(false)`, and `BorderForeground(previewBorderColor)` to wrap the body, rendering the left/right/bottom edges only.
4. Concatenate styled top edge + bordered body.

Also update `NewPreviewModel` to call `viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead))` so the initial viewport size matches what the resize handler would produce.

**Outcome**: `pagePreview.View()` returns a string containing all four rounded corner glyphs (`╭ ╮ ╰ ╯`), all four edges coloured via `previewBorderColor`, the chrome line embedded in the top border row **with chrome content rendered in terminal default foreground (no explicit foreground SGR for the chrome region)**, and SGR resets on every non-empty viewport content row. The first frame on construction is correct without any `WindowSizeMsg` having been dispatched. `pageSessions`'s `View()` is unchanged and renders no frame.

**Do**:
- In `internal/tui/pagepreview.go` `NewPreviewModel`, change `viewport: viewport.New(width, max(0, height-previewFrameOverhead))` to:
  ```go
  viewport: viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead)),
  ```
- Add `composeChromeLineParts` alongside `composeChromeLine` in `internal/tui/pagepreview.go`. It runs the same cascade as `composeChromeLine` but returns `(left, chrome, right string)` where `left` and `right` are the border parts (corners + padding `─` + filler `─`) and `chrome` is the unstyled chrome content. For tier 4 (chrome dropped), `chrome` is `""` and `left + right` together equal the full filler `╭{─ × (outer-2)}╮`. Concatenating `left + chrome + right` reproduces what `composeChromeLine` returns exactly. Both functions share the same internal tier-selection logic — extract it into a private helper so the two stay in lockstep.
- Rewrite `View()`:
  ```go
  func (m previewModel) View() string {
      // Compose top-edge parts. Recomputed every tick — no cached field. Pure
      // function — no I/O. The structured split exists so the call site can
      // apply the spec's "two stylings concatenated" composition: border parts
      // coloured, chrome content default foreground.
      left, chrome, right := composeChromeLineParts(
          m.width-previewFrameOverhead,
          m.windowIdx, len(m.groups),
          m.paneIdx, len(m.currentGroup().Panes),
          m.currentGroup().WindowName,
      )
      borderStyle := lipgloss.NewStyle().Foreground(previewBorderColor)
      styledTop := borderStyle.Render(left) + chrome + borderStyle.Render(right)

      // Style the body: borders on left, right, bottom (the three lipgloss-
      // rendered edges). Top border is hand-composed and concatenated above.
      bodyBorderStyle := lipgloss.NewStyle().
          Border(lipgloss.RoundedBorder(), false, true, true, true).
          BorderForeground(previewBorderColor)
      body := bodyBorderStyle.Render(injectSGRResets(m.viewport.View()))

      return styledTop + "\n" + body
  }
  ```
- Add a `pagepreview_view_frame_test.go` that:
  - Constructs `previewModel` with mocks (1 window, 1 pane, window name `"nvim-editor"`) and width/height = 80/24.
  - Dispatches `Update(tea.WindowSizeMsg{Width: 80, Height: 24})`.
  - Calls `View()`.
  - Asserts the output contains `╭`, `╮`, `╰`, `╯` (all four corners).
  - Asserts the output contains the chrome substring `"Window 1 of 1 · Pane 1 of 1 · win: nvim-editor"`.
  - Asserts (via `strings.Contains` of the raw rendered bytes) that the SGR-reset bytes `"\x1b[0m"` appear at least once per non-empty viewport content row. Construct the mock `ScrollbackReader` to return a fixture with an unterminated SGR (e.g. `"\x1b[41mhello\nworld\n"`); assert both content lines end with reset before frame composition.
  - Asserts `m.width == 80` is honoured: the rendered output's first line (after splitting on `\n`) has `lipgloss.Width == 80`.
  - **Asserts that the chrome content region of the rendered top row is not preceded by a foreground-colour SGR** — strip ANSI from the top row, locate the chrome substring, and verify the raw byte sequence preceding the chrome substring ends with a reset (or contains no `Foreground` SGR for the chrome span). The simplest concrete assertion: split the styled top row at the chrome substring; assert the prefix and suffix carry colour SGR codes but the chrome span itself does not.
- Also assert first-frame correctness: construct `previewModel` with `width=80, height=24` and immediately call `View()` (no `WindowSizeMsg`). The first frame's top row must have `lipgloss.Width == 80`.

**Acceptance Criteria**:
- [ ] `pagePreview.View()` output contains rounded corners `╭ ╮ ╰ ╯`.
- [ ] All four edges are styled with `previewBorderColor` — verified by checking the SGR sequences in the rendered output contain the truecolor or downgraded colour codes corresponding to the adaptive value, or (simpler) by asserting both the top edge and the lipgloss-rendered body include `\x1b[` style codes (lipgloss emits these unconditionally when a foreground is set).
- [ ] **Chrome content is rendered with no explicit foreground SGR** — verified by the structured split test described in Do. The top edge is composed as `styledBorder(left) + chrome + styledBorder(right)`, matching spec § Color application's "two stylings concatenated" rule.
- [ ] `composeChromeLineParts(width, …) (left, chrome, right string)` exists in `internal/tui/pagepreview.go` and `left + chrome + right` exactly equals `composeChromeLine(width, …)` for every input (cross-checked by a property-style test across the cascade-threshold widths).
- [ ] `View()` calls `composeChromeLineParts` every tick — no cached chrome field on the model.
- [ ] `NewPreviewModel` initialises the viewport with `viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead))`.
- [ ] First-frame correctness: calling `View()` on a freshly-constructed `previewModel` (no prior `WindowSizeMsg`) renders with correct dimensions.
- [ ] `injectSGRResets` is applied to `viewport.View()` output before frame composition.
- [ ] Degenerate widths handed to lipgloss without panic — test with `width=2, height=4`.
- [ ] No production code outside `internal/tui/pagepreview.go` is modified.

**Tests**:
- `"View output contains all four rounded corner glyphs"`
- `"View output top row width equals outer terminal width"`
- `"View chrome line contains window pane indicators and window name at wide width"`
- `"View renders chrome content with no explicit foreground SGR"`
- `"composeChromeLineParts left plus chrome plus right equals composeChromeLine at every cascade threshold"`
- `"View applies SGR reset to every non-empty viewport content row"`
- `"View first-frame correctness without prior WindowSizeMsg"`
- `"View at degenerate width 2 height 4 renders without panic"`
- `"View recomputes chrome every tick — no cached field"` (verify by changing `m.windowIdx` and confirming the rendered output reflects the new value without an intermediate `Update` call setting any cache)

**Edge Cases**:
- Chrome recomputed every tick — no cached field; verified by mutating `m.windowIdx` between two `View()` calls and asserting both reflect the current value.
- First-frame correctness at construction — no race between preview-open and the first `WindowSizeMsg`, no "first frame at zero width" edge case.
- Degenerate widths (e.g. `width=2`) handed to lipgloss without panic — `lipgloss` clips when it cannot render; the production code does not need a special case.
- Tier 4 in `composeChromeLineParts` — `chrome` is empty and `left + right` together equal `╭{─ × (outer-2)}╮`. Concatenation `left + "" + right` still reproduces `composeChromeLine` exactly.

**Context**:
> From spec § Top edge composition > Color application: the top edge is composed as **two stylings concatenated** — border parts (corners + padding `─` + filler `─`) wrapped in `lipgloss.NewStyle().Foreground(previewBorderColor).Render(…)` so they pick up the design colour; chrome content rendered with **no explicit foreground**, inheriting terminal default. Final assembly at the View() call site, conceptually:
>
> ```
> styledBorder("╭─") + chromeContent + styledBorder(filler + "─╮")
> ```
>
> From § Top edge composition > Color application > Implication for composeChromeLine's purity: top-edge styling — border parts coloured, chrome parts default — happens at the call site in `View()`. The structured split (`composeChromeLineParts`) exposes the boundary so the call site can apply the two-style composition without re-running cascade logic.
>
> From § Initial sizing and preview-open ordering: `viewport.SetSize(max(0, width − 2), max(0, height − 2))` is called once with initial dimensions (same `max(0, …)` clamp as the resize handler). `View()` recomputes the chrome line on every tick, so no separate pre-computation is needed at construction time. The first `View()` call on the freshly-constructed `previewModel` renders with correct dimensions — no race between preview-open and the first `WindowSizeMsg`.
>
> From § Style sourcing: corner and edge characters used in the manually-composed top edge are sourced from the chosen `lipgloss` border value (`lipgloss.RoundedBorder()`) rather than hardcoded.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Frame structure, § Border style, § Border colour, § Top edge composition, § Top edge composition > Color application, § Resize behaviour, § Initial sizing and preview-open ordering, § SGR reset injection.
```

**Resolution**: Fixed
**Notes**:

---

### 2. Task 1-4 lacks the structured-split helper required by faithful Color application

**Type**: Incomplete coverage
**Spec Reference**: § Top edge composition > Color application > "Implication for composeChromeLine's purity"
**Plan Reference**: Phase 1 / Task `preview-visual-distinction-1-4` — Implement composeChromeLine cascade tiers 1-4
**Change Type**: add-to-task

**Details**:
Task 1-4 implements `composeChromeLine` as a single function returning the fully-assembled top edge row. The spec's § Color application requires the call site to apply two distinct stylings (border parts coloured, chrome content default). Without a structured split that exposes the boundary between border parts and chrome content, the call site (task 1-8) cannot perform the spec's prescribed composition without re-running cascade logic or string-slicing the assembled output.

Finding 1 introduces `composeChromeLineParts` at the task 1-8 layer. For tasks to remain TDD-cycle-coherent, the parts-returning variant belongs in the same task as the cascade implementation — task 1-4 — so its tier selection is tested once and shared by both surfaces. Add `composeChromeLineParts` to task 1-4 alongside `composeChromeLine`, sharing a private tier-selection helper so the two functions cannot drift.

**Current** (task 1-4's Solution + Do sections; only the structural shape is reproduced here for brevity — the existing tier algorithm stays as-is):

```markdown
**Solution**: Implement `composeChromeLine(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) string` as a pure function in `internal/tui/pagepreview.go`. It builds candidate strings tier by tier, measures each via `lipgloss.Width`, and returns the first candidate whose width equals `width + 2` (the outer terminal width). Tier 4 is the load-bearing fallback that always fits.

**Outcome**: `composeChromeLine` exists in `internal/tui/pagepreview.go`. For every `width ≥ 0` it returns a single-row string (no `\n`) of display-cell width `width + 2`. For `width < 0` it returns the empty string. Tier selection is verifiable at threshold widths via direct return-value inspection. Border parts (corners, filler `─`) are sourced from `lipgloss.RoundedBorder()`, not hardcoded.
```

**Proposed** (add alongside the existing `composeChromeLine` implementation; the cascade tier code itself is unchanged):

```markdown
**Solution**: Implement `composeChromeLine(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) string` as a pure function in `internal/tui/pagepreview.go`. It builds candidate strings tier by tier, measures each via `lipgloss.Width`, and returns the first candidate whose width equals `width + 2` (the outer terminal width). Tier 4 is the load-bearing fallback that always fits.

Additionally implement a sibling pure function `composeChromeLineParts(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) (left, chrome, right string)` that runs the same cascade and returns the three structural parts (left border parts, unstyled chrome content, right border parts) of the same top-edge row. The two functions share a single private tier-selection helper so they cannot drift. Concatenating `left + chrome + right` reproduces `composeChromeLine`'s output exactly. The parts-returning variant exists so the `View()` call site can apply spec § Color application's "two stylings concatenated" composition (border parts coloured, chrome content default foreground) without re-running cascade logic.

**Outcome**: `composeChromeLine` and `composeChromeLineParts` exist in `internal/tui/pagepreview.go`. For every `width ≥ 0`, `composeChromeLine` returns a single-row string (no `\n`) of display-cell width `width + 2`; for `width < 0` it returns the empty string. For the same inputs, `composeChromeLineParts` returns `(left, chrome, right)` such that `left + chrome + right == composeChromeLine(...)` byte-for-byte. Tier selection is verifiable at threshold widths via direct return-value inspection. Border parts (corners, filler `─`) are sourced from `lipgloss.RoundedBorder()`, not hardcoded. Tier 4 returns `chrome == ""` and `left + right == ╭{─ × (outer-2)}╮`.
```

Also add to **Do**:

```markdown
- Implement `composeChromeLineParts` as a sibling of `composeChromeLine`. Extract the cascade tier-selection logic into a private helper (e.g. `selectChromeTier(...) (tier int, chrome string, fillerCells int)`) and have both public functions consume it. `composeChromeLine` then concatenates `tl + h + chrome + filler + h + tr` (or the tier-4 collapse) into one string; `composeChromeLineParts` returns the three parts separately: `left = tl + h`, `chrome = chrome`, `right = filler + h + tr` (or the tier-4 split where `chrome = ""` and the filler is partitioned between `left` and `right` such that `left + right == ╭{─ × (outer-2)}╮`; the partition can put all filler on the left or split it arbitrarily — concatenation is what tests assert).
```

Also add to **Acceptance Criteria**:

```markdown
- [ ] `composeChromeLineParts(width, …) (left, chrome, right string)` exists in `internal/tui/pagepreview.go` and a property test asserts `left + chrome + right == composeChromeLine(width, …)` byte-for-byte across the cascade-threshold widths (200, 60, 40, 25, 15, plus the 8/7-cell boundary cases and the degenerate widths 2/3/4).
- [ ] For tier 4 widths, `composeChromeLineParts` returns `chrome == ""`.
- [ ] For tier 1 widths with a non-truncated name, `composeChromeLineParts` returns a `chrome` value whose `lipgloss.Width` equals the chrome region's display-cell width (i.e. the chrome substring as it appears in `composeChromeLine`'s output).
```

Also add to **Tests**:

```markdown
- `"composeChromeLineParts left plus chrome plus right equals composeChromeLine at every cascade threshold"`
- `"composeChromeLineParts returns empty chrome at tier 4 widths"`
- `"composeChromeLineParts and composeChromeLine share tier-selection logic via a private helper"` (structural assertion — single tier-selection entry point exists; verified by inspection or by a test that mutates a fixture and confirms both functions reflect the change consistently)
```

**Resolution**: Fixed
**Notes**: Linked to Finding 1 — both findings together cover the chrome-styling deviation. Finding 1 fixes the call site (task 1-8); this finding fixes the underlying primitive (task 1-4) so the call site has the right tool. Approving Finding 1 without this finding would force task 1-8 to either inline the slicing logic (brittle) or re-run cascade logic at the call site (duplication).

---
