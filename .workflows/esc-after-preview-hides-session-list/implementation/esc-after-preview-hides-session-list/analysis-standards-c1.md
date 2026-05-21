# Analysis — Standards (cycle 1)

STATUS: clean
FINDINGS_COUNT: 0

## Summary

Implementation conforms to specification and project conventions. All seven acceptance criteria satisfied:

- AC #4: `applySessions` returns `tea.Cmd` (model.go:662-671). Both call sites propagate — `SessionsMsg` handler at model.go:900,913,918; `previewSessionsRefreshedMsg` at model.go:1025-1027.
- AC #5: Secondary sweep applied — `WithInsideTmux` at model.go:408-410 (panic-on-unreachable), `ProjectsLoadedMsg` at model.go:940-951 (cmd captured and returned).
- AC #3: Boot path unchanged — `Init` flow and `applySessions` return `nil` cmd when unfiltered.
- AC #6: `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` augmented with `VisibleItems()` slice-equality assertion and cursor-index assertion. New `TestKillRefreshUnderFilterPreservesFilteredList` covers the kill-refresh latent variant via the full real-keystroke path.
- AC #1, #2: covered by the test suite end-to-end (preview-dismiss + kill-refresh under filter).
- AC #7: no `t.Parallel()` in either new file (CLAUDE.md rule honoured).

`drainRefilterCmd` extension of `pressSpaceThenEscWithRefresh` correctly handles the nil-cmd case (unfiltered boot) and is independently unit-tested.

## Informational

### Panic message in WithInsideTmux matches spec verbatim
- SEVERITY: low (informational)
- FILES: internal/tui/model.go:408-410
- NOTE: Spec § Fix Approach explicitly permits `panic("unreachable: WithInsideTmux runs before any filter can be applied")`. Implementation uses this exact wording.

### killerStub vs existing mockSessionKiller — package-boundary divergence
- SEVERITY: low (informational)
- FILES: internal/tui/kill_refresh_filter_test.go:13-21
- NOTE: Necessary because the internal-package test can't import `tui_test`. Matches CLAUDE.md DI/testing pattern shape.
