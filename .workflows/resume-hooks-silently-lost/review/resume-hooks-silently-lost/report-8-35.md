TASK: resume-hooks-silently-lost-8-35 (tick-7eeef8) — "Each Config File's Identity Is Restated At Every Call Site, And Two Production Sites Restate The Resolution Rule"

ACCEPTANCE CRITERIA:
- Renaming a config file's env var is one edit, reaching `cmd` and the two test-only packages.
- `internal/hookstest` resolves `hooks.json` from the shared identity rather than its own pair.
- The `portal/` path segment appears in one production place.
- Themes-dir resolution still runs no migration and emits no breadcrumb; state-dir resolution is behaviourally unchanged.

STATUS: complete

SPEC CONTEXT: This is a phase-8 implementation-analysis task, so its authority is its own body rather than the specification. The spec's only touchpoint is line 337 of `.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md` — the hooks lock sidecar is derived from the *resolved* `hooks.json` path so it follows a `PORTAL_HOOKS_FILE` override. That property is preserved: `hooksFilePath()` (cmd/hooks.go:256) still routes through `configFilePath`, whose env-var-wins arm is unchanged, and the sidecar derivation in `internal/hooks` reads the store's resolved path and was not touched by this commit (363cc46d).

IMPLEMENTATION:
- Status: Implemented (with one sound, documented divergence from the Do list's suggested location)
- Location:
  - `internal/xdg/configfile.go:36-52` — the new `ConfigFileID{EnvVar, Filename, LogComponent}` type and the five-file table (`ProjectsFile`, `AliasesFile`, `HooksFile`, `PrefsFile`, `TerminalsFile`); `ConfigFilePath` now takes the id (`configfile.go:75`).
  - `internal/xdg/configdir.go:7,15-24,36-46` — `portalDirName`, `ConfigDirID{EnvVar, Dirname}` (`StateDir`, `ThemesDir`) and `ConfigDirPath`.
  - Call sites routed: `cmd/config.go:56,85,187`, `cmd/alias.go:97`, `cmd/hooks.go:256`, `cmd/spawn_seams.go:53`, `internal/hookstest/hooks.go:53-64`, `internal/hookstest/staging.go:104`, `internal/restoretest/restoretest.go:151-152`.
  - Directory resolvers folded onto the shared rule: `internal/state/paths.go:26-28` and `cmd/config.go:200-202`.
  - `configFileComponents` is deleted (no occurrence remains anywhere in the tree).
- Notes:
  - The Do list proposed the table live "beside `configFileComponents` in `cmd/config.go`"; the implementation put it in `internal/xdg` instead. That is a strict improvement and is what the Do list's own step 1 preamble asks for ("Decide where a config file's identity lives"): the identity has to be reachable from `internal/hookstest` and `internal/restoretest`, neither of which can import `cmd`. `internal/xdg` remains stdlib-only, and its leaf guard (`internal/xdg/leaf_guard_test.go:20-24`, empty allowlist across both lanes with `ForbiddingThirdParty`) still holds.
  - Behavioural parity verified by reading the diff: `state.Dir()` was `os.Getenv("PORTAL_STATE_DIR")` → `xdg.ConfigBase()` → `Join(base,"portal","state")`; `ConfigDirPath` is the identical sequence with `ConfigBaseFrom(OSEnv)` (equal by construction to `ConfigBase()`). Same for `themesDirPath()`. Empty-env-treated-as-unset, verbatim override, and the home fallback all behave as before.
  - The `portal/` config-base segment now has exactly one production home (`internal/xdg/configdir.go:7`, consumed by both `ConfigFilePath` and `ConfigDirPath`). The remaining production occurrence, `cmd/config.go:70`, is the *old macOS* `~/Library/Application Support/portal/` path — a different path root that was never one of the three sites the task counted, and folding it into `portalDirName` would conflate a legacy location with the live one.
  - Pre-existing restatements in `internal/portaltest/fingerprint.go:286-293` and `internal/portaltest/isolated_env.go:72` remain, but `portaltest` was named by neither the Do list nor the acceptance criteria, its inlining carries a stated reason in-source, and neither file is in this task's change-set.

TESTS:
- Status: Adequate
- Coverage:
  - All three named tests exist and assert what they say: `cmd/config_identity_test.go:62` ("it resolves hooks.json from the shared file identity") checks the shared rule, the production route (`hooksFilePath()`) and the seeder (`hookstest.ResolveHooksFilePathFromEnv`) all land on the same path; `cmd/config_themes_test.go:128` ("it resolves the themes dir with no migration") seeds the old macOS themes dir, asserts it is untouched, asserts zero log records over a `logtest` sink, and pins the result against `xdg.ConfigDirPath`; `internal/state/paths_test.go:69` ("it resolves the state dir through the shared config base") covers all three precedence arms plus a creates-nothing case.
  - Behavioural parity is anchored by literal-path pins that survive the refactor rather than by tautologies: `cmd/config_themes_test.go:12-83` pins `/tmp/x`, `<xdg>/portal/themes`, `<home>/.config/portal/themes` and the home-resolution-failure arm; `internal/state/paths_test.go:13-61` does the same for the state dir. The new shared-rule tests sit on top of those, so a re-authored resolver with the wrong env var or segment still fails.
  - Identity pinning: `cmd/config_identity_test.go:96-125` pins every id's env var, filename and log component, and separately pins that exactly `PrefsFile`/`TerminalsFile` carry the empty component — so losing or gaining a suppressed migration breadcrumb is a visible edit rather than a silent one.
  - Drift guard: `cmd/config_identity_test.go:156-241` derives the declared identity set from `internal/xdg`'s own AST and fails both directions (declared-but-unpinned, pinned-but-undeclared). With zero declarations parsed it reports every row as undeclared rather than passing vacuously, and `sourceguardtest.ParsePackageSources` is fatal on an empty parse.
  - Rewired existing tests correctly compensate for cmd's `TestMain` env poison: `cmd/config_migrate_logging_test.go:238` and `cmd/config_identity_test.go:67` add `t.Setenv("PORTAL_HOOKS_FILE", "")`, which the previous fake-env-var-name form did not need.
- Notes: no under- or over-testing found. The AST completeness guard is on the heavier side for a seven-row table, but it is this repo's established idiom (~20 source guards) and it enforces a real drift risk rather than restating the table.

CODE QUALITY:
- Project conventions: Followed. `internal/xdg` stays stdlib-only and guarded; no new log component or attr key; the themes-dir carve-out (`ConfigDirID`, no component, no migration) is preserved and documented; the CLAUDE.md `xdg` row and the "Config path resolution" section were updated in the same commit and match the code (`ConfigFilePath(lookup, id)`, the `ConfigFileID` table, `ConfigDirPath`/`ConfigDirID`, and the corrected consumer list).
- SOLID principles: Good. `ConfigFilePath`/`ConfigDirPath` resolve and nothing more; the migration side effect stays in `cmd/config.go:56-74`, which is what keeps a test seeder resolving the same rule without ever moving a real file.
- Complexity: Low. Every routed call site became a single-argument call; two resolvers collapsed from eight lines to one.
- Modern idioms: Yes. Typed identity structs replace positional `(envVar, filename)` string pairs, removing the two-string-argument transposition hazard at every call site.
- Readability: Good. Both new doc comments state why the directory shape differs from the file shape (no old location ⇒ no `Overridden` to gate a migration on), which is the non-obvious part.
- Issues: none rising to a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
