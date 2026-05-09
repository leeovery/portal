# Hydrate failure cascade: killed sessions resurrect, on-resume hooks never fire

Reproduced against the Brew-installed `portal 0.3.1` binary. There are at least two user-visible symptoms here, but they appear to share a single upstream trigger inside the hydrate path. They are logged together because the reporter and a prior investigator agreed a fix to the upstream trigger would likely resolve both.

## Symptom A — killed sessions resurrect after Portal restart

When a tmux session is killed — either by the user pressing the tmux prefix + `Alt-Q` (`kill-session`) from inside the session, or by selecting the session in the Portal TUI list and pressing `K` then confirming — the session disappears from the Portal list as expected. But on the very next `portal open`, the killed session is back in the list. The kill does not persist across a Portal restart for the affected sessions.

The bug does not appear to affect every session — only some. On the reporter's machine the offenders are consistently the same two sessions (`leeovery-Gi5NLG`, `leeovery-feqhpg`); other sessions kill and stay killed normally. Whatever state distinguishes "sticky" sessions from normal ones is the conditioning factor.

Impact: low-grade but persistent — users cannot reliably clear sessions they no longer want, leading to growing session-list clutter and confusion about what `K` actually does. A confirmed kill that is silently undone is also a credibility issue.

## Symptom B — on-resume hooks never fire

The reporter has Claude hooks configured at the user level that register a `portal hooks set --on-resume "…"` command for Claude-hosting tmux panes. The intended behaviour: when Portal restores the session at next bootstrap, the on-resume command runs in the restored pane and Claude restarts there. The reporter has not yet observed this work end-to-end.

Reading the spec and `cmd/state_hydrate.go`: the hook-firing terminal exec (`execShellOrHookAndExit`) is only reached on the **signal-arrived** and **file-missing** paths. The **timeout** path goes through `execShellAndExit` — explicitly bare-shell, deliberately no hook firing (per the spec's "Helper Behavior on Startup" step 3e). So whenever hydrate times out, on-resume hooks for that pane are silently skipped. Tying it back to symptom A: the same panes that show stuck `@portal-skeleton-*` markers are the panes whose hydrate timed out, which means they are also the panes whose on-resume hook never fired.

Impact: the on-resume-hooks feature appears non-functional under any condition that produces a hydrate timeout, which on the reporter's machine is "every bootstrap, for at least some panes".

## Suspicious supporting state observed at the time of the report

- Three `@portal-skeleton-<paneKey>` tmux server-options were still set despite no restoration being in progress: `agentic-workflows-XXrJ3J__1.1`, `leeovery-Gi5NLG__1.1`, `leeovery-feqhpg__1.1`. Symptom A reproducers are in this set.
- `~/.config/portal/state/portal.log` contains, dated to the most recent bootstrap, repeated entries of two distinct shapes for those same paneKeys:
  - `WARN | hydrate | write fifo …/hydrate-<paneKey>.fifo: open …: no such file or directory` — emitted by the `signal-hydrate` server-side handler (the `client-attached` hook), failing because the per-pane FIFO does not yet exist when the signal lands.
  - `WARN | hydrate | timeout waiting for signal on --hook-key=<sess>:<w>.<p> --fifo=…` — emitted by the per-pane hydrate helper after waiting 3 s for a signal on its FIFO that never arrives.
- The reporter's `~/.config/portal/state/sessions.json` was being rewritten by the daemon (recent mtime), and the saved index continues to list the affected sessions even after they were killed.

## The chain (working hypothesis — to be validated)

The upstream trigger appears to be a race between `signal-hydrate` (the global `client-attached` handler, which writes a byte to the per-pane FIFO) and the hydrate helper (which `mkfifo`s and `O_RDONLY`-blocks on the FIFO as the pane's initial process). Both run during bootstrap step 5. When the signal lands before the helper has the FIFO open, the write fails with ENOENT and the helper times out 3 s later.

Two design decisions then compound the timeout into the user-visible symptoms:

1. **`handleHydrateTimeout` deliberately leaves the `@portal-skeleton-<paneKey>` server-option set** (commented as "marker stays set so the next attach re-signals"). The "next attach re-signals" promise is itself questionable — by the time the timeout fires, the helper has exec'd a bare shell, so the FIFO has no reader anymore; a subsequent `client-attached` signal would just hit ENOENT again. Meanwhile the persistent marker drives Symptom A.
2. **`handleHydrateTimeout` deliberately routes to `execShellAndExit` (bare shell), bypassing the hook-firing exec.** This drives Symptom B.

A third bug — independently logged — is that the daemon's index-merge logic re-injects sessions whose paneKey appears in the (now-stale) marker set without first checking whether the session still exists in tmux. That's what turns "stale marker" into "killed session reappears in `sessions.json`". See `2026-05-08--daemon-merge-reintroduces-dead-sessions.md`.

## Other observations (may or may not be related; flagged so they are not lost)

- `signal-hydrate` "write fifo … no such file or directory" entries appear **twice at the same timestamp** for the same paneKey on some bootstraps. Suggests the `client-attached` hook is firing more than once in quick succession (possibly from the alt-screen toggle in TUI bootstrap, or from how the signal-hydrate global hook is registered). Whether this matters is unclear.
- `portal.log` also contains transient `WARN | daemon | capture pane <paneKey>: failed to capture pane "<paneKey>": exit status 1` entries against live, non-stuck sessions. They appear sporadic and the same panes capture cleanly on later ticks; possibly a race with `respawn-pane` during the hydrate window. Not user-visible but worth investigating alongside this work.
- The volume of warnings on every bootstrap is high enough to drown out future genuine warnings.

## Reproduction note

The reporter has only observed both symptoms against the Brew binary (v0.3.1); the in-repo HEAD has not been verified to reproduce or to be free of the same defects. The relevant code paths in HEAD are unchanged in spot-checks, so HEAD is expected to reproduce, but the next investigator should confirm before assuming this is release-only.

## Addendum 2026-05-09 — orphan `sh -c` wrappers post-timeout

When investigating a separate slowness issue today, three `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from yesterday's bootstrap were still alive (~20 hours old) for the same three paneKeys this bug already names (`agentic-workflows-XXrJ3J__1.1`, `leeovery-Gi5NLG__1.1`, `leeovery-feqhpg__1.1`). On inspection, the inner `portal state hydrate` had long exited (presumably via the timeout path → `execShellAndExit` → exec'd the user's shell into the pane). The wrapper `sh` is parked waiting on the now-interactive shell child, which won't exit while the user has the pane open. The trailing `; exec $SHELL` in the wrapper is therefore dead code in practice — the helper has already exec'd `$SHELL` itself, and the wrapper's own `; exec $SHELL` after that is unreachable.

Probably minor (not load-causing), but: every timed-out hydrate leaves a `sh` process parented to the tmux server until the pane closes, which in long-running sessions effectively means forever. Worth considering as part of the timeout-path redesign here, since the wrapper construction is the same code site that owns the bypass-hooks decision (Symptom B).

## Likely-relevant code paths (NOT a fix proposal — pointers only)

- `cmd/state_signal_hydrate.go` — the `client-attached` handler that writes to the FIFO. Race partner.
- `cmd/state_hydrate.go` — `runHydrate`, `handleHydrateTimeout`, `execShellAndExit`, `execShellOrHookAndExit`. Race partner; also where the timeout-path's "no hook firing" and "marker stays set" decisions live.
- `internal/state/markers.go` — `@portal-skeleton-*` server-option lifecycle (`SetSkeletonMarker`, `UnsetSkeletonMarker`).
- `cmd/state_hydrate.go` step 8 — the only success-path marker unset; non-fatal on failure.
- `cmd/bootstrap/bootstrap.go` — step 2 (`RegisterPortalHooks`) for how `client-attached` is registered, step 5 (`Restore`) for the skeleton-build that primes the FIFO, step 7 (`SweepOrphanFIFOs`) for orphan-FIFO cleanup.
- `~/.config/portal/state/portal.log` and `sessions.json` are useful artefacts to inspect on a reproducing machine.
- The `built-in-session-resurrection` feature work (in `.workflows/completed/`) is the originating scope; the spec there is the source of truth for the timeout-path design choices and should be re-read before changing any of them.
