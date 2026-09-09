TASK: resume-hooks-silently-lost-7-10 — "Integration-Tagged Source Carries Untouched Lint Debt, And The Issue Cap Hides Findings" (tick-52617a)

ACCEPTANCE CRITERIA:
- [ ] `golangci-lint run ./...` reports 0 issues with no flags.
- [ ] The config sets the integration build tag and both issue caps to 0.
- [ ] The 21 findings are fixed, not excluded or nolint-suppressed.
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both pass after the rewrites.
- [ ] CLAUDE.md's lint line matches how the config now behaves.

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task (per the shared verifier context, phases 6–9 are consolidation/quality tasks the implementation generated). Its authority is its own body, not the bugfix specification — the specification says nothing about lint invocation, and nothing in it bears on this change. The binding project context is CLAUDE.md's lint sentence and the two-lane build rule (`go test ./...` vs `-tags integration -p 1 ./...`).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `.golangci.yml:7-22` — new `run.build-tags: [integration]` block with the rationale comment, and `issues.max-same-issues: 0` / `issues.max-issues-per-linter: 0`. The pre-existing `testmain_isolation_test.go` errcheck exclusion is retained at `.golangci.yml:32-41`.
  - `CLAUDE.md:19` — lint sentence rewritten: states the config sets the `integration` tag and uncaps both limits, that no second `--build-tags integration` invocation exists, and names the residual blind spot (`//go:build !integration` files).
  - Rewrites (commit `11c155bb`), all still present in the current tree:
    - rangeint ×14: `cmd/bootstrap/composition_e2e_harness_integration_test.go:227`, `cmd/bootstrap/composition_e2e_prefix_integration_test.go:53`, `cmd/bootstrap/orphan_sweep_integration_test.go:277`, `cmd/state_daemon_hysteresis_measurement_test.go:91`, and 10 in `internal/restore/integration_full_test.go` (the `for w := range 2` / `for p := range 2` pairs and singletons — counted: 10 occurrences).
    - stringsseq ×4: `cmd/bootstrap/phase5_marker_suppression_integration_test.go:96`, `cmd/bootstrap/reboot_roundtrip_test.go:171` and `:276`, `cmd/doctor_fix_transient_listpanes_shared_integration_test.go:152`.
    - stringscutprefix ×1: `cmd/doctor_fix_transient_listpanes_shared_integration_test.go:136`.
    - stringscut ×1: `cmd/state_daemon_hysteresis_measurement_test.go:225`.
    - minmax ×1: `cmd/state_daemon_hysteresis_measurement_test.go:108`.
  - Enumerated total: 14 + 4 + 1 + 1 + 1 = 21, matching the task's stated finding breakdown exactly.
- Notes:
  - Every rewrite is semantics-preserving on inspection:
    - `min(max(doubled, 3), 9)` reproduces the two sequential `if` clamps (floor 3 then ceiling 9) exactly; the clamp assertion at `cmd/state_daemon_hysteresis_measurement_test.go:123` still reads `3 ≤ N ≤ 9`, so code and assertion agree.
    - `strings.Cut(e, "=")` matches the old `IndexByte`/slice pair including the `ok == false` → `continue` branch and the empty-value case (`FOO=` still yields `v == ""`).
    - `strings.CutPrefix` matches the old `HasPrefix`+`TrimPrefix` pair.
    - `strings.SplitSeq` yields the same element sequence as `strings.Split` for all four line-iteration loops.
    - `for range 2` at `cmd/bootstrap/orphan_sweep_integration_test.go:277` drops the loop variable; I read the body at the commit and `attempt` was unused inside it, so the rewrite compiles and iterates twice as before.
  - Language-version safety: `go.mod` declares `go 1.26.0`, so range-over-int (1.22) and `strings.SplitSeq`/`CutPrefix`/`Cut` (≤1.24) are all available. `hysteresisRunsPerScenario` (`cmd/state_daemon_hysteresis_measurement_test.go:37`) is an untyped int constant, so ranging over it is legal.
  - No `//nolint` directive and no new exclusion rule appears anywhere in the commit — the findings are fixed rather than suppressed, as the criterion requires.
  - The `!integration` blind spot the config introduces is real (`internal/state/pgrep_sandbox_prod.go` is a production file carrying `//go:build !integration`, and is no longer analysed). It is an owned, documented trade — named in both the config comment and CLAUDE.md line 19 — and the excluded file is eleven lines of inert stubs, so it is not a loss worth reversing.
  - Verification limits: as a reading reviewer I did not execute `golangci-lint` or either test lane, so AC-1 ("0 issues") and AC-4 ("both lanes pass") are judged structurally — all 21 named findings are gone from the sources, the rewrites are semantics-preserving, and no residual candidate of the same shape survives in an integration-tagged file (the only similar loops left are `for i := 0; i <= len(s); i++` at `cmd/reattach_integration_test.go:103`, whose `<=` is not the rangeint pattern, and unit-lane files such as `cmd/state_daemon_self_supervision_test.go:272` whose `i < N-1` shape the linter already reported clean before this change).

TESTS:
- Status: Adequate (correctly none added)
- Coverage: The task is a config change plus mechanical rewrites confined to `_test.go` files with no behaviour change, so its verification is the linter's own output plus the two existing lanes. Adding a test here would be over-testing. The rewritten files carry the suites the task named as the ones that must survive unchanged: `internal/restore/integration_full_test.go`'s round-trip assertions (`verifyTopologyShape`, `verifyLiveStructure`, `verifyZoomFlags`, `verifyActivePanes` — all four rewritten loops still index `s.Windows[w]` / `activePanes[w]` identically) and the `cmd/bootstrap` composite suites.
- Notes: The rewrite most capable of changing behaviour — the clamp fold in the hysteresis harness — is itself guarded by the two invariant assertions immediately below it (`cmd/state_daemon_hysteresis_measurement_test.go:117` and `:123`), which are unchanged and would fail if the clamp bounds moved. No test lost an observation to the rewrites.

CODE QUALITY:
- Project conventions: Followed. The config change is the documented "one bare invocation" route; CLAUDE.md was updated in the same commit so the binding project doc and the config cannot disagree. No test file was moved between lanes, and no `t.Parallel()` was introduced.
- SOLID principles: N/A (config + mechanical test rewrites).
- Complexity: Low — the only complexity change is a reduction (five lines of clamp branches to one expression).
- Modern idioms: Yes — that is the whole change: range-over-int, `strings.SplitSeq`, `strings.Cut`, `strings.CutPrefix`, builtin `min`/`max`.
- Readability: Good. The `.golangci.yml` comments state why the tag is set here rather than passed per-invocation and what the tag cannot see, so a future reader does not have to rediscover the trade.
- Comment accuracy: The three comments touched or added all hold against the code. `.golangci.yml:8-14` correctly describes both the inclusion and the `!integration` exclusion; `.golangci.yml:19-20` correctly names the defaults it overrides; CLAUDE.md:19 matches the config byte-for-byte in substance.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
