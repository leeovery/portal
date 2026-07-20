# Implementation Review: CLI Verb Surface Redesign

**Plan**: cli-verb-surface-redesign
**QA Verdict**: Approve

## Summary

All **65 plan tasks across 17 phases** (6 feature phases + 11 analysis/remediation cycles) have now been independently verified against their acceptance criteria and the specification, and **every task is fully complete with adequate, well-balanced tests and no blocking issues**. The governing principle (split the surface by outcome), both axioms (absorb/net-N; attach-vs-mint), the domain-pin contract, the multi-target burst, the `doctor`/`uninstall` reshuffle, the `attach`/`spawn` retirement, the `hooks`→`hook` rename, `state` hiding, and tab completion are all implemented as specified.

**The one blocking issue from the prior review is resolved.** The earlier `Request Changes` verdict rested entirely on Task 8-2's shortfall: the divergent silent-first-match glob branch was removed from `QueryResolver.Resolve` but survived in the two single-pin paths (`ResolveSessionPin` / `ResolveAliasPin`). Phase 13's remediation (Task 13-1) swept both branches — they now hard-fail loudly on any glob (no `matches[0]` collapse), the doc comments reflect the exact-only contract, `QueryResolver.Resolve` is untouched, and the two tests that had enshrined the forbidden first-match behaviour are retargeted to the loud-miss contract. Independently verified: no single-pin path can collapse a multi-match glob on any code path, including an `os.Args`-assumption break.

The analysis tail (Phases 14–17) then hardened the surface further: a typed `resolver.Domain` now replaces the two hand-aligned string vocabularies across the cmd↔resolver routing boundary (14-1); a single `isExactSession` helper unifies the three exact-match sites and their lister-error policy (14-2); doctor's host-terminal seams route through the shared `buildProductionSpawnSeams` bundle instead of a hand-built third copy (14-3); the `checkStatus` enum gains a `checkUnknown` sentinel at iota 0 so a zero-value check can never read as healthy (14-4); staleness classification is now store-owned so doctor's diagnosis and the pruner cannot drift (15-1); the down-server result shape is single-sourced (16-1); the fabricated-`Domain` latent trap in the degenerate single-surface burst is removed by threading the real `QueryResult` through (16-2); and the host-terminal resolution seam is named `spawn.AdapterResolver` at all 8 sites (17-1). Six analysis cycles reported clean for four consecutive rounds and cycle 12 proposed zero tasks — convergence is genuine.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across the full redesigned surface. The byte-pinned strings the spec calls out remain preserved (the single-target miss string with its U+2014 em-dash, the Projects-picker banner, the command-on-attach usage error), and the governed `resolve` log-component contract is intact — the typed-`Domain` refactor (14-1) explicitly sources the `domain` attr from the enum's `String()` so the emitted `session`/`path`/`alias`/`zoxide`/`miss` values are byte-identical to before. No deviations from the spec were found in the Phase 13–17 work.

### Plan Completion

- [x] Phase 1–17 acceptance criteria met (the Phase 8 partial from the prior review is closed by Phase 13's Task 13-1)
- [x] All 65 tasks completed and independently verified
- [x] No scope creep — Phases 13–17 are quality/DRY/typing remediation and review follow-up; no new user-facing surface, no behavioural change beyond the corrected glob-collapse fix and the two honest-reporting fixes (13-3)

### Code Quality

No issues found. The remediation and analysis cycles left the codebase in strong shape: the domain vocabulary is now compiler-anchored rather than convention-plus-guard-test; the exact-session match, the down-server result, the staleness predicate, and the host-terminal seam are each single-sourced; the pin set is declared once and consumed by both the exclusivity guard and the dispatch table (13-2); and two success-on-silent-failure paths were corrected — `killSaver` now discriminates transient tmux faults via `HasSessionProbe`, and the open-burst trigger-connect failure emits a corrective `spawn` WARN so the durable log stays honest (13-3). The `checkUnknown` iota-0 sentinel (14-4) brings the health enum into line with the project's `golang-naming` convention.

### Test Quality

Tests adequately verify requirements throughout, with balanced coverage and no over-testing flagged. The symmetric test-parity gaps the prior review identified are closed (13-5 added all seven sibling tests — dash-dash command spellings, the path-pin-miss hard-fail, the project-prune idle-branch, the `-s` engine subtest, the argv-scan reverse/mid-list guards, and the completion-excludes-`--ack`/`state` assertions), and the new structural guards (typed-domain routing, `TestResolveDoctorDepsUsesSharedSpawnSeams`, the zero-value-check-not-healthy test, predicate-parity tables) pin the refactors' invariants. The two tests that formerly enshrined the forbidden first-match glob collapse now assert the loud-miss contract.

### Required Changes

None. The prior blocking issue is resolved; all remaining items are non-blocking.

## Recommendations

*The prior review's recommendation set was substantially absorbed by Phase 13's remediation and a broad accompanying do-now/quickfix sweep (openTargetPins doc, OpenBurstDeps.Logger comment, the uninstall bootstrap-anchor assertion, the reattach header, the bare_root cobra comment, and the CLAUDE.md `doctor` entry are all applied). What follows are the surviving non-blocking notes from the Phase 13–17 verification plus a small residual carried forward.*

### Applied in this review session (do-now, verified: build + `go test ./...` + `golangci-lint run` all green)

- **[APPLIED] gofmt drift (4 pure-format files):** `gofmt -w` on `internal/resolver/{gitroot_test.go,zoxide_test.go}`, `internal/spawn/terminalsconfig_test.go`, `internal/tmux/portal_saver_integration_test.go` (struct-field alignment, trailing newline, doc-comment list bullet). Formatting only.
- **[APPLIED] `internal/spawn/recipe_test.go:185`** — swapped the retired `"attach"` sample token (and its coupled `want` string) to `open` (was prior Report 5-1). Same behaviour verified.
- **[APPLIED] `internal/state/count_panes.go`** — moved the pure `CountPanes` helper out of `index_reader.go` into a new `count_panes.go` so it matches the existing `count_panes_test.go` (was prior Report 8-1). No import/logic change.

### Dropped (not actually applicable)

- **`spawn.AdapterResolver` test-file sweep (Report 17-1)** — the 8 remaining sites are function *literals* (`func(spawn.Identity) (spawn.Adapter, spawn.Resolution) { … }`) assigned to fields/vars already typed `spawn.AdapterResolver`, not type spellings. Go has no way to name a literal's type in place; forcing it requires a `spawn.AdapterResolver(func(…){…})` conversion wrapper, which is *more* verbose, not cleaner. Nothing to do.

### Quick-fixes (remaining)

1. `internal/resolver/query_test.go` — add a `Resolve`-level glob→`MissResult` subtest guarding the fall-through documented at `query.go:137-142`, mirroring the pin-level loud-miss guards. Closes the coverage gap implied by the plan's reference to a `TestQueryResolver_Resolve_GlobFallsThroughToMiss` that does not exist (Report 13-1)
2. `internal/spawn/recipe.go:94` — the last gofmt-drift file, deliberately left. The implementation handoff's suggested fix (wrap `'\''` in a backtick code span) **does not work**: Go doc comments have no inline backtick code spans, so gofmt's doc-comment formatter still converts the `''` pair to a curly `”`. Making it gofmt-clean requires a small *reword* to remove the literal `'\''` from prose (e.g. defer to the `shellQuote` body below) — a wording judgment, not a mechanical edit. Best as a standalone commit

### Ideas

3. `cmd/open_surfaces.go:66` — the `switch t.Domain` has no default arm. Provably safe today (`Target.Domain` is constrained to the five emitted domains), but the task's stated outcome was "switches become exhaustive-checkable" — decide whether to wire the `exhaustive` linter (or a defensive default) so a future Target-producing domain is caught mechanically rather than silently yielding no surface (Report 14-1)
4. `internal/resolver/query_test.go:503,677` — consider consolidating the two pre-existing per-method lister-error subtests now that `TestQueryResolver_ExactSessionMatch_UnifiedAcrossEntryPoints` covers the same policy across all three entry points; defensible to keep them as full single-method characterization (Report 14-2)
5. `internal/project/store_test.go:566`, `cmd/doctor_test.go:1167` — the permission-denied cases rely on `chmod 0000`, which does not produce a permission error when the suite runs as root; a `if os.Geteuid()==0 { t.Skip(...) }` guard would harden them. Marginal — matches the codebase convention (no CI, developer never runs tests as root) (Report 15-1)
6. `cmd/open_burst_run_test.go:775` — optionally assert the corrective mint-trigger WARN carries the directory `Value` as its `session` attr; low value since the emission site is shared and already asserted on the attach path (Report 13-3)
