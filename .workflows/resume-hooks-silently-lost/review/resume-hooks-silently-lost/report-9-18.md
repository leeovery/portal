TASK: resume-hooks-silently-lost-9-18 — The Repo-Wide Source-Scan Preamble Is Re-Authored At Every Guard (tick-69c5be)

ACCEPTANCE CRITERIA:
1. `internal/sourceguardtest` exports one repo-wide scan entry point, and `portalbintest.ProjectRoot` is called from exactly one place across the guard family.
2. Findings from every re-pointed guard read as root-relative repo paths, produced by one implementation.
3. No guard declares its own relativisation helper, inline `filepath.Rel` or `TrimPrefix` variant.
4. Each re-pointed guard still fails on exactly the violations it failed on before, with its own wording preserved.
5. The scanned-nothing tripwire fires from one place, and a driver handed an empty tree is fatal.

STATUS: complete

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context: phases 6–9 are consolidation/quality tasks the implementation phase
itself generated). The governing project convention is CLAUDE.md's `sourceguardtest` architecture-table row, which
declares that package the home of "the Go-source scanning primitives the repo's ~20 unit-lane source guards share",
and the unit-lane purity rule (a guard built on these primitives must stay untagged and stdlib-adjacent, or it drags
a lane onto every guard at once).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/sourceguardtest/reposources.go:87` — `RepoSources(t, sel, opts...) (root, []ParsedSource)`, composing
    `ProjectRoot` → `GoSourceFiles` → the `Selection` narrowing → `ParseSources`, then one `filepath.Rel` per source
    at `internal/sourceguardtest/reposources.go:110`.
  - `internal/sourceguardtest/reposources.go:14-35` — `Selection` (`AllSources`/`TestSources`/`NonTestSources`).
  - `internal/sourceguardtest/reposources.go:45` — `Rooted(root)`, the option that anchors the same scan at a staged
    fixture tree, which is what lets each guard's own rule tests drive the real driver.
  - `internal/sourceguardtest/reposources.go:53` — `ProjectRoot(t)`, the single fatal-on-failure root resolution.
  - `internal/sourceguardtest/parsesources.go:56` — `PackageSource(t, dir, name)`, the package-narrowed sibling.
  - `internal/sourceguardtest/parsesources.go:31` — `ParsedSource.Position`, which renames the position's file to the
    scan's own `Path` so a `file:line:col` finding needs no relativisation of its own.
  - Re-pointed repo-wide guards: `internal/portalbintest/lane_guard_test.go:51`,
    `internal/portaltest/teardown_guard_coverage_test.go:161`, `internal/restoretest/literal_guard_scan_test.go:91`,
    `internal/logtest/install_guard_test.go:57`, `internal/theme/loader_construction_guard_test.go:17`,
    `internal/prefs/appearance_api_guard_test.go:31`, plus four beyond the six the task listed:
    `internal/theme/slug_collapse_guard_test.go:17`, `internal/theme/broken_builtin_test.go:262`,
    `internal/log/discard_guard_test.go:124`, `internal/log/migration_guard_test.go:22`,
    `internal/tui/theme_source_guard_test.go:18`, `internal/tui/restore_source_guard_test.go:89,164`.
  - Delivered across two commits: `33e01bda` (the bulk — driver, tests, re-points) and `51616eac` (polish: dropped the
    now-redundant `rel string` parameter beside `ParsedSource`, corrected two `<file>:<line>` doc comments to
    `<file>:<line>:<column>`, added `filesByName` so a suite parses its package once, and amended `doc.go`'s stated
    dependency set to admit `portalbintest`).
- Notes: verified each acceptance criterion against the tree as it now stands.
  - AC1: `RepoSources` is the sole repo-wide entry point — `GoSourceFiles`, still exported, now has no caller outside
    its own test (`internal/sourceguardtest/gosourcefiles_test.go:75,82`). `portalbintest.ProjectRoot` has exactly one
    real call site, `internal/sourceguardtest/reposources.go:56`; the other two grep hits are `portalbintest`'s own
    test of that function (`internal/portalbintest/project_root_test.go:15`) and a raw-string fixture body inside
    `internal/portalbintest/lane_guard_rule_test.go:37`, which is text, not a call.
  - AC2: every re-pointed guard names findings from `source.Path` (or `source.Position(...)`), both set by the single
    `filepath.Rel` at `reposources.go:110`.
  - AC3: no relativisation helper survives in any guard — `relToRoot` (formerly `internal/logtest/install_guard_test.go`),
    `relToProjectRoot` (formerly `internal/theme/slug_collapse_guard_test.go`) and the
    `strings.TrimPrefix(finding, root+separator)` variant (formerly `internal/restoretest/literal_guard_scan_test.go`)
    are all gone. The seven inline `filepath.Rel` sites named in the task body are gone; the six that remain in the
    tree are unrelated (`internal/portaltest/fingerprint.go:59`, `internal/portaltest/isolated_env_test.go:342`,
    `cmd/testmain_home_poison_test.go:107`,
    `cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go:78`, and the two enumerator tests
    `internal/sourceguardtest/gosourcefiles_test.go:88` / `packagegofiles_test.go:71`).
  - AC4: read the diff of every re-pointed guard against its predecessor. Rule wording, exemption sets and defect
    messages are preserved verbatim in each; the mechanical substitutions are `rel` → `source.Path` and
    `parser.ParseFile` → the shared parse. Three deliberate widenings, each a strict superset of the old subject with
    no reachable violation dropped: `internal/log/migration_guard_test.go` moves from a `.git`/`vendor`/`node_modules`
    exclusion to `GoSourceFiles`' any-dot-dir exclusion; `internal/tui/theme_source_guard_test.go`'s
    `TestNoPackageLevelThemeVar` moves from the curated `centralisedColourSites` list to every production file of
    `internal/tui` (both the curated list and `allGoFiles` are deleted with no remaining reader); and the two
    `internal/restoretest` literal guards' findings gained a `:column` because they now render through
    `ParsedSource.Position`, with both doc comments corrected to match in `51616eac`.
  - AC5: the tripwire is `ParseSources`' `"parsed no sources, so a guard over them would pass by having stopped
    looking"` (`internal/sourceguardtest/parsesources.go:85`), reached by every selection that matches nothing;
    a wholly empty tree short-circuits one rung earlier in `GoSourceFiles`, whose own pre-existing error carries the
    same "stopped looking" wording, and `RepoSources` fatals on it at `reposources.go:97`. Both routes are fatal and
    both are asserted (`internal/sourceguardtest/reposources_rooted_test.go:38`,
    `internal/log/discard_guard_test.go:99`).
  - The three `internal/theme` `find-the-package-source-named-X` twins are collapsed onto `PackageSource`
    (`internal/theme/badge_test.go:223`, `internal/theme/resolution_test.go:377`,
    `internal/theme/setting_test.go:267`).
  - `ParseMode` now has one application site. The only surviving direct `parser.ParseFile` calls parse in-memory
    fixture strings in rule tests, and all but one pass `sourceguardtest.ParseMode`
    (`internal/portalbintest/lane_guard_test.go:132`, `internal/logtest/install_guard_test.go:149`,
    `internal/portaltest/teardown_guard_coverage_rule_test.go:148`,
    `internal/sourceguardtest/buildconstraint_test.go:135`).
  - CLAUDE.md's `sourceguardtest` row was amended in the same work and its claims hold: `RepoSources`, `Rooted`,
    `ParsedSource.Position`, `PackageSource` and the "taken directly by a guard that only needs to name a path within
    the tree" note all match the code (`cmd/config_identity_test.go:158`,
    `cmd/capturetool/import_guard_test.go:33`, `cmd/capturetool/shared_constructor_test.go:13` take `ProjectRoot`
    directly).

TESTS:
- Status: Adequate
- Coverage: the three tests the task named all exist and assert what they claim.
  - `"it returns root-relative paths for every parsed source"` — `internal/sourceguardtest/reposources_test.go:12`;
    asserts the root is absolute, every `Path` is not, every source came back parsed, and that the suite's own file is
    present (so the scan cannot pass by having narrowed to nothing).
  - `"it narrows to test sources or to non-test sources as asked"` — `reposources_test.go:36`; checks both suffix
    predicates and that the two selections partition `AllSources`, which is the property a suffix check alone misses.
  - `"it fatals when the selection matches no source at all"` — `reposources_rooted_test.go:38`; drives the real
    driver through `harnesstest.Recorder` over a staged tree holding only a production file, and pins exactly one
    fatal carrying the "stopped looking" wording.
  - `"it scans the tree it is rooted at"` — `reposources_rooted_test.go:16`; pins the returned root and the exact
    ordered relative paths including a nested directory.
  - `internal/sourceguardtest/projectroot_test.go:13` pins that `ProjectRoot` and `RepoSources` resolve the same root,
    which is the property that keeps a directly-taken root and a scanned one interchangeable.
  - `internal/sourceguardtest/parsedsource_position_test.go:11` pins `Position`'s filename rewrite and the full
    `path:line:col` rendering — the mechanism the restoretest guards' findings now go through.
  - `internal/sourceguardtest/packagesource_test.go:12` covers the package-narrowed sibling's hit and its
    not-found fatal.
  - The re-pointed guards' own rule suites were adapted to pass `Rooted(...)` rather than a root parameter, with
    their assertions untouched (`internal/log/discard_guard_test.go:86,102`, `internal/restoretest/literal_guard_test.go:170,195,214,260`).
    This is the change the driver was meant to enable: a rule test now drives the production scan over a fixture tree
    instead of a parallel loop.
- Notes: no over-testing — each subtest pins a distinct property, and there is no duplicate of the partition or
  fatal assertions. The one uncovered branch is `reposources.go:110-114`'s relativise fatal, which is unreachable
  given the paths come from a walk of the same root; testing it would require a fake enumerator the package does not
  have, and its absence costs nothing.

CODE QUALITY:
- Project conventions: Followed. `internal/sourceguardtest` stays untagged and its leaf guard was amended in the same
  change to admit `portalbintest` with the reason stated inline (`internal/sourceguardtest/leaf_guard_test.go:23-32`),
  so the new edge is asserted rather than merely allowed — and `portalbintest` is itself stdlib-only and untagged, so
  the unit-lane purity rule CLAUDE.md states is preserved. The `doc.go` dependency sentence was corrected in the same
  breath (`internal/sourceguardtest/doc.go:3-6`).
- SOLID principles: Good. `Selection` and `ScanOption` separate "which lane" from "which tree" without either
  leaking into the other; `RepoSources` composes the four existing primitives rather than absorbing them, so
  `GoSourceFiles`/`ParseSources` remain independently usable by the four guards that legitimately need paths before
  parsing (`cmd/seam_guard_test.go:40`, `internal/capture/theme_panel_message_fixtures_test.go:327`,
  `internal/tui/builtin_theme_table_test.go:66`, `internal/tmux/target_composition_guard_test.go:177`).
- Complexity: Low. The driver is one linear function; `scanRoot` and `Selection.accepts` are each a handful of lines.
- Modern idioms: Yes. Functional options with an unexported config, a typed enum with an `accepts` predicate rather
  than a boolean pair, and named results used only where the doc comment refers to them.
- Readability: Good. Each exported symbol states why it exists rather than what it does, and the fatal messages say
  what a guard would wrongly report if the condition were tolerated — consistent with the rest of the guard family.
- Issues: none.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
