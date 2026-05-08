TASK: session-scrollback-preview-1-4 — Tail-N performance benchmark

ACCEPTANCE CRITERIA:
- go test -bench=BenchmarkTailScrollback ./internal/state/ runs and reports per-op times.
- b.ResetTimer() invoked after fixture generation.
- TestTailScrollback_PerformanceBudget passes with elapsed < 5 ms after warmup.
- Regression-guard test skips cleanly when PORTAL_SKIP_PERF is set.
- Fixture content includes ANSI escapes and varied line widths.

STATUS: Complete

SPEC CONTEXT:
Spec § History Depth > Read Pipeline > Performance budget pins tail-N read p99 < 5 ms on a 4 MB .bin (warmed-cache, busy-session worst case). Build phase ships the benchmark; if a future change pushes p99 above the budget, the synchronous-in-Update decision must be revisited.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/scrollback_tail_bench_test.go (single file holds benchmark, regression-guard test, and shared buildPerfFixture helper).
- Notes:
  - BenchmarkTailScrollback (line 114): buildPerfFixture runs first, b.ResetTimer() at line 117 AFTER fixture write, then b.N loop calls state.TailScrollback(path, 1000). Result consumed via _ = got (line 125) to defeat dead-code elimination.
  - TestTailScrollback_PerformanceBudget (line 142): checks PORTAL_SKIP_PERF opt-out (143), builds fixture, performs warmup call (151), then measures a second call with time.Now()/time.Since and asserts < 5*time.Millisecond against named perfBudget constant (26).
  - Fixture (buildPerfFixture, line 46): 50_000 lines × ~80 bytes (~4 MB), seeded rand.NewSource(42) for determinism, ±20% width jitter, ANSI SGR escapes (\x1b[3{1-6}m...\x1b[0m) every 3-5 lines from a 5-colour palette.
  - Helper signature buildPerfFixture(tb testing.TB, dir string) reuses across *testing.B and *testing.T cleanly.

TESTS:
- Status: Adequate
- Coverage:
  - Benchmark exists, name matches plan suggestion.
  - Regression-guard test asserts elapsed < 5 ms with env-var opt-out.
  - Warmup-then-measure sequence (151-157) makes disk cache a controlled variable.
  - Sanity check len(got) == 0 (161) prevents false-negative perf passes on degenerate result.
- Notes: Not over-tested. "Benchmark excludes fixture generation" verified by inspection.

CODE QUALITY:
- Project conventions: Followed. External test package state_test. t.TempDir()/b.TempDir() for isolation. No t.Parallel(). Fixture mode 0o600.
- SOLID: Good. buildPerfFixture single-purpose; benchmark and regression-guard separated.
- Complexity: Low. Fixture builder is one loop; benchmark and test are linear.
- Modern idioms: Good. bytes.Buffer.Grow pre-sizes, deterministic seed, testing.TB for shared helper, named constants.
- Readability: Good. Inline comments explain "why" at every decision (deterministic seed, jitter rationale, ANSI sprinkle interval, DCE prevention, warmup discipline). Spec citations at load-bearing points.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Regression-guard test's pass/fail rests on a single sample; budget has ample headroom so flake risk is low. Could measure several iterations and assert on a percentile.
- [idea] math/rand v1 is fine; Go 1.22+ math/rand/v2 would be marginally more idiomatic. Mechanical only.
- [quickfix] for i := 0; i < b.N; i++ could use Go 1.22+ integer-range form. Stylistic.
- [idea] Benchmark exercises only N=1000. Sub-benchmarks at N=100 / N=10_000 would catch chunk-boundary regressions a single N cannot. Out of scope — spec pins N=1000 / 4 MB as the audit anchor.
