# TASK: kill-rename-prefix-collision-1-3 — Migrate five behaviour-neutral session-target sites onto exactTarget

STATUS: Complete
FINDINGS_COUNT: 0

## Acceptance Criteria
- [x] HasSession, HasSessionProbe, SwitchClient (tmux.go) and saverPanePID, SaverPaneID (saver_pane_pid.go) build their `-t` target via `exactTarget(...)`.
- [x] Argv byte-identical (no test assertions changed; existing pins stay green).
- [x] No inline `"="+name` session-target strings remain in tmux.go or saver_pane_pid.go (only the spec-allowed window-level `SelectWindow` construction remains).
- [x] Out-of-scope sites untouched.
- [x] `go build -o portal .` and `go test ./...` are green (run by orchestrator — BUILD OK; full suite ALL GREEN).

## Implementation — Implemented, byte-identical
- `internal/tmux/tmux.go:136` — HasSession → `exactTarget(name)`
- `internal/tmux/tmux.go:166` — HasSessionProbe → `exactTarget(name)`
- `internal/tmux/tmux.go:406` — SwitchClient → `exactTarget(name)`
- `internal/tmux/saver_pane_pid.go:49` — saverPanePID → `exactTarget(sessionName)`
- `internal/tmux/saver_pane_pid.go:84` — SaverPaneID → `exactTarget(sessionName)`
- Helper at `tmux.go:593` (`return "=" + session`) is the single sanctioned session-level construction.

## Grep Audit (re-run by orchestrator)
Across internal/tmux the only non-comment `"=" +` constructions are `tmux.go:594` (the `exactTarget` helper body, sanctioned) and `tmux.go:983` (`SelectWindow` window-level `"=" + bareTarget`, allowed by spec). All other `"="` matches are godoc comments. `saver_pane_pid.go` has zero inline session-target constructions.

## Out-of-scope sites verified untouched/bare
`display-message -t <paneID>`, `set-option -t session`, `list-panes -s -t session` reads, `show-environment`/`set-environment -t session`, `PaneTarget` (no-prefix hooks key), `PaneTargetExact` (pane-level), `SelectWindow` (window-level) — all left as-is per spec.

## Tests — Adequate (correctly no new tests; neutrality proven by existing pins)
- `tmux_test.go:397` `has-session -t =my-session`; `:453` `TestHasSessionUsesExactMatchPrefix`; `:533` HasSessionProbe `has-session -t =my-session`; `:827` `TestSwitchClient` `switch-client -t =my-session`; saver tests `list-panes -t =_portal-saver ...`; `exact_target_internal_test.go:10` helper assertion. Every migrated site has a pre-existing argv pin that fails loudly on a dropped prefix. Not over-tested.

## Code Quality
gofmt-shaped; godoc on exported fns; `%w` retained. `exactTarget` is the single canonical session-target constructor — removes per-site duplication and closes the drift surface that allowed the original bug. Pure substitution, no control-flow change.

## Blocking Issues
None.

## Non-Blocking Notes
None.
