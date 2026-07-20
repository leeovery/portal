---
scope: cli-verb-surface-redesign review remediation — resolver glob-pin divergence (blocking) plus non-blocking polish (DRY, latent success-on-failure honesty, doctor copy/coverage, test parity)
cycle: 1
source: review
total_findings: 26
deduplicated_findings: 24
proposed_tasks: 6
---
# Review Report: CLI Verb Surface Redesign (Cycle 1)

## Summary
The review verdict is Request Changes on a single blocking Required Change: the divergent silent-first-match glob branch that Task 8-2 removed from `QueryResolver.Resolve` still survives in the two single-pin resolve paths (`ResolveSessionPin`, `ResolveAliasPin`) and is enshrined by two unit tests — corroborated across reports 8-2, 2-1, and 2-3, deduplicated into one task. The remaining findings are all non-blocking polish; the two `### Do now` staleness sweeps were already applied and committed and are excluded. Applying the severity/clustering filter, five worthwhile non-blocking clusters are promoted (pin-set DRY consolidation, two latent success-on-failure reporting fixes, doctor copy+coverage, a test-parity cluster, and small DRY/legibility cleanups); the purely cosmetic one-offs are discarded below.

## Discarded Findings
- Do-now #1 — doc/comment/test-assertion staleness sweep (open_targets.go:27, open_burst_run.go:49-50, reattach_integration_test.go:3, recipe_test.go:170/174, bare_root_test.go:26, CLAUDE.md:39) — already applied and committed prior to this synthesis.
- Do-now #2 — uninstall_test.go:109 `_portal-bootstrap` anchor-kill assertion — already applied and committed prior to this synthesis.
- Quick-fix #6 — merge the two duplicate `Resolve` zoxide-miss table rows (query_test.go:97-116) — trivial test dedup, no coverage change; cosmetic.
- Quick-fix #13 — fold the byte-duplicate "machine-generated hooks set via alias" subtest (hooks_test.go:825-846) — trivial test dedup; cosmetic.
- Quick-fix #15 — optional bare `domain=alias` resolve-line assertion (open_test.go:2102) — explicitly optional; marginal coverage gain.
- Quick-fix #16 — relocate `count_panes_test.go` next to its subject (index_reader.go) — pure file-organisation cosmetic; no behaviour or coverage impact.
- Idea #18 — extract a shared read-only staleness classifier (doctor checks vs pruners) and a shared throttled-gate helper (maybeRunHookCleanup/maybeRunProjectCleanup) — premature abstraction for two call sites; churn not justified and the divergence risk is already captured. Discarded (not folded into the doctor task).
- Idea #19 — new source-walking drift guards for the `resolve`/`process:exec` single-site invariants and a doctor `WalkDir`/`portal.log` guard — speculative new guard-test infrastructure, not a found defect; defer.
- Idea #22 — name/comment the `_portal-bootstrap`-independence intent in doctor_test.go:588 — trivial test naming.
- Idea #23 — harden `TestRetiredSurface_AbsentFromCompletion` against a future completion-V2 migration (retired_surface_test.go:123-135) — acknowledged by the reviewer as acceptably narrow given the child-registration test's redundancy.
