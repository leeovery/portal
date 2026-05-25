TASK: 6-2 — Assert pre-fix three-daemon state reproduces under harness

STATUS: Complete

SPEC CONTEXT: Spec § Composite step 3 — pre-fix state reproduces: pgrep returns 3, scrollback oscillates 0–1 .bin file across ticks. Locks broken baseline for 6-3..6-7 convergence claims.

IMPLEMENTATION:
- Status: Implemented (with minor plan drift; spec acceptance fully met)
- Location: `cmd/bootstrap/composition_e2e_prefix_integration_test.go` (203 lines)
- `TestCompositeHarness_PreFixDysfunctionReproduces` (64-106); helpers `sampleScrollbackBinCounts`, `countBinFiles`, `assertScrollbackOscillation`, `dirNames`
- Constants: `prefixOscillationSampleCount = 4` (47), `prefixOscillationSampleInterval = 900ms` (54)
- Three pre-fix observations:
  1. `len(pids) == 3` via `portaltest.PgrepPortalDaemons()` snapshot with rich diagnostic
  2. Explicit `len(pids) == 1` guard against false-positive converged-healthy collapse
  3. Scrollback oscillation via 4 samples at 900ms
- Two-layer pgrep check: harness `waitForPgrepCount(t, 3, ...)` + test-owned snapshot
- Pre-fix assertion runs BEFORE bootstrap (file contains no `Orchestrator.Run`)

TESTS:
- Status: Adequate
- One test function covering all three pre-fix observations (plan listed 4 names, collapsed legitimately)
- Failure diagnostics: PID-by-PID alive status, sample counts, dir listing on "no activity" branch
- No over-testing

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; `t.Helper`
- SOLID: Good; each helper single-responsibility
- Complexity: Low; cyclomatic ≤3
- Readability: Excellent; file-header explains contract, variance rationale
- Diagnostics: Strong

Plan drift (non-blocking):
- Plan asked `TickerPeriod + 200ms ~ 3.6s` window; impl uses 900ms × 3 ≈ 2.7s; comments still say "~3.6s"
- Plan asked path-keyed fingerprint map from 4-2; impl uses simpler `.bin` file count (closer fit to spec "oscillates 0–1 `.bin` file")
- Plan asked `requirePreFixState(...)` named helper; impl inlines (body is short)
- Plan asked literal per-PID-presence membership; impl asserts `len == 3` and surfaces PIDs in diagnostic

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] File header and constant doc describe "~3.6s"; actual is ~2.7s — bump interval or correct comments
- [quickfix] `dirNames` line 199 — `filepath.Base(e.Name())` redundant; replace with `e.Name()`
- [idea] Literal per-PID membership loop would harden against unexpected external daemons
- [idea] Extracted helper would aid reuse if 6-3..6-7 ever re-run mid-test
