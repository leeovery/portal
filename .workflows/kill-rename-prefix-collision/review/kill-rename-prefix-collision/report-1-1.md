# TASK: kill-rename-prefix-collision-1-1 — Introduce exactTarget primitive and fix KillSession to exact-match

STATUS: Complete
FINDINGS_COUNT: 0

## Acceptance Criteria
- [x] `exactTarget(session string) string` exists in `internal/tmux` (sibling to `PaneTargetExact`) and returns `"=" + session`.
- [x] A direct unit assertion proves `exactTarget("foo") == "=foo"`.
- [x] `KillSession("my-session")` issues exactly `kill-session -t =my-session` (verified by updated `TestKillSession` happy path).
- [x] `KillSession` carries a rationale godoc block mirroring the already-fixed sites.
- [x] A new prefix-collision regression test proves a live colliding session (`foo-2`) is never killed when `foo` is not a live exact match.
- [x] `go build -o portal .` and `go test ./internal/tmux/...` are green (run by orchestrator — BUILD OK; tmux package `ok` 11.9s; full `go test ./...` ALL GREEN).

## Implementation
- `internal/tmux/tmux.go:578-595` — `exactTarget` helper placed directly after `PaneTargetExact` (`:562-576`); body `return "=" + session`; full rationale godoc (collision explanation + anti-drift "no inline `=`+name should remain" note).
- `internal/tmux/tmux.go:366` — `KillSession` argv now `c.cmd.Run("kill-session", "-t", exactTarget(name))`. Signature and error-wrapping unchanged.
- `internal/tmux/tmux.go:351-364` — rationale godoc on `KillSession`: `=` exact-match rationale, destructive/no-undo/silent framing, explicit note that the chokepoint covers `_portal-saver` callers harmlessly. Mirrors `HasSession`/`SwitchClient` style.

## Tests — Adequate
- `internal/tmux/exact_target_internal_test.go:9-13` — `TestExactTarget` (internal `package tmux`) asserts `exactTarget("foo") == "=foo"`. Correctly internal (external `tmux_test.go` cannot reach the unexported helper).
- `internal/tmux/tmux_test.go:737` — `TestKillSession` happy path updated to `kill-session -t =my-session`, replacing the buggy bare-`-t` pin.
- `internal/tmux/tmux_test.go:744-753` — error-path subtest unchanged, stays green.
- `internal/tmux/tmux_test.go:756-811` — `TestKillSessionUsesExactMatchPrefix` mirrors `TestHasSessionUsesExactMatchPrefix`. `RunFunc` simulates exact-match semantics (live server holds only `foo-2`): `=foo` errors, bare-`foo` arm calls `t.Errorf` so a dropped prefix fails loudly. Asserts `KillSession("foo")` returns error and recorded `-t` arg begins with `"="`.
- No under-testing or over-testing; no `t.Parallel()` (project constraint).

## Code Quality
Idiomatic Go; Commander/MockCommander DI; chokepoint fix; godoc on both helper and method; `%w` wrapping intact; internal/external test-package split per convention. No issues.

## Blocking Issues
None.

## Non-Blocking Notes
None.
