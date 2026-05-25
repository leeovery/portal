TASK: 3-1 — Split saver command constants into placeholder and daemon variants

STATUS: Complete

SPEC CONTEXT: Component F mandates decoupling session creation from daemon launch via benign placeholder (`tail -f /dev/null`) so `destroy-unattached=off` applied before real daemon runs. Spec rejects `sleep infinity` (macOS BSD parse error → exits immediately).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/portal_saver.go:30-54` — `portalSaverPlaceholderCommand` with 4-bullet doc-comment (cross-platform indefinite-block, macOS BSD `sleep infinity` rejection, structural incapacity to write state, lifecycle bounded by respawn-pane -k / kill-session)
  - `internal/tmux/portal_saver.go:56-65` — `portalSaverDaemonCommand` with doc-comment
  - `internal/tmux/portal_saver.go:711` — `createPortalSaverWithRetry` passes placeholder
  - `internal/tmux/portal_saver.go:577` — `BootstrapPortalSaver` respawn uses daemon command
  - `internal/tmux/export_test.go:40-52` — test-only re-exports

TESTS:
- Status: Adequate
- `portal_saver_test.go:2625-2646` pins both constants literal-value; comments cite macOS sleep rationale

CODE QUALITY:
- Project conventions: Followed; godoc-style comments; `portal*` naming
- Complexity: Trivial
- Readability: Good — doc-comments explain load-bearing rationale future maintainers might "simplify" away

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan said 3-1 should be strictly constants-only with `createPortalSaverWithRetry` temporarily passing `portalSaverDaemonCommand`, then 3-2 swing to placeholder. Merged tree skips intermediate step; final composed state meets every spec criterion
