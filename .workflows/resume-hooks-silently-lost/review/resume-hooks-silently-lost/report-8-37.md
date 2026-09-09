TASK: resume-hooks-silently-lost-8-37 — `Client.SendKeys` Has No Production Callers And Is Exported Anyway (tick-a1d4e4)

ACCEPTANCE CRITERIA:
- `SendKeys` is unchanged in behaviour and still exported.
- Its declaration says why it has no production caller.
- No claim in the repository asserts the client exports no method without a production caller.

STATUS: complete

SPEC CONTEXT: None applicable. This is a phase-8 implementation-analysis (quality) task, so its authority is its own body; the specification never mentions `SendKeys` or `send-keys` (grepped: zero hits in `.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md`). The surrounding context is the dead-export sweep that deleted `Client.ResolveStructuralKey`, `Client.ListAllPanes` (task 6-9) and `Client.ListPanes` (task 7-31); this task's judgement was that `SendKeys` is materially different — a shared real-tmux test driver with three drivers in two packages — so it should be kept and documented, and the over-broad prose claim narrowed instead.

IMPLEMENTATION:
- Status: Implemented as specified, and subsequently superseded by a later plan task (not a regression).
- Location:
  - Delivered change: commit `a95daf0a` (`impl(resume-hooks-silently-lost): Tresume-hooks-silently-lost-8-37 — state why SendKeys has no production caller`) — the sole edit was a four-line doc comment on `Client.SendKeys` in `internal/tmux/tmux.go`, stating it has no production caller and is a sanctioned shared test driver. Behaviour and export were untouched (the diff adds comment lines only).
  - Claim narrowing: commit `f5f375b5`, staged in the same breath, narrowed the over-broad outcome statement from "the tmux client exports no method with zero production callers" to "…no **pane-listing** method…" in `.workflows/resume-hooks-silently-lost/implementation/resume-hooks-silently-lost/analysis-tasks-c1.md:225` and `analysis-tasks-c2.md:947,951,953`, which is where the claim actually lived. CLAUDE.md's `tmux` row and the Go sources made no such claim at that commit (verified by `git grep` at `a95daf0a^`: the only "production caller" hits were `internal/restore/session.go:34`, `internal/spawn/command.go:3` and `internal/theme/loader_construction_guard_test.go`, none of them the claim), so Do-item 2's "checking CLAUDE.md's `tmux` row and any in-source statement" correctly resulted in no edit there. Do-item 3 (leave `SendKeys` and its drivers alone) was honoured — the commit touched one file.
  - Current tree: `Client.SendKeys` no longer exists. A **later** plan task, `resume-hooks-silently-lost-9-14` (commit `ac936233`, "the send-keys driver moves to the harness that owns the socket"), removed it from `internal/tmux/tmux.go`, dropped its name from CLAUDE.md's `tmux` row, and re-homed the driver as `internal/tmuxtest/socket.go:76` `func (s *Socket) SendKeys(t *testing.T, target tmux.Target, keys string)`, documented at `internal/tmuxtest/socket.go:73-75`.
- Notes: The supersession is a deliberate, later, in-plan decision, not drift or loss, and it preserves this task's substance in a stronger form. 8-37's outcome was "a shared test driver kept in one place with its role stated"; the role is now structural rather than asserted in prose — `internal/tmuxtest/socket.go:1-3` declares the package test-only ("production code must not"), and the method takes `*testing.T`, so "no production caller" is enforced by the signature instead of promised by a comment. The three drivers still route through one invocation: `cmd/bootstrap/eager_signal_hydrate_integration_test.go:154,235`, `internal/restore/exit_closes_pane_integration_test.go:35,55`, plus `internal/portaltest/isolated_env_realtmux_test.go:32-33`. The duplication class the task existed to avoid was not reintroduced.

TESTS:
- Status: Adequate.
- Coverage: The task's own Tests section required no new coverage ("No behaviour change: `SendKeys`' unit test and the two integration drivers stay exactly as they are"), and commit `a95daf0a` changed no test file — correct for a comment-only change. In the current tree the behaviour is covered by `internal/tmuxtest/socket_realtmux_test.go:84` `TestSocket_SendKeys`, whose two subtests cover the two properties that matter: keys land in the pane on the fixture's own socket (`:90-98`, capture-pane poll for the marker), and the trailing `Enter` actually runs the command rather than leaving it at the prompt (`:107-116`, sentinel file created by the typed command). Both would fail if the driver stopped appending `Enter` or dialled the wrong socket.
- Notes: Not over-tested — two subtests, one property each, no redundant argv restatement. Neither poll is unbounded (`harnesstest.PollUntil`, 2s / 20ms), and both fatal messages render the observed state.

CODE QUALITY:
- Project conventions: Followed. The delivered comment stated the "why" at the declaration rather than restating the code, which is the house style. The later relocation puts the driver in `internal/tmuxtest`, the package CLAUDE.md names for real-tmux socket fixtures, alongside `Run`/`TryRun`/`KillServer`, and it composes through `Socket.Run` so `-f /dev/null` and the isolated `-S` socket apply — which is exactly the tmux-boundary isolation invariant (never the ambient server).
- SOLID principles: Good. The move takes an export off the production client that only tests consumed, shrinking the production surface without losing the shared helper.
- Complexity: Low. A three-line method delegating to `Socket.Run`.
- Modern idioms: Yes. Takes the named `tmux.Target` type and spends it as a string at the single argv boundary, consistent with the target-type contract in CLAUDE.md's `tmux` row.
- Readability: Good. `internal/tmuxtest/socket.go:73-75` states both what the trailing `Enter` is for and that it runs on the fixture's own socket.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
