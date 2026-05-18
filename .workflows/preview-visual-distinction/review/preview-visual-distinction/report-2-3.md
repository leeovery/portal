TASK: Add chromeLineAtModelWidth Test Helper Alongside chromeLineForTest (preview-visual-distinction-2-3)

ACCEPTANCE CRITERIA:
- Add `chromeLineAtModelWidth(m previewModel) string` test helper alongside `chromeLineForTest` in `pagepreview_helpers_test.go`
- Helper composes chrome at the model's actual inner width
- Eliminate the verbatim 6-arg `composeChromeLine(...)` re-spelling in `pagepreview_layout_test.go` and `pagepreview_externalkill_test.go`

STATUS: Complete

SPEC CONTEXT: Phase 2 (Analysis Cycle 1) low-severity duplication remediation. The 6-arg composeChromeLine call was re-spelled in two test files because `chromeLineForTest` pins width=200.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/pagepreview_helpers_test.go:55-63` (helper definition)
- Call sites migrated:
  - `internal/tui/pagepreview_layout_test.go:38` (`wantChrome := chromeLineAtModelWidth(m)`)
  - `internal/tui/pagepreview_externalkill_test.go:388` (`want := chromeLineAtModelWidth(mm)`)
- Helper body uses `m.innerWidth()` (extracted in 3-1) — picks up the SSoT refactor for free.
- Notes: Docstring is clear and explicit about when to use this helper vs `chromeLineForTest`.

TESTS:
- Status: Adequate
- Coverage: Test-only helper; verification is consumer tests still pass and no longer duplicate 6-arg production call.
- Notes: No new test needed for the helper itself.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, no `tmuxtest`.
- SOLID: Good — single-purpose helper, no hidden state, no I/O.
- Complexity: Low — single expression.
- Modern idioms: Yes — composes over `innerWidth()`.
- Readability: Good. Docstring contrasts new helper against `chromeLineForTest`. Call-site comments at both consumers updated.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Helper could absorb `stripANSI` wrap that both consumers apply, but would couple it to ANSI-stripping; current symmetric shape with `chromeLineForTest` is preferred.
