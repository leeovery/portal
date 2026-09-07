## Attempt 1

ISSUES:
- `cmd/capturetool/import_guard_test.go:34` and `cmd/capturetool/shared_constructor_test.go:13` still call `portalbintest.ProjectRoot()` directly, each with its own `resolve project root` fatal — the acceptance criterion "`ProjectRoot` is called from exactly one place across the guard family" is unmet. The executor flagged this as a deliberate deviation, and the *reasoning* is right (neither site is a source scan; routing them through `RepoSources` would parse ~1000 files to obtain one string) — but the conclusion does not follow, because the criterion is reachable without doing that. It also makes a claim now written into `CLAUDE.md` false: "…through `sourceguardtest.RepoSources`, which is the one place the guard family calls it from."
  FIX: Extract the fatal-wrapping root resolution already inside `scanRoot` (`internal/sourceguardtest/reposources.go:51-67`) as an exported `ProjectRoot(t harnesstest.TestingT) string`, have `scanRoot` call it, and re-point both `cmd/capturetool` sites onto it. That deletes the last two copies of the preamble, makes the CLAUDE.md sentence true as written, and costs no extra parsing — `import_guard_test.go` still gets a bare root for `sourceguardtest.InDir`, `shared_constructor_test.go` still joins its two named paths.
  ALTERNATIVE: Accept the deviation and amend the CLAUDE.md sentence to say `RepoSources` is the only place the *scan* resolves it, recording the two root-anchoring sites as sanctioned. Cheaper, but leaves the criterion unmet and two copies of the preamble standing — which is the duplication the task exists to remove. Take the fix.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- `internal/restoretest/orchestrator_literal_guard_test.go:127` — names a `root` parameter the function no longer has.
  OLD: // orchestrator type in a _test.go under root, as "<file>:<line>". It reads the
  NEW: // orchestrator type in a _test.go the scan reaches, as "<file>:<line>". It reads the
- `internal/restoretest/session_restorer_literal_guard_test.go:151` — same stale `root` reference.
  OLD: // session-restorer type in an integration-tagged _test.go under root, as
  NEW: // session-restorer type in an integration-tagged _test.go the scan reaches, as

NOTES:
- Verdict preservation independently confirmed on eleven guards by planting violations in a throwaway tree; all fired with their own wording and a root-relative path. The reviewer additionally probed the one guard the executor reported as unprobeable (a file need only parse, not compile, for an AST guard) and confirmed it fires, and confirmed one lane-discriminating guard correctly stays silent on an untagged plant.
- The leaf-guard allowlist edit is sound rather than a weakening: the assertion judges the transitive set, so admitting one stdlib-only untagged package admits exactly that package, and the day it grows a dependency the guard flags it.
- Both additions beyond the Do list earn their place: `Rooted` is what lets the driver serve the two fixture-driven guards without a second export, and `ParsedSource.Position` is the direct replacement for the deleted variant, without which the deleted sites would grow back.
- One scan-set change, judged sanctioned: two `internal/log` guards previously walked with a narrower skip list, so twelve vendored skill-asset files under a dot-directory leave their scan. Grepped — none contains the symbols either guard polices, so no verdict changed, and the Do list mandates composing the enumerator whose documented rule is exactly this.
- Residue worth a later sweep: `internal/tui/theme_flash_precedence_test.go:194-201` now parses one package twice in a subtest; four `rel := source.Path` aliases stand where the conversion left them; and several guards still reach `source.Fset.Position(...)` for a bare line where `source.Position(...)` now exists, so two routes to a position coexist.
- `Selection.accepts` uses `default: return true`, so an out-of-range value behaves as `AllSources`. Fine for a test helper; noted because the enum is exported.

## Attempt 2

ISSUES:
- `internal/tui/restore_source_guard_test.go:40-45` — `TestRestorePath_ReadsNoTheme` still hand-authors a parse of an on-disk source under its own parse mode (`parser.ParseFile(fset, filepath.Join(".", restoreFileName), nil, parser.SkipObjectResolution)`), which is the last such site in the tree. This is exactly the shape the task added `PackageSource` for, and the same file's third subtest at line 96 *was* re-pointed onto `RepoSources` — so the file is left half-converted, and the stated outcome "the parse mode stops varying by site" does not hold: `restore.go` is the one file in the repo parsed without `ParseComments`.
  FIX: Replace lines 40-45 with `file := sourceguardtest.PackageSource(t, ".", restoreFileName).File`. `fset` and `path` are dead afterwards (`grep -n "token\.\|parser\." internal/tui/restore_source_guard_test.go` shows both are used only on those lines), so drop the now-unused `go/token` and `go/parser` imports. The sibling `internal/tui/theme_source_guard_test.go` in the same test package already reaches the package's production sources this way, so the pattern is established next door.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- `internal/sourceguardtest/doc.go:3-5` — the package doc claims a stdlib-only dependency set, which this change falsified: `reposources.go` imports `internal/portalbintest`, and the package's own leaf guard was amended to admit it.
  OLD:
// files rather than by executing them. Beyond the shared *testing.T stand-in its
// helpers report through, it depends on stdlib alone, and it carries no build
// tag, so every guard it serves runs in the unit lane.
  NEW:
// files rather than by executing them. Beyond the shared *testing.T stand-in its
// helpers report through and portalbintest, whose module-root resolution the
// repo-wide scan is anchored at, it depends on stdlib alone, and it carries no
// build tag, so every guard it serves runs in the unit lane.

NOTES:
- The four surviving direct `PackageGoFiles` callers (`cmd/seam_guard_test.go:40`, `internal/capture/theme_panel_message_fixtures_test.go:327`, `internal/tui/builtin_theme_table_test.go:66`, `internal/tmux/target_composition_guard_test.go:177`) are correct as they stand — each needs paths rather than ASTs. No action.
- The remaining `parser.ParseFile` calls outside the driver all parse in-memory fixture strings, which have no file on disk to enumerate. Three already pass `sourceguardtest.ParseMode`; `internal/capture/theme_panel_fixture_test.go:475` and `internal/sourceguardtest/foreachfunccall_test.go:91` pass their own mode, harmless for a hand-written fixture.
- Three of the five new test files are named after a symbol rather than the source file that declares it (`projectroot_test.go` and `reposources_rooted_test.go` for `reposources.go`; `packagesource_test.go` and `parsedsource_position_test.go` for `parsesources.go`). Not raised as an issue because the repo at large does not hold that line.
- Two behavioural side effects of routing text guards through the driver, both benign and both widening safety: `GoSourceFiles` skips every dot-directory where the old `internal/log` walks skipped only `.git`/`vendor`/`node_modules`, and a `.go` file that fails to parse now fatals the whole guard instead of being text-scanned anyway.
- Acceptance criteria: 4 of 5 fully met. `portalbintest.ProjectRoot` has exactly one real call site (`reposources.go:56`); the executor's report misclassified `internal/portalbintest/lane_guard_rule_test.go:37` as a real call blocked by an import cycle — it is a string-literal fixture, neither. The criterion holds regardless.

## Attempt 3

ISSUES:
- `internal/tui/theme_flash_precedence_test.go:194` and `:200` — the internal/tui production sources are enumerated and parsed twice inside one subtest. `parsePackageFilesByName(t)` (`nomination_test.go:251`) *is* `sourceguardtest.ParsePackageSources(t, ".", false)` reshaped into a map, and line 200 runs the identical call again eight lines later. Before this change the subtest parsed once and iterated that result; the refactor split it in two. In a task whose subject is collapsing repeated scan preambles, this re-introduces one.
  FIX: adopt the shape this same commit already uses at `internal/tui/theme_source_guard_test.go:36-43` — call `ParsePackageSources` once into `parsed`, build the `map[string]*ast.File` from it for `themeCopyVocabulary`, then range over `parsed` for the `setFlash` scan.
  CONFIDENCE: high

- `internal/logtest/install_guard_test.go:77` and `internal/portalbintest/lane_guard_test.go:72` — the `rel string` parameter now carries exactly `source.Path` at every call site. Production loops pass `source.Path` (`install_guard_test.go:64`, `lane_guard_test.go:62`) and the fixture stagers construct `ParsedSource{Path: rel, …}` and pass `rel` beside it (`install_guard_test.go:153`, `lane_guard_test.go:154,158`). The parameter existed because `source.Path` used to be absolute; now it is a second carrier for one value, and a caller passing a `rel` that disagrees with the source it scanned would produce a finding whose path names one file and whose line names another — the exact two-behaviours-for-one-rule condition this task set out to end.
  FIX: drop the `rel` parameter from both helpers and read `source.Path` inside (`helperRef.File`/`handlerInstall.File`); the stagers already set `Path: rel` on the `ParsedSource` they build, so their call sites just lose an argument.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- `internal/restoretest/orchestrator_literal_guard_test.go:126-131` — the stated finding shape is falsified by the code: `source.Position(lit.Pos()).String()` renders `file:line:column`, not `<file>:<line>`. The diff touched this sentence and left the over-long line unwrapped.
  OLD:
// scanTestOrchestratorLiterals reports every composite literal of the
// orchestrator type in a _test.go the scan reaches, as "<file>:<line>". It reads the
// AST rather than the text, so a mention of the type inside a string — this
// guard's own fixtures — is not a finding. Every lane is policed: the
// integration-tagged files are most of the subject, and an unpinned literal is
// as silent in one lane as the other.
  NEW:
// scanTestOrchestratorLiterals reports every composite literal of the
// orchestrator type in a _test.go the scan reaches, as "<file>:<line>:<column>".
// It reads the AST rather than the text, so a mention of the type inside a
// string — this guard's own fixtures — is not a finding. Every lane is policed:
// the integration-tagged files are most of the subject, and an unpinned literal
// is as silent in one lane as the other.

- `internal/restoretest/session_restorer_literal_guard_test.go:150-153` — same falsified finding shape, same touched sentence.
  OLD:
// scanIntegrationSessionRestorerLiterals reports every composite literal of the
// session-restorer type in an integration-tagged _test.go the scan reaches, as
// "<file>:<line>". It reads the AST rather than the text, so a mention of the
// type inside a string — this guard's own fixtures — is not a finding.
  NEW:
// scanIntegrationSessionRestorerLiterals reports every composite literal of the
// session-restorer type in an integration-tagged _test.go the scan reaches, as
// "<file>:<line>:<column>". It reads the AST rather than the text, so a mention
// of the type inside a string — this guard's own fixtures — is not a finding.

NOTES:
- All five acceptance criteria verified met, independently: one repo-wide entry point (`reposources.go:87`), `portalbintest.ProjectRoot` at exactly one guard-family call site, root-relative findings confirmed by planting violations in three guards with original wording preserved, no surviving relativisation helper or TrimPrefix variant, single scanned-nothing tripwire with an empty tree fatal.
- Performance is a non-issue: `RepoSources` parses all 999 repo `.go` files per call and is reached ~18 times across the unit lane, but the seven re-pointed `internal/tui` guards together run in 0.17s.
- Latent fragility worth knowing rather than fixing: `GoSourceFiles` does not exclude `testdata/`, so a deliberately-malformed `.go` fixture dropped there would fatal the four text-only guards. No such file exists today.
- `PackageSource` infers its search scope from the filename suffix (`parsesources.go:59`); `packagesource_test.go` covers only non-test names. A third case asserting a `_test.go` name resolves would pin the inference.
- `reposources_rooted_test.go:14` exercises `Rooted` only under `TestSources`.
- Five one-line `rel := source.Path` / `name := source.Path` aliases survive as pure restatement (`internal/tui/restore_source_guard_test.go:166`, `internal/theme/broken_builtin_test.go:266`, `internal/theme/loader_construction_guard_test.go:21`, `internal/portaltest/teardown_guard_coverage_test.go:165`, `cmd/state_daemon_lock_pid_ordering_test.go:90`). Inlinable if the fix round is already in those files.
- `internal/restoretest/literal_guard_scan_test.go:17` has an over-long first comment line left by the same-line edit. Cosmetic.
- `ProjectRoot`, `Selection`, `ScanOption` and `Rooted` all live in `reposources.go` while the package otherwise runs one primitive per file, and `ProjectRoot` has a `projectroot_test.go` with no `projectroot.go`. Defensible as one cohesive scan family.
