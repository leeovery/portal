# Specification: Kill-Rename Prefix Collision

## Specification

## Problem & Root Cause

### The bug

`internal/tmux/tmux.go` exposes two session-mutating Client methods that build their tmux `-t <session>` target argv **without** tmux's `=` exact-match prefix:

- `KillSession(name)` → `kill-session -t <name>`
- `RenameSession(oldName, newName)` → `rename-session -t <oldName> <newName>`

tmux's default target resolution is **prefix-match**. When the named session is not a live exact match but another live session has that name as a prefix (e.g. target `foo` with a live `foo-2`), tmux silently resolves `-t foo` to `foo-2` — operating on the wrong session.

The **kill path is the dangerous one**: destructive, no undo, and silent on a wrong-session kill. `KillSession("foo")` can destroy `foo-2`'s session and surface no error. The rename path is less severe (recoverable) but still incorrect.

### Why it happens

The `=` exact-match policy was introduced by the sibling `enter-attaches-from-preview` work, which applied it only to the five sites in *its* blast radius (the preview Enter / pre-select-and-attach pipeline). The policy lives as an inline `"="+name` string repeated per call site, with **no centralising helper** — so these two destructive callers, sitting outside that pipeline, were never brought under the policy, and there is no single point that enforces it.

Contributing factors:
- **Inline-string policy, no chokepoint** — the prefix is hand-applied per call site, so any `-t` caller can silently opt out.
- **Session names are `{project}-{nanoid}` and freely renamed by the user**, so prefix collisions between coexisting sessions are plausible in practice, not theoretical.

### Why it wasn't caught

The existing unit tests actively **pinned the buggy form**:
- `TestKillSession` (`tmux_test.go:737`) asserts `kill-session -t my-session`
- `TestRenameSession` (`tmux_test.go:953`) asserts `rename-session -t old-name new-name`

Both lock in the bare-`-t` argv, so the fix must **update** these assertions, not merely add new ones.

### Impact

- **Severity:** High — `KillSession` is destructive with no undo and fails silently on the wrong session.
- **Scope:** Any portal user with prefix-colliding session names.
- **Business impact:** Data/work loss (a wrong tmux session killed).

## Required Behaviour & The Fix

### Required behaviour

`KillSession(name)` and `RenameSession(oldName, newName)` must operate on **exactly** the named session. The `-t` target must use tmux's `=` exact-match prefix so it binds only to the literal session name and never prefix-matches a different session.

### Chosen approach

**Centralising helper + uniform migration of session-level sites** (the investigation's Option 2, executed for a uniform end-state). Fix the two destructive callers *and* close the inline-string drift surface for session targets, so the codebase reads as if the gap was never there.

**Deciding factor:** the user's explicit steer — "do it properly, not a hack… clean, as if it was never there." A minimal two-caller patch would leave a mixed state (helper for two callers, inline `"="+name` for the rest), itself a new inconsistency. The uniform migration is behaviour-neutral (identical argv) and removes the exact drift surface that allowed the bug, without widening into unrelated sites.

### 1. Introduce the `exactTarget` session-level primitive

In `internal/tmux`:

```go
func exactTarget(session string) string { return "=" + session }
```

This is the session-level sibling of the existing `PaneTargetExact` (pane-level). Together they become the two canonical ways to build an exact-match `-t` target — no inline `"="+name` for a session name left anywhere in `tmux.go`.

### 2. Fix the two destructive callers (the actual bug)

Each via the helper, each with a rationale godoc block mirroring the already-fixed sites:

- `KillSession`: `kill-session -t exactTarget(name)`
- `RenameSession`: `rename-session -t exactTarget(oldName) <newName>`

**Edge case (the one implementer trap):** in `RenameSession`, the prefix goes on the **target only**. `newName` is the literal positional new-name argument and **must stay bare** — prefixing it would corrupt the new session name (the session would literally be named `=...`).

The fix lives entirely at the Client-method chokepoint. Both methods are the single argv-construction point, so fixing the argv inside them covers every caller uniformly — **no caller-side change anywhere**. This includes the internal `_portal-saver` `KillSession` callers (`cmd/state_cleanup.go`, `internal/tmux/portal_saver.go`), which gain the `=` prefix harmlessly (fixed literal name, no possible prefix collision).

---

## Working Notes

[Optional - capture in-progress discussion if needed]
