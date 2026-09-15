TASK: Deliver the concurrent bootstrap's terminal event to the command-pending picker (tick-417c10, open-with-forced-filter-7-1)

ACCEPTANCE CRITERIA:
- A cold-route command-pending model issues its progress receiver from `Init`; a warm one (nil receiver) issues nothing extra and its batch is otherwise unchanged
- A `BootstrapFatalMsg` delivered to a command-pending model sets `fatalActive`, so `FatalError()` is non-nil at teardown and the process exits non-zero with the fatal's user message
- A bootstrap fatal on `portal open -- <command>` does not mint a session
- A `SaverDownWarning` accumulated on the cold command-pending route reaches stderr exactly once, after teardown, via the pending route task 6-4 established — not the notice band, and not twice
- The loading page's behaviour is byte-identical: the picker's concurrent route still dismisses on `BootstrapCompleteMsg`, still surfaces warnings in its notice band, and still shows the destructive error frame on a fatal
- `isTUIPath`, `shouldRunConcurrentBootstrap` and the notice band are unchanged
- `go test ./...` passes; `go vet ./...`, `gofmt -l .` and `golangci-lint run ./...` are clean

STATUS: issues_found

SPEC CONTEXT: §7.6 (specification.md:312 onwards) now carries the command-pending picker as "the one member with no loading gate", protected by a hold behind the picker rather than a gate in front of it, and states that "a bootstrap fatal ends the run rather than minting". The 2026-09-15 corrigendum (specification.md:490) records that the live defect the 2026-09-14 corrigendum tracked is closed, naming `BootstrapFatalMsg` quitting a command-pending model outright as one of the four pieces — the half this task owns; the staging/replay half belongs to 7-2. §7.6 also fixes the second member of the concurrent-bootstrap set as `portal open -- <command>`, admitted by the pre-dash reading of `isTUIPath` (`cmd/root.go:171-176`, via `preDashPositionals` at `cmd/open_search.go:29-34`).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:1573-1577` — the receiver enters the batch from one unconditional append after the branch switch, so the `commandPending` arm (`:1552-1553`) subscribes. (Task 7-1 landed this as an `if m.progressReceiver != nil` append inside the branch, `c7ce79fe5`; phase 10's `tick-1df0e1` later consolidated it to the single tail append. The substance is preserved, and a nil receiver is dropped by `tea.Batch`, so the warm command-pending batch is unchanged and synthesizes nothing.)
  - `internal/tui/model.go:1673-1680` — the `else if m.commandPending` arm appends the terminal event's warnings onto `pendingBootstrapWarnings` rather than into `bufferedWarnings`; the `PageLoading` buffering gate at `:1666-1672` is untouched.
  - `internal/tui/model.go:1698-1711` — `fatalActive`/`fatalStep`/`fatalMessage`/`fatalErr` are set for every model, and a command-pending one returns `tea.Quit`.
  - `internal/tui/model.go:1843-1849` — `createSession` refuses to act while `fatalActive`, ahead of the `bootstrapInFlight` staging branch, so neither a mint nor a stage survives a fatal. Both mint entry points reach it: `handleProjectEnter` (`:1955-1965`) and `handleNewInCWD` → `createSessionInCWD` (`:2872-2882`) — those are the only two callers of `createSession`.
  - `cmd/open.go:602-628` — `processTUIResult` returns `model.FatalError()` before consulting `Selected()`, and `finishTUI` writes `model.WarningsOwedAtTeardown()` before it.
- Notes:
  - The exit path is whole: the pipe carries the orchestrator's `*bootstrap.FatalError` through on `Err` (`cmd/bootstrap_progress.go:129-139`), `Execute` prints its `UserMessage` (`cmd/root.go:206-208`) and `classify` maps it to exit 1 (`main.go:63-66`). The criterion's "exits non-zero with the fatal's user message" holds end to end.
  - Single-write is structural, not incidental: on a TUI path `PersistentPreRunE` never calls `bootstrapWarnings.EmitTo` (`cmd/root.go:112-114`, `:146-148` — both gated on `!isTUIPath`), the sink is drained onto the model by `stageBootstrapWarningsOnModel` (`cmd/open.go:741`), and `WarningsOwedAtTeardown` (`internal/tui/model.go:487`) concatenates buffered + pending once.
  - Drift from the task text is downstream consolidation, not loss: 7-2 turned `createSession` into a value-in/value-out method and added the staged-mint branch beneath this task's fatal guard; phase 10 moved the receiver append. Both leave every one of this task's criteria satisfied in the tree.
  - `isTUIPath`, `shouldRunConcurrentBootstrap` and `internal/tui/notice_band.go` carry no change from this task's commit (`c7ce79fe5` touches `internal/tui/model.go` and two test files only).

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tui/command_pending_bootstrap_test.go:59-72` — cold route issues the receiver (asserts the orchestrator's `BootstrapProgressMsg{Index: 3}` actually arrives from `Init`'s batch, not that a field is set).
  - `:74-83` — warm route produces no `BootstrapProgressMsg`/`BootstrapCompleteMsg`/`BootstrapFatalMsg`, which is the "do not synthesize on this branch" instruction pinned.
  - `:85-103` — a fatal on a command-pending model records `FatalError()` and returns `tea.Quit`.
  - `:105-130` — Enter on a fatal'd command-pending Projects page mints nothing (`creator.createdDir == ""`) and selects nothing.
  - `:132-147` / `:149-167` — the terminal event's warnings land in `PendingBootstrapWarnings()` and never in `BufferedWarnings()`, and are appended behind an already-staged set rather than assigned over it.
  - `:169-193` — the loading page still dismisses on the terminal event with both warning fields empty.
  - `cmd/open_command_pending_warnings_test.go:47-59` / `:61-77` / `:79-90` — the teardown writes the warning bytes exactly once, byte-identical to the CLI path's own rendering, and nothing when the bootstrap warned about nothing.
  - `cmd/open_command_pending_warnings_test.go:92-105` — `FatalError()` non-nil at teardown and `finishTUI` returning the orchestrator's error, with a `refusingConnector` that fails the test if the attach is ever reached.
  - The loading route's own fatal behaviour is held by pre-existing tests (`internal/tui/loading_fatal_test.go:58` `TestFatalMsg_StaysOnLoadingPage`, `:26` `TestFatalMsg_RendersErrorState`), which would catch the new `tea.Quit` leaking onto a non-command-pending model.
- Notes:
  - Each sub-test would fail if its half broke: deleting the receiver append fails `:59`, deleting the `commandPending` quit fails `:85`, deleting the `else if` arm fails `:132` and the cmd-level `:47`, deleting `createSession`'s `fatalActive` guard fails `:105`.
  - Not over-tested: seven model-level sub-tests across two files, one per criterion plus two byte-level properties of the teardown write. No redundant restatements, no assertions on internals the behaviour does not expose.
  - `initMessages` (`internal/tui/command_pending_bootstrap_test.go:27-56`) runs each batched command on its own goroutine with a 100ms cap and a buffered channel, so a slow command is skipped rather than leaked or deadlocked; the cold assertion fails safe if the receiver is never issued.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`; the `cmd` teardown test drives `finishTUI` with injected writers and a connector double rather than real tmux; the shared warning helpers (`soakedWarnings`, `wantWarningOutput`, `accumulateWarnings` in `cmd/open_search_warnings_test.go`) are reused rather than re-authored, and `accumulateWarnings` resets the package-level sink through `resetBootstrapWarnings(t)`.
- SOLID principles: Good. The change adds one branch each to two existing message arms and one guard clause; no new type, no new seam, no widened surface.
- Complexity: Low. The fatal arm is straight-line plus one `if`; the complete arm's three-way warning routing reads as one `if/else if`.
- Modern idioms: Yes. `slices.Concat` for the disjoint union on the loading branch; plain `append` for the staged one, which is the correct choice given the pending set is the model's own.
- Readability: Good. The comments state the reason rather than the mechanics, and carry no task/phase/spec references.
- Issues: One pre-existing comment on the field this change hangs off is now readable as a rule the delivered branches do not follow — see FINDINGS.

BLOCKING ISSUES:
- None

FINDINGS:
- [out-of-scope] [contained] internal/tui/model.go:237 — the `progressReceiver` field comment ends "when nil, Init synthesizes it", unqualified, but only the `PageLoading` branch synthesizes a `BootstrapCompleteMsg` for a nil receiver (`:1559-1567`); the `commandPending` branch (`:1552-1553`) and the `default` branch (`:1569-1570`) synthesize nothing, which is exactly what this task decided. Replace the clause with "when nil, the loading branch synthesizes it". (Text predates this task — it survives from the cold-path startup flip, so the wrong claim is not one this change introduced — but it is the field the change hangs off, and its remedy is comment text alone, so it can never block.) — FAILS: a maintainer reading the field's own doc while working the command-pending route concludes every nil-receiver model gets a synthesized terminal event, and "restoring" one on that branch is precisely the synthesis the task forbade and the warm command-pending route must not have.

UNSETTLED:
- "`go test ./...` passes; `go vet ./...`, `gofmt -l .` and `golangci-lint run ./...` are clean" — settled only by running the unit lane, `go vet`, `gofmt -l .` and `golangci-lint run` from the repo root; reading cannot measure a suite result or a linter verdict.
