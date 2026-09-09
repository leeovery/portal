TASK: resume-hooks-silently-lost-7-31 — `Client.ListPanes` Has No Production Caller (delete it; keep the exact-coordinate-target routing proofs alive on a called method)

ACCEPTANCE CRITERIA:
- `Client.ListPanes` is gone and no seam interface declares it.
- The exact-coordinate-target routing proofs still exist and still assert the same target-vocabulary property, on a method with a production caller.
- No helper is orphaned by the deletion.
- `go build ./...`, `go test ./...` and the integration lane all pass.

STATUS: complete

SPEC CONTEXT:
The work unit's §1.3 originally listed `internal/tmux`'s `StructuralKeyFormat` / `ResolveStructuralKey` / `ListAllPanes` as "checked against the change, not changed by it". The specification's Corrigenda section records the correction authoritatively (specification.md:558): `ResolveStructuralKey`, `ListAllPanes` and `ListPanes` were all deleted during this work unit, because their only callers were the hook paths the token-keyed change replaced; `StructuralKeyFormat` survives. This task is the third and last of that family, so the deletion is the documented, spec-sanctioned end state rather than drift.

IMPLEMENTATION:
- Status: Implemented (with one deliberate, sound divergence from the Do list — see Notes)
- Location:
  - Deletion commit 087da9da (`internal/tmux/tmux.go`, `internal/tmux/tmux_test.go`, `internal/tmux/exact_session_target_test.go`, `internal/tmux/exact_session_target_realtmux_test.go`, `CLAUDE.md`).
  - `internal/tmux/tmux.go:492` — `ListPanesInSession` now sits where `ListPanes` and `parsePaneOutput` were; both are gone.
  - `CLAUDE.md:60` — the `tmux` row's windows/panes list now reads `NewWindow, SplitWindow, ListPanesInSession, …` with no `ListPanes`.
- Notes:
  - Emptiness confirmed at HEAD: a repo-wide grep for `ListPanes` across `*.go` returns no `Client.ListPanes` declaration, call or interface method. The only residual textual matches are unrelated identifiers that merely contain the substring — `internal/state/capture_test.go:47` (`answerListPanes`, a capture mock's helper), `internal/tmux/portal_saver_test.go:128` (`saverScriptListPanesFormat`), and four test *function* names in `cmd` and `internal/restore` naming the tmux `list-panes` command, not the deleted method. No seam interface in `cmd`, `internal/state`, `internal/restore` or `internal/tui` declares it.
  - Divergence from Do step 3, and why it is sound: the task said to re-point the two `exactCoordTarget` routing proofs onto a called method. The implementer instead deleted the `ListPanes` rows outright, because the identical assertions already sat in the same two tables on methods that have production callers — re-pointing would have produced a duplicate table row and a colliding subtest name. The acceptance criterion is about the *property* surviving on a called method, and it does: the mock-level table row `internal/tmux/exact_session_target_test.go:68-72` (`ListPanesInSession`, command `list-panes`, want `coordTargetForm`) pins the same command and the same `-t` form, and the real-tmux halves at `internal/tmux/exact_session_target_realtmux_test.go:97-104` (gone session must fail, not reach the prefix sibling) and `:210-220` (live session resolves every pane of every window) carry the same subject. `ListPanesInSession` has three production/consumer call sites — `cmd/state_signal_hydrate.go:31`, `internal/restore/session.go:123` and the test harness at `internal/restoretest/restoretest.go:116` — so the proofs now ride a method that is genuinely used.
  - The one angle the deleted method covered that `ListPanesInSession` does not — a `list-panes` form *without* `-s`, which addresses only the current window — is covered independently and verifiably: `internal/tmux/saver_pane_pid.go:13` and `:36` both run `list-panes -t <CoordTargetExact> -F …` with no `-s`, and both have gone-session and live-session real-tmux cases (`exact_session_target_realtmux_test.go:153-177` and `:274-298`) plus mock-table rows (`exact_session_target_test.go:79-90`). The fixture is deliberately two windows of two panes with the second current (`exact_session_target_realtmux_test.go:26-45`), which is exactly what keeps the `-s` and no-`-s` forms distinguishable. At the time of the commit that coverage lived in `internal/tmux/saver_pane_pid_realtmux_test.go` (present in the tree at 087da9da), so there was no window in which the angle was uncovered.
  - `Client.ListPanes` was the only caller of `parsePaneOutput`; it was deleted with the method. The realtmux fixture var `siblingCurrentWindowPaneKeys` existed only for the deleted subtest and went with it. `siblingAllCoords` survives and is still consumed (`exact_session_target_realtmux_test.go:217`); `StructuralKeyFormat` survives and still has a production consumer (`cmd/bootstrap/stale_marker_cleanup.go:57`); `CoordTargetExact` is used widely in production. No orphan remains.

TESTS:
- Status: Adequate — coverage moved rather than shrank
- Coverage: The deleted `TestListPanes` suite (six subtests) tested only the deleted method's own parsing and wire form, so it had no subject left. The target-vocabulary property it also touched survives in both guards on called methods: the mock table (`TestSessionTargetsAreComposedExactly`) and the real-tmux prefix-sibling pair (`TestSessionTargets_GoneSessionDoesNotReachPrefixSibling` / `TestSessionTargets_LiveSessionStillResolves`), which at HEAD run eight routes each and are cross-checked by `TestSessionTargets_EveryPerSessionRouteIsCovered` (`exact_session_target_realtmux_test.go:331-345`) so a route in the mock table with no real-tmux case fails the build.
- Notes: No over-testing introduced; this task is a net removal of ~190 lines of test and production code with no new assertions. Import sets in both edited test files remain fully used (`strings`, `fmt` in `tmux_test.go`; `slices`, `strings` in the realtmux file), so the deletion left no compile-level residue. Per the task rules I read the tests rather than executing them; the fourth acceptance criterion (`go build` / both lanes pass) is asserted by the implementation record and is consistent with everything statically observable here — no dangling symbol, no unused import, no orphaned helper.

CODE QUALITY:
- Project conventions: Followed. The `CLAUDE.md` architecture row was updated in the same commit, which the project treats as binding documentation; the specification carries a matching corrigendum. Lane placement is untouched (the realtmux file stays in the unit lane per the `internal/tmux/*_realtmux_test.go` carve-out).
- SOLID principles: Good — removing an exported method that satisfies no seam narrows the package's public surface.
- Complexity: Low (pure deletion).
- Modern idioms: N/A.
- Readability: Good. The surviving fixture comment at `exact_session_target_realtmux_test.go:23-26` ("A single-window sibling would let a form that resolves only the current window pass as if it had resolved the whole session") still holds true after the deletion — the no-`-s` saver routes are what it now discriminates, so it is not a stale rationale left behind by the removed method.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
