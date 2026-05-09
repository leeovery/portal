TASK: Consolidate duplicated daemon-tick test helpers in cmd/bootstrap (3-1)

ACCEPTANCE CRITERIA:
- One source of truth for daemon-tick simulation.
- Helper accepts skipSet and useEmptyScrollback knobs.
- Both former call sites preserve original semantics.
- Helper gated `//go:build integration`.

STATUS: Complete

SPEC CONTEXT: Cycle-1 duplication finding #1: two near-identical helpers (`captureAndCommit` in reboot_roundtrip_test.go and `runDaemonTick` in scrollback_resumption_test.go) drove a daemon-tick-equivalent capture-and-commit, differing in skip-save guard and CaptureAndHashPane vs empty bytes. Recommendation: consolidate.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/daemon_tick_test_helpers_test.go:1-149` (new shared helper, build tag at first line).
  - `cmd/bootstrap/reboot_roundtrip_test.go:273, 917, 1140` (was `captureAndCommit`; now uses `WithoutSkipGuard()` + `WithEmptyScrollback()`).
  - `cmd/bootstrap/scrollback_resumption_test.go:147, 240, 334` (production-shape default).
- Notes:
  - Helper mirrors production `cmd/state_daemon.go captureAndCommit` (lines 115-158) faithfully.
  - Functional-options pattern (`WithoutSkipGuard`, `WithEmptyScrollback`) — defaults match production daemon.
  - Both former functions fully removed.
  - PrevIndex=nil correctly noted in docstring.

TESTS:
- Status: Adequate (helper itself is test code; coverage validated by 6 integration call sites).
- Coverage: All 6 call sites preserve original test intent.
- Notes: No new direct unit tests for the helper itself; appropriate for a thin `t.Helper()`.

CODE QUALITY:
- Project conventions: Followed. Functional options idiom; `t.Helper()` used.
- SOLID: Good. Single responsibility.
- Complexity: Low.
- Modern idioms: Good — functional options, struct value defaults.
- Readability: Excellent. Long preamble docstring (lines 1-27) anchors helper to production invariants.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Plan task wording says "skipSet and useEmptyScrollback knobs" but implementation correctly fetches skipSet internally and exposes `skipGuard` boolean. Plan wording could be tightened.
- [idea] Helper exports `DaemonTickOption` (capitalised) but in `package bootstrap_test`; effectively unused outside package. Could be lowercase.
- [quickfix] reboot_roundtrip_test.go preamble at lines 509-515 references helper behaviour twice; could be trimmed.
