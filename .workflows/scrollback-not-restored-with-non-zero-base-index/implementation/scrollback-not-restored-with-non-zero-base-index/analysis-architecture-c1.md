AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 4

FINDINGS:

- FINDING: `MigrationLogger` interface duplicates the existing `bootstrap.Logger` / `*state.Logger` shape rather than reusing one
  SEVERITY: low
  FILES: internal/tmux/hooks_register.go:131-142, internal/bootstrapadapter/adapters.go:60-70
  DESCRIPTION: A new `tmux.MigrationLogger` interface (`Info`, `Warn` with `(component, format, args...)`) was introduced in `internal/tmux/hooks_register.go` to satisfy the migration's logging needs. `*state.Logger` already provides exactly that signature, and `cmd/bootstrap` already defines a `bootstrap.Logger` seam used by the orchestrator. The justification in the doc comment cites a hypothetical dependency cycle ("internal/state imports internal/tmux transitively via its callers"), but the tree as-is has no such cycle — `internal/tmux` does not import `internal/state`. The smell is duplication of an established convention, not a genuine fault.
  RECOMMENDATION: Optional: when next touching this code, consider whether `tmux.MigrationLogger` could be replaced with a re-export of an existing logger interface (or moved to a shared leaf package) so there is one canonical structural-logger seam used by both the bootstrap orchestrator and the tmux migration. No action required for this fix.

- FINDING: Two parallel `RegisterPortalHooks` / `RegisterPortalHooksWithLogger` entry points create a pit-of-failure split
  SEVERITY: low
  FILES: internal/tmux/hooks_register.go:233-282, internal/bootstrapadapter/adapters.go:68-70
  DESCRIPTION: The package now exposes both `RegisterPortalHooks(c)` (no logger) and `RegisterPortalHooksWithLogger(c, log)`. The doc claims the former is the "stable external-caller surface for sites that do not have a logger in hand," but the only production caller (`bootstrapadapter.HookRegistrar`) already has a logger and calls the `WithLogger` form. The no-logger entry point is used only by `hooks_register_test.go`. The internal guard already does `if log == nil { log = noopMigrationLogger{} }`, so the "pretty" form adds no safety — only a name. Future regressions could route production through the no-logger form and silently lose eviction visibility.
  RECOMMENDATION: Consider collapsing to a single `RegisterPortalHooks(c *Client, log MigrationLogger) error` that tolerates `nil`. Tests pass `nil` or a recorder; production passes the orchestrator logger. One name, one shape, zero footguns. Local change to `internal/tmux` plus a one-line update at the `bootstrapadapter.HookRegistrar` call site.

- FINDING: `hydrationTriggerEvents` shadowed twice across package boundaries
  SEVERITY: low
  FILES: internal/tmux/hooks_register.go:26-29, internal/tmux/hooks_register_test.go:28-31, cmd/bootstrap/reboot_roundtrip_test.go:1144-1147
  DESCRIPTION: The canonical `hydrationTriggerEvents` is unexported. Two test-side shadows exist: `expectedHydrationTriggerEvents` (in-package, justified) and `leadingDashHydrationTriggerEvents` (cross-package in `cmd/bootstrap`). The cross-package shadow is rationalised as "duplicated rather than imported because the underlying var is unexported" — but the spec § Fix Scope is explicit that "if the slice is later extended, the migration scan must follow it." A drift between the canonical list and the integration shadow would silently under-cover any newly-added event in the highest-fidelity end-to-end test.
  RECOMMENDATION: Export the list (e.g. `tmux.HydrationTriggerEvents`) and consume it directly from both test files. Eliminates two shadows and one drift risk at the cost of one exported identifier.

- FINDING: `MigrateHydrationHooks` exported despite being conceptually internal to `RegisterPortalHooksWithLogger`
  SEVERITY: low
  FILES: internal/tmux/hooks_register.go:189-227, internal/tmux/hooks_migration_test.go:346,395,441
  DESCRIPTION: `MigrateHydrationHooks` is exported, and the doc comment pins it as "intended to be called once per bootstrap, immediately before the install step reaches the hydration-trigger category" — which is exactly what `RegisterPortalHooksWithLogger` does. The only callers outside that one production site are three tests (`PartialFailureLogsWarnAndContinues`, `HydrationTriggerEventsSliceIsRespectedAtRuntime`, `ShowHooksFailureWrapsError`) that exercise the migration in isolation. The export adds a second valid entry point for "hook installation" — a future contributor could invoke it standalone, skip the install step, and end up with no hydration hooks registered.
  RECOMMENDATION: Consider unexporting `MigrateHydrationHooks` and refactoring the three direct-target tests to drive `RegisterPortalHooksWithLogger` with a recording logger and a `MockCommander`. Lower priority than the other items.

SUMMARY: Implementation architecture is sound — fix is well-scoped, the deletion of `PredictLiveIndices` and its consumers is complete (verified via repo-wide grep), and the `--` separator + migration eviction compose cleanly with the existing `RegisterHookIfAbsent` primitive via a dedicated table entry plus a one-shot pre-loop migration call. The four findings above are minor API-surface refinements — none are correctness issues; all are local cleanups landable any time.
