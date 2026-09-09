TASK: resume-hooks-silently-lost-8-18 — "The list-panes Live-Coord Read Is Written Nine Ways, Six Of Them Prefix-Unsafe" (tick-0dceb4). Collapse the nine hand-authored `list-panes -s -F "#{window_index}:#{pane_index}"` reads in the restore corpus onto one `restoretest` reader that pins its target exactly.

ACCEPTANCE CRITERIA:
- One reader performs the live-coord enumeration; no site composes the format string itself.
- Every consumer's target is prefix-safe.
- Each site's assertions are unchanged.
- The integration lane passes.

STATUS: complete

SPEC CONTEXT: Phase 8 is an implementation-analysis cycle, so the task's own body is its authority rather than the specification. It sits downstream of the work unit's session-target pinning theme: tmux prefix-matches a bare `-t <session>`, so a read against a session that is gone can be answered by a live sibling whose name merely extends it (CLAUDE.md's `internal/tmux` row records `CoordTargetExact`/`=name:` as the pinned form and why the trailing colon is load-bearing). The six unpinned test reads were assertions whose whole job is to prove restore placed panes where it claimed, which is exactly the assertion a stranger's session must not be able to satisfy.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - New reader: `internal/restoretest/live_pane_coords.go:14` (the single `livePaneCoordFormat` declaration), `:18` `LivePaneCoords` (fatal), `:33` `TryLivePaneCoords` (error-returning), running `list-panes -s -t <string(tmux.CoordTargetExact(session))> -F <format>` at `:34`.
  - Re-pointed sites (commit 2f7fd4b5, nine call sites): `cmd/bootstrap/reboot_roundtrip_test.go:253`, `:264`, `:546`; `internal/restore/integration_full_test.go:215`; `internal/restore/armed_restore_integration_test.go:140`; `internal/restore/exit_closes_pane_integration_test.go:174`; `internal/restore/prefix_sibling_integration_test.go:68`; plus the two former in-package readers (`multipane_legacy_integration_test.go`'s `assertLivePanes` and `rename_reboot_shared_test.go`), whose assertions a later task (8-32, `73bcfd0c`) folded into `internal/restore/reboot_fixture_test.go:110`.
- Notes:
  - Criterion 1 holds at HEAD: a repo-wide grep for `#{window_index}:#{pane_index}` returns exactly three hits — the reader, production `internal/tmux/tmux.go:495` (`ListPanesInSession`), and `internal/tmux/tmux_test.go:2080`, which asserts that production argv. No test composes the read any more.
  - Criterion 2 holds: every consumer reaches tmux only through `TryLivePaneCoords`, whose sole target expression is `tmux.CoordTargetExact(session)`.
  - Criterion 3 holds in substance. The comparison shape necessarily changed with the reader's return type: sites that ran `strings.Contains(rawOutput, "w:p")` now run `slices.Contains(coords, "w:p")`. That is strictly stricter (substring matching would have accepted `"0:0"` inside `"10:0"`), so no assertion was weakened, and each site's subject, wants and failure messages are otherwise untouched. `internal/restore/exit_closes_pane_integration_test.go:174-186` keeps its early-return-on-error and its budgeted poll; the pin additionally makes a prefix-sibling answer impossible there, which is the correct reading of "the pane is gone".
  - Lane placement is right: `live_pane_coords.go` carries `//go:build integration` and every consumer file is integration-tagged, matching the rule stated in `internal/restoretest/doc.go` (a helper carries the tag only when every caller does).
  - The reader survives the standing target guard: `internal/tmux/target_composition_guard_test.go` scans the non-test sources of every package importing `internal/tmux` (which includes `internal/restoretest`), and `string(<exact helper>(...))` after `-t` is the sanctioned shape its own fixture at `:436` pins as passing.

TESTS:
- Status: Adequate
- Coverage: `internal/restoretest/live_pane_coords_test.go:14` carries exactly the two cases the task named. `"it reads the coords of a session's live panes"` (`:15`) builds a two-window/three-pane session and pins the full ordered result `"0:0 0:1 1:0"`, so both the enumeration and the doc's "in tmux's own order" claim are observed. `"it does not read a prefix sibling's panes"` (`:30`) creates only `sib-2` and reads `sib`: under the unpinned form tmux would prefix-match and return the sibling's panes at exit 0, so the assertion that the read errors is a genuine guard on the pin — remove `CoordTargetExact` and this test fails. It also asserts `coords == nil` alongside the error, pinning the reader's two-shape return.
- Notes: Not over-tested — two cases, no redundant assertions, no case that would survive the behaviour being broken. The nine re-pointed sites keep their own coverage; as a pure read-substitution they need none of their own.

CODE QUALITY:
- Project conventions: Followed. Test-only helper in `internal/restoretest` (production must not import it); real tmux confined to a per-test `tmuxtest` socket, never the ambient server; no `t.Parallel()`; the fatal/try pair mirrors the `Run`/`TryRun` split the `tmuxtest.Socket` already uses.
- SOLID principles: Good. One reason to change (how a session's live pane coords are read); the fatal wrapper is a one-line composition of the error-returning form, so there is one implementation of the read.
- Complexity: Low — two functions, no branching beyond the error path.
- Modern idioms: Yes. `slices.Contains` replaces substring matching at the call sites; `%w` wrapping preserves the underlying exec error while carrying the session name and trimmed output.
- Readability: Good. The doc comment on `TryLivePaneCoords:27-32` states why the target is pinned (the prefix-sibling trap) rather than restating the code, and both comments hold true against the implementation.
- Issues: None that clear the bar. `internal/restore/prefix_sibling_integration_test.go:66` retains a package-local `livePaneCoords` string-joining wrapper over the shared reader, and the reader's own fixture at `live_pane_coords_test.go:20` composes the literal target `"alpha:0.0"` for a `split-window` rather than `tmux.PaneTarget` — both are preferences on a fresh per-test socket where no sibling exists, neither is a defect, and neither is reported as a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
