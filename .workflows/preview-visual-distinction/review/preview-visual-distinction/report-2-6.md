TASK: Collapse composeChromeLine to a One-Liner Over composeChromeLineParts (preview-visual-distinction-2-6)

ACCEPTANCE CRITERIA:
- Preserve composeChromeLine's exported (package-internal) signature
- width<0 guard remains observable (empty string return)
- chromeLineForTest still compiles
- composeChromeLine becomes a thin delegate to composeChromeLineParts

STATUS: Complete

SPEC CONTEXT: composeChromeLine and composeChromeLineParts must share a single tier-selection cascade (specification.md § Width cascade); the parts variant exists so View() can wrap border parts in BorderForeground while chrome content stays unstyled (§ Top edge composition > Color application). composeChromeLine's contract: width < 0 returns "", width >= 0 yields a single-row top-edge string of outer width width+2 with no embedded newlines.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/pagepreview.go:169-172`
- Notes: composeChromeLine is now a true one-liner delegating to composeChromeLineParts and concatenating the three returned pieces. Signature preserved. width < 0 guard owned by composeChromeLineParts (line 187-188); composeChromeLine inherits transparently. Tier selection lives in selectChromeTier; tier-4 collapsed row construction centralised in tier4Row. No logic duplication remains.

TESTS:
- Status: Adequate
- Coverage:
  - Direct composeChromeLine cascade tests at threshold widths (pagepreview_compose_chrome_test.go) exercise tier 1/2/3/4 outputs, width<0 empty-string guard (line 35), embedded-newline invariant (line 67-69), corner-glyph invariants (line 77-81), tier 4 minimum frames at args 0/1 (line 200-208).
  - Parity property test `TestComposeChromeLineParts_ConcatenationEqualsComposeChromeLineAtAllThresholds` (line 212-221) asserts left+chrome+right == composeChromeLine across 14 width thresholds — the regression guard pinning the delegation refactor.
  - chromeLineForTest and chromeLineAtModelWidth still compile with unchanged signature.
- Notes: Neither over- nor under-tests the change. Parity test makes refactor's intent explicit.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — single responsibility tightened; selectChromeTier owns cascade selection, tier4Row owns collapsed-row reconstruction, composeChromeLineParts owns styled-vs-unstyled split, composeChromeLine is a pure adapter.
- DRY: Good — duplicate cascade-tier construction eliminated.
- Complexity: Low — composeChromeLine is 3 lines of body, cyclomatic complexity 1.
- Modern idioms: Yes — idiomatic Go multi-return delegation.
- Readability: Good — doc comment accurately describes cascade and names composeChromeLineParts as structural sibling.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Doc comment on composeChromeLine duplicates cascade-tier rules documented on selectChromeTier and composeChromeLineParts. Could trim to "Thin wrapper over composeChromeLineParts; see that function for the cascade" so tier rules have a single canonical home. Low priority.
