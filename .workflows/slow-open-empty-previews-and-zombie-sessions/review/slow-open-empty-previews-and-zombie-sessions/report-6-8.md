TASK: 6-8 — Assert portaltest cleanup fingerprint backstop reports clean on test exit

STATUS: Issues Found (non-blocking)

SPEC CONTEXT: Spec § Composite final acceptance — test uses `portaltest.IsolateStateForTest`; no developer-state mutations on test exit. Spec § Component G: backstop verifying dev `~/.config/portal/state/` untouched.

IMPLEMENTATION:
- Status: Implemented with deliberate simplifications vs plan's Do list
- Location:
  - `cmd/bootstrap/composition_e2e_fingerprint_backstop_integration_test.go` — dedicated backstop test `TestCompositeBootstrap_FingerprintBackstopRunsClean` (87-114)
  - `cmd/bootstrap/composition_e2e_harness_integration_test.go:143-244` — `setupCompositeHarness` step ordering (step 1 staging, step 2 `IsolateStateForTest` at 166, then steps 3-7 register subsequent cleanups)
  - `internal/portaltest/isolated_env.go:99-106` + `installBackstopCleanup` (122-130) — backstop wiring registered inside `IsolateStateForTest` so its `t.Cleanup` runs LAST per LIFO
- Plan's Do list specifies modifications to composite test body (meta-assertions, positive-control logging, three-causes preamble); implementation creates separate dedicated test consuming existing harness — defensible interpretation but diverges from literal Do
- Dedicated test does NOT include positive-control `t.Logf` of isolated stateDir contents
- Preamble documents host-noise + LIFO ordering thoroughly but does NOT enumerate three failure causes

TESTS:
- Status: Adequate (with caveats)
- `TestCompositeBootstrap_FingerprintBackstopRunsClean` is canonical assertion (silent backstop == pass)
- Helper wiring independently exercised by `installBackstopCleanup` meta-tests at `internal/portaltest/fingerprint_test.go:523-590` via `fakeBackstopT`
- Plan named five test cases under "Tests"; only first materially exists; audit-style tests upheld by structural review
- Audit-grep style criteria upheld:
  - `exec.Command` in `cmd/bootstrap/composition_e2e_*` returns one match (`ps` in `f_observables`) — not portal spawn
  - All portal spawns via `portaltest.SpawnIsolatedDaemon` (applies isolated env + per-call PORTAL_STATE_DIR)
  - Single in-test `state.AcquireDaemonLock` (`composition_e2e_fresh_acquire_integration_test.go:166`) takes `h.StateDir` from harness — isolated

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; narrow `backstopT` interface (`isolated_env.go:117-120`) demonstrates ISP/DIP
- Complexity: Low
- Readability: Good; dense but consistent rationale
- Literal "positive control" guidance dropped — adding `t.Logf` over `filepath.Walk(h.StateDir)` would cost ~10 LOC

BLOCKING ISSUES:
- None — structural contract satisfied

NON-BLOCKING NOTES:
- [quickfix] Add positive-control `t.Logf` walking `h.StateDir` reporting file count + total bytes before return
- [quickfix] Extend preamble to enumerate three documented failure causes: (i) subprocess inherited dev XDG_CONFIG_HOME; (ii) direct file write bypassed env; (iii) helper snapshot semantics changed
- [idea] Plan named five test cases; only one implemented; consider splitting audit-grep assertions into compile-time structural tests visible in `go test -v`
- [idea] Composite-package meta-test "deliberate write fails with diagnostic" exists only at internal/portaltest level via `fakeBackstopT`; thin composite-package wrapper writing sentinel file into devStateDir would close gap
