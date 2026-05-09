# Investigation: Killed Sessions Resurrect On Restart

## Symptoms

### Problem Description

**Expected behavior:**
- When a tmux session is killed — either by the user pressing the tmux prefix + `Alt-Q` (`kill-session`) from inside the session, or by selecting the session in the Portal TUI list and pressing `K` then confirming — the session should disappear from the Portal list AND stay gone across the next `portal open`.
- Per-pane on-resume hooks registered with `portal hooks set --on-resume "<cmd>"` should fire end-to-end on the next bootstrap, in the restored pane, with the configured command running before the user's interactive shell.

**Actual behavior:**
- **Symptom A** — Killed sessions disappear from the Portal list as expected, but on the very next `portal open` they reappear. Only some sessions are affected, consistently the same ones (on the reporter's machine: `leeovery-Gi5NLG`, `leeovery-feqhpg`). Other sessions kill cleanly.
- **Symptom B** — On-resume hooks never fire end-to-end for the affected panes. The reporter has not yet observed the registered command run on any restored pane.

The reporter and a prior investigator agreed both symptoms appear to share a single upstream trigger inside the hydrate path.

### Manifestation

Reported against Brew-installed `portal 0.3.1`. HEAD spot-checks of the relevant code paths suggest unchanged from the released binary, so HEAD is expected to reproduce — to be confirmed.

Observed at the time of the report:

- Three `@portal-skeleton-<paneKey>` tmux server-options were still set despite no restoration being in progress:
  - `agentic-workflows-XXrJ3J__1.1`
  - `leeovery-Gi5NLG__1.1`
  - `leeovery-feqhpg__1.1`
- Symptom A reproducers (`leeovery-Gi5NLG`, `leeovery-feqhpg`) are in this set.
- `~/.config/portal/state/portal.log` contains, dated to the most recent bootstrap, repeated entries of two distinct shapes for those same paneKeys:
  - `WARN | hydrate | write fifo …/hydrate-<paneKey>.fifo: open …: no such file or directory` — emitted by the `signal-hydrate` server-side handler (the global `client-attached` hook), failing because the per-pane FIFO does not yet exist when the signal lands.
  - `WARN | hydrate | timeout waiting for signal on --hook-key=<sess>:<w>.<p> --fifo=…` — emitted by the per-pane hydrate helper after waiting 3 s for a signal on its FIFO that never arrives.
- The reporter's `~/.config/portal/state/sessions.json` was being rewritten by the daemon (recent mtime), and the saved index continued to list the affected sessions even after they were killed.

### Reproduction Steps

1. Boot Portal so that bootstrap step 5 reconstructs the affected sessions.
2. Observe `portal.log` for `WARN | hydrate | write fifo …` and `WARN | hydrate | timeout waiting for signal on …` entries against specific paneKeys.
3. Inspect server options: `tmux show-options -s | grep '@portal-skeleton-'` — affected paneKeys still have markers set.
4. Kill one of the affected sessions (TUI `K`, or `tmux kill-session` from inside it). Confirm it disappears from the Portal list.
5. Run `portal open` again. Affected session reappears.
6. Inspect any registered on-resume hook for an affected pane — it has not run; the pane is just a bare interactive shell.

**Reproducibility:** Always, for the affected panes on the reporter's machine. Not all panes are affected — the conditioning factor is whatever causes the hydrate-signal race to land "signal first, FIFO second" for a specific pane.

### Environment

- **Affected environments:** Reporter's local macOS install (Brew `portal 0.3.1`).
- **Platform:** tmux + Portal CLI (Go).
- **User conditions:** Long-running tmux sessions reconstructed by bootstrap step 5 (Restore). Affected paneKeys are the same across reboots once they enter the stuck state.

### Impact

- **Severity:** Medium. Symptom A is low-grade but persistent — users cannot reliably clear sessions and a confirmed kill that is silently undone is a credibility issue. Symptom B effectively makes the on-resume-hooks feature non-functional in any environment where any pane times out hydrate.
- **Scope:** All users whose hydrate signals race ahead of the helper FIFO open. Probably common, not rare — same paneKeys reproduce across every bootstrap on the reporter's machine.
- **Business impact:** N/A (single-user CLI tool). User-trust impact: confirmed kills being undone, and a documented feature (on-resume hooks) silently not running.

### Supporting Observations (may or may not be related)

- `signal-hydrate` "write fifo … no such file or directory" entries appear **twice at the same timestamp** for the same paneKey on some bootstraps. Suggests the `client-attached` hook is firing more than once in quick succession — possibly from the alt-screen toggle in TUI bootstrap, or from how the `signal-hydrate` global hook is registered. Whether this matters for the bug at hand is unclear.
- `portal.log` also contains transient `WARN | daemon | capture pane <paneKey>: failed to capture pane "<paneKey>": exit status 1` entries against live, non-stuck sessions. Sporadic; same panes capture cleanly on later ticks. Possibly a race with `respawn-pane` during the hydrate window. Not user-visible but worth investigating alongside this work.
- The volume of warnings on every bootstrap is high enough to drown out future genuine warnings.

### Addendum 2026-05-09 — orphan `sh -c` wrappers post-timeout

When investigating a separate slowness issue, three `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from the previous day's bootstrap were still alive (~20 hours old) for the same three paneKeys this bug names. On inspection, the inner `portal state hydrate` had long exited (presumably via the timeout path → `execShellAndExit` → exec'd the user's shell into the pane). The wrapper `sh` is parked waiting on the now-interactive shell child, which won't exit while the user has the pane open. The trailing `; exec $SHELL` in the wrapper is therefore dead code in practice — the helper has already exec'd `$SHELL` itself, and the wrapper's own `; exec $SHELL` after that is unreachable.

Probably minor (not load-causing), but every timed-out hydrate leaves a `sh` process parented to the tmux server until the pane closes, which in long-running sessions effectively means forever. Worth considering as part of the timeout-path redesign here, since the wrapper construction is the same code site that owns the bypass-hooks decision (Symptom B).

### References

- Original inbox file (now archived): `.workflows/.inbox/.archived/bugs/2026-05-08--killed-sessions-resurrect-on-restart.md`
- Related bug (already fixed): `.workflows/completed/daemon-merge-reintroduces-dead-sessions/` — the daemon merge defect that turns "stale `@portal-skeleton-*` marker" into "killed session reappears in `sessions.json`".
- Originating feature scope: `.workflows/completed/built-in-session-resurrection/` — source of truth for the timeout-path design choices that this bug interrogates.
- Likely-relevant code paths (pointers, NOT a fix proposal):
  - `cmd/state_signal_hydrate.go` — the `client-attached` handler that writes to the per-pane FIFO. Race partner.
  - `cmd/state_hydrate.go` — `runHydrate`, `handleHydrateTimeout`, `execShellAndExit`, `execShellOrHookAndExit`. Race partner; also where the timeout-path's "no hook firing" and "marker stays set" decisions live.
  - `internal/state/markers.go` — `@portal-skeleton-*` server-option lifecycle (`SetSkeletonMarker`, `UnsetSkeletonMarker`).
  - `cmd/state_hydrate.go` step 8 — the only success-path marker unset; non-fatal on failure.
  - `cmd/bootstrap/bootstrap.go` — step 2 (`RegisterPortalHooks`) for how `client-attached` is registered, step 5 (`Restore`) for the skeleton-build that primes the FIFO, step 7 (`SweepOrphanFIFOs`) for orphan-FIFO cleanup.
  - `~/.config/portal/state/portal.log` and `~/.config/portal/state/sessions.json` — useful artefacts to inspect on a reproducing machine.

---

## Analysis

### Initial Hypotheses

**Working hypothesis (to be validated):** The upstream trigger is a race between two bootstrap-step-5 actors:

- `signal-hydrate` (the global `client-attached` handler) writes a single byte to the per-pane FIFO at `…/hydrate-<paneKey>.fifo`.
- The hydrate helper (`portal state hydrate`, launched as the pane's initial process via `respawn-pane -k` during skeleton reconstruction) `mkfifo`s the FIFO and then `O_RDONLY`-blocks reading from it.

When the signal lands before the helper has the FIFO open, the writer hits ENOENT and the helper times out 3 s later.

Two design decisions in `handleHydrateTimeout` then compound the timeout into the user-visible symptoms:

1. **`handleHydrateTimeout` deliberately leaves the `@portal-skeleton-<paneKey>` server-option set** (commented as "marker stays set so the next attach re-signals"). The "next attach re-signals" promise is itself questionable — by the time the timeout fires, the helper has exec'd a bare shell, so the FIFO has no reader anymore; a subsequent `client-attached` signal would just hit ENOENT again. Meanwhile the persistent marker drives Symptom A (via the daemon-merge re-injection path, which is itself an already-fixed adjacent bug).
2. **`handleHydrateTimeout` deliberately routes to `execShellAndExit` (bare shell), bypassing the hook-firing exec.** This drives Symptom B.

### Code Trace

_To be filled in during code analysis._

### Root Cause

_To be confirmed during code analysis._

### Contributing Factors

_To be filled in._

### Why It Wasn't Caught

_To be filled in._

### Blast Radius

_To be filled in._

---

## Fix Direction

_To be discussed in findings review after code analysis is complete._

---

## Notes

- The "already-fixed" daemon-merge bug referenced above is currently in the `daemon-merge-reintroduces-dead-sessions` branch (review phase, in progress on `main`). That fix removes one of two compounding factors on Symptom A — the marker-driven session re-injection — but the upstream race and the marker-leakage design choice still exist. Investigate whether, with the daemon-merge fix in place, Symptom A still reproduces purely from the `@portal-skeleton-*` marker leakage, or whether it has been incidentally masked.
