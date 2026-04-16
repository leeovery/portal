---
type: research
status: complete
thread: detached-session-host-verification
---

# Deep Dive: Detached Session as Background Process Host — Verification

## Brief

Verify the behavioral specifics of using a detached tmux session (`tmux new-session -d -s NAME "COMMAND"`) as a host for a long-running Portal save process. Five specific questions about session lifecycle, signal propagation, command interaction, tmux 3.5+ primitives, and real-world experience from tmux-slay.

## Key Findings

### Q1: Session Lifecycle When Command Exits

**The session dies when the command exits.** This is the default tmux behavior. From the man page: "When the shell command completes, the window closes." When the last window in a session closes, the session is destroyed. If `exit-empty` is on (default), the server exits when there are no sessions left.

Chain of events for `tmux new-session -d -s _portal-saver "portal save-state --periodic"`:
1. Command exits (clean or error) -> window closes
2. That was the only window -> session destroyed
3. If no other sessions exist and `exit-empty` is on (default) -> server exits

**`remain-on-exit` interaction:** This is a *window/pane option*, not a session option. When set, the pane stays visible (showing exit status) instead of closing when the command exits. The window survives, therefore the session survives. Set it with `tmux set-option -t _portal-saver remain-on-exit on` after creating the session, or inline: `tmux new-session -d -s _portal-saver \; set remain-on-exit on \; send-keys "portal save-state --periodic" Enter`. However, the pane becomes a dead pane — it cannot be reused without `respawn-pane`.

**`destroy-unattached` interaction:** This is a *session option*. Default is `off`. From tmux source (`server-fn.c`, `server_check_unattached()`), the option has four values: `off` (0), `on` (1), `keep-last` (2), `keep-group` (3). With `off` (default), a detached session persists indefinitely. With `on`, a session is destroyed the moment all clients detach — but a session created with `-d` *never had* a client attached, so `destroy-unattached` should not apply at creation time (the check runs in `server_check_unattached()` which iterates sessions with `s->attached != 0` as a skip condition — detached sessions with 0 attached clients *would* match, meaning `destroy-unattached on` would immediately kill a `-d` session).

**Recommendation for Portal's use case ("die so Portal can recreate"):** Leave `remain-on-exit` at default (`off`) and `destroy-unattached` at default (`off`). When the saver process crashes or exits, the session auto-destroys. Portal checks `tmux has-session -t _portal-saver` at bootstrap and recreates if missing. This is the simplest pattern. If debugging is needed, temporarily set `remain-on-exit on` on the saver session to see exit output.

### Q2: Signal Propagation from tmux Server to Hosted Processes

**`tmux kill-server` path** (verified from tmux source):
1. `cmd-kill-server.c`: sends `kill(getpid(), SIGTERM)` to the tmux server process itself.
2. `server.c`, `server_signal()`: catches SIGTERM, sets `server_exit = 1`, calls `server_send_exit()`.
3. `server_send_exit()`: iterates all sessions, calls `session_destroy(s, 1, ...)` for each.
4. Session destroy -> window destroy -> pane destroy.
5. `window.c`, `window_pane_destroy()`: closes the PTY master fd via `close(wp->fd)`. **No explicit `kill()` signal is sent to the child process.** The kernel sends SIGHUP to the process group on the slave side of the PTY when the master is closed — this is standard POSIX PTY behavior.

**So the signal path is:** tmux server receives SIGTERM -> destroys sessions -> closes PTY fds -> kernel delivers SIGHUP to child processes.

**Can the hosted process trap SIGHUP?** Yes. Go's `signal.Notify` can catch SIGHUP. The process receives SIGHUP (not SIGTERM, not SIGKILL) from the kernel PTY teardown. There is time to flush pending state and exit cleanly — SIGHUP is trappable. However, there is no configurable grace period in tmux; the PTY close happens immediately during the shutdown sequence, and the kernel delivers SIGHUP promptly. The Go process needs to handle the signal quickly (flush save, exit).

**`tmux kill-session -t _portal-saver` path:** Same mechanism — `session_destroy()` is called, panes are destroyed, PTY master closed, kernel sends SIGHUP to child. Same signal received by the hosted process.

**`killall tmux` / external SIGTERM:** Same path — tmux's signal handler catches SIGTERM and runs `server_send_exit()`. If tmux is SIGKILL'd, it dies immediately without cleanup — child processes get SIGHUP from kernel when the PTY master fd is closed by OS as part of process cleanup.

**Important caveat from tmux issue #1174:** When tmux is killed via signal (SIGHUP, SIGTERM, etc.), tmux *hooks* (like `session-closed`) do NOT run. But for Portal's use case this is irrelevant — the *hosted Go process* traps the signal directly, it doesn't depend on tmux hooks.

### Q3: Interaction with Standard tmux Commands

**`tmux ls` visibility:** Yes, `_portal-saver` shows up in `tmux ls`. There is no tmux option to hide a session from `list-sessions` output.

**Filtering:** `tmux list-sessions` supports `-f filter` (confirmed in man page and source). Portal can exclude it:
```
tmux list-sessions -f '#{?#{==:#{session_name},_portal-saver},0,1}'
```
Or equivalently filter by prefix pattern in Portal's Go code when calling `ListSessions`.

**No hidden-session convention in tmux.** There is no official mechanism for "system" or "internal" sessions. The underscore prefix is a Portal convention only — tmux does not treat it specially.

**Portal's own session picker:** Already filterable. Portal's `ListSessions` in `internal/tmux/tmux.go` can trivially add a name-prefix filter (`strings.HasPrefix(name, "_portal-")`) to exclude internal sessions from the TUI picker.

**Accidental attachment:** If a user runs `tmux attach -t _portal-saver`, they'd see the running Go process's stdout/stderr (the ticker loop output). Harmless — Ctrl-C would kill the save process (sending SIGINT), but Portal would recreate it at next bootstrap. Not ideal but acceptable. tmux-slay addresses this by naming its session with a clear `bg` prefix.

### Q4: tmux 3.5 / 3.6 Periodic Primitives

**No periodic execution primitive was added in tmux 3.5, 3.5a, or 3.6.** Verified by reviewing the CHANGES files for both releases.

3.5 added: `command-error` hook, prefix timeout option, systemd cgroup support.
3.6 added: scrollbars, Mode 2031 themes, OSC 8 hyperlinks, synchronized updates, `display-popup` enhancements.

No new hook names containing "interval", "timer", "periodic", or "tick". No enhancements to `set-hook` for interval-based invocation.

The only existing timer-adjacent mechanism is `status-interval` (which runs `status-right`/`status-left` shell commands at a fixed interval) — this is what tmux-continuum piggybacks on, and it's the exact fragile pattern Portal is replacing. There is no general-purpose periodic execution facility in tmux.

**The detached-session pattern remains the only viable approach** for running a periodic process within the tmux server's lifetime without external process management (systemd, launchd, cron).

### Q5: tmux-slay Implementation Details

From reading the tmux-slay source:

**Idempotency:** tmux-slay checks if a command is already running via `check_on_command()`, which calls `get_cmd_window_id()` to match against stored metadata. If found, the duplicate invocation is skipped. Portal's equivalent: `tmux has-session -t _portal-saver` — a single tmux API call, simpler than tmux-slay's approach because Portal uses one dedicated session, not a shared session with multiple windows.

**Init window pattern:** tmux-slay creates its background session with an "init" window (`-n "$TMUX_SLAY_INIT_WINDOW_TITLE"`) that stays alive permanently. This is a keepalive — when individual command windows close, the session survives because the init window is still there. Portal does NOT need this pattern because the saver process is designed to run indefinitely (30-second ticker loop). If it dies, Portal *wants* the session to die too (auto-cleanup), then recreates at next bootstrap.

**Session naming:** Configurable via `TMUX_SLAY_SESSION` env var, defaults to `bg`. Portal's `_portal-saver` naming (underscore prefix) is a stronger convention for filtering.

**Signal cleanup:** tmux-slay's cleanup is limited — it provides `tmux-slay kill COMMAND` and `tmux-slay killall` commands that destroy windows/sessions. No special signal handling. The follow-logs feature has a signal trap for temp file cleanup, but the hosted processes themselves have no graceful shutdown mechanism from tmux-slay's side.

**GitHub issues with the pattern:** No significant issues found with the detached-session-as-host pattern itself. The main tmux issues around sessions dying (#2410, #2081, #2626) relate to `exit-empty` and `destroy-unattached` interacting unexpectedly — both avoidable by leaving defaults in place and not setting `destroy-unattached on`.

## Limitations and Caveats

1. **SIGHUP vs SIGTERM:** The Go process will receive SIGHUP (from PTY close), NOT SIGTERM, when tmux shuts down. Portal's signal handler must trap SIGHUP explicitly, not just SIGTERM. This is a subtle but important implementation detail — many examples show SIGTERM trapping only.

2. **No grace period guarantee:** tmux closes the PTY fd immediately during shutdown. The Go process gets SIGHUP and should flush synchronously in the signal handler. If the save is mid-write, `AtomicWrite` (temp file + rename) ensures no corruption — but the current save cycle might be lost. The periodic 30-second cadence limits worst-case data loss to 30 seconds regardless.

3. **`destroy-unattached` edge case not fully verified:** The claim that `destroy-unattached on` would immediately kill a `-d` session needs testing. The source code suggests it would (the session has `attached == 0`), but this depends on when `server_check_unattached()` runs relative to session creation. Portal should NOT set `destroy-unattached` on the saver session — but if a user has `destroy-unattached on` in their global tmux config, it could kill `_portal-saver` immediately after creation. Worth testing.

4. **tmux-slay source review was partial.** The main script was readable but the tmux plugin entry point (`tmux-slay.tmux`) returned 404. The core patterns (idempotency, init window) were visible; signal-level details were sparse.

## Open Questions

1. If the user has `set-option -g destroy-unattached on` in their `.tmux.conf`, does this kill the `_portal-saver` session immediately on creation with `-d`? May need to explicitly `set-option -t _portal-saver destroy-unattached off` after creation as a defensive measure.

2. Should Portal trap both SIGHUP and SIGTERM in the saver process? SIGHUP covers the tmux-shutdown path, but a direct `kill <pid>` would send SIGTERM. Trapping both is cheap and covers more cases.

3. The `exit-empty` server option: if `_portal-saver` is the last session and dies, the tmux server exits. This is fine for the reboot case but could be surprising if the user kills their last "real" session while the saver is also dying. Probably not an issue in practice — the saver outlives user sessions because it only dies with the server.

## Sources

- https://man7.org/linux/man-pages/man1/tmux.1.html — tmux man page (remain-on-exit, destroy-unattached, new-session behavior)
- https://github.com/tmux/tmux/blob/master/server-fn.c — server_check_unattached(), server_destroy_pane(), destroy-unattached option handling
- https://github.com/tmux/tmux/blob/master/window.c — window_pane_destroy(), PTY close logic (SIGHUP via kernel, no explicit kill)
- https://github.com/tmux/tmux/blob/master/cmd-kill-server.c — kill-server sends SIGTERM to self
- https://github.com/tmux/tmux/blob/master/server.c — server_signal() and server_send_exit() shutdown sequence
- https://raw.githubusercontent.com/tmux/tmux/3.6/CHANGES — tmux 3.6 changelog (no periodic primitives)
- https://raw.githubusercontent.com/tmux/tmux/3.5a/CHANGES — tmux 3.5/3.5a changelog
- https://github.com/tmux/tmux/issues/1174 — hooks don't run when tmux killed by signal
- https://github.com/pschmitt/tmux-slay — tmux-slay implementation (idempotency, init window, session naming)
- https://forum.upcase.com/t/tmux-kill-session-and-dangling-processes/5242 — dangling processes after kill-session
- https://github.com/tmux/tmux/issues/1354 — remain-on-exit interaction issues
- https://github.com/tmux/tmux/issues/2410 — server immediately exits on new session (exit-empty behavior)
