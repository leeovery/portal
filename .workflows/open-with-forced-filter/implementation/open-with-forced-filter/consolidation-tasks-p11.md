# Consolidation Tasks: Open With Forced Filter (Phase 11)

## Task 1: Put the two remaining order-unspecified returns on the shape the phase adopted
placement: phase 11
severity: behaviour

**Problem**: Phase 11's task 2 converted four methods so that a mutation they perform can no longer be dropped by the spelling of the call. Two sites in `internal/tui/model.go` still carry the shape it removed, and the phase deleted the package's only written record of the hazard — the warning above `createSession` — while converting its four siblings away from it. `model.go:1621` is `return m, tea.Batch(cmd, m.maybeDispatchDetectionCmd())`, whose callee (`internal/tui/spawn_detect.go:47`, pointer receiver) sets `detectDispatched`; `model.go:1720` is `return m, m.exitPreviewToSessions(captured)`, whose callee (`model.go:1383`, pointer receiver) sets `activePage` and clears `m.preview`. In each, the plain operand `m` that must carry the mutation out is read in the same return statement as the mutating call, and Go leaves that order unspecified. Measured under this tree's toolchain the call is evaluated first, so both are correct today — the hazard is latent. If the order ever flipped, the first would walk the host-terminal process tree a second time per picker session (noticed as duplicated `spawn` DEBUG lines and a slower Sessions page) and the second would leave a user dismissing the scrollback preview stranded on the preview page with no way back, with no error. A repo-wide scan of `return m, …` against every pointer-receiver `Model` method found exactly these two beyond the one the phase already fixed.

**Solution**: Apply the same one-line respelling the phase's own fix used — assign the call's result to a local, then return the model and that local — at both sites. Task 2 rejected a guard test and a replacement comment by name, so neither is proposed here: the point is that no site in the package relies on an unspecified evaluation order, not that the reliance is documented. Behaviour-preserving under every toolchain, and identical under this one.

**Outcome**: No `return m, <mutating call>` shape remains anywhere in the package, so the property phase 11 established for four methods holds for the whole of it.

**Do**:
- In `internal/tui/model.go`'s `SessionsMsg` arm, hoist the detection call off the return statement: `detectCmd := (&m).maybeDispatchDetectionCmd()` on its own line after `m.evaluateDefaultPage()`, then `return m, tea.Batch(cmd, detectCmd)`. The local needs a name of its own — `cmd` is already bound to `m.applySessions(msg.Sessions)` earlier in the same arm — and the batch keeps its current order, the sessions command first.
- In `internal/tui/model.go`'s `previewDismissedMsg` arm, hoist likewise: keep `captured := m.preview.session` first (the callee zeroes `m.preview` in place), then `refreshCmd := (&m).exitPreviewToSessions(captured)`, then `return m, refreshCmd` — which is the spelling the `previewAttachBailMsg` arm immediately below already uses.
- Leave both callees untouched: `maybeDispatchDetectionCmd` (`internal/tui/spawn_detect.go`) and `exitPreviewToSessions` (`internal/tui/model.go`) keep their pointer receivers and their signatures. Add no guard test and no replacement warning comment.
- Re-run the package scan that found these two — `return m, …` statements in `internal/tui`'s non-test files whose second operand calls a pointer-receiver `Model` method — and confirm it now finds none.

**Acceptance Criteria**:
- [ ] The `SessionsMsg` arm assigns `maybeDispatchDetectionCmd()`'s result to a local in a statement of its own before the return reads `m`, and still returns a `tea.Batch` of the `applySessions` command followed by the detection command.
- [ ] The `previewDismissedMsg` arm assigns `exitPreviewToSessions(captured)`'s result to a local before the return reads `m`, with `captured` still read from `m.preview.session` ahead of the call.
- [ ] No `return m, …` statement in any non-test file of `internal/tui` calls a pointer-receiver `Model` method in its second operand.
- [ ] `maybeDispatchDetectionCmd` and `exitPreviewToSessions` are unchanged — same receivers, same signatures — and no new guard test, warning comment or test-file edit lands with this task.
- [ ] `go test ./internal/tui/...` passes with no test changed.

**Tests**: No new test is written — the mutation already lands under this tree's toolchain, so a test cannot distinguish before from after, and phase 11's task 2 rejected a guard test for this shape by name. These existing tests cover the two arms and must stay green:
- `"TestDetection_WarmSessionsEntry_DispatchesOnce"` (`internal/tui/spawn_detect_test.go`) — asserts `DetectDispatched()` on the model `Update(SessionsMsg{…})` returned, which is exactly the latch the respelling protects.
- `"TestDetection_SessionsMsgRefresh_NoReDispatch"` — a later `SessionsMsg` on a resolved model dispatches no second host-terminal walk.
- `"TestEscDismissPathUnchangedAfterBailHandlerAdded"` (`internal/tui/preview_attach_bail_test.go`) — dismissing the preview still lands on `PageSessions` and still issues exactly one `ListSessions`.
- `"TestPreviewAttachBailFlipsToPageSessions"` — the sibling arm sharing `exitPreviewToSessions` still flips the page and zeroes the preview.

## Task 2: Decide how far the completer's separator bound reaches
placement: phase 11
severity: behaviour

**Problem**: Phase 11's task 3 stopped tab completion offering search candidates for words a `--` separator hands to a trailing command. It stops one word short: the first word after the separator still takes the search arm. So `portal open api -- /usr/local/bin/tool<TAB>` is answered with live session names carrying a leading slash, and on a shell that honours Portal's no-filenames directive the path completion the user wanted is suppressed in the same breath — the only offer on screen is a session name in the command position, and accepting it produces a command line that cannot run. The task recorded this as an accepted residual, on the ground that the word is "byte-identical to the first pre-dash positional" and so cannot be told apart. Measurement shows the record wrong on both counts: the residual is not confined to the zero-target line the task quoted (`portal open api -- /po` hits it identically, and so does every arity — any line whose separator is its last complete word), and the two states are indistinguishable only within the flag set, since the raw completion argv still holds the separator and this package already recovers argv order that way (`orderedOpenTargets`, `cmd/open_burst.go`). The code also now overstates the bound: `cmd/completion.go:88-89` claims a word the separator leaves to the trailing command takes the session-name arm "however it is spelled", which the boundary word falsifies.

**Solution**: The bound stays where it is — one word past the separator. Settled to side 1 at the walk: absolute-path trailing commands are rare, and the alternative makes the completer read two sources to answer one question (the parsed flag set and the raw completion argv) in a predicate this phase just consolidated onto one, which is a permanent cost against a rare case. The rule stays stated once in `cmd/open_search.go` beside `preDashPositionals`, reading the flag set alone.

Two edits carry the settlement:
- Correct `cmd/completion.go:88-89`, which now overstates the bound. Replace "belongs to that command, so it takes the session-name arm however it is spelled." with wording that names the exception: the word immediately after the separator is, at the flag layer this predicate reads, indistinguishable from the next pre-dash positional, so it still takes the sigil arm.
- Pin the boundary case in `cmd/completion_test.go`'s `TestCompleteOpenPositionalSeparatorBound` — `portal open -- /po` and `portal open api -- /po` both still take the sigil arm — so the edge is a recorded decision rather than an untested gap.

The specification's §8.1 already records the exception (corrected this pass to match the code), so no further spec change is owed.

**Outcome**: The completer's bound has one documented exception rather than an unrecorded one, tested at both arities that reach it.

**Do**:
- Correct the doc comment above `completeOpenPositional` in `cmd/completion.go`. Its closing clause — "belongs to that command, so it takes the session-name arm however it is spelled." — is the overstatement; replace it with wording that names the exception: the word immediately after the separator is, at the flag layer `completingPreDashPositional` reads, indistinguishable from the next pre-dash positional, so it still takes the sigil arm. Wording is the executor's; the exception and its reason are what must be there.
- Add a subtest to `TestCompleteOpenPositionalSeparatorBound` in `cmd/completion_test.go` driving the zero-target boundary line through the real completion flow — `completionCandidates(t, "__complete", "open", "--", "/po")` under the suite's existing `twoSessions` seed — asserting the sigil arm's answer, `[]string{"/portal-a1b2"}`.
- Add its arity sibling in the same table — `completionCandidates(t, "__complete", "open", "api", "--", "/po")`, same expected candidates — which is the case the original residual record missed and is what makes the exception a boundary-word rule rather than a zero-target one.
- Change nothing in `cmd/open_search.go`: `completingPreDashPositional` keeps its `<=` and reads the flag set alone, and `completeOpenPositional` keeps reading that one predicate — no call site consults the raw `__complete` argv.

**Acceptance Criteria**:
- [ ] `completeOpenPositional`'s doc comment no longer claims a post-separator word takes the session-name arm however it is spelled, and states the boundary-word exception together with the reason the flag layer cannot see it.
- [ ] `TestCompleteOpenPositionalSeparatorBound` drives `open -- /po` and `open api -- /po`, each asserting candidates `["/portal-a1b2"]` — the sigil arm — rather than the session-name arm's `["portal-a1b2", "web-9"]`.
- [ ] The four existing subtests are unedited and still pass: `-- ls /po` matches `completeSessionNames("/po")` in both candidates and directive, the two no-separator lines still offer `/portal-a1b2`, and the probe-parse subtest still pins the `<=`.
- [ ] No production behaviour changes — `completingPreDashPositional`, `preDashPositionals`, `completeOpenPositional`'s body and `completeSearchTerm` are all untouched.
- [ ] No specification edit is made; §8.1 already records the exception.
- [ ] `go test ./cmd -run TestCompleteOpenPositional` passes.

**Tests**:
- `"it keeps the sigil arm for the word immediately after the separator"` — `__complete open -- /po` answers `/portal-a1b2`.
- `"it keeps the sigil arm for the boundary word beside a target"` — `__complete open api -- /po` answers `/portal-a1b2`, pinning the exception at the arity the earlier record missed.
