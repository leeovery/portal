TASK: resume-hooks-silently-lost-8-20 (tick-2db159) — "The Go-Source Guard Scan Skeleton Is Re-Authored At Fourteen Sites; Two Packages Independently Extracted Their Own Local Version"

Move the enumerate → parse → count-what-was-scanned skeleton into `internal/sourceguardtest` as a
`ParsedSource` value plus `ParsePackageSources` / `ParseSources`, re-point the fourteen sites that
write it inline, and delete the two private extractions.

ACCEPTANCE CRITERIA (from the plan task):
1. No guard writes its own enumerate-parse-count loop; all fourteen route through the shared helper.
2. Both private extractions are gone.
3. A guard whose scan yields nothing fatals, at every site.
4. One parse mode across the tree, expressed at the helper.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 task — an implementation-analysis (consolidation) cycle, not specified bugfix work —
so its authority is its own body rather than the specification, per the shared verifier context. The
subject is test scaffolding for the repo's ~20 source guards; the specification's resume-hook subject
matter does not bear on it. The binding project convention is CLAUDE.md's `sourceguardtest` row (the
package must stay stdlib-only-plus-`harnesstest`/`portalbintest` and untagged so every guard built on
it runs in the unit lane) and the repo rule that a guard which scans nothing must fail rather than
report a safety it is not providing.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Helper: `internal/sourceguardtest/parsesources.go` — `ParseMode` (:18), `ParsedSource` (:22),
    `ParsePackageSources` (:41), `ParseSources` (:71). Both entry points take `harnesstest.TestingT`,
    fatal on an unparseable file (:79) and on an empty result (:85), and return nil after fatalling so
    a non-stopping stand-in can observe the failure.
  - Commit: `2583a929` — 29 files, the fourteen sites named in the Do list minus one that no longer
    existed (see Notes), plus the CLAUDE.md architecture-row amendment.
  - Deletions: `internal/hooks`'s `scanPackageCalls` and its coverage file
    `cleanstale_staleness_guard_scan_test.go`; `internal/theme`'s `parseThemeSources` /
    `parsedThemeSource` / `themeSourceFiles`. A repo-wide grep for either name returns nothing.
- Notes:
  - Criterion 1 holds in the delivered tree. Every remaining `parser.ParseFile` call is over an
    in-memory fixture string, never a disk enumeration: `internal/logtest/install_guard_test.go:149`,
    `internal/portalbintest/lane_guard_test.go:132`,
    `internal/portaltest/teardown_guard_coverage_rule_test.go:148`,
    `internal/sourceguardtest/buildconstraint_test.go:135` (all four under `sourceguardtest.ParseMode`),
    `internal/sourceguardtest/foreachfunccall_test.go:91` and
    `internal/capture/theme_panel_fixture_test.go:475`. No enumerate-parse-count loop survives anywhere.
  - Do-list item 3 named three "variants", one of which — `internal/session/panetoken_test.go` — had
    already been deleted by an earlier phase (`git ls-tree 2583a929^` finds no such path). Nothing to
    re-point; the other two (`internal/tmux/target_composition_guard_test.go`,
    `cmd/open_theme_nomination_test.go`) were re-pointed.
  - Four of the listed paths have since been renamed or moved by phases 9–10 and still route through
    the helper at their new homes: `cmd/deps_seam_guard_test.go` → `cmd/seam_guard_test.go:62,125,132`;
    `cmd/run_hook_stale_cleanup_decline_error_guard_test.go` →
    `internal/hooksweep/decline_error_guard_test.go:16`; `internal/restoretest/orchestrator_literal_guard_test.go`
    → `internal/restoretest/literal_guard_scan_test.go:91` (now via `RepoSources`, itself built on
    `ParseSources` at `internal/sourceguardtest/reposources.go:108`);
    `internal/portaltest/teardown_guard_coverage_test.go:161` likewise via `RepoSources`.
  - Criterion 3 holds. The three sites that previously carried their own `scanned == 0` tripwire
    (`internal/theme/loader_construction_guard_test.go`, `cmd/seam_guard_test.go`'s
    `runSeamAssignmentGuard`, `internal/hooks`'s `scanPackageCalls`) dropped it only because
    `ParseSources` now fatals on an empty path set — the property moved rather than being lost.
    Sites that filter an enumeration before parsing (`cmd/seam_guard_test.go:55-62`,
    `internal/theme/broken_builtin_test.go`, `internal/restoretest`) hand the filtered slice to
    `ParseSources`, so a filter that selects nothing fatals. Guards with a second, narrower tripwire
    kept it: `cmd/hooks_pane_token_width_guard_test.go:49`,
    `internal/theme/loader_construction_guard_test.go:42`, `internal/theme/broken_builtin_test.go:281`,
    `internal/tmux/target_composition_guard_test.go:591`, `internal/restoretest/literal_guard_scan_test.go:101-106`.
  - The mode changes the consolidation forces are behaviour-neutral. Two sites lost
    `parser.ImportsOnly` (`internal/hooks/leaf_guard_test.go`) — `File.Imports` is populated under a
    full parse too — and several gained `ParseComments`. No re-pointed predicate matches
    `*ast.Comment`/`*ast.CommentGroup`; they match `*ast.BasicLit`, `*ast.CompositeLit`,
    `*ast.SelectorExpr`, `*ast.CallExpr`, `*ast.AssignStmt` and `*ast.Ident`, none of which a retained
    comment can now impersonate. No new false positive is reachable.
  - Each re-pointed guard keeps its predicate, its rationale comment and its verdict; the diffs are
    substitutions of `source.Path` / `source.Fset` for the loop-local `path` / shared `fset`. One
    consequence worth naming: `cmd/open_theme_nomination_test.go`'s `parsePackageFilesByName` moved
    from one shared `FileSet` to one per file, but its three consumers
    (`:155`, `:187`, `:229`) read identifiers only and resolve no positions, so nothing depends on the
    discarded file set.
  - CLAUDE.md's `sourceguardtest` architecture row was amended in the same commit to name
    `ParsePackageSources` / `ParseSources`, the `ParsedSource` shape and the stated `ParseMode`; the
    row matches the code as it now stands.

TESTS:
- Status: Adequate
- Coverage: `internal/sourceguardtest/parsesources_test.go` carries exactly the four invariants the
  task names, one test each, plus two that the helper's own contract earns:
  - "it returns one ParsedSource per file" — `:12`, asserting path, non-nil fset/file, and that
    positions resolve against the file each source was parsed from.
  - "it fatals on an unparseable file" — `:53`, asserting the fatal fired, that nil came back, and that
    the message names the offending file.
  - "it fatals when the package yields no source" — `:88`, asserting the message names the directory,
    which is what distinguishes the enumerate fatal from the parse-nothing one.
  - "it includes test sources only when asked" — `:105`.
  - `:40` pins the stated mode by reading a build-tag comment back off a parsed file — the one property
    that would silently regress if `ParseMode` dropped `ParseComments`.
  - `:72` pins the empty-input fatal by its wording ("stopped looking"), so the two empty-set fatals
    cannot be confused for one another.
- Notes:
  - The task asked that `scanPackageCalls`'s coverage test be re-aimed at the shared helper "asserting
    *which* fatal it exercised rather than recording it unread". The old test
    (`internal/hooks/cleanstale_staleness_guard_scan_test.go`) captured `msg` and never read it; the
    replacements at `parsesources_test.go:67`, `:83` and `:100` all assert on it. Delivered.
  - Not over-tested: no case duplicates another, and the three fatal cases each pin a distinct fatal.
  - The tests drive a local non-stopping `recordingT` (declared at
    `internal/sourceguardtest/packagedeps_test.go:49`) rather than `harnesstest.Recorder`. That is the
    documented carve-out, not a lapse: their subject is what the helper *returns* after it fatals
    (`sources != nil` at `:64`, `:79`, `:97`), which a stand-in that stops cannot observe — the same
    reason CLAUDE.md records for `PackageDeps`, whose stub this reuses in the same test package.
  - The temporary `recordingT` stubs this commit added to `internal/tmux` and `internal/restoretest`
    are gone from the current tree (both now drive `harnesstest.Recorder`), and no orphan copy remains:
    a repo-wide grep finds `recordingT` only in `internal/sourceguardtest`.
  - The helper's tests are untagged and in `package sourceguardtest_test`, so they run in the unit lane
    as CLAUDE.md requires of everything this package serves.

CODE QUALITY:
- Project conventions: Followed. `internal/sourceguardtest/leaf_guard_test.go:28-31` pins the package's
  transitive dependency set to `harnesstest` + `portalbintest` across both lanes and asserts no build
  tag on any of its sources, so `parsesources.go`'s `harnesstest` import is inside the declared
  allowlist and the new file cannot gate the guards out of the unit lane. `doc.go:20` names
  `ParsePackageSources` in the `-overlay` rationale and reads true of the code (it parses from disk
  with a nil `src`, `parsesources.go:77`).
- SOLID principles: Good. The helper does one thing (enumerate → parse → refuse an empty result) and
  leaves the predicate, the rationale and the verdict at each guard, which is what keeps fourteen
  differently-shaped guards on one skeleton. `ParsePackageSources` composes `PackageGoFiles` with
  `ParseSources` rather than restating either.
- DRY: The point of the change, and it lands: one parse step, one stated mode, one scanned-nothing
  tripwire, where there were three modes and a per-copy decision on the tripwire.
- Complexity: Low. Both entry points are straight-line loops with early fatal returns.
- Modern idioms: Yes. `ParseMode` is a typed `parser.Mode` const with the reason for each flag stated;
  the `t.Fatalf(...); return nil` pairing is the established shape for a helper whose failure path its
  own suite drives with a non-stopping stand-in.
- Readability: Good. Every doc comment states the *reason* for the rule it encodes — the empty-result
  fatal is justified in the comment at `parsesources.go:39-40` and again at `:69-70` in the words the
  guards themselves use, so a reader who deletes it knows what they are deleting.
- Issues: None that clear the bar. Two observations recorded here rather than as findings, because
  neither has a consequence anyone can name:
  - `internal/tmux/target_composition_guard_test.go:580-582` returns nil when the parsed set is empty.
    That branch was load-bearing when this commit introduced it (the then-local non-stopping stub would
    otherwise have taken a second fatal and overwritten the message the test asserts on) and became
    unreachable when a later phase swapped in the stopping `harnesstest.Recorder`. It cannot mask a
    scanned-nothing pass: `ParsePackageSources` fatals first for any non-empty `dirs`, and an empty
    `dirs` is foreclosed upstream by `importersOfTmux` (`:90-92`), which errors when the import scan
    resolves nothing.
  - Criterion 4 is met for every scan of the tree, but `internal/capture/theme_panel_fixture_test.go:475`
    and `internal/sourceguardtest/foreachfunccall_test.go:91` still parse authored in-memory fixture
    strings under `0` and `parser.SkipObjectResolution`. Neither reads comments, neither reads a file
    on disk, and neither is one of the fourteen sites; the criterion's subject is the enumerate-parse
    skeleton, which they are not part of.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
