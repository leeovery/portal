TASK: 1-2 — Implement portaltest.NewIsolatedStateEnv helper (subsequently renamed `IsolateStateForTest` via T9-5)

ACCEPTANCE CRITERIA:
- Helper compiles, callable from `*_test.go`
- Env contains exactly one `XDG_CONFIG_HOME=` entry inside `t.TempDir()`
- Env does NOT contain developer's pre-test `XDG_CONFIG_HOME`
- `stateDir` exists at `<tempDir>/config/portal/state`
- Env directly usable as `exec.Cmd.Env`
- `*testing.T` parameter structurally enforces test-only

STATUS: Complete

SPEC CONTEXT: Spec § Component G item 1. Per-test state-directory isolation. `*testing.T` param structurally prevents production import.

IMPLEMENTATION:
- Status: Implemented (with approved post-task evolution)
- Location: `internal/portaltest/isolated_env.go:56-109`; `internal/portaltest/doc.go`; `internal/portaltest/fingerprint.go:362-370` (`resolveDevStateDir`)
- Notes:
  - RENAMED `NewIsolatedStateEnv` → `IsolateStateForTest` via Phase 9 task 9-5 (approved). Verb-shaped name signals env mutation
  - Helper went beyond original 1-2 scope by FOLDING IN host-noise mitigation (`t.Setenv("HOME", t.TempDir())`, `t.Setenv("XDG_CONFIG_HOME", "")`) and snapshot + backstop wiring (originally Task 1-3 scope). Consolidation driven by c1/c2 architecture analyses
  - Env-construction core matches plan: filter inherited `XDG_CONFIG_HOME` via `filterXDGConfigHome` (lines 136-146), append fresh `XDG_CONFIG_HOME=<configDir>`, `MkdirAll` configDir and stateDir 0o700
  - "SIDE EFFECT" docstring (lines 19-35) warns at API surface

TESTS:
- Status: Adequate
- Location: `internal/portaltest/isolated_env_test.go`
- Coverage: `TestSetsXDGConfigHomeInsideTempDir`, `TestRemovesPreExistingXDGConfigHome`, `TestRemovesEmptyPreExistingXDGConfigHome`, `TestPreservesPath`, `TestNeutralizesHomeAndXDGConfigHome`, `TestStateDirUnderXDGConfigHome`, `TestEnvUsableAsExecCmdEnv`, `TestDistinctStateDirPerCall`, `TestConfigDirPermissions`
- `TestMain` (lines 31-42) preemptively redirects HOME/XDG so package self-tests don't trip backstop on machines running live `portal state daemon`
- Not over-tested

CODE QUALITY:
- Project conventions: Followed; leaf package, `*testing.T` enforcement, narrow `backstopT` interface
- SOLID: Good; small focused helpers
- Complexity: Low; linear flow
- Modern idioms: `errors.Is(err, fs.ErrNotExist)`, `filepath.WalkDir`, `t.Setenv`/`t.TempDir`
- Readability: Good; dense but justified docstring

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan's acceptance criterion "Returned env preserves all other entries from os.Environ() (e.g., HOME, PATH)" was superseded by host-noise mitigation. `TestNeutralizesHomeAndXDGConfigHome` pins new contract; plan document never amended (doc-hygiene only)
- [idea] Audit deliverable file still references pre-rename name `NewIsolatedStateEnv` in prose
- [quickfix] `isolated_env.go:1-3` top-of-file comment mostly duplicates package godoc in `doc.go`
