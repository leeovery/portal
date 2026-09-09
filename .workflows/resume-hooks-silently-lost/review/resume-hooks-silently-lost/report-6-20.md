TASK: resume-hooks-silently-lost-6-20 — Close The Low-Severity Helper Duplication In The Hooks Test Surface
(tick-e6883a; commit eae5b896, preceded by one recorded fix round in
`.workflows/resume-hooks-silently-lost/implementation/resume-hooks-silently-lost/fix-tracking-resume-hooks-silently-lost-6-20.md`)

ACCEPTANCE CRITERIA:
- No inline re-implementation of an assertion the package already exports as a helper remains in the hooks test surface.
- The 20× lock-bound multiplier appears once.
- The sidecar-free probe appears once.
- Verdict count unchanged.

STATUS: complete

SPEC CONTEXT: This is a phase-6 consolidation task, so its authority is its own body rather than the
specification; the spec's §9.2 table nonetheless names the behaviours the touched suites carry (hook-key
resolution, `hook set`/`hook rm` exit contracts, reaper shape-awareness, `hooks.json` concurrency under
§6). That makes the constraint on a helper consolidation here a strict one: the assertions those suites
make must keep exactly the teeth they had. Every conversion was checked against that.

IMPLEMENTATION:
- Status: Implemented (all seven Do items, a–g)
- Location:
  (a) `cmd/testhelpers_test.go:230` names the shared byte-identity assertion, delegating to
      `internal/hookstest/hooks.go:113`; nine call sites in `cmd`
      (`cmd/doctor_test.go:911,1257,1422,1576`, `cmd/hooks_write_lock_test.go:78,205,222`,
      `cmd/hook_prune_single_report_test.go:61`, `cmd/hooks_rm_exit_test.go:349`) and five in
      `internal/hooksweep` (`lock_timeout_test.go:39`, `sweep_test.go:33,57,247,402`, which reach
      `hookstest.AssertHooksFileUnchanged` directly). The raw `os.ReadFile` + `t.Fatalf` pairs the task
      names are gone — `readFileBytes` (`cmd/testhelpers_test.go:130`) is the single read.
  (b) `standDownRecord` / `lockStandDownRecord` are collapsed into one `assertStandDown(t, sink, level,
      reason)` at `cmd/hookkey_vocabulary_test.go:236`, selecting the record set from the level exactly as
      the two originals did (DEBUG: exactly one record, nothing at WARN or above; WARN: exactly one at or
      above WARN, so the degraded pre-read's own DEBUG does not disturb the count).
  (c) `assertReturnsAtLockBound` at `cmd/hooks_write_lock_test.go:43`, driven from both subtests
      (`:176`, `:233`). Its signature is `run func() error` — the narrowing the fix round's NOTES
      suggested, adopted.
  (d) `hookstest.AssertSidecarFree` at `internal/hookstest/hooks_lock.go:76`, over the existing
      `openSidecar` (`:87`), consumed from `internal/hooks/lock_test.go:210,231,244`,
      `internal/hooks/read_lock_test.go:73` and `internal/hooksweep/snapshot_order_test.go:51`.
  (e) One hydrate builder: `hydrateCfgOpts` / `hydrateCfg` at `cmd/state_hydrate_test.go:977,995`.
      `replayCfg` and `timeoutCfg` are gone and the inline literal in
      `cmd/state_hydrate_empty_hookkey_test.go` is converted; the only remaining `hydrateConfig{}`
      literals are the builder's own return (`:1019`) and production (`cmd/state_hydrate.go:275`).
  (f) The two `cmd/state_daemon_test.go` sites now call `hooksFileInTempDir` (`:719`, `:740`), as does
      `cmd/version_guard_test.go:138`. The two `PORTAL_HOOKS_FILE` `t.Setenv` calls still in
      `state_daemon_test.go` (`:768` seeds through a store at that path, `:809` sets it empty to force
      home-dir fall-through) are deliberately different fixtures, not missed conversions.
  (g) `internal/hooks/lookup_test.go:17` holds `assertNoHook`, used by seven subtests; the five-line seed
      preamble is now a one-line `hookstest.StageStore` per case.
- Notes: Files this task edited under `cmd/run_hook_stale_cleanup*` and `cmd/hook_sweep_*` were later
  relocated to `internal/hooksweep` by task 9-12. The consolidation survived the move intact — the moved
  suites call `hookstest.AssertHooksFileUnchanged` and a package-local `assertStandDown`
  (`internal/hooksweep/helpers_test.go:114`) rather than re-opening the assertions. That second
  `assertStandDown` is a cross-package sibling forced by the move (a `cmd` test cannot reach an unexported
  helper in `internal/hooksweep`), not a duplicate this task left behind.

TESTS:
- Status: Adequate
- Coverage: Verdict count measured per touched file across the commit (count of `func Test` + `t.Run(`
  before and after `eae5b896`): doctor_test 87/87, hook_sweep_lock_timeout 15/15,
  hook_sweep_snapshot_order 5/5, hooks_write_lock 10/10, run_hook_stale_cleanup 33/33,
  state_hydrate_empty_hookkey 6/6, state_hydrate_replayed_log 8/8, state_hydrate 56/56,
  state_hydrate_timeout_log 5/5, cleanstale_snapshot 10/10, hooks/lock_test 17/17, hooks/lookup_test 12/12
  — unchanged everywhere, as the criterion requires.
- Notes:
  - Each conversion preserves what it replaced. `assertNoHook` keeps all three legs (nil err, ok false,
    empty cmd); `assertStandDown` keeps both counting rules; `assertReturnsAtLockBound` keeps the
    lower bound, the 20× ceiling and the non-nil-error precondition.
  - The hydrate builder wired both production handlers at this commit, matching what `replayCfg` and
    `timeoutCfg` supplied, so no converted case changed route. (The loud `unexpectedTimeout` /
    `unexpectedFileMissing` stand-ins and the `AbsentHandler` selector at `cmd/state_hydrate_test.go:936,
    945, 1037` came later and strengthen the builder further.)
  - One benign semantic narrowing, recorded rather than raised: the shared assertion compares with
    `bytes.Equal` (`internal/hookstest/hooks.go:119`) where three converted sites used
    `reflect.DeepEqual`, so an absent file and a zero-byte file now compare equal. Unreachable in
    practice — the store writes through `json.MarshalIndent` (`internal/hooks/store.go:104`), whose
    smallest output is `{}`, so no route can produce the zero-byte file that would be needed to exploit
    it. Nothing to act on.
  - `internal/hooks/store_test.go:26` keeps a package-local `readFileBytes` with the opposite ENOENT rule
    (fatal, not nil) and says why in its comment — a considered divergence, not a bypass.

CODE QUALITY:
- Project conventions: Followed. Helpers call `t.Helper()`; the promoted `hookstest` helpers keep the
  package's test-only shape; `AssertLockWarn`'s `harnesstest.TestingT` parameter is retained where its own
  failure path is under test. No production file was touched by this task.
- SOLID principles: Good — each promoted helper has one job and one home.
- Complexity: Low. `assertStandDown` carries the one branch that earned the merge (which record set the
  count is taken over) and its comment states the reason.
- Modern idioms: Yes.
- Readability: Good. The fix round's two comment corrections landed: `hydrateCfgOpts`
  (`cmd/state_hydrate_test.go:964-968`) no longer claims a universal default for `Logger`/`HookStore`,
  and `AssertSidecarFree` (`internal/hookstest/hooks_lock.go:74-75`) names its `O_CREATE` side effect, so
  it cannot be mistaken for a proof the sidecar is absent — the assertion
  `internal/hooksweep/snapshot_order_test.go:116` makes instead with `os.Stat`.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
