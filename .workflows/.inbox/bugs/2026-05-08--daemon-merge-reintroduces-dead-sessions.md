# Daemon's mergeSkippedPanes re-introduces sessions that no longer exist in tmux

This bug is logged separately from `2026-05-08--killed-sessions-resurrect-on-restart.md` because, although it is one of the contributing causes of that user-visible symptom, the defect lives in a different layer (the index-merge model) and is independently wrong even if the upstream hydrate failure is fixed. A future cause of stale `@portal-skeleton-*` markers — e.g. a daemon crash mid-hydrate, a manual `tmux kill-session` against a not-yet-hydrated pane, or any new code path that sets the marker — would still produce dead-session resurrection unless this defensive bug is fixed.

## Behaviour

The save daemon's `captureAndCommit` (`cmd/state_daemon.go`) calls `state.CaptureStructure` (`internal/state/capture.go`) once per tick. `CaptureStructure` builds a fresh structural index from `tmux list-sessions` + `tmux list-panes -a`, then optionally merges entries from the *previous* index back in via `mergeSkippedPanes`, keyed by the set of paneKeys whose `@portal-skeleton-<paneKey>` server-option is currently set.

The merge's stated purpose (per its own doc-comment) is to preserve pre-boot state for panes that have been skeleton-restored but not yet hydrated — i.e., panes whose live tmux state is "empty" but whose authoritative content lives in the prior `sessions.json`. That intent is correct.

The defect: `mergeSkippedPanes` → `mergePane` → `findOrAppendSession` (`capture.go:154`) appends a session from `prev.Sessions` into the fresh index *without checking whether that session still exists in tmux*. The marker is treated as sufficient evidence that the session is "in flight"; the actual live-tmux session list, which `CaptureStructure` already obtained earlier in the same call, is not consulted at the merge step.

Consequence: any paneKey in the marker set whose session has been killed in tmux causes that session to be re-created in the freshly committed `sessions.json`. The next bootstrap then reconstructs the killed session via the restore phase. To the user this presents as "killed sessions reappear after Portal restart" (see the companion bug for the user-facing narrative).

## Why this is independent

- The fix is local to `internal/state/capture.go` (most likely to `mergeSkippedPanes` or `findOrAppendSession`). The hydrate-side bugs are in `cmd/state_hydrate.go` and `cmd/state_signal_hydrate.go`.
- The fix is conceptually simple — the merge already knows the fresh session list (it was just built); it can intersect the skipSet against that list before merging. No coordination with the hydrate path required.
- Even with a perfect FIFO IPC, markers can become stale through other paths (process crashes, version-upgrade restart, manual tmux ops). The merge logic should not assume marker validity on the user's behalf.
- Investigators looking at the merge logic do not need to load the spec for the resurrection feature to reason about this bug; conversely, investigators looking at the hydrate IPC should not be blocked on understanding the merge invariants.

## Likely-relevant code paths (NOT a fix proposal — pointers only)

- `internal/state/capture.go` — `CaptureStructure`, `mergeSkippedPanes`, `mergePane`, `findOrAppendSession`. The fresh-vs-merged decision boundary.
- `internal/state/capture.go` `keepSessionNames` — already produces the live-session set used for the fresh capture; the merge step could plausibly consult it (subject to the next investigator's call).
- `internal/state/markers.go` — `ListSkeletonMarkers` is the source of the skipSet. Behaviour and contract worth re-reading.
- `cmd/state_daemon.go` — `captureAndCommit` is the single caller of `CaptureStructure` from the daemon path. Note the `prev` is updated to the newly-committed index on every successful tick — so once a killed session is committed once, it persists in `prev` indefinitely.
- The `built-in-session-resurrection` feature work in `.workflows/completed/` for the merge logic's original spec intent.

## Reproduction expectation

Should reproduce under any condition that produces a stale `@portal-skeleton-<paneKey>` server-option for a paneKey whose tmux session has been killed. The companion bug describes one such condition (hydrate timeout); a quick synthetic repro is to manually `tmux set-option -s @portal-skeleton-<some-existing-paneKey> 1`, kill that session, wait one daemon tick (≤30 s), and inspect `~/.config/portal/state/sessions.json` to see the killed session re-introduced.
