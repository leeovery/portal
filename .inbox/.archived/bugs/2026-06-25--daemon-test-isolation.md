# Daemon-spawning integration test bypasses IsolateStateForTest

`cmd/state_daemon_integration_test.go:178-179,254-256` isolates via raw `t.TempDir()` + `t.Setenv("PORTAL_STATE_DIR")` + `daemon.Env = append(os.Environ(), "PORTAL_STATE_DIR="+stateDir)` rather than `portaltest.IsolateStateForTest(t)`.

State writes are redirected to the tempdir, so the dev install is not directly corrupted — but this bypasses BOTH safety nets CLAUDE.md mandates for daemon-spawning tests:
- the **`XDG_CONFIG_HOME` scrub** — a daemon code path that reads `XDG_CONFIG_HOME` (rather than `PORTAL_STATE_DIR`) could still touch the developer's real `~/.config/portal/`; and
- the **fingerprint-diff cleanup backstop** that walks the dev state dir post-test and fails on any delta.

This is the exact discipline the slow-open / empty-previews / zombie-session incident motivated (the canonical example of a leaked test daemon corrupting the live install). The test is pre-existing — NOT introduced by task 5-8 — and out of 5-8's "updated for concurrent boot" scope, but 5-8's own discipline criterion says "every daemon-spawning test," which is why it surfaced here.

`reattach_integration_test.go` uses the same raw-env pattern but does not spawn `portal state daemon` directly (its saver step is a NoOp), so the risk there is lower; worth a glance while in the area.

Fix: route the daemon-spawning test through `portaltest.IsolateStateForTest(t)` (and prefer `portaltest.SpawnIsolatedDaemon` / `RegisterSubprocessCleanup` per CLAUDE.md), applying the returned env to every spawned subprocess.

Source: review of spectrum-tui-design/spectrum-tui-design (report 5-8).
