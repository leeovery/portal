TASK: resume-hooks-silently-lost-8-13 (tick-d996ce) — "The Temp-HOME Teardown Race Is Unguarded, And Its Remedy Already Exists Unshared In One Fixture"

ACCEPTANCE CRITERIA:
- One exported helper owns the wait; the local copy in the coldboot suite is gone.
- Every fixture registering the teardown guard also gets the wait, without opting in.
- The isolated env neutralises the shell's session directory as well as its history, in both the process env and the returned slice.
- Ten consecutive runs of the named fixture tear down cleanly.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 implementation-analysis task, not specified bugfix work — per the shared verifier context its authority is its own body rather than the specification. Its subject is the test harness, not the resume-hook mechanism: `IsolateStateForTest` re-points HOME at a `t.TempDir()`, a restored pane's interactive `$SHELL` writes per-session files there after kill-server, and the framework's `RemoveAll` of that tree races those writes and fails a test whose assertions already passed. The spec's own concern (token-keyed hook durability across reboot) is only the source of the fixtures that expose it — `TestNonContiguousWindowReboot_KeepsTokenKeyedHooks` is the observed victim.

IMPLEMENTATION:
- Status: Implemented (with one deliberate, later-refined divergence on Do item 3 — see Notes)
- Location:
  - `internal/portaltest/tmux_server_wait.go:22` — `AwaitTmuxServerGone(t, socketPath)`, the exported bounded wait; `:28` the budget/tick-parameterised `awaitTmuxServerGone` seam; `:35` `tmuxServerUnreachable`, the `tmux -S <path> -f /dev/null list-sessions` probe (argv matches `internal/tmuxtest/socket.go:17-19`'s `socketArgs`, so the probe dials the same socket the fixture created).
  - `cmd/concurrent_coldboot_integration_test.go:63-67` — `reapTmuxServer` retained as a two-line kill-then-wait composition over the shared helper; the hand-rolled poll loop it used to hold is gone. Two callers remain (`:76`, `:386`), so the local wrapper is not orphaned.
  - `internal/portaltest/teardown_guard.go:69-79` — `registerHomeQuiescenceGuard` (a package-level var seam, matching the existing `installBackstop` convention at `isolated_env.go:137`) over `registerDirQuiescenceGuard`; `:83-93` `awaitDirQuiescent`, extracted verbatim from the state-dir guard body and now shared by both.
  - `internal/portaltest/isolated_env.go:53` — the HOME-scoped quiescence wait registered inside `IsolateStateForTest`, so every isolating fixture gets it without opting in; `:46-47` the `HISTFILE`/`ZDOTDIR` pins, set before the `os.Environ()` read at `:94` so the returned slice carries them.
- Notes:
  - **Do item 2 took the offered alternative.** Rather than folding the socket wait into `RegisterStateDirTeardownGuard`, the work registers a HOME-scoped quiescence wait inside `IsolateStateForTest`. This is the wider of the two: it reaches every isolating fixture, including those that never register the state-dir guard. AC 2 is met in substance.
  - **Do item 3 was superseded within the same plan and the current code is better.** This task shipped `SHELL_SESSIONS_DISABLE=1` + `ZDOTDIR=homeDir` exactly as written; task 9-35 (`afe08b01`) replaced that with `ZDOTDIR` pointed at a directory created *outside* the framework's temp tree (`isolated_env.go:118` `shellConfigDirOutsideTempTree`). Verified against this machine's `/etc/zshrc`: it assigns `HISTFILE=${ZDOTDIR:-$HOME}/.zsh_history` unconditionally for every interactive zsh whatever it inherited, and `/etc/zshrc_Apple_Terminal:102` derives `SHELL_SESSION_DIR="${ZDOTDIR:-$HOME}/.zsh_sessions"` from the same prefix — so pointing `ZDOTDIR` at `homeDir` (as this task's own wording asked) would have left both inside the tree the framework removes, and only the outside-tree target actually closes the class. The in-source comment at `isolated_env.go:39-45` states that reasoning and holds against the system files. This is a sound divergence from the task's literal Do list, not a loss.
  - Ordering is correct and load-bearing: the wait is registered after the first `t.TempDir()` (whose lone cleanup removes the parent holding `homeDir`) and before any fixture's `tmuxtest.New`, so LIFO runs kill-server → the wait → the `RemoveAll`. The `shellConfigDirOutsideTempTree` cleanup is registered one line earlier, so its own removal lands after the wait.
  - No import cycle introduced: `internal/portaltest` now reaches `internal/harnesstest` (stdlib-only) for `PollUntil`; `internal/tmuxtest` does not import `portaltest`.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/portaltest/tmux_server_wait_realtmux_test.go:13` — one real tmux server driven across its own death. `:20` pins the probe's argv and error *direction* (a probe that never answers would make the wait return instantly with nothing to show for it); `:26` is the plan's "it returns at its bound rather than hanging on a server that will not exit" — asserts the wait spends its budget and does not exceed it by more than 2s; `:44` is the plan's "it blocks until the tmux server is unreachable" — kills on a 300ms delay so a non-polling wait returns while the server still answers, then asserts elapsed ≥ settle, the probe now reports gone, and the full budget was not burned.
  - `internal/portaltest/teardown_guard_test.go:90` — `TestIsolateStateForTest_RegistersAQuiescenceWaitOverTheTempHome` swaps the registrar seam and asserts the directory handed to it is the temp HOME. This is what pins AC 2 (registered without opting in).
  - `internal/portaltest/teardown_guard_test.go:100` — the dir-quiescence wait returns inside budget over a settled dir seeded with a `.zsh_sessions` file.
  - `internal/portaltest/isolated_env_test.go:310` — the plan's two env tests, renamed to the mechanism the code actually uses: `:316` "it points ZDOTDIR outside the framework temp tree" (asserts non-empty, not under `filepath.Dir(t.TempDir())`, and present on disk — with a `/decoy/should/not/leak` inherited value to prove it is replaced) and `:331` "it carries that ZDOTDIR in the returned env slice" (exactly one entry, equal to the process value). Together these are the plan's "neutralises the shell session directory" / "carries the shell-session env in the returned slice".
  - Downstream, `internal/portaltest/isolated_env_realtmux_test.go:16` (from task 9-35) drives a real interactive `/bin/zsh` pane to exit and asserts the temp HOME is left empty — the end-to-end observation of the whole class.
- Notes:
  - The three `TestAwaitTmuxServerGone` subtests are distinct (probe direction, bound, blocking) — no redundancy, and no mocking where a real server is the only thing that pins the argv.
  - Not covered by any test: the LIFO *ordering* of the HOME quiescence wait against the framework's `RemoveAll`. That ordering is stated in the comment at `isolated_env.go:50-53` and is structurally hard to invert (the wait cannot be registered before `homeDir` exists), and the framework's own TempDir cleanup offers no interception point — so this is a noted limit rather than a gap worth acting on.
  - AC 4 ("ten consecutive runs of `TestNonContiguousWindowReboot_KeepsTokenKeyedHooks` tear down cleanly") is a manual verification step with no durable artifact; it cannot be judged by reading and was not re-run here (test execution is outside this review's remit). The standing substitute is the real-shell fixture above, which fails if the class reopens.

CODE QUALITY:
- Project conventions: Followed. The new unit-lane test carries the `*_realtmux_test.go` name and `tmuxtest.SkipIfNoTmux`, starts no daemon, builds no portal binary and execs no built binary — so CLAUDE.md's integration-tagging rule does not reach it (`internal/session`, `internal/state` and `internal/tmuxtest` already hold untagged real-tmux tests of the same shape). The package-level var used as a test seam matches the package's existing `installBackstop` pattern, and the swap in `teardown_guard_test.go:83-85` registers its own restore. `t.Parallel()` is not used anywhere in the change.
- SOLID principles: Good. `awaitDirQuiescent` is one wait with two registrars; `AwaitTmuxServerGone` is one exported entry point over a budget/tick-parameterised seam.
- Complexity: Low. Every added function is a loop or a two-line composition.
- Modern idioms: Yes. `slices.Backward` in the recorder's LIFO replay; `strings.CutPrefix` in the env helpers.
- Readability: Good. Each comment states a reason rather than restating the code, and the ordering rationale is written where the ordering is established.
- Issues: None that clear the bar. The two near-identical budget/tick constant pairs in one package (`teardownGuardBudget`/`teardownGuardPollTick` and `tmuxServerGoneBudget`/`tmuxServerGonePollTick`) and the probe's restatement of `tmuxtest.socketArgs` are folds nobody is harmed by, and are deliberately not reported.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
