TASK: lazy-resume-on-attach-6-7 — The Help Modal Indicator Legend

ACCEPTANCE CRITERIA:
- The Sessions help modal renders a legend row for each indicator, below the key rows, separated by the panel's own divider.
- Each legend label says what its indicator means and names no particular tool: the rendered legend rows carry only the indicator forms the row's own renderer produces and the labels this task declares, and no other alphabetic text.
- The Projects help body and the preview help body are byte-identical to their pre-change renders, in every built-in theme and in colourless mode.
- Under `NO_COLOR` the legend names the letters `A` and `P` — the same forms the row renders — rather than two indistinguishable dots.
- The legend's indicators come from the delegate's own renderer, so a change to the row's glyph or token moves the legend with it.
- `sessionsKeymap()` is unchanged, no new key is advertised on any footer, and the descriptor-to-dispatch guard passes without being widened or exempted.
- The Sessions help panel with the legend is no taller than the content region at the smallest terminal the existing help-modal suites render it at.
- The `?` self-entry is still skipped from the body, exactly as today.
- No raw hex appears at any call site — the colour-literal guard passes with no exemption added.

STATUS: complete

SPEC CONTEXT: The session row drops the word `attached` and carries up to two bare indicators packed right in a fixed order (attached in state.positive, then pending in accent.attention), rendered as `A`/`P` letters under NO_COLOR so state stays glyph-backed. Because a bare indicator carries no meaning, the help modal gains a legend for both in the same change, worded tool-agnostically (Portal's resume machinery runs whatever command a registration holds). The help modal is descriptor-driven, and the legend is a separate block, not a keymap entry; the dispatch guard is neither widened nor exempted.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/help_modal.go:20-23 — label constants "Session attached" / "Resume pending"
  - internal/tui/help_modal.go:25-29 — helpLegendEntry, deliberately not a keymapEntry
  - internal/tui/help_modal.go:34-40 — sessionsIndicatorLegend builds each indicator through SessionDelegate.indicatorCluster (internal/tui/session_item.go:341), the same renderer the row calls at internal/tui/session_item.go:321, in attached-then-pending order
  - internal/tui/help_modal.go:42-59 — renderHelpModalContent takes the legend; with none it builds exactly the old {title}/{body} compartments, otherwise it appends a third compartment through renderJoinedPanel, so the divider is the frame's own
  - internal/tui/help_modal.go:63-74 — legend rows go through keyColumnRow with an empty key style. lipgloss v2 returns a props-less style's input unchanged (apart from tab conversion), so the delegate's bytes come through without being re-styled
  - internal/tui/modal.go:25-28 — renderHelpModalOnClearedCanvas passes the legend through
  - internal/tui/model.go:3431 (Sessions, legend passed), internal/tui/model.go:3344 (Projects, nil), internal/tui/pagepreview.go:420 (preview, nil)
- Notes: With a nil legend, the only change to the shared path is swapping the width loop's `if w > contentWidth` for `max`, which gives the same result. Projects and preview therefore go through the pre-change composition byte for byte. keymap.go, keymap_dispatch_guard_test.go and the colour-literal guard are untouched across the feature's commits. The one edit to keymap_dispatch_guard_theme_test.go (task 6-6) is an unrelated applySessions signature update. No hex is added. Height by reading: the full Sessions keymap has 13 body rows after the `?` skip, so the panel is 1+1+1+13+1+2+1 = 20 rows. That fits both the 28-row content region of the help suites' 90x30 fixture and the 22-row region of the smallest 90x24 render elsewhere (canvas_fill_test). The wording follows the modal's register: sentence case, no trailing period, no tool named.

TESTS:
- Status: Adequate
- Coverage: internal/tui/help_modal_legend_test.go has one subtest per planned test name:
  - Legend presence: two rows in a third compartment after exactly two dividers, each row's text, and placement below the key rows.
  - Tool-agnostic check: no alphabetic text beyond the indicator and the declared label, across every built-in theme plus colourless, with sentence case checked.
  - Projects and preview byte-identity against a reconstruction of the pre-change composition, across every theme plus colourless. The Projects check also covers the full placed view, and the preview check covers the composited overlay.
  - NO_COLOR rows lead with `A` and `P` and carry no dot.
  - Legend bytes equal the delegate's indicatorCluster output and appear in the rendered row.
  - No keymap entry or footer advertisement.
  - Panel height ≤ content region in dark and light at the help suites' 90x30 fixture.
  - `?` self-entry absent.
  The existing frame and preview suites were updated only to pass nil at the new parameter.
- Notes: No redundant or bloated cases. The renderer-identity subtest compares against a live call to the delegate, so a legend given its own glyph or token rule would fail as soon as the row's glyph or token moved.

CODE QUALITY:
- Project conventions: Followed (token-only styling, no process-artifact comments, no t.Parallel, colourless carve-out honoured)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (slices.Concat, built-in max, strings.SplitSeq in tests)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
