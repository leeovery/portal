# Distinguish Transport Errors From Missing Option in GetServerOption

`GetServerOption` in `internal/tmux/tmux.go` maps all Commander errors to the sentinel `ErrOptionNotFound`. This means a genuine transport or connectivity failure (tmux server crashed mid-call, permission error, etc.) is indistinguishable from the option simply not existing.

In the hook executor's two-condition check, this conflation means a transient tmux error could be misread as "marker absent" and trigger hook re-execution when it shouldn't. In practice this is low risk — tmux commands are local and fast, and the worst case is a harmless re-execution of a restart command — but it's architecturally imprecise.

A possible improvement would be to inspect the Commander error output for tmux's specific "unknown option" or "invalid option" message and only return `ErrOptionNotFound` for that case, propagating other errors as-is. The executor could then decide whether an unexpected error should be treated as "marker absent" (current behavior) or "skip this pane" (safer behavior). This keeps the sentinel error meaningful while giving callers the information they need to make the right call.

Relevant file: `internal/tmux/tmux.go`, `GetServerOption` method.
