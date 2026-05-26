# Deterministic mid-tick gate for SIGHUP integration test

In `cmd/state_daemon_integration_test.go`, `TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow` uses a static `tickStartDelay = 1.2 * time.Second` (against the daemon's 1s tick interval) to schedule SIGHUP "mid-tick". The worst-case timing analysis in the test's inline comment (~lines 319-339) is thorough, but on a slow CI cold-start the SIGHUP can still land in the idle outer `select` arm rather than mid-tick — flipping the test from "exercises ctx.Done() inside captureAndCommit" to "exercises the outer select", which is a different code path.

Replace the static sleep with a deterministic gate that fires SIGHUP only once the daemon has demonstrably entered a tick. Candidate gates (design call):
- Poll for the first per-pane scrollback file appearing in stateDir (proves `captureAndCommit` has reached the per-pane loop).
- Poll for an instrumentation hook the daemon could emit (would require adding a test-only seam).
- Poll for `daemon.pid`'s mtime changing post-startup (weaker — only proves the tick wrote something).

Closes a real CI-flake risk and tightens what the test actually exercises.

Source: review of saver-kill-respawn-loop-leaks-daemons (Task 2-5 non-blocking note #11).
