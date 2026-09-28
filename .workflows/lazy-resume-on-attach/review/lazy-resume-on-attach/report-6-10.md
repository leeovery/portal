TASK: 6-10 — The Pending Dot's Contrast on the Selected Row Is Guarded (tick-21a011)

ACCEPTANCE CRITERIA:
- Rule: the contrast guard holds `accent.attention` on `bg.selection` to 3.0 for every embedded built-in, the pairing carrying its own floor rather than the 4.5 text floor — a built-in measuring below 3.0 on that pairing fails, the failure naming the theme and `accent.attention on bg.selection`
- Every shipped built-in clears the new pairing with no `.theme` file changed — measured today `tokyo-night` 7.36:1, `nord` 5.52:1, `tokyo-night-day` 3.64:1 (`#9A5200` on `#D0C6F0`)
- Rule: the five pairings the guard already holds — `text.on-selection`, `text.secondary`, `text.tertiary` and `state.positive` on `bg.selection`, `text.on-attention` on `bg.attention` — stay at 4.5; giving the new pairing its own floor lowers none of them
- Rendered under any theme, the contrast swatch's `bg.selection` band shows a bare `●` in `state.positive` and a `●` in `accent.attention`, both on the selection tint and packed as the picker's row packs them (attached, then pending, adjacent); the word `attached` appears nowhere on the swatch
- The swatch's selection-band caption names `accent.attention`, in the band's left-to-right order after `state.positive`
- `docs/theming.md`'s `accent.attention` row names the pending dot and tells an author it renders on both the canvas and the selected row

STATUS: complete

SPEC CONTEXT: Spec section 8.3 drops the word `attached` from the session row and adds a second dot in `accent.attention` for a session holding a waiting pane. The dots pack hard right in a fixed order (attached, then pending) with no reserved lanes, and NO_COLOR renders them as `A`/`P`. Because the pending dot now draws on the cursor row over `bg.selection`, that fg-on-tint pairing needs the same guard, swatch and documentation coverage the other selected-row pairings already have. The spec states no contrast floor itself. The task takes the 3.0 non-text floor from the project's own precedent: `accent.attention` as a bar against the canvas is already held to `floorLargeUI` (`internal/theme/contrast_test.go:138`).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/theme/contrast_test.go:220-236 — `tintPairing` gains a `floor` field. `foregroundOnTintPairings` gives the five existing pairings `floorNormal` (4.5) and adds `{"accent.attention", "bg.selection", …, floorLargeUI}` at :233.
  - internal/theme/contrast_test.go:188-197 — `TestForegroundOnTintPairings` asserts each pairing at its own `pair.floor`. The subtest name and the failure leg both read `<slug> accent.attention on bg.selection`, so a failure names the theme and the pairing.
  - internal/theme/contrast_test.go:83 — `accent.attention` against the canvas stays at `floorNormal`, as the task requires.
  - internal/capture/swatch.go:105-111 — `swatchAttached` is gone and `swatchIndicatorDot = "●"` replaces it.
  - internal/capture/swatch.go:180-193 — `selectionBand` renders a `state.positive` `●` immediately followed by an `accent.attention` `●`, both on the `bg.selection` tint. This matches the picker's `indicatorCluster` (internal/tui/session_item.go:341-350), which concatenates attached then pending with no gap and no bold.
  - internal/capture/swatch.go:131 — the caption now ends `state.positive · accent.attention`, which matches the band's left-to-right order.
  - docs/theming.md:73 — the `accent.attention` row names the `●` pending dot. It tells authors that the dot renders on both the canvas and the selected row, so the value must be tuned against `bg.selection` as well as `canvas`.
- No file under internal/theme/builtins/ is in the change-set.
- Hand check of the WCAG ratio for tokyo-night-day `#9A5200` on `#D0C6F0`: about 3.63:1, clear of 3.0. Nord `#EBCB8B` on `#434C5E` came to about 5.6:1. tokyo-night `#FF9E64` on `#28243a` is well above the floor. Every built-in clears the new pairing unchanged.
- Notes: none.

TESTS:
- Status: Adequate
- Coverage:
  - `TestForegroundOnTintPairings` (contrast_test.go:188-197) runs the new pairing against every embedded built-in at 3.0. A built-in below 3.0 fails with the theme slug and `accent.attention on bg.selection` in both the subtest name and the error.
  - `TestForegroundOnTintPairingFloors` (contrast_test.go:201-218) pins the floor of every pairing: the five existing ones at `floorNormal` and the new one at `floorLargeUI`. It is the test that catches a lowered text floor. Without it, lowering a text pairing to 3.0 would still pass every built-in, because their margins are wide.
  - `TestSwatchCoversForegroundOnTintPairings` (swatch_test.go:216-264) asserts four things:
    - the adjacent `state.positive` `●` + `accent.attention` `●` span on `bg.selection`, whose byte order fails if the dots swap;
    - a caption naming `accent.attention`;
    - the caption sequence `state.positive · accent.attention`;
    - no `attached` anywhere in the rendered swatch.
  - `TestSwatchBandsCoverEveryPinnedTint` still holds the band at `bandWidth`. The content is 41 cells, under 56.
- Notes: The `state.positive` and `accent.attention` table entries in the swatch test share the same `want` span. The shared span is what encodes the adjacent packing, and the two entries differ in the caption label they check, so this is not wasted assertion.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel`, and token names are literals as the swatch's own comment states. The comments added in the change (contrast_test.go:199-200 and swatch.go:130 and :187-188) are true against the code and cite no process artifacts.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
