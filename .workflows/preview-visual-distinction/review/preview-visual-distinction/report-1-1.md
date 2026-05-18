TASK: Add Keymap Glyph Constants and Adaptive Border Color (preview-visual-distinction-1-1)

ACCEPTANCE CRITERIA:
- `verboseKeymap` and `compactKeymap` exist as package-level `const` declarations with exact spec byte content.
- `previewBorderColor` exists as a package-level `var` `lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}`.
- A test asserts both constants by literal byte equality.
- `go build ./...` succeeds.
- No other production code modified (within task scope).

STATUS: Complete

SPEC CONTEXT:
Spec § Keymap glyphs > Constants pins the verbose form (interpunct-separated segments) and compact form (single-space separated, 9 display cells). § Border colour and § Style sourcing name `previewBorderColor` (role over hue), AdaptiveColor with specified hex codes, applied to all four edges.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/pagepreview.go:23-26` — grouped `const` block with `verboseKeymap` and `compactKeymap`.
  - `internal/tui/pagepreview.go:42` — `var previewBorderColor = lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}`.
- Notes: Byte sequences match spec exactly. Doc comments cite the spec sections.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/pagepreview_keymap_constants_test.go` — 4 tests:
  1. `TestVerboseKeymapExactByteContent`
  2. `TestCompactKeymapExactByteContent`
  3. `TestCompactKeymapSingleSpaceSeparatedNoInterpuncts`
  4. `TestPreviewBorderColorHexCodes`
- Notes: No `t.Parallel()`, no `tmuxtest`. Not over-tested; not under-tested.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — pure declarations, single responsibility.
- Complexity: Trivial.
- Modern idioms: Yes — grouped `const`, named struct literal for AdaptiveColor.
- Readability: Good — doc comments explain role and spec provenance.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] All four constant-pinning tests share one file; could be split into keymap- and border-color-specific files for clarity.
