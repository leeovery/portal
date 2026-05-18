# Architecture Analysis — cycle 2

AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 2

## Findings

### 1. Inner-dimension arithmetic repeated at six sites with inconsistent clamping
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:304,452-453,541-542`, `internal/tui/pagepreview_helpers_test.go:41,51`
- **DESCRIPTION**: The translation from outer terminal dimensions (`m.width`/`m.height`) to inner viewport/chrome width (`m.width − previewFrameOverhead`) appears verbatim across constructor (line 304: `max(0, width-previewFrameOverhead)` for both `viewport.New` args), `WindowSizeMsg` handler (lines 452-453: same pattern for `viewport.Width`/`Height` field writes), and `View()`'s `composeChromeLineParts` call (line 542: `m.width-previewFrameOverhead`, **unclamped** — relies on callee short-circuit). Two test helpers in `pagepreview_helpers_test.go` (lines 41, 51) re-spell the same arithmetic. The contract is correct but the unclamped-vs-clamped asymmetry is a latent footgun.
- **RECOMMENDATION**: Add value-receiver methods on `previewModel` — `innerWidth() int` and `innerHeight() int` — returning `max(0, m.width − previewFrameOverhead)` / `max(0, m.height − previewFrameOverhead)`. Replace the four production call sites and two test helper sites. Single edit point if frame geometry changes; removes the clamping asymmetry.

### 2. composeChromeLine shim is a production-dead testing-only surface (informational, not actionable)
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:169-172`, `internal/tui/pagepreview_compose_chrome_test.go:30`, `internal/tui/pagepreview_helpers_test.go:41,51`
- **DESCRIPTION**: Cycle-1 collapsed `composeChromeLine` to a one-line forwarder. `View()` uses `composeChromeLineParts` directly; the shim is production-dead. ~30 legacy tests funnel through it via `chromeLineForTest`/`chromeLineAtModelWidth`.
- **RECOMMENDATION**: Not worth churning this cycle. Re-evaluate only if a third surface arrives in the same family.

## Summary

Cycle-1 fixes retired the two highest-impact architectural concerns. One actionable seam remains — inner-dimension arithmetic duplicated with inconsistent clamping. Cycle-1 deferred findings #2 (width-convention asymmetry) and #4 (6-param signature) remain non-blocking; cycle-1 reduced their footprint further.
