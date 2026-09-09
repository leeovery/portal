TASK: resume-hooks-silently-lost-9-31 — "Three ConfigFileID Filenames Are Asserted By Nothing" (tick-06c0c0, phase 9, severity medium, source: bank)

ACCEPTANCE CRITERIA:
- [x] All five `ConfigFileID` filenames are asserted, including the three that are asserted by nothing today.
- [x] All five env vars and all five log components are asserted in the same rows.
- [x] Both `ConfigDirID`s are covered for env var and directory name.
- [x] A typo in any filename, env var, directory name or component fails the test.
- [x] Adding a new identity without a table row fails the test.

STATUS: complete

SPEC CONTEXT:
The specification (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md`)
says nothing about config-file identities — it is scoped to the durable pane-token hook key. This is a phase-9
implementation-analysis task, so per the shared verifier context its authority is its own body, which I judged it
against. The binding convention is CLAUDE.md's `xdg` row: `ConfigFileID{EnvVar, Filename, LogComponent}` is the
single home of each config file's identity, `ConfigDirID{EnvVar, Dirname}` the directory-shaped sibling, and
`PrefsFile`/`TerminalsFile` deliberately carry the empty log component so their migration runs silently.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/config_identity_test.go` (whole file; the change is commit 69f5a0c3, +186/-9, this file only)
  - `cmd/config_identity_test.go:18-33` — `fileIdentity` / `dirIdentity` row types
  - `cmd/config_identity_test.go:39-47` — `wantFileIdentities()`, five rows with literal env var / filename / component
  - `cmd/config_identity_test.go:50-55` — `wantDirIdentities()`, `StateDir` + `ThemesDir`
  - `cmd/config_identity_test.go:96-110` — per-identity env var / filename / component assertions
  - `cmd/config_identity_test.go:112-125` — the empty-component set-equality pin
  - `cmd/config_identity_test.go:131-144` — `TestConfigDirIdentity`
  - `cmd/config_identity_test.go:156-241` — `TestConfigIdentityCompleteness` + `declaredIdentities` /
    `compositeLitTypeName` / `assertSameNames`
- Notes:
  - Every row's expectation is a **literal**, compared against the live `xdg` value (`want.id.EnvVar` vs
    `want.envVar`, etc.), so the pin is not circular. I re-read `internal/xdg/configfile.go:47-51` and
    `internal/xdg/configdir.go:23-24`: all seven rows match the declarations exactly, and the three previously
    unasserted filenames (`projects.json`, `aliases`, `terminals.json`) are now covered.
  - The pin reaches production behaviour rather than just a struct: every consumer routes through the identity —
    `cmd/config.go:85` (`ProjectsFile`), `cmd/alias.go:97` (`AliasesFile`), `cmd/hooks.go:256` (`HooksFile`),
    `cmd/config.go:187` (`PrefsFile`), `cmd/spawn_seams.go:53` (`TerminalsFile`), `cmd/config.go:201`
    (`ThemesDir`), `internal/state/paths.go:27` (`StateDir`). A typo in a filename therefore fails here before it
    can point a store at a file nothing writes.
  - The removed subtest ("the identity declares the env var and filename together") is fully subsumed: its
    `HooksFile` env var / filename / component assertions land in the table row at line 43, and its
    `PrefsFile.LogComponent == ""` assertion lands in both line 44 and the set-equality pin at 112. Nothing was lost.
  - The completeness guard scans `internal/xdg`'s non-test sources for package-level vars initialised with a
    composite literal, grouped by type name. I confirmed all seven identity literals live in that directory and
    nowhere else (`grep 'ConfigFileID{\|ConfigDirID{'` finds only `internal/xdg/configfile.go`,
    `internal/xdg/configdir.go`, plus synthetic fixture literals inside `cmd`/`xdg` *test* files, which are not
    identities).

TESTS:
- Status: Adequate
- Coverage: All four test names from the plan's Tests list exist verbatim —
  `"it pins every config file identity's env var, filename and log component"` (:96),
  `"it pins the deliberately empty log components rather than skipping them"` (:112),
  `"it pins both config directory identities' env var and directory name"` (:132),
  `"it fails when an identity is declared with no table row"` (:157).
- Notes:
  - **Would fail if the behaviour broke.** A typo in any of the seven env vars, five filenames, five components or
    two dirnames fails the corresponding row. I traced the completeness guard's failing directions: a new
    `var X = ConfigFileID{…}` with no row trips the "declared and pinned by no table row" arm
    (`cmd/config_identity_test.go:233`); a row naming a removed identity trips the symmetric arm (:239). The guard
    also cannot pass vacuously — if the scan silently stopped recognising the declarations, the five/two rows would
    all fall out as orphans and error, and `sourceguardtest.ParsePackageSources` fatals on an empty enumeration
    (`internal/sourceguardtest/parsesources.go:85`, `packagegofiles.go:31`).
  - **Not over-tested.** The empty-component subtest (:112) looks like a restatement of the table's two `""` rows
    but is not: it computes the empty set from the live `xdg` values and asserts set-equality with
    `{PrefsFile, TerminalsFile}`, so a *sixth* identity added with an empty component — table row and all — is a
    visible failure rather than a silent pass. That is the distinct property the task's third Do-item asks for.
  - **Cache correctness holds without the phase-10 treatment.** `PackageDeps`'s cache-input read (added by
    Tresume-hooks-silently-lost-10-1) exists because `go list` runs as a subprocess whose reads the test cache
    cannot see. This guard has no such gap: it reads `internal/xdg`'s directory and files in-process through
    `os.ReadDir` / `parser.ParseFile`, so both are recorded as inputs of the `cmd` test binary; and `cmd` imports
    `internal/xdg` anyway, so the build id moves with any edit there.
  - **Lane placement is correct.** Untagged, unit lane: it compiles no binary, spawns no daemon and touches no
    tmux, matching CLAUDE.md's lane rule. It reads no env, so `cmd`'s package-wide `PORTAL_*`/`TMUX`/`HOME` poison
    (`cmd/testmain_isolation_test.go:64-72`) neither affects it nor is affected by it, and it mutates nothing
    outside the repo — the ABSOLUTE INVARIANT holds.

CODE QUALITY:
- Project conventions: Followed. Source scanning routes through `sourceguardtest` (`ParsePackageSources`,
  `ProjectRoot`) rather than re-authoring the walk; `ProjectRoot` is taken directly, which is the sanctioned use for
  "a guard that only needs to name a path within the tree ... joining a file it reads itself"
  (`internal/sourceguardtest/reposources.go:49-52`). Subtest naming follows the repo's `"it …"` form. No new
  package-scope collisions: `fileIdentity`, `dirIdentity`, `wantFileIdentities`, `wantDirIdentities`, `fileIDType`,
  `dirIDType`, `declaredIdentities`, `compositeLitTypeName`, `assertSameNames` are each declared once across the
  whole tree.
- SOLID principles: Good. The table types carry data only; `declaredIdentities` enumerates, `compositeLitTypeName`
  classifies one declaration, `assertSameNames` reports the two-way set difference — one job each.
- Complexity: Low. The deepest nesting is the AST walk's decl/spec/name loop, each level a single type assertion
  with an early `continue`.
- Modern idioms: Yes. `slices.Sort` / `slices.Equal` / `slices.Sorted(maps.Keys(…))` are used where each fits
  (Go 1.26 module, `go.mod:3`); the helpers return fresh slices per call rather than sharing package-level mutable
  test state.
- Readability: Good. Both failure messages say what to do about the failure rather than only what differed
  ("add one, or a typo in it ships silently"), and the sorted orphan report gives a deterministic message order.
- Issues: None. Every comment in the changed code holds against it, and none references a task id, phase or spec
  section.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
