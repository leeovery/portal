TASK: enter-attaches-from-preview-2-2 — Render conditional flash row between filter input and Sessions list

ACCEPTANCE CRITERIA:
- Empty `flashText` → no flash row, byte-identical to before.
- Non-empty `flashText` → exactly one flash row between filter input and list.
- Activation shifts list down by 1 row; clearing pops up by 1.
- No existing chrome row replaced/overlaid.
- Flash never above filter input or below/inside list.
- Other pages render byte-identically.

STATUS: Complete

SPEC CONTEXT: Spec § Inline flash — feature-local infrastructure > Render mandates a single chrome line between filter input and Sessions list. No row reserved when inactive. Filter input keeps existing position; no existing chrome row replaced or overlaid.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:1698-1727 — `viewSessionList` with conditional flash-row insertion via newline split after the bubbles/list first line.
  - internal/tui/model.go:1729-1741 — package-level `flashRowStyle` (subdued `#888888`) and `(m *Model) renderFlashRow()` helper.
  - internal/tui/model.go:1620-1622 — only the `default` (Sessions) branch routes to `viewSessionList`; non-Sessions pages cannot see flash by construction.
- Notes:
  - Implementation uses manual `strings.IndexByte` split + concat rather than the plan's recommended `lipgloss.JoinVertical(parts...)` shape. This matches the local `pageProjects` status-line insertion convention (model.go:1606-1614). Acceptable deviation.
  - `formatSessionGoneFlash` helper appears in sessions_flash.go:87-89 — strictly task 2-5's territory but landing early is harmless.

TESTS:
- Status: Adequate
- File: internal/tui/sessions_flash_render_test.go
- Coverage:
  - TestSessionsView_NoFlashRow_WhenFlashTextEmpty — strongest byte-identity assertion.
  - TestSessionsView_FlashRow_AppearsBetweenTitleAndList — strict ordering: title < flash < row.
  - TestSessionsView_FlashActivation_ShiftsListDownByOne / TestSessionsView_FlashDeactivation_ShiftsListUpByOne — symmetric ±1 row contract.
  - TestSessionsView_FlashText_AppearsVerbatim — flash with spaces and embedded quotes.
  - TestSessionsView_OnlyOneFlashRowAdded — exact +1 line delta and exactly one occurrence.
  - TestProjectsPage_FlashTextNotRendered / TestLoadingPage_FlashTextNotRendered — non-Sessions isolation.
- Notes:
  - FileBrowser/Preview pages not explicitly tested for isolation, but structurally impossible to fail.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good — `renderFlashRow` is single-responsibility.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/model.go:1729-1733 — comment claims `flashRowStyle is constructed fresh per render`, but it's a package-level `var` constructed once. Behaviour is correct (lipgloss styles are immutable value types) — only the comment is misleading.
- [quickfix] internal/tui/model.go:1739 — `renderFlashRow` uses pointer receiver `(m *Model)` but performs no mutation. Could be value-receiver for consistency.
- [idea] If a third caller of "insert chrome row after list title" appears, extract a shared `insertChromeRow(listView, row string) string` helper.
- [idea] FileBrowser/Preview isolation tests would lock structural-impossibility against future View-dispatch refactors.
