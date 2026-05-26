# Specification: Bootstrap CleanStale Wipes Hooks On Tmux Transient

## Specification

## Problem Statement

### Defect

Bootstrap step 11 (`CleanStale`) and the `portal clean` subcommand silently wipe all entries from `~/.config/portal/hooks.json` whenever `tmux list-panes -a` returns empty for any reason during execution. The destructive call leaves no log breadcrumb at the adapter layer — the only post-hoc evidence is the wiped file.

**Observed evidence (2026-05-26, ~17:11Z):** `hooks.json` went from 22 valid per-pane on-resume entries to `{}` empty during a bootstrap cycle. The same bootstrap left a Component B WARN at `17:11:25Z` (`sweep: list-panes _portal-saver failed, legitimate set empty: ...`), time-correlating the wipe with a tmux transient.

### Expected Behavior

When `list-panes -a` returns a non-zero exit code or an empty result, the cleanup must treat the live-pane set as **unknown**, not as **empty**. Specifically:

- Skip the destructive call.
- Log a `Warn` at `ComponentBootstrap`.
- Continue (next bootstrap retries).

This matches the posture already in force at:
- Bootstrap step 9 (`CleanStaleMarkers` — error-propagating helper + mass-deletion hazard guard).
- Component B orphan-sweep (logs and skips on transient `list-panes` failure).

### Failure Modes Covered

The fix must close both failure modes that produce the destructive end-state:

- **(a)** `list-panes -a` exit ≠ 0 (transient tmux failure during saver-respawn or under load) — evidenced by the observed Component B WARN.
- **(b)** `list-panes -a` exit 0 with empty stdout (saver mid-respawn momentary "no panes" reply) — plausible but unobserved; precautionary coverage.

### Scope of Wipe (Bounding)

The wipe affects **user-session hooks only**. Portal-internal sessions (`_portal-bootstrap`, `_portal-saver`) never have `hooks.json` entries — they are filtered out at registration. During a fired event: 100% of user hooks affected, 0% of portal-internal hooks.

### User-Visible Impact

At next reboot, no Claude session auto-resumes because every per-pane on-resume hook entry is gone. The user must manually identify and resume each Claude session via Claude's own session picker — precisely the scenario the Portal resume system was built to prevent. Recovery one-shot exists (`~/.claude/hooks/portal-resume-backfill.sh --apply`) but is not durable until this bug is fixed.

### Symptom Distinguishability

User-facing symptoms collide with the earlier `slow-open-empty-previews-and-zombie-sessions` bug ("none of my Claude sessions resumed"), but `portal.log` distinguishes them:

- FIFO race: 53 eager-signal `ENOENT` warnings.
- CleanStale wipe: **zero** warnings (silent at the adapter).

"Silent in logs" is itself a fingerprint for this defect class until the new adapter logging lands.

---

## Working Notes
