# Plan: Killed Session Resurrects Within Tick Window

## Phases

### Phase 1: Synchronous Commit On Kill
status: approved
approved_at: 2026-05-21

**Goal**: Eliminate the kill→resurrection race window by making the `session-closed` tmux hook synchronously rewrite `sessions.json` before the hook subprocess exits, covering every kill path (TUI `K`, `portal kill`, `Option-Q`, `M-q`, external `tmux kill-session`) through one tmux-side seam.

**Why this order**: The bug has a single root cause (eventual-consistency `sessions.json` rewrite on kill) and one structural fix (synchronous capture-and-commit invoked from the `session-closed` hook). The new `portal state commit-now` subcommand and the hook registration migration are co-dependent — neither half alone changes user-visible behaviour, so they ship as one vertical slice. The phase includes the spec-mandated real-tmux re-entrancy gate, which validates the chosen mechanism before declaring the work complete; if that gate fails, the work returns to specification, not to a follow-up phase.

**Acceptance**:
- [ ] For every kill path (TUI `K`, `portal kill`, `Option-Q`, `M-q`, external `tmux kill-session`), the killed session is absent from `sessions.json` by the time the kill-triggered hook subprocess exits — no sleep, no retry.
- [ ] Immediately after any kill, a fresh `portal` bootstrap does not reconstruct the killed session as a skeleton pane via step 5 `Restore`.
- [ ] Immediately after any kill, a fresh `portal` invocation does not list the killed session in the TUI Sessions page.
- [ ] When `@portal-restoring` is set on the tmux server, `portal state commit-now` performs no work and `sessions.json` is byte-identical before and after invocation.
- [ ] `_portal-saver` self-kill in steady state (marker clear) produces a `sessions.json` that omits `_portal-saver` via the `keepSessionNames` underscore-prefix filter and retains all other live sessions intact.
- [ ] `_portal-saver` self-kill during bootstrap step 4 version-upgrade (marker set) short-circuits; `sessions.json` is byte-identical before and after the hook subprocess runs.
- [ ] A bootstrap from a pre-fix install (with `notifyCommand` registered on `session-closed`) results in exactly one `commitNowCommand` and zero `notifyCommand` registrations on `session-closed`; repeated bootstraps append no duplicates.
- [ ] `commit-now` failure (tmux unreachable, disk error) exits non-zero, touches `save.requested` as the daemon fallback, and does not block or revert the kill.
- [ ] On successful sync commit, `save.requested` is not touched; on `@portal-restoring` short-circuit, `save.requested` is touched.
- [ ] The six other save-trigger events (`session-created`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`) continue to fire `portal state notify` unchanged — no structural capture cost is added.
- [ ] The daemon's `tick()` body, period, scrollback `.bin` ownership, content hashing, and `mergeSkippedPanes` merge filter are unmodified.
- [ ] The daemon's next tick after any `commit-now` invocation produces a `sessions.json` whose session-name set still omits the killed session (semantic non-regression for `daemon-merge-reintroduces-dead-sessions`).
- [ ] Bootstrap steps 7 (`Clear @portal-restoring`), 8 (`CleanStaleMarkers`), and 9 (`SweepOrphanFIFOs`) continue to function identically.
- [ ] `portal state notify` semantics are preserved: zero tmux calls, zero `sessions.json` writes, only touches `save.requested`.
- [ ] Real-tmux integration fixture confirms `commit-now` invoked from inside the `session-closed` hook completes without deadlock or hang within a reasonable bound (~1s).
- [ ] All required unit, integration, and regression tests enumerated in the spec's Testing Requirements section pass.
