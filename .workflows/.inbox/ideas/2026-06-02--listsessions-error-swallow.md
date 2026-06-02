# Harden the `ListSessions` error swallow (discriminate server-absent from other failures)

`internal/tmux/tmux.go` `ListSessions` collapses *all* `list-sessions` errors to an empty-slice success. The intent (documented in the task 4-6 comment) is "no tmux server running is a valid zero-sessions state," but a non-server error (malformed `-F` format string, tmux binary crash) is also silently swallowed to zero sessions — an observability blind spot in exactly the kind of external boundary this feature exists to illuminate.

Recommendation: discriminate `ErrNoSuchSession`/server-absent from other failures per Boundary class 2, returning/logging the genuine error case while preserving the "no server → empty" behaviour.

Caveat (from review): this carries behavioural risk — the "no server → empty list" path is load-bearing in bootstrap and the TUI picker, so the change needs its own focused work with tests to avoid regressing that contract. It is also somewhat orthogonal to the logging layer (a tmux-boundary behaviour fix). Worth a tracked follow-up, not urgent.

Source: review of portal-observability-layer/portal-observability-layer
