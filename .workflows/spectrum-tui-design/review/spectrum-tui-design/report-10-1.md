TASK: spectrum-tui-design-10-1 — Extract the shared pad-right geometry behind a fill-style parameter (padRightWithStyle), keeping headerPadRight / noticeBandPadRight as thin delegating wrappers.

ACCEPTANCE CRITERIA:
- A single `padRightWithStyle(seg string, segWidth, w int, fill lipgloss.Style) string` helper exists in internal/tui and contains the only copy of the return-if-full-else-join-styled-pad geometry.
- `headerPadRight` and `noticeBandPadRight` are thin wrappers that each only supply their fill style and delegate to `padRightWithStyle`; signatures unchanged.
- No call site of either wrapper is modified.
- `go build ./...` succeeds and `go test ./internal/tui/...` passes (header + notice-band behaviour unchanged).
- Rendered output of header band and notice band byte-identical to before for all width/mode/colourless/tint combos.

STATUS: Complete

SPEC CONTEXT: §1 (owned canvas) pins the "painted on every cell" contract — every cell must carry the canvas/tint background with no edge-bleed island. The right-pad geometry exists precisely to satisfy this: a sub-w segment is joined with a fill-styled pad of exactly w-segWidth cells so the trailing region carries the supplied fill. The header pads with the owned canvas (headerCanvasBg); the notice band pads with the band's tint (noticeBandTintStyle). This is a pure DRY chore — no behavioural contract changes, output must be byte-identical.

IMPLEMENTATION:
- Status: Implemented (clean, matches the task prescription exactly)
- Location:
  - internal/tui/header.go:225-231 — `padRightWithStyle` core: `if segWidth >= w { return seg }; pad := fill.Render(strings.Repeat(" ", w-segWidth)); return lipgloss.JoinHorizontal(lipgloss.Top, seg, pad)`. Exactly the prescribed shared geometry, owned in one place.
  - internal/tui/header.go:238-240 — `headerPadRight` one-line delegation supplying `headerCanvasBg(mode, colourless)`. Signature unchanged: `(seg string, segWidth, w int, mode theme.Mode, colourless bool) string`.
  - internal/tui/notice_band.go:330-332 — `noticeBandPadRight` one-line delegation supplying `noticeBandTintStyle(tint, mode, colourless)`. Signature unchanged: `(seg string, segWidth, w int, tint theme.Token, mode theme.Mode, colourless bool) string`.
- Notes:
  - No call site touched. All five `headerPadRight` callers (section_header.go:97,173; panel.go:133; header.go:203,212; footer.go:173) and all three `noticeBandPadRight` callers (notice_band.go:266,303) retain their terse mode/colourless(/tint)-bound calls.
  - Unrelated functions untouched and confirmed genuinely distinct: `padTo` (session_item.go:472) is an unstyled `s + spaces(n-w)` pad with no fill style; `padLineToCanvasWidth` (model.go:3761) is a `TrimRight`-then-canvas-styled pad — a different (trim-first) shape. Neither overlaps the extracted geometry; correctly out of scope.
  - Imports satisfied: both files retain multiple `strings.`/`lipgloss.` uses (header 4/14, notice_band 5/15), so no import dropped or newly required. Fill-style helpers `headerCanvasBg` (header.go:99) and `noticeBandTintStyle` (notice_band.go:137) exist and are passed through unchanged.
  - Doc comments updated accurately: `padRightWithStyle` documents the shared geometry and the wrapper relationship; both wrappers now state they "bind their fill" and "delegate the pad geometry to padRightWithStyle". The notice_band comment's historic "mirrors headerPadRight" note is correctly reworded to "the same core headerPadRight routes through".

TESTS:
- Status: Adequate
- Coverage (pad_right_consolidation_test.go):
  - TestPadRightWithStyle_ReturnsSegUnchangedWhenFull — pins the guard clause (segWidth == w and segWidth > w both return seg verbatim) for a canvas fill AND a tint fill, across Dark/Light × colourless. Covers the return-if-full branch.
  - TestPadRightWithStyle_JoinsStyledPadOfCorrectWidth — pins the fill branch: byte-identical to a verbatim `JoinHorizontal` of a fill-rendered `strings.Repeat(" ", w-segWidth)`, AND asserts `lipgloss.Width(got) == w` (the pad-to-exactly-w invariant), for canvas + tint × modes × colourless.
  - TestHeaderPadRight_DelegatesToPadRightWithStyle / TestNoticeBandPadRight_DelegatesToPadRightWithStyle — the golden-preservation tests: `preHeaderPadRight`/`preNoticeBandPadRight` reproduce the ORIGINAL inline bodies verbatim, and the wrappers are asserted byte-identical across width set {0, segWidth-1, segWidth, segWidth+1, 40, 80} × modes × colourless (notice-band also × two tints BgSelection/BgWarning). This directly proves the "byte-identical for all width/mode/colourless/tint combos" acceptance criterion at the unit level.
- Notes:
  - The verbatim pre-refactor reproductions are the correct technique for a consolidation gate — they make the test independent of the new implementation, so it fails if delegation drifts. Matches the *_consolidation_test.go convention noted in the task.
  - Width coverage includes the boundary triple (segWidth-1 / segWidth / segWidth+1) and w==0, exercising both branches at the edge. Not over-tested: no redundant assertions, each test pins a distinct facet (guard, fill width, header golden, notice golden).
  - Correctly carries the no-t.Parallel() contract (documented in-file; consistent with the cmd-package mutable-state convention and the shared style helpers).

CODE QUALITY:
- Project conventions: Followed. Idiomatic Go; same-package unexported helper; no new interface needed (correct — this is pure geometry, not an external dependency). No t.Parallel(). lipgloss.Style passed by value (idiomatic for lipgloss). Naming (`padRightWithStyle`, `fill`) is clear and role-based.
- SOLID principles: Good. Single-responsibility extraction; the wrappers now do exactly one thing (bind a fill) and the core does exactly one thing (the pad geometry).
- Complexity: Low. Core is a 3-line function with one branch; wrappers are one line each.
- Modern idioms: Yes. Nothing to modernise.
- Readability: Good. Doc comments are precise and explain the why (no terminal-bg island / edge-bleed) not just the what.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. The extraction is exactly as prescribed, fully tested with verbatim golden preservation, and introduces no drift. No actionable change to propose.
