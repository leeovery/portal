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

## Fix Scope

A single coordinated fix across three code sites that addresses the upstream trigger and the two downstream defects:

1. **Bootstrap eager-signaling step** — new step that writes the hydrate signal to every freshly-armed skeleton pane immediately after restore, removing the per-session signaling gap.
2. **Timeout-path corrections** in `cmd/state_hydrate.go` — turn the timeout fall-through from a leaky bypass into a correct recovery path (unset marker, fire hooks).
3. **Wrapper drop** in `internal/restore/session.go` — replace `sh -c '<helper>; exec $SHELL'` with the bare helper invocation.

The three changes are bundled because (a) the eager-signaling step resolves the root cause architecturally, and (b) the timeout-path corrections and wrapper drop are cheap defensive changes at the same code sites the eager-signaling step interacts with — splitting them would leave the recovery path incoherent against the new steady state.

### What stays in place

- **Both `client-attached` and `client-session-changed` registrations remain** as defensive idempotent fallbacks. They cover disjoint attach paths: outside-tmux `attach-session` fires only `client-attached`; inside-tmux `switch-client` fires only `client-session-changed`. Removing either would regress one path. Their second-fire on already-hydrated panes is a no-op (marker already unset, `signal-hydrate` skips).
- **The 3-second helper timeout is preserved** as a safety net for genuine signal-flow bugs (FIFO disappears between `mkfifo` and helper open; `signal-hydrate` regression). Eager signaling makes the timeout path rare-but-correct rather than common-and-broken.

### What is explicitly out of scope

- **Removing either hydration-trigger hook registration.** Both cover disjoint attach paths.
- **Panic-resilience wrapping for `portal state hydrate`.** If a real-world panic in the helper surfaces, address with a `defer recover` inside `runHydrate`, not with a respawn-pane-level fallback.
- **The transient `daemon | capture pane … exit status 1` warnings** noted in the investigation — sporadic, same panes capture cleanly on later ticks. Tracked separately if it persists.
- **Any change to the daemon's merge logic** — the companion fix on `main` already neutralises Symptom A's user-visible resurrection.

## Fix 1: Bootstrap Eager-Signaling Step

### Behaviour

A new bootstrap orchestrator step is inserted **after step 5 (Restore) and before step 6 (Clear `@portal-restoring`)**. The step iterates the freshly-set `@portal-skeleton-*` marker map and writes the single-byte hydrate signal to every pane's per-pane FIFO. After the step completes, every helper armed during restore has received its signal — usually within milliseconds of being respawned — and proceeds through the success path: marker unset, scrollback replayed, hooks fired (if registered), then `exec $SHELL`.

### Placement and Ordering Invariant

The step **must** run while `@portal-restoring` is still set. The daemon's `captureAndCommit` loop is suppressed during the `@portal-restoring` window, so helpers can dump scrollback and unset their markers without any chance of a concurrent capture on a pane mid-replay. This matches the existing helper invariants (the 100 ms settle sleep + marker-unset sequence) without introducing a new contract.

The placement also runs the step **after** restore has populated the skeleton-marker set, so the iteration source — `state.ListSkeletonMarkers` (via the existing `@portal-skeleton-*` server-option enumeration) — has the complete set of freshly-armed panes.

### Pane Enumeration and FIFO Resolution

The step does not require an additional tmux round-trip beyond `state.ListSkeletonMarkers`. The pane-target list is the marker map itself; the corresponding FIFO path is deterministic via `state.FIFOPath(stateDir, paneKey)`. No `list-panes` enumeration is needed at this layer.

### Write Primitive

The step uses the existing `writeFIFOSignal` helper and `signalHydrateRetryDelays` retry schedule from `cmd/state_signal_hydrate.go`, mirroring the per-session signaling posture so failure semantics remain identical between the eager step and the existing `client-attached` / `client-session-changed` paths.

### Failure Posture

- **Per-FIFO write failures are soft warnings.** The step logs a `WARN | hydrate | …` entry mirroring `runSignalHydrate`'s posture and continues to the next pane. A failed write does not abort the step.
- **The step itself never escalates to a fatal bootstrap error.** Mirrors steps 7 (CleanStaleMarkers), 8 (SweepOrphanFIFOs), and 9 (CleanStale) — best-effort cleanup classed alongside other non-fatal post-restore steps.
- **Zero markers post-Restore is a no-op.** No FIFO writes attempted; step returns nil.

### Relationship to Existing Hook-Driven Signaling

The `client-attached` and `client-session-changed` registrations remain in place. After the eager step has run, the user's subsequent attach (which is what causes the bare-CLI handoff or `tmux switch-client`) fires its hook against panes whose markers have already been unset by the helpers' success path; `signal-hydrate` enumerates the now-empty marker set for that session and exits cleanly. This is the desired "second-fire is a no-op" behaviour.

### Bootstrap Step Numbering Update

After this insertion the orchestrator's step list becomes:

1. EnsureServer
2. RegisterPortalHooks
3. Set `@portal-restoring`
4. EnsureSaver
5. Restore
6. **EagerSignalHydrate** *(new)*
7. Clear `@portal-restoring`
8. CleanStaleMarkers
9. SweepOrphanFIFOs
10. CleanStale

The `CLAUDE.md` "Server bootstrap" section will be updated to reflect the new ordering as part of the fix.

### Adapter Wiring

The new step's seam interface is wired through `internal/bootstrapadapter` in the same shape as the existing post-restore steps (concrete `*tmux.Client`, `state` package functions). No new package-level dependencies introduced.

---

## Working Notes
