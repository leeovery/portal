# Standards Findings — killed-sessions-resurrect-on-restart (cycle 1)

```
AGENT: standards
FINDINGS:
- FINDING: Stale `sh -c` wrapper documentation in integration test comments contradicts Fix 3
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:117-118, /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:158-159, /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:73-74
  DESCRIPTION: Three doc comments still describe the helper invocation as `respawn-pane -k 'sh -c portal state hydrate ...; exec $SHELL'` — the exact shape Fix 3 (Wrapper Drop in `buildHydrateCommand`) deliberately removed. The implementation in `internal/restore/session.go:426-433` correctly emits the bare `portal state hydrate ...` form, and the regression guards (`internal/restore/session_build_hydrate_test.go` and `internal/restore/exit_closes_pane_integration_test.go`) explicitly forbid `sh -c` envelope re-introduction. These stale comments mis-describe live behaviour to future readers, directly contradicting spec § Fix 3 (Defect-D Closure). Cosmetic — the test assertions themselves are correct — but the spec calls the wrapper-drop out as load-bearing for AC5, so the divergence in three places near the test entry-points creates a misleading paper trail.
  RECOMMENDATION: Replace the three quoted strings with the post-Fix-3 bare form: `respawn-pane -k 'portal state hydrate --fifo X --file Y --hook-key Z'`. Drop the `; exec $SHELL` trailer reference — post-Fix-3 the helper owns the exec via `syscall.Exec`, so referencing the trailer in current-state docs perpetuates the same confusion.

SUMMARY: Implementation conforms to spec on all three Fix sites and the eight acceptance criteria. The new EagerSignalHydrate step lands at the spec-mandated ordinal (after Restore, before Clear `@portal-restoring`); the seam interface, soft-warning posture, FIFOPath derivation source, and shared `internal/state.WriteFIFOSignal` primitive all match the spec's "Sharing mechanism" / "Failure Posture" / "Pane Enumeration and FIFO Resolution" subsections. `handleHydrateTimeout` unsets the marker via the canonical `unsetSkeletonMarkerOrLog` wrapper and routes to `execShellOrHookAndExit` symmetrically with `handleHydrateFileMissing`. `buildHydrateCommand` emits the bare form with no `sh -c` envelope and the unit test asserts the negative directly. Only standards drift found is three stale doc comments in integration test files that still describe the dropped wrapper shape.
```
