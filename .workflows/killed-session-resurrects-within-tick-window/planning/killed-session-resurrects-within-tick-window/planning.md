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

#### Tasks
status: approved
approved_at: 2026-05-21

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-session-resurrects-within-tick-window-1-1 | Add `portal state commit-now` happy-path subcommand | zero sessions, one session, multi-window multi-pane session, underscore-prefixed sessions filtered out, missing/corrupt `sessions.json` falls back to zero-value `PrevIndex` with WARN log |
| killed-session-resurrects-within-tick-window-1-2 | Add `@portal-restoring` short-circuit to `commit-now` | marker present (skip + dirty-flag touch), marker absent (proceed), `save.requested` touch failure during skip (best-effort, still exit 0) |
| killed-session-resurrects-within-tick-window-1-3 | Add `commit-now` failure-path discipline | tmux unreachable, disk error during commit, `save.requested` touch also fails (still exit non-zero, original failure dominates) |
| killed-session-resurrects-within-tick-window-1-4 | Migrate `session-closed` hook registration to `commitNowCommand` | empty hook state, pre-fix install with `notifyCommand` on `session-closed`, post-fix install (no-op idempotent re-run), `ShowGlobalHooks` failure, per-index `UnsetGlobalHookAt` failure (best-effort WARN), exact-string match must not remove user-customised `notify`-referencing hooks |
| killed-session-resurrects-within-tick-window-1-5 | Real-tmux re-entrancy integration gate | hang/deadlock signals spec-level pivot (test must fail visibly, not time out silently), tmux server unreachable during test |
| killed-session-resurrects-within-tick-window-1-6 | Real-tmux kill→bootstrap canonical symptom integration test | TUI `K`, `portal kill`, `Option-Q`, `M-q`, external `tmux kill-session` all converge through the same hook so one external kill suffices for the gate; `_portal-saver` self-kill under marker-set (byte-identical file) and marker-clear (underscore filter) |
| killed-session-resurrects-within-tick-window-1-7 | Non-regression tests for daemon merge and six-event eventual consistency | `PrevIndex` staleness in daemon merge, `pane-focus-out` high-frequency fire path, `session-renamed` rename without kill |

### Phase 2: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-session-resurrects-within-tick-window-2-1 | Promote save.requested touch into state package | ENOENT parent dir, mtime tolerance; test fake delegation; near-variant `os.WriteFile` callsite |
| killed-session-resurrects-within-tick-window-2-2 | Treat IsRestoring query failure as marker presumed set | `(false, err)` symmetric to `(true, nil)`; WARN log with cause; existing branches unchanged |
| killed-session-resurrects-within-tick-window-2-3 | Collapse dumpStateDir / dumpStateDirRaw duplication | single-caller rename; no other references |
| killed-session-resurrects-within-tick-window-2-4 | Collapse pollSessionsJSON / pollSessionsJSONForKill duplication and sessionNames variants | strict-subset predicate; poll-constant dedup; `map[string]struct{}` canonical helper |
| killed-session-resurrects-within-tick-window-2-5 | Replace errCommitNowFailed empty-message sentinel and preserve cause | `errors.Is` detection; `%w` cause wrap; subprocess exit-code/empty-stderr regression |
| killed-session-resurrects-within-tick-window-2-6 | Replace resolveCommitNowDeps tuple-of-six with *Deps struct | nil-field fallback; struct-shape test stubs |
| killed-session-resurrects-within-tick-window-2-7 | Remove redundant MigrationLogger noop fallback | real-cycle vs. structural-cycle branch; `(*state.Logger)(nil)` no-op contract; build/test green |

### Phase 3: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killed-session-resurrects-within-tick-window-3-1 | Delete defaultTouchSaveRequested wrapper | symmetry with sibling CommitNowDeps defaults; tests substitute the field not the wrapper symbol |
| killed-session-resurrects-within-tick-window-3-2 | Replace ErrStatusUnhealthy empty-string sentinel with a descriptive message | sentinel identity preserved via errors.Is; doc-comment now cites IsSilentExitError; main.go silent-exit path unchanged |
| killed-session-resurrects-within-tick-window-3-3 | Extract runPortalSubprocess helper to consolidate runPortalCommitNow and runPortalList | t.Helper() propagation; byte-equivalent failure-message format; trampolines retain names |
