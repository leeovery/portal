STATUS: findings
FINDINGS_COUNT: 1

AGENT: duplication

FINDINGS:

- FINDING: Eight near-identical seam-install helpers in portal_saver_test.go share an identical save-install-restore skeleton
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go:964-970 (installBarrierReadPID), :973-979 (installBarrierIsAlive), :982-988 (installBarrierPollInterval), :991-997 (installBarrierTimeout), :1000-1006 (installBarrierLogger), :1322-1328 (installKillSaverFn), :1655-1661 (installReadVersionFile), :2083-2089 (installWriteVersionFile)
  DESCRIPTION: Eight test helpers all follow the exact same four-line body — `seam := tmux.<Name>Seam(); prev := *seam; *seam = fn; t.Cleanup(func() { *seam = prev })`. The only variation is the seam-getter call and the parameter type. Pattern repeats 8 times across ~50 lines. A Go-1.18+ generic helper `swapSeam[T any](t *testing.T, ptr *T, v T)` collapses each body to one line. Borderline value: low LOC savings (~16 lines), but the rote pattern is a maintenance burden — a future seam will likely produce a ninth copy.
  RECOMMENDATION: Optional. Introduce `func swapSeam[T any](t *testing.T, ptr *T, v T) { t.Helper(); prev := *ptr; *ptr = v; t.Cleanup(func() { *ptr = prev }) }`. Each install* helper collapses to a single-line wrapper. Alternatively, drop the install* wrappers entirely and inline `swapSeam(t, tmux.<X>Seam(), <fn>)` at call sites. Low priority.

SUMMARY: Cycle-1 findings 1-5 all resolved. One new low-severity pattern: eight `install*` seam helpers share an identical 4-line body that could collapse to a single generic helper. Modest savings.
