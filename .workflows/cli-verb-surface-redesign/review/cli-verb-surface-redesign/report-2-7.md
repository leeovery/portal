TASK: cli-verb-surface-redesign-2-7 — Command with no target → Projects (mint-only) picker with banner

ACCEPTANCE CRITERIA (plan row 2-7 + Phase 2 AC):
- `open -e <cmd>` / `open -- <cmd>` (no target) → picker in Projects mode with `Pick a project to run <cmd>` banner (preserved, NOT a usage error)
- `-f <text> -e <cmd>` → filtered Projects picker (distinct from `-f` alone → Sessions page)
- banner wording exactly `Pick a project to run <cmd>`
- Projects-only mode always mints a fresh session

STATUS: Complete

SPEC CONTEXT:
Spec § "Mint-only command with no target → picker in Projects mode" (§215-221): `open -e <cmd>` / `open -- <cmd>` with no target opens the picker restricted to Projects (mint-only) mode with a `Pick a project to run <cmd>` banner — preserved exactly from today, not a usage error. A pending command switches the picker into Projects mode, and Projects only ever mint a fresh session, so the command always lands in a clean session. `-f <text> -e <cmd>` likewise coheres (filtered Projects picker running the command). The command's only error case is zero mint targets in an all-attach explicit set (that is Task 2-6, not this task). This is a "preserved from today" carve-out: the command-pending → Projects-mode mechanism (WithCommand) and the §11.4 banner predate this feature (built by the Modern Vivid reskin, task 4-4); this task's deliverable is the CLI routing that carves the no-target command case OUT of Task 2-6's usage-error surface and threads it into the picker.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/open.go:274-276 — no-target case (`destination == ""`) returns `openTUIFunc(cmd, "", command, …)`, threading the command into the picker. Positioned AFTER the --ack guard, -f branch, multi-target gate, and pin dispatch, so `open -e claude` (no positional, no pin) reaches it and resolution/openResolved never runs — the Task 2-6 mint-scoped *SessionResult guard cannot fire.
  - cmd/open.go:206-217 — the -f branch permits an accompanying command (`only the command (-e/--) may accompany it`) and returns `openTUIFunc(cmd, filterVal, command, …)`, so `-f <text> -e <cmd>` threads both filter + command; `-f` + a pin/positional is still a usage error.
  - cmd/open.go:464-504 (parseCommandArgs) — `open -e claude` → (command=[claude], dest=""); `open -- claude` → dashIdx=0 so dest="" and command=[claude]. Both yield an empty destination → the no-target path.
  - cmd/open.go:709 → internal/tui/build.go:245-250 — Deps.Command / InitialFilter wired via `WithCommand` / `WithInitialFilter`.
  - internal/tui/model.go:756-763 (WithCommand) — a non-empty command sets `commandPending=true` and `activePage=PageProjects`.
  - internal/tui/model.go:1834-1848 (evaluateDefaultPage) — command-pending unconditionally forces PageProjects regardless of session-list contents.
  - internal/tui/model.go:1857-1868 (applyInitialFilter) — in command-pending mode the else branch applies the filter to the projectList (FilterApplied), so `-f <text> -e <cmd>` lands on a filtered Projects list.
  - internal/tui/notice_band.go:81 — `commandBandText = "Pick a project to run"` (single source of truth); renderCommandBand (line 292-302) composes `▌` bar + `▸` caret + fixed text + the joined command in an orange chip.
  - internal/tui/model.go:2573-2581, 2656-2665, 3738-3747 — the Projects page mints only: handleProjectEnter → createSession → CreateFromDir(dir, m.command); handleNewInCWD → createSession(m.cwd). There is no attach path from Projects, so Projects-only mode always mints.
- Notes: Banner is rendered as fixed text + a separate orange command chip (Modern Vivid §11.4), not a literal contiguous `Pick a project to run <cmd>` string. This is the pre-existing MV rendering the spec's `<cmd>` placeholder maps onto; the visible reading is "Pick a project to run <command>". Within scope of the "preserved from today" clause — not a drift.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/open_test.go:2713 TestOpenCommand_CommandNoTarget_ExecFlag_OpensProjectsPicker — `open -e claude` (no target): asserts openTUIFunc called, gotFilter=="", gotCommand==[claude], and NO resolution outcome (session/path) fires and the query resolver is never consulted (Task 2-6 guard must not run). Directly proves "not a usage error" + command threaded.
  - cmd/open_test.go:2785 TestOpenCommand_CommandNoTarget_DashDash_OpensProjectsPicker — same contract via `open -- claude`.
  - cmd/open_test.go:2853 TestOpenCommand_Filter_ThreadsCommandToPicker — `open -f web -e claude`: gotFilter=="web", gotCommand==[claude] (the `-f <text> -e <cmd>` routing).
  - cmd/open_test.go:2426 TestOpenCommand_Filter_OpensPickerPrefilteredAndSkipsResolution — `-f blog` alone: gotCommand==nil (distinctness anchor for `-f` alone).
  - internal/tui/command_pending_band_test.go:102 TestCommandBand_FixedTextConstant — pins commandBandText byte-exact to "Pick a project to run" (the spec-exact banner wording).
  - internal/tui/command_pending_band_test.go:166 TestViewProjectList_CommandPendingBandOverFullChrome — full Projects view renders both the banner text AND the joined command over the full Projects chrome; legacy "Select project to run" wording is absent.
  - internal/tui/model_test.go:965 & 2262 (WithCommand([]{"claude"}).WithInitialFilter("myapp")) — command + initial filter lands on filtered Projects (banner shown; projectList FilterApplied with value "myapp"), and model_test.go:2281 "no command starts in session list view" confirms `-f`/no-command lands on Sessions — the distinctness pair.
  - internal/tui/command_pending_band_test.go:279 TestCommandPending_DispatchParity — Enter (run here) and n (run in cwd) both route to CreateFromDir with the command forwarded unchanged; Esc cancels. Proves Projects-only mode mints.
- Notes: Correct CLI/TUI layering — CLI tests verify routing/threading; TUI tests verify the mode decision (PageProjects), banner text, filtered-Projects behaviour, and mint-only dispatch. No under- or over-testing observed for this task.

CODE QUALITY:
- Project conventions: Followed. Package-level *Deps + openTUIFunc seam injected/restored via t.Cleanup; no t.Parallel; DI seams honoured (golang-testing/golang-cli conventions).
- SOLID principles: Good. Routing is a single ordered chain in RunE with each branch responsible for one concern; the banner text is single-sourced (commandBandText) and the band render is one function.
- Complexity: Low. The no-target carve-out is one early-return; the -f-plus-command allowance is one condition.
- Modern idioms: Yes (slices.Equal in tests, cobra ArgsLenAtDash for `--`).
- Readability: Good. cmd/open.go:197-217 comments explain the -f + command carve-out and its Task 2-7 role; model.go/notice_band.go comments document the command-pending → Projects flow.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
