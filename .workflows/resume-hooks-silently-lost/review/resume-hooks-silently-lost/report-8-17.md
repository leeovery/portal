TASK: resume-hooks-silently-lost-8-17 — "tmuxtest.WaitForSession prefix-matches, so every fixture staging a prefix sibling gets a readiness guard that does nothing" (phase 8, implementation-analysis task; authority is its own body)

ACCEPTANCE CRITERIA:
- [x] `WaitForSession` does not return for a prefix sibling of the named session.
- [x] The local exact-form duplicate is gone.
- [x] The three seed helpers are one, in the package's shared-fixture file.
- [~] Both lanes pass, with no fixture's timeout newly exhausted. (Not executable by this reviewer — assessed by reading; see TESTS.)

STATUS: complete

SPEC CONTEXT: This is a phase-8 analysis task, not a specified bugfix item, so its authority is its own body. It is the scaffolding-layer echo of the work unit's production defect: tmux prefix-matches a bare `-t <name>`, so a live `foo-2` answers a query for a gone `foo`. Production closed this with the `<kind>TargetExact` vocabulary (`internal/tmux/tmux.go:392-478`), where every per-session `-t` the client composes takes `CoordTargetExact` (`=name:`) because a measured has-session parses a window/pane target first. The task carries the same rule into the test harness's readiness guard, which several fixtures in this work unit rely on precisely because they stage a prefix sibling beside the session under test.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmuxtest/socket.go:118-137` — `WaitForSession` polls `has-session -t` with `string(tmux.CoordTargetExact(name))`, and takes `harnesstest.TestingT` rather than `*testing.T` so its own timeout path is assertable.
  - `internal/tmux/hookkey_realtmux_shared_test.go:20-114` — the single parameterised seed helper: `sessionSettleTimeout`, `realTmuxFixture{socketPrefix, sessions, topology}`, `seedRealTmuxServer`, the `hookKeyFixture` convenience constructor, and the `sessionPaneIDs` / `livePanePID` pair (the latter folded in by this task).
  - `internal/tmux/saver_pane_pid_realtmux_test.go:14-22` — `seedSaverServer` replaced by `saverFixture(...)`; `internal/tmux/exact_session_target_realtmux_test.go:32-44` — `seedPrefixSiblingServer` replaced by the `prefixSiblingFixture` value; `internal/tmux/list_all_pane_hookkeys_realtmux_test.go:13,36` and `internal/tmux/resolve_hookkey_realtmux_test.go:17` re-pointed off `seedHookKeyServer`.
- Notes:
  - AC1 verified: the harness now composes the same target form production's `HasSession` composes (`internal/tmux/tmux.go:88,97` — both `string(CoordTargetExact(name))`), which is what the `WaitForSession` doc claims at `internal/tmuxtest/socket.go:120-123`. The claim holds.
  - AC2 verified by enumeration: a repo-wide `.go` search for `waitForExactSession` returns zero matches; its sole consumer (`saver_pane_pid_realtmux_test.go`) now routes through the shared helper.
  - AC3 verified: `seedSaverServer`, `seedPrefixSiblingServer` and `seedHookKeyServer` are all absent from the tree; the 18 `exact_session_target` cases, the 4 saver cases and the hook-key suites all call the one `seedRealTmuxServer`.
  - Sequencing note, not a defect: the exact-form switch in `WaitForSession` had already landed in task 8-4 (`7b96ebdc`), so this commit (`91e680d6`) delivered the *proof* of AC1 plus the deletion and the collapse. The delivered state satisfies the criterion either way. Two later tasks refined it further — 9-2 moved the form from `SessionTargetExact` (`=name`) to `CoordTargetExact` (`=name:`) on the measurement that a bare `=name` is split on a period, and 9-17 turned the helpers into the `Target` type — so the line reads as it does today rather than as the task body wrote it (`tmux.ExactSessionTarget`, a name no longer in the tree). That is the code moving past the plan's words in the right direction.
  - The collapse changed `seedHookKeyServer`'s session dir from a raw `t.TempDir()` to `filepath.EvalSymlinks(t.TempDir())`. No hook-key assertion compares a dir, and the resolved form is what tmux itself reports on macOS, so nothing is lost; it is what lets the one helper serve the `exact_session_target` suite, which does compare.

TESTS:
- Status: Adequate
- Coverage: `internal/tmuxtest/socket_realtmux_test.go:26-82` carries exactly the three cases the task named. "it does not return for a prefix sibling of the named session" stages a live `sib-2` and asserts the wait for `sib` times out with the exact message — this is the case that fails if the fuzzy form ever comes back, and it is the criterion's direct proof. "it returns once the exact session exists" stages `sib-2` then `sib` and asserts no failure, which is what stops the first case from being satisfiable by a wait that never returns at all. "it fails at its timeout when the session never appears" additionally asserts `elapsed >= 300ms`, so a helper that gave up on the first poll is caught.
- Notes:
  - The timeout paths are observed through `harnesstest.Recorder` (`internal/harnesstest/recorder.go:32-76`), whose `Fatalf` panics with a private sentinel that `Run` absorbs — so the helper stops where a real `Fatalf` would, and `captureWait` (`socket_realtmux_test.go:17-24`) reads `Fatals[0]` only after checking the slice is non-empty. Correct use.
  - Not over-tested: the first and third cases look alike but pin different properties (target exactness vs. the full timeout being spent), and neither subsumes the other.
  - AC4 ("both lanes pass, no fixture's timeout newly exhausted") is not executable here. Read against the tree: all 48 `WaitForSession` call sites wait for a session the same fixture just created under that exact name — including the two that deliberately stage a sibling (`saver_pane_pid_realtmux_test.go:55` waits for `_portal-saver` beside a live `_portal-saver-old`, and `hookkey_moved_pane_realtmux_test.go:25` waits for `mvpane-elsewhere` beside a live `mvpane`) — so no caller depends on a sibling answering, and none can newly exhaust its budget.
  - Lane placement is right: the new file carries no build tag and belongs in the unit lane per CLAUDE.md — it spawns per-test `-S` sockets only, builds no `portal` binary and starts no daemon.

CODE QUALITY:
- Project conventions: Followed. The harness composes its target through the exactness vocabulary rather than a hand-rolled `"="+name`, matching the rule the work unit established for production; `tmuxtest` stays test-only (`*testing.T`-first on the seeding helpers) and the stand-in comes from the single `harnesstest.TestingT` declaration rather than a package-local twin.
- SOLID principles: Good. `realTmuxFixture` separates the description of a fixture (socket prefix, sessions, topology) from the act of seeding it, so a suite states what it needs and the one helper decides how; `hookKeyFixture` and `saverFixture` are thin named constructors over it rather than parallel implementations.
- Complexity: Low. `seedRealTmuxServer` is a straight-line seed loop with one optional topology hook.
- Modern idioms: Yes.
- Readability: Good. `sessionSettleTimeout` names the magic 2s that was repeated at each call site, and each fixture's doc says what topology it builds and why.
- Comment accuracy: The comments in the changed code hold. `WaitForSession`'s doc names both failure modes of the fuzzy form and matches the `CoordTargetExact` call beneath it; `seedRealTmuxServer`'s parenthetical about `/private` explains the `EvalSymlinks` that is actually there; `livePanePID`'s "wire truth rather than a second Portal read" matches its raw `ts.Run`.
- Issues: None reaching the bar.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
