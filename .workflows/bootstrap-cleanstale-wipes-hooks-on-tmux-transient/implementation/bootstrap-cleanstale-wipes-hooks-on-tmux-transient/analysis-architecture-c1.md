AGENT: architecture
FINDINGS:

- FINDING: `commanderFactory` is a package-level mutable seam without synchronisation or godoc
  SEVERITY: low
  FILES: cmd/bootstrap_production.go:197, cmd/cleanstale_transient_listpanes_integration_test.go:361-368
  DESCRIPTION: `commanderFactory` is a package-level `var` mutated by integration tests under `t.Cleanup` restore. The cmd package's `*Deps` convention is documented in CLAUDE.md as serial-only (no `t.Parallel()`), so absence of synchronisation is consistent, but the godoc on `commanderFactory` does not call out the no-parallel constraint, and the seam is at a different layer from `*Deps`. The other adapters take Commander via constructor injection; this is the only construction-time global in `buildProductionOrchestrator`. A future contributor adding a second per-build seam is likely to repeat the pattern.
  RECOMMENDATION: Add a one-line godoc note that `commanderFactory` shares the cmd-package no-parallel discipline. Optionally consider, in a future cycle, a `productionOverrides` struct (analogous to `*Deps`) as a structured home for additional construction-time seams.

- FINDING: `cleanStaleNoopLogger` duplicates `cmd/bootstrap.noopLogger` verbatim
  SEVERITY: low
  FILES: cmd/bootstrap_production.go:48-69, cmd/bootstrap/bootstrap.go:185-202
  DESCRIPTION: `cleanStaleNoopLogger` is a byte-identical four-method no-op stand-in for the unexported `noopLogger` in `cmd/bootstrap`. The duplication will recur every time a future cleanup-style adapter in `cmd/` (or `internal/bootstrapadapter`) needs nil-tolerance for an injected Logger.
  RECOMMENDATION: Export `noopLogger` from `cmd/bootstrap` as `bootstrap.NoopLogger` and delete `cleanStaleNoopLogger`. The nil-substitute becomes `logger = bootstrap.NoopLogger{}`. Single source of truth.

- FINDING: Hazard-guard algorithm duplicated across adapter, `portal clean` RunE, and test-local mirror
  SEVERITY: medium
  FILES: cmd/bootstrap_production.go:129-180, cmd/clean.go:60-141, cmd/bootstrap_production_test.go:90-133
  DESCRIPTION: The six-branch "load → enumerate → debug → hazard-guard → cleanStale → log removed" algorithm now exists in three places: production adapter, `portal clean` RunE, and test-local `cleanStaleAdapterT`. All three encode the same logger format strings, the same component, and the same branch decisions. Integration tests substring-match the format strings — drift between callsites would silently pass the parallel assertion against the un-drifted callsite. Rule of Three threshold crossed.
  RECOMMENDATION: Extract a package-private `runHookStaleCleanup(lister AllPaneLister, store *hooks.Store, logger bootstrap.Logger) (removed []string, err error)` helper. Both production callsites become thin wrappers; the test-local mirror is deleted; tests target the helper directly. The two callsite differences (`portal clean` early-exit on `persisted==0`, stdout "Removed stale hook:" lines) stay at the callsite as decoration around the shared core.

- FINDING: Test-local mirror `cleanStaleAdapterT` exists only because production adapter has no seam
  SEVERITY: medium
  FILES: cmd/bootstrap_production.go:89-93, cmd/bootstrap_production_test.go:86-133
  DESCRIPTION: `cleanStaleAdapter` carries `client *tmux.Client` directly — not an `AllPaneLister` interface — so the test file constructed a parallel struct (`cleanStaleAdapterT`) substituting `lister AllPaneLister` and copied the six-branch algorithm verbatim. The `AllPaneLister` interface already exists in `cmd/clean.go:13-15` for exactly this purpose at the sister callsite; the adapter could adopt it without a new export.
  RECOMMENDATION: Change `cleanStaleAdapter.client *tmux.Client` to `cleanStaleAdapter.lister AllPaneLister`. Production wiring still passes `*tmux.Client` (satisfies the interface). Tests drive the production adapter directly with `stubAllPaneLister`, deleting `cleanStaleAdapterT`. Composes naturally with the shared-helper extraction. The integration tests provide adequate end-to-end coverage that the unit-test interface seam no longer needs to be paid for in mirror code.

- FINDING: Helper duplication across `package cmd` and `package bootstrap_test`
  SEVERITY: low
  FILES: cmd/bootstrap/transient_listpanes_helpers_integration_test.go:110-336, cmd/cleanstale_transient_listpanes_integration_test.go:101-353
  DESCRIPTION: `failureMode`/`transientListPanesCommander`/`socketCommander`/`seedHooksJSON`/`hooksJSONBytes`/`resolveHooksFilePathFromEnv` declared in `package bootstrap_test` and re-declared in `package cmd`. Already divergent (smoke test and OneShot only in bootstrap_test).
  RECOMMENDATION: Lift shared Commander + env-resolution helpers into `internal/transienttest` (or a similar `internal/*test` package alongside `tmuxtest`/`restoretest`/`portalbintest`). Both packages import the same shapes.

- FINDING: `ListAllPanes` docstring's "any sentinel" claim is technically vacuous
  SEVERITY: low
  FILES: internal/tmux/tmux.go:683-707
  DESCRIPTION: The docstring says callers can `errors.Is / errors.As against any sentinel in the chain`, but `ListAllPanesWithFormat` wraps with bare `fmt.Errorf("failed to list panes: %w", err)` — the only sentinel that survives is whatever `Commander.Run` returns. `list-panes -a` has no per-target session, so no `ErrNoSuchSession`-style sentinel is added. The claim is vacuous but not misleading.
  RECOMMENDATION: No action this cycle. If future callers need finer discrimination, classify at the `ListAllPanesWithFormat` boundary alongside the existing `wrapNoSuchSession` pattern.

SUMMARY: Fix is correct and well-tested but pays the same algorithm out three times. Highest-value structural improvements: (1) extract the six-branch algorithm into a shared helper that both production callsites delegate to, and (2) swap `cleanStaleAdapter.client *tmux.Client` for `lister AllPaneLister`. Together these delete the test-local mirror, eliminate drift risk on the load-bearing log format strings, and reduce integration-test substring assertions to a single source of truth. `commanderFactory` seam and `cleanStaleNoopLogger` duplication are minor by themselves but reinforce a precedent of per-feature redeclaration over export.
