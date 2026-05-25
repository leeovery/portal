TASK: 7-3 — Promote applyHostNoiseMitigation into internal/portaltest

STATUS: Complete

SPEC CONTEXT: c1 Finding 3 — 2-line `t.Setenv("HOME", t.TempDir()); t.Setenv("XDG_CONFIG_HOME", "")` helper + ~12-line rationale inlined in three packages (cross-package `_test` import impossible). T7-3 collapses triplication by folding into `IsolateStateForTest` so ordering invariant (env scrub → snapshot → env construction) becomes non-bypassable.

IMPLEMENTATION:
- Status: Implemented (delivered under T9-5 per c5 cross-reference; rename to `IsolateStateForTest` + fold combined)
- Location:
  - `internal/portaltest/isolated_env.go:56-66` — scrub at function head
  - `internal/portaltest/isolated_env.go:13-55` — consolidated rationale block (side-effect warning, host-noise rationale, ordering invariant)
  - Migrated callers: `cmd/bootstrap/orphan_sweep_integration_test.go`, `cmd/state_daemon_self_supervision_integration_test.go`, `internal/tmux/portal_saver_endstate_integration_test.go` — local `applyHostNoiseMitigation` definitions gone
- c5 duplication audit explicitly closes c1 F3
- Repo-wide grep for `applyHostNoiseMitigation` returns zero production/test matches

TESTS:
- Status: Adequate
- `internal/portaltest/isolated_env_test.go` exercises env scrub, decoy XDG filtering, empty-string XDG handling, backstop install/fire
- Three migrated integration tests cover end-to-end via real subprocess spawns

CODE QUALITY:
- Project conventions: Followed
- SOLID: Single responsibility; narrow `backstopT` interface enables unit-level cleanup-hook testing
- Complexity: Low; linear; only branching is optional snapshot/backstop skip when `devStateDir` unresolvable
- Modern idioms: `t.Setenv`, `t.TempDir`, `t.Cleanup`; `filterXDGConfigHome` handles `XDG_CONFIG_HOME=""` via full `KEY=` prefix
- Readability: Excellent; godoc is single source of truth

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] T7-3 AC phrasing references pre-T9-5 name `NewIsolatedStateEnv`; plan row not retro-edited
