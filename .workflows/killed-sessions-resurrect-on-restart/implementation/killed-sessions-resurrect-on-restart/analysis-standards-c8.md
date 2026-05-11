# Standards Findings — killed-sessions-resurrect-on-restart (cycle 8)

```
AGENT: standards
FINDINGS: none

SUMMARY: Cycle 7's two cleanup tasks (T10-1 gofmt drift fix at internal/restore/session.go:425 docstring; T10-2 final OpenTestLogger migration at internal/restore/exit_closes_pane_integration_test.go:375) both landed cleanly. Repository now passes go build ./..., go vet ./..., and golangci-lint run ./... with zero issues; gofmt -l . reports only the five pre-existing baseline files (internal/browser/listing.go, internal/resolver/{gitroot,query,zoxide}_test.go, internal/session/create.go) — all out of scope for this work unit, none introduced by it.

Spec conformance verified end-to-end:
- Bootstrap step ordering matches spec § Bootstrap Step Numbering Update (EagerSignalHydrate at step 6, between Restore and Clear @portal-restoring).
- CLAUDE.md "Server bootstrap" section synced to the new 10-step list.
- Production wiring in cmd/bootstrap_production.go uses state.DefaultFIFOSignaler for the shared internal/state primitive.
- Eager-signal log shape matches spec § Failure Posture (`WARN | hydrate | eager-signal: write fifo <path>: <err>`).
- Failure posture is swallow-and-warn, never escalates to a *FatalError.
- handleHydrateTimeout calls unsetSkeletonMarkerOrLog and routes the fall-through through execShellOrHookAndExit per spec § Fix 2.
- buildHydrateCommand emits the bare `portal state hydrate ...` form with no `sh -c` envelope per spec § Fix 3.
- ComponentHydrate constant resolves to "hydrate" matching the spec's log-format string.
- No t.Parallel violations introduced.

The cycle-7 standards finding is now closed.
```

STATUS: clean
