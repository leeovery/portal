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

### Constraints & Confirmed Context

- **Live state preserved.** The broken state on the reporter's machine (three stale daemons, dead `_portal-saver`, sparse scrollback dir) is to be kept intact while investigation proceeds, so the live system can be used as an evidence source alongside code analysis.
- **Regression window is within the v0.5.x line.** Reporter is confident the session preview was working under some v0.5.x version. Investigation should establish the precise within-v0.5.x regression point rather than treating this as a long-standing latent fragility.

### References

- Inbox report (archived): `.workflows/.inbox/.archived/bugs/2026-05-22--slow-open-empty-previews-and-zombie-sessions.md`
- Related prior bugfixes: `multiple-state-daemons-running-concurrently` (introduced `daemon.lock` in v0.5.0), `killed-session-resurrects-within-tick-window` (introduced kill-barrier in v0.5.6).

---

## Analysis

### Initial Hypotheses (from reporter)

1. Multi-daemon contention is clobbering scrollback writes.
2. The killed-session-resurrects-within-tick-window kill-barrier in `portal_saver.go` can't reach orphan daemons because their lifetime is no longer bound to the saver pane.
3. `CaptureStructure`'s per-session error path aborts the whole tick instead of log-and-continuing, poisoning capture for every session.

Live probing during this investigation confirmed (1) and (3) and partially supported (2) — but the *trigger* turned out to be more specific than originally suspected.

### Live System Re-Enumeration

At analysis time (14:07 BST, ~3.5 h after the inbox report) the live state was different from the snapshot in the inbox: pids 10745 and 32832 are gone, and pids 41493 (in `_portal-saver`), 72588 (orphan, parent = real tmux server, binary v0.5.6), and 50897 (yesterday) remain.

```
$ pgrep -xfl 'portal state daemon'
41493 portal state daemon
50897 portal state daemon
72588 portal state daemon
```

**Each daemon holds `daemon.lock` at a different inode:**

| pid    | started      | binary                                     | daemon.lock inode | tmux socket / parent                                       |
|--------|--------------|--------------------------------------------|-------------------|------------------------------------------------------------|
| 50897  | yesterday 21:39 | `/private/tmp/portalbin/portal` (TEST build) | 170216314         | parent = `tmux -S /tmp/test_hook_debug2/s …` (leaked test fixture, still alive, 3 sessions) |
| 72588  | today 12:56  | `/opt/homebrew/Cellar/portal/0.5.6/bin/portal` | 172093006         | parent = pid 94966 (real tmux server); NOT in `_portal-saver` |
| 41493  | today 13:47  | `/opt/homebrew/Cellar/portal/0.5.6/bin/portal` | 172173283 ← **current `daemon.lock` inode** | parent = pid 94966; IS the pane process of `_portal-saver` |

`daemon.pid` points at **41493** (the legitimate one).

### Trigger: Leaked test-fixture tmux server + unsandboxed state dir

```
$ ls -la /tmp/test_hook_debug2/
.rw-------@ 0 leeovery 21 May 21:39 portal.log
srw-------@ - leeovery 21 May 21:39 s
drwx------@ - leeovery 21 May 21:39 scrollback
$ tmux -S /tmp/test_hook_debug2/s list-sessions
A: 1 windows (created Thu May 21 21:39:38 2026)
_anchor: 1 windows (created Thu May 21 21:39:38 2026)
_portal-saver: 1 windows (created Thu May 21 21:39:38 2026)
```

A test-fixture tmux server at `/tmp/test_hook_debug2/s` is still alive from yesterday evening. The test binary at `/private/tmp/portalbin/portal` was launched against this socket and is still running as pid 50897. **The test daemon writes to the user's real state directory** (`~/.config/portal/state/`) — it inherited that path from the user's environment because no test isolated XDG_CONFIG_HOME. The grep `test_hook_debug2` returns no matches in the repo, so this fixture was created by an external/manual test session yesterday rather than by an existing tmuxtest pattern.

### Observed scrollback churn (1 s sampling, 10 ticks)

```
[14:07:16]
[14:07:17] portal-efoxir__1.1.bin
[14:07:18]
[14:07:19]
[14:07:20] portal-efoxir__1.1.bin
[14:07:21] portal-efoxir__1.1.bin
[14:07:22]
[14:07:23]
[14:07:24] portal-efoxir__1.1.bin
[14:07:25]
```

Scrollback dir oscillates 0 / 1 file every tick. `sessions.json` at the same moment contains **only session "A"** (`SSH_CONNECTION = 10.0.1.41 …`) — i.e. the test-fixture daemon's view, not the real tmux's 22 sessions. The `saved_at` advances every ~1–2 s.

### Code Trace

**Daemon tick driver — `cmd/state_daemon.go:132-207` `captureAndCommit`:**

```go
idx, err := state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex)
if err != nil {
    return fmt.Errorf("capture structure: %w", err)   // ABORTS the entire tick
}
// ... per-pane scrollback write loop (correctly log-and-continue per-pane)
if err := state.Commit(deps.Dir, idx, anyScrollbackChanged, deps.Logger); err != nil {
    return fmt.Errorf("commit: %w", err)
}
```

**Per-session abort site — `internal/state/capture.go:62-106` `CaptureStructure`:**

```go
for _, name := range sortedKeys(keep) {
    envRaw, err := c.ShowEnvironment(name)
    if err != nil {
        return empty, err     // ANY single-session error aborts the whole tick
    }
    sessions = append(sessions, Session{...})
}
```

The per-pane loop in `captureAndCommit` (lines 185-192) correctly logs and continues on per-pane errors; the per-session loop in `CaptureStructure` is missing this defensive pattern. `git blame` shows the abort-on-error returned in commit `7dc990be4` on 2026-04-27 — present in every v0.5.x release.

**Commit + GC — `internal/state/commit.go:36-58` `Commit` and `:102-138` `gcOrphanScrollback`:**

```go
if err := fileutil.AtomicWrite0600(SessionsJSON(dir), data); err != nil { ... }
if err := gcOrphanScrollback(dir, idx, logger); err != nil { ... }
```

`gcOrphanScrollback` walks the scrollback directory and removes every `.bin` not referenced by the just-committed `idx`. Critically, **GC trusts whatever index the calling daemon produced** — there is no cross-check against any other daemon's view.

**Daemon-lock singleton — `internal/state/daemon_lock.go:55-77` `AcquireDaemonLock`:**

```go
f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)   // opens whatever inode is there NOW
if err := lockAcquire(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil { ... }
```

`flock(2)` excludes per-**inode**, not per-path. There is no post-open cross-check to verify that the inode we just locked is still the inode referenced by `daemon.lock`. If `daemon.lock` is unlinked + recreated between two daemon spawns (by any external cause — old code path, manual rm, test scaffolding), the two daemons end up flocking different inodes and the singleton invariant is silently broken.

**Kill-barrier — `internal/tmux/portal_saver.go:212-248` `killSaverAndWaitForDaemon`:**

```go
priorPID, _ := killBarrierReadPID(stateDir)
if !killBarrierIsAlive(priorPID) { _ = c.KillSession(PortalSaverName); return nil }
_ = c.KillSession(PortalSaverName)
// poll killBarrierIsAlive(priorPID) every 50ms until timeout (5s) or process dies
```

If the prior PID is not the saver pane's process (e.g. it's an orphan with a different controlling tmux server), `KillSession(_portal-saver)` cannot reach it. The barrier polls for an exit that never happens, times out at 5 s, and proceeds. No SIGTERM/SIGKILL escalation is attempted.

### Symptom → cause mapping (with live evidence)

**Empty previews (Bug 2)** — *fully explained*. The leaked test daemon (pid 50897) is connected to a different tmux server than the user's. It enumerates only session "A" from `/tmp/test_hook_debug2/s` (the only non-internal session there), writes a 1-session `sessions.json` to the user's real state dir, and then `gcOrphanScrollback` removes every `.bin` except `A__0.0.bin`. The legitimate daemon (pid 41493) writes the real 22-session view a fraction of a second later, restoring 22 `.bin` files; the leaked daemon overwrites again next tick. Live observation shows the dir oscillating 0/1 file. The TUI preview reads `state.ScrollbackFile(stateDir, paneKey)` (`internal/tui/preview_adapter.go:34-40`) and finds nothing most of the time. The per-session abort-on-error in `CaptureStructure` (lines 87-89) is a *separate* latent fragility that would amplify this even with a single daemon — any transient `show-environment` failure on an alphabetically-early session would empty all subsequent panes' scrollback for the same reason (Commit + GC after a partial-but-committed cycle).

**Slow `portal open` (Bug 1)** — *explained*. The reporter's earlier snapshot had `daemon.pid` pointing at pid 32832, an orphan with no live `_portal-saver` membership. `killSaverAndWaitForDaemon` killed the saver session and polled 32832; 32832 doesn't die from `kill-session` so the barrier timed out at 5 s every bootstrap. The current snapshot has `daemon.pid` = 41493 which IS the saver pane process, so the kill-barrier currently completes fast — but the 5 s timeout is structurally always one-orphaned-PID away from recurring.

**Killed sessions resurrect (Bug 3)** — *primary mechanism explained*. Multiple daemons write `sessions.json` independently every tick with each their own view. The legitimate daemon's tick after a user kill correctly writes a `sessions.json` without the dead session; another daemon (e.g. 72588 — same socket, different stale `prev`) rewrites `sessions.json` with the killed session still in it before the next bootstrap's Restore reads it. Restore then reconstructs the dead session as a skeleton pane. The merge-filter fix from `daemon-merge-reintroduces-dead-sessions` is intact and correctly applies inside each daemon, but it operates only on *that* daemon's `prev`; it cannot defend against a competing daemon's stale `prev` being committed seconds later.

### Key Files

| File | Role in the bug |
|------|-----------------|
| `internal/state/daemon_lock.go` | Singleton primitive that fails silently when the lock file inode is replaced between daemon spawns. |
| `internal/tmux/portal_saver.go` (`killSaverAndWaitForDaemon`) | Kill-barrier that cannot reach daemons not bound to the saver pane process. |
| `internal/state/capture.go:87-89` (per-session loop in `CaptureStructure`) | Aborts the whole tick on any per-session error, poisoning capture for every later session in the same tick. |
| `internal/state/commit.go` (`Commit` + `gcOrphanScrollback`) | Trusts the calling daemon's index for GC; produces destructive results when multiple daemons each commit different views. |
| `cmd/state_daemon.go:132-207` (`captureAndCommit`) | Propagates `CaptureStructure`'s error before any scrollback write or commit; correctly per-pane-tolerant for the per-pane loop. |
| `internal/tui/preview_adapter.go` | Reads scrollback `.bin` per paneKey; surfaces "no saved content" when the file is missing (which is most of the time under the GC race). |

### Dead Ends / Ruled Out

- **TOCTOU on session A in `ShowEnvironment`**: Manual `tmux show-environment -t A` succeeded every attempt; not intermittent. The daemon log entry `failed to show environment for session "A": no such session: A` appears to be from a different daemon connected to a different/transitional tmux state, not a structural per-attempt failure. It was distracting evidence; the abort-on-error path in CaptureStructure is the real latent fragility.
- **Merge filter regression**: `daemon-merge-reintroduces-dead-sessions`' Fix Component A is intact in current code (`mergeSkippedPanes` calls `buildLiveStructure` and three-level filter). Zombie sessions are caused by competing daemons rewriting `sessions.json`, not by merge-filter regression.
- **`saver-kill-respawn-loop-leaks-daemons` ctx-cancellable fix missing**: The fix shipped in v0.5.4 and is present in current code (`cmd/state_daemon.go` has three `<-ctx.Done()` observation points in `captureAndCommit`). The legitimate daemon does exit promptly on signal; the orphan daemons survive because they are no longer reachable from the saver-side kill path, not because they fail to honour cancellation.

### Contributing Factors

- **`daemon.lock` inode replacement is undefended.** No `O_EXCL`-create+stat-cross-check, and `flock` is per-inode. Any external cause of the lock file being unlinked + recreated breaks the singleton across the affected daemon generations.
- **The kill-barrier assumes daemon lifetime is bound to the saver pane process.** True only for daemons spawned by the production bootstrap path. Daemons spawned by tests, manual runs, or older code paths that didn't enforce this binding survive `kill-session` indefinitely.
- **`CaptureStructure` is not per-session-error-tolerant**, so a single bad session at the alphabetical head of the list defeats scrollback capture for everything else (latent fragility; manifests independently of multi-daemon contention).
- **`gcOrphanScrollback` is destructive based on a single daemon's view**, so any daemon with a partial/stale index can wipe `.bin` files written by another daemon in the same dir.
- **No test isolation of `XDG_CONFIG_HOME` for daemon-spawning tests** — the leaked test daemon (pid 50897) writes to the user's real state directory. Test fixtures should never be capable of corrupting a developer's live install.

### Why It Wasn't Caught

- The `multiple-state-daemons-running-concurrently` design (v0.5.0) assumed `daemon.lock` is a stable file whose inode persists for the state directory's lifetime. The tests for that bugfix exercise contention through the seam (`acquireDaemonLock` fake) rather than through real inode replacement at the path layer.
- The `saver-kill-respawn-loop-leaks-daemons` (v0.5.4) and `killed-session-resurrects-within-tick-window` (v0.5.6) fixes both *assume the daemon being killed is the saver pane process*. Neither has an escalation path for "the PID we recorded is alive but not killable via session kill".
- The abort-on-error in `CaptureStructure` has been latent since 2026-04-27 (commit `7dc990be4`); it does not manifest unless `ShowEnvironment` actually fails for some session, which is rare under normal use.
- Test scaffolding that spawns `portal state daemon` against a custom tmux socket is not required to also override `XDG_CONFIG_HOME` — there is no enforced isolation contract.

### Blast Radius

**Directly affected:**
- `internal/state/daemon_lock.go` — the singleton primitive needs to be robust against inode replacement.
- `internal/tmux/portal_saver.go` (`killSaverAndWaitForDaemon`) — needs SIGTERM/SIGKILL escalation when `kill-session` is insufficient.
- `internal/state/capture.go:87-89` — per-session error handling needs to log-and-continue.
- Bootstrap orchestrator — likely needs a new step that detects and signals orphan `portal state daemon` processes before the kill-barrier runs.
- Test infrastructure (tmuxtest / state-daemon integration tests) — needs an enforced state-dir isolation contract so a leaked test daemon can never write to the user's `~/.config/portal/`.

**Potentially affected:**
- Anything that relies on `sessions.json` being authoritative — restore (skeleton reconstruction), the TUI session list, `portal clean`, any tooling that reads it.
- Any user upgrading across binaries while a daemon from a prior version is still running — the singleton's inode-replacement gap is the upgrade-time landmine.

---

## Fix Direction

*To be populated after root cause synthesis and findings review.*

---

## Notes

Reporter's local diagnosis surfaced several candidate code locations (`internal/state/capture.go`, `cmd/state_daemon.go`, `internal/tmux/portal_saver.go`, `internal/state/daemon_lock.go`) and hypotheses about the failure modes — these are listed in the inbox report but **not** carried forward as conclusions. The investigation phase will re-derive findings independently before recording any analysis here.
