TASK: 2-1 — Introduce tmux.ErrNoSuchSession sentinel and wrap ShowEnvironment at the tmux boundary

STATUS: Complete

SPEC CONTEXT: Component E — typed sentinel introduced once in `internal/tmux/`; daemon layer classifies via `errors.Is`. Substring matching in higher layers rejected.

IMPLEMENTATION:
- Status: Implemented (with documented deviation that strengthens design)
- Location: `internal/tmux/errors.go:35`, `internal/tmuxerr/errors.go:15`, `internal/tmux/errors.go:84-93` (`wrapNoSuchSession`), `internal/tmux/tmux.go:670-681` (`ShowEnvironment` call)
- Notes: Underlying value factored into new leaf package `internal/tmuxerr` to break the cycle internal/state would otherwise close. `internal/tmux` re-exports as `tmux.ErrNoSuchSession` (identity-equal). Documented in `internal/tmuxerr/doc.go`. Architecturally consistent with existing leaf-package precedents (`tmuxout`, `xdg`).

TESTS:
- Status: Adequate
- Coverage: `internal/tmux/errors_test.go` — six focused subtests covering match-on-stderr, empty stderr, non-CommandError, errors.As recoverability, case-sensitive (mixed-case rejection), unrelated non-zero exit
- Not over-tested

CODE QUALITY:
- Project conventions: Followed; multi-`%w`, errors.Is/As, leaf-package cycle-breaking
- SOLID/Complexity: Good; ~10 LOC helper
- Modern idioms: Go 1.20+ multi-`%w`
- Readability: Good; godoc warns downstream against substring matching

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `internal/tmux/errors.go` also declares `ErrEmptyPaneList`/`ErrPanePIDParse` (out of scope for 2-1, added later)
- [idea] Deviation to `internal/tmuxerr` is a meaningful design improvement; consider amending planning doc retrospectively
