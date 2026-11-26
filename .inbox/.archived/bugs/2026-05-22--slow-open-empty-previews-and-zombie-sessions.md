# Slow open, empty previews, and zombie sessions after kill

Three symptoms observed simultaneously on the same install, suspected to share underlying state.

## Symptoms

**`portal open` is slow.** Five-to-eight second delay before the TUI appears. Affects every invocation, not just the first of the day.

**Every session preview is empty.** Highlighting any session in the picker and pressing `Space` shows "no saved content" for every session in the list. Previously, the preview reliably displayed each session's scrollback. Entering a session shows that scrollback is still present in tmux itself — so the issue is specific to Portal's preview path, not to tmux's own buffer.

**Killed sessions resurrect.** Pressing `K` on a session and confirming "yes" removes it from the list, but the next `portal open` shows it back. Same behaviour when killing via `Option-Q` (a user-bound tmux shortcut) from inside the session. Before the killed-session bugfix in v0.5.6, sessions would briefly resurrect within a "tick window" then disappear after roughly five seconds; now they persist indefinitely across multiple `portal open` cycles.

## Conditions

Currently observed under v0.5.6. The empty-preview symptom was already present under v0.5.5 — the upgrade was performed in the hope it would help, but it did not. Local inspection at time of report showed three concurrent `portal state daemon` processes (pids 10745, 32832, 50897) — one started yesterday evening, two this morning. Each held a different inode for `~/.config/portal/state/daemon.lock` (171463046, 171582571, 170216314); `daemon.pid` pointed at 32832. None of the daemon pids matched any live tmux pane.

The `_portal-saver` tmux session did not exist. Bootstrap log contained repeated entries of the form `prior daemon (pid=32832) did not exit within 5s` followed by `another daemon holds the lock; exiting` and `step 4 (EnsureSaver) failed: bootstrap _portal-saver: set destroy-unattached: failed to set session option destroy-unattached on _portal-saver: exit status 1: no such session: _portal-saver`.

The daemon log also contained repeated `tick: capture structure: failed to show environment for session "A": exit status 1: no such session: A` entries, even though session "A" did exist in tmux at inspection time and `tmux show-environment -t A` succeeded when run manually. Session "A" was created today at 10:39, never attached, and carried an `SSH_CONNECTION` environment variable indicating an SSH origin.

Scrollback directory `~/.config/portal/state/scrollback/` contained only one `.bin` file at a time despite `sessions.json` listing 22 sessions. The single file in the directory changed across observations, suggesting churn rather than a stable partial capture.

## Relevant code locations surfaced

- `internal/state/capture.go` — `CaptureStructure` per-session env query loop.
- `cmd/state_daemon.go` — `captureAndCommit` tick driver.
- `internal/tmux/portal_saver.go` — `killSaverAndWaitForDaemon` kill-barrier and `BootstrapPortalSaver`.
- `internal/state/daemon_lock.go` — flock-based singleton primitive.

## Impact

The empty-preview symptom is the most user-visible failure: the entire purpose of the preview is to inspect a session's contents before switching to it. The slow-open delay is painful on every invocation. The kill-not-sticky symptom means dead sessions accumulate indefinitely, polluting the picker and reintroducing the exact problem the kill UI exists to solve.
