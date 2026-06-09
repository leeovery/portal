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

**The two buggy sites (confirmed in current source):**

- `internal/tmux/tmux.go:352-358` — `KillSession(name)`:
  ```go
  _, err := c.cmd.Run("kill-session", "-t", name)
  ```
  Bare `-t name` — no `=` prefix. No godoc rationale block (unlike the fixed
  sites).
- `internal/tmux/tmux.go:360-367` — `RenameSession(oldName, newName)`:
  ```go
  _, err := c.cmd.Run("rename-session", "-t", oldName, newName)
  ```
  Bare `-t oldName` — no `=` prefix. **Only `oldName` (the `-t` target) needs
  the prefix; `newName` is the literal new-name positional argument and must
  stay bare** (it is not a target lookup).

**The already-fixed sites (carry `=`, with rationale godoc) — for contrast:**

- `HasSession` (136) / `HasSessionProbe` (166): `has-session -t =name`
- `SwitchClient` (378): `switch-client -t =name`
- `SelectWindow` (936-937): `select-window -t =session:window`
- `SelectPane` (955-957): `select-pane -t` via `PaneTargetExact`
- `ResizePaneZoom` (974-976): via `PaneTargetExact`
- `PaneTargetExact` (546): the `=`-prefixed pane-target formatter; `PaneTarget`
  (530) is the deliberately non-prefixed hooks.json key formatter (out of scope).

**The fix chokepoint is the Client method, not the callers.** Both methods are
the single argv-construction point. Fixing the argv inside `KillSession` /
`RenameSession` covers every caller uniformly — no caller-side change needed.

**Callers (blast radius of the destructive paths):**

- `KillSession`:
  - `cmd/kill.go:37` — `portal kill <name>` (user session, by name) — **exposed**
  - `internal/tui/model.go:2171` — TUI kill key (user session) — **exposed**
  - `cmd/state_cleanup.go:185`, `internal/tmux/portal_saver.go:366,372,385` —
    kill the fixed-name `_portal-saver` internal session. Low collision risk
    (fixed literal name) but the method-level fix covers them harmlessly and
    uniformly.
- `RenameSession`:
  - `internal/tui/model.go:2225` — TUI rename (user session) — **exposed**

### Root Cause

`KillSession` and `RenameSession` construct their tmux `-t <session>` argv
**without** tmux's `=` exact-match prefix. tmux's default target resolution is
**prefix-match**, so when the named session is not a live exact match but
another live session has that name as a prefix (e.g. target `foo` with live
`foo-2`), tmux silently resolves to the prefix-matching session — killing or
renaming the wrong one.

**Why this happens:** the `=` exact-match policy was introduced by the
`enter-attaches-from-preview` work, which applied it only to the five sites in
*its* blast radius (the preview Enter / pre-select-and-attach pipeline). The
policy lives as an inline `"="+name` string repeated per call site, with no
centralising helper — so a site outside that pipeline (these two destructive
callers) was never brought under the policy and there is no single point that
enforces it.

### Contributing Factors

- **Inline-string policy, no chokepoint.** The `=` prefix is hand-applied at
  each call site rather than centralised, so adding a new `-t` caller (or
  leaving an old one) silently opts out of the policy.
- **Sibling-work scope boundary.** `enter-attaches-from-preview` deliberately
  fixed only the sites it touched; the two destructive callers sit outside that
  pipeline and were left bare (later surfaced in that work's own review →
  inbox bug → this work unit).
- **Session names are `{project}-{nanoid}` and freely renamed by the user**, so
  prefix collisions between coexisting sessions are plausible in practice, not
  theoretical.

### Why It Wasn't Caught

**The existing unit tests actively pinned the buggy form.**

- `TestKillSession` (`tmux_test.go:737`): `wantArgs := "kill-session -t my-session"`
- `TestRenameSession` (`tmux_test.go:953`): `wantArgs := "rename-session -t old-name new-name"`

Both assert the bare-`-t` argv, so they *locked in* the bug — a fix must
**update** these existing assertions, not merely add new ones. Contrast with
`TestHasSessionUsesExactMatchPrefix` (`tmux_test.go:443`), the regression test
the sibling work added that pins `=foo` and even simulates tmux's prefix-match
semantics ("killed foo with live foo-2 reports absent"). `TestSwitchClient`
(`tmux_test.go:770`) already expects `switch-client -t =my-session`.

The sibling work (`enter-attaches-from-preview`) fixed the five sites in its
own blast radius (the preview Enter / pre-select-and-attach pipeline) and did
not sweep the two destructive callers, which sit outside that pipeline.

### Blast Radius

**Directly affected (the fix):**
- `tmux.go` `KillSession`, `RenameSession`.
- `tmux_test.go` `TestKillSession`, `TestRenameSession` (assertions updated).

**Same hazard class but OUT OF SCOPE per seed** — other bare `-t <session>`
sites in `tmux.go` that also lack the `=` prefix. None are destructive (reads
or option/env sets), and the seed scoped only the two destructive callers.
They inform the helper-vs-minimal scope question (a centralising `exactTarget`
helper could eventually cover them to prevent drift) but are not part of this
bugfix:
- `ActivePaneCurrentPath` (344): `display-message -t session` (read)
- `SetSessionOption` (399): `set-option -t session`
- `ListPanesInSession` (555), and the other `list-panes -t session` reads
  (631, 686)
- `ShowEnvironment` (712): `show-environment -t session` (read)
- `SetSessionEnvironment` (898): `set-environment -t session`

Explicitly out of scope (per seed): `PaneTarget` (530) — the no-prefix
hooks.json key formatter; changing it would invalidate existing hook entries.

---

## Fix Direction

### Core fix (settled)

Apply tmux's `=` exact-match prefix to the `-t` target in both methods:

- `KillSession`: `kill-session -t =<name>`
- `RenameSession`: `rename-session -t =<oldName> <newName>` (prefix on the
  target only; `newName` stays bare).

Add the rationale godoc block to each (mirroring `HasSession` / `SwitchClient`),
referencing the spec § Exact-match target syntax. Update the two existing tests
(`TestKillSession`, `TestRenameSession`) to expect the `=`-prefixed argv, and
add prefix-collision regression coverage mirroring
`TestHasSessionUsesExactMatchPrefix`.

### Open scope question (for findings review)

How to apply the prefix — two options:

1. **Minimal two-caller patch.** Inline `"="+name` at the two sites, matching
   the existing inline style at the five fixed sites. Smallest diff; consistent
   with the current codebase pattern. Leaves the policy as a repeated inline
   string (the same drift surface that allowed this bug).
2. **Centralising `exactTarget` helper first** (the seed's suggestion). Add
   `func exactTarget(session string) string { return "=" + session }`, use it in
   the two callers, and optionally migrate the existing inline-`=` sites onto it
   to make the policy a single chokepoint and prevent future drift.

The seed leans toward option 2 (helper first, optional migration of existing
sites). Either way, `PaneTarget` stays out of scope and the other bare
`-t <session>` sites (reads / option-sets) are out of scope for this bugfix.

### Testing Recommendations

- Update `TestKillSession` → expect `kill-session -t =my-session`.
- Update `TestRenameSession` → expect `rename-session -t =old-name new-name`.
- Add prefix-collision regression tests for both (mirror
  `TestHasSessionUsesExactMatchPrefix`: simulate tmux exact-match semantics so a
  bare-`-t` regression fails loudly).
- If a helper is introduced: a focused test that `exactTarget("foo") == "=foo"`.

### Risk Assessment

- **Fix complexity:** Low (argv string change + test updates; optional small
  helper).
- **Regression risk:** Low. The `_portal-saver` internal callers gain the `=`
  prefix harmlessly (fixed literal name). No caller-side changes. Option 2's
  optional migration of existing sites is behaviour-neutral (same argv) but
  widens the diff.
- **Recommended approach:** Regular release (no hotfix infra in this project).

---

## Notes

- Out of scope per seed: `PaneTarget` (the no-prefix hooks.json key formatter)
  must stay as-is — changing it would silently invalidate existing hook entries.
- Open scope question carried from discovery: introduce a centralising
  `exactTarget` helper (and optionally migrate existing inline-`=` sites onto
  it to prevent drift) vs. a minimal two-caller patch.
