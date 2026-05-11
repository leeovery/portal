TASK: killed-sessions-resurrect-on-restart-3-1 — Update buildHydrateCommand snapshot test to assert bare-command shape, then drop the outer `sh -c '…; exec $SHELL'` wrapper in `internal/restore/session.go`

ACCEPTANCE CRITERIA:
- buildHydrateCommand returns bare `portal state hydrate --fifo <fifo> --file <file> --hook-key <hookKey>` string with each value-arg shell-escaped; no sh -c envelope, no `; exec $SHELL` trailer.
- RespawnPane interface signature unchanged.
- Unit/snapshot test updated to assert new bare-command shape.
- Edge cases: paths/hook-keys containing single quotes; empty/unset hook-key value.

STATUS: Complete

SPEC CONTEXT: Spec § Fix 3 → Behaviour (bare form replaces wrapped form). Spec § Why the Outer Wrapper Is Removable (trailer unreachable; comment-stated semantics empirically broken). § Argument Quoting (shell-escape values; interface signature unchanged). Phase 8 task 8-1 later deleted shellQuoteSingle.

IMPLEMENTATION:
- Status: Implemented (post-Phase-8-task-8-1 simplification)
- Location:
  - /Users/leeovery/Code/portal/internal/restore/session.go:431-436 — `buildHydrateCommand` emits bare `portal state hydrate --fifo %s --file %s --hook-key %s`.
  - /Users/leeovery/Code/portal/internal/tmux/tmux.go:577 — RespawnPane(target, command string) signature unchanged.
- Notes: shellQuoteSingle deleted per task 8-1; doc comment honestly captures the apostrophe-input caveat.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/internal/restore/session_build_hydrate_test.go — snapshot + edge-case tests.
  - /Users/leeovery/Code/portal/internal/restore/session_test.go:568-594 — negative-assertion regression guards (no sh -c envelope, no trailer).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low. Simpler than pre-fix.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
