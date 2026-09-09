TASK: resume-hooks-silently-lost-8-38 — "ResolveSessionDir Classifies Sentinels Its Only Production Implementation Cannot Emit, For A Caller That Discards The Error" (tick-9b6352, severity: dead-code)

ACCEPTANCE CRITERIA:
- The seam has one degradation path.
- No test asserts `ErrNoSuchSession` or `ErrEmptyPaneList` on this path.
- The TUI's lazy dir-resolution fallback behaves identically — a non-ok outcome degrades the same way it does today.
- `internal/tmux`'s sentinels are untouched; only this seam's classification changes.

STATUS: complete

SPEC CONTEXT: The specification does not mention `ResolveSessionDir`, `dirresolve` or `ActivePaneCurrentPath` (grepped — zero hits), which is expected: this is a phase-8 implementation-analysis task whose authority is its own body, per the shared verifier context. The surrounding fact it depends on is established in `internal/tmux/tmux.go:250-262`: `ActivePaneCurrentPath` answers an unmatched `display-message` target with an empty expansion at exit 0, so no `ErrNoSuchSession` can arise on that path — pinned by `internal/tmux/tmux_test.go:2798` ("it makes no ErrNoSuchSession promise, even for stderr saying so").

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/session/dirresolve.go:34-53` (function), `internal/session/dirresolve.go:21-33` (rewritten doc); commit `8298beb8`, touching only `internal/session/dirresolve.go` and `internal/session/dirresolve_test.go`.
- Notes:
  - The `errors.Is(err, tmux.ErrNoSuchSession) || errors.Is(err, tmux.ErrEmptyPaneList)` branch is gone; a reader error now unconditionally returns `("", false, fmt.Errorf(...))` at `internal/session/dirresolve.go:36-38`. The now-unused `errors` import was dropped; the `tmux` import remains live via the compile-time assertion at `internal/session/dirresolve.go:19`.
  - Criterion 1 (one degradation path) holds for the reader outcome: the empty-path guard at `internal/session/dirresolve.go:41-43` is the sole silent degradation of a *read*. The `ResolveGitRoot` failure at `internal/session/dirresolve.go:47-50` also returns `("", false, nil)`, but that is a pre-existing, separately-documented stage (`internal/session/dirresolve.go:45-46`) outside this task's Do list.
  - Criterion 4 holds: `ErrNoSuchSession` / `ErrEmptyPaneList` are declared in `internal/tmuxerr/errors.go:8` and `:14` and still consumed by `internal/state/capture.go:71`, `cmd/uninstall.go:95`, `cmd/state_daemon.go:325` and `internal/tmux/saver_pane_pid.go:55` — untouched by this commit.
  - The seam's only production implementation is `*tmux.Client` (`internal/tmux/tmux.go:263`), wired at `cmd/open.go:641` (`dirReader: client`) → `cmd/open.go:532` → `internal/tui/build.go:29`. A repo-wide grep for `PaneCurrentPathReader` / `ActivePaneCurrentPath` finds no second production implementer, so the derivation the task rests on holds as written.
  - Criterion 3 holds and the consumer was left untouched: `internal/tui/model.go:1163` still reads `ok && err == nil`. Before the change a sentinel-bearing reader error gave `(false, nil)` and was skipped; it now gives `(false, err)` and is skipped by the same condition — and no production reader could produce the sentinel anyway, so the live path is bit-identical.

TESTS:
- Status: Adequate
- Coverage: `internal/session/dirresolve_test.go:46-191` — six subtests: happy-path canonical git root, exactly-one-active-pane-read, reader failure (re-aimed, `:92-117`), empty-path-with-nil-error (`:119-141`), canonical-key match against stored `Project.Path`, and non-repo cwd fallback. The re-aimed failure subtest asserts `err != nil`, `errors.Is(err, readFailed)`, that the message names the session, `ok == false`, empty dir, and that the git runner was never called.
- Notes:
  - Criterion 2 verified by grep: no occurrence of `ErrNoSuchSession` or `ErrEmptyPaneList` anywhere under `internal/session` or `internal/tui`.
  - The removal is guarded rather than merely untested: reintroducing the branch would make `internal/session/dirresolve_test.go:99-101` fail on `err == nil`.
  - The TUI grouped-render fallback tests are unchanged, as the task's Tests note required — `internal/tui/rebuild_dir_resolution_test.go:147-170` still drives the empty-and-nil reader into the Unknown catch-all, and the commit touched no file under `internal/tui`.
  - Not over-tested: no subtest duplicates another's subject, and none asserts on the deleted classification.

CODE QUALITY:
- Project conventions: Followed. Unit-lane test, no `t.Parallel()`, no new logging or tmux surface, no build-tag implications.
- SOLID principles: Good — the seam's contract narrows to what its single production implementation can emit and its single consumer can read.
- Complexity: Low — one branch removed, nothing added.
- Modern idioms: Yes — `fmt.Errorf` with `%w` preserving the underlying reader error for `errors.Is`.
- Readability: Good. The rewritten doc at `internal/session/dirresolve.go:21-33` states the production shape (empty expansion at exit 0) and what a non-nil error now means, and the two in-function comments (`:40`, `:45-46`) each name the reason for their guard.
- Issues: None rising above preference. The doc's "an empty path is the whole signal" is bounded by the preceding "Both arrive the same way" to the two causes it enumerates; the third `(false, nil)` route (`ResolveGitRoot` failing when `paneCwd` is gone from disk) is named at its own site four lines below, so nothing in the prose is falsified by the code in a way that could mislead an action.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
