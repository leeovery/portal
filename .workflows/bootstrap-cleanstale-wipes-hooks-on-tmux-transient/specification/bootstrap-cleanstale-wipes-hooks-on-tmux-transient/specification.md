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

## Root Cause

**Dual conflation across two layers, neither of which independently makes the bug appear, but jointly produce silent destruction of `hooks.json` whenever `tmux list-panes -a` returns an empty result for any reason during a bootstrap-triggering command.**

### Layer 1 — Helper Swallow (`internal/tmux/tmux.go:687-693`)

`(*tmux.Client).ListAllPanes` swallows every error class — transient transport failures, exit ≠ 0 from a saver-respawn race, server-gone, and legitimate empty — into the same `([]string{}, nil)` signal. The contract is irreversibly ambiguous from this layer up.

```go
func (c *Client) ListAllPanes() ([]string, error) {
    output, err := c.cmd.Run("list-panes", "-a", "-F", "...")
    if err != nil {
        return []string{}, nil
    }
    return parsePaneOutput(output), nil
}
```

The peer helper `ListAllPanesWithFormat` (same file, lines 655-665) does the opposite — it propagates errors. The conflation in `ListAllPanes` is a documented, intentional behavioural divergence. The cost of that divergence is exactly this bug.

### Layer 2 — Unguarded Destructive Consumer

Two callsites pass the (possibly empty) live-pane slice straight to `hooks.Store.CleanStale` with no hazard guard against the `empty live set + non-empty store` combination, and no adapter-level logging:

- `cmd/bootstrap_production.go:76-83` — `cleanStaleAdapter.CleanStale` (bootstrap step 11).
- `cmd/clean.go:75-91` — `portal clean` subcommand's hook-cleanup tail.

The hooks store itself (`internal/hooks/store.go:130-159`) is correctly scoped — it does precisely what its name says ("remove entries for keys not present in `liveKeys`"). The destructive end-state is therefore a caller-side defect, not a store-side defect.

### Why This Happens

The helper's docstring describes the swallow as a convenience for the "no tmux server" case — a real, legitimate situation where `portal clean` should not error out. The convenience was retained when the helper was reused inside bootstrap step 11, where the same swallow becomes a vector for silent destruction because the caller's context guarantees a tmux server *is* running. The error-swallow assumption that was safe outside tmux became unsafe inside tmux without the docstring or implementation being revisited.

### Architectural Inconsistency

Bootstrap step 9 (`CleanStaleMarkers`) implements the same diff-then-delete shape against tmux server-option markers and is **immune** to this defect by construction:

1. Uses `ListAllPanesWithFormat` (the error-propagating helper).
2. Has an explicit mass-deletion hazard guard (`cmd/bootstrap/stale_marker_cleanup.go:126-141`) that refuses to unset markers when the parsed live set is empty but markers exist.

Step 11 has the same shape of work but neither safeguard. **The architectural inconsistency is the bug.** The fix lifts the step-9 pattern verbatim into the two affected callsites.

---

## Working Notes
