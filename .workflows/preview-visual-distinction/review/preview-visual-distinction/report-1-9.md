TASK: End-to-End Cascade-Tier Update + View Test (preview-visual-distinction-1-9)

ACCEPTANCE CRITERIA:
- Test function exists in `internal/tui/pagepreview_cascade_e2e_test.go`.
- All five width rows pass with expected tier signatures.
- Every row asserts SGR-reset bytes present.
- Width 15 asserts tier-4 ASCII pattern `╭` + 13 × `─` + `╮` after stripping ANSI.
- No `t.Parallel()`.
- No `tmuxtest` import.
- Uses constructor-injected mocks for `TmuxEnumerator` and `ScrollbackReader`.

STATUS: Complete

SPEC CONTEXT: Spec § Tests > Surface 5 prescribes an end-to-end test driving Update → View at cascade-threshold widths to verify rendered frame and pure cascade function do not drift apart. Each row asserts tier-signature substrings plus SGR-reset bytes.

IMPLEMENTATION:
- Status: Implemented (with documented threshold adjustment).
- Location: `internal/tui/pagepreview_cascade_e2e_test.go:43-166`.
- Notes:
  - Spec literally names widths {200, 60, 40, 25, 15} based on stale assumption verboseKeymap=82 cells. Real `lipgloss.Width(verboseKeymap) == 57`. Implementer derived actual tier intervals (tier1-full ≥ 108, tier1-truncated ∈ [105,107], tier2 ∈ [89,104], tier3 ∈ [41,88], tier4 ∈ [2,40]) and picked interior widths {200, 105, 95, 50, 15}. Threshold rationale documented in top-of-file comment. Using spec literals would misclassify tiers. Correct deviation.
  - Width 15 retains spec's literal ASCII pattern assertion verbatim.
  - Reuses canonical helpers (`newFramePreviewModelAt`, `stripANSI`, `firstLine`).

TESTS:
- Status: Adequate.
- Coverage: All five spec-mandated cascade tiers exercised end-to-end; tier signatures verified by presence-of-expected AND absence-of-other-tier substrings; SGR-reset bytes asserted per row.
- Notes:
  - Tier 3 assertion correctly checks `compactKeymap` present AND `next pane` absent.
  - Tier-1-truncated assertion isolates name token between `"win: "` and next space, then checks prefix + ellipsis suffix.
  - SGR-reset assertion uses raw output, preserving bytes strip would erase.
  - Fixture body `"\x1b[41mhello\nworld\n"` contains unterminated SGR so `injectSGRResets` must run.
  - Not over-tested.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — table-driven, closure-per-row.
- Complexity: Low.
- Modern idioms: Idiomatic Go.
- Readability: Excellent — top-of-file threshold-rationale comment is exemplary.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Specification's tier-mapping widths {200, 60, 40, 25, 15} are mathematically wrong against real `verboseKeymap` width. Implementer correctly adjusted, but spec doc still carries misleading widths. Follow-up doc-edit pass on spec to align with real math.
- [idea] Test asserts `strings.Contains(raw, "\x1b[0m")` at file-global level; spec's stricter form is "every non-empty viewport content row carries `\x1b[0m`". Single-presence check still passes if only one row carries reset. Per-row count assertion would tighten the guard.
