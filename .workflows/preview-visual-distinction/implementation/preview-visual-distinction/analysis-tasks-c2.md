# Analysis Tasks: preview-visual-distinction (Cycle 2)

- topic: preview-visual-distinction
- cycle: 2
- total_proposed: 2

## Task 1: Extract innerWidth/innerHeight methods on previewModel
- status: pending
- severity: low
- sources: architecture

**Problem**: Translation from outer terminal dimensions to inner viewport/chrome width (`m.width − previewFrameOverhead` / `m.height − previewFrameOverhead`) is repeated verbatim across four production call sites and two test helper sites. The latent footgun is the clamping asymmetry: the constructor (`pagepreview.go:304`) and `WindowSizeMsg` handler (`pagepreview.go:452-453`) clamp via `max(0, …)`, but `View()`'s `composeChromeLineParts` call (`pagepreview.go:541-542`) is unclamped.

**Solution**: Add two value-receiver methods on `previewModel` — `innerWidth() int` and `innerHeight() int` — each returning `max(0, m.{width|height} − previewFrameOverhead)`. Replace the production + test helper sites.

**Outcome**: One canonical definition of inner-frame dimensions. Clamping applied uniformly. Frame-geometry changes become a single-site edit.

**Do**:
1. Add near the existing `previewModel` methods in `pagepreview.go`:
   - `func (m previewModel) innerWidth() int  { return max(0, m.width  - previewFrameOverhead) }`
   - `func (m previewModel) innerHeight() int { return max(0, m.height - previewFrameOverhead) }`
2. Replace the constructor site (`pagepreview.go:304`) with `m.innerWidth()` / `m.innerHeight()`. If `m`'s `width`/`height` aren't yet populated at that point in construction, leave the constructor's local arithmetic in place and migrate only the other three production sites + two test helper sites — add an inline comment at the constructor explaining the exception.
3. Replace the `WindowSizeMsg` handler sites (`pagepreview.go:452-453`) with `m.innerWidth()` / `m.innerHeight()`.
4. Replace the `View()` site (`pagepreview.go:541-542`) with `m.innerWidth()`. This tightens behaviour: the call now passes a clamped value rather than relying on the callee's short-circuit on negatives.
5. In `pagepreview_helpers_test.go:41,51` replace the re-spelled arithmetic with the new methods.
6. Run `go test ./internal/tui/...`.

**Acceptance Criteria**:
- `innerWidth()` and `innerHeight()` value-receiver methods exist on `previewModel`.
- Literal `width-previewFrameOverhead` / `height-previewFrameOverhead` appears in `pagepreview.go` and `pagepreview_helpers_test.go` only inside method bodies (constructor exception permitted with an inline comment).
- `View()`'s `composeChromeLineParts` call passes a clamped width.
- `go test ./internal/tui/...` passes.

**Tests**: Existing TUI tests cover the call sites; no new tests required.

---

## Task 2: Extract firstLine test helper for top-row extraction idiom
- status: pending
- severity: low
- sources: duplication

**Problem**: The three-line idiom `topRow := out; if i := strings.IndexByte(out, '\n'); i >= 0 { topRow = out[:i] }` is duplicated across 5 sites in three new test files (`pagepreview_view_frame_test.go:47-51,88-92,193-197`; `pagepreview_cascade_e2e_test.go:137-141`; `pagepreview_view_routing_test.go:64-66`).

**Solution**: Add a private test helper `firstLine(s string) string` in `pagepreview_helpers_test.go` (alongside `chromeLineAtModelWidth`). Migrate all five call sites.

**Outcome**: One canonical helper for top-row extraction in the preview frame test suite.

**Do**:
1. In `pagepreview_helpers_test.go`, add:
   ```go
   func firstLine(s string) string {
       if i := strings.IndexByte(s, '\n'); i >= 0 {
           return s[:i]
       }
       return s
   }
   ```
2. Replace the three-line idiom at each of the 5 call sites with `topRow := firstLine(out)`.
3. Run `go test ./internal/tui/...`.

**Acceptance Criteria**:
- `firstLine` helper exists in `pagepreview_helpers_test.go`.
- None of the three test files contain the literal `strings.IndexByte(out, '\n')` idiom for top-row extraction.
- All five originally-listed call sites call `firstLine`.
- `go test ./internal/tui/...` passes.

**Tests**: Helper exercised by migrated call sites; no separate unit test required.
