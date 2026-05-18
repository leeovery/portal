# Analysis Tasks: preview-visual-distinction (Cycle 1)

- topic: preview-visual-distinction
- cycle: 1
- total_proposed: 6

## Task 1: Unify ANSI-stripping in cascade e2e test on package helper
- status: created
- severity: medium
- sources: duplication

**Problem**: `internal/tui/pagepreview_cascade_e2e_test.go:49,174` rolls its own `ansiSGRRe = regexp.MustCompile("\x1b\\[[0-9;]*m")` plus `ansiSGRRe.ReplaceAllString(raw, "")` to strip ANSI before substring assertions. The in-package helper `stripANSI` (in `pagepreview_chrome_test.go:14-16`) already delegates to `github.com/charmbracelet/x/ansi`'s `ansi.Strip`, which handles the full ANSI escape grammar — not just SGR. Two helpers in the same package cover the same intent and can silently diverge on non-SGR inputs.

**Solution**: Delete `ansiSGRRe` and every `ansiSGRRe.ReplaceAllString(...)` call in `pagepreview_cascade_e2e_test.go`. Replace those call sites with `stripANSI(...)`.

**Outcome**: Single canonical ANSI-stripping primitive in the test suite; no risk of the bespoke regex variant drifting on non-SGR sequences.

**Do**:
1. Open `internal/tui/pagepreview_cascade_e2e_test.go`.
2. Remove the `ansiSGRRe` declaration.
3. For each call site, replace `ansiSGRRe.ReplaceAllString(raw, "")` with `stripANSI(raw)`.
4. Remove any now-unused `regexp` import.

**Acceptance Criteria**:
- No reference to `ansiSGRRe` remains anywhere in `internal/tui/`.
- `pagepreview_cascade_e2e_test.go` uses `stripANSI` at every former call site.
- `go test ./internal/tui/...` passes.

**Tests**: Existing cascade e2e tests continue to pass unchanged.

---

## Task 2: Promote `newFramePreviewModelAt` to shared preview-model test helper
- status: created
- severity: medium
- sources: duplication

**Problem**: The 5-line idiom — build `stubEnumerator{groups: ...}`, build `recordingReader{bytes: ...}`, call `NewPreviewModel("work", enum, reader, nil, W, H)`, fail-fast on `!ok` — is open-coded in `internal/tui/pagepreview_resize_test.go:17-27,51-60` (twice) and `internal/tui/pagepreview_cascade_e2e_test.go:161-170` (inside a table loop). The pair `newFramePreviewModel` / `newFramePreviewModelAt` already exists in `pagepreview_view_frame_test.go:266-287`.

**Solution**: Promote `newFramePreviewModelAt(t, windowName, payload, width, height) previewModel` to a shared in-package helper. Migrate the three duplicating call sites.

**Outcome**: Single construction primitive across new preview tests.

**Do**:
1. Confirm `newFramePreviewModelAt` is accessible at package scope. If gated, lift into `pagepreview_helpers_test.go`.
2. In `pagepreview_resize_test.go` (~17-27 and 51-60), replace each open-coded construction with `newFramePreviewModelAt(t, ...)`.
3. In `pagepreview_cascade_e2e_test.go` (~161-170), replace the open-coded block.

**Acceptance Criteria**:
- The 5-line construction idiom no longer appears verbatim in `pagepreview_resize_test.go` or `pagepreview_cascade_e2e_test.go`.
- Three call sites use `newFramePreviewModelAt`.
- `go test ./internal/tui/...` passes.

**Tests**: Existing tests in both files continue to pass with the migrated helper.

---

## Task 3: Add `chromeLineAtModelWidth` test helper alongside `chromeLineForTest`
- status: created
- severity: low
- sources: duplication

**Problem**: The 6-arg call `composeChromeLine(m.width-previewFrameOverhead, m.windowIdx, len(m.groups), m.paneIdx, len(m.currentGroup().PaneIndices), m.currentGroup().WindowName)` appears in production `View()` at `internal/tui/pagepreview.go:541-546` and is re-spelled verbatim in `pagepreview_layout_test.go:37-43` and `pagepreview_externalkill_test.go:387-393`.

**Solution**: Add a sibling helper `chromeLineAtModelWidth(m previewModel) string` in `pagepreview_helpers_test.go`.

**Outcome**: Tests asserting against the actual cascade tier share one helper with production `View()`'s argument extraction.

**Do**:
1. Add helper adjacent to `chromeLineForTest`:
   ```go
   func chromeLineAtModelWidth(m previewModel) string {
       return composeChromeLine(m.width-previewFrameOverhead, m.windowIdx, len(m.groups), m.paneIdx, len(m.currentGroup().PaneIndices), m.currentGroup().WindowName)
   }
   ```
2. Replace open-coded 6-arg calls at `pagepreview_layout_test.go` (~37-43) and `pagepreview_externalkill_test.go` (~387-393).

**Acceptance Criteria**:
- `chromeLineAtModelWidth` exists in `pagepreview_helpers_test.go`.
- The 6-arg verbatim form no longer appears at the two named test sites.
- `go test ./internal/tui/...` passes.

**Tests**: Existing tests at those sites pass with the new helper.

---

## Task 4: Extract `tier4Row` helper to deduplicate collapsed-row reconstruction
- status: created
- severity: low
- sources: duplication, architecture

**Problem**: `border.TopLeft + strings.Repeat(border.Top, max(0, outer-2)) + border.TopRight` appears verbatim in `composeChromeLine` (`pagepreview.go:169`) and `composeChromeLineParts` (`pagepreview.go:197`).

**Solution**: Extract `func tier4Row(border lipgloss.Border, outer int) string`. Call from both sites.

**Outcome**: One source of truth for tier-4 collapsed-row construction.

**Do**:
1. Add `tier4Row` helper near the chrome helpers in `pagepreview.go`.
2. Replace the verbatim expression at line ~169 with `tier4Row(border, outer)`.
3. Replace the verbatim expression at line ~197 with `tier4Row(border, outer)`.

**Acceptance Criteria**:
- `tier4Row` exists in `pagepreview.go`.
- The literal expression appears at most once in `pagepreview.go`.
- `go test ./internal/tui/...` passes.

**Tests**: Existing tier-4 cascade tests continue to pass unchanged.

---

## Task 5: Fix stale `chromeLine` reference in helpers-test docstring
- status: created
- severity: low
- sources: standards

**Problem**: The docstring on `newPreviewModelForHelpers` at `internal/tui/pagepreview_helpers_test.go:17` lists `chromeLine` (the deleted method) among the "helpers under test".

**Solution**: Replace `chromeLine` in the parenthesised list with `composeChromeLine`.

**Outcome**: Docstring matches current code shape.

**Do**: Edit the docstring at `pagepreview_helpers_test.go:17` to replace `chromeLine` with `composeChromeLine`.

**Acceptance Criteria**:
- The standalone `chromeLine` token is not referenced in the `newPreviewModelForHelpers` docstring.
- `go test ./internal/tui/...` passes.

**Tests**: No new tests; docstring-only change.

---

## Task 6: Collapse `composeChromeLine` to a one-liner over `composeChromeLineParts`
- status: created
- severity: low
- sources: architecture

**Problem**: `View()` exclusively uses `composeChromeLineParts`; `composeChromeLine` at `pagepreview.go:160-172` has no production caller. The cascade logic is exhaustively covered via `composeChromeLineParts + selectChromeTier`; `composeChromeLine` duplicates the assembly step (corner-glyph rendering) only to provide tests a single concatenated string.

**Solution**: Collapse `composeChromeLine` to a one-line shim returning `left+chrome+right` derived from `composeChromeLineParts`. Preserve the existing function signature exactly.

**Outcome**: One canonical assembly path. `composeChromeLine` survives as a thin concat shim; corner-glyph assembly lives in exactly one place.

**Do**:
1. Replace the body of `composeChromeLine` (~lines 160-172) with `left, chrome, right := composeChromeLineParts(width, windowIdx, windowCount, paneIdx, paneCount, windowName); return left + chrome + right`.
2. Preserve the existing signature and width<0 guard if needed (the guard already exists in `composeChromeLineParts`).
3. Confirm `chromeLineForTest` (which calls `composeChromeLine`) still compiles.

**Acceptance Criteria**:
- `composeChromeLine` body is a single concat over `composeChromeLineParts`'s three returns.
- Tier-by-tier corner-glyph rendering logic appears exactly once in `pagepreview.go` (inside `composeChromeLineParts`).
- `go test ./internal/tui/...` passes — in particular the concat property test and all `chromeLineForTest`-driven tests.

**Tests**: All existing tests continue to pass unchanged.
