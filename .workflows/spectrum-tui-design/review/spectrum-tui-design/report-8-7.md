TASK: spectrum-tui-design-8-7 — Extract the right-anchored footer row assembler shared by footer.go and filter_footer.go (DRY consolidation chore)

ACCEPTANCE CRITERIA:
- The fit test, `headerPadRight` narrow-degrade, and flex-spacer join exist in exactly one assembler (`assembleRightAnchoredRow`); neither `footerKeyRow` nor `filterFooterRow` re-implements them.
- The left-cluster fitting/ellipsis logic (`fitLeftCluster`) remains footer.go-specific (not merged).
- Both the standard and filter footers render byte-identically to current output at wide widths and at/below the narrow-degrade boundary.
- Tests: existing footer/filter-footer render tests pass byte-identical; a test exercises the narrow-degrade boundary (`leftWidth+1+rightWidth > w`) for both footers through the shared assembler.

STATUS: Complete

SPEC CONTEXT:
- §3.4 (footer): a single condensed key row above a 1px `border.footer` top rule — Core keys as a dot-separated left cluster, right-aligned `? help` anchor, key glyphs accent.blue, labels text.detail, `?` glyph accent.violet. §2.7 narrow-degrade: lower-priority entries drop on one line, never wrap.
- §7.1 (filter footer): two contextual filter footers that REPLACE the §3.4 footer while filtering; they explicitly reuse the §3.4 footer machinery (the 1px rule, dot separator, canvas flex spacer, right-aligned `? help` anchor) so chrome stays byte-consistent — only entries/per-entry colours differ. This shared geometry is exactly what the task consolidates.
- This is a pure DRY chore inside the "Reskin, not rebuild" frame: provably cosmetic, behaviour-preserving.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/footer.go:160-179 — new `assembleRightAnchoredRow(left, leftWidth, rightSeg, rightWidth, w, mode, colourless)`: owns the fit test (`rightSeg=="" || leftWidth+1+rightWidth > w`), the `headerPadRight` narrow-degrade, and the canvas flex-spacer join (`spacerWidth := w-leftWidth-rightWidth` → `JoinHorizontal`).
  - internal/tui/footer.go:140-158 — `footerKeyRow` renders its own left cluster via `fitLeftCluster`, resolves the shared `? help` anchor (accent.violet), then calls the assembler.
  - internal/tui/filter_footer.go:152-168 — `filterFooterRow` renders its own left cluster via `renderFilterCluster`, resolves the SAME `? help` anchor from `sessionsKeymap()`, then calls the assembler.
- Notes:
  - Verified via `git show 75000374`: the extracted body is byte-for-byte the original duplicated logic from both row functions (verbatim move, no behavioural edit). `filter_footer.go` dropped its now-unused `strings` import.
  - Sole-ownership confirmed: `grep` for `leftWidth+1+rightWidth` / the footer spacer join across `internal/tui/*.go` (non-test) returns only `assembleRightAnchoredRow`. Other `spacerWidth` sites (header.go, section_header.go, edit_modal.go, help_modal.go) are distinct right-anchor layouts with their own fit semantics — NOT the footer degrade rule, correctly out of scope.
  - `fitLeftCluster` (footer.go:202-256) ellipsis logic is untouched and stays footer.go-specific — correctly NOT merged into the assembler. The assembler's doc comment explicitly states callers render their own left cluster.
  - The `rightSeg==""` arm preserves the original "no right entry → pad left" branch from both functions.

TESTS:
- Status: Adequate
- Coverage (internal/tui/right_anchored_row_test.go, package tui, no t.Parallel — conforms to cmd/tui mutable-state rule):
  - TestAssembleRightAnchoredRow_WideEmitsClusterSpacerAnchor — wide path: row exactly w wide, leads with cluster, ends flush with anchor, spacer > 1 cell.
  - TestAssembleRightAnchoredRow_NarrowDegradePadsLeftAndReturns — boundary `leftWidth+1+rightWidth > w`: result == `headerPadRight(left, leftWidth, w, ...)` exactly (byte-identical degrade) and drops `? help`. Setup includes a guard asserting the chosen width is actually at/below the boundary.
  - TestAssembleRightAnchoredRow_NoRightAnchorPadsLeft — `rightSeg==""` arm pads-left regardless of fit.
  - TestFooters_RouteThroughSharedAssembler_NarrowDegradeIdentical — the load-bearing one: drives all three real clusters (standard `renderFooterCluster`, `filtering`, `applied`) through the assembler at the boundary and asserts each equals `headerPadRight(...)` and drops the anchor; plus an end-to-end tiny-width (6) render of `renderSessionsFooter`/`renderFilteringFooter`/`renderFilterAppliedFooter` asserting all three drop `? help`. This directly satisfies the "boundary test through the shared assembler for both footers" acceptance criterion.
- Notes:
  - Wide-width byte-identity is covered by the UNCHANGED existing render tests (footer_test.go: SingleRowCoreKeysWithRightAlignedHelp, TokenColours, PaintsCanvasNoEdgeBleed; filtering_reskin_test.go: InputActive/ListActiveFooter + colours), which still pass against the same render path — a pure-extraction refactor cannot alter their output. The commit additionally proves byte-identity across 288 renders (6 entry points × 12 widths × dark/light × colourless). Not over-tested: the new file pins only the newly-shared assembler contract; it does not duplicate the per-footer entry/colour assertions that already live in the footer/filtering test files.
  - The test file's NOTE-on-scope comment correctly documents that filter footers have no left-cluster fitting (out of scope) and the test pins only the assembler-owned degrade — honest scoping, not a gap.

CODE QUALITY:
- Project conventions: Followed. Package-level test naming, no `t.Parallel()`, lipgloss/theme idioms consistent with sibling renderers. golangci-lint reported 0 issues (commit message).
- SOLID principles: Good — SRP improved (single owner for the right-anchor geometry); the two callers now depend on one assembler. DRY served exactly as intended.
- Complexity: Low — the assembler is a guard-clause + a three-line join; cyclomatic complexity trivial.
- Modern idioms: Yes — idiomatic Go, guard-clause early return, no premature abstraction.
- Readability: Good — the assembler's doc comment is thorough and states the fit-test predicate, the degrade, the sole-ownership intent, and the explicit "callers render their own left cluster / fitLeftCluster stays out" boundary. Both call sites carry a one-line comment pointing at the shared assembler.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
