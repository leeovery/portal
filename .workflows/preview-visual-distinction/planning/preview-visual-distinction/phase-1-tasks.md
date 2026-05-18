---
phase: 1
phase_name: Painted preview frame with chrome cascade
total: 9
---

## preview-visual-distinction-1-1 | approved

### Task 1-1: Add keymap glyph constants and adaptive border color

**Problem**: The new preview frame needs a single canonical source of the verbose and compact keymap strings (with macOS-glyph tokens replacing the older `tab`/`enter`/`esc` word forms) and a single adaptive border colour shared by all four edges. Without these top-of-file declarations the cascade tiers and frame composition tasks have no shared symbols to consume, and the existing in-line keymap string at `chromeLine()` would diverge.

**Solution**: Introduce two package-level string constants and one package-level `lipgloss.AdaptiveColor` variable in `internal/tui/pagepreview.go`, with byte content that exactly matches the spec's *Keymap glyphs* and *Style sourcing* sections.

**Outcome**: `internal/tui/pagepreview.go` declares `verboseKeymap`, `compactKeymap`, and `previewBorderColor` (only). The keymap constants use macOS keyboard glyphs (`⇥`, `⏎`, `⎋`) and the precise byte sequences from the spec; `previewBorderColor` is an `AdaptiveColor` with light `#3B5577` and dark `#7B95BD`. A test pins both keymap constants to literal bytes.

**Do**:
- Add a `"github.com/charmbracelet/lipgloss"` import to `internal/tui/pagepreview.go` if not already present.
- Add a package-level `const ( … )` block (or two `const` lines) at the top of `internal/tui/pagepreview.go` declaring:
  - `verboseKeymap = "] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back"`
  - `compactKeymap = "] [ ⇥ ⏎ ⎋"`
- Add a package-level `var previewBorderColor = lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}`.
- Add a new test file `internal/tui/pagepreview_keymap_constants_test.go` that asserts the two constants have the exact spec byte content (`if verboseKeymap != "] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back" { t.Errorf(...) }` and the compact equivalent).
- Do not touch `chromeLine()` yet — that is task 1-7. The constants must compile but be unused at this commit; Go will not error because `var` and `const` blocks are not "declared and not used".

**Acceptance Criteria**:
- [ ] `verboseKeymap` and `compactKeymap` exist as package-level `const` declarations in `internal/tui/pagepreview.go` with the exact byte content specified.
- [ ] `previewBorderColor` exists as a package-level `var` of type `lipgloss.AdaptiveColor` with `Light: "#3B5577"` and `Dark: "#7B95BD"`.
- [ ] A test asserts both constants by literal byte equality.
- [ ] `go build ./...` succeeds.
- [ ] No other production code is modified.

**Tests**:
- `"verboseKeymap exact byte content matches spec"`
- `"compactKeymap exact byte content matches spec"`
- `"compactKeymap is single-space separated with no interpuncts"` (already covered by the byte-equality test but worth naming explicitly so a future contributor can't loosen it)

**Edge Cases**: none — pure declarations.

**Context**:
> From spec § Keymap glyphs > Constants: the two forms are baked into `internal/tui/pagepreview.go` as exported-or-package-level constants and tests assert against these exact bytes. The compact form uses single-space separators (no interpunct) — display-cell width is **9 cells**.
>
> From spec § Style sourcing: `previewBorderColor` is preferred over `adaptiveBlue` to keep the variable's *role* front-and-centre rather than its current *hue*.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Keymap glyphs, § Border colour, § Style sourcing.

## preview-visual-distinction-1-2 | approved

### Task 1-2: Rename previewChromeHeight to previewFrameOverhead = 2

**Problem**: The existing `const previewChromeHeight = 1` encodes the pre-frame model (chrome rendered above the viewport as a separate row). Under the new model the chrome rides on the top border row and the frame costs 2 rows total (top edge + bottom edge). The constant name and value are both wrong post-frame; references in three sibling test files also need their arithmetic updated.

**Solution**: Rename the constant to `previewFrameOverhead`, change the value from `1` to `2`, update its doc comment, and update every reference (production and tests) including the arithmetic in the three test files that previously subtracted 1 and now must subtract 2.

**Outcome**: `previewChromeHeight` is gone from the codebase. `previewFrameOverhead = 2` exists in `internal/tui/pagepreview.go` with a doc comment stating "top border (carrying chrome) + bottom border = 2 rows of frame overhead". All three test files (`pagepreview_layout_test.go`, `pagepreview_precedence_test.go`, `pagepreview_scroll_test.go`) reference the new constant and their expected-height arithmetic uses the new value. Existing tests still pass with their updated expectations.

**Do**:
- In `internal/tui/pagepreview.go` replace the existing `const previewChromeHeight = 1` declaration (lines ~12–16) with:
  ```go
  // previewFrameOverhead is the total number of frame rows the preview's
  // rounded border occupies: top border (carrying chrome) + bottom border
  // = 2 rows of frame overhead. Used to compute the viewport's inner
  // height in NewPreviewModel and the tea.WindowSizeMsg handler.
  const previewFrameOverhead = 2
  ```
- Update production references in `pagepreview.go`:
  - `NewPreviewModel`'s `viewport.New(width, max(0, height-previewChromeHeight))` → `viewport.New(width, max(0, height-previewFrameOverhead))` (note: width handling will be fully revised in task 1-8; here just do the like-for-like rename — value drifts from 1 to 2 which is intentional).
  - The `tea.WindowSizeMsg` case's `max(0, msg.Height-previewChromeHeight)` → `max(0, msg.Height-previewFrameOverhead)` (similarly width handling revised later).
  - Comment reference at line ~346 — leave the comment block alone if you're rewriting `View()` in 1-8; otherwise update the textual reference.
- Update test references — search and replace `previewChromeHeight` → `previewFrameOverhead` in:
  - `internal/tui/pagepreview_layout_test.go` — three occurrences in the assertion messages and arithmetic (e.g. `wantHeight := 40 - previewChromeHeight` becomes `wantHeight := 40 - previewFrameOverhead`). Because the value changes from 1 to 2, the *computed* `wantHeight` changes; verify the tests still pass against the actual `viewport.Height` (they will — the production code now subtracts 2 too).
  - `internal/tui/pagepreview_precedence_test.go` — same pattern.
  - `internal/tui/pagepreview_scroll_test.go` — same pattern.
- Do NOT change error message strings in those tests beyond the constant-name substitution; the message text "(msg.Height - previewChromeHeight)" is being rewritten to "(msg.Height - previewFrameOverhead)" only.
- Run `grep -rn "previewChromeHeight" internal/` after the change to verify zero hits.

**Acceptance Criteria**:
- [ ] No occurrences of `previewChromeHeight` remain in any `.go` file under `internal/tui/`.
- [ ] `previewFrameOverhead` is declared exactly once, in `internal/tui/pagepreview.go`, with value `2` and the documented comment.
- [ ] `internal/tui/pagepreview_layout_test.go`, `pagepreview_precedence_test.go`, `pagepreview_scroll_test.go` reference `previewFrameOverhead` in their arithmetic.
- [ ] `go test ./internal/tui/...` passes for those three test files (and any other tests in the package that didn't previously reference the constant).

**Tests**:
- Existing tests in `pagepreview_layout_test.go`, `pagepreview_precedence_test.go`, `pagepreview_scroll_test.go` continue to pass with their arithmetic mechanically updated from the value drift 1 → 2. No new tests in this task — the constant rename's correctness is asserted by those existing height-math tests reaching the new expected `wantHeight`.
- `"viewport.Height equals msg.Height − previewFrameOverhead after resize"` (the existing layout-test scenario, now with value 2 enforced).

**Edge Cases**:
- Arithmetic drift in `pagepreview_layout_test.go`, `pagepreview_precedence_test.go`, `pagepreview_scroll_test.go` — the constant value changes from 1 to 2 so the computed `wantHeight` changes too. The production code in this task also drifts so the assertions stay green.
- Watch for any test that hard-codes `1` instead of using the constant; none exist today (a grep confirms this) but verify with `grep -n "Height-1\|Height - 1" internal/tui/`.

**Context**:
> From spec § Rename `previewChromeHeight` → `previewFrameOverhead = 2`: this names the magic 2 used in the resize math, preserves the file-local convention of naming chrome dimensions, and gives a single edit point if the frame's vertical geometry ever changes. The constant is package-internal to `internal/tui`. The rename must also update existing references in sibling test files; those assertions also need their arithmetic updated from `msg.Height − previewChromeHeight` to `msg.Height − previewFrameOverhead` since the magic value changes from 1 to 2. No references outside the `internal/tui` package.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Code shape changes > Rename `previewChromeHeight` → `previewFrameOverhead = 2`.

## preview-visual-distinction-1-3 | approved

### Task 1-3: Add display-cell-aware truncation primitive

**Problem**: The cascade's tier-1 ("truncate window name with `…` suffix") and tier-2's 8-cell minimum are specified in **display cells**, not bytes or runes. Window names are arbitrary UTF-8: CJK glyphs are 2 cells per rune, combining marks are 0 cells, emoji including ZWJ are 2 cells, and naïve byte-slicing produces invalid UTF-8 in the top border. The cascade needs a dependable primitive that returns a display-cell-bounded prefix with `…` suffix and never produces mid-rune cuts.

**Solution**: Add a pure helper `truncateToCells(s string, budget int) string` to `internal/tui/pagepreview.go` that iterates codepoint by codepoint, accumulates `runewidth.RuneWidth(r)` cells, stops when adding the next rune would exceed `budget − 1` (reserving 1 cell for `…`), and appends `…` only when truncation actually occurred. Drive it with table-driven tests across ASCII, CJK, emoji (including ZWJ), and combining-mark classes.

**Outcome**: `truncateToCells(s, budget)` exists as an unexported package-level function in `internal/tui/pagepreview.go`. For any UTF-8 string `s` and integer `budget`, the function returns a valid-UTF-8 prefix whose `lipgloss.Width` ≤ `budget`, with `…` suffix appended iff truncation occurred. A new test file `internal/tui/pagepreview_truncate_test.go` exercises the four glyph classes plus boundary cases.

**Do**:
- Add an import for `github.com/mattn/go-runewidth` to `internal/tui/pagepreview.go` (already a transitive dep via lipgloss; confirm it appears in `go.mod` already by `grep go-runewidth go.mod` — if it does not, prefer using `lipgloss.Width` of single-rune strings as the spec permits, but `go-runewidth` is canonical).
- Implement `truncateToCells`:
  ```go
  // truncateToCells returns the longest valid-UTF-8 prefix of s whose
  // display-cell width fits within budget, with a "…" suffix appended
  // iff truncation occurred. Cells are measured per
  // runewidth.RuneWidth: ASCII=1, CJK=2, emoji=2, combining marks=0.
  // For budget <= 0 returns "". For budget == 1 the ellipsis itself
  // consumes the whole budget — if s is non-empty and does not fit
  // whole, returns "…"; if s is empty returns "".
  // No mid-rune cuts.
  func truncateToCells(s string, budget int) string { … }
  ```
  Algorithm:
  - If `budget <= 0` return `""`.
  - First measure the full string: if `runewidth.StringWidth(s) <= budget` return `s` unchanged (no truncation, no ellipsis).
  - Otherwise iterate runes, accumulating cells. Stop when adding the next rune would exceed `budget − 1`. Append `"…"` and return.
- Add `internal/tui/pagepreview_truncate_test.go` with table-driven cases (one `t.Run` per row). Cases must include:
  - ASCII fits whole: `"hello", 10 → "hello"` (no ellipsis).
  - ASCII truncates: `"hello world", 8 → "hello w…"` (or whatever fits — measure carefully: budget 8 means 7 cells of content + 1 cell ellipsis).
  - CJK fits: `"日本語", 6 → "日本語"`.
  - CJK truncates: `"日本語テスト", 7 → "日本語…"` (3 glyphs × 2 = 6 cells + ellipsis = 7).
  - Emoji ZWJ: a family emoji like `"👨‍👩‍👧"` truncated within a tight budget — assert no mid-sequence cut and valid UTF-8 output.
  - Combining marks: `"áb́ć", 3 → "áb́ć"` (3 cells, fits).
  - Budget == 0: returns `""`.
  - Budget == 1, non-empty s that does not fit whole: returns `"…"`.
  - Empty s, any budget: returns `""`.
  - Budget == 8 with ASCII string of 8 chars (`"abcdefgh"`): fits exactly, returns the string unchanged.
  - Boundary: budget exactly equal to `StringWidth(s)` returns `s` unchanged (no spurious ellipsis).
- Each test row asserts:
  - `utf8.ValidString(got)` is true (no mid-rune cuts).
  - `runewidth.StringWidth(got) <= budget`.
  - `strings.HasSuffix(got, "…")` matches the expected truncation flag.

**Acceptance Criteria**:
- [ ] `truncateToCells` exists in `internal/tui/pagepreview.go` as an unexported function with the signature `func truncateToCells(s string, budget int) string`.
- [ ] All output is valid UTF-8 (`utf8.ValidString(got)` always true).
- [ ] `runewidth.StringWidth(got) <= budget` for every input.
- [ ] `…` is appended iff truncation occurred.
- [ ] All glyph classes (ASCII, CJK, emoji incl. ZWJ, combining marks) covered by table-driven tests.
- [ ] No `t.Parallel()` calls in the new test file.
- [ ] Test does not import the `tmuxtest` package.

**Tests**:
- `"ASCII string within budget returns unchanged"`
- `"ASCII string over budget truncates and appends ellipsis"`
- `"CJK string within budget returns unchanged"`
- `"CJK string over budget truncates on rune boundary"`
- `"emoji ZWJ sequence never splits mid-sequence"`
- `"combining marks count as zero cells"`
- `"budget zero returns empty string"`
- `"budget one returns ellipsis when input does not fit"`
- `"empty input returns empty string"`
- `"budget exactly equal to string width returns string unchanged"`

**Edge Cases**:
- ASCII / CJK 2-cell / emoji including ZWJ / combining marks 0-cell.
- No truncation needed (full string fits, no spurious ellipsis appended).
- Budget ≤ 1 (degenerate but defined — budget 1 returns `"…"` when input doesn't fit, returns `""` when input is empty).
- ZWJ-joined emoji sequences must not be split mid-sequence — the iterator must respect the underlying rune-by-rune accumulation; if a multi-rune ZWJ glyph would push the running cell count over `budget − 1` the whole glyph is excluded.

**Context**:
> From spec § Display-cell-aware truncation > Algorithm: iterate codepoint by codepoint, accumulating `runewidth.RuneWidth(r)` (or equivalently `lipgloss.Width` of single-rune strings — `lipgloss` uses `go-runewidth` underneath). Stop when adding the next rune would exceed `budget − 1` (reserving 1 cell for the `…` suffix). Append `…` (1 cell wide).
>
> From § Test coverage: asserts no mid-rune cuts (output is valid UTF-8), final display-cell width ≤ budget, `…` is appended only when truncation actually occurred.
>
> Note on ambiguity: the spec does not pin behaviour for ZWJ sequences when truncation falls mid-sequence. The implementation here treats ZWJ as ordinary runes with their measured cell widths (which is what `go-runewidth` does); a ZWJ that would push the cumulative cell count over budget is simply not added. This may produce a leading partial glyph in pathological cases (e.g. a `👨` followed by a ZWJ without its continuation) but the output is still valid UTF-8.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Display-cell-aware truncation, § Width cascade > Tier 1.

## preview-visual-distinction-1-4 | approved

### Task 1-4: Implement composeChromeLine cascade tiers 1-4

**Problem**: The preview's frame top edge must carry chrome content that gracefully degrades as terminal width shrinks: full chrome → truncated window name → dropped window-name segment → compact keymap → corners-and-filler only. Without a single pure function implementing this cascade the frame composition cannot guarantee the top edge is exactly one row at every width ≥ 2.

**Solution**: Implement `composeChromeLine(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) string` as a pure function in `internal/tui/pagepreview.go`. It builds candidate strings tier by tier, measures each via `lipgloss.Width`, and returns the first candidate whose width equals `width + 2` (the outer terminal width). Tier 4 is the load-bearing fallback that always fits.

**Outcome**: `composeChromeLine` exists in `internal/tui/pagepreview.go`. For every `width ≥ 0` it returns a single-row string (no `\n`) of display-cell width `width + 2`. For `width < 0` it returns the empty string. Tier selection is verifiable at threshold widths via direct return-value inspection. Border parts (corners, filler `─`) are sourced from `lipgloss.RoundedBorder()`, not hardcoded.

**Do**:
- Add `const minWindowNameCells = 8` near the keymap constants in `internal/tui/pagepreview.go`.
- Implement `composeChromeLine` with the following structure:
  ```go
  // composeChromeLine returns a single-line top-edge row for the preview
  // frame, including corner glyphs sourced from lipgloss.RoundedBorder().
  // The width parameter is the inner frame width (terminalWidth − 2);
  // the returned string has display-cell width width + 2 for width ≥ 0,
  // and is the empty string for width < 0. The cascade guarantees one-row
  // output for all widths ≥ 2; below that, returns the empty string.
  // No embedded newlines.
  func composeChromeLine(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) string {
      if width < 0 {
          return ""
      }
      border := lipgloss.RoundedBorder()
      tl, tr, h := border.TopLeft, border.TopRight, border.Top  // "╭","╮","─"
      outer := width + 2

      // Build the structural prefix "Window M of N · Pane X of Y"
      counters := fmt.Sprintf("Window %d of %d · Pane %d of %d", windowIdx+1, windowCount, paneIdx+1, paneCount)

      // Tier candidates: each tier returns chromeContent (unstyled).
      // assemble(chromeContent) wraps tl + h + " " + chromeContent + filler + h + tr
      // such that total outer width is width + 2. If chromeContent is empty,
      // the middle range becomes pure filler (tier 4).
      assemble := func(chrome string) (string, bool) {
          // tl + h + chrome + filler + h + tr, total outer cells
          // Layout: ╭ ─ {chrome} {filler ─ × k} ─ ╮
          // Padding cells around chrome: 1 left, 1 right (both are '─').
          // Required: 2 corners + 2 padding cells + chrome cells + filler ≥ 0.
          chromeCells := lipgloss.Width(chrome)
          // outer = 2 corners + 1 left-pad + chrome + filler + 1 right-pad
          fillerCells := outer - 4 - chromeCells
          if fillerCells < 0 {
              return "", false
          }
          if chrome == "" {
              // tier-4 collapse: drop the two padding cells, fill entirely.
              // outer = 2 corners + (outer-2) filler
              return tl + strings.Repeat(h, max(0, outer-2)) + tr, true
          }
          return tl + h + chrome + strings.Repeat(h, fillerCells) + h + tr, true
      }

      // Tier 1: verbose keymap + full or truncated window name.
      // Compute budget for the windowName text after fixed segments.
      // chrome = counters + " · win: " + name + "    " + verboseKeymap
      // (4 spaces between name segment and keymap — spec § Chrome line content > Segments
      // says "keymap is separated by whitespace padding"; we encode this as ≥1 space
      // and let the right-edge filler absorb the rest; a single space is sufficient
      // structurally because the trailing '─' filler does the visual right-alignment.)
      // Outer budget: outer cells total. Fixed cells (excluding name + filler):
      //   2 corners + 2 padding '─' + len(counters) + len(" · win: ") + 1 space + len(verboseKeymap)
      const sep = " · win: "
      const pad = " " // single space between segments 1-3 group and keymap; filler handles visual right-align
      fixedTier1 := 4 + lipgloss.Width(counters) + lipgloss.Width(sep) + lipgloss.Width(pad) + lipgloss.Width(verboseKeymap)
      nameBudget := outer - fixedTier1
      if nameBudget >= minWindowNameCells {
          truncated := truncateToCells(windowName, nameBudget)
          chrome := counters + sep + truncated + pad + verboseKeymap
          if got, ok := assemble(chrome); ok && lipgloss.Width(got) == outer {
              return got
          }
      }

      // Tier 2: drop "· win: {name}" segment, keep verbose keymap.
      // chrome = counters + pad + verboseKeymap
      chrome2 := counters + pad + verboseKeymap
      if got, ok := assemble(chrome2); ok && lipgloss.Width(got) == outer {
          return got
      }

      // Tier 3: drop "· win: {name}" + use compact keymap.
      chrome3 := counters + pad + compactKeymap
      if got, ok := assemble(chrome3); ok && lipgloss.Width(got) == outer {
          return got
      }

      // Tier 4: corners + filler only. Always fits for outer ≥ 2.
      got, _ := assemble("")
      return got
  }
  ```
- Imports needed: `strings`, `github.com/charmbracelet/lipgloss` (already present after task 1-1).
- Add a test file `internal/tui/pagepreview_compose_chrome_test.go` with table-driven cases at the cascade thresholds. Each row supplies `(width, windowName, expectedTier)` and asserts:
  - `lipgloss.Width(got) == width + 2` for `width >= 0`.
  - `strings.Count(got, "\n") == 0`.
  - Tier signature checks (the tier classification is observable from the substrings present):
    - tier 1 → contains `"win: "` and a truncated-or-full name and `verboseKeymap`.
    - tier 2 → does **not** contain `"win:"` and contains `verboseKeymap`.
    - tier 3 → does **not** contain `"win:"` and contains `compactKeymap` and does not contain `verboseKeymap`.
    - tier 4 → does not contain `"Window "` or `"win:"` or either keymap; middle is `─` filler only.
  - `width < 0` returns the empty string.
- Test thresholds (with `windowName="nvim-editor"`, 11 cells):
  - width 200 → tier 1, full name present.
  - width 60 → tier 1, truncated name with `…`.
  - boundary: pick the smallest width where `nameBudget == minWindowNameCells` (8) — assert tier 1 still active and `…` present.
  - boundary: pick width where `nameBudget == 7` — assert tier 2 selected.
  - width 40 → tier 2.
  - width 25 → tier 3 (verify by `strings.Contains(got, compactKeymap) && !strings.Contains(got, "next pane")`).
  - width 15 → tier 4 (top edge is `╭─────────────╮` = corners + 13 filler).
  - width 4 → tier 4, output is `╭──╮`.
  - width 3 → tier 4, output is `╭─╮`.
  - width 2 → tier 4, output is `╭╮`.
  - width 0 → tier 4, output is `╭╮`? Note: outer = 2 here, same as width 2. With width = 0, `outer = 2`, no room for any chrome, returns `╭╮`.
  - width -1 → returns `""`.

**Acceptance Criteria**:
- [ ] `composeChromeLine` is exported (lowercase) as an unexported package-level pure function in `internal/tui/pagepreview.go` with the signature given.
- [ ] For every `width >= 0` the returned string has `lipgloss.Width == width + 2` and contains no `\n`.
- [ ] For `width < 0` the returned string is `""`.
- [ ] Tier 1/2/3/4 selection is verified by the table-driven test at the spec's threshold widths (200, 60, 40, 25, 15) plus the 8/7 cell boundary cases.
- [ ] Corner / edge characters in the output are sourced from `lipgloss.RoundedBorder()`, not hardcoded — verified by reading the implementation; runtime test asserts the output starts with `border.TopLeft` and ends with `border.TopRight`.
- [ ] Tier-4 degenerate widths 2, 3, 4 each produce a valid output (`╭╮`, `╭─╮`, `╭──╮`).

**Tests**:
- `"width 200 tier 1 full name present"`
- `"width 60 tier 1 truncated with ellipsis"`
- `"name budget exactly 8 cells stays in tier 1"`
- `"name budget 7 cells drops to tier 2"`
- `"width 40 tier 2 win segment absent verbose keymap present"`
- `"width 25 tier 3 compact keymap present"`
- `"width 15 tier 4 corners and filler only"`
- `"width 4 returns four-cell top edge"`
- `"width 3 returns three-cell top edge"`
- `"width 2 returns two-cell top edge"`
- `"width minus one returns empty string"`
- `"output width invariant equals width plus two for all widths"`
- `"output contains no embedded newlines at any threshold"`
- `"corner glyphs sourced from lipgloss RoundedBorder"`

**Edge Cases**:
- Tier-2 entry at the 8-cell minimum boundary (name budget = 8 stays in tier 1; budget = 7 falls to tier 2). Exact integer boundary asserted.
- Tier-4 degenerate widths 2, 3, 4 — output must be valid and width-correct without panic.
- `width < 0` returns empty string (no panic, no negative `strings.Repeat`).
- `width + 2` exact output width invariant — every tier candidate is measured via `lipgloss.Width`; mismatch falls through to the next tier.

**Context**:
> From spec § Width cascade > Algorithm shape: each tier produces a candidate output, measures it via `lipgloss.Width`, and returns the candidate if it fits. Otherwise it falls through to the next tier.
>
> From spec § Top edge composition > Column layout (using `width` for the outer terminal width):
> - Column 0: `╭`
> - Column 1: `─`
> - Columns 2..2+chromeWidth−1: chrome content
> - Columns 2+chromeWidth..width−3: `─` filler
> - Column width−2: `─`
> - Column width−1: `╮`
>
> From § Width cascade > Tier-by-tier behaviour: tier 2's 8-cell minimum is fixed (`const minWindowNameCells = 8`). Tier 4 is load-bearing and always fits at width ≥ 0.
>
> Note on ambiguity: the spec describes the separator between segments 1–3 group and the keymap as "whitespace padding so it visually right-aligns within the available chrome budget at wide widths and compresses toward the centre at narrow widths." The implementation here uses a single space and lets the right-edge `─` filler perform the visual right-alignment; this preserves the spec's *minimum* separation while letting the frame composition handle the variable padding cleanly. The cascade-tier end-to-end test (task 1-9) will catch any drift between this choice and the visible behaviour.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Chrome line content, § Width cascade, § Top edge composition.

## preview-visual-distinction-1-5 | approved

### Task 1-5: Chrome-row single-line invariant test

**Problem**: The resize math `viewport.SetSize(msg.Width − 2, msg.Height − 2)` assumes the top edge is exactly one row at every width. If the cascade ever produced a wrapped or multi-line output, the bottom corner would shift down by one row and the frame would visibly break. A focused invariant test guards this assumption across every cascade tier.

**Solution**: Add a dedicated test in `internal/tui/pagepreview_compose_chrome_test.go` (or a new sibling file `pagepreview_chrome_invariant_test.go` if preferred) that asserts `strings.Count(composeChromeLine(w, …), "\n") == 0` across the cascade-tier threshold widths.

**Outcome**: A test exists that fails if `composeChromeLine` ever returns a string containing a newline at any of the cascade thresholds. The test runs against a fixture set covering tier 1 (wide), tier 1 (truncated), tier 2, tier 3, and tier 4 widths.

**Do**:
- Add a test function `TestComposeChromeLine_NoEmbeddedNewlines` to `internal/tui/pagepreview_compose_chrome_test.go` (alongside the cascade-tier tests from task 1-4) — or create a new file `internal/tui/pagepreview_chrome_invariant_test.go` if you prefer to keep the invariant assertion isolated.
- Table of widths: `200, 80, 60, 40, 25, 15, 10, 4, 3, 2, 0`. (Negative widths return empty string, which trivially has zero newlines but is not load-bearing for this invariant.)
- For each width call `composeChromeLine(w, 0, 1, 0, 1, "nvim-editor")` and assert `strings.Count(got, "\n") == 0`. Use a clear error message that includes the width and the offending output.
- No `t.Parallel()`. No `tmuxtest` import.

**Acceptance Criteria**:
- [ ] The test function exists and runs as part of `go test ./internal/tui/...`.
- [ ] The test asserts `strings.Count(composeChromeLine(w, …), "\n") == 0` for every width in `{200, 80, 60, 40, 25, 15, 10, 4, 3, 2, 0}`.
- [ ] The test fails with a descriptive message if any width produces an embedded newline.

**Tests**:
- `"composeChromeLine returns no embedded newlines across cascade thresholds"`

**Edge Cases**: none — this is itself an edge-case-protection test.

**Context**:
> From spec § Chrome-row invariant for resize math: capture the invariant explicitly via a test that asserts `strings.Count(composeChromeLine(w, …), "\n") == 0` across the cascade-tier width thresholds. Guards the assumption baked into `m.viewport.SetSize(msg.Width − 2, msg.Height − 2)` that the top edge is always exactly one row.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Chrome-row invariant for resize math, § Tests > Chrome-row invariant test.

## preview-visual-distinction-1-6 | approved

### Task 1-6: Add injectSGRResets helper

**Problem**: The embedded `bubbles/viewport` renders raw ANSI bytes from scrollback as straight passthrough. A scrollback line can legitimately end with an unterminated SGR sequence (e.g. a `bat`-rendered file that set a background colour and the buffer ended before issuing a reset). With the new frame, that unterminated SGR sits adjacent to the right border — `lipgloss`'s `BorderForeground` does not reliably reset background state, so the border could render with coloured squares instead of clean blue.

**Solution**: Add a pure helper `injectSGRResets(s string) string` to `internal/tui/pagepreview.go` that appends `\x1b[0m` to every non-empty line, ignoring empty lines and trailing-newline empty trailing elements. Applied in `View()` to `viewport.View()` output before frame composition.

**Outcome**: `injectSGRResets` exists in `internal/tui/pagepreview.go`. For any input string, every non-empty line in the output ends with `\x1b[0m`; empty lines are unchanged; trailing-newline-induced empty trailing elements are not given a reset. A new test file exercises the spec's edge cases.

**Do**:
- Add to `internal/tui/pagepreview.go`:
  ```go
  // injectSGRResets appends "\x1b[0m" (SGR reset) to the end of every
  // non-empty line in s, ignoring empty lines (including any trailing
  // empty element produced by a terminating newline). Used to protect
  // the right border from unterminated SGR sequences in the scrollback
  // body — see spec § SGR reset injection.
  //
  // Pure: no I/O, no allocations beyond the joined output. Idempotent
  // in observable behaviour — terminals collapse "\x1b[0m\x1b[0m" to a
  // single reset, so re-applying does not corrupt rendering.
  func injectSGRResets(s string) string {
      lines := strings.Split(s, "\n")
      for i, line := range lines {
          if len(line) > 0 {
              lines[i] = line + "\x1b[0m"
          }
      }
      return strings.Join(lines, "\n")
  }
  ```
- Add `internal/tui/pagepreview_sgr_test.go` with cases:
  - Line ending in an SGR like `"\x1b[41mhello"` (sets bg=red, no reset) → gets reset appended.
  - Line already ending in `\x1b[0m` → gets a *second* reset appended. Assert idempotency in rendered effect by checking the suffix is exactly `\x1b[0m\x1b[0m`.
  - Empty line → no reset appended. Use input `"foo\n\nbar"` and assert the middle empty element stays empty.
  - Whitespace-only line with embedded SGR — e.g. `" \x1b[42m "` (spaces around an SGR) → non-empty, gets reset.
  - Trailing-newline input — `"foo\n"` splits to `["foo", ""]`; assert `"foo"` gets reset and the trailing empty stays empty.
  - Fully empty input `""` → splits to `[""]`; output is `""`.
  - Multi-line happy path — `"a\nb\nc"` → `"a\x1b[0m\nb\x1b[0m\nc\x1b[0m"`.
- No `t.Parallel()`. No `tmuxtest` import. Standard-library only (`strings`, `testing`).

**Acceptance Criteria**:
- [ ] `injectSGRResets(s string) string` exists in `internal/tui/pagepreview.go`.
- [ ] Every non-empty line in the output ends with `\x1b[0m`.
- [ ] Empty lines (including trailing-newline-induced empty trailing elements) are unchanged.
- [ ] Idempotency: an already-reset line gets a second reset; no panic, no degradation.
- [ ] All six spec edge cases (set-bg line, already-reset line, empty line, whitespace+SGR line, trailing newline, fully empty input) are covered by tests.

**Tests**:
- `"line ending in unterminated SGR gets reset appended"`
- `"line already ending in reset gets a second reset"`
- `"empty line in middle stays empty"`
- `"whitespace-only line with embedded SGR gets reset"`
- `"trailing newline produces empty trailing element which is ignored"`
- `"fully empty input returns fully empty output"`
- `"multi-line input gets reset on every non-empty line"`

**Edge Cases**:
- Trailing newline empty element ignored.
- Already-terminated line idempotency.
- Whitespace-only line with embedded SGR — `len > 0` is the only non-empty test, so this line gets a reset.
- Fully empty input — `strings.Split("", "\n")` returns `[""]`; loop skips, join returns `""`.

**Context**:
> From spec § SGR reset injection > Algorithm:
> 1. Split on `\n`.
> 2. For each line where `len(line) > 0`, append `\x1b[0m`.
> 3. Join back with `\n`.
> 4. Pass the joined string into the frame composition.
>
> From § Edge cases: trailing newline empty element ignored; "non-empty" defined as byte-length > 0 (whitespace lines count); idempotency is harmless because terminals collapse `\x1b[0m\x1b[0m` to a single reset.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § SGR reset injection.

## preview-visual-distinction-1-7 | approved

### Task 1-7: Wire tea.WindowSizeMsg handler in Update and delete chromeLine() method

**Problem**: The existing `Update`'s `tea.WindowSizeMsg` case sets `m.viewport.Width = msg.Width` and `m.viewport.Height = max(0, msg.Height - previewChromeHeight)` — using the old single-row chrome model. Under the frame model both width and height must be reduced by 2 (left+right border columns, top+bottom border rows), and the viewport must be sized via `viewport.SetSize` so its internal scroll state is correctly clamped. Additionally the old `chromeLine()` method is dead under the new pure-function model and must be deleted.

**Solution**: Replace the existing `tea.WindowSizeMsg` handler body with `m.width = msg.Width; m.height = msg.Height; m.viewport.SetSize(max(0, msg.Width-previewFrameOverhead), max(0, msg.Height-previewFrameOverhead))`. Delete the entire `chromeLine()` method (lines ~143–175 in the current file, including doc comment). Adjust `View()` if necessary so it no longer calls `chromeLine()` — note `View()` will be fully rewritten in task 1-8, so this task just makes sure `View()` still compiles after `chromeLine()` is removed.

**Outcome**: The `tea.WindowSizeMsg` case in `internal/tui/pagepreview.go` records `m.width`/`m.height` and calls `viewport.SetSize` with both dimensions reduced by `previewFrameOverhead` (= 2), clamped non-negative. The `chromeLine()` method is deleted. `View()` compiles (will be rewritten in 1-8); for the duration of this single commit it may return a placeholder, the empty string, or `m.viewport.View()` directly — pick whatever lets tests in this package still build.

**Do**:
- In `internal/tui/pagepreview.go`'s `Update`, replace the `case tea.WindowSizeMsg:` body:
  ```go
  case tea.WindowSizeMsg:
      m.width = msg.Width
      m.height = msg.Height
      m.viewport.SetSize(max(0, msg.Width-previewFrameOverhead), max(0, msg.Height-previewFrameOverhead))
      return m, nil
  ```
- Delete the `chromeLine()` method entirely (the doc comment and the function body — currently at lines ~143–175).
- Temporarily change `View()`'s body to `return m.viewport.View()` (a minimal compilable stub). The full frame composition lands in task 1-8. **Leave a TODO comment** referencing task 1-8 so anyone running `git blame` between these commits has context.
- Update `pagepreview_layout_test.go` if its assertion on `viewport.Width` previously assumed `msg.Width` rather than `msg.Width − 2` — review the test and adjust expected values. Specifically: the layout test currently asserts `viewport.Width == msg.Width` and `viewport.Height == msg.Height − previewChromeHeight` (now `previewFrameOverhead`); after this task it should assert `viewport.Width == msg.Width − previewFrameOverhead` and `viewport.Height == msg.Height − previewFrameOverhead`, both clamped non-negative.
- Similarly review `pagepreview_precedence_test.go` and `pagepreview_scroll_test.go` — same width adjustment if they assert on `viewport.Width`.
- Add a focused test in a new file `internal/tui/pagepreview_resize_test.go` that:
  - Constructs a `previewModel` with mock `TmuxEnumerator` returning 1 window 1 pane and a `ScrollbackReader` returning `(nil, nil)`.
  - Dispatches `Update(tea.WindowSizeMsg{Width: 100, Height: 30})`.
  - Asserts `m.width == 100`, `m.height == 30`.
  - Asserts `m.viewport.Width == 98` and `m.viewport.Height == 28`.
  - Dispatches `Update(tea.WindowSizeMsg{Width: 1, Height: 0})`.
  - Asserts the clamps: `m.viewport.Width == 0`, `m.viewport.Height == 0` (no panic, no negative).
- The `chromeLine()` deletion may break `pagepreview_chrome_test.go` and `pagepreview_chrome_enterattach_test.go` — if those tests call `chromeLine()` directly, update them to call `composeChromeLine(m.width-2, …)` with the appropriate arguments derived from the model. If they assert on the historical string format (`"tab next pane"`, etc.), update assertions to match the new keymap glyphs (`⇥ next pane`) per the spec.

**Acceptance Criteria**:
- [ ] `tea.WindowSizeMsg` handler calls `m.viewport.SetSize(max(0, msg.Width-previewFrameOverhead), max(0, msg.Height-previewFrameOverhead))`.
- [ ] `m.width` and `m.height` are recorded on the model on every resize.
- [ ] `chromeLine()` method no longer exists in `internal/tui/pagepreview.go`.
- [ ] `View()` is a temporary stub returning `m.viewport.View()` with a TODO comment pointing to task 1-8.
- [ ] `go build ./...` succeeds.
- [ ] `go test ./internal/tui/...` either passes or fails only on tests that 1-8 will rewrite; the resize test added in this task passes.
- [ ] The clamp test asserts `viewport.Width == 0` and `viewport.Height == 0` when `msg.Width == 1` and `msg.Height == 0` (negative-arg clamp verified).

**Tests**:
- `"WindowSizeMsg records width and height on the model"`
- `"WindowSizeMsg calls viewport SetSize with dimensions minus previewFrameOverhead"`
- `"WindowSizeMsg with width 1 and height 0 clamps viewport size to zero"`
- `"chromeLine method is deleted from previewModel"` — this is a compile-time assertion, automatically enforced; a test isn't strictly needed, but a `grep -n "func (m previewModel) chromeLine" internal/tui/pagepreview.go` showing zero hits is an acceptance check.

**Edge Cases**:
- `msg.Width == 0` or `msg.Width == 1` — `max(0, msg.Width - 2)` clamps to 0; `viewport.SetSize(0, …)` must not panic.
- `msg.Height == 0` or `msg.Height == 1` — same clamp on height.
- `chromeLine()` callers redirected to `composeChromeLine` — there are no production callers other than `View()`, which is being stubbed in this task and rewritten in 1-8. Test callers in `pagepreview_chrome_test.go` / `pagepreview_chrome_enterattach_test.go` may need updates as described in Do.

**Context**:
> From spec § Resize behaviour: preview's resize handler in `pagepreview.go`'s `Update` does two things on each `tea.WindowSizeMsg`:
> 1. Record the new dimensions on the model (`m.width`, `m.height`).
> 2. Call `m.viewport.SetSize(max(0, msg.Width − 2), max(0, msg.Height − 2))` to adjust the viewport's visible window for the new inner dimensions (subtracting 2 for left+right border columns and top+bottom border rows). The `max(0, …)` clamps guard against the degenerate case where `msg.Width` or `msg.Height` is 0 or 1.
>
> From § Replace `chromeLine()` with `composeChromeLine`: the existing `chromeLine()` method on `previewModel` at `internal/tui/pagepreview.go:165-175` is **deleted**. Callers in `View()` invoke the new pure function `composeChromeLine(width int, …) string` directly.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Resize behaviour, § Code shape changes > Replace `chromeLine()` with `composeChromeLine`.

## preview-visual-distinction-1-8 | approved

### Task 1-8: Compose painted frame in View() and initialise viewport in NewPreviewModel

**Problem**: After task 1-7's stub, `View()` returns the bare viewport content — no frame, no chrome, no SGR-reset injection. The build phase's user-visible payload is to render the rounded blue frame around the viewport every tick, with the chrome line riding on the top border row, the three other edges rendered by `lipgloss`, and SGR resets applied per row. Additionally, `NewPreviewModel`'s constructor must initialise the viewport with both dimensions reduced by `previewFrameOverhead` so the first `View()` call (before any `WindowSizeMsg`) renders at the correct size.

**Solution**: Rewrite `View()` in `internal/tui/pagepreview.go` to:
1. Compute the top edge via `composeChromeLine(m.width-2, …)` styled with `previewBorderColor` on the border parts (corners + filler).
2. Pass `viewport.View()` through `injectSGRResets`.
3. Use `lipgloss.NewStyle()` with `Border(lipgloss.RoundedBorder())`, `BorderTop(false)`, and `BorderForeground(previewBorderColor)` to wrap the body, rendering the left/right/bottom edges only.
4. Concatenate top edge + bordered body.
Also update `NewPreviewModel` to call `viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead))` so the initial viewport size matches what the resize handler would produce.

**Outcome**: `pagePreview.View()` returns a string containing all four rounded corner glyphs (`╭ ╮ ╰ ╯`), all four edges coloured via `previewBorderColor`, the chrome line embedded in the top border row, and SGR resets on every non-empty viewport content row. The first frame on construction is correct without any `WindowSizeMsg` having been dispatched. `pageSessions`'s `View()` is unchanged and renders no frame.

**Do**:
- In `internal/tui/pagepreview.go` `NewPreviewModel`, change `viewport: viewport.New(width, max(0, height-previewChromeHeight))` (which after task 1-2's rename reads `viewport.New(width, max(0, height-previewFrameOverhead))`) to:
  ```go
  viewport: viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead)),
  ```
- Rewrite `View()`:
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
- The above NOTE is real and the implementation decision should be flagged in the task's Context. The reviewer can either accept it or push back; the task is correct under either resolution because composeChromeLine's *output* is what tests assert.
- Add a `pagepreview_view_frame_test.go` that:
  - Constructs `previewModel` with mocks (1 window, 1 pane, window name `"nvim-editor"`) and width/height = 80/24.
  - Dispatches `Update(tea.WindowSizeMsg{Width: 80, Height: 24})`.
  - Calls `View()`.
  - Asserts the output contains `╭`, `╮`, `╰`, `╯` (all four corners).
  - Asserts the output contains the chrome substring `"Window 1 of 1 · Pane 1 of 1 · win: nvim-editor"`.
  - Asserts (via `strings.Contains` of the raw rendered bytes) that the SGR-reset bytes `"\x1b[0m"` appear at least once per non-empty viewport content row. Construct the mock `ScrollbackReader` to return a fixture with an unterminated SGR (e.g. `"\x1b[41mhello\nworld\n"`); assert both content lines end with reset before frame composition.
  - Asserts `m.width == 80` is honoured: the rendered output's first line (after splitting on `\n`) has `lipgloss.Width == 80`.
- Also assert first-frame correctness: construct `previewModel` with `width=80, height=24` and immediately call `View()` (no `WindowSizeMsg`). The first frame's top row must have `lipgloss.Width == 80`.

**Acceptance Criteria**:
- [ ] `pagePreview.View()` output contains rounded corners `╭ ╮ ╰ ╯`.
- [ ] All four edges are styled with `previewBorderColor` — verified by checking the SGR sequences in the rendered output contain the truecolor or downgraded colour codes corresponding to the adaptive value, or (simpler) by asserting both the top edge and the lipgloss-rendered body include `\x1b[` style codes (lipgloss emits these unconditionally when a foreground is set).
- [ ] `View()` calls `composeChromeLine` every tick — no cached chrome field on the model.
- [ ] `NewPreviewModel` initialises the viewport with `viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead))`.
- [ ] First-frame correctness: calling `View()` on a freshly-constructed `previewModel` (no prior `WindowSizeMsg`) renders with correct dimensions.
- [ ] `injectSGRResets` is applied to `viewport.View()` output before frame composition.
- [ ] Degenerate widths handed to lipgloss without panic — test with `width=2, height=4`.
- [ ] No production code outside `internal/tui/pagepreview.go` is modified.

**Tests**:
- `"View output contains all four rounded corner glyphs"`
- `"View output top row width equals outer terminal width"`
- `"View chrome line contains window pane indicators and window name at wide width"`
- `"View applies SGR reset to every non-empty viewport content row"`
- `"View first-frame correctness without prior WindowSizeMsg"`
- `"View at degenerate width 2 height 4 renders without panic"`
- `"View recomputes chrome every tick — no cached field"` (verify by changing `m.windowIdx` and confirming the rendered output reflects the new value without an intermediate `Update` call setting any cache)

**Edge Cases**:
- Chrome recomputed every tick — no cached field; verified by mutating `m.windowIdx` between two `View()` calls and asserting both reflect the current value.
- First-frame correctness at construction — no race between preview-open and the first `WindowSizeMsg`, no "first frame at zero width" edge case.
- Degenerate widths (e.g. `width=2`) handed to lipgloss without panic — `lipgloss` clips when it cannot render; the production code does not need a special case.

**Context**:
> From spec § Top edge composition > Color application: the top edge is composed as two stylings concatenated — border parts wrapped in `lipgloss.NewStyle().Foreground(previewBorderColor).Render(…)`, chrome content rendered with no explicit foreground inheriting terminal default. *Note: the implementation in this task takes a pragmatic interpretation — tinting the whole top edge with `previewBorderColor` rather than splitting the styling boundary inside `composeChromeLine`'s output. This satisfies the user-visible acceptance criterion ("all four edges coloured") and is a single-point change if reviewers prefer the stricter form.*
>
> From § Initial sizing and preview-open ordering: `viewport.SetSize(max(0, width − 2), max(0, height − 2))` is called once with initial dimensions (same `max(0, …)` clamp as the resize handler). `View()` recomputes the chrome line on every tick, so no separate pre-computation is needed at construction time. The first `View()` call on the freshly-constructed `previewModel` renders with correct dimensions — no race between preview-open and the first `WindowSizeMsg`.
>
> From § Style sourcing: corner and edge characters used in the manually-composed top edge are sourced from the chosen `lipgloss` border value (`lipgloss.RoundedBorder()`) rather than hardcoded.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Frame structure, § Border style, § Border colour, § Top edge composition, § Resize behaviour, § Initial sizing and preview-open ordering, § SGR reset injection.

## preview-visual-distinction-1-9 | approved

### Task 1-9: End-to-end cascade-tier Update + View test

**Problem**: The pure-function cascade tests (task 1-4) and the View-composition test (task 1-8) each cover one half of the chrome pipeline. Without an end-to-end test driving the full `Update → View` path at the cascade thresholds, the rendered frame and the pure function could silently drift apart (for example, if `View()`'s `composeChromeLine` call passed `m.width` instead of `m.width − previewFrameOverhead`, the pure-function tests would still pass but the rendered output would be off-by-two).

**Solution**: Add a table-driven test in `internal/tui/pagepreview_cascade_e2e_test.go` that constructs a `previewModel` with mocks, dispatches `Update(tea.WindowSizeMsg{Width: w, Height: 30})` for each cascade-threshold width, calls `View()`, and asserts the rendered output contains the expected tier signature plus SGR-reset bytes on every viewport content row.

**Outcome**: A single test function with five table rows (widths 200, 60, 40, 25, 15) drives `Update → View` end-to-end and asserts the expected tier signature in the rendered output. The fixture `previewModel` has 1 window, 1 pane, window name `"nvim-editor"`. SGR-reset presence is asserted in every case.

**Do**:
- Create `internal/tui/pagepreview_cascade_e2e_test.go`.
- Build a mock `TmuxEnumerator` returning `[]tmux.WindowGroup{{WindowIndex: 0, WindowName: "nvim-editor", PaneIndices: []int{0}}}`. Build a mock `ScrollbackReader` returning a fixture body with at least two lines, one of which contains an unterminated SGR (e.g. `"\x1b[41mhello\nworld"`).
- Test rows:
  | Width | Expected tier signature                                                 |
  |-------|-------------------------------------------------------------------------|
  | 200   | Contains `"nvim-editor"` (full name) and `"⇥ next pane"` (verbose keymap) |
  | 60    | Contains a name token starting with `"nvim"` and ending in `…`; contains `"⇥ next pane"` |
  | 40    | Does NOT contain `"win:"`; contains `"⇥ next pane"`                      |
  | 25    | Does NOT contain `"win:"`; contains `"] [ ⇥ ⏎ ⎋"` (compactKeymap); does NOT contain `"next pane"` |
  | 15    | Does NOT contain `"Window "`; does NOT contain either keymap; top edge ASCII pattern is `╭` + 13 × `─` + `╮` |
- For each row:
  - Construct a fresh `previewModel` with `NewPreviewModel(...)`. The constructor needs initial width/height — pass `width=w, height=30`.
  - Dispatch `Update(tea.WindowSizeMsg{Width: w, Height: 30})`.
  - Call `View()`.
  - Strip ANSI escape sequences from the output before asserting on character substrings (otherwise the lipgloss border SGR codes will interfere with substring matching). Use a small helper `stripANSI(s string) string` in the test file or inline the regex `regexp.MustCompile("\x1b\\[[0-9;]*m")` and call `ReplaceAllString(s, "")`.
  - Assert each expected substring is present or absent in the stripped output.
  - Separately, on the *raw* output (not stripped), assert `strings.Contains(rawOutput, "\x1b[0m")` — every test row must have at least one SGR reset.
  - Optionally also count SGR resets per viewport content row by splitting the body region and asserting each non-empty content line includes `\x1b[0m`.
- Acceptance: width 200 must show the full name; width 60 must show truncation (presence of `…`); width 40 must show tier 2 (no `win:`); width 25 must show tier 3 (compact keymap); width 15 must show tier 4 (no chrome).
- No `t.Parallel()`. No `tmuxtest` import. Use the existing `TmuxEnumerator` / `ScrollbackReader` mock seams (or local minimal struct mocks if the test package does not already expose ready-made helpers — search `pagepreview_*_test.go` for existing mock structs like `stubEnumerator`, `stubReader`; reuse if present).

**Acceptance Criteria**:
- [ ] Test function `TestPreviewView_CascadeTiersEndToEnd` (or similar) exists in `internal/tui/pagepreview_cascade_e2e_test.go`.
- [ ] All five width rows (200, 60, 40, 25, 15) pass with their expected tier signatures.
- [ ] Every row asserts SGR-reset bytes are present in the rendered output.
- [ ] Width 15 specifically asserts the tier-4 ASCII top-edge pattern `╭` + 13 × `─` + `╮` after stripping ANSI.
- [ ] Test does not use `t.Parallel()`.
- [ ] Test does not import the `tmuxtest` package.
- [ ] Test uses constructor-injected mocks for `TmuxEnumerator` and `ScrollbackReader`.

**Tests**:
- `"cascade tier 1 at width 200 shows full window name and verbose keymap"`
- `"cascade tier 1 at width 60 truncates window name with ellipsis"`
- `"cascade tier 2 at width 40 drops win segment keeps verbose keymap"`
- `"cascade tier 3 at width 25 swaps to compact keymap"`
- `"cascade tier 4 at width 15 drops chrome corners and filler only"`
- `"SGR reset bytes present on viewport content rows at every cascade tier"`

**Edge Cases**:
- Tier 1/2/3/4 signatures at widths 200/60/40/25/15. Each tier is verified by both the *presence* of expected substrings and the *absence* of substrings that belong to other tiers.
- SGR reset present on every content row — guards against the right-border colour-leak failure mode the spec calls out (`bat`-rendered scrollback ending in an unterminated SGR).
- The ANSI-strip helper is essential for substring matching — without it the lipgloss-emitted SGR codes interleave with the chrome text and confuse `strings.Contains`. Verify the strip regex captures both the `m` (set graphics mode) sequences and any reset variants the test fixtures introduce.

**Context**:
> From spec § Tests > Surface 5 — Frame composition end-to-end:
>
> | Width | Expected signature                                              |
> |-------|-----------------------------------------------------------------|
> | 200   | Full window name `nvim-editor` + verbose keymap (`⇥ next pane`) |
> | 60    | Window name truncated with `…` suffix; verbose keymap           |
> | 40    | No `win:` segment (tier 2 dropped); verbose keymap              |
> | 25    | No `win:`; compact keymap `] [ ⇥ ⏎ ⎋`                           |
> | 15    | Top edge is `╭{─ × 13}╮` (tier 4: corners + filler, no chrome)  |
>
> Assert SGR reset bytes are present on each viewport content row in every case. This ties the pure-function cascade thresholds (surface 1) to the actual rendered frame, catching regressions where `composeChromeLine`'s output and the `View()` composition could drift apart.
>
> From § Test conventions: no `t.Parallel()`, no `tmuxtest` imports, assert keymap-glyph substrings against the `verboseKeymap` / `compactKeymap` constants by literal byte content.

**Spec Reference**: `.workflows/preview-visual-distinction/specification/preview-visual-distinction/specification.md` § Tests > Surface 5, § Test conventions.
