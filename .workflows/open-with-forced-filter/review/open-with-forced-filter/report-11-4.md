TASK: open-with-forced-filter-11-4 (tick-390004) — Put the two remaining order-unspecified returns on the shape the phase adopted

ACCEPTANCE CRITERIA: [from plan]
1. The `SessionsMsg` arm assigns `maybeDispatchDetectionCmd()`'s result to a local in a statement of its own before the return reads `m`, and still returns a `tea.Batch` of the `applySessions` command followed by the detection command.
2. The `previewDismissedMsg` arm assigns `exitPreviewToSessions(captured)`'s result to a local before the return reads `m`, with `captured` still read from `m.preview.session` ahead of the call.
3. No `return m, …` statement in any non-test file of `internal/tui` calls a pointer-receiver `Model` method in its second operand.
4. `maybeDispatchDetectionCmd` and `exitPreviewToSessions` are unchanged — same receivers, same signatures — and no new guard test, warning comment or test-file edit lands with this task.
5. `go test ./internal/tui/...` passes with no test changed.

STATUS: complete

SPEC CONTEXT: The feature specification (`.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md`) says nothing about Go evaluation order, detection dispatch or the preview-dismiss arm — greps for `evaluateDefaultPage`, `maybeDispatchDetection`, `SessionsMsg`, `exitPreviewToSessions`, `evaluation order` and `unspecified` all return nothing. The task is an analysis-cycle hygiene finding raised inside phase 11, and its real context is the phase's own sibling task 11-2 (tick-61e334), which converted four staging methods (`dismissLoadingGate`, `resolveSearchDecision`, `createSession`, `createSessionInCWD`) to value-in/value-out so the state-dropping spelling `return m, m.x()` becomes a compile error, and deleted the prose warning above `createSession` that had held the contract. 11-4 finishes that sweep for the two remaining sites whose callees keep pointer receivers, by respelling the call rather than the signature.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/model.go:1632-1633` (SessionsMsg arm), `internal/tui/model.go:1740-1742` (previewDismissedMsg arm); commit `30848009a`, which touches `internal/tui/model.go` only (+4/-2).
- Notes:
  - AC1 holds: `internal/tui/model.go:1632` is `detectCmd := m.maybeDispatchDetectionCmd()` on its own line, immediately after `m.evaluateDefaultPage()` (`:1631`), and `:1633` is `return m, tea.Batch(cmd, detectCmd)` — the `applySessions` command (bound at `:1621`) first, the detection command second, batch order preserved.
  - AC2 holds: `:1740` `captured := m.preview.session`, `:1741` `refreshCmd := m.exitPreviewToSessions(captured)`, `:1742` `return m, refreshCmd` — the capture still precedes the call whose callee zeroes `m.preview` in place (`internal/tui/model.go:1382-1387`), and the spelling now matches the `previewAttachBailMsg` arm at `:1746-1748`.
  - AC3 holds. I re-ran the package scan over `internal/tui`'s non-test files: I enumerated every `return m, …` statement (56 of them, none spanning a line break) and every pointer-receiver `Model` method declaration (83 of them). No `return m, …` second operand is a pointer-receiver `Model` method call. The second operands that are calls are: `m.burstPipe.receiver()` (`burst_progress.go:195`, `model.go:1782`) — `burstPipe` is declared `*burstProgressPipe` at `model.go:324`, so the call copies a pointer and takes no address of any part of `m`; and the value-receiver `Model` methods `searchDecisionCmd` (`search_decision.go:40`), `mintSession` (`model.go:1869`), `refreshSessionsAfterPreviewCmd` (`model.go:1365`), `deleteAndRefreshProjects` (`model.go:1528`), `loadProjects` (`model.go:1462`), `killAndRefresh` (`model.go:2799`) and `renameAndRefresh` (`model.go:2862`), none of which can mutate the returned copy. The remaining second operands are `nil`, `tea.Quit`, a plain local, a field read (`m.progressReceiver`), `flashTickCmd(m.flashGen)`, or the two `tea.Batch` calls at `:1633` and `:1748` whose arguments are locals and a field read. The scan covers four later commits that touched `model.go` after this task's (`fddd439b4`, `e44308029`, `b1b809fc4`, `54d9a9f36`) — none reintroduced the shape.
  - AC4 holds: `maybeDispatchDetectionCmd` is still `func (m *Model) maybeDispatchDetectionCmd() tea.Cmd` (`internal/tui/spawn_detect.go:47`) and `exitPreviewToSessions` is still `func (m *Model) exitPreviewToSessions(preserveName string) tea.Cmd` (`internal/tui/model.go:1382`); the commit diff touches neither declaration, adds no comment, and edits no test file. The paired record commit `79388acd1` touches only `.tick/tasks.jsonl` and the manifest.
  - Divergence, judged sound and not reported as a finding: the Do prescribed `detectCmd := (&m).maybeDispatchDetectionCmd()` and `refreshCmd := (&m).exitPreviewToSessions(captured)`; the implementation drops the explicit `(&m)`. `m` is the addressable value receiver of `Update` (`model.go:1581`), so Go takes its address implicitly and the two spellings compile to the same call. It also matches the sibling `previewAttachBailMsg` arm at `:1746`, which the task itself named as the shape to copy, and `model.go:1524`, which calls the same detection method bare. No loss.

TESTS:
- Status: Adequate
- Coverage: No test was added or changed, which is what the task prescribes — the mutation lands under this tree's toolchain both before and after, so no test can distinguish the two, and sibling task 11-2 rejected a guard test for this shape by name. The four existing pins the task names all exist and exercise the two arms: `TestDetection_WarmSessionsEntry_DispatchesOnce` (`internal/tui/spawn_detect_test.go:63`) asserts `m.DetectDispatched()` on the model returned by `Update(SessionsMsg{…})` — exactly the latch the respelling protects — and then drains the batch to assert exactly one `Detect()` call; `TestDetection_SessionsMsgRefresh_NoReDispatch` (`:157`) drives a second `SessionsMsg` through the same arm on a resolved model and asserts no re-dispatch; `TestEscDismissPathUnchangedAfterBailHandlerAdded` (`internal/tui/preview_attach_bail_test.go:204`) asserts the dismissed preview lands on `PageSessions` with exactly one `ListSessions`; `TestPreviewAttachBailFlipsToPageSessions` (`:30`) covers the sibling arm sharing `exitPreviewToSessions`.
- Notes: Not under-tested (the behaviour under test is already pinned, and the change has no observable behaviour delta to pin); not over-tested (nothing added).

CODE QUALITY:
- Project conventions: Followed. Two locals, each named for what it carries (`detectCmd`, `refreshCmd`), the latter matching the identifier the adjacent `previewAttachBailMsg` arm already uses for the same command. `refreshCmd` does not collide with `Update`'s named return `cmd`; the pre-existing shadow of that named return by `cmd := m.applySessions(…)` at `:1621` is untouched.
- SOLID principles: Good — no signature, receiver or responsibility moved.
- Complexity: Low — two statements split into four.
- Modern idioms: Yes.
- Readability: Good. No comment was added, per the task and per 11-2's stance that a prose warning is not how this property is held.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./internal/tui/...` passes with no test changed" — the "no test changed" half is settled by reading: commit `30848009a` touches `internal/tui/model.go` alone. The "passes" half needs the suite run; reading cannot settle it. Settle it by running `go test ./internal/tui/...` from the project root in the executing pass.
