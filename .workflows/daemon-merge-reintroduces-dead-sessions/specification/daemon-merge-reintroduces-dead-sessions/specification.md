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

## Fix Component B: Stale-Marker Cleanup Bootstrap Step

**Location:** New step in the bootstrap orchestrator (`cmd/bootstrap/`), inserted **between current step 5 (Restore) and step 7 (SweepOrphanFIFOs)** — making it the new step 6, with subsequent steps renumbered (the existing "Clear `@portal-restoring`" step remains immediately after Restore as it does today; the new cleanup runs after that and before SweepOrphanFIFOs).

**Note on numbering:** The existing nine-step orchestrator has steps "5. Restore" → "6. Clear `@portal-restoring`" → "7. SweepOrphanFIFOs". The new cleanup step inserts between steps 6 and 7 in the existing sequence (i.e. after the restoring marker is cleared, before orphan FIFO sweep), pushing SweepOrphanFIFOs and later steps down by one.

### Behavior

1. Enumerate the live `@portal-skeleton-*` server-option markers via tmux.
2. Enumerate live tmux panes (paneKeys).
3. Compute the set difference: markers whose paneKey is **not** present in the live pane set.
4. For each stale marker, unset it via tmux (`set-option -us @portal-skeleton-<key>`).

### Soft-Warning Posture

Best-effort, mirrors the warning-soft semantics of the existing `CleanStale` step (step 8). Failure (tmux unavailable, individual unset error) surfaces as a soft warning collected by the orchestrator and drained post-bootstrap; it never escalates to a fatal abort.

### Adapter Wiring

A new seam interface exposed by the bootstrap Orchestrator, with the production adapter in `bootstrapadapter` wiring concrete dependencies:
- Marker enumeration (`state.ListSkeletonMarkers` or equivalent live read).
- Live pane enumeration (via `*tmux.Client`).
- Marker unset (via `*tmux.Client`).

Tests exercise the seam with mock implementations following the existing `bootstrap` testing pattern.

### Why This Step Is Needed

Fix Component A alone resolves the user-visible resurrection symptom because `sessions.json` self-heals once the merge filter rejects dead sessions. However, a quieter side-effect remains: while a marker is live for a paneKey, the daemon's capture loop **skips scrollback save** for that pane (`cmd/state_daemon.go:131-133`). For panes whose markers leaked but whose underlying sessions are still alive (or were re-created with the same key), scrollback content is silently not being saved. The cleanup step closes this gap and prevents indefinite marker accumulation across the tmux server's lifetime.

### Rejected Alternative

- **Defer marker cleanup to a separate work unit** — Rejected per user direction. The scrollback-save side effect is real for users now; bundling produces the cleaner outcome and both changes are local to layers already in scope for the merge logic.

## Testing Requirements

### Existing Tests to Replace

**`internal/state/capture_test.go:570`** — The test `TestCaptureStructureMergeSkippedPanes/merges a skipped pane's session and window from prev when missing from fresh` codifies the buggy behaviour as correct and **must be replaced** with its inverse:

> A prev session whose name is not in fresh must NOT be merged, even when its paneKey is in `skipSet`.

### Tests to Add

**Merge filter — structural-level tests:**
- Window-level filtering: a marker for a window that exists in prev but not in fresh (within an otherwise-live session) must be dropped from the merge result.
- Pane-level filtering: a marker for a pane that exists in prev but not in fresh (within an otherwise-live window) must be dropped from the merge result.

**Merge filter — regression test mirroring the empirical scenario:**
- Marker set, session killed, daemon tick → fresh capture must NOT reintroduce the session.

**Stale-marker cleanup — unit:**
- Given a marker whose paneKey doesn't correspond to a live pane, the cleanup unsets it.
- Given a live marker (paneKey still corresponds to a live pane), the cleanup leaves it alone.

**Stale-marker cleanup — bootstrap integration:**
- The new cleanup step runs at the right point in the orchestrator sequence (after step 6 "Clear `@portal-restoring`", before existing step 7 SweepOrphanFIFOs).
- The cleanup degrades to a warning on tmux failure, matching the soft-warning posture of `CleanStale`.

### Tests to Preserve

- Existing happy-path skeleton-marker tests in `internal/restore/session_markers_test.go` — the fix must not regress legitimate hydrate-in-progress merge behaviour.

## Acceptance Criteria

The fix is complete when:

1. The synthetic repro (set marker, kill session, wait one daemon tick) does **not** reintroduce the killed session into `sessions.json`.
2. The user's empirical scenario (the three resurrecting sessions with matching stale markers) does not recur after applying both Fix Component A and Fix Component B.
3. `sessions.json` self-heals on the next daemon tick after a previously-polluted commit (the polluted `prev` no longer perpetuates dead sessions).
4. After bootstrap, no `@portal-skeleton-*` marker exists for a paneKey that has no corresponding live pane.
5. While a stale marker exists between daemon ticks (before bootstrap cleanup runs), the merge filter prevents resurrection regardless of marker staleness.
6. The legitimate hydrate-in-progress flow remains correct — phase A skeleton-restored panes (marker set, session/window/pane present in tmux) are still merged from prev as expected.
7. All new tests pass; the previously-buggy test is replaced; existing happy-path tests remain green.

## Scope and Risk

### In Scope

Both changes are local to layers already in scope for the merge logic — they compose without architectural surgery:

- **Fix Component A** — Live-set filtering in `mergeSkippedPanes` (`internal/state/capture.go`). Approximately 15 lines (session/window/pane filtering).
- **Fix Component B** — New stale-marker cleanup bootstrap step. Approximately 50 lines including adapter wiring, plus orchestrator sequence and test updates.

### Files Touched

- `internal/state/capture.go` — `mergeSkippedPanes`, `mergePane`, `findOrAppendSession`.
- `internal/state/capture_test.go` — replace the codifying-bug test; add new structural-level and regression tests.
- `cmd/bootstrap/` — orchestrator sequence (insert new step), seam interface for marker cleanup.
- `internal/bootstrapadapter/` — production adapter wiring for the cleanup step.
- Bootstrap tests for the new step (sequence + soft-warning behaviour).

### Regression Risk

**Low.** Every consumer of `sessions.json` and the daemon's `prev` was traced; no caller depends on the buggy "merge can introduce arbitrary prev sessions" behaviour. The merge's intended use case (hydrate-in-progress) is structurally distinct from the bug surface and remains correct because phase A creates sessions in tmux before setting markers.

### Release Posture

**Regular release.** No hotfix needed — the symptom is recoverable (kill the same session twice, or restart the tmux server) and a manual workaround exists for affected users (`tmux set-option -us @portal-skeleton-<key>`).

## Out of Scope

### Companion Bug

The companion bug `killed-sessions-resurrect-on-restart` (logged 2026-05-08) is the most likely producer of stale markers in normal use, but it lives in a different layer (`cmd/state_hydrate.go` / `cmd/state_signal_hydrate.go`) and is independently scoped. **This work unit does not depend on it; the fixes are orthogonal.**

This bug is independently wrong from the companion hydrate-cascade bug. Even with a perfect FIFO IPC, markers can become stale via process crashes, version-upgrade restart, or manual tmux operations. The merge logic should not assume marker validity on the user's behalf — that property must hold regardless of how the companion bug is eventually resolved.

### Marker Production Path

This work unit does not modify the marker-set path (`internal/restore/session.go:380-384` `setSkeletonMarker`) or the marker-unset path (`cmd/state_hydrate.go:312` `UnsetSkeletonMarkerForFIFO`). The fix is defensive: it accepts that markers can leak and ensures the consumer (merge) and one new periodic cleanup (bootstrap) handle stale markers correctly.

---

## Working Notes
