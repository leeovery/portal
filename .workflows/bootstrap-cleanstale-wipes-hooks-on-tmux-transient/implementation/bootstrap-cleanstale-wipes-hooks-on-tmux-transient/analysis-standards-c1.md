AGENT: standards
FINDINGS:

- FINDING: Stale comparative comment in ListAllPanesWithFormat docstring
  SEVERITY: low
  FILES: internal/tmux/tmux.go:657-658
  DESCRIPTION: The docstring on `ListAllPanesWithFormat` still reads "Unlike `ListAllPanes`, this method propagates the underlying error so callers can distinguish 'no panes' from 'tmux failed'." Post-fix, `ListAllPanes` also propagates errors (it is now a thin wrapper around `ListAllPanesWithFormat` per spec Change 1). The "Unlike ListAllPanes" framing is now misleading — both helpers share the propagating contract; the difference is format-string flexibility, not error policy. Spec Change 1 directed rewriting the `ListAllPanes` docstring (done correctly at tmux.go:683-700); the peer rewrite is implied by the contract collapse but was not explicitly listed.
  RECOMMENDATION: Rewrite the second sentence on `ListAllPanesWithFormat` to describe what genuinely differentiates the two helpers post-fix — `ListAllPanesWithFormat` exposes raw tmux output for callers that need a non-default format string and leaves parsing to the caller; `ListAllPanes` is the structural-key-format convenience wrapper. Drop the "Unlike ListAllPanes" comparison since the error contract is no longer divergent.

SUMMARY: Implementation matches spec verbatim across all four changes — log line shapes are byte-identical between `cmd/bootstrap_production.go` and `cmd/clean.go`, `state.ComponentBootstrap` used uniformly, mutual-exclusivity of terminal lines asserted in `cmd/bootstrap_production_test.go`, hazard guard / `persisted==0` early-exit branches preserved per spec Change 4's special case, all six acceptance criteria covered, integration tests use `//go:build integration` plus `portaltest.IsolateStateForTest`, no `t.Parallel()` in cmd-package tests, and the `*Deps` mock-injection pattern (`CleanDeps`) preserved. One low-severity stale-docstring nit on the peer helper implied by the spec's contract collapse.
