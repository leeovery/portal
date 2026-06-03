# Discovery Session 001

Date: 2026-06-03
Work unit: harden-daemon-integration-test

## Description (as of session)

Harden cmd/state_daemon_integration_test.go against silent passes: export tmux.KillBarrierTimeoutCeiling and reuse it instead of a local mirror, and swap a t.Logf warning for t.Skip when a fast host cannot exercise the mid-tick SIGHUP cancellation path.

## Seed

- seeds/2026-05-20-export-kill-barrier-timeout-ceiling.md (inbox:quickfix)
- seeds/2026-05-20-skip-instead-of-warn-on-fast-capture-pane-host.md (inbox:quickfix)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Two inbox quick-fixes, both originating from the review of saver-kill-respawn-loop-leaks-daemons (Task 2-5 non-blocking notes #12 and #13), were promoted together and bundled into a single quick-fix. Both target the same file, `cmd/state_daemon_integration_test.go`, and share one intent: stop the integration test from silently passing under conditions it is meant to guard.

First change: the test currently mirrors the production `killBarrierTimeout` (5s) as a local `killBarrierTimeoutCeiling` constant for its "exit within 5s" regression assertion. A future drop in the production value would silently desync the mirror. Export the constant from `internal/tmux` (e.g. `tmux.KillBarrierTimeoutCeiling`) and import it in the test, closing the desync gap with zero behaviour change.

Second change: `TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow` derives an anchor threshold from measured `capture-pane` wall time; on a fast host the aggregate falls below 2s, and the test emits a `t.Logf` warning and proceeds — passing trivially without exercising the "tick spans the kill barrier" cancellation path. Swap the warning for `t.Skip(...)` so the test explicitly skips rather than degrading to a no-op pass.

The user confirmed quick-fix shape (small, mechanical, test-only edits, no behaviour debate, nothing to diagnose) and elected to run both as one bundled quick-fix.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
