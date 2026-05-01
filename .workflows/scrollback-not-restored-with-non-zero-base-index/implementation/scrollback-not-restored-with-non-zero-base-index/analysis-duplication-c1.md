AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 3

FINDINGS:

- FINDING: `hydrationTriggerEvents` slice duplicated three times across packages
  SEVERITY: medium
  FILES: `internal/tmux/hooks_register.go:26-29`, `internal/tmux/hooks_register_test.go:28-31`, `cmd/bootstrap/reboot_roundtrip_test.go:1144-1147`
  DESCRIPTION: The two-element slice `["client-attached", "client-session-changed"]` is hand-rolled in three places — the production source of truth (`hydrationTriggerEvents`), the in-package external-test mirror (`expectedHydrationTriggerEvents`), and the new round-trip integration mirror (`leadingDashHydrationTriggerEvents`). All three were touched/added in this implementation. The spec's Part 1 explicitly anticipates the slice being extended later ("If the slice is later extended, the migration scan must follow it"). On such an extension the production list updates while both test mirrors silently under-cover. `hooks_migration_test.go` iterates `expectedHydrationTriggerEvents` in eight sites; `reboot_roundtrip_test.go::verifyHydrationHookEntries` iterates `leadingDashHydrationTriggerEvents` at line 1347. Each iteration silently drifts on extension. The round-trip docstring at lines 1137-1143 acknowledges the duplication but accepts it ("the underlying var is unexported and the round-trip test runs in the bootstrap_test external package").
  RECOMMENDATION: Export the canonical slice from `internal/tmux` (e.g. `tmux.HydrationTriggerEvents()` returning a defensive copy, or a public `HydrationTriggerEvents` var). Replace both test mirrors with consumption of the exported symbol. The `tmux_test` and `bootstrap_test` packages both already import `tmux`, so the change is mechanical.

- FINDING: portal.log scan helpers share boilerplate as parallel implementations
  SEVERITY: low
  FILES: `cmd/bootstrap/reboot_roundtrip_test.go:443-458` (`verifyNoPredictedVsLiveWarns`), `cmd/bootstrap/reboot_roundtrip_test.go:1375-1397` (`verifyNoHydrateTimeoutWarns`)
  DESCRIPTION: Both helpers were added in this implementation to assert spec AC #4 and the leading-dash regression. Each opens `portal.log`, returns silently on `os.IsNotExist`, splits on `"\n"`, iterates lines, and fails the test on a match. The only point of variation is the per-line predicate (regex match vs. dual substring match). The surrounding ~12 lines of file-IO + ENOENT-tolerance + line-iteration plumbing are byte-identical between the two functions.
  RECOMMENDATION: Extract a single helper in the same file: `assertNoLogLineMatches(t, logPath string, pred func(string) bool, failFmt string, args ...any)`. The two existing functions become one-line wrappers that supply the predicate and per-AC failure message — preserving distinct diagnostics while collapsing the IO scaffolding to a single implementation.

- FINDING: `applyBaseIndices` helper not reused by the in-package restore integration test
  SEVERITY: low
  FILES: `cmd/bootstrap/reboot_roundtrip_test.go:513-519` (`applyBaseIndices`), `internal/restore/integration_test.go:325-328` (inline four-line `set-option` block)
  DESCRIPTION: The newly-added `applyBaseIndices` issues the canonical four-call sequence (`set-option -g/-s × base-index/pane-base-index`). The pre-existing `TestPhase3Integration_RestoreUsesLiveIndicesUnderBaseIndexDrift` already performs the same four calls inline at lines 325-328 with hard-coded `"1"`. Only the round-trip test was touched in this implementation, but the helper and the inline block now coexist, encoding the same "configure tmux base indices" semantics in two shapes across two test packages.
  RECOMMENDATION: Promote `applyBaseIndices` to `internal/tmuxtest` (or `internal/restoretest` since both call sites are integration-tag tests already). Signature `(t *testing.T, ts *tmuxtest.Socket, base, paneBase int)` carries no Portal-specific dependencies, so relocation is mechanical. Then replace the inline block in `integration_test.go:325-328` with a single helper call.

SUMMARY: Three duplication points were introduced or exacerbated by this implementation: the hydration-trigger event slice now lives in three hand-rolled copies (drift-prone given the spec's own forecast of slice extension), two new portal.log assertion helpers share ~12 lines of file-IO plumbing, and the new `applyBaseIndices` helper missed an opportunity to consolidate a pre-existing inline equivalent in the in-package restore integration test.
