---
status: complete
created: 2026-07-01
cycle: 3
phase: Gap Analysis
topic: session-rename-orphans-resume-hook
---

# Review Tracking: session-rename-orphans-resume-hook - Gap Analysis

## Findings

### 1. "Retire the stale doc-comments" deliverable omits the `StructuralKeyFormat` and `ListAllPanes` comments, which also assert hooks.json ownership

**Source**: Specification analysis (Hook-Key Derivation → Decoupling); verified against `internal/tmux/tmux.go`
**Category**: Gap/Ambiguity
**Affects**: Hook-Key Derivation → "Deliverable — retire the stale doc-comments" (spec line 52); consistency with Stage 2 (spec line 64)

**Details**:
The deliverable at spec line 52 scopes the stale-comment cleanup to `PaneTarget`/`PaneTargetExact` only (`tmux.go:551-558`, `572-573`). But two *other* in-source doc-comments in the same file assert the exact same (now-false) claim — that a name-based structural format IS the `hooks.json` lookup key — and the fix invalidates them too:

- **`StructuralKeyFormat`** (`tmux.go:771-779`): "the load-bearing join key between live-pane enumeration ... persisted hook entries in hooks.json, and @portal-skeleton-* marker names. Every tmux call whose output is consumed as a structural key MUST request exactly this format so ... the hook lookup table all agree on what constitutes a paneKey. Drift here would silently desync the cleanup paths' interpretation..."
- **`ListAllPanes`** (`tmux.go:781-798`): "Keys have the form ... the same format produced by (*Client).ResolveStructuralKey and used as the lookup key in hooks.json, so callers can intersect the returned slice with persisted hook entries directly." and "Treating a tmux failure as 'no live panes' silently elides every entry that depends on the live set (notably hooks.json) ..."

Per the spec's own Stage 2 (line 64), after the fix the hook-cleanup enumeration switches OFF `ListAllPanes()`/`StructuralKeyFormat` and onto `HookKeyFormat`; those primitives "remain available for any non-hook structural use" only. So the two comments above become false in exactly the way the deliverable is meant to prevent: they still steer a reader/future caller toward name-based keying as the hooks.json key. This is precisely the trap the deliverable names — "Leaving the old comments in place would invite a future caller back into name-based keying — re-establishing the exact drift this fix removes."

A related sub-point: `ResolveStructuralKey` (`tmux.go:320-329`) is what Stage 1 (line 59) replaces with `ResolveHookKey` for hook registration; the `ListAllPanes` comment (line 784) references `ResolveStructuralKey` as the producer of "the lookup key in hooks.json," compounding the same false assertion.

Why it matters for planning: an implementer following the deliverable literally would update only 2 of the (at least) 4 stale hooks.json-ownership comments, leaving two authoritative-sounding "this IS the hooks.json key / drift silently invalidates hooks.json" assertions on the very primitives the fix is steering hook callers away from. That defeats the stated purpose of the deliverable (prevent future name-based-keying regressions) and would force the implementer to either notice the omission independently or leave the drift trap half-open. The deliverable should enumerate all comment sites whose hooks.json-ownership language must move to `HookKey`/`HookKeyFormat` (or be reworded to "structural/target use only, NOT the hook key"), not just the `PaneTarget` pair.

**Current**:
> **Deliverable — retire the stale doc-comments.** `PaneTarget`/`PaneTargetExact` today carry in-source doc-comments (`tmux.go:551-558`, `572-573`) asserting `PaneTarget` *is* the canonical `hooks.json` key formatter and that its format must never change or it orphans `hooks.json`. After the fix those comments are false and must be updated: the canonical hook-key formatter is now `HookKey` / `HookKeyFormat`, and the load-bearing "format is stable across releases — changing it silently invalidates every `hooks.json` entry" invariant **transfers to those new primitives**, it does not disappear. Leaving the old comments in place would invite a future caller back into name-based keying — re-establishing the exact drift this fix removes.

**Proposed Addition**:
_Leave blank until discussed._ (Direction: broaden the deliverable to enumerate every in-source doc-comment that currently claims a name-based format is the hooks.json lookup key — at minimum `PaneTarget`/`PaneTargetExact` (`tmux.go:551-558`, `572-573`), `StructuralKeyFormat` (`tmux.go:771-779`), and `ListAllPanes` (`tmux.go:781-798`, incl. its `ResolveStructuralKey` reference at line 784). For the `StructuralKeyFormat`/`ListAllPanes` pair the correct rewording is not "the invariant transfers" but "these remain the name-based structural/target formatters for non-hook use; they are NO LONGER the hooks.json key — see `HookKey`/`HookKeyFormat`," matching Stage 2's "remain available for any non-hook structural use.")

**Resolution**: Approved
**Notes**: Approved via auto mode. The 'retire the stale doc-comments' deliverable now enumerates all four sites: PaneTarget (551-558), PaneTargetExact (572-573), StructuralKeyFormat (771-779), and ListAllPanes (781-798); notes they remain valid for name-based targeting but must stop claiming hooks.json ownership.

---
