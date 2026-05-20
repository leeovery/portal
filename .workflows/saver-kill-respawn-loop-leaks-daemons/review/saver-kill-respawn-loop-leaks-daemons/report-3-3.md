TASK: Extract PollUntil Helper To Eliminate Six Near-Identical Polling Loops In Integration Tests

ACCEPTANCE CRITERIA:
- PollUntil returns bool (no t.Fatal inside)
- Each helper preserves external signature + fatal message wording
- skip-on-tmux-absent paths intact

STATUS: Complete

SPEC CONTEXT: Cycle 1 analysis identified six near-identical deadline-based polling loops across the integration test suite. Extraction target is a leaf helper in internal/tmuxtest returning bool so callers retain full control over fatal-message shape and success-side return-value extraction.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmuxtest/poll.go:17 — PollUntil(t, timeout, tick, cond) bool
  - internal/tmux/portal_saver_integration_test.go:522 (waitForDaemonNotAlive), :534 (waitForSessionAbsent), :545 (waitForVersionFile), :632 (waitForLiveDaemon), :657 (waitForNewLiveDaemon)
  - cmd/state_daemon_integration_test.go:572 (waitForDaemonAlive)
- Notes: All six call sites accounted for. Helper is correctly minimal (deadline loop, t.Helper, no t.Fatal). Doc comment explicitly states the no-fatal contract and rationale.

TESTS:
- Status: Adequate
- Coverage: internal/tmuxtest/poll_test.go covers both branches:
  - Success: cond flips true on 3rd call (forces ≥2 ticks, guards against first-iteration false positive), asserts return true + elapsed < timeout
  - Timeout: constant-false cond, asserts return false + elapsed >= timeout
- Notes: Tight and focused. Not over-tested.

CODE QUALITY:
- Project conventions: Followed — leaf helper in internal/tmuxtest (canonical home per CLAUDE.md package table).
- SOLID principles: Good — single responsibility, inverted control via cond callback.
- Complexity: Low — 13-line function, single loop, two exits.
- Modern idioms: Yes — deadline computed once via time.Now().Add; t.Helper() invoked so failures attribute to caller line.
- Readability: Good — doc comment explains both API contract and design rationale.
- Issues: None.

Per-helper preservation verified:
- External signatures unchanged at all six sites
- Fatal-message wording preserved verbatim
- Diagnostic context preserved (portal.log blob in waitForDaemonAlive; state dir in version/pid fatals)
- The two pure-bool helpers (waitForDaemonNotAlive, waitForSessionAbsent) correctly do not call t.Fatal — they propagate the bool to cascade-test callers that own the failure shape
- t.Helper() preserved at each migrated helper site

skip-on-tmux-absent paths intact: tmuxtest.SkipIfNoTmux(t) verified at all integration test entrypoints. PollUntil does not touch tmux, so skip wiring is orthogonal.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
