# Specification: Daemon Merge Reintroduces Dead Sessions

## Specification

## Bug Summary

The daemon's structural index merge re-introduces sessions that have been killed in tmux. `mergeSkippedPanes` → `mergePane` → `findOrAppendSession` (`internal/state/capture.go:154`) appends sessions from `prev.Sessions` into the freshly-built index without checking whether those sessions still exist in tmux. Any paneKey present in the `@portal-skeleton-<paneKey>` server-option marker set whose session has been killed in tmux causes the dead session to be re-committed to `sessions.json`. On the next bootstrap, the restore phase reconstructs the killed session from the polluted index. To the user this presents as: **"killed sessions reappear after Portal restart."**

### Root Cause

`mergeSkippedPanes` treats the `@portal-skeleton-<paneKey>` marker set as authoritative evidence that the session is "in flight" (skeleton-created, awaiting hydrate). It does not validate against the live tmux session list — which is already known inside the same `CaptureStructure` call as `keep` / `idx.Sessions`. A stale marker therefore causes `findOrAppendSession` to append a dead session into the freshly-built index, which then gets committed to `sessions.json` and persists into `prev` indefinitely (self-reinforcing through `captureAndCommit`'s `deps.PrevIndex = &idx`).

### Why Markers Become Stale

The marker is set by `internal/restore/session.go` during bootstrap step 5 skeleton restore and unset by the hydrate helper after scrollback replay completes. Markers leak through any of:

1. Hydrate timeout — pane never gets hydrated; marker survives.
2. Daemon crash mid-hydrate — never reaches the unset.
3. User runs `tmux kill-session` against a not-yet-hydrated pane.
4. Version-upgrade of `_portal-saver` mid-hydrate.
5. Manual `tmux set-option -s @portal-skeleton-<key>`.

No cleanup path currently exists. Once a marker leaks, it persists for the tmux server's lifetime.

### Impact

- **Severity:** High — silent corruption of persisted state; user-visible "zombie" sessions; eroded trust that `kill-session` is permanent.
- **Scope:** All users running `portal state daemon`; triggers under any path producing a stale `@portal-skeleton-*` marker.
- **Manifestation:** Killed session reappears in `~/.config/portal/state/sessions.json` within one daemon tick (≤30s). No error or warning surfaces.

### Empirical Confirmation

Live in-the-wild observation (2026-05-08): three specific sessions resurrected after kill — `agentic-workflows-XXrJ3J`, `leeovery-Gi5NLG`, `leeovery-feqhpg`. `tmux show-options -s` revealed exactly three matching stale `@portal-skeleton-*` markers (`agentic-workflows-XXrJ3J__1.1`, `leeovery-Gi5NLG__1.1`, `leeovery-feqhpg__1.1`). Killing an unmarkered session (`game-ideas`) did NOT resurrect it. Marker presence is necessary AND sufficient (given a daemon tick) for the resurrection symptom.

## Fix Component A: Live-Set Filtering in `mergeSkippedPanes`

**Location:** `internal/state/capture.go`

**Behavior change:** Before processing prev's panes, build a structural map from the fresh index — session names → per-session window indices → per-window pane indices. The merge proceeds for a given prev pane only when **all three structural levels** (session, window, pane) exist in the fresh index. A skeleton marker is no longer treated as authoritative; it only protects panes whose full structural path tmux still acknowledges.

### Filtering Levels

All three levels must filter, not just session:

- **Session level** — A prev session whose name is not in fresh must NOT be merged, even when its paneKey is in `skipSet`.
- **Window level** — A prev window that exists in `skipSet` but whose window index is not present in the (otherwise-live) fresh session must be dropped from the merge result.
- **Pane level** — A prev pane that exists in `skipSet` but whose pane index is not present in the (otherwise-live) fresh window must be dropped from the merge result.

Session-level filtering alone was rejected: the same defensive flaw exists at window and pane level — `kill-window` or `kill-pane` against a still-live session leaves the analogous resurrection path open.

### Self-Healing Behavior

Once `mergeSkippedPanes` no longer reintroduces dead sessions, `sessions.json` self-heals on the next daemon tick. The polluted `prev` from prior ticks is discarded when the dead session no longer survives the merge — `captureAndCommit` then commits the clean index, and `deps.PrevIndex = &idx` propagates clean state forward.

### Preserved Behavior

The merge's intended use case — hydrate-in-progress panes briefly invisible to `list-sessions` — must remain correct. Phase A of restore creates the session in tmux **before** the marker is set, so legitimate hydrate-in-progress panes always have their session/window/pane visible in the fresh enumeration. The filter is structurally distinct from this case and does not affect it.

### Rejected Alternatives

- **Pre-filter `skipSet` in `captureAndCommit`** — Costs an extra `ListSessionNames` tmux call per tick that `CaptureStructure` already makes internally; staleness is a merge-layer concern.
- **Drop "introduce missing session" merge behaviour entirely** — May break the legitimate hydrate-phase-A race where a skeleton-restored session is briefly invisible to list-sessions. Higher behavioural risk than targeted filtering.

---

## Working Notes
