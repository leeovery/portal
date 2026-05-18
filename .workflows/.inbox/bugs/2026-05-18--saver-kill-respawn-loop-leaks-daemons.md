# Saver kill-respawn loop leaks daemons and pads every bootstrap by ~520ms

Every `portal` invocation that runs the bootstrap orchestrator is paying ~520ms for a useless kill+respawn cycle on the `_portal-saver` session, and leaking an orphan `portal state daemon` process each time. On this machine three orphan daemons are currently alive (PIDs 34503 from Saturday, 59610, 57161 — all parented to the same tmux server, PID 94966), and by the end of bootstrap there is no live saver session at all, which means saves are silently paused between portal invocations and the next run repeats the cycle. The visible user-facing symptom is portal startup hovering around 3–5 seconds and `portal.log` filling with three repeating WARN lines per run.

The chain of causes — all in the saver-bootstrap path, all fixable together:

**1. `portalSaverVersionMismatch` in `internal/tmux/portal_saver.go` returns `true` on any read error from `daemon.version`, including the "file absent" case.** Whenever `daemon.version` is missing, every bootstrap therefore fires the kill barrier in `EnsurePortalSaverVersion`, even when the running daemon's binary is identical to the invoking binary. Versions match — `portal version` is `0.5.0` and `daemon.version` was `0.5.0` when present — but the file keeps disappearing.

**2. The daemon doesn't exit within the 5s kill-barrier window.** `cmd/state_daemon.go:306` registers SIGHUP/SIGTERM via `signal.Notify`, but the capture loop appears to block on long per-pane work without checking the signal channel mid-cycle. `killSaverAndWaitForDaemon` times out and logs `prior daemon (pid=X) did not exit within 5s`, then proceeds anyway.

**3. The newly-spawned daemon can't acquire `daemon.lock`** (the orphan still holds it) and exits with `another daemon holds the lock; exiting`. That makes its pane process exit, which destroys the just-created `_portal-saver` session, which makes the immediately-following `SetSessionOption(_portal-saver, destroy-unattached, off)` fail with `no such session: _portal-saver` — surfaced as the `step 4 (EnsureSaver) failed` warning in `portal.log`.

Evidence captured this session: `portal.log` contained the three WARN lines verbatim; `ps` showed three orphan daemons with the same tmux-server PPID; `lsof` confirmed only the most-recent daemon held `daemon.lock`; a tmux-call trace showed `has-session ×2 + new-session + set-option` adding ~520ms even with the tmux server already running.

Open sub-question to investigate alongside the fix: **why does `daemon.version` keep disappearing?** It was present as `0.5.0` at the start of this session and gone by the end — the whole state dir (`daemon.lock` / `daemon.pid` / `daemon.version`) got wiped during the investigation, and it's not yet clear which code path nuked them. Candidates include `portal clean`, an atomic-write race in `state.WriteVersionFile`, or something else entirely. The deleter needs to be identified before settling on the shape of fix #1.

Likely single fix scope: `internal/tmux/portal_saver.go` (mismatch classification) and `cmd/state_daemon.go` (signal-responsive capture loop). Fixing #1 alone would eliminate the unnecessary kill cycle in the common case; fixing #2 closes the orphan leak in the legitimate kill case (real version upgrade). They're complementary and should ship together.
