# Duplication Analysis — cycle 1

AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 4

## Findings

### 1. Independent ANSI-stripping implementations across new test files
- **SEVERITY**: medium
- **FILES**: `internal/tui/pagepreview_chrome_test.go:14-16`, `internal/tui/pagepreview_cascade_e2e_test.go:49,174`
- **DESCRIPTION**: Two separate primitives strip ANSI escape sequences before substring assertions on chrome content. `stripANSI` (in `pagepreview_chrome_test.go`) delegates to `github.com/charmbracelet/x/ansi`'s `ansi.Strip`. The newly added cascade e2e test rolls its own `ansiSGRRe = regexp.MustCompile("\x1b\\[[0-9;]*m")` plus `ansiSGRRe.ReplaceAllString(raw, "")`. Both cover the same intent, but the regex form only handles SGR (CSI ... m) sequences — not the full ANSI escape grammar that `ansi.Strip` handles — so the two helpers can silently diverge on inputs outside SGR. Both helpers live in the same package; no reason for two.
- **RECOMMENDATION**: Delete `ansiSGRRe` and its `ReplaceAllString` call in `pagepreview_cascade_e2e_test.go`; use the in-package `stripANSI(...)` helper instead.

### 2. Construct-preview-model-with-mocks setup repeated across new test files
- **SEVERITY**: medium
- **FILES**: `internal/tui/pagepreview_resize_test.go:17-27,51-60`, `internal/tui/pagepreview_cascade_e2e_test.go:161-170`, `internal/tui/pagepreview_view_frame_test.go:266-287`
- **DESCRIPTION**: The same 5-line idiom — build a `stubEnumerator{groups: ...}`, build a `recordingReader{bytes: ...}`, call `NewPreviewModel("work", enum, reader, nil, W, H)`, fail-fast on `!ok` — is open-coded in `pagepreview_resize_test.go` (twice) and `pagepreview_cascade_e2e_test.go` (once inside a table loop). `pagepreview_view_frame_test.go` already extracted the helper pair `newFramePreviewModel` / `newFramePreviewModelAt` for exactly this purpose.
- **RECOMMENDATION**: Promote `newFramePreviewModelAt(t, windowName, payload, width, height) previewModel` to a shared in-package test helper. Call sites in `pagepreview_resize_test.go` and `pagepreview_cascade_e2e_test.go` switch to it.

### 3. View()-shaped chrome recomputation duplicated in test assertions
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:541-546`, `internal/tui/pagepreview_layout_test.go:37-43`, `internal/tui/pagepreview_externalkill_test.go:387-393`
- **DESCRIPTION**: The 6-argument call `composeChromeLine(m.width-previewFrameOverhead, m.windowIdx, len(m.groups), m.paneIdx, len(m.currentGroup().PaneIndices), m.currentGroup().WindowName)` (or its `Parts` sibling) appears in production `View()` and is re-spelled verbatim in two test files. `chromeLineForTest` exists but pins width=200, so tests comparing against the actual rendered cascade tier must re-spell the full 6-arg form.
- **RECOMMENDATION**: Add a `chromeLineAtModelWidth(m previewModel) string` test helper alongside `chromeLineForTest` in `pagepreview_helpers_test.go`.

### 4. Tier-4 collapsed-row formula duplicated across composeChromeLine sibling functions
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:169`, `internal/tui/pagepreview.go:197`
- **DESCRIPTION**: The tier-4 collapsed-row reconstruction `border.TopLeft + strings.Repeat(border.Top, max(0, outer-2)) + border.TopRight` appears verbatim in `composeChromeLine` and `composeChromeLineParts`. The two functions otherwise share tier selection through `selectChromeTier`, so this is the one remaining drift point.
- **RECOMMENDATION**: Extract a tiny private helper `func tier4Row(border lipgloss.Border, outer int) string` returning the corners-plus-filler string.
