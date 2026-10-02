# Investigation: Killing All Sessions Wipes Restore State

## Symptoms

### Problem Description

**Expected behavior:**
Rebooting the Mac, killing the tmux server, or killing every session while Portal's runtime is live leaves the saved restore state (`sessions.json`) and the saved scrollback (`.bin` files) intact, with no special Portal shutdown step needed first. The user works normally and never has to think about safely shutting Portal down before killing tmux or rebooting. Legitimately closing every session must still eventually persist as empty — a session the user deliberately kills must not come back on the next restore.

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

- **H1: Killing every user session while the tmux server stays up (held open by Portal's own hidden sessions) makes the next daemon tick save an empty restore state and delete every saved scrollback file** [tracing]
  Basis: capture's all-sessions-failed guard only runs when at least one user session exists (`internal/state/capture.go:131` — `:93` in the 0.11.0 code the seed read); `Commit` has no emptiness check (`internal/state/commit.go:22`) and its housekeeping (`gcOrphanScrollback`) deletes every `.bin` the new index doesn't name.
  Code trace (see Code Trace → Route 1): confirmed by reading. The daemon tick is not even needed — every user kill fires `session-closed` → `commit-now`, so the commit that follows the *last* user kill is itself the empty commit, through the identical `RunCommitCycle` path. The daemon tick (dirty flag or the 30s `MaxGap`) is a second committer reaching the same result if the hook is absent. Live sandbox reproduction pending.
- **H2: On tmux server shutdown (kill-server, or SIGTERM at reboot) tmux destroys every session and then fires the session-closed hooks; any commit-now that still reaches an answering server sees zero sessions, commits empty and deletes the scrollback** [suspected]
  Basis: the seed's second route; `session-closed` runs `portal state commit-now` synchronously (`internal/tmux/hooks_register.go:84`) through the same `RunCommitCycle` → `Commit` → housekeeping.
- **H3: On server shutdown the daemon's final flush — SIGHUP when its pane closes, or SIGTERM delivered to it directly at reboot — captures a half-torn-down server and commits a partial or empty index** [suspected]
  Basis: `defaultShutdownFlush` (`cmd/state_daemon.go:360`) runs a full `captureAndCommit` guarded only by `@portal-restoring`.
- **H4: On a real reboot every teardown-time committer finds the server already gone, its tmux read fails and nothing commits — which is why the user's reboots have survived** [suspected]
  Basis: several clean reboots with all sessions detached and the runtime live; a failed `ListSessionNames` returns an error before `Commit` is reached.
- **H5: At reboot, macOS's shutdown SIGTERM reaches the processes inside panes (and the daemon itself) independently of the tmux server; any session whose panes all die before the server processes its own SIGTERM is destroyed while the server still answers, so its `session-closed` commit-now — and the daemon's SIGTERM-triggered flush — commit a shrunken index and delete that session's scrollback** [suspected]
  Basis: tmux source shows that once the server begins exiting it refuses every new client (`server.c:392`), so loss at teardown needs sessions to die while the server is *not* exiting (Code Trace → tmux teardown semantics); a pane's process exiting on its own closes the pane, and the last pane closing destroys the session with the server alive. Portal's own restored panes run `sh -c '<hook>; exec $SHELL'`, a non-interactive shell that SIGTERM kills.

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
2. `internal/state/capture.go:88` `ListSessionNames` succeeds, returning only `_portal-saver` / `_portal-bootstrap`.
3. `internal/state/capture.go:93` `keepSessionNames` (`capture.go:453`) drops every `_`-prefixed name → `keep` empty.
4. `internal/state/capture.go:96` the pane enumeration is skipped (`len(keep) > 0` false); the per-session loop runs zero times.
5. `internal/state/capture.go:131` the all-sessions-failed guard requires `len(keep) > 0` → not reached. A zero-session `Index` returns with a nil error. (The waiting-pane carry, `carryMissedWaitingSessions`, carries only sessions holding a pane the live enumeration lists as resume-pending — with no enumeration there are none.)
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

### Root Cause

{Clear, precise statement of what causes the bug}

**Why this happens:**
{Explanation of the underlying issue}

### Contributing Factors

- {Factor 1 - why it enables the bug}
- {Factor 2 - why it enables the bug}

### Why It Wasn't Caught

- {Testing gap}
- {Edge case not considered}
- {Recent change that introduced it}

### Blast Radius

**Directly affected:**
- {Component/feature}
- {Component/feature}

**Potentially affected:**
- {Component/feature that shares code/patterns}

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
