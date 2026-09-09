TASK: resume-hooks-silently-lost-9-3 — "cmd's TestMain Does Not Poison HOME, So A Test Resolving A Default Config Path Moves The Developer's Real Config Files" (tick-0fd515)

ACCEPTANCE CRITERIA:
- [x] `HOME` is set package-wide in `cmd`'s `TestMain` to a per-run temp directory that exists at the time every test runs.
- [x] A `cmd` test that resolves a non-overridden config path without pinning its own `HOME` reads the poisoned home; nothing is created or renamed under the developer's real `~/Library/Application Support/portal/` or `~/.config/portal/`.
- [x] The temp home is removed after `m.Run()` returns, with the run's exit code preserved.
- [x] Every subtest that already pins its own temp `HOME` passes unchanged.
- [x] `go test ./cmd` leaves no `HOME`-rooted artifacts outside the per-run temp directory.

STATUS: issues_found (0 blocking; 2 non-blocking findings)

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its authority is its own body rather than the specification (per the shared verifier context). The spec touches the subject once in passing — line 483 notes that "`cmd`'s `TestMain` poisons `TMUX` package-wide" as the reason a real-tmux `cmd` test is avoided — and says nothing about `HOME`. The governing project standard is CLAUDE.md's ABSOLUTE INVARIANT ("a test must NEVER mutate ... the filesystem outside its temp dirs") and its statement that the `PORTAL_*`/`TMUX` poison is *structural* enforcement rather than discipline. The hazard the task names is real and reachable: `cmd/config.go:56-72`'s `configFilePath` runs `migrateConfigFile` — an `os.Rename` of `<home>/Library/Application Support/portal/<file>` into the resolved path — as a side effect of any non-overridden resolve, and its home comes from `os.UserHomeDir()`, i.e. `$HOME` on darwin.

IMPLEMENTATION:
- Status: Implemented (with a defensible, well-reasoned addition beyond the plan's Do list)
- Location:
  - `cmd/testmain_isolation_test.go:49-82` — `TestMain` now creates a per-run `os.MkdirTemp("", "portal-test-home")` and `os.Setenv("HOME", …)` (lines 56-64) ahead of the existing `PORTAL_*`/`TMUX` poisons, captures `m.Run()`'s code into `code`, removes the directory, then `os.Exit(code)` (lines 77-81).
  - `cmd/testmain_isolation_test.go:3-6` — file header re-voiced to name `HOME` among the poisoned boundaries.
  - `cmd/testmain_isolation_test.go:19-47` — `pinToolchainCaches`, an addition not in the Do list: it re-exports `GOMODCACHE`/`GOCACHE` from `go env` *before* the poison, so a subprocess `go build` (the integration lane's `portalbintest` route) does not re-derive its caches under the temp home and leave read-only module-cache directories the run's own `RemoveAll` cannot delete. This is a necessary consequence of the poison rather than scope creep, it is reasoned in-source, and it is covered by its own subtest.
  - `cmd/config_test.go:12-15` — per-subtest comment re-voiced from "pin a temp HOME or you will move the developer's files" to what the pin now buys the subtest.
- Notes:
  - Verified the mechanism end-to-end: `xdg.ConfigFilePath` (`internal/xdg/configfile.go:70-81`) falls back to `ConfigBaseFrom` → `os.UserHomeDir()` (`internal/xdg/xdg.go:27-33`) when the per-file env var and `XDG_CONFIG_HOME` are both empty, and `configFilePath` composes the Application Support source path from the same `os.UserHomeDir()`. Poisoning `HOME` therefore closes both the migration source and the default destination.
  - The "real directory, not `/nonexistent`" choice is correct and the in-source justification holds: `os.UserHomeDir` only reads the variable, so an absent home resolves fine and fails later at the write.
  - The lint exclusion in `.golangci.yml:38-41` is keyed on `path: testmain_isolation_test\.go` + `source: os\.Setenv`, so both new unchecked `os.Setenv` calls (line 64 and line 42) stay excluded — no new errcheck finding. (Its *comment* no longer matches; see FINDINGS.)
  - Side benefit worth recording: `terminals.json` is the one `xdg.ConfigFileID` with no `PORTAL_*` poison in `TestMain` (lines 66-72 poison state, hooks, projects, aliases, themes, prefs — not `PORTAL_TERMINALS_FILE`; isolation there is the per-test `isolateTerminalsFile` helper at `cmd/spawn_seams_test.go:25-28`, used at three call sites). Before this change, a `cmd` test reaching `buildResolver` (`cmd/spawn_seams.go:51-57`) without that helper resolved — and ran the migration against — the developer's real home. The `HOME` poison closes that too.
  - `pinToolchainCaches` hard-fails the whole package (`os.Exit(1)`) if `go env` cannot run. Acceptable for a local-only toolchain-driven suite, and the failure message names exactly what went wrong; noted, not a finding.

TESTS:
- Status: Adequate
- Coverage: `cmd/testmain_home_poison_test.go` adds `TestPackageWideHomePoison` with four subtests: the poisoned-home default resolve (`:40-53`), the Application Support migration running against the poisoned home (`:55-89`, staging an old file and asserting both the migrated body and the source's disappearance), the toolchain-cache pin (`:94-111`), and the subtest-pin-wins case (`:113-131`). Three of these are the plan's named micro-acceptance tests verbatim; the fourth covers the `pinToolchainCaches` addition.
- Notes:
  - The tests fail if the feature breaks. `ambientHomeAtInit` (`:12-15`) is a package-level var, so it is initialised before `TestMain` runs and holds the real home; `requirePoisonedHome` (`:17-37`) fatals when `HOME` is unset or still equals it, so removing the poison from `TestMain` fails all four subtests rather than silently passing.
  - The migration subtest is the strongest one: it asserts the moved body and the vanished source, so it would catch a poison that names a home the migration does not read.
  - Not over-tested. The one redundancy is `:128-130`, a second assertion in the pin-wins subtest that the path is not the poisoned one — already implied by the `got != want` check at `:125-127`. Harmless; not reported.
  - Criterion 3 (removal + exit-code preservation) has no test and cannot reasonably have one — `TestMain` cannot observe its own teardown. Verified by reading `:77-81`.
  - `cmd/alias_test.go:40` and `:132` read `os.UserHomeDir()` to build their expectations for tilde expansion; both they and the production expansion now read the same poisoned home, so they stay consistent.

CODE QUALITY:
- Project conventions: Followed. The lane rule is respected (this is unit-lane test scaffolding, builds nothing); the `os.Setenv`-in-`TestMain` pattern matches the twelve sibling `testmain_isolation_test.go` files and the existing `.golangci.yml` exclusion; no production code touched.
- SOLID principles: Good. `pinToolchainCaches` is one named concern with an error return, kept separate from `TestMain`'s sequencing.
- Complexity: Low.
- Modern idioms: Yes. No `modernize` opportunities in the added code (the `strings.Split` result is indexed, not ranged).
- Readability: Good. Every non-obvious decision — real directory over `/nonexistent`, ordering the cache pin before the poison, reading values from the toolchain rather than rebuilding them — carries a comment stating why, and each of those comments holds against the code.
- Issues: One stale rationale outside the changed files (see FINDINGS).

BLOCKING ISSUES:
- None. The named hazard is closed structurally, the tests prove it, and every acceptance criterion is met in substance.

FINDINGS:
- [in-scope] [contained] .golangci.yml:34-37 — the errcheck exclusion's rationale reads "TestMain poisons PORTAL_* env vars ... and the values are compile-time constants, so the error is a dead branch", but two of the `os.Setenv` calls it now covers take runtime values: `cmd/testmain_isolation_test.go:64` passes the `os.MkdirTemp` result and `:42` passes a value parsed out of `go env`. Restate the reason as what is still true — the *keys* are constant, valid names, so `os.Setenv`'s only error mode (empty key, or `=`/NUL in key or value) is unreachable — and drop the "values are compile-time constants" clause. FAILS: an auditor deciding whether the exclusion still earns its place is handed a justification the code it points at contradicts, and the true reason (constant keys, not constant values) is nowhere recorded.
- [in-scope] [contained] cmd/testmain_isolation_test.go:64 — `HOME` is poisoned but `XDG_CONFIG_HOME` is not, so the poison only decides the default resolve when that variable is empty. Set `os.Setenv("XDG_CONFIG_HOME", "")` beside the `HOME` poison (safe as written: every `XDG_CONFIG_HOME` reference in `cmd`'s test files is a `t.Setenv`, none read an ambient value, and `internal/xdg/xdg.go:26` already treats empty as unset), and extend `TestPackageWideHomePoison`'s first subtest to stop pinning it locally so the poison is what the assertion observes. FAILS: on a machine where the developer sets `XDG_CONFIG_HOME`, a `cmd` test resolving a non-overridden config path without pinning it reads the developer's real config base rather than the poisoned home — concretely, any future test reaching `buildResolver` (`cmd/spawn_seams.go:53`) without `isolateTerminalsFile`, since `PORTAL_TERMINALS_FILE` is the one config-file variable `TestMain` does not poison — which makes `cmd/config_test.go:12-13`'s "can never reach the developer's files" false in that environment. The rename half of the hazard stays closed either way (the migration's source path is `HOME`-rooted), and `XDG_CONFIG_HOME` is unset on the current machine, which is why nothing fails today.
