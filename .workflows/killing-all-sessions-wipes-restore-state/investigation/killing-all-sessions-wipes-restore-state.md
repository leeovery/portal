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

**Checkpoint depth:** {straight-through | check-ins}

{Live ledger — ids are permanent, statuses evolve through the analysis:}
- **{H1}: {hypothesis}** [{suspected | tracing | confirmed | ruled-out}]
  {basis, then evidence as it accumulates}

### Code Trace

**Entry point:**
{Where the problematic flow starts}

**Execution path:**
1. {file:line - description}
2. {file:line - description}
3. {file:line - description}

**Key files involved:**
- {file} - {role in the bug}
- {file} - {role in the bug}

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
