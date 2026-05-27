AGENT: architecture
FINDINGS:

- FINDING: Asymmetric shape for parallel bootstrap-step cleanup cores
  SEVERITY: low
  FILES: cmd/run_hook_stale_cleanup.go:90-147, cmd/bootstrap/stale_marker_cleanup.go:104-161
  DESCRIPTION: Step 9 (marker cleanup) and step 11 (hook cleanup) implement the same six-branch algorithm in two structurally divergent shapes. Step 9 is a typed `MarkerCleanupCore` struct in `cmd/bootstrap/` with seam fields + method. Step 11 is a free function `runHookStaleCleanup` in `package cmd` with a `listErrorPolicy` enum parameter. Spec cross-references the two as prior-art/new-art pair, yet code shapes do not mirror each other. Structural drift between the parallel pair cannot be enforced by a shared type.
  RECOMMENDATION: Consider lifting `runHookStaleCleanup` into `cmd/bootstrap/` as a `HookCleanupCore` struct mirroring `MarkerCleanupCore`. The `listErrorPolicy` becomes a `SwallowListError bool` field; `onRemoved` becomes optional `OnRemoved func(string)`. `cleanStaleAdapter` collapses to inline construction matching the existing EagerSignalCore / MarkerCleanupCore wiring documented in `cmd/bootstrap_production.go:32-37,164-189`. Not blocking — defer until pressure increases.

- FINDING: `listErrorPolicy` enum is a boolean in disguise
  SEVERITY: low
  FILES: cmd/run_hook_stale_cleanup.go:61-76, cmd/run_hook_stale_cleanup.go:101-108
  DESCRIPTION: The two-value enum branches in exactly one location. The 32-line docblock describes a policy axis, but the implementation is a binary toggle with no plausible third value.
  RECOMMENDATION: Replace `policy listErrorPolicy` with `swallowListError bool`, OR always return the err and let each caller decide (parameter disappears entirely). Minor.

- FINDING: `cleanStaleAdapter.Logger` field visibility inconsistent with siblings
  SEVERITY: low
  FILES: cmd/bootstrap_production.go:72-76
  DESCRIPTION: `cleanStaleAdapter` is unexported. Its `lister` and `store` fields are lowercase but `Logger` is exported. No consumer outside `package cmd` exists. Looks like a porting artefact.
  RECOMMENDATION: Rename `Logger` → `logger` for visibility consistency. Pure cosmetic.

- FINDING: `cleanStaleAdapter.CleanStale` has no direct unit coverage
  SEVERITY: low
  FILES: cmd/bootstrap_production.go:85-87, cmd/bootstrap_production_test.go:1-14
  DESCRIPTION: After cycle-1 consolidation, the adapter method's specific composition — passing `returnError`, nil `onRemoved`, and the adapter's own `Logger` field — is exercised only by `//go:build integration` end-to-end tests. A regression that flipped policy to `swallow` or accidentally passed a non-nil `onRemoved` printing to stdout would slip past `go test ./...`.
  RECOMMENDATION: Add a one-shot non-integration unit test that constructs a `cleanStaleAdapter` with stub lister + temp hooks store + recording logger, invokes `CleanStale()`, asserts (i) recording logger received the entry-point Debug (proves Logger flowed through), (ii) on stub returning a non-nil list-panes error, the method returns err (proves `returnError` policy was passed). ~30 lines.

- FINDING: `transienttest.ResolveHooksFilePathFromEnv` re-implements `cmd/config.go`'s resolution chain
  SEVERITY: low
  FILES: internal/transienttest/hooks.go:23-42, cmd/config.go
  DESCRIPTION: The test-side helper walks the env slice for `PORTAL_HOOKS_FILE` then `XDG_CONFIG_HOME`, mirroring production. Independent implementations. If the production chain ever grows a third tier (e.g. macOS migration path), the test mirror silently resolves the wrong file and produces false positives.
  RECOMMENDATION: Either (a) export an env-slice-consuming variant from `cmd/config.go` consumed by both production and `transienttest`, or (b) add a unit test in `internal/transienttest/` asserting agreement with `configFilePath`'s output across the env-tier matrix. Prefer (b) — pins the contract without a production refactor.

SUMMARY: Five low-severity architectural residuals. Headline: structural asymmetry between the two parallel cleanup cores — a `HookCleanupCore` mirroring `MarkerCleanupCore` would collapse the divergence. Four remaining findings are cosmetic / coverage / drift-risk items.
