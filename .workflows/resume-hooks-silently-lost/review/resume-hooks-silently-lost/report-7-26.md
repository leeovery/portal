TASK: resume-hooks-silently-lost-7-26 — "Seven Bespoke Commander Fakes Across The cmd Test Files" (phase 7, implementation-analysis consolidation; authority is the task body, not the specification)

ACCEPTANCE CRITERIA:
- One scripted `Commander` fake exists and serves the retired sites.
- An unmatched argv produces an explicit, stated outcome rather than a silent zero value.
- Each surviving bespoke fake names the behaviour a script cannot express.
- Every converted test asserts on the same argv and the same results it did before.
- `transienttest`'s failure-mode contract is unchanged.
- `go test ./cmd` and the integration lane pass.

STATUS: issues_found (0 blocking; one text-only finding)

SPEC CONTEXT: None applies. Per the shared verifier context, phases 6–9 are consolidation/quality tasks the implementation generated; this one is a test-scaffolding deduplication with no user-visible behaviour and no specification clause behind it. The binding project context is CLAUDE.md's test conventions (the `commandertest` row, the "no `t.Parallel()`" and lane rules) — the fake is unit-lane, untagged, and touches no production code.

IMPLEMENTATION:
- Status: Implemented (and legitimately moved on since the task landed)
- Location:
  - `internal/commandertest/scripted.go:45` — `Scripted`, the one argv-pattern-to-result script with a recorded call log; `internal/commandertest/scripted.go:192` / `:200` — `Trim` / `Verbatim`, the single home of the Run/RunRaw contract.
  - Delivered by commit `722f7242` as `cmd/scripted_commander_test.go` (package-local); promoted verbatim to `internal/commandertest` by the later task `8-16` (`cfd0eea5`) and extended by `8-24` / `8-33`. The move is a legitimate later change, not drift from this task: CLAUDE.md's architecture table now documents `commandertest` as the fake's home.
  - `cmd/state_daemon_run_test.go:23-33` — the one surviving bespoke fake, `daemonFakeCommander`, with its reason stated at the declaration, and its `Run`/`RunRaw` routed through `commandertest.Trim` / `commandertest.Verbatim` (`:71`, `:83`).
- Notes:
  - Six of the seven named fakes are gone. Grepping `") Run(args ...string)"` across `cmd` and `internal` test files returns exactly one implementation — `daemonFakeCommander` — so no unnamed bespoke `Commander` survives in the `cmd` suites (`cmd/bootstrap`'s `panicCommander` is a different package and outside this task's named seven).
  - `cmd` now has 56 `commandertest.{New,Quiet,Delegating,FromFunc}` construction sites across 18 files.
  - AC2 holds structurally: the default reports the argv through the `TestingT` *and* returns an error (`scripted.go:229-235`), so a production path that swallows the error still leaves a failed test; quiet behaviour is opt-in at the construction site (`Quiet` / `AllowingUnmatched` / `DelegatingTo`). Every `AllowingUnmatched`/`Quiet` site I checked replaced an old fake whose own default was already `("", nil)` — no site that previously fataled on an unmatched argv was downgraded (`cmd/uninstall_test.go`, `cmd/abridged_saver_test.go`, `cmd/state_signal_hydrate_test.go` all converted a `t.Fatalf` default into the loud `New(t, …)` default).
  - AC5 holds: commit `722f7242` does not touch `internal/transienttest`, and `FailureMode` / `PassThrough` / `FailExitNonZero` / `FailEmptyStdout` stand unchanged at `internal/transienttest/commander.go:11-16`.
  - One deliberate, documented semantic change: the old fakes collapsed the Run/RunRaw split (most delegated `RunRaw` to `Run`) and returned `out` alongside a non-nil error; the shared pair now trims on `Run`, is verbatim on `RunRaw`, and returns `""` on error, mirroring `internal/tmux`. The commit message names this and the affected assertions (pane pids, list-panes rows) parse identically either way.

TESTS:
- Status: Adequate
- Coverage: `internal/commandertest/scripted_test.go` carries all five tests the task named — `"it returns the scripted result for a matching argv"` (`:32`), `"it records every call in order"` (`:55`), `"it takes the stated default for an unmatched argv"` (`:129`, with all three policies as sub-cases), the trim contract split across `"it trims Run output"` (`:179`) and `"it returns RunRaw output verbatim"` (`:187`), and `"it returns the scripted error for a failing argv"` (`:218`) — plus the side-effect (`:234`), predicate-matcher (`:253`) and function-answer (`:263`) arms. The later tasks added the `CallsMatching`/`ResetCalls`/`Strict` arms.
- Notes:
  - The loud default is genuinely observed, not merely asserted in prose: `:130` drives it through a recording stand-in and checks the reported argv, the returned error and the empty output together, so a regression to a silent `("", nil)` fails.
  - Not over-tested: each subtest pins one distinct behaviour of the fake; there is no redundant re-assertion of the same path.
  - AC4 sampled across the whole change-set (`uninstall_test.go`, `abridged_saver_test.go`, `abridged_route_test.go`, `state_daemon_test.go`, `state_daemon_capture_logging_test.go`, `state_signal_hydrate_test.go`, `state_hydrate*_test.go`, `hooks_seams_test.go`, `open_test.go`, `spawn_seams_test.go`, `version_guard_test.go`, `concurrent_bootstrap_*_test.go`): every conversion is a mechanical `cmder.Calls` → `cmder.Calls()` plus a script that reproduces the old switch arm-for-arm. Entry ordering is correct where it matters — `commandertest.When(panePIDProbe, …)` precedes the broader `Returns(…, "list-panes")` at `cmd/abridged_saver_test.go:90-91` and `:113-114`, so the pane-pid probe is not shadowed by the pane-id read.
  - `cmd/hooks_seams_test.go:119-122` narrows rather than weakens: the fake's `CommandError.Args` is now a literal, but the entry only matches the exact `show-options -p -t %999` prefix, so a differently-composed probe argv falls to the loud default instead of being answered.
  - No test execution performed (per instructions); adequacy judged by reading.

CODE QUALITY:
- Project conventions: Followed. Unit-lane and untagged, so both lanes reach it; no `t.Parallel()`; reports through the shared `harnesstest.TestingT` rather than a private stand-in; structurally typed against the `Commander` interface (`scripted.go:18-21`) so `internal/tmux`'s own tests can use it with no cycle — the reason is stated in the package doc and is true.
- SOLID principles: Good. One responsibility (script an argv, record the call, apply the trim contract); the unmatched policy is a small closed set of three, each opted into at a construction site.
- Complexity: Low. `dispatch` is a linear first-match scan with three terminal policies.
- Modern idioms: Yes.
- Readability: Good. The unmatched-argv policy and the mutex/`Errorf`-not-`Fatalf` choice are both explained where they are made (`scripted.go:34-44`), and the surviving bespoke fake explains itself at its declaration (`cmd/state_daemon_run_test.go:23-33`) exactly as the task's Do list required.
- Issues: `dispatch` returns `(string, error, bool)` — the error is not the final result, which is unconventional Go — but it is unexported, has one caller pair, and the trailing bool is what distinguishes "no entry matched" from "an entry answered with an error". Not worth changing.

BLOCKING ISSUES:
- None. All six acceptance criteria are met in substance; the lane-pass criterion was not re-run here (test execution is out of this reviewer's remit) but nothing read suggests a break.

FINDINGS:
- [in-scope] [contained] cmd/spawn_seams_test.go:51 — the failure message reads `"Exists returned false; quietCommander defaults to no error, want true"`, but no `quietCommander` identifier exists anywhere in the tree; this task introduced the name (renaming the message from `recordingCommander`) and the later promotion to `internal/commandertest` renamed the helper to `commandertest.Quiet`, which is what line 40 of the same file now constructs. Change the message to name `commandertest.Quiet`. — FAILS: when this assertion fires, the reader is told the answer comes from a helper they cannot find — grepping `quietCommander` returns only the message itself — so the diagnostic misdirects instead of pointing at `commandertest.Quiet`'s unmatched-argv default. Remedy is message text alone, so it is non-blocking.
