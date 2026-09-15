TASK: open-with-forced-filter-12-1 (tick-5b120c) — Give the bootstrap's terminal event one warnings provenance

ACCEPTANCE CRITERIA:
1. The message `Init` synthesizes on the warm route carries no warnings, and no production path reads the staged set into a `BootstrapCompleteMsg`.
2. A warning staged before a concurrent launch reaches `bufferedWarnings` alongside the orchestrator's own, the staged one first, and `PendingBootstrapWarnings()` is empty afterwards.
3. The warm route still buffers exactly the staged set — one copy of each warning, not two.
4. `bufferedWarnings` and `pendingBootstrapWarnings` never share a backing array: appending to the buffered union writes nothing into the staged slice.
5. `WarningsOwedAtTeardown` returns that union on a search-attach exit, so a warning staged before a concurrent launch is still written when the gate quits without painting a picker.
6. The `commandPending` branch, `WarningsOwedAtTeardown`'s own rule and the drop of a complete arriving after dismissal are unchanged, and `go test ./...` is green.

STATUS: complete

SPEC CONTEXT:
Specification §7.1/§7.2 make a sigil invocation a picker invocation whose soft bootstrap warnings take the in-TUI route rather than stderr, and §7.5 requires that a sigil resolving to a direct attach (K=1) still delivers its accumulated warnings — written to the terminal after teardown, before the attach, since the notice band never paints. That delivery is `finishTUI`'s `model.WarningsOwedAtTeardown()` write (cmd/open.go:626). This task removes the only route by which a warning staged before a concurrent launch could be silently eaten on the way to that write: the double-provenance `BootstrapCompleteMsg.Warnings` field.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:1559-1568 — the warm-route synthesis under the unchanged `m.progressReceiver == nil` test now emits `BootstrapCompleteMsg{}`; the `pending := m.pendingBootstrapWarnings` capture is gone.
  - internal/tui/model.go:1666-1672 — the `PageLoading` arm sets `m.bufferedWarnings = slices.Concat(m.pendingBootstrapWarnings, msg.Warnings)` (staged first) and then clears `m.pendingBootstrapWarnings`.
  - internal/tui/model.go:1673-1680 — the `commandPending` arm's `append` is untouched, as instructed.
  - internal/tui/model.go:490-494 — `SetPendingBootstrapWarnings`'s doc comment no longer claims the loading page folds the staged set into `Init`'s synthesized message.
  - cmd/bootstrap_warnings.go:42-52 — the mirror comment on `stageBootstrapWarningsOnModel` corrected the same way.
- Notes:
  - Criterion 1 verified by enumeration: the only production construction of the message carrying warnings is cmd/bootstrap_progress.go:115 (`tui.BootstrapCompleteMsg{Warnings: ev.Warnings}` — the orchestrator's own set off the progress pipe). The other two production mentions (internal/tui/model.go:1567, and the type at model.go:126) carry nothing. `SetPendingBootstrapWarnings` has exactly one production caller, cmd/bootstrap_warnings.go:51.
  - Criterion 4 is structural: `slices.Concat` always allocates, so the buffer can never alias the staged slice; the previous `m.bufferedWarnings = msg.Warnings` aliased the message's slice and, on the warm route, the staged one.
  - Criterion 5 holds in the delivered tree: the search-attach exit quits from `applySearchDecision` (internal/tui/search_decision.go:57-62) without running `surfaceBufferedWarnings`, and `WarningsOwedAtTeardown` (model.go:486) returns `slices.Concat(m.bufferedWarnings, m.pendingBootstrapWarnings)` — so the union this task builds is exactly what teardown writes. (That concat is task 12-5's change, landed after this one; at this task's own commit the accessor returned the staged set alone. The delivered state satisfies the criterion.)
  - No drift from the plan: every bullet of the Do list is present, including all four re-pointed drivers (internal/tui/model_test.go's two warm-route drivers and cmd/open_search_warnings_test.go:462 and :486, which now deliver `BootstrapCompleteMsg{}`).
  - Behaviour preserved on every live route: warm loading page → union is the staged set; concurrent → union is the orchestrator's set (cmd/root.go:122-132 returns before the sink is added to on that branch, and the only `Add` site is cmd/abridged_saver.go:22 on the abridged branch, which returns at root.go:118); command-pending → unchanged.

TESTS:
- Status: Adequate
- Coverage: All seven micro-acceptance tests exist and drive the real arms:
  - internal/tui/model_test.go:6918 "it buffers the staged set and the message's own, staged first" — asserts both present and the staged one first (criterion 2).
  - internal/tui/model_test.go:6900 "it empties the staged set once the loading gate has taken it" — asserts `PendingBootstrapWarnings()` empty after the gate takes it (criterion 2's second half).
  - internal/tui/model_test.go:7148 "its warm-route synthesized complete message carries no warnings" — drives `Init`, finds the synthesized message in the batch, asserts zero warnings and that the staged set is still on the model (criterion 1).
  - internal/tui/model_test.go:6939 "it buffers a warm route's staged set exactly once" — the no-double-count case (criterion 3).
  - internal/tui/model_test.go:6954 "the buffered union does not share a backing array with the staged slice" — stages a `len 1, cap 4` slice, appends to the buffered union and asserts the staged array's index 1 is untouched (criterion 4). Would fail against the aliasing implementation (`append(m.pendingBootstrapWarnings, msg.Warnings...)` with an empty message returns the same slice), so it observes the property rather than restating it.
  - cmd/open_search_warnings_test.go:213 "it still owes a staged warning at teardown when the gate quits on a search attach" — builds a concurrent-route model with a receiver and a deciding search form, stages one warning, delivers the orchestrator's own through `driveLoadingGates` (cmd/testhelpers_test.go:262, which also drives the dispatched decision), asserts `SearchAttached()` and that `WarningsOwedAtTeardown()` renders staged-then-orchestrator (criterion 5).
  - internal/tui/command_pending_bootstrap_test.go:149 "it leaves a command-pending model appending the message's warnings onto the staged set" — pins the untouched arm (criterion 6).
- Notes: No redundancy worth flagging — each test names a distinct property, and the two warm-route tests differ in what they assert (emptying vs. exactly-once). No test drives a staged set *and* a warnings-bearing message through a non-loading route, so nothing hand-simulates a provenance production cannot produce: of the 22 test sites constructing `BootstrapCompleteMsg{Warnings: …}`, none of the four that stage first does so on a route where the field would double-count (checked by enumerating every `SetPendingBootstrapWarnings` call site in the tree — ten, across four files).

CODE QUALITY:
- Project conventions: Followed. No new seam, no new log component, no test executing an uninjected command body; `slices` was already imported.
- SOLID principles: Good. The change removes a cross-package invariant (the loading arm no longer depends on `cmd/root.go` leaving the sink empty on the deferred branch); the message field now has one producer and one meaning.
- Complexity: Low — two lines of production change plus two comment corrections.
- Modern idioms: Yes — `slices.Concat` is the right primitive for "fresh union", and it is what `WarningsOwedAtTeardown` already uses.
- Readability: Good. Both changed sites carry a comment stating the rule rather than restating the code.
- Comment accuracy: The four comments touched or added hold against the code — model.go:1565-1566 ("the message's field is the orchestrator's set alone"), model.go:1667-1668 ("a disjoint union into a fresh slice"), model.go:490-492 and cmd/bootstrap_warnings.go:43-45. No process-artifact references.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The `commandPending` branch, `WarningsOwedAtTeardown`'s own rule and the drop of a complete arriving after dismissal are unchanged, and `go test ./...` is green." — the first three clauses are settled by reading (the `commandPending` append at internal/tui/model.go:1679, the accessor at model.go:486 and the `activePage == PageLoading` guard at model.go:1666 are all as described, and the task's own commit touched none of them). The trailing clause needs the unit lane executed: `go test ./...` from the project root, green.
