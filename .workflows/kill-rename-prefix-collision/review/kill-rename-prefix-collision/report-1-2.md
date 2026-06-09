# TASK: kill-rename-prefix-collision-1-2 — Fix RenameSession to exact-match target with bare newName

STATUS: Complete
FINDINGS_COUNT: 0 blocking (1 non-blocking godoc-wording nit)

## Acceptance Criteria
- [x] `RenameSession("old-name", "new-name")` issues exactly `rename-session -t =old-name new-name` (verified by updated `TestRenameSession` happy path).
- [x] The `=` prefix appears on the `-t` target only; `newName` remains bare — verified by argv inspection.
- [x] `RenameSession` carries a rationale godoc block mirroring the already-fixed sites, explicitly noting the target-only / bare-`newName` trap.
- [x] A new prefix-collision regression test proves a live colliding session (`foo-2`) is never renamed when `foo` is not a live exact match.
- [x] `go build -o portal .` and `go test ./internal/tmux/...` are green (run by orchestrator — BUILD OK; tmux `ok`; full `go test ./...` ALL GREEN).

## Implementation
- `internal/tmux/tmux.go:390` — `c.cmd.Run("rename-session", "-t", exactTarget(oldName), newName)`. `=` on target only, `newName` bare. Error-wrapping and signature unchanged.
- `internal/tmux/tmux.go:373-389` — godoc mirrors `KillSession` and explicitly states the implementer trap (`=` on TARGET ONLY; `newName` must stay bare or the session is literally named `=...`).
- Shared helper `exactTarget` at `tmux.go:593-595`.

## Tests — Adequate
- Happy path: `TestRenameSession` "runs rename-session with old and new name" (`tmux_test.go:996-1015`) asserts joined argv `rename-session -t =old-name new-name`.
- Error path: `tmux_test.go:1017-1026` unchanged, green.
- Regression + bare-`newName` guard: `TestRenameSessionUsesExactMatchPrefix` (`tmux_test.go:1046-1096`). Live server holds only `foo-2`; `=foo` errors and the bare-`foo` arm `t.Fatalf`s if reached. Asserts recorded argv equals `["rename-session", "-t", "=foo", "bar"]` AND a dedicated `strings.HasPrefix(got[3], "=")==false` check — the genuine bare-`newName` guard, asserted two complementary ways.
- `MockCommander.Run` appends to `m.Calls` before delegating to `RunFunc`, so post-error argv inspection is valid. Not over-tested.

## Code Quality
Followed conventions; helper centralisation matches anti-drift intent; single argv chokepoint; `fmt.Errorf`/`%w`; trap loudly documented in source and test. No `t.Parallel()`.

## Blocking Issues
None.

## Non-Blocking Notes
- [nit] `internal/tmux/tmux.go:378-379` — the godoc parenthetical example reads slightly muddled vs. the clearer `KillSession` example. Pure comment edit, zero logic impact. Optional polish.
