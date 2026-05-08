# Investigation: Daemon Merge Reintroduces Dead Sessions

## Symptoms

### Problem Description

**Expected behavior:**
The daemon's structural index merge should not re-introduce sessions that have been killed in tmux. The `@portal-skeleton-<paneKey>` server-option marker is meant to preserve pre-hydrate state for panes that have been skeleton-restored but not yet hydrated — it must not override the live tmux session list.

**Actual behavior:**
`mergeSkippedPanes` → `mergePane` → `findOrAppendSession` (`internal/state/capture.go:154`) appends a session from `prev.Sessions` into the freshly-built index without checking whether that session still exists in tmux. Any paneKey in the marker set whose session has been killed in tmux causes the dead session to be re-committed to `sessions.json`. On the next bootstrap, the restore phase reconstructs the killed session from the saved state. To the user this presents as: "killed sessions reappear after Portal restart."

### Manifestation

- Killed session reappears in `~/.config/portal/state/sessions.json` within one daemon tick (≤30s) after deletion in tmux.
- Subsequent Portal bootstrap reconstructs the dead session via `internal/restore` from the polluted index.
- No error or warning surfaces — silent data corruption of the source of truth.

### Reproduction Steps

Synthetic repro (does not require triggering the hydrate cascade):

1. Inside a live tmux server, identify an existing pane and its paneKey.
2. Set the marker manually: `tmux set-option -s @portal-skeleton-<paneKey> 1`
3. `tmux kill-session -t <session>` against the session containing that pane.
4. Wait one daemon tick (≤30s).
5. Inspect `~/.config/portal/state/sessions.json` — the killed session is present.

**Reproducibility:** Always, given the marker set + session killed conditions.

### Environment

- **Affected environments:** Local — any installation running `portal state daemon`.
- **User conditions:** Any user running Portal with the resurrection feature enabled (i.e. all current users).

### Impact

- **Severity:** High — silent corruption of the persisted state; user-visible "zombie" sessions; eroded trust that `kill-session` is permanent.
- **Scope:** All users; triggers under any path that produces a stale `@portal-skeleton-*` marker (hydrate timeout, daemon crash mid-hydrate, version-upgrade restart, manual tmux ops).
- **Business impact:** Trust regression on a core product promise (user controls their session list).

### References

- Inbox source: `.workflows/.inbox/.archived/bugs/2026-05-08--daemon-merge-reintroduces-dead-sessions.md`
- Companion bug (different layer, shared symptom): `.workflows/.inbox/bugs/2026-05-08--killed-sessions-resurrect-on-restart.md`
- Spec context: `built-in-session-resurrection` feature work in `.workflows/completed/`

---

## Analysis

### Initial Hypotheses

The merge step in `CaptureStructure` trusts the `@portal-skeleton-<paneKey>` marker set as authoritative evidence that a session is "in flight" (skeleton-created, awaiting hydrate). It does not cross-check the live tmux session list — which it has just enumerated in the same call — so a stale marker pulls a dead session forward indefinitely.

### Code Trace

To be filled during code analysis.

### Root Cause

To be filled during synthesis.

### Contributing Factors

- Marker is server-scoped (`set-option -s`), persists across hydrate failures, daemon restarts, and manual tmux ops.
- `prev` in `captureAndCommit` is updated to the just-committed index every tick, so once a dead session is committed once, it persists in `prev` indefinitely — the bug is self-sustaining.
- No defensive check at the merge step; the merge currently treats the marker set as the sole gate.

### Why It Wasn't Caught

To be filled during analysis.

### Blast Radius

**Directly affected:**
- `internal/state` — committed `sessions.json` becomes inconsistent with live tmux.
- `internal/restore` — reconstructs ghost sessions on bootstrap.

**Potentially affected:**
- Any consumer that reads `sessions.json` (CLI list commands, TUI session picker after a restart) sees the ghost session.

---

## Fix Direction

To be filled during findings review.

---

## Notes

This bug is independently wrong from the companion hydrate-cascade bug. Even with a perfect FIFO IPC, markers can become stale via process crashes, version-upgrade restart, or manual tmux operations. The merge logic should not assume marker validity on the user's behalf.
