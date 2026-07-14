TASK: restore-host-terminal-windows-4-6 — Wire the config tier into the resolver + resolution observability

ACCEPTANCE CRITERIA:
- Identity matching a valid config entry that ALSO matches native Ghostty → Resolve returns the config adapter + ResolutionConfig (config ahead of native).
- No config entry → Ghostty identity resolves native + ResolutionNative; unmatched/unknown → (nil, ResolutionUnsupported).
- Invalid config recipe (structural, or missing/non-exec script) for a native-matching identity → falls through to ResolutionNative (with the 4.2/4.5 WARN); for a non-native identity → falls through to ResolutionUnsupported.
- NULL identity → (nil, ResolutionUnsupported) even with a `*` catch-all config entry (config tier skipped for NULL).
- portal spawn on a config-matched terminal logs the batch summary with resolution=config (no new emission code).
- terminals.json resolves via configFilePath("PORTAL_TERMINALS_FILE","terminals.json") (XDG chain), loaded once, and the tier imports none of internal/state, the daemon, prefs, or restore.

STATUS: Complete

SPEC CONTEXT: Spec *Adapter Contract & Extensibility → Resolution precedence* mandates "config override → native adapter → unsupported" with config able to override a built-in and a NULL/unmatched identity → unsupported. *Config Schema → Precedence* adds within-config most-specific selection; an invalid winning entry "falls through to native → unsupported". *Observability → Attr keys*: `resolution` is a closed attr with values config|native|unsupported. *State/daemon footprint*: reads terminals.json (read-only); does NOT touch sessions.json, the daemon capture loop, prefs.json, or restore.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/resolver.go:44-60 — Resolver struct { Config, runner } + NewResolver wiring the production execRecipeRunner.
  - internal/spawn/resolver.go:80-96 — Resolve: IsNull → unsupported short-circuit; config tier before native registry; native family loop relocated unchanged; unsupported fall-through.
  - internal/spawn/resolver.go:104-123 — resolveConfig: matchConfig → validRecipeForEntry → argv/script adapter switch; every not-ok path returns (nil,false) → native fall-through.
  - internal/spawn/resolver.go:128-130 — ResolveAdapter thin zero-config wrapper preserved.
  - internal/spawn/configadapter.go:116-128 — newScriptRecipeAdapter validity gate (tilde expand + stat + exec-bit), returning the (Adapter,bool) the resolver switch passes straight through.
  - cmd/spawn.go:382-388 — buildResolver: configFilePath("PORTAL_TERMINALS_FILE","terminals.json") → TerminalsStore.Load → NewResolver, degrading to an empty config (native-only) on a configFilePath error (fails safe).
  - cmd/spawn.go:292-302, 340-341 — buildProductionSpawnSeams wires Resolve = buildResolver().Resolve; the shared bundle is built at most once and only when a shared field needs defaulting (loaded-once honoured; fully-injected tests never read terminals.json).
  - cmd/config.go:30-35 — "terminals.json": "" mapped with the read-only / no-macOS-predecessor comment (mirrors prefs.json).
- Notes: Precedence, NULL short-circuit, and invalid→native fall-through all match the spec literally. Import audit of every config-tier file (resolver.go [no imports], configadapter.go [fmt/os/strings/internal/resolver], configmatch.go [strings], recipe.go [errors/fmt/strings], terminalsconfig.go [encoding/json/errors/os]) confirms zero dependency on internal/state, internal/restore, internal/prefs, or the daemon — AC6 satisfied structurally. The two internal/state grep hits in the package are comments explicitly noting the absence of that import.

TESTS:
- Status: Adequate
- Coverage: internal/spawn/resolver_config_test.go covers all six planned cases plus a valuable seventh:
  - config-ahead-of-native (asserts *argvRecipeAdapter type, template contents, and injected-runner wiring).
  - no config → (*ghosttyAdapter, ResolutionNative).
  - invalid config (both argv+script) for Ghostty → (*ghosttyAdapter, ResolutionNative) + exactly 1 WARN.
  - invalid config for a NON-native identity → (nil, ResolutionUnsupported) + exactly 1 WARN (beyond the planned list — exercises the second AC3 branch directly).
  - unmatched unknown identity with a non-matching config entry → (nil, ResolutionUnsupported).
  - NULL identity + `*` catch-all → (nil, ResolutionUnsupported).
  - valid script entry (executable temp file) → (*scriptRecipeAdapter, ResolutionConfig) with scriptPath + runner wiring asserted.
  - cmd/spawn_test.go:552-585 asserts the batch summary carries resolution=config via a logtest.Sink, with every tmux-touching seam injected through spawnPipelineDeps + withBurster (hermetic, no real recipe exec).
- Notes: Runner is a fake in every resolver test (Resolve never runs it — only OpenWindow does), so no real process spawns. Tests assert behaviour (adapter type + resolution + WARN count), not internals. No redundancy, no over-mocking. Two composition paths are covered only transitively rather than at the resolver boundary (see non-blocking notes) — a minor completeness gap, not a correctness gap.

CODE QUALITY:
- Project conventions: Followed. Small DI seam (recipeRunner), package-level component logger, tolerant fail-safe wiring, closed `detail` attr for the entry-key/reason — all consistent with the codebase and the golang skills.
- SOLID principles: Good. Resolve owns precedence orchestration; resolveConfig owns the config-tier pipeline; adapter construction is delegated. Open/closed: adding a native driver is a registry append; adding a recipe kind is a switch arm.
- Complexity: Low. Resolve is a linear 4-step precedence; resolveConfig is match→validate→build with early returns.
- Modern idioms: Yes (slices.Equal in tests, early-return guards, pointer-receiver adapters with compile-time Adapter assertions).
- Readability: Good. Doc comments state the precedence contract and the NULL/invalid fall-through rationale precisely.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/resolver_config_test.go — AC3 covers BOTH a structural invalid recipe AND a missing/non-executable script falling through to native, but the resolver-level tests only exercise the structural (both argv+script) case. Add a resolver case: a config entry with a RecipeScript pointing at a missing (or non-exec) path for a Ghostty identity → (*ghosttyAdapter, ResolutionNative) + 1 WARN — covering the RecipeScript switch arm's (nil,false) pass-through at the resolver boundary (currently only Task 4.5's constructor tests it in isolation).
- [idea] internal/spawn/resolver_config_test.go — Consider a test pinning the spec-literal "invalid most-specific entry falls through to NATIVE, not to a less-specific valid config entry": a config with both a valid broad `*` entry and an invalid `com.mitchellh.ghostty*` entry for a Ghostty identity → ResolutionNative (not the catch-all's config adapter). Behaviour is structurally guaranteed by matchConfig returning a single winner and resolveConfig not re-matching, so this is a low-value guard against a future refactor rather than a current gap.
