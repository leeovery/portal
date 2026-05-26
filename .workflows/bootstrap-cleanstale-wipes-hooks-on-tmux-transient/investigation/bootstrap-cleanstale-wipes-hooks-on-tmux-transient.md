# Investigation: Bootstrap CleanStale Wipes Hooks on Tmux Transient

## Symptoms

### Problem Description

**Expected behavior:**
Bootstrap step 11 (`CleanStale`) should preserve `hooks.json` entries when tmux is in a transient state where `list-panes -a` returns a non-zero exit code. A transient tmux failure should be treated as "I don't know the live pane set right now — skip cleanup, log, continue," not "the live pane set is empty, therefore every persisted hook entry is stale and should be removed." The same posture is already in force for Component B's orphan-sweep, which logs and skips on a transient `list-panes` failure (`sweep: list-panes _portal-saver failed, legitimate set empty: ...`).

**Actual behavior:**
On 2026-05-26 at ~17:04 BST, `~/.config/portal/hooks.json` went from 22 valid per-pane on-resume hook entries (registered earlier that day by `~/.claude/hooks/portal-resume-backfill.sh --apply`, covering every live Claude session in every active tmux pane) to `{}` empty. No user action that should have produced that outcome occurred:

- No `portal hooks rm` invocations.
- No `portal clean` invocations.
- No Claude `SessionEnd` events firing the removal branch of `~/.claude/hooks/portal-resume-hook.sh` — `/tmp/portal-resume-hook.log` only shows CodexBar probe invocations that exited early at the `TMUX_PANE unset` guard.
- The user-level SessionEnd branch had already been softened earlier the same day so it no longer calls `portal hooks rm` even if it fires.

The wipe correlated tightly in time with a Portal bootstrap that left a single WARN entry in `~/.config/portal/state/portal.log` at `17:11:25Z`:

```
sweep: list-panes _portal-saver failed, legitimate set empty: list-panes -t _portal-saver: exit status 1: can't find window: _portal-saver
```

That WARN is Component B's orphan-sweep noting a transient tmux state where `_portal-saver` was momentarily absent. Component B logged and continued (its documented best-effort posture). The same bootstrap cycle also ran step 11 (`CleanStale`), which appears to have hit a related transient failure when enumerating live panes — and unlike Component B, `CleanStale` did not log-and-skip. It interpreted the transient as "zero live panes exist" and proceeded to remove every hook entry whose key was not in the empty live set — i.e. all 22 of them.

### Manifestation

- Silent data destruction: `hooks.json` content goes from 22 entries to `{}` with no error returned to the user, no WARN in portal.log specifically for the hook wipe, and no UI signal during the bootstrap that ran the cleanup.
- User-visible follow-on impact: at next reboot, no Claude session auto-resumes because every per-pane on-resume hook entry is gone. The user has to manually identify and resume each Claude session via Claude's own session picker — exactly the scenario the Portal resume system was built to prevent.
- Indistinguishable user-symptom from the earlier `slow-open-empty-previews-and-zombie-sessions` bug ("none of my Claude sessions resumed"), even though the upstream cause is entirely different — that bug was about FIFO timing during restore; this one is the cleanup-after-restore step destroying state during a tmux transient.

### Reproduction Steps

1. Have a populated `~/.config/portal/hooks.json` (multiple per-pane `on-resume` entries).
2. Run any Portal-bootstrap-triggering command (`portal open`, `x`, `portal hooks set`, `portal hooks rm`, etc.) while the tmux server is in a transient state where `tmux list-panes -a` returns a non-zero exit code OR returns an empty list.
3. Observe `hooks.json` becomes `{}` empty.

**Reproducibility:** Intermittent in the field (depends on hitting the tmux transient window). Should be deterministic in a unit test by injecting a `Commander` that returns `exit 1` or empty stdout for the `list-panes -a` call.

### Environment

- **Affected environments:** Any environment running `portal` against a live tmux server. Observed on macOS / tmux 3.6b.
- **Trigger windows of highest empirical risk:**
  - Saver-respawn window after the zombie-sessions bugfix triggers kill-and-recreate of `_portal-saver`.
  - Version-upgrade kill cycle in `EnsurePortalSaverVersion`.
  - Any time the tmux server is under heavy load.
  - The same race that produced the observed `sweep: list-panes _portal-saver failed` WARN.
- **User conditions:** Anyone using `portal hooks set --on-resume` to persist per-pane Claude-resume hooks. Severity escalates with the number of registered hooks (one bootstrap can wipe all of them).

### Impact

- **Severity:** High — silent destruction of user-persisted configuration that the user did not request and cannot detect until the next reboot.
- **Scope:** Every user of the Portal hooks subsystem who experiences a tmux transient during a bootstrap-triggering command. Empirically, this includes anyone running on macOS with `_portal-saver` enabled.
- **Business impact:** The Portal resume system's reliability promise is undermined. Recent same-day mitigations (softening user-level SessionEnd to a no-op, rebuilding the backfill script) are fully nullified by this defect — every bootstrap is a fresh opportunity to silently empty `hooks.json`.

### References

- portal.log entry: `~/.config/portal/state/portal.log` line 1 (`sweep: list-panes _portal-saver failed, legitimate set empty: ...` at `17:11:25Z` 2026-05-26). Previous log content was rotated during one of today's bootstrap cycles.
- Recovery one-shot: `~/.claude/hooks/portal-resume-backfill.sh --apply` walks every live Claude session inside tmux and re-registers a hook for each — not durable until this bug is fixed.
- Related memory: [[project_reboot_hooks_followup]] flagged re-verifying reboot Claude-resume hook firing after the slow-open-empty-previews bugfix shipped.

### Initial File Pointers

- `cmd/bootstrap_production.go` — `cleanStaleAdapter.CleanStale` (~line 76-83). No adapter-level logging of the call or the live-panes count.
- `cmd/clean.go` — `portal clean` subcommand (~line 82), same destructive pattern via the same `ListAllPanes` path.
- `internal/tmux/tmux.go` — `ListAllPanes` (~line 687). Collapses `list-panes -a` exit ≠ 0 into `([]string{}, nil)` — indistinguishable from a legitimate empty-pane reply.
- `internal/hooks/store.go` — `Store.CleanStale` (~line 130).

### Observation Boundary

Direct observation extends only to:

1. Component B logged `list-panes -t _portal-saver: exit status 1: can't find window: _portal-saver` at `19:48:39Z`.
2. `hooks.json` went from 23 → 1 entries.
3. No log entry from `cleanStaleAdapter.CleanStale` itself.

From those facts we infer `hookStore.CleanStale` must have been called with an empty (or near-empty) `livePanes` slice. We **cannot** tell from logs whether the upstream `list-panes -a` call returned exit ≠ 0 (which `ListAllPanes` collapses to `([]string{}, nil)`) or returned exit 0 with empty stdout. Both paths produce the same destructive end-state. Component B's WARN is a different list-panes call (`-t _portal-saver`, not `-a`), so it confirms tmux was transient at that moment but does not pin down step 11's exact behavior.

### Failure Modes That Must Be Covered

Both failure modes are plausible and produce the same destructive end-state:

- **(a) `list-panes -a` exit ≠ 0** — collapsed to `([]string{}, nil)` inside `ListAllPanes`. The error is swallowed; the caller cannot distinguish "tmux is transient" from "tmux has no panes."
- **(b) `list-panes -a` exit 0 with empty stdout** — possible during a `_portal-saver` mid-respawn transient (Component F's placeholder-then-respawn ordering reduces but doesn't eliminate this window). Tmux can momentarily reply "no panes" on `-a` with exit 0 while the saver is being recreated.

A fix that propagates the error from `ListAllPanes` alone closes (a) but a real exit-0-with-empty-stdout reply from tmux during a saver-respawn transient (b) would still bypass the error guard.

### Defensive Sanity Gate Worth Considering

Cross-check `ListSessionNames` (or `tmux has-session`) returning non-empty at the adapter layer. If tmux says it has live sessions but `list-panes -a` returns zero panes, that's incoherent and the wipe should be refused. Bounds the blast radius cheaply but is **defense-in-depth, not the root-cause fix.**

### Audit Scope

All other callers of `ListAllPanes` must be audited as part of the investigation. `portal clean` uses the same path at `cmd/clean.go:82`. Any other consumer that interprets `ListAllPanes → empty` as ground truth has the same defect latent.

### Relationship to Recent Releases

The fix in v0.5.11 (`hooks-skip-bootstrap`) reduces trigger frequency by eliminating the `SessionStart` cascade but does **not** change the latency of this bug. `portal open` / `x` / attach during a tmux transient can still wipe everything.

---

## Analysis

(pending)

---

## Fix Direction

(pending)

---

## Notes

- Not a regression from any recent change. The destructive pattern has existed since `CleanStale` was wired to the live-pane enumeration. Recent zombie-sessions and hydrate-command-shell-safety releases increase exposure because they involve more bootstrap activity during which tmux transients become more frequent.
- Component B (orphan-sweep) already implements the "log and skip on transient" posture this bug is asking for elsewhere — Component B's behavior is the prior-art reference.
