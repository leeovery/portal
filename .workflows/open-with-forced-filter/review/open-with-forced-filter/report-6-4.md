TASK: Deliver the soft bootstrap warnings the warm picker route drops (open-with-forced-filter-6-4 / tick-fa4ede)

ACCEPTANCE CRITERIA:
1. On the warm picker route — model on `PageSessions` from frame one, no progress receiver, no loading gate — `finishTUI` writes the staged warnings to its warnings writer, byte-identical to `warning.WriteLines`' rendering on the CLI path
2. The `BootstrapCompleteMsg` arm clears the pending slice whenever it buffers it, and the cleared value is on the model the arm returns
3. Nothing is written twice: on the warm loading-page route `surfaceBufferedWarnings` surfaces them and `finishTUI` adds nothing, and on the cold concurrent route the notice band still owns them and `finishTUI` adds nothing
4. On the K=1 decision attach and the failed-read teardown, `emitSearchTeardownWarnings` still writes `BufferedWarnings()` exactly once and the new write contributes nothing
5. A cancelled loading page still writes nothing
6. `finishTUI` writes nothing when no warnings accumulated
7. Both writes land before the connector runs
8. A warm `portal open -f term` and a warm bare `portal open` report a down saver at teardown too, not only the sigil form

STATUS: complete

SPEC CONTEXT:
§7.4 classifies a sigil invocation as a picker invocation, so its soft bootstrap warnings take the in-TUI route rather than stderr — the classification exists to keep a write out of the frame the picker is about to claim (specification.md:288-296). §7.5 states the counterpart this task extends: on K=1 the TUI tears down before the connector runs, so the notice band never surfaces and the accumulated warnings go to the terminal after teardown and before the attach, "which is where a warm-server sigil attach already puts them" (specification.md:308-310). The warm picker has no loading gate and therefore no band, so the same post-teardown terminal write is the only surface left; that is precisely the hole this task names.

IMPLEMENTATION:
- Status: Implemented, then legitimately evolved past the task's literal wording by three later plan tasks (tick-b8464e 9-3, tick-5b120c 12-1, tick-cb4c82 12-5). The task's substance survives in the current code.
- Location:
  - `cmd/open.go:621-628` — `finishTUI` runs `tui.RestoreTerminalBackground` (`:624`), then `tui.WriteBootstrapWarnings(warnings, model.WarningsOwedAtTeardown())` (`:626`), then `processTUIResult` (`:628`).
  - `internal/tui/model.go:481-488` — `WarningsOwedAtTeardown` returns `slices.Concat(m.bufferedWarnings, m.pendingBootstrapWarnings)`; this is what replaced the task's prescribed pair of writes (`emitSearchTeardownWarnings` + a second `PendingBootstrapWarnings()` write). Both sources still reach the same writer exactly once.
  - `internal/tui/model.go:1666-1672` — the `BootstrapCompleteMsg` arm buffers `slices.Concat(pending, msg.Warnings)` and nils `m.pendingBootstrapWarnings`, inside the `activePage == PageLoading` block; the arm holds `m` by value and returns it (`:1693-1698`), so the clear reaches the caller.
  - `internal/tui/model.go:475-478` — `PendingBootstrapWarnings` carries the one-line doc the task asked for ("the staged warnings no loading gate consumed, which the teardown still owes the terminal").
  - `cmd/bootstrap_warnings.go:43-52` — `stageBootstrapWarningsOnModel` still drains the sink onto the model; `cmd/open.go:741` still calls it before `tea.NewProgram`, so the sink and the model stay disjoint and no route can write twice.
  - Original landing: commit `984eeda20`.
- Notes on the two divergences from the task text, both sound:
  - AC4 names `emitSearchTeardownWarnings`. That helper no longer exists; tick-b8464e (9-3, "Let the model answer what the teardown still owes the terminal") moved the question onto the model. The substance is preserved: the buffered set is still written exactly once on the K=1 attach (`cmd/open_search_warnings_test.go:204-211`, `:322-348`) and on the failed read (`:239-247`), and the staged half is nil on those paths so the union adds nothing.
  - AC5 says a cancelled loading page still writes nothing. The current code writes what the gate took. This is a deliberate, recorded reversal by tick-cb4c82 (12-5), whose problem statement is exactly this hole: after 12-3 made Ctrl-C on the loading page the designed escape from a slow decision, the old rule silently discarded a saver-down warning for the user most likely to need it. The union rule replaced the discriminator, and the subtest was inverted on purpose (`cmd/open_search_warnings_test.go:270-291`, `:293-318`; teardown side `:479-502`). A loss of the earlier behaviour, chosen with a reason, and better than what 6-4 wrote — not a finding.
- AC-by-AC: 1 met (`cmd/open.go:626` → `internal/tui/bootstrap_warnings.go:21-23` → `warning.WriteLines`, the same function `BootstrapWarningsSink.EmitTo` uses at `cmd/bootstrap_warnings.go:38`); 2 met; 3 met (warm loading-page: `surfaceBufferedWarnings` nils the buffer synchronously at `internal/tui/bootstrap_warnings.go:46`/`:60` before returning its command, and the staged half was already nil'd at `model.go:1672`; cold concurrent: same, via the band arm); 4 met in substance; 5 deliberately superseded; 6 met (`slices.Concat` of two empty sets renders nothing); 7 met (`cmd/open.go:624-628` ordering, with the connect last); 8 met.

TESTS:
- Status: Adequate.
- Coverage: every test the task named exists, one under a reworded name.
  - `cmd/open_search_warnings_test.go:421` "it writes the staged bootstrap warnings at teardown when no loading gate consumed them" (AC1)
  - `:435` "it writes the same lines as the CLI path" — compares against `bootstrapWarnings.EmitTo`'s own bytes, so it pins CLI parity rather than restating the format (AC1)
  - `:453` "it writes nothing at teardown when the loading gate already surfaced them" (AC3, warm loading-page half)
  - `:504` "it writes nothing at teardown when no warnings accumulated" (AC6)
  - `:517` "it writes the staged warnings before the connector runs" — reads the stderr buffer from inside `observingConnector.Connect`, which is the only way to observe the exec'd attach's ordering (AC7)
  - `:542` "it writes the staged warnings for a warm picker opened by -f and by a bare open", table-driven over `tui.Deps{InitialFilter: "term"}` and `tui.Deps{}` (AC8)
  - `internal/tui/model_test.go:6900` "it empties the staged set once the loading gate has taken it" — the task's `"it clears the pending warnings when the complete message buffers them"`, reworded; asserts both halves (buffer holds one, pending holds none) on the model the arm returned (AC2)
  - AC3's cold-concurrent half: `cmd/open_search_warnings_test.go:249-268` (`it owes the pending set when a picker frame was painted` / `it owes nothing once a loading gate has surfaced the buffer`), driven through `searchTeardownModel` with a live `ProgressReceiver`.
  - `warmPickerModel` (`:396-418`) asserts `ActivePage() == PageSessions` before handing the model back, so a regression that put the warm route back on a loading page fails the fixture rather than passing vacuously. `WithServerStarted` (`internal/tui/model.go:661-668`) is what makes that distinction real, and the constructor's `activePage: PageSessions` (`:873`) is the other half.
- Notes: the tests would fail if the feature broke — `:421` and `:542` fail if `finishTUI` stops writing the staged set, `:453` fails if it starts writing a consumed one, and `model_test.go:6913` fails if the arm's clear stops propagating. Not over-tested: each subtest pins a distinct route, and the `slices.Concat` aliasing subtest (`model_test.go:6955-6970`) guards a real hazard the arm's comment names. The CLI-parity assertion recurs in three route-scoped suites (`:350`, `:435`, `cmd/open_command_pending_warnings_test.go:61`), which is mild redundancy given all three share one `warning.WriteLines` call — each was requested by its own task, and none is misleading.

CODE QUALITY:
- Project conventions: Followed. The teardown writer stays an injected `io.Writer` (`cmd/open.go:621`) rather than `os.Stderr`, which is what makes the ordering testable; the model exposes the question (`WarningsOwedAtTeardown`) rather than its fields, so `cmd` does not reassemble the rule.
- SOLID principles: Good. The model answers what is owed; `cmd` decides where it goes. The sink, the model's two fields and the writer each have one owner, and `stageBootstrapWarningsOnModel` is the single handoff between sink and model.
- Complexity: Low. `finishTUI` is three statements in a stated order; the owed-set rule is one `slices.Concat`.
- Modern idioms: Yes — `slices.Concat` for the disjoint union, which is also what keeps the buffer off the staged slice's backing array.
- Readability: Good. `WarningsOwedAtTeardown`'s doc (`internal/tui/model.go:481-485`) states the invariant that makes the union correct — each field is emptied by whoever consumes it, so anything still sitting in either is unsurfaced — which is the non-obvious part.
- Issues: None. The comment at `internal/tui/model.go:1664-1665` ("Warnings arriving after dismissal are dropped") reads as governing the whole if/else chain whose second arm accumulates, but `commandPending` forces `PageProjects` at construction so no command-pending model can ever have dismissed a loading page; the claim is not falsified, and that arm belongs to task 12-1's change-set, not this one.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
