# Redundant `tmux show-hooks -g` calls in bootstrap hook registration burn ~1.5s per portal startup

Every portal invocation that runs the bootstrap orchestrator pays ~1.5s of wall time on step 2 (`RegisterPortalHooks`) due to redundant `tmux show-hooks -g` dumps. The step is functionally correct — hooks are installed idempotently — but each `show-hooks -g` is a separate tmux subprocess fork+exec (30–100ms each), and the registrar issues ten of them sequentially during a single bootstrap.

Captured during the saver-kill-respawn-loop-leaks-daemons investigation on 2026-05-18: a PATH shim wrapping `tmux` produced this trace for one `portal hooks list` run, with timestamps relative to the start of bootstrap:

```
0.05s show-hooks -g
0.29s show-hooks -g          ← Go work in between, then re-query
0.32s show-hooks -g
0.36s show-hooks -g
0.71s show-hooks -g
0.74s show-hooks -g
0.77s show-hooks -g
0.84s set-hook -ga window-layout-changed ...
1.48s show-hooks -g          ← re-query AFTER set-hook
1.51s set-hook -ga pane-focus-out ...
1.90s show-hooks -g
1.94s show-hooks -g
```

The pattern suggests the registrar re-queries the global hook table before AND after each `set-hook` insertion, instead of reading the full table once and diffing locally against the desired hook set. With ~7 portal hooks to install, this multiplies into ten subprocess invocations on the steady-state path.

Source location: bootstrap step 2 is composed in `cmd/bootstrap/` and calls into `internal/tmux/hooks_register.go` — the registrar's idempotency loop is the culprit. Production wiring lives in `internal/bootstrapadapter`.

Net effect on the user:

- ~1.5s of every steady-state bootstrap is spent dumping the same global hook table over and over.
- `RegisterPortalHooks` runs on every portal invocation that triggers bootstrap (`portal open`, `portal x`, `portal hooks list`, etc.), so the cost amortises poorly — it's paid on every TUI launch, every direct attach, every status query.
- The symptom is purely performance. Correctness is unaffected; the hooks are installed correctly, just expensively. No portal.log noise, no orphan state, no user-visible warning — just a quietly slow startup.

This is orthogonal to the saver-kill-respawn-loop bug logged the same day, which contributes a separate ~520ms via a different mechanism (`internal/tmux/portal_saver.go` + `cmd/state_daemon.go`). The two bugs are independently scoped and should ship as independent fixes; the combined steady-state bootstrap would drop from ~3.2s to ~1.2s.

There's adjacent opportunity worth flagging during scoping: `CleanStaleMarkers`, `SweepOrphanFIFOs`, and `CleanStale` each independently invoke `tmux show-options -s` (~128ms each, ~400ms total) for similar "enumerate and diff" reasons. That's a separate bug at the same architectural pattern — caching the post-Restore server-option dump across the cleanup steps would knock another ~300ms off. Mentioning it here so a future scoping conversation has the breadcrumb; not part of this bug's scope.

Parent investigation referenced for trace context: `.workflows/saver-kill-respawn-loop-leaks-daemons/investigation/saver-kill-respawn-loop-leaks-daemons.md`.
