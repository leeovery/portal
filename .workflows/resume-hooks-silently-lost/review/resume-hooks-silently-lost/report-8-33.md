TASK: resume-hooks-silently-lost-8-33 (tick-2158d6) — "Four Command-Driver Twins And Four Call-Log Filters In One Package"

ACCEPTANCE CRITERIA:
- One driver runs all four commands; the four twins are gone.
- `resetRootCmd` resets the state command's flags, so no caller sequences two resets.
- One call-log filter serves every query the four covered.
- No helper in the package is left without a consumer.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 implementation-analysis (duplication) task, so per the shared verifier context its
authority is its own body rather than the specification — the spec governs the resume-hook bugfix, and
this task touches only test scaffolding (`cmd`'s test drivers and `internal/commandertest`'s call-log
query). No production code is in the change-set, so no spec behaviour is at stake. The project
conventions that do bear on it are CLAUDE.md's `commandertest` contract ("the single home of the tmux
`Commander` fake … beyond the shared `harnesstest.TestingT` it reports through, it is stdlib-only") and
the `cmd` test-seam staging rules; both hold after the change.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/root_test.go:82-87` — the folded state-hydrate flag reset inside `resetRootCmd`.
  - `cmd/root_test.go:90-102` — the single `runRootCmd(t *testing.T, args ...string)` driver.
  - `internal/commandertest/scripted.go:255-299` — `MatchedCall`, `MatchedCalls`, `MatchedCalls.FirstIndex`,
    the widened `CallsMatching(cmd string, argSubstrs ...string)` and its `argvContainsAll` helper.
  - Consumers: `cmd/uninstall_test.go:52-55,73,174`; `cmd/abridged_saver_test.go:72,76,102,123,196`;
    `cmd/concurrent_bootstrap_gate_test.go:243`.
- Notes:
  - AC1 holds. A repo-wide grep for `runStateCommitNow`, `runStateDaemon`, `runStateNotify` and
    `runUninstall` across `*.go` and `*.md` returns nothing; every former call site now reads
    `runRootCmd(t, "state", "daemon")` / `(t, "state", "commit-now")` / `(t, "state", "notify")` /
    `(t, "uninstall")`. No site calls `runRootCmd(t)` with zero args (which would leave `SetArgs(nil)`
    and let cobra fall back to `os.Args[1:]`), so the variadic form introduces no trap here.
  - AC2 holds. `resetStateCmdFlags` is gone from the package, and every one of its former call sites
    (`cmd/state_test.go` ×3, `cmd/state_signal_hydrate_test.go` ×2, `cmd/bootstrap_orchestrator_test.go`,
    `cmd/version_guard_test.go`, `cmd/state_hydrate_empty_hookkey_test.go`, `cmd/root_test.go`) sat
    immediately after a `resetRootCmd()` call, so the fold is semantics-preserving at each. The folded
    loop covers `fifo`/`file`/`hook-key`, which is the complete set of flags `stateHydrateCmd` declares
    (`cmd/state_hydrate.go:293-297`).
  - AC3 holds, and the substitutions are behaviour-identical to what they replaced:
    `countOp(calls, op)` → `len(CallsMatching(op))` (both count first-element matches);
    `callIndex(calls, op, substr)` → `CallsMatching(op, substr).FirstIndex()` (both match on the
    space-joined argv including element 0, both answer -1 on no match, and an empty `substr` narrows
    nothing in either form because `argvContainsAll` over an empty list is vacuously true);
    `setHookCalls` was re-expressed over `CallsMatching` rather than re-scanning the raw log.
  - AC4 holds as delivered: `CallsMatching`, the helper the task named as consumer-less, gained eleven
    call sites across three `cmd` test files in this commit.
  - Scope check: the `resetRootCmd` + `SetArgs` + `Execute` shape still appears inline in many `cmd` test
    files (`open_test.go`, `list_test.go`, `hooks_test.go`, …), but those were never call sites of the
    four named twins and the Do list scoped the work to replacing those four and re-pointing their
    callers. Converting the rest is a package-wide refactor this task did not undertake and was not
    asked to.

TESTS:
- Status: Adequate
- Coverage: The task is a pure refactor, and every re-pointed assertion keeps its arguments and its
  claim — the diff at `eb12b37e` is a mechanical name/shape substitution with no assertion text or
  expectation altered (the one wording change is a `t.Fatalf` label, `runStateDaemon:` →
  `runRootCmd(state daemon):`, in `cmd/state_daemon_run_test.go`). The genuinely new surface —
  index pairing, `FirstIndex`, and substring narrowing — is covered by three new subtests in
  `internal/commandertest/scripted_test.go:75-114`: index pairing plus `FirstIndex` on a hit (75-90),
  narrowing that both admits and rejects (92-104, with the first `set-hook -g …` call proving `-gu`
  actually discriminates rather than passing trivially), and `FirstIndex` = -1 on an empty result
  (106-114).
- Notes: Not over-tested — each subtest names one property, and none restates a property another
  already pins. Each would fail if its behaviour broke: dropping the `Index` field fails the
  `reflect.DeepEqual` at line 84, dropping the substring narrowing makes `FirstIndex()` answer 0 at
  line 98, and returning 0 for an empty match fails line 111.

CODE QUALITY:
- Project conventions: Followed. `internal/commandertest` stays stdlib-only beside `harnesstest`
  (`strings` was already imported, `internal/commandertest/scripted.go:9-15`), so CLAUDE.md's leaf
  claim for the package still holds. No `*Deps` or function seam is assigned directly, so
  `cmd/seam_guard_test.go` is unaffected.
- SOLID principles: Good. `CallsMatching` is the single query; `FirstIndex` is a projection over its
  result rather than a second filter, which is what keeps the surface from re-growing the
  cross-product the task set out to remove.
- Complexity: Low. `CallsMatching` is one loop with two guards; `argvContainsAll` is one loop.
- Modern idioms: Yes. Named slice type with a method rather than a free function over `[][]string`,
  variadic narrowing rather than an options struct.
- Readability: Good. The three new doc comments state what each type/method answers with, and each
  holds against the code — `MatchedCalls` really is in call-log order (the loop appends in `s.Calls()`
  order), and `FirstIndex` really does answer -1 for an empty query.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
