# Duplication Analysis — cycle 2

AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1

## Findings

### 1. First-line ("top row") extraction idiom duplicated across new frame tests
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview_view_frame_test.go:47-51,88-92,193-197`, `internal/tui/pagepreview_cascade_e2e_test.go:137-141`, `internal/tui/pagepreview_view_routing_test.go:64-66`
- **DESCRIPTION**: The same three-line "take everything before the first newline" idiom — `topRow := out; if i := strings.IndexByte(out, '\n'); i >= 0 { topRow = out[:i] }` — appears 5 times across new test files. Each site reaches for the same semantic: isolate the rendered top border row for substring/width assertions. Left in place, it's a drift point.
- **RECOMMENDATION**: Add a small private test helper `firstLine(s string) string` in `pagepreview_helpers_test.go` (alongside `chromeLineAtModelWidth`); migrate all five call sites.

## Summary

Cycle-1 findings are all resolved. One remaining low-severity duplication: the top-row extraction idiom in 5 sites across the new frame tests.
