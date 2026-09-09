TASK: resume-hooks-silently-lost-7-32 — Claims The Rewrite Left Behind — Stale Prose And An Unreachable Error Promise (tick-c5daef, severity: comments)

ACCEPTANCE CRITERIA:
- No test name or failure message in `cmd` names `ListAllPanes`.
- Every renamed assertion asserts exactly what it asserted before.
- `ActivePaneCurrentPath`'s doc describes the empty-and-nil return and makes no unreachable `ErrNoSuchSession` promise.
- The lazy dir-resolution fallback treats an empty path with a nil error as unresolved, and that is covered by a test.
- A genuine `display-message` command failure still returns a wrapped error.

STATUS: complete

SPEC CONTEXT: Phase 7 is an implementation-analysis cycle, so the task's own body is its authority rather than the specification. The only spec touchpoint is the 2026-09-01 corrigendum recording that `ListAllPanes` (and `ResolveStructuralKey`, `ListPanes`) were deleted during this work unit once the hook paths that called them were replaced — which is exactly the deletion that left the eight `cmd` test names naming a method that no longer exists. The `ActivePaneCurrentPath` half is outside the spec's subject entirely: it is the TUI's lazy dir-resolution fallback, reached through `internal/session`'s `PaneCurrentPathReader` seam.

IMPLEMENTATION:
- Status: Implemented (with one considered divergence from the task's wording, judged sound)
- Location: commit f2578916. `internal/tmux/tmux.go:247-268` (doc rewritten, `wrapNoSuchSession` call dropped, outer wrap kept); `internal/session/dirresolve.go:21-43` (contract doc corrected); `cmd/state_daemon_hook_cleanup_test.go:169`, `cmd/state_daemon_hook_cleanup_integration_test.go:219` (renamed messages); five further renames landed in `cmd/run_hook_stale_cleanup_test.go`, a file later phases deleted when the sweep moved into `internal/hooksweep`.
- Notes:
  - AC1 verified tree-wide, not just in `cmd`: a repo-wide grep for `ListAllPanes` excluding `ListAllPanesWithFormat` and `ListAllPaneHookKeys` returns nothing in any `.go` file. The only surviving occurrences are in `CHANGELOG.md:296` and prior work units' `.workflows/` artifacts — historical records of a release, correctly untouched.
  - Divergence: the task said rename all eight onto `ListAllPanesWithFormat`; the implementer split them by the seam each test actually drives. Verified accurate in both directions — `ListAllPaneHookKeys` delegates to `ListAllPanesWithFormat` (`internal/tmux/tmux.go:658-664`), so `cmd/state_daemon_hook_cleanup_test.go:169` naming `ListAllPanesWithFormat` for an error injected at the `Commander` (`daemonFakeCommander{panesErr}`) names where the error originates, and `cmd/state_daemon_hook_cleanup_integration_test.go:219` naming `ListAllPaneHookKeys` for "the live pane set" names the enumeration whose result the retention assertion is about. Both hold; not a loss.
  - AC3: the doc now states the `("", nil)` return, names the empty path as the unresolvable signal, and says outright that no `ErrNoSuchSession` can be produced on this path. `wrapNoSuchSession` is not left dead — it is still reached through `wrapSessionTargetErr` (`internal/tmux/errors.go:125`).
  - AC5: `internal/tmux/tmux.go:266` keeps the outer `fmt.Errorf(... %w)`, so a genuine failure still names the session and preserves the `*CommandError`.
  - AC4 verified at both levels of the caller rather than assumed: `internal/session/dirresolve.go:41-43` returns `("", false, nil)` for a blank path before `ResolveGitRoot` can `os.Stat("")`, and the sole production consumer `internal/tui/model.go:1163` gates on `ok && err == nil`, so an empty-and-nil read caches nothing and the session falls to the Unknown catch-all.
  - The central factual claim is independently corroborated against real tmux, not just asserted: `internal/tmux/exact_session_target_realtmux_test.go:82-95` drives `ActivePaneCurrentPath` against a gone session on a real per-test socket and asserts an empty path with a nil error.

TESTS:
- Status: Adequate
- Coverage: All three named tests exist and assert their subject. `internal/tmux/tmux_test.go:2758` ("it returns empty and nil for a session no pane answers to"), `:2774` ("it returns a wrapped error when display-message itself fails" — asserts the session name is in the message and the `*CommandError` is recoverable), `internal/session/dirresolve_test.go:119` ("treats an empty path with a nil error as unresolved" — also pins that the git-root runner is never reached), `internal/tui/rebuild_dir_resolution_test.go:147` ("it treats an empty current path as unresolved in the grouped render" — CatchAll, the `Unknown` heading, and nothing cached into `m.sessions`).
- Notes:
  - The inverted subtest at `internal/tmux/tmux_test.go:2798` ("it makes no ErrNoSuchSession promise, even for stderr saying so") is a deliberate negative pin against reinstating the dead classification, and is not redundant with `:2774` — different stderr, opposite assertion. Reading it, a reinstated `wrapNoSuchSession` on this path would fail it, so the removal is pinned rather than merely done.
  - `internal/tui/rebuild_dir_resolution_test.go:160-165` strengthened the pre-existing assertion (added the `GroupHeading == unknownHeading` check) rather than only renaming it — no assertion was weakened anywhere in the change.
  - Not over-tested: five subtests across `TestActivePaneCurrentPath` cover argv shape, the miss, a genuine failure, the negative classification pin and a non-`*CommandError` failure; each fails for a distinct reason.
  - The five renamed subtests in `cmd/run_hook_stale_cleanup_test.go` no longer exist — later phases deleted that file with the sweep's move to `internal/hooksweep`. That is superseding work, not a regression: the acceptance criterion is a tree-wide absence and it holds.

CODE QUALITY:
- Project conventions: Followed. No production behaviour moved outside the one wrap change; no lane rule touched (every changed test stays in the lane it was already in); no logging vocabulary touched.
- SOLID principles: Good. `internal/session/dirresolve.go:38-39`'s added comment correctly frames the classification as the seam's contract rather than tmux's, which is what keeps the interface honest for a non-tmux reader.
- Complexity: Low — the production delta is one removed call in a three-line method body.
- Modern idioms: Yes.
- Readability: Good. The rewritten doc states the return, the reason, the caller's handling and what a non-nil error then means, in that order.
- Issues: One stale cross-reference in the rewritten doc (below). It was accurate when written — it named `ExactSessionTarget`, whose doc then carried the sentence — and was re-pointed at `CoordTargetExact` by the later target-vocabulary rework (a0cbed5d, task 9-2), which had already dropped the sentence from both sibling docs.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] internal/tmux/tmux.go:252 — the parenthetical "the same fact CoordTargetExact's doc states from the target-form side" cites a statement that doc does not make: `CoordTargetExact`'s doc (internal/tmux/tmux.go:452-467) mentions `display-message` only inside its list of commands taking that target form, and says nothing about an empty expansion at exit 0; `SessionTargetExact`'s doc (internal/tmux/tmux.go:421-448), which held the sentence when this doc was written, no longer does either — grepping `display-message` across internal/tmux/tmux.go returns lines 219, 240, 251, 260, 264 and 457, and only 251 states the fact. Drop the parenthetical, or point it at the real-tmux case that proves it (internal/tmux/exact_session_target_realtmux_test.go:82-95, which asserts the empty path and nil error against a gone session). — FAILS: a reader checking the measurement follows the citation to a doc that does not contain it, and the single doc now recording the fact carries no marker that another doc leans on it, so the next rewrite of either can silently strand the other again — the exact failure mode this task was raised to remove.
