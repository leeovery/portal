TASK: 3-6 — Integration test for environment inheritance parity across respawn

STATUS: Complete

SPEC CONTEXT: Component F "Environment inheritance across respawn" — respawned daemon sees same environment as pre-F initial-pane command. `NewDetachedSessionNoCwd` passes no `-e` overrides; this task adds tests pinning that contract.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Integration test: `internal/tmux/portal_saver_endstate_integration_test.go:447-520` `TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn`
  - Helpers: `envValue` (522-541), `parseShowEnvironmentKeys` (556-585), `dumpEnvMap` (592-603)
  - Unit test: `internal/tmux/portal_saver_test.go:3447-3480` `TestNewDetachedSessionNoCwd_ArgvHasNoEnvOverrides`
  - Source guarantee: `internal/tmux/tmux.go:372-382` argv is `["new-session","-d","-s",name,shellCommand]` no `-e`
- Integration reuses placeholder shape (`sh -c 'exec tail -f /dev/null'`) for `_env-baseline` reference session

TESTS:
- Status: Adequate
- Integration: per-key parity across XDG_CONFIG_HOME, HOME, PATH; `envValue` distinguishes unset vs empty-set
- Unit: `HasPrefix("-e")` scan catches both `-e KEY=VAL` and `-eKEY=VAL` forms
- Split between integration (load-bearing) and unit (CI-safe without tmux) is correct
- Diagnostic includes per-key value, full parsed maps, verbatim show-environment output

CODE QUALITY:
- Project conventions: Followed; uses `portaltest.IsolateStateForTest` per CLAUDE.md; uses `tmuxtest.SkipIfNoTmux` + `portalbintest.StagePortalBinary`
- SOLID: Good
- Complexity: Low
- Modern idioms: `strings.IndexByte`, `TrimRight`, `strconv.Quote`
- Readability: Good; three states (unset, empty-set, absent) cleanly distinguished

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `psArgsForPID` helper unused by this test (used by sibling clean-bootstrap test); coherent
- [idea] `parseShowEnvironmentKeys` silently ignores malformed lines; a `t.Logf` for unexpected shapes would aid future debugging
