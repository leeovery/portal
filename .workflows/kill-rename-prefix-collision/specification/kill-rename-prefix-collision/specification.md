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

---

## Working Notes

[Optional - capture in-progress discussion if needed]
