# Investigation: Killing All Sessions Wipes Restore State

## Symptoms

### Problem Description

**Expected behavior:**
The user's rule: **a session the user kills never comes back; a session the user detaches from does.** Killing is intentional by name — the picker's `k` then `y`, `tmux kill-session`, or the user ending the session themselves removes it for good, and its scrollback and resume hooks go with it; killing every session that way is the same rule applied to each, and correctly ends with an empty restore state. Every other way the sessions stop — detaching, `tmux kill-server`, a reboot of the Mac — must leave them restorable, with `sessions.json`, their scrollback `.bin` files and their resume hooks intact, and with no special Portal shutdown step needed first. The user works normally and never has to think about safely shutting Portal down before killing tmux or rebooting.

Scope correction: the seed listed "killing every session while Portal is running" among the triggers that must not empty the restore state. The user has ruled that case intended behaviour — it is the kill path working as designed — so route 1 below, as the seed framed it (the user's own kills emptying the state), is not part of this defect. What remains is the same loss reached when sessions end *without* the user killing them.

**Actual behavior:**
From a code read (no live reproduction yet), two routes reach the same loss:

1. **Empty-capture route (daemon tick).** Once the user's own sessions are gone but the server survives hosting Portal's internal sessions (`_portal-saver`, `_portal-bootstrap`), `keepSessionNames` (`internal/state/capture.go`) filters out every `_`-prefixed name, so `ListSessionNames` succeeds while `keep` comes back empty. `CaptureStructure` takes the empty-`keep` path — skips pane enumeration, builds an empty `sessions` slice, never reaches the all-sessions-failed guard (`capture.go:93`, conditioned on `len(keep) > 0`) — and returns a structurally valid zero-session `Index` with a nil error, indistinguishable from a healthy capture of an empty machine. `Commit` (`internal/state/commit.go:22`) has no emptiness check: a populated → empty change is structural, so it writes the empty index over `sessions.json` (`AtomicWrite0600`) and then `gcOrphanScrollback`, whose referenced set from an empty index is empty, deletes every `.bin` file. One 1s tick replaces a full restore state with an empty one and deletes all saved scrollback.
2. **Sequential-close route (`session-closed` hook).** The `session-closed` global hook runs `portal state commit-now` synchronously on every session close (`internal/tmux/hooks_register.go` — synchronous by design so a killed session cannot be resurrected by the next tick), and commit-now commits through the same `Commit` + `gcOrphanScrollback`. A teardown that ends sessions one at a time while the hooks are registered would commit a progressively shrinking index and delete each closed session's scrollback as it goes — no empty capture needed. Whether a real reboot or `tmux kill-server` actually ends sessions in sequence while `run-shell` hooks still execute is unverified.

The irreversible half is the scrollback deletion: `sessions.json` is reconstructable by reopening sessions, the `.bin` files are not once removed. The shape to guard is the collapse from populated to empty (or near-empty in quick succession) with deletion attached, not the empty commit on its own.

**Started:**
Identified 2026-08-16 from a code read (Portal 0.11.0), not from an incident. It has never cost the user any state. The user has detached all sessions and rebooted the computer several times with no loss; their own guess — explicitly not known — is that the race has always landed in Portal's favour by chance.

### Manifestation

- Data loss: `sessions.json` overwritten with an empty (or shrunken) index.
- Data loss, irreversible: every saved scrollback `.bin` file removed by the housekeeping pass.
- No confirmation, no warning — the next restore simply has nothing to restore.

### Reproduction Steps

Route 1 (from the code read):

1. Portal runtime live (`_portal-saver` daemon running, global hooks registered) with a populated `sessions.json` and scrollback directory.
2. Kill every user session while the tmux server stays up hosting `_portal-saver` / `_portal-bootstrap`.
3. Within about one daemon tick, `sessions.json` holds zero sessions and the scrollback directory is empty.

Route 2 (from the code read, unverified against a real teardown):

1. Same live runtime.
2. Reboot the Mac or `tmux kill-server` such that sessions end in sequence while hooks still fire.
3. Each `session-closed` commit shrinks the index and deletes that session's scrollback.

The user's normal reboot — the real-world sequence a reproduction has to cover:

1. Close everything down first; often run `brew update` + `brew upgrade`.
2. Quit Ghostty — the tmux client detaches; the server, every session and the `_portal-saver` daemon stay up.
3. Reboot through macOS.
4. After login, reopen things by hand (a small number of apps reopen on login by themselves); `x` re-attaches Portal.

**Reproducibility:** Never observed. The user has rebooted several times with all sessions detached and lost nothing. Neither route has been reproduced live.

### Environment

- **Platform:** macOS 26.7.1, Portal 0.12.0, tmux 3.7c.
- **User conditions:** Portal runtime live — `_portal-saver` daemon running, global hooks (including `session-closed`) registered — with a populated `sessions.json` and scrollback directory. The user's install restores ~38–41 sessions.

### Impact

- **Severity:** Critical — silent, irreversible data loss (saved scrollback) from an ordinary action, with no warning, should either route fire. Never observed in practice.
- **Scope:** Anyone who reboots, kills the tmux server, or kills every session while the Portal runtime is live — the obvious thing to do before a reboot, and what someone tidying up would try.

### References

- Seed: `seeds/2026-08-16-killing-all-sessions-wipes-restore-state.md` (inbox bug).
- Current workaround — the only reboot path verified to preserve everything (2026-09-30, Portal 0.12.0: 38/38 sessions restored, 35/35 resume hooks fired, 38/38 scrollbacks replayed, zero warnings): back up `~/.config/portal` → `portal uninstall` (kills `_portal-saver`, whose SIGHUP makes the daemon flush a final atomic state and exit, then unregisters the global hooks including `session-closed`; touches no files) → confirm `flush_completed=true` in `portal.log` → reboot without running `x` / `portal open` → `x` after login.
- The one reboot on record before that (2026-09-02) happened with the daemon already dead, so it says nothing about the live-daemon case.

---

## Analysis

### Hypotheses

**Checkpoint depth:** check-ins

- **H1: Killing every user session while the tmux server stays up (held open by Portal's own hidden sessions) makes the next daemon tick save an empty restore state and delete every saved scrollback file** [ruled-out]
  Ruled out as a defect: the mechanism is real (E1, E1b, E9) but it is the kill path working as designed — the user's rule is that a killed session never comes back, so removing each killed session's record, scrollback and resume hooks is correct, and killing every session correctly ends empty. The mechanism's evidence below stays in the record because the defect routes (H3/H6, H5) run through the same pipeline.
  Basis: capture's all-sessions-failed guard only runs when at least one user session exists (`internal/state/capture.go:131` — `:93` in the 0.11.0 code the seed read); `Commit` has no emptiness check (`internal/state/commit.go:22`) and its housekeeping (`gcOrphanScrollback`) deletes every `.bin` the new index doesn't name.
  Code trace (see Code Trace → Route 1): confirmed by reading. The daemon tick is not even needed — every user kill fires `session-closed` → `commit-now`, so the commit that follows the *last* user kill is itself the empty commit, through the identical `RunCommitCycle` path. The daemon tick (dirty flag or the 30s `MaxGap`) is a second committer reaching the same result if the hook is absent.
  Reproduced (Experiments E1, E1b): with the hook, each kill shrinks `sessions.json` and deletes that session's `.bin` immediately, the last kill leaving 0 sessions / 0 files; without the hook, the daemon's next `MaxGap` tick (30s) commits the same empty state. The wipe leaves no INFO trace beyond `process: start … state commit-now` lines or a `capture: tick complete sessions=0`.
- **H2: On tmux server shutdown (kill-server, or SIGTERM at reboot) tmux destroys every session and then fires the session-closed hooks; any commit-now that still reaches an answering server sees zero sessions, commits empty and deletes the scrollback** [ruled-out]
  Basis: the seed's second route; `session-closed` runs `portal state commit-now` synchronously (`internal/tmux/hooks_register.go:84`) through the same `RunCommitCycle` → `Commit` → housekeeping.
  Ruled out by: tmux sets `server_exit` before destroying any session and from then on closes every new client connection (`server.c:392`, `:439`); `kill-server` is the same SIGTERM path. Reproduced (E2): `kill-server` on a live runtime with 5 saved sessions left `sessions.json` and every `.bin` intact; the one `commit-now` the hooks spawned failed its first read (`server exited unexpectedly`) and stood down. The protection is incidental: both committers stand down at their `@portal-restoring` read, whose failure is treated as "restore in progress, don't commit" (`state.RestoreWindowActive`), not by any teardown awareness. Sessions dying *before* the server exits are H5's territory.
- **H3: On server shutdown the daemon's final flush — SIGHUP when its pane closes, or SIGTERM delivered to it directly at reboot — captures a half-torn-down server and commits a partial or empty index** [confirmed]
  Basis: `defaultShutdownFlush` (`cmd/state_daemon.go:360`) runs a full `captureAndCommit` guarded only by `@portal-restoring`.
  Ruled out (SIGHUP half) by: the SIGHUP arrives because the server is destroying the saver's pane, i.e. after `server_exit` is set, so the flush's first read is refused. Reproduced (E2): `daemon: shutdown reason=sighup flush_completed=false` after a refused `@portal-restoring` read. Real-world (2026-09-02 15:47:27, Portal 0.11.0): the live daemon's SIGHUP flush hit `no server running`, nothing committed, the 46-session index survived. The direct-SIGTERM half was first read as a restatement of H5; the reboot-timing trials (E5–E7) confirmed it as its own route: a daemon SIGTERMed shortly before tmux itself begins exiting runs its flush capture *while* the server shuts down, and that capture reads the dying server as zero sessions (H6) and commits an empty index — `flush_completed=true`, 0 sessions, 0 files. Status moved from ruled-out (SIGHUP half only) to confirmed (SIGTERM half).
- **H4: On a real reboot every teardown-time committer finds the server already gone, its tmux read fails and nothing commits — which is why the user's reboots have survived** [ruled-out]
  Basis: several clean reboots with all sessions detached and the runtime live; a failed `ListSessionNames` returns an error before `Commit` is reached.
  Evidence so far: true for `kill-server` (E2) and for the one live-daemon server death in the logs (2026-09-02). The log retention window (2026-09-02 → today) holds no reboot taken with the daemon live: 2026-09-02's server died at 15:47:27 (no `commit-now` ran — a crash or SIGKILL rather than `kill-server`, which spawns one) before the reboot, and 2026-09-30 went through the uninstall procedure (`flush_completed=true` at 09:26:58 / 09:27:21, boot 10:07:19). The user's earlier live-daemon reboots predate the logs. Whether a real reboot always lets the server exit before any session dies is H5's question.
  Ruled out by (E5–E7): when the server, the daemon and the pane programs are each sent SIGTERM, the outcome is set by milliseconds of ordering Portal does not control. In the sandbox, tmux beginning to exit within ~6ms of the daemon's SIGTERM preserved everything (14/14 trials at 0–6ms); at 10–30ms, 9 of 22 trials wiped everything (H6), 2 lost all but one session and 11 kept everything (13 of the 22 ran with debug logging or a logging shim, which shift the timing); at 50ms–1.5s every session whose program died on SIGTERM was removed (H5, 3/3). Nothing structural guarantees "the server is gone first". Apple documents that loginwindow SIGKILLs user background processes at restart without a grace period ([The Life Cycle of a Daemon](https://developer.apple.com/library/mac/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/Lifecycle.html)), but whether the detached tmux server, the daemon and the pane programs receive SIGKILL or SIGTERM, and in what order, is undocumented — so the user's clean reboots are consistent with favourable delivery (or SIGKILL, under which no committer runs), not with a property Portal has.
- **H5: At reboot, macOS's shutdown SIGTERM reaches the processes inside panes (and the daemon itself) independently of the tmux server; any session whose panes all die before the server processes its own SIGTERM is destroyed while the server still answers, so its `session-closed` commit-now — and the daemon's SIGTERM-triggered flush — commit a shrunken index and delete that session's scrollback** [confirmed]
  Basis: tmux source shows that once the server begins exiting it refuses every new client (`server.c:392`), so loss at teardown needs sessions to die while the server is *not* exiting (Code Trace → tmux teardown semantics); a pane's process exiting on its own closes the pane, and the last pane closing destroys the session with the server alive. Portal's own restored panes run `sh -c '<hook>; exec $SHELL'`, a non-interactive shell that SIGTERM kills.
  Confirmed (E4, E5 at 50–200ms): with pane programs and the daemon signalled first and the server later, every session whose program died on SIGTERM (`exec sleep` panes, the restored resume-hook shape `sh -c '…; exec $SHELL'`, and `_portal-bootstrap`) closed with the server still answering; each close's `commit-now` committed a shrunken index and deleted that session's `.bin`, and the daemon's SIGTERM flush committed the same survivors (`flush_completed=true`). Interactive `zsh -i` panes ignore SIGTERM, so their sessions — including one whose foreground child died — survived to the save. A real-world reboot hits this only if the panes' programs die before tmux begins exiting.

- **H6: A tmux server that begins exiting while a committer is mid-capture reads as "zero sessions" — so the capture commits an empty index and the housekeeping deletes every scrollback file, including those of sessions that were still alive** [confirmed]
  Basis: found in the reboot-timing trials (E5): a 10ms gap between the daemon's SIGTERM and the server's wiped 5/5 sessions including two whose programs had survived.
  Mechanism, two paths: (1) Portal — `Client.ListSessions` swallows every `list-sessions` failure and returns an empty list ("the error is the no-server signal", `internal/tmux/tmux.go:130-135`), and `ListSessionNames` passes that on as a successful empty read; the committer's earlier reads (`@portal-restoring`, skeleton markers) had already succeeded, so nothing stands it down. Measured (E7, logging shim on `tmux`): every wiping trial's `list-sessions` returned `rc=1 server exited unexpectedly`, swallowed into `sessions=0`. (2) tmux — a command client accepted before the server began exiting is ended with a payload-less `MSG_SHUTDOWN`, which leaves `client_exitval` at its zero default, so `list-sessions` exits 0 with no output (`client.c:654-657`, `client_dispatch_exit_message`). Measured (E6): 2 of ~5,800 `list-sessions` calls racing a SIGTERM returned `rc=0` and nothing. Either path yields a valid empty `Index` with a nil error.
  Reach: any committer — the daemon's SIGTERM flush (E5, E7) and a hook-run `commit-now` (E7, gap 30ms #1: 0/0 with `flush_completed=false`). Where `list-sessions` succeeds but a later per-session read is refused, the all-sessions-failed guard holds (E7, gap 20ms #1 and 30ms #2: 5/5 kept).
  Beyond `list-sessions`: tmux's exit-0-and-nothing answer comes from the client side and is command-agnostic. Measured for `list-panes -a` (E8: 2 of ~2,900 calls); not observed for `capture-pane` in ~2,900 calls but taking the same client path. Two further destructive consumers follow from it, neither reproduced in a committer: (a) an empty pane listing with a surviving `show-environment` records a session with no windows (`capture.go:540-565`), so its `.bin` files are deleted while its name stays in `sessions.json`; (b) an empty `capture-pane` during the daemon's dump hashes differently from the saved file and `WriteScrollbackIfChanged` (`internal/state/scrollback.go:77-87`, from `dumpPane`, `cmd/state_daemon.go:316-337`) atomically overwrites that pane's `.bin` with zero bytes — a loss that involves no housekeeping pass and no change to the session list.

Trace lines, in order:
1. Empty-capture path: daemon tick and commit-now through capture → commit → scrollback housekeeping, then reproduce in a throwaway tmux server (own socket, isolated state dir, test-built binary) by killing every user session with the runtime live.
2. tmux 3.7c teardown semantics: kill-server / SIGTERM / SIGHUP ordering of session destruction, `session-closed` hook execution, and whether the dying server still answers client commands — from source, then the sandbox.
3. Daemon shutdown flush against a dying server: what the final capture sees under kill-server and under a direct SIGTERM — sandbox.
4. Real-world evidence: the user's `portal.log` around past reboots (read-only) — what commit-now and the shutdown flush logged during teardown.
5. macOS reboot delivery: how launchd's shutdown signals reach the tmux server and the daemon independently, versus kill-server.

### Code Trace

**Entry point:**
Every committer funnels into one cycle, `state.RunCommitCycle` (`internal/state/commit_cycle.go:52`), under the exclusive `commit.lock`. Three callers reach it:
- the daemon tick — `tick` (`cmd/state_daemon.go:174`) → `captureAndCommit` (`cmd/state_daemon.go:245`), run when `save.requested` is set or 30s (`MaxGap`, `cmd/state_daemon.go:446`) have passed since the last save;
- the daemon's shutdown flush — `defaultShutdownFlush` (`cmd/state_daemon.go:360`) → `captureAndCommit`, on SIGHUP or SIGTERM (`cmd/state_daemon.go:453`), skipped only while `@portal-restoring` is set;
- `portal state commit-now` (`cmd/state_commit_now.go`), run synchronously by the `session-closed` global hook (`internal/tmux/hooks_register.go:84`: `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`), skipped only while `@portal-restoring` is set.

**Execution path (Route 1 — sessions gone, server alive):**
1. `internal/state/scrollback.go:295` `captureAndRefile` — first tmux read is `ListSkeletonMarkers`, then `captureStructure`.
2. `internal/state/capture.go:88` `ListSessionNames` succeeds and returns an empty slice: `parseSessionList` (`internal/tmux/tmux.go:188-196`) already strips every `_`-prefixed session ("an underscore-prefixed session must never leak into user-visible output"), so `_portal-saver` / `_portal-bootstrap` never reach capture.
3. `internal/state/capture.go:93` `keepSessionNames` (`capture.go:453`) has nothing left to drop on the production client (it filters only for fakes) → `keep` empty.

At the `CaptureClient` boundary three different situations therefore arrive as the identical value `([]string{}, nil)`: only Portal's own sessions are alive; `list-sessions` failed (swallowed, `tmux.go:130-135`); `list-sessions` answered exit 0 with no output (tmux shutting down, E6). No check inside `internal/state` can tell them apart.
4. `internal/state/capture.go:96` the pane enumeration is skipped (`len(keep) > 0` false); the per-session loop runs zero times.
5. `internal/state/capture.go:131` the all-sessions-failed guard requires `len(keep) > 0` → not reached. A zero-session `Index` returns with a nil error. The waiting-pane carry — the protection capture already has for exactly this hazard, documented at `capture.go:45-50` ("would lose its only transcript to the commit's housekeeping pass") — is switched off on this path: `carryMissedWaitingSessions` is fed by `waitingTokens(grouped)` (`capture.go:152-153`), and `grouped` is only populated inside the `len(keep) > 0` branch, so with no enumeration no waiting pane's session is carried.
6. `internal/state/commit_cycle.go:69` `Commit` — `structuralChange` (populated prior vs empty) is true, so `AtomicWrite0600` overwrites `sessions.json` with the empty index (`commit.go:35`).
7. `internal/state/commit.go:39` `gcOrphanScrollback` — `ComputeReferencedSet` of an empty index is empty, so every `scrollback/*.bin` is removed (`commit.go:103`).

**tmux 3.7c teardown semantics (source: tag `3.7c`):**
- `kill-server` is `kill(getpid(), SIGTERM)` on the server itself (`cmd-kill-server.c`).
- On SIGTERM/SIGINT the server sets `server_exit = 1` and only then calls `server_send_exit` (`server.c:439-440`), which marks every client for exit and destroys every session in one synchronous loop (`server.c:310-326`); `session_destroy` queues a `session-closed` notification per session (`session.c`, `notify_session`), whose hooks run later from the command queue.
- Once `server_exit` is set, `server_accept` closes every new client connection immediately (`server.c:392-395`). Every `tmux` command a committer runs is a new client connection, so after the server has begun exiting no committer — a hook-spawned `commit-now`, or the daemon's shutdown flush reacting to the SIGHUP its pane's closure delivers — can read it: `ListSkeletonMarkers` fails first and `Commit` is never reached.
- The server keeps running until no clients remain and no non-`JOB_NOWAIT` job is running (`server.c:264-301`), so the hook jobs do run — against a server that refuses them.
- Consequence: under `kill-server`, and under any SIGTERM the server processes before its sessions die, no committer can observe a half-torn-down server. A teardown can only commit loss where sessions die *while the server is not exiting* — their panes' processes exiting on their own, or the user killing them.

**Key files involved:**
- `internal/state/capture.go` — empty `keep` yields a valid empty index with a nil error; the emptiness guard only covers "every session errored".
- `internal/state/commit.go` — `Commit` writes any structural change, including populated → empty; `gcOrphanScrollback` deletes every unreferenced `.bin`.
- `internal/state/commit_cycle.go` — the single cycle every committer runs.
- `cmd/state_commit_now.go` / `internal/tmux/hooks_register.go` — the per-kill synchronous commit on `session-closed`.
- `cmd/state_daemon.go` — the tick and the shutdown flush.

### Experiments

Sandbox: a dedicated tmux 3.7c server on `-S /Volumes/Scratch/tmp/claude-501/ptl-inv/<name>/s` (`-f /dev/null`), every process under `env -i` with a throwaway `HOME` / `PORTAL_STATE_DIR` and a `PATH` whose only `portal` is a `-tags integration` build of the working tree (0.12.0+); `_portal-bootstrap` and `_portal-saver` built as bootstrap builds them (`respawn-pane -k … 'portal state daemon'`, `destroy-unattached off`), the production `session-closed` body registered by hand. Each user session's pane prints 300 lines then idles. Saves forced by touching `save.requested`.

- **E1 — kill every user session, hook registered.** 3 sessions / 3 `.bin` saved. `kill-session` each in turn: 2/2 → 1/1 → 0/0, each step within ~1.5s, driven by the hook's `commit-now` (log shows only `process: start … state commit-now`).
- **E1b — kill every user session, no hook.** 3/3 saved; all three killed at once; the daemon's next tick 30s later (`MaxGap`) committed 0/0 (`capture: tick complete sessions=0 panes=0`).
- **E2 — `kill-server` on a live runtime.** 5/5 saved; `kill-server`; 6s later still 5/5. One hook-spawned `commit-now` ran and stood down (`isRestoring query failed; presuming @portal-restoring set` — `server exited unexpectedly`); the daemon got SIGHUP and stood down (`read @portal-restoring at shutdown failed; skipping final flush` — `no server running`; `shutdown reason=sighup flush_completed=false`).

Reboot-shaped population for E3–E7: `plain-a`, `plain-b` (pane program `exec sleep` — dies on SIGTERM), `shell-c` (`zsh -i` at its prompt — ignores SIGTERM), `shellfg-d` (`zsh -i` with `sleep` in the foreground — the child dies, the shell survives), `hook-e` (the restored resume-hook shape, `sh -c 'sleep …; exec zsh -i'` — the non-interactive `sh` dies), plus `_portal-bootstrap` (`exec sleep`) and `_portal-saver` (the daemon). "Pane programs" below means every descendant of the sandbox server except the daemon; every PID signalled was verified as a descendant of the sandbox server, whose argv names the sandbox socket, and checked against the live server's PID and the real daemon's.

- **E3 — one simultaneous SIGTERM to the whole tree.** Server first in the list (E3a) and server last (E3b): 5/5 kept both times. The daemon's flush stood down on `no server running`; no `commit-now` committed.
- **E4 — staggered: pane programs, 1.5s, daemon, 1s, server.** `plain-a`, `plain-b`, `hook-e` and `_portal-bootstrap` closed with the server answering; four `commit-now`s took the state to 2/2 (`shell-c`, `shellfg-d`); the daemon's SIGTERM flush committed the same 2/2 (`flush_completed=true`); `_portal-saver` closing fired a fifth `commit-now`. Three sessions' scrollback deleted.
- **E5 — pane programs and daemon together, server after a gap.** 0ms ×4, 3ms ×4, 6ms ×4: 5/5 kept. 10ms: 5 of 5 trials wiped to 0/0, including the two sessions whose shells survived — the daemon's flush logged `capture: tick complete sessions=0 panes=0` then `shutdown reason=signal flush_completed=true`. 20ms ×4: 2 kept 5/5, 2 left 1/1. 50ms and 200ms: 2/2 (H5's partial loss). With debug logging on, 10ms ×5 kept 5/5 (logging shifts the timing).
- **E6 — `list-sessions` racing a server SIGTERM.** 16 parallel clients hammering a 4-session sandbox server, SIGTERM mid-burst, six trials (~5,800 calls): `rc=0` with all sessions 634, `rc=1 no server running` the rest, and **2 calls `rc=0` with no output** — tmux reporting "no sessions" successfully while shutting down.
- **E7 — E5 with a logging shim on `tmux`** (records each `list-sessions` exit status and output). Every trial that wiped (15ms ×2, 20ms, 30ms) shows the committer's `list-sessions` returning `rc=1 server exited unexpectedly`, which Portal read as zero sessions; at 30ms the wiping committer was a hook-run `commit-now` (`flush_completed=false`). Trials where `list-sessions` still succeeded (`rc=0`, three sessions listed) kept 5/5: the later per-session reads were refused and the all-sessions-failed guard held.
- **E8 — `list-panes -a` and `capture-pane` racing a server SIGTERM** (E6's method, 16 clients alternating the two commands, six trials, ~5,800 calls). `list-panes` answered `rc=0` with nothing 2 times; `capture-pane` never did in ~2,900 calls; the rest `rc=0` with content or `rc=1`.
- **E9 — kill every user session with resume hooks registered.** Two sessions, one `portal hook set --on-resume` each (tokens stamped, `hooks.json` 2 entries). Both sessions killed: state 0/0 at once (H1), and 7s later the daemon's idle-tick sweep logged `hooks: clean-stale … entries=2`, each removed command at INFO; `hooks.json` → `{}`. The sweep's empty-read stand-down (`internal/hooksweep/sweep.go:98`) never fired because the live pane listing (`ListAllPaneHookKeys`, `tmux.go:721`) still returned Portal's own `_portal-saver` / `_portal-bootstrap` panes.

Sandbox fidelity: the sandbox ran `_portal-bootstrap` as `exec sleep`, which dies on SIGTERM; production `StartServer` creates it with no command (`internal/tmux/tmux.go:216-217`), i.e. the default interactive shell, which ignores SIGTERM. Saved-state outcomes are unaffected (internal sessions are never captured); E4's teardown sequencing differs — in production `_portal-bootstrap` survives, so one of E4's four `commit-now` runs would not occur and the server would not empty itself on that account.

Real-world logs (`~/.config/portal/state/portal.log.*`, 2026-09-02 → 2026-10-02, read-only): no reboot taken with the daemon live. 2026-09-02 15:47:27 (0.11.0): the server died abruptly with the daemon live (last tick 15:47:14, 46 sessions) — no `commit-now` ran, the daemon's SIGHUP flush hit `no server running`, nothing committed, and the 16:43 restore rebuilt 43 sessions (the 3 missing were `respawn-pane` restore failures, not lost state). That pattern — no hooks, a SIGHUP flush finding no server — is what a SIGKILLed server produces; what killed it is not determinable from the logs, and the power log (`pmset -g log`) does not reach back that far. 2026-09-30: the uninstall procedure (`flush_completed=true` at 09:26:58 and 09:27:21), boot 10:07:19, restore of 38 sessions at 10:12:53.

### Root Cause

Every Portal save treats the session list tmux gives it at that instant as the complete, authoritative truth, and acts on it irreversibly in the same step: whatever sessions the capture does not see are dropped from `sessions.json`, and the housekeeping pass that runs inside every commit (`gcOrphanScrollback`, `internal/state/commit.go:39`) deletes every scrollback file the new index no longer names. The same trust extends to the scrollback read: a pane whose read comes back empty has its saved `.bin` overwritten with zero bytes (`WriteScrollbackIfChanged`), with no housekeeping pass involved. For a session the user kills that is exactly right — a kill is final. The defect is that nothing in the pipeline distinguishes "the user ended this session" from "tmux or the machine is going away", so sessions that end — or merely appear to end — without the user killing them are removed just as finally. Two ordinary situations at reboot hand a save such a view, and each becomes permanent loss:

1. **The server begins shutting down while a save is mid-capture** (H6, via H3). A failed `list-sessions` is swallowed by `Client.ListSessions` and returned as an empty list (`internal/tmux/tmux.go:130-135`), and tmux itself can answer an in-flight `list-sessions` with exit 0 and no output during shutdown (`client.c` `MSG_SHUTDOWN` handling). The committer's earlier reads have already succeeded, so nothing makes it stand down: it commits an empty index and deletes every scrollback file, including those of sessions that were still alive. At reboot the daemon's SIGTERM-triggered final flush (`cmd/state_daemon.go:360`) is the save most likely to be in flight at that moment. The same exit-0-and-nothing answer can reach the pane listing and the scrollback read (H6 reach), losing a session's scrollback with its name kept, or zeroing one pane's `.bin`.
2. **At reboot, sessions die before tmux does** (H5). Programs inside panes receive the shutdown SIGTERM independently of the tmux server; a session whose programs exit closes while the server still answers, and the `session-closed` hook's synchronous `commit-now` — built so that a deliberately killed session is never resurrected — removes it and its scrollback, as does the daemon's SIGTERM flush.

**Why this happens:**
The save pipeline was designed around two premises that hold during interactive use and fail at teardown. First, that a session disappearing from tmux means the user removed it: the killed-session fix made every close commit synchronously on that premise, and the housekeeping pass was designed as "self-healing by construction" on the same one — an unreferenced file can only be an orphan. Second, that a tmux read which cannot be trusted reports itself as an error: the all-sessions-failed guard, the restore marker's fail-closed read and the commit lock all assume a broken read surfaces as `err != nil`. At shutdown neither holds: sessions vanish because the machine is going down, and the session listing — the one read whose emptiness is the destructive signal — reports a dying or unreachable server as a successful empty answer. Because deletion is coupled to the commit, the moment Portal is most wrong is also the moment it destroys the only copy of the scrollback. `kill-server` survives today only because both committers' *first* read (`@portal-restoring`) happens to fail and is read as "a restore is in progress" (`state.RestoreWindowActive`) — an incidental stand-down, not teardown awareness, and one that a server dying a few milliseconds later than that first read walks straight past.

### Contributing Factors

- **The session listing swallows failure as "no sessions".** `Client.ListSessions` returns an empty list on any `list-sessions` error (`internal/tmux/tmux.go:130-135`, "the error is the no-server signal"), a contract the v1 picker specification set so an absent server shows the empty state. Capture reuses it through `ListSessionNames` (`tmux.go:200`), where an unreadable server and an empty one become indistinguishable. A discriminating variant, `ListSessionsProbe` (`tmux.go:143`), already exists and is not used by capture.
- **tmux reports "no sessions" successfully while shutting down.** An in-flight command client caught by the server's exit is ended with a payload-less `MSG_SHUTDOWN`, leaving its exit status at 0 with no output (E6). Even a non-swallowing listing can read an exiting server as empty.
- **The emptiness guard covers only "every session errored".** The all-sessions-failed guard (`capture.go:131`) requires `len(keep) > 0`; an empty listing skips the pane enumeration and every per-session read, so no anomalous error can ever accumulate to trip it.
- **`Commit` has no plausibility check.** A populated → empty change is just a structural change (`commit.go:31`), written like any other.
- **Irreversible deletion is synchronous with every commit.** `gcOrphanScrollback` runs inside `Commit` immediately after the write (`commit.go:39`), so there is no window in which a wrong commit can be noticed or superseded before the scrollback is gone. `sessions.json` is reconstructable by reopening sessions; the `.bin` files are not.
- **Every session close commits.** `session-closed` → `commit-now` (`internal/tmux/hooks_register.go:84`) fires for deliberate kills and shutdown-time deaths alike; the hook has no way to tell them apart.
- **The daemon flushes on SIGTERM.** The shutdown flush runs a full capture-and-commit on SIGTERM as well as SIGHUP (`cmd/state_daemon.go:453`), guarded only by `@portal-restoring`. The resurrection specification reasoned about SIGHUP from `kill-server` as the dominant path and treated the final flush as safe because the write is atomic — atomicity prevents a torn file, not a valid-but-wrong one.
- **Destructive paths take inconsistent postures on an empty read.** The hook-staleness sweep stands down on an empty pane read and on a failed one (resume-hooks-silently-lost specification, mass-deletion guard); the session commit path, which deletes far more, has no equivalent.
- **Exposure depends on what runs in each pane.** Interactive shells ignore SIGTERM; plain-command panes and both of Portal's own restored resume-hook shapes do not — the eager `sh -c '<hook>; exec $SHELL'` (`cmd/state_resume_chain.go:201-203`) and the lazy waiting pane's parked chain `/bin/sh -c 'trap : INT QUIT; …'` (`cmd/state_hydrate.go:272`, `:280`), whose trap does not cover TERM. Lazy is the shipped default, so after a restore every pane not yet answered sits in that shape. The sessions most likely to be lost at reboot are the ones carrying resume hooks.
- **The waiting-pane transcript carry is bypassed by the empty-listing shortcut.** Capture already carries forward a waiting pane's session that missed a capture, precisely so its only transcript survives the housekeeping pass (`capture.go:45-50`), but the carry is fed by the pane enumeration that the empty-`keep` path skips (`capture.go:96`, `:152-153`), so under H6 it protects nothing.
- **The hooks path was hardened against this exact misreading; the sessions path was not.** `ListAllPaneHookKeys` returns an error on a failed read because "a caller reading a failure as 'no live panes' would orphan every hooks.json entry at once" (`internal/tmux/tmux.go:716-720`), while `ListSessions` reads a failure as no sessions.

### Why It Wasn't Caught

- **An empty save is a deliberate, tested contract — and the tests never ask what else produces it.** The empty result is pinned in three places: the "keep is empty after filtering" subtest of `TestCaptureStructurePreLoopFailFatal` (`internal/state/capture_test.go:1672` — nil error, no pane enumeration), the all-natural-churn subtest (`capture_test.go:759`), and `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (`cmd/state_commit_now_test.go:155`). They encode the deliberate close-all. What no test covers is that the same value also arrives for a failed or shutting-down server: the first subtest of `TestCaptureStructurePreLoopFailFatal` (`capture_test.go:1614`) asserts that a `ListSessionNames` failure makes capture error — through a fake that can return one — while `TestListSessions` (`internal/tmux/tmux_test.go:16`) pins "returns empty slice when tmux server is not running". The real `*tmux.Client` never delivers the error the capture test guards against.
- **The emptiness guard was built for the adjacent case.** It protects "tmux readable, every session errored"; "tmux reports nothing at all" was treated as truth (as the seed notes).
- **No test exercises a save racing a dying server, or the daemon SIGTERMed while tmux shuts down.** The integration suites cover `kill-server` and saver kills, where the first read fails and everything stands down — so the incidental protection looks like design.
- **The per-kill commit was validated only for deliberate kills.** The killed-session fix's premise — "every close is a decision to remove" — was never examined against machine shutdown.
- **The loss is silent.** A wipe leaves `capture: tick complete sessions=0` or bare `process: start … state commit-now` lines at INFO; no WARN marks a collapse, and scrollback deletions are not logged at INFO. Reboots with the runtime live have, by timing, landed favourably, and the documented workaround (`portal uninstall` before a reboot) keeps the user off the dangerous paths.

### Blast Radius

**Directly affected:**
- `sessions.json` — overwritten with an empty or shrunken index; the next restore brings back nothing, or only the survivors.
- `scrollback/*.bin` — deleted for every session the wrong view omits; unrecoverable.
- `scrollback/*.bin` zeroed in place — a pane whose scrollback read comes back empty during a dump has its file overwritten with zero bytes (H6 reach; not reproduced in a committer).
- Resume hooks (`hooks.json`). Entries are keyed by pane tokens; once the panes carrying them are gone, the daemon's hook-staleness sweep — every 10s on its idle tick (`cmd/state_daemon.go:106`, `:186`) — judges them stale and deletes the user-authored `on-resume` commands (each logged at INFO with its command, the only remaining copy). For sessions the user killed that is correct (E9 reaped both killed sessions' hooks 7s after the kills — Portal's own `_portal-saver` / `_portal-bootstrap` panes keep the pane listing non-empty, so the empty-read stand-down at `internal/hooksweep/sweep.go:98` does not fire). After a teardown-time wipe (H6) or partial loss (H5) and the next start, the same sweep reaps every hook whose pane was not restored — the user loses the resume commands of sessions they never killed.

**Potentially affected:**
- Any committer that reaches the session listing — the daemon's tick, its shutdown flush, and `commit-now` — whenever tmux is unreachable *after* the restore-marker read succeeds: a server crash mid-save, or the `tmux` client failing for any other reason at that point.
- `restore.Orchestrator.snapshotLiveSessions` (`internal/restore/restore.go:99`) reads a failed listing as "no live sessions" — non-destructive (restore would try to rebuild sessions that exist and fail on the collision), but the same misreading.
- `internal/resolver` and the picker read the listing the same way; read-only, and the empty state is their intended behaviour.

---

## Fix Direction

### Chosen Approach

{High-level description of the chosen fix direction}

**Deciding factor:** {Why this approach was selected over alternatives}

### Options Explored

{List whatever approaches were discussed — could be one, could be several. For each unchosen option, note why it wasn't selected.}

### Discussion

{Journey notes from the fix discussion — user priorities, concerns raised, edge cases surfaced, what shifted thinking. Brief for simple bugs, detailed for complex.}

### Testing Recommendations

- {Test that should be added}
- {Test that should be added}
- {Existing test that should be modified}

### Risk Assessment

- **Fix complexity:** {Low / Medium / High}
- **Regression risk:** {Low / Medium / High}
- **Recommended approach:** {Hotfix / Regular release / Feature flag}

---

## Notes

{Any additional observations, questions for later, or context}
