TASK: session-scrollback-preview-4-3 — Fewer-than-N lines renders all available lines (non-placeholder partial read)

ACCEPTANCE CRITERIA:
- (bytes, nil) with 1 line renders bytes in viewport.
- 50 lines renders all 50.
- 999 lines renders all 999.
- Placeholder string "(no saved content)" absent in any of the above.
- Viewport at scroll-tail on initial open / focus change.
- Scroll-up at top boundary is silent no-op.

STATUS: Complete

SPEC CONTEXT:
- § Read-Failure Handling > Placeholder > Non-triggering condition: "fewer than N lines does not trigger the placeholder."
- § History Depth > Scroll within bounds: top boundary is hard edge.

IMPLEMENTATION:
- Status: Implemented (test-only task; production behaviour was already in place from Phase 2/4-1).
- Location:
  - Test file: internal/tui/pagepreview_fewerthann_test.go (215 lines, six tests).
  - Production dispatcher: internal/tui/pagepreview.go:202-215.
  - Constructor: pagepreview.go:73-104.
  - Up-at-top no-op: handled by default tea.Msg passthrough at pagepreview.go:309-311 routing into bubbles/viewport.

TESTS:
- Status: Adequate
- Coverage: All six advertised cases:
  - 1-line case: asserts "line1" visible, placeholder absent, TotalLineCount >= 1.
  - 50-line case: GotoTop reveals "line1", anchored on "line50".
  - 999-line case (one below N=1000 boundary).
  - Scroll-tail on open.
  - Scroll-up no-op at top: GotoTop, sends tea.KeyUp via Update(), asserts YOffset still 0, cmd is nil, view unchanged.
  - Sweep regression-anchor: t.Run subtest matrix over {1,2,50,500,999}.
- Use of TotalLineCount() rather than walking View() across pages is pragmatic.
- Combined View() check (207-211) extends the contract beyond the bare acceptance bullet.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(). No tmuxtest import.
- SOLID: N/A for tests.
- Complexity: Low.
- Modern idioms: strings.Builder, t.Run subtests.
- Readability: Good. Comments anchor each test back to spec.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] decimal() helper duplicates strconv.Itoa with a hand-rolled itoa; could replace decimal with strconv.Itoa for ~12 fewer LOC.
- [idea] "Up at top boundary returns nil cmd" assertion pins a property of bubbles/viewport's current implementation; a future viewport version returning a tea.Cmd at the top boundary would fail despite user-visible behaviour being unchanged.
