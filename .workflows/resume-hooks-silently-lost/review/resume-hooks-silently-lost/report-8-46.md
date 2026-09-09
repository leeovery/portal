TASK: resume-hooks-silently-lost-8-46 — "The Target-Composition Guard's Exemption Is Wider Than Its Recognition" (tick-e7e9ed, phase 8, severity low, sourced from the opportunity bank)

ACCEPTANCE CRITERIA:
- [x] A bare target spent through a non-method helper is caught (proven by the staged probe).
- [x] The real tree produces zero new findings under the tightened guard.
- [x] The split-composition branch has a fixture and fails when broken.
- [x] The four helpers share one naming shape, and the file name matches its subject.

STATUS: complete

SPEC CONTEXT:
The work unit's specification governs the resume-hook key change and its sweep/lock machinery. It says nothing about
tmux target exactness — a grep of
`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md` for
"target composition", "exactness", "TargetExact" and "prefix-match" returns nothing, and no corrigendum touches the
subject. This is a phase-8 implementation-analysis task, so its authority is its own body (as the verifier context
states). The binding project standard is CLAUDE.md's `internal/tmux` row: every per-session `-t` Portal composes must
be pinned, because tmux prefix-matches a bare session name and would resolve `-t foo` onto a live `foo-2` once `foo`
is gone. The guard under review is the standing enforcement of that rule.

IMPLEMENTATION:
- Status: Implemented; partly superseded by a later task, in the direction the task itself nominated.
- Location: commit `46af78db` (21 files). The five Do items landed as written:
  1. `internal/tmux/target_composition_guard_test.go:658-672` — `targetTakingFuncs` (renamed from
     `targetTakingMethods`) no longer skips a declaration with no receiver: "Methods and plain functions alike are
     read". `internal/tmux/target_composition_guard_test.go:762-766` — `bareTargetArguments` reads the callee through
     `sourceguardtest.CalleeName` instead of requiring an `*ast.SelectorExpr`, so a bare-identifier call reaches the
     rule. Together these close the exemption-wider-than-recognition gap the task exists to shut.
  2. `internal/tmux/target_composition_guard_test.go:226-248` — the missing `-t` ends its argv fixture.
  3. Vocabulary unified onto the `<kind>TargetExact` shape: `internal/tmux/tmux.go:448` (`SessionTargetExact`),
     `:467` (`CoordTargetExact`), `:417` (`PaneTargetExact`), `:488` (`windowTargetExact`), with
     `exactTargetHelpers` (`target_composition_guard_test.go:100-106`) and `routeItThrough` (`:564`) tracking them.
     No `ExactSessionTarget` / `ExactCoordTarget` spelling survives anywhere in the Go sources or CLAUDE.md.
  4. `exact_target_internal_test.go` → `internal/tmux/exact_target_forms_test.go`, moved from `package tmux` to
     `package tmux_test` and rewritten against the exported names; both original assertions are preserved verbatim in
     substance.
  5. The deferred `type Target string` was recorded at the vocabulary's declaration.
- Two collateral changes the tightening forced, both correct: the same-name collision between
  `cmd.stampPaneToken` and `(*SessionRestorer).stampPaneToken` was resolved by renaming the restore method
  (`internal/restore/session.go:162` `restampPaneToken`, matching its own doc "re-establishes"), and
  `internal/tmux/target_composition_guard_test.go:677-681` `unionPositions` merges the positions of same-named
  declarations rather than overwriting them — the round-1 review finding, fixed.
- Notes: item 5 has since been superseded by the real thing. `internal/tmux/tmux.go:396` now declares
  `type Target string`, all four constructors return it, and every `-t` parameter takes it
  (`ResolveHookKey` at `:232`, `SetPaneOption` at `:314`, `RespawnPane` at `:687`, `CapturePane` at `:731`,
  `NewWindow` at `:757`, `SplitWindow` at `:776`). The doc at `internal/tmux/tmux.go:442-447` now describes the type
  as landed and names the two residual shapes it cannot refuse (untyped constant, explicit conversion). That is the
  decision this task recorded being picked up, which is exactly the outcome its Do list asked for — not drift.
  The `liveTarget` name the old name-keyed allow-list had cost is restored at `internal/restore/session.go:162`.

TESTS:
- Status: Adequate.
- Coverage:
  - AC1 (non-method helper caught). The task's own staged-probe subtest was folded away when the `Target` type
    landed, but the substance is guarded by fixtures that fail if either half of the tightening regresses.
    `target_composition_guard_test.go:303-325` declares `func RespawnPane(target Target, command string)` — a plain
    function — and calls it as a bare identifier `RespawnPane(SaverName, …)`; reinstating the `fn.Recv == nil` skip
    empties `takers` of that entry and the subtest's `want 1` fails, and restoring the `*ast.SelectorExpr`
    requirement in `bareTargetArguments` does the same. `:327-347` repeats the pair over an explicit conversion.
    The real tree carries the live case too: `cmd/hooks.go:108` `stampPaneToken(paneID tmux.Target)` is a non-method
    taker whose call site at `cmd/hooks.go:208` is now checked.
  - AC2 (zero findings on the real tree). Verified by reading rather than running. The scanned set is the twelve
    packages importing `internal/tmux` plus `internal/tmux` itself (confirmed by `go list`; `internal/state` is
    absent, which is why its generic `CapturePane(target)` at `internal/state/scrollback.go:67` is not a false
    positive). Every `"-t"` literal in a non-test source of those packages is followed by a vocabulary call or by an
    identifier the scan binds: `cmd/open.go:95`, `internal/tmux/tmux.go:88,97,236,240,264,272,288,296,315,326,495,552,614,688,732,758,777,792,804,814,827,841`,
    `internal/tmux/clients.go:19`, `internal/tmux/saver_pane_pid.go:13,36`, `internal/restoretest/live_pane_coords.go:34`,
    `internal/tmuxtest/stamp.go:15,28`, `internal/tmuxtest/socket.go:78,128`, and
    `internal/session/quickstart.go:67-68`, whose `target` is bound by the paired-assignment arm through
    `unwrapStringConversion` of `string(tmux.CoordTargetExact(…))` at `quickstart.go:65`. Every call site of a
    target-taking function passes a bound identifier or a vocabulary call; `cmd/hooks.go:208` binds through the
    multi-valued producer arm, since `resolveCurrentPaneKey` (`cmd/hooks.go:67`) declares `tmux.Target` at result
    position 1.
  - AC3 (split-composition fixture). `target_composition_guard_test.go:226-248` stages `[]string{"kill-session", "-t"}`
    and pins the exact detail string, so deleting the `i+1 == len(elems)` branch at `:741-747` drops the count to 0
    and fatals the subtest.
  - AC4 (naming). Pinned structurally rather than by assertion: `exactTargetHelpers` and `routeItThrough` are
    name-keyed, so a helper renamed away from the shape without updating them turns every call site into a finding.
- Notes: no over-testing. The fixtures are one-behaviour-each, assert on exact detail strings, and the two
  "stopped looking" tripwires (`:498-509` empty file set, `:514-532` empty taker set) are distinct conditions rather
  than duplicates. No test execution was attempted, per this reviewer's remit.

CODE QUALITY:
- Project conventions: Followed. The guard is a unit-lane source-scanning test routed through `sourceguardtest`
  (`PackageGoFiles`, `ParsePackageSources`, `ForEachFuncCall`, `CalleeName`) rather than re-authoring the
  enumerate-parse walk, which is the shape CLAUDE.md prescribes for the repo's ~20 source guards. It reports through
  `harnesstest.TestingT` so its own fatal paths are testable. No tmux server, no subprocess beyond `go list`, no
  writes into the repository (`stagePackageWithProbe` copies into `t.TempDir()`).
- SOLID principles: Good. Recognition (`targetTakingFuncs` / `targetReturningFuncs`), binding (`boundTargets`),
  and reporting (`bareTargetsIn` / `bareTargetArguments`) are separate, each with one job.
- Complexity: Low. `unionPositions` is three lines of `slices`; the walk's two passes are documented as to why both
  exist and cannot double-report (`ForEachFuncCall` visits only `*ast.FuncDecl` bodies, and the `ast.Inspect` arm
  guards with `enclosingFunc(...) == nil`).
- Modern idioms: Yes — `slices.Concat`/`Sort`/`Compact`, `strings.SplitSeq`, `sync.OnceValues`, `max`, range-over-int.
- Readability: Good. The doc on `targetTakingFuncs` states the pooling consequence and its remedy ("give the two
  operations distinct names") rather than leaving the reader to discover it, and `enclosingDecl` carries a comment
  distinguishing it from `enclosingFunc`.
- Comment accuracy: The comments the task touched hold against the code as it stands. The `SessionTargetExact` doc
  block (`internal/tmux/tmux.go:442-447`) was rewritten when the type landed, so it no longer describes the type as
  deferred. `internal/restore/session.go:157-161` matches the renamed `restampPaneToken`. CLAUDE.md's `tmux` row
  carries the "the four share one `<kind>TargetExact` shape" clause this task added and has been kept current since.
- Issues: none.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
