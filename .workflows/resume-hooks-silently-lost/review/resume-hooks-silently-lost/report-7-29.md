TASK: resume-hooks-silently-lost-7-29 — hookstest Re-Implements cmd/config.go's Hooks-Path Resolution Chain (tick-de3a3a)

ACCEPTANCE CRITERIA:
- The precedence — `PORTAL_HOOKS_FILE`, then `XDG_CONFIG_HOME`, then the home fallback — is declared once and read by both routes.
- `hookstest` no longer walks the env slice by its own rule.
- The seeder path performs no config migration and creates nothing the production read would not.
- The seeder still fatals when the env slice carries neither variable.
- Adding a third env layer to the production precedence changes both routes together, provably by the new test.
- Both lanes pass.

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context). The specification says nothing about config-path resolution beyond
one line noting the hooks lock sidecar follows a `PORTAL_HOOKS_FILE` override
(`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md:337`), which this
change preserves: the sidecar is still derived from whatever path the shared rule resolves. The task's own subject is a
false-green hazard — a seeder that resolves `hooks.json` by a copy of production's precedence can seed a file the
binary under test never reads, and the destructive integration suites would then pass while asserting on an untouched
file.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/xdg/configfile.go:12` (`Lookup` seam), `:17` (`EnvSlice`, last-wins), `:36` (`ConfigFileID`), `:49`
    (`HooksFile`), `:55` (`ConfigFile{Path, Overridden}`), `:75` (`ConfigFilePath` — the single declaration of the
    per-file precedence).
  - `cmd/config.go:56` (`configFilePath` — the production route: `xdg.ConfigFilePath(xdg.OSEnv, id)`, then the
    Application Support migration gated on `!resolved.Overridden` at `:61`–`:71`).
  - `internal/hookstest/hooks.go:53` (`ResolveHooksFilePathFromEnv` — delegates via `xdg.EnvSlice(env)` +
    `xdg.ConfigFilePath(lookup, xdg.HooksFile)`; the neither-variable fatal at `:56` is a presence probe, not a
    precedence rule).
- Notes:
  - Criterion 1 holds: `xdg.ConfigFilePath` is the only declaration of the ladder, and `cmd/config.go:57` and
    `internal/hookstest/hooks.go:59` are its only readers (`migrateConfigFile` is called from exactly one site,
    `cmd/config.go:71`, so the doc claim at `cmd/config.go:51`–`:55` that the migration "lives here and only here"
    is true).
  - Criterion 2 holds: the old prefix walk (`strings.CutPrefix` over the slice) is gone; the helper's only
    environment reads are through the injected `Lookup`.
  - Criterion 3 holds: `ConfigFilePath` stats, creates and migrates nothing (only `ConfigBaseFrom`'s
    `os.UserHomeDir` reads outside the lookup), and the seeder never reaches the migration.
  - Criterion 4 holds: the fatal survives (`internal/hookstest/hooks.go:56`) and its message still names the
    isolation regression.
  - Two latent divergences were closed rather than merely relocated: the old helper returned the *first*
    `PORTAL_HOOKS_FILE` but the *last* `XDG_CONFIG_HOME` (`EnvSlice` is now uniformly last-wins, matching
    `exec.Cmd`'s dedupe), and it returned an empty path for a slice carrying `PORTAL_HOOKS_FILE=` (now treated as
    unset, as production treats it).
  - Consumer check (task Do item 4) verified: every caller of `SeedHooksJSON`/`HooksJSONBytes`/
    `ResolveHooksFilePathFromEnv` pins `PORTAL_HOOKS_FILE` to a writable temp path *before* deriving the env slice
    (`cmd/state_daemon_hook_cleanup_integration_test.go:78`, `cmd/doctor_fix_transient_listpanes_shared_integration_test.go:26`,
    `cmd/bootstrap/transient_listpanes_helpers_integration_test.go:103`), so each slice carries exactly one entry for
    the variable and the first→last change cannot move any of them.
  - Production behaviour is unchanged for `configFilePath`: same order (override wins before the home read), same
    error shapes.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/xdg/configfile_test.go:11` (`EnvSlice`: read, absent, last-wins, whole-name-not-prefix) and `:45`
    (`ConfigFilePath`: override wins + `Overridden` true, config base + `Overridden` false, empty-as-unset, home
    fallback, the `OSEnv` route, and "resolves a path and creates nothing").
  - `internal/hookstest/resolve_test.go:13` — the four delegation cases the task names, including
    "it triggers no config migration on the seeder path" (`:42`), which stages a real legacy
    `~/Library/Application Support/portal/hooks.json`, asserts it is untouched after both a resolve and a seed, and
    asserts the resolve created no directory.
  - `internal/hookstest/resolve_test.go:81` — the fatal case, driven through a re-exec of the test binary, which is
    the only way to observe a `t.Fatalf` without failing the observing test. The child fatals before any resolution,
    so it touches nothing.
  - `cmd/config_seeder_parity_test.go:20` — drives both routes over one env (the helper at `:87` puts each case on
    the process environment *and* into the slice, so a case cannot describe two environments), across all three
    layer combinations; `:60` proves the migration asymmetry with a staged legacy file rather than asserting it.
  - `cmd/config_precedence_single_source_test.go:19` — the AST guard backing criterion 5: both route bodies must
    call `ConfigFilePath` and must not call `Getenv`/`LookupEnv`/`Environ`/`CutPrefix`/`ConfigBase`/`ConfigBaseFrom`,
    with a fatal if the named function has disappeared from the file. Verified against
    `sourceguardtest.ForEachFuncCall`/`CalleeName` semantics (`internal/sourceguardtest/foreachfunccall.go:11`,
    `calleename.go:13`): calls in nested closures are attributed to the enclosing declaration, and `lookup(name)` —
    the rule's own seam — correctly reports as `lookup` and is deliberately excluded.
  - Would these fail if the feature broke? Yes: reverting the delegation (a private walk restating the ladder)
    fails the guard on `CutPrefix`; adding a layer to `configFilePath` alone fails the guard on `Getenv`; letting
    the seeder migrate fails both migration subtests.
  - Every test name the task listed is present verbatim.
- Notes: The overlap between `internal/hookstest/resolve_test.go:23` and `internal/xdg/configfile_test.go:62` is not
  redundancy — one pins the shared rule, the other pins that the seeder reaches it. Both new test files are
  untagged, so they run in the unit lane and compile in the integration lane. Tests were assessed by reading; none
  were executed.

CODE QUALITY:
- Project conventions: Followed. `internal/xdg` stays a stdlib-only leaf (pinned across both lanes by
  `internal/xdg/leaf_guard_test.go:20`), so `internal/hookstest` taking the edge introduces no cycle and no
  production/test coupling. No `t.Parallel()`; `t.Setenv` used throughout. The migration breadcrumb keeps its
  existing `hooks` log component via `ConfigFileID.LogComponent`, inventing no vocabulary. The parity test's
  temp-`HOME` pinning keeps the legacy-path migration off the developer's real files, and the re-exec'd child in the
  fatal test resolves nothing and writes nothing.
- SOLID principles: Good. `Lookup` is a one-method seam that inverts the environment dependency; the migration
  (a side effect) is separated from resolution (a pure function), which is exactly what makes one rule safe to share
  with a test seeder.
- Complexity: Low. `ConfigFilePath` is two branches; `EnvSlice` is a single last-wins scan.
- Modern idioms: Yes — `strings.CutPrefix`, a func-typed seam rather than an interface for a single method.
- Readability: Good. The comments state why rather than what, and each one checked out against the code:
  `internal/hookstest/hooks.go:45`–`:49` ("the rule's home fallback is deliberately out of reach here") is true
  because the fatal fires whenever both variables are empty, which is the only condition under which
  `ConfigBaseFrom` would reach `os.UserHomeDir`.
- Issues: None reaching the reporting bar. (`internal/hookstest/hooks.go:37` restates the literal
  `"XDG_CONFIG_HOME"`, which `internal/xdg/xdg.go:25` also spells — but it serves the isolation tripwire rather than
  the precedence, `xdg` exports no constant for it, and the name is an external standard, so no failure can be named
  from the duplication.)

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
