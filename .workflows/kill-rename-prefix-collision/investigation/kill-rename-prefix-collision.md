# Investigation: Kill / Rename Prefix Collision

## Symptoms

### Problem Description

**Expected behavior:**
`KillSession(name)` and `RenameSession(oldName, newName)` must operate on
**exactly** the named session. tmux's `=` exact-match target prefix should be
used so `-t =<name>` binds only to the literal session name and never
prefix-matches a different session.

**Actual behavior:**
- `KillSession(name)` issues `kill-session -t <name>` with a bare `-t` argv.
- `RenameSession(oldName, newName)` issues `rename-session -t <oldName>` with a
  bare `-t` argv.

tmux's default target resolution is **prefix-match**. With a live `foo-2`
coexisting with a killed-then-recreated `foo`, `-t foo` can silently bind to
`foo-2` — killing or renaming the wrong session. The kill path is the dangerous
one: destructive, no undo, and silent on a wrong-session kill.

### Manifestation

- Silent wrong-session kill: `KillSession("foo")` can destroy `foo-2`'s tmux
  session if `foo` is not an exact live match but `foo-2` prefix-matches `foo`.
  No error surfaces.
- Wrong-session rename: `RenameSession("foo", ...)` can rename `foo-2`. Less
  severe (recoverable) but still incorrect.

### Reproduction Steps

1. Have two sessions whose names share a prefix, e.g. `foo` and `foo-2`.
2. Kill the exact-match session `foo` (so only `foo-2` remains, but a caller
   still references `foo`), OR have a state where `foo` no longer exists as an
   exact match.
3. Call `KillSession("foo")` → tmux prefix-matches `foo-2` and kills it.

**Reproducibility:** Deterministic given the prefix-collision precondition
(tmux default target resolution is prefix-match).

### Environment

- **Affected environments:** All — this is core CLI behaviour in
  `internal/tmux/tmux.go`.
- **Browser/platform:** n/a (tmux 3.6b confirmed in shaping).
- **User conditions:** Any state where a target session name is a prefix of
  another live session name.

### Impact

- **Severity:** High — `KillSession` is destructive with no undo and fails
  silently on the wrong session.
- **Scope:** Any portal user who has prefix-colliding session names (session
  names are `{project}-{nanoid}`, so prefix collisions across projects sharing
  a name stem are plausible).
- **Business impact:** Data/work loss (a wrong tmux session killed).

### References

- Seed: `.workflows/kill-rename-prefix-collision/seeds/2026-05-16-kill-rename-prefix-collision.md`
- Sibling work: `enter-attaches-from-preview` (fixed 5 sites with the `=` prefix).
- Discovery: `.workflows/kill-rename-prefix-collision/discovery/session-001.md`

---

## Analysis

### Initial Hypotheses

`KillSession` and `RenameSession` build a bare `-t <name>` argv, missing the `=`
exact-match prefix that the five sites fixed in `enter-attaches-from-preview`
already carry. Root cause is believed to already be located; the investigation
must confirm it in the current source and assess blast radius / scope.

### Code Trace

(to be filled during code analysis)

### Root Cause

(to be filled during synthesis)

### Contributing Factors

(to be filled)

### Why It Wasn't Caught

(to be filled)

### Blast Radius

(to be filled)

---

## Fix Direction

(to be filled during findings review)

---

## Notes

- Out of scope per seed: `PaneTarget` (the no-prefix hooks.json key formatter)
  must stay as-is — changing it would silently invalidate existing hook entries.
- Open scope question carried from discovery: introduce a centralising
  `exactTarget` helper (and optionally migrate existing inline-`=` sites onto
  it to prevent drift) vs. a minimal two-caller patch.
