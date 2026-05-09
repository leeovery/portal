# Specification: Killed Sessions Resurrect on Restart

## Specification

## Problem Statement

Portal's bootstrap-step-5 skeleton-restore runs a per-pane hydrate helper that waits up to 3s on a per-pane FIFO for a "go" signal from tmux's `client-attached` / `client-session-changed` hooks. When the signal fails to arrive, the helper times out into a recovery path that leaks state and silently degrades user-visible features.

This bug fixes the upstream signaling contract and corrects two compounding defects in the timeout-recovery path.

### Observed Symptoms

- **Symptom A — killed sessions resurrect on next `portal open`** (driven by stuck `@portal-skeleton-<paneKey>` markers feeding the daemon-merge re-injection path).
- **Symptom B — on-resume hooks never fire** for panes that hit the timeout path; the pane gets a bare interactive shell.
- **Symptom C — scrollback save silently skipped** for any pane whose marker is stuck (the daemon's capture loop skips marked panes indefinitely).
- **Defect D — orphan `sh -c` wrapper** parked on every restored pane: the wrapper's trailing `; exec $SHELL` is unreachable on success, breaks pane-close-on-`exit`, and leaves a parked `sh` parent under tmux for the lifetime of the pane.

### Root Cause

`signal-hydrate` is fired by tmux's `client-attached` / `client-session-changed` per-session hooks, scoped via `#{session_name}` to the session the user is attaching to or switching to. Bootstrap step 5 creates skeletons for **every** non-live saved session; `signal-hydrate` writes only to the FIFOs of the attached session. On a cold-start with N saved sessions, only the attached session's panes get signaled. The remaining N−1 sessions' helpers wait 3s, never receive the signal, and time out — by design of the per-session signaling contract.

This is not a race with the helper's `O_RDONLY` open. It is a missing trigger: no signal will ever arrive for non-attached sessions during this server lifetime under the current contract.

The `handleHydrateTimeout` handler then compounds the missing-signal condition into the user-visible symptoms via two deliberate-but-now-incorrect choices: (1) leaves `@portal-skeleton-<paneKey>` set ("marker stays set so the next attach re-signals" — a promise that cannot be kept because the FIFO is unlinked at the same site, leaving no reader for any subsequent signal), and (2) routes the fall-through through `execShellAndExit` (bare shell), bypassing the hook-firing exec path used by every other recovery branch.

### Scope Boundary

The bug surfaces only on **cold-start** bootstrap (first `portal` invocation after the tmux server starts), because `internal/restore/restore.go` skips already-live sessions on subsequent invocations. The cardinality is "once per tmux-server lifetime, affecting all-saved-sessions-minus-one" — not "once per `portal open`".

Symptom A's user-visible behaviour was structurally fixed on `main` by the companion daemon-merge live-set filter (`.workflows/completed/daemon-merge-reintroduces-dead-sessions/`). Markers still leak under the bug pre-fix, but the merge no longer turns the leak into a resurrection. This work additionally hardens the upstream cause so the marker stops leaking in the first place.

---

## Working Notes
