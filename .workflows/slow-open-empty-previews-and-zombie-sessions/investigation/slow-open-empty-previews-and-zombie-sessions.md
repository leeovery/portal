# Investigation: Slow Open, Empty Previews, and Zombie Sessions

## Symptoms

### Problem Description

**Expected behavior:**
- `portal open` launches the TUI quickly (sub-second).
- Highlighting a session in the picker and pressing `Space` shows that session's captured scrollback in the preview pane.
- Killing a session with `K` (or via the user's `Option-Q` tmux shortcut from inside the session) removes it permanently from the picker.

**Actual behavior:**
- `portal open` takes 5–8 seconds before the TUI appears on every invocation, not just the first of the day.
- Every session preview shows "no saved content" for every session in the list. The scrollback is still present inside tmux when the session is entered — only Portal's preview path is empty.
- Killed sessions resurrect on the next `portal open` and persist indefinitely across multiple open cycles. Pre-v0.5.6, they would briefly reappear within a "tick window" then disappear after ~5 s; now they never disappear.

### Manifestation

Bootstrap log (`~/.config/portal/state/portal.log`) contains a repeating cycle:

```
WARN | bootstrap | prior daemon (pid=32832) did not exit within 5s
WARN | daemon | another daemon holds the lock; exiting
WARN | bootstrap | step 4 (EnsureSaver) failed: bootstrap _portal-saver: set destroy-unattached: failed to set session option destroy-unattached on _portal-saver: exit status 1: no such session: _portal-saver
WARN | hydrate | scrollback file not found for --hook-key=A:0.0 --file=/Users/.../scrollback/A__0.0.bin
```

Daemon log also contains repeated:

```
WARN | daemon | tick: capture structure: failed to show environment for session "A": exit status 1: no such session: A
```

— even though session "A" *does* exist in tmux and `tmux show-environment -t A` succeeds when invoked manually.

### Reproduction Steps

1. Have multiple `portal state daemon` processes running concurrently (observed empirically; root mechanism for accumulation TBD during code analysis).
2. Run `portal open`.
3. Observe ~5–8 s delay before TUI renders.
4. Highlight any session, press `Space` → preview pane shows "no saved content."
5. Press `K` on a session, confirm "yes." Session disappears from current view.
6. Exit, run `portal open` again. Killed session is back.

**Reproducibility:** Always, while the multi-daemon / dead-saver state persists.

### Environment

- **Portal version:** 0.5.6 (upgraded from 0.5.5; upgrade did not improve the preview-empty symptom which was already present on 0.5.5).
- **Platform:** macOS (Darwin 25.3.0), zsh.
- **tmux:** running, session "_portal-saver" missing.
- **State directory:** `~/.config/portal/state/`.

### Reporter's local diagnostic observations

- Three concurrent `portal state daemon` processes were alive (pids 10745 — start 07:37 today, 32832 — start 08:38 today, 50897 — start 21:39 yesterday). None matched any live tmux pane (`tmux list-panes -a` enumeration confirms).
- Each daemon's `daemon.lock` fd referenced a different inode (171463046, 171582571, 170216314 — confirmed via `lsof`). `daemon.pid` pointed at 32832.
- Pids 10745 and 32832 had PPID 94966 (the tmux server process); pid 50897 had PPID 50812 (other).
- Pid 32832 was spawned ~1 min after the v0.5.6 tag (08:37 BST today); pids 50897 and 10745 predate that tag and would have been launched by the v0.5.5 binary.
- `_portal-saver` tmux session was missing.
- Scrollback directory contained 1 `.bin` file at any moment despite `sessions.json` listing 22 sessions; the file changed across observations.
- `daemon.version` file content was `0.5.5`.
- Session "A" in tmux was created today 10:39, never attached, carried `SSH_CONNECTION` env from an SSH origin.

### Impact

- **Severity:** High — preview is functionally useless (empty for every session); kill operation is functionally broken (dead sessions accumulate indefinitely); every `portal open` pays a 5–8 s cost.
- **Scope:** This install confirmed; potentially affects any user whose state directory has accumulated stale daemons across upgrades.
- **Business impact:** Tool-author dogfooding; degrades core workflow value of session preview and session hygiene.

### References

- Inbox report (archived): `.workflows/.inbox/.archived/bugs/2026-05-22--slow-open-empty-previews-and-zombie-sessions.md`
- Related prior bugfixes: `multiple-state-daemons-running-concurrently` (introduced `daemon.lock` in v0.5.0), `killed-session-resurrects-within-tick-window` (introduced kill-barrier in v0.5.6).

---

## Analysis

*To be populated during code analysis.*

---

## Fix Direction

*To be populated after root cause synthesis and findings review.*

---

## Notes

Reporter's local diagnosis surfaced several candidate code locations (`internal/state/capture.go`, `cmd/state_daemon.go`, `internal/tmux/portal_saver.go`, `internal/state/daemon_lock.go`) and hypotheses about the failure modes — these are listed in the inbox report but **not** carried forward as conclusions. The investigation phase will re-derive findings independently before recording any analysis here.
