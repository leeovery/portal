# Discovery Session 001

Date: 2026-06-04
Work unit: cleanup-purge-test-no-state-isolation

## Description (as of session)

Apply temp-dir PORTAL_STATE_DIR isolation to all subtests of
TestStateUserFacingSubcommandsExitZero so the cleanup and --purge cases stop
running against the developer's real state dir.

## Seed

- seeds/2026-06-02-cleanup-purge-test-no-state-isolation.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Originated from an inbox bug: `TestStateUserFacingSubcommandsExitZero` in
`cmd/state_test.go` applies `PORTAL_STATE_DIR=t.TempDir()` isolation only when
`tt.args[1] == "status"` (line 232). The two `cleanup` table cases ("cleanup
with no flags", "cleanup with --purge") therefore resolve the state directory
via the normal XDG path and run against the developer's real
`~/.config/portal/state`. The `--purge` case calls `os.RemoveAll` on the
resolved dir, so it both flakes (intermittent failure when a live daemon is
writing) and risks deleting live persisted state — the same test-isolation-gap
class CLAUDE.md warns about.

Shaping settled on quick-fix rather than bugfix: the root cause is already
pinpointed and the resolution is mechanical (lift the temp-dir env out of the
`status`-only conditional so it applies to all three subtests). No behaviour to
debate, nothing to diagnose — a bugfix investigation phase would only re-derive
what the report already nailed.

One scoping nuance surfaced (for the scoping phase, not resolved here): the
report suggests "ideally adopt `portaltest.IsolateStateForTest`", but that
helper returns an `env []string` built for subprocess tests
(`cmd.Env = env`), whereas this test runs in-process via `rootCmd.Execute()`
with `t.Setenv`. The simple in-process fix (unconditional `t.Setenv`) is the
natural fit; whether to do more is a scoping call.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to scoping.
