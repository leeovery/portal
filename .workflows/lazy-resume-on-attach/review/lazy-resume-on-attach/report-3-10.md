TASK: lazy-resume-on-attach-3-10 — The Appearance Query's Own Reply Cannot Answer the Panel

ACCEPTANCE CRITERIA:
- `ResolvePaneTheme` takes the drop and runs it after the appearance query resolves — on every nomination arm and every probe outcome, exactly once per resolution.
- The drop is never entered before the query is written, and never before the probe's reader is closed.
- A nil drop resolves the same palette today's code resolves for every arm, and returns a nil error — the constant and zero arms still write nothing to the terminal and still open it zero times.
- A drop that returns an error still returns the resolved palette, and returns that error unchanged, so the caller can paint the fallback screen and report the reason.
- A `colourless` or constant resolution writes no query and opens no terminal, and still runs the drop exactly once.
- `newPaneAppearanceProbe` has exactly one call site in the tree outside its declaration and the package's own tests.
- The probe's existing guards still pass unchanged: `TestPaneAppearance_TakesThePickerTimeout`, `TestPaneAppearance_OpensNothingButThePanesTerminal`, `TestBackgroundSet_ConfinedToRestore`.

STATUS: issues_found

SPEC CONTEXT: §5.2 (corrigendum 2026-09-21) states that the pane's appearance gate reads under a deadline and closes, so a late OSC 11 reply (`ESC ] 11 ; rgb:…`, which opens with the confirmation's cancel key and can carry a hex `d`) stays in the pane's input queue. §4.3's rule (nothing the user did not send may answer the panel) governs it, and the ordering is fixed: the input drop runs after the appearance query, never before. The corrigendum also directs that the reversed ordering in tasks 4-1 and 5-4 be corrected rather than implemented as written. A reply arriving after the drop is left to the waiter's escape-sequence handling, which consumes OSC sequences to their terminator (cmd/state_resume_wait.go:28-35, 222-257).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/pane_appearance.go:65-67 — `ResolvePaneTheme(n, colourless, dropInput func() error) (theme.Theme, error)`, delegating to the injectable `resolvePaneTheme` with `newPaneAppearanceProbe()`
  - internal/tui/pane_appearance.go:69-75 — `resolvePaneTheme` resolves the palette through `selectPaneTheme` first, then returns `resolved, nil` for a nil drop or `resolved, dropInput()`: one call, after resolution, on every arm, with the error passed through unwrapped and the palette returned regardless
  - internal/tui/pane_appearance.go:77-85 — `selectPaneTheme`, the previous arm logic moved unchanged (constant → `Constant()`, zero or colourless → `Select(MemberDark)`, pair → `Select(p.detect())`)
  - internal/tui/pane_appearance.go:95, 101 — the `reader.Close` and raw-mode restore are deferred inside `detect`, so both have run before `detect` returns to `selectPaneTheme`. The drop therefore always follows the query write and the reader close, and this ordering does not depend on anything a caller does.
- Notes: The ordering is structural, as the task asks. The later wiring in phase 4/5 holds the corrected ordering. The single production caller, cmd/state_resume_draw.go:86-89 (`paneDrawTheme`), hands the drop to `tui.ResolvePaneTheme`, and `dropStdinInputQueue` (cmd/state_resume_draw.go:119-124) is the only production drop site in `cmd`. No drop runs before the query. The doc comment at pane_appearance.go:61-64 accurately describes the code.

TESTS:
- Status: Adequate
- Coverage:
  - Ordering (internal/tui/pane_appearance_test.go:275-299): a shared recorder through `recordingWriter` (query write), `fakePaneReader.Close` and the drop asserts the exact sequence [write query, close reader, drop] for a reply before the deadline and a reply that never comes, and [drop] alone for an unopenable terminal. A drop taken before the query, or between the write and the close, fails this test.
  - Exactly once per nomination (:301-333): constant, zero, adaptive pair and colourless each get `drops == 1`. The non-detecting arms also get "wrote nothing" and `opens == 0`.
  - Nil drop (:185-273, :335-362): absolute palettes are pinned with a nil drop on every arm (constant, zero, NO_COLOR ×3, pair light/dark/no-reply) with a nil error. The nil-versus-drop comparison covers the four named arms.
  - Drop failure (:364-377): the probe's light answer is returned beside a non-nil error.
  - Drop's own error (:379-389): `errors.Is` against the sentinel.
  - Single route (internal/tui/pane_appearance_guard_test.go:60-76): counts `newPaneAppearanceProbe` calls across the package's non-test sources and requires exactly 1. Zero also fails, so the guard cannot pass after it stops seeing the call. The function is unexported, so the package scope is the whole tree.
- Notes: The "unchanged" half of the error criterion is not pinned (see FINDINGS). The pre-existing guards are unmodified by the task commit (3bd302bb8). By reading, nothing they police changed: internal/tui still has exactly one `OpenFile` (pane_appearance.go:156, on `openTTYPath`'s own parameter), `SetBackgroundColor` is called only in restore.go:35, pane_appearance.go still names `appearanceDetectTimeout` (:51), and it spells no `time.<unit>`.

CODE QUALITY:
- Project conventions: Followed. The seam stays injectable through the unexported `resolvePaneTheme`, the source guard goes through `sourceguardtest`, and nothing logs from the leaf.
- SOLID principles: Good. `selectPaneTheme` keeps palette selection separate from the drop's sequencing.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None beyond the finding below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/tui/pane_appearance_test.go:386 — The subtest "it returns the drop's own error" asserts `!errors.Is(err, errSeam)`, which also accepts a wrapped error. The acceptance criterion requires the error "unchanged", and the subtest's own failure message claims "unchanged — nothing here wraps it". Fix: pin identity instead, `if err != errSeam {` (the `errors` import stays used by `errSeam = errors.New(...)` at :32). — FAILS: a regression that wraps the drop error inside `resolvePaneTheme` (e.g. `fmt.Errorf("resolve pane theme: %w", err)`) passes the package's tests. It also changes the text the discard-fallback panel shows the user, because the caller renders `dropErr.Error()` into the report verbatim (cmd/state_resume_draw.go:57). No cmd test catches it either, since they fake `ResolveTheme`.

UNSETTLED:
- None
