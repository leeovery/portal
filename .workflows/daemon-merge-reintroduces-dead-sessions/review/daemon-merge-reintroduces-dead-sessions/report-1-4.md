TASK: Add empirical-scenario regression test (kill-mid-flight self-heal) (1-4)

ACCEPTANCE CRITERIA:
- Regression test mirrors empirical scenario; seeds `prev.Sessions` so it would fail on buggy code rather than false-greening via the `prev != nil` gate.
- `sessions.json` self-heals on the next daemon tick after a previously-polluted commit.

STATUS: Complete

SPEC CONTEXT: Spec's "Empirical Confirmation" cites three live-in-the-wild resurrected sessions (e.g. `agentic-workflows-XXrJ3J`) whose paneKeys had matching stale `@portal-skeleton-*` markers. AC #1 calls out that prev-population is load-bearing. AC #3 requires self-heal on the next tick.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/capture_test.go:1076-1160` — subtest `kill_mid_flight_self_heal`.
- Notes: Test seeds `prev` with the killed session (`agentic-workflows-XXrJ3J`, window 1, pane 1) — same identifier from the empirical-confirmation log. Skip set carries the killed paneKey throughout. Tick 1 asserts merge does not reintroduce killed. Tick 2 threads the just-returned (clean) `idx` back as `prev` with the same `skipSet`, mirroring `cmd/state_daemon.go:156`, asserting the killed session stays absent.

TESTS:
- Status: Adequate
- Coverage:
  - Empirical-scenario fidelity: identifier matches spec.
  - Load-bearing precondition: prev seeded directly with the killed session.
  - Marker stays in `skipSet` throughout both ticks.
  - Two-tick self-heal verified.
  - Survivor invariant ruling out trivial pass via blanket filtering.
- Notes: Well-scoped, no over-testing. Comments document why prev-population is load-bearing.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Idiomatic Go test conventions.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Subtest name `kill_mid_flight_self_heal` (snake_case) diverges from sentence-style subtest names elsewhere.
- [idea] Tick 2 reuses the same mock; a one-line comment confirming "tmux state unchanged across ticks; only prev differs" would help future readers.
