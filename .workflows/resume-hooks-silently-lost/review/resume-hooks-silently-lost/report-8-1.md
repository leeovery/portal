TASK: resume-hooks-silently-lost-8-1 — "go test ./cmd Moves The Developer's Real Legacy Config"
Point the `cmd` subtests that resolve a real config path at a temp HOME so the one-shot Application Support
migration they trigger lands in the test's own sandbox, and sweep `cmd` for any other subtest doing the same.

ACCEPTANCE CRITERIA:
- Every `cmd` subtest that resolves a real config path runs against a `t.TempDir()` HOME, never the ambient one.
- The home-fallback assertions compare against the temp home.
- `go test ./cmd` run against a staged HOME holding a legacy Application Support config leaves that directory untouched.
- No production path-resolution behaviour changes — only the environment the tests resolve against.

STATUS: complete

SPEC CONTEXT: The specification (`.workflows/resume-hooks-silently-lost/specification/.../specification.md`) does not
govern this task — it is a phase-8 implementation-analysis task whose authority is its own body, as the shared verifier
context states. The binding standard is instead CLAUDE.md's ABSOLUTE INVARIANT: "a test must NEVER mutate or affect the
real system: not the filesystem outside its temp dirs …". The hazard the task names is real and confined to `cmd`:
`migrateConfigFile` is called from exactly one production site, `configFilePath` (cmd/config.go:70), and the only other
production mention of the legacy location is the explanatory comment at internal/xdg/configfile.go:60 — so no package
outside `cmd` can trigger the move, and the sweep's `cmd`-only scope is the whole risk surface.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/config_test.go:16-30 — "it resolves projects.json under the temp home when XDG_CONFIG_HOME is empty":
    `homeDir := t.TempDir()` + `t.Setenv("HOME", homeDir)` (17-18), assertion `want` built from `homeDir` (26).
  - cmd/config_test.go:47, 63, 93 — the three other `TestConfigFilePath` subtests that reach the non-overridden
    resolution (`respects XDG_CONFIG_HOME when set`, `treats empty XDG_CONFIG_HOME as unset`,
    `XDG_CONFIG_HOME with trailing slash is normalized`) each gained a temp-HOME pin.
  - cmd/config_test.go:386-421 — new subtest "it migrates only the temp home's legacy config directory": stages
    `<tempHome>/Library/Application Support/portal/projects.json`, resolves through `configFilePath`, and asserts both
    the resolved path and that the migration landed inside the temp home.
  - cmd/prefs_path_test.go:46-60 — "it resolves prefs.json under the temp home when XDG_CONFIG_HOME is empty", plus the
    pin added at :28 for the XDG subtest.
  - Delivering commit 661c68c1 touched `cmd/config_test.go` and `cmd/prefs_path_test.go` only — no production file,
    so the fourth criterion (no production path-resolution change) holds by construction. `configFilePath`
    (cmd/config.go:56-72) and `migrateConfigFile` (cmd/config.go:16-47) are untouched.
- Notes:
  - The sweep (Do item 3) holds at HEAD. Every `cmd` test call site of `configFilePath` / `prefsFilePath` /
    `themesDirPath` / `loadHookStore` / `loadProjectStore` / `loadPrefsStore(NoMigrate)` either pins HOME to a temp dir,
    pins the per-file `PORTAL_*` override (which returns before the migration at cmd/config.go:61-62), or deliberately
    blanks HOME so resolution errors (cmd/state_daemon_test.go:810, cmd/state_daemon_project_cleanup_test.go:197).
    The shared helper `applyEnvCase` (cmd/config_seeder_parity_test.go:87-97) pins a temp HOME by default for the
    parity cases. The only remaining ambient-home reads in `cmd` tests are cmd/alias_test.go:40 and :132, which call
    `os.UserHomeDir()` purely to compute an expected tilde expansion while `PORTAL_ALIASES_FILE` points at a temp
    file — no config-path resolution and no migration.
  - Later work in this plan strengthened the same property package-wide: `TestMain` now replaces HOME with a per-run
    temp directory (cmd/testmain_isolation_test.go:56-64), and cmd/testmain_home_poison_test.go pins that contract
    (including, at :113-131, that a subtest's own pin still wins). That is a superset of this task's per-subtest pins
    rather than a replacement for them: each subtest's `want` is anchored to its own `t.TempDir()`, so removing a pin
    makes the assertion fail against the package-wide home rather than silently passing.
  - Third criterion (a staged legacy HOME survives `go test ./cmd`) is not executable here — no shell test runs — but
    it is readable: with `TestMain` overriding HOME before any test body, an externally staged `HOME=…` never reaches
    `configFilePath`'s `os.UserHomeDir()`, and every subtest that resolves a default path supplies its own temp home.

TESTS:
- Status: Adequate
- Coverage: All three test names the task prescribes exist and are the ones delivered —
  cmd/config_test.go:16, cmd/config_test.go:386, cmd/prefs_path_test.go:46. Each observes the pin it is protecting:
  the two resolution tests derive `want` from their own `t.TempDir()` home, so dropping the `t.Setenv("HOME", …)` makes
  `got` resolve under the package-wide temp home and the comparison fail; the migration test stages the legacy file
  under its own temp home, so an unpinned HOME leaves that file in place and the `!os.IsNotExist` assertion at :414
  fails. The migration test is not redundant with its neighbours: "migrates file from old macOS path to new path"
  (:320) drives `migrateConfigFile` directly and "migration runs when XDG_CONFIG_HOME is set" (:423) goes through the
  XDG branch, whereas this one is the only end-to-end cover of `configFilePath` + home fallback + migration.
- Notes: cmd/config_test.go:16 and cmd/config_test.go:61 are now byte-identical in body (same env, same
  `configFilePath` call, same `want`) and differ only in name. This duplication predates the task — both originals
  set `XDG_CONFIG_HOME=""` and asserted the same home-fallback path — and the change only renamed one of them, so it
  is recorded here as an observation, not raised as a finding.

CODE QUALITY:
- Project conventions: Followed. Test-only change; no `t.Parallel()` introduced (cmd forbids it); no new `slog.Handler`
  or logging construction; no production seam assigned directly, so `cmd/seam_guard_test.go` is unaffected.
- SOLID principles: N/A (test environment staging only).
- Complexity: Low — three `t.Setenv` pins and one new subtest, no helper or abstraction introduced.
- Modern idioms: Yes. `t.Setenv` + `t.TempDir()` are the framework's own scoping and cleanup, replacing the previous
  `os.UserHomeDir()` reads.
- Readability: Good. Both files carry a comment stating why the pin exists (cmd/config_test.go:12-15,
  cmd/prefs_path_test.go:44-46), and the renamed subtests say what they assert rather than what the old path was.
- Comment accuracy: The comments hold. cmd/config_test.go:12-15's claim that "TestMain poisons HOME package-wide"
  is true of cmd/testmain_isolation_test.go:59-64, and its stated reason for a per-subtest pin ("a home no other
  subtest writes into … lets it assert on the exact path") matches what the subtests do.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
