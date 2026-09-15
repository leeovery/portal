TASK: Make openTUI's teardown ordering reachable by a test (tick-fc6181 / open-with-forced-filter-4-8)

ACCEPTANCE CRITERIA:
1. `openTUI` ends in a single `finishTUI` call, and the three teardown statements exist only inside `finishTUI`, in the order canvas → warnings → connect.
2. `finishTUI` writes only through its two parameters and returns `processTUIResult`'s result unchanged; `os.Stdout` and `cmd.ErrOrStderr()` are named at the `openTUI` call site alone.
3. `TestFinishTUI` drives the production function — no hand-composed warnings-emit + `processTUIResult` sequence remains in any test.
4. Moving either writer below `processTUIResult` inside `finishTUI` fails `TestFinishTUI` (demonstrated, then reverted).
5. Behaviour is unchanged on every teardown route.
6. `go test ./...` is green.

STATUS: issues_found

SPEC CONTEXT: The specification (line 310) fixes the ordering this task makes testable: "On K = 1 the TUI tears down before the connector runs, so the notice band never surfaces. The accumulated soft warnings are written to the terminal at that point instead — after teardown, before the attach". Line 122 adds the second route with the same shape: a failed session-list read "reaches the user after teardown" and exits non-zero without an in-TUI error frame. Both routes end in a connect that, outside tmux, reaches `syscall.Exec` and never returns — which is what makes statement order load-bearing rather than stylistic. The canvas set-back has its own contract in `internal/tui/restore.go:20-24`: a set-back rather than an OSC 111 reset, because terminals that ignore the reset keep Portal's canvas after it quits.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/open.go:619-629` (`finishTUI`), `cmd/open.go:754` (the sole call site), `internal/tui/restore_source_guard_test.go:162-271` (the source guard extended to follow the extraction).
- Notes:
  - Criterion 1 holds: `openTUI`'s tail is the single statement `return finishTUI(model, connector, os.Stdout, cmd.ErrOrStderr())` (`cmd/open.go:754`), and the three statements sit inside `finishTUI` in order — `tui.RestoreTerminalBackground(canvas, model)` (`:624`), the warnings write (`:626`), `return processTUIResult(model, connector)` (`:628`). Both ordering comments moved with the statements (`:622-623`, `:625`).
  - Criterion 2 holds: `finishTUI` names no writer of its own — `os.Stdout` occurs once in `cmd/open.go`, at `:754`, and `cmd.ErrOrStderr()` likewise (grep over the file returns that one line for both). The return is `processTUIResult`'s value unchanged.
  - Criterion 5 holds by reading the delivering commit (849a0d3b5): the three statements were moved verbatim with the same writers in the same order, so no teardown route changed behaviour at this task. The warnings statement now reads `tui.WriteBootstrapWarnings(warnings, model.WarningsOwedAtTeardown())` rather than the `emitSearchTeardownWarnings(warnings, model)` the task text names — a later task in this plan moved that helper into `internal/tui` and widened what is owed (a cancelled loading page now writes the warnings the gate took rather than nothing). That is downstream evolution of the delivered change-set, not drift introduced here; the extraction's shape is untouched by it.
  - The extraction broke the pre-existing writer-identity guard (`TestLaunchSites_RestoreIdentically`, `internal/tui/restore_source_guard_test.go:103-135`, which required each launch site to call the restore with `os.Stdout` literally), because the first argument at the call is now a parameter. The implementation restored that reach rather than weakening the guard: `writerArgs` (`:202-240`) resolves a parameter one hop out to whatever the enclosing function's callers **in the same package** pass in that position, `enclosingFunc` (`:190-200`) recovers the enclosing declaration by position so whole-file traversal is retained (a call in a package-level initializer stays visible — the shape `cmd` uses for its function-var seams), and `assignsTo` (`:242-258`) refuses to hop for a parameter the function rebinds. Every unresolvable case yields the bare parameter name, which fails the `want os.Stdout` assertion rather than vanishing — the guard fails closed.
  - Residual in the guard, stated in its own comment at `:207-210` and covered elsewhere: a same-named binding introduced by a nested `var`, a range clause or a closure parameter still resolves to the caller's argument. That shape is caught by `TestFinishTUI` (the canvas writer would receive nothing at connect), so the two cover each other; not a gap worth reporting.

TESTS:
- Status: Adequate
- Coverage: `TestFinishTUI` (`cmd/open_search_warnings_test.go:321-367`) drives the production function with a buffer per writer and the package's `observingConnector` (`:385-393`) snapshotting both buffers inside `Connect`. `withCapturedBackground` (`:369-383`) feeds a `tea.BackgroundColorMsg{Color: color.RGBA{0x12,0x34,0x56,0xff}}` so the echo guard in `RestoreTerminalBackground` has a set-back to write, and it fatals if the model captured no original background — so the canvas assertion cannot pass vacuously. `TestFinishTUI_StagedBootstrapWarnings` (`:420-565`) and `TestFinishTUI_ColdCommandPendingRoute` (`cmd/open_command_pending_warnings_test.go:46-105`), both later tasks' work, add the picker, cancelled-loading, no-warnings and bootstrap-fatal routes through the same function.
- Notes:
  - Criterion 4 is settled by reading, not merely by the executor's claim. Moving `tui.RestoreTerminalBackground` below `processTUIResult` leaves `canvasAtConnect == ""` at `cmd/open_search_warnings_test.go:345-347`, which errors; moving the warnings write below it leaves `warningsAtConnect == ""` against a non-empty `want` at `:338-341`, which errors. The fatal-on-error at `:334-336` plus the `stderr.String() != want` check at `:342-344` also mean a connect that never ran fails the test rather than passing it.
  - Criterion 3 holds: `emitSearchTeardownWarnings` no longer exists anywhere in the tree, and no test pairs a warnings write with `processTUIResult`. The remaining direct `processTUIResult` callers (`cmd/open_fatal_test.go:24,46,61`, `cmd/open_test.go:1813,1835`, `cmd/open_search_deferred_test.go:250,270,286`) all exercise that helper alone — selection, fatal precedence, search-error pass-through — with no writer in sight, which is the same "exercise the helper directly" carve-out the task grants the other subtests.
  - Not over-tested for this task's reach: the added subtest asserts exactly the two order-sensitive facts plus the once-only write. The canvas assertion is a non-empty check rather than an exact byte comparison — right, since the set-back's exact bytes are already pinned in `internal/tui/restore_divergence_test.go:44,59` and the subject here is ordering.

CODE QUALITY:
- Project conventions: Followed. `finishTUI` is an ordinary package function rather than a `*Deps` field or a package-level function var, so it is correctly outside `cmd/seam_guard_test.go`'s two derived seam families (that guard reads `var xDeps *XDeps` declarations and package-level vars whose declaration says they hold a function; a plain `func` decl is neither). No `withFuncSeam` staging is needed or added — the test drives the function directly with its own writers, which is the cheaper and more honest shape here.
- SOLID principles: Good. The extraction is a pure parameterisation of two writers; `processTUIResult` keeps its single responsibility and is still separately drivable.
- Complexity: Low — three statements, no branching.
- Modern idioms: Yes. `io.Writer` parameters rather than concrete types; the grouped `canvas, warnings io.Writer` reads cleanly.
- Readability: Good. Both load-bearing "why" comments travelled with the statements they explain.
- Issues: One inaccurate doc-comment claim, below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/open.go:619-620 — the doc comment says `finishTUI` "returns the connect's own result", which the function does not do on two live paths: `processTUIResult` (`cmd/open.go:602-617`) returns `model.FatalError()` or `model.SearchError()` before any connect happens, and `cmd/open_command_pending_warnings_test.go:101-104` asserts exactly that (`finishTUI` returning the orchestrator's fatal while `refusingConnector` guarantees no connect ran). Change the second clause to say it returns `processTUIResult`'s result unchanged — the wording the acceptance criterion itself uses. — FAILS: a reader diagnosing an exit code or editing the tail takes the returned error to be the connector's, when on a bootstrap fatal or a failed session-list read it is the model's and no connect occurred.

UNSETTLED:
- "`go test ./...` is green." — reading cannot settle a suite result; needs one `go test ./...` run over the current tree (the unit lane, per CLAUDE.md, is the lane holding these `cmd` and `internal/tui` tests).
