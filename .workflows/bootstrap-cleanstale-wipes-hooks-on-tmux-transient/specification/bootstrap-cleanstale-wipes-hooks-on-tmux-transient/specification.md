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

## Fix Specification

**Defense-in-depth across both layers**, lifting the prior-art pattern from bootstrap step 9 (`CleanStaleMarkers`) verbatim. Three coordinated changes:

### Change 1 — Repurpose `ListAllPanes` to Wrap the Error-Propagating Helper

**File:** `internal/tmux/tmux.go` (~line 687-693).

Replace the current swallow body so that `ListAllPanes` becomes a thin wrapper around `ListAllPanesWithFormat` plus a shared `parseLivePaneSet` utility. Approximate shape:

```go
func (c *Client) ListAllPanes() ([]string, error) {
    raw, err := c.ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")
    if err != nil {
        return nil, err
    }
    set := parseLivePaneSet(raw, /* logger */ nil)
    keys := make([]string, 0, len(set))
    for k := range set {
        keys = append(keys, k)
    }
    return keys, nil
}
```

**Disposition rationale (locked):** repurpose, not delete or deprecate.
- Deletion forces every call site (production and test) to be touched in this work unit; high blast radius for a contract-narrowing change.
- Deprecation with `// Deprecated:` keeps the footgun alive — the compiler does not enforce the tag.
- Repurpose structurally eliminates the swallow contract while keeping every existing call site compiling unchanged. New consumers inherit the safe behaviour by default.

### Change 2 — Promote `parseLivePaneSet` to a Shared Utility

The `session:window.pane` parser currently lives in `cmd/bootstrap/stale_marker_cleanup.go`. It must be promoted to a shared location (likely `internal/tmux` or a new helper next to it) so all three consumers — the new `ListAllPanes`, `CleanStaleMarkers`, and the two `CleanStale` callsites — use one parser.

Existing `parseLivePaneSet` test coverage in `cmd/bootstrap/stale_marker_cleanup_test.go` must move or be duplicated for the new location.

### Change 3 — Add Mass-Deletion Hazard Guard at Both `CleanStale` Callsites

**Files:**
- `cmd/bootstrap_production.go:76-83` — `cleanStaleAdapter.CleanStale`.
- `cmd/clean.go:75-91` — `portal clean` hook-cleanup tail.

Before passing the live-pane slice to `hooks.Store.CleanStale`, check the combination `len(livePanes) == 0 && len(persistedHooks) > 0`. When that combination holds:

1. Emit `Logger.Warn(ComponentBootstrap, ...)` with both counts.
2. Skip the destructive call.
3. Return nil (next bootstrap retries).

This mirrors the prior-art at `cmd/bootstrap/stale_marker_cleanup.go:126-141` exactly.

**Logger plumbing:** `cleanStaleAdapter` currently has no `Logger` field. It must gain one, populated from the orchestrator-scope logger using the same field-population pattern at `cmd/bootstrap_production.go:147-152` where `MarkerCleanupCore` already receives one. The orchestrator-scope logger is resolved at lines 109-110 of the same file.

The `portal clean` subcommand must apply the same hazard guard at `cmd/clean.go:75-91`.

### Change 4 — Add Adapter-Level Logging

At both `CleanStale` callsites, emit:

- **`Debug` on entry** — live count, persisted count, what would be removed.
- **`Warn` when the hazard guard fires** — both counts (covers mode (b)).
- **`Debug` on the normal-path completion** — count removed.
- **`Warn` on propagated error from `ListAllPanesWithFormat`** — surfaces mode (a) with the wrapped error message.

**Post-fix log distinguishability:** failure modes (a) (exit ≠ 0) and (b) (exit 0 with empty stdout) become distinguishable in `portal.log` — mode (a) surfaces as the propagated-error `Warn`; mode (b) surfaces as the hazard-guard `Warn`. Currently both modes are silent at the adapter.

### Bootstrap Posture Preserved

Bootstrap step 11 remains **best-effort** under the orchestrator's "never abort `PersistentPreRunE`" rule. The propagated error from `ListAllPanesWithFormat` and the hazard-guard skip both manifest as soft warnings, not fatal aborts. The change is "treat empty as unknown, log, and continue," not "fail loudly on empty."

### Closing Both Failure Modes

| Failure mode | Closed by |
|---|---|
| (a) `list-panes -a` exit ≠ 0 | Change 1 (helper propagates error → adapter returns it as soft warning) |
| (b) `list-panes -a` exit 0 with empty stdout | Change 3 (hazard guard refuses wipe when `len(live)==0 && len(persisted)>0`) |

Either change alone leaves one failure mode open. Both are required.

---

## Working Notes
