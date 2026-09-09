TASK: resume-hooks-silently-lost-7-7 — "Bare -t Targets Are Composed Outside The tmux Client, With Nothing Enforcing The Rule" (tick-af68d2, phase 7, severity medium)

ACCEPTANCE CRITERIA:
- `internal/session/quickstart.go` composes no unprefixed `-t` target; a `new-session` that fails no longer lets the chained `set-option` land on a prefix sibling.
- `internal/restore/session.go` composes no unprefixed `<session>:` target; splits and new windows still land in the restored session's active window.
- The source guard passes over the three packages after the two fixes and fails when a bare `-t` composition is reintroduced in any of them.
- The guard fatals rather than passing when it enumerates no files.
- Quickstart still stamps `@portal-dir` before attaching, and restore still reconstructs multi-window/multi-pane skeletons identically.

STATUS: complete

SPEC CONTEXT: This is a phase-7 task (implementation-analysis cycle), so its authority is its own body rather than the specification — the spec's body and its ten corrigenda say nothing about tmux target exactness. The surrounding project standard is CLAUDE.md's `tmux` architecture row, which states the rule this task enforces: every per-session `-t` is pinned to an exact-match target because tmux prefix-matches, the helper is chosen by measurement per command, and the vocabulary is `SessionTargetExact` / `CoordTargetExact` / `PaneTargetExact` / `windowTargetExact` / `PaneIDTarget` returning the named `Target` type, with `internal/tmux/target_composition_guard_test.go` surviving as the reduced scan over the residue the type cannot express.

IMPLEMENTATION:
- Status: Implemented (and legitimately moved past the task's wording by later phases — judged against the current tree)
- Location:
  - `internal/session/quickstart.go:65-69` — both chained steps take one pinned target, `target := string(tmux.CoordTargetExact(prepared.SessionName))`, spent as a string at the exec-chain argv the client never runs. The task's Do list expected two *different* helpers (`ExactCoordTarget` for `set-option`, `ExactSessionTarget` for `attach-session`), which is what commit 6895b659 delivered; phase 9's re-measurement (a0cbed5d, "every per-session target takes the form tmux resolves") moved `attach-session` onto `CoordTargetExact` too, because a bare `=name` is parsed as a window/pane first. That divergence is a measured improvement, is recorded in CLAUDE.md's tmux row, and is covered by a real-tmux measurement (`internal/tmux/period_session_target_realtmux_test.go:146` runs `attach-session -t` with `CoordTargetExact`) — not a loss.
  - `internal/restore/session.go:89` — `target := tmux.CoordTargetExact(sess.Name)`, fed to `SplitWindow`/`NewWindow` at :93, :101, :106. The trailing-colon "session's active window" semantics the old comment relied on are preserved and the replacement comment at :85-88 states them accurately.
  - `internal/restore/session.go:140` — the coordination point named in the task's own fix-tracking round 1 was taken: `liveTarget := tmux.PaneTargetExact(...)`, consumed by `SetPaneOption` (:166) and `RespawnPane` (:149).
  - `internal/tmux/portal_saver.go:360` — the third site the guard surfaced (the saver's own respawn spending a bare session name as a pane target) is pinned through `CoordTargetExact(PortalSaverName)`.
  - `internal/tmux/target_composition_guard_test.go` — the standing guard. `TestTmuxTargetsAreComposedThroughTheExactnessVocabulary` (:125) runs `scanBareTargets` (:573) over the package set derived at :39-94 from `go list` (every package importing `internal/tmux`, plus `internal/tmux` itself), which is a superset of the three packages the task named and covers `internal/session` and `internal/restore` by construction.
- Notes:
  - The guard implements both halves the task asked for and one more: the argv rule (`bareTargetsIn`, :735 — a literal `"-t"` whose next element is neither a vocabulary call nor an identifier bound to one, plus a `"-t"` that ends its argv) and the argument rule (`bareTargetArguments`, :762 — an argument reaching a parameter declared with the `Target` type that the vocabulary did not produce). The second rule is what closes the blind spot the task's own fix-tracking round 1 verified: the restore site composes its target in one package and spends it behind a `-t` in another.
  - I enumerated every literal `"-t"` in the repository's non-test `.go` files (34 sites). Every one is either a vocabulary call, a `string(...)` of one, or a parameter/named result declared `tmux.Target` (`cmd/hooks.go:49,67,108`; `internal/tmuxtest/socket.go:76`, `stamp.go:13,26`; `internal/restore/session.go:162`). I also enumerated every production call to a `Target`-taking function (`ResolveHookKey`, `SetPaneOption`, `RespawnPane`, `CapturePane`, `NewWindow`, `SplitWindow`, `restampPaneToken`) — all pass an argument the vocabulary produced or a bound `Target`. The scan therefore has no offender to report and no false positive to absorb over the current tree.
  - `internal/session` gained no new package edge: `internal/session/dirresolve.go:9` already imported `internal/tmux`.

TESTS:
- Status: Adequate
- Coverage: every test the task named exists, and each fails if its half breaks.
  - `internal/session/quickstart_exact_target_test.go:18` / `:24` — the two quickstart pins, asserted against the literal `"=" + name + ":"` so a silent helper change is caught; `:33` pins the argv to the typed constructor rather than to a second spelling of the prefix.
  - `internal/session/quickstart_prefix_sibling_realtmux_test.go:22` — the mid-chain miss, run against a real per-test `-L` socket holding only the prefix sibling, with a control at :43-50 proving a bare target *would* have reached the stranger (so a target form tmux merely refuses cannot read as a pass). `:81` adds the live half that fix-round 3 required: both composed steps still resolve the live session, which matters because a wrong form on `set-option` would abort the `;` chain before `attach-session` runs.
  - `internal/restore/session_exact_target_test.go:16` — the split and new-window targets, asserted for every such call.
  - `internal/restore/prefix_sibling_integration_test.go:19` — the multi-window reconstruction with a prefix sibling live, asserting both the restored coords (`0:0 0:1 1:0`) and that the sibling gained nothing. Correctly `//go:build integration`: it builds a portal binary via `restoretest.BuildPortalBinaryDir`.
  - `internal/tmux/target_composition_guard_test.go:198` — a bare `-t` fails the scan (four distinct shapes in one fixture: a package-level composite literal, a local assigned bare, an `fmt.Sprintf`, and a `+ ":"` inside an argv slice), with further subtests for a `-t` ending its argv (:226), a hand-composed local (:250), a concatenation inside the tmux package itself (:278), and the two shapes the `Target` type admits — an untyped constant (:303) and an explicit conversion (:327). Pass cases at :349, :417, :440, :458 pin that the vocabulary's own output, a `Target`-typed parameter whatever it is named, and a target spent as a string on an argv the client does not run are all clean.
  - `internal/tmux/target_composition_guard_test.go:498` — the enumerates-no-files fatal, driven through `harnesstest.Recorder` and asserting the message says the directory held no sources. `:488` and `:514` add the two sibling stopped-looking tripwires (an import scan resolving no importer, a package declaring no target-taking function).
  - `:143` stages a probe into a copy of the real `cmd` package's sources and asserts exactly one finding across the whole derived set — so the rule is exercised against real sources, not only fixtures, and the repository staying clean is part of the assertion.
- Notes: the fixture arithmetic holds — I counted the expected findings in each fixture by hand against the rules and they match the asserted counts (4, 1, 1, 1, 1, 1, 1, and 0 for each pass case). No over-testing: each subtest exercises a distinct rule branch, and the two pass cases that look alike differ in mechanism (a string-returning helper versus a `Target`-typed parameter).

CODE QUALITY:
- Project conventions: Followed. The unit-lane real-tmux test uses a per-test `tmuxtest` `-L` socket with no daemon and no built binary, so it sits correctly outside the integration tag under CLAUDE.md's three-clause lane rule; the binary-building restore test carries the tag. The guard is built on `sourceguardtest` primitives (`PackageGoFiles`, `ParsePackageSources`, `ForEachFuncCall`, `CalleeName`) as the task's Do item 4 required, and reports through `harnesstest.TestingT` so its own fatal path is testable.
- SOLID principles: Good. The exactness rule stays in `internal/tmux`; the two call sites consume it rather than re-deriving the `=` prefix, which was the explicit instruction.
- Complexity: Acceptable. `scanBareTargets` is decomposed into one small function per rule (`bareTargetsIn`, `bareTargetArguments`, `boundTargets`, `bindAssignedTargets`, `bindMultiValuedTargets`), each with a stated reason for its shape.
- Modern idioms: Yes — `slices.Sorted(maps.Values(...))`, `strings.SplitSeq`, `sync.OnceValues` for the once-per-binary `go list`, `max`/`min` builtins.
- Readability: Good. The doc comments carry the measurement rather than the mechanics — `internal/session/quickstart.go:43-52` states why both steps take the coordinate form and why the guarantee may not rest on tmux abandoning a `;` chain, and `internal/restore/session.go:85-88` states what the empty coordinate half resolves to. I checked both against the code they sit on and against the vocabulary's own docs; neither makes a claim the code falsifies.
- Issues: none.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
