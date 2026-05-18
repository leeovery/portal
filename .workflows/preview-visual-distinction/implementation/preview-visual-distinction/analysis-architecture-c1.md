# Architecture Analysis — cycle 1

AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 6

## Findings

### 1. composeChromeLine is production-dead; only tests call it
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:160-172`, `internal/tui/pagepreview_compose_chrome_test.go:29-31`, `internal/tui/pagepreview_helpers_test.go:40-42`
- **DESCRIPTION**: `View()` exclusively uses `composeChromeLineParts` (the styled-parts variant); `composeChromeLine` has no production caller. It exists solely as a testing surface — a property test asserts `left+chrome+right == composeChromeLine`, and ~30 legacy chrome-content tests funnel through `chromeLineForTest`, which calls `composeChromeLine` at a fixed inner width of 200. The cascade logic is already covered exhaustively via `composeChromeLineParts + selectChromeTier`; `composeChromeLine` duplicates the assembly step (corner-glyph rendering) only to give tests a single concatenated string.
- **RECOMMENDATION**: Treat `composeChromeLineParts` as the sole canonical surface. Either (a) delete `composeChromeLine` and have the concat property test assemble parts itself; or (b) collapse `composeChromeLine` to a one-line helper `return left+chrome+right` over `composeChromeLineParts`.

### 2. Inner/outer width asymmetry across composeChromeLine vs selectChromeTier is a footgun
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:160`, `internal/tui/pagepreview.go:185`, `internal/tui/pagepreview.go:213`
- **DESCRIPTION**: `composeChromeLine` and `composeChromeLineParts` take an *inner* width parameter (terminalWidth−2) and convert locally (`outer := width + 2`), while `selectChromeTier` takes an *outer* width. Three sibling functions with the same parameter name (`width`/`outer`) but different semantic conventions — the discipline is enforced only by reading doc comments.
- **RECOMMENDATION**: Pick one convention and apply it across all three helpers. Inner-width is the more useful boundary; alternatively introduce named types `innerWidth int` / `outerWidth int` so the compiler enforces the distinction.

### 3. Inner-dimension arithmetic (width−2, height−2) repeated at four call sites without a helper
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:304`, `internal/tui/pagepreview.go:452-453`, `internal/tui/pagepreview.go:542`, `internal/tui/pagepreview.go:553`
- **DESCRIPTION**: The translation from outer terminal dimensions (`m.width / m.height`) to inner viewport dimensions (`m.width − previewFrameOverhead`) appears verbatim in the constructor, the `WindowSizeMsg` handler, `View()`'s `composeChromeLineParts` call, and `View()`'s body composition. `previewFrameOverhead` names the magic number 2, but the arithmetic itself is still duplicated, as is the `max(0, …)` non-negative clamp.
- **RECOMMENDATION**: Add `innerWidth`/`innerHeight` value-receiver methods on `previewModel` and call them at every site that currently computes the subtraction.

### 4. Chrome composition helpers have a six-parameter signature, exceeding the long-parameter-list threshold
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:160`, `internal/tui/pagepreview.go:185`, `internal/tui/pagepreview.go:213`
- **DESCRIPTION**: `composeChromeLine`, `composeChromeLineParts`, and `selectChromeTier` each take six positional parameters (`width, windowIdx, windowCount, paneIdx, paneCount, windowName`). code-quality.md flags long parameter lists (4+) as an anti-pattern. The parameters all describe one concept — "the dynamic chrome content for a focused position" — and travel as a group across every call site, with `View()` and the test helper pulling them out of the same `previewModel` each time.
- **RECOMMENDATION**: Introduce a `chromeInputs` value type holding the five non-width fields. Keep `width` as a separate parameter because it varies independently across cascade-threshold tests.

### 5. Tier 4 collapse uses asymmetric chrome=="" sentinel rather than a typed signal
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview.go:166-171`, `internal/tui/pagepreview.go:191-201`, `internal/tui/pagepreview.go:540-548`
- **DESCRIPTION**: `selectChromeTier` signals tier-4 collapse by returning `chrome == ""`. `composeChromeLineParts` then encodes the entire collapsed row in `left` and returns empty chrome/right. The contract works but depends on caller discipline.
- **RECOMMENDATION**: Pre-emptive change not warranted today; if a second caller of `composeChromeLineParts` appears, return an explicit tier indicator (`tier int` or `collapsed bool`).

### 6. Test surface is materially larger than implementation surface and overlaps across files
- **SEVERITY**: low
- **FILES**: `internal/tui/pagepreview*_test.go` (30 files)
- **DESCRIPTION**: The preview package now has roughly 13:1 test-to-production line ratio across 30 test files, with substantial overlap in concerns. Most predates this work unit (carried over from prior preview features) and is not in scope to delete here, but the structural shape of "every new chrome decision spawns a new test file" is now a maintenance signal worth naming.
- **RECOMMENDATION**: Out-of-scope for this work unit. Worth surfacing in a follow-up cleanup.
