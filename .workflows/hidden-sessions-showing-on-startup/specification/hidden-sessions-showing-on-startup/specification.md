# Specification: Hidden Sessions Showing On Startup

## Specification

## Problem Statement

### Symptoms

After fresh tmux/Portal startup, two sessions appear in the Portal session
list that should be hidden by default:

1. **`0`** — an unnamed bootstrap session created by Portal's own
   `StartServer` (a side-effect of `tmux new-session -d`).
2. **`_portal-saver`** — Portal's internal saver session that hosts
   `portal state daemon`.

Both surface in:

- The TUI session picker (`internal/tui`).
- `portal list` CLI output (`cmd/list.go`).

### Reproduction

1. Start tmux fresh (no existing server) or kill the tmux server.
2. Run `portal` (or `portal open` / `x`).
3. Open the TUI session picker (or run `portal list`).
4. Both `0` and `_portal-saver` are visible.

Reproducibility: always.

### Scope

- Severity: low (cosmetic / UX clutter, no data loss).
- Affected: all Portal users on all platforms.
- Confirmed independent of `tmux-resurrect` / `tmux-continuum` —
  Portal itself is the source of both unwanted sessions.

### Design Intent

The `built-in-session-resurrection` specification mandates that Portal
filters sessions whose names begin with `_` from:

- The TUI session picker.
- `sessions.json` capture.
- Any future internal-only sessions.

The capture-side filter shipped. The TUI / `portal list` filter did
not. The bootstrap session was never given an underscore-prefixed name,
so even with the spec-mandated filter in place it would still leak.

## Root Causes

Two distinct, related root causes — both in production code.

### Root Cause 1 — TUI / `portal list` Skip Underscore Filter

The `built-in-session-resurrection` spec mandates filtering of `_*`
sessions at three sites. Implementation landed at two:

| Site | Status |
|------|--------|
| `internal/state/capture.go` (sessions.json capture) | filtered ✓ |
| `internal/restore/restore.go` (defensive restore-side skip) | filtered ✓ |
| `internal/tmux/tmux.go` (`Client.ListSessions`) | **not filtered ✗** |
| `internal/tui/model.go` (`filteredSessions`) | **not filtered ✗** |
| `cmd/list.go` (`portal list`) | **not filtered ✗** |

`Client.ListSessions` returns every running session as tmux reports
it. `filteredSessions` only excludes the current session when inside
tmux. `cmd/list.go` prints `ListSessions` output verbatim. The
doc-comment on `tmux.PortalSaverName` documents an invariant the code
does not enforce.

The `built-in-session-resurrection` planning phase authored a
capture-side filtering task (`2-8`) but no equivalent task for the
TUI picker or `portal list`. The traceability check between
specification and plan did not catch the gap.

### Root Cause 2 — Bootstrap `0` Session Never Cleaned Up

`internal/tmux/tmux.go` `StartServer` runs `tmux new-session -d`
without `-s`, so tmux assigns the default name (lowest unused numeric
index from `base-index`, typically `0`). The change was deliberate
(commit `bd659a3`) — replacing `tmux start-server` with
`tmux new-session -d` keeps the server alive long enough that
`exit-empty on` does not kill it before restoration completes.

The original rationale relied on `tmux-resurrect` recognising and
cleaning up the `0` session. With Portal's own session resurrection
now authoritative, that delegation is obsolete and never runs.
Bootstrap step 5 (`Restore`) does not target the `0` session, and no
other bootstrap step or cleanup mechanism removes it. The session
lingers indefinitely.

### Why The Two Causes Surface Together

Both unwanted sessions appear at first startup because both are
products of the same bootstrap sequence. Each has a separate fix,
but both fixes ride on the same mechanism — once Portal filters
`_*` sessions at the chokepoint, both `_portal-saver` and the
renamed bootstrap session disappear from view together.

### Why It Wasn't Caught Earlier

- Tests for `internal/state/capture.go` cover the underscore-prefix
  filter. No equivalent test exists for the TUI session list,
  `cmd/list.go`, or `Client.ListSessions`. Test surface drove what
  got implemented.
- Tests for `StartServer` check that the command runs; they do not
  assert on whether the bootstrap session is cleaned up at the end
  of bootstrap. The `0` session is invisible to current assertions.

## Fix A — Filter `_*` Sessions In `Client.ListSessions`

### Behaviour Contract

`internal/tmux.Client.ListSessions` MUST exclude any session whose
name begins with `_` from its returned slice. The exclusion is
unconditional and applies to every caller.

After this change, the TUI session picker, `portal list`, and any
future caller of `ListSessions` automatically inherit the filter
without per-consumer code changes.

### Placement Rationale

The filter is applied at the chokepoint — `tmux.Client.ListSessions` —
rather than at each consumer (`internal/tui`, `cmd/list.go`).
Rationale:

- The spec describes `_*` hiding as a Portal-wide invariant, not a
  per-consumer concern.
- Single source of truth — future consumers cannot forget the rule.
- One filter site, one test, one mental model.

A per-consumer placement (each of `internal/tui` and `cmd/list.go`)
was rejected because it loses the invariant — the next consumer to
appear would forget, and the bug would repeat.

### Interaction With The Capture Path

`internal/state/capture.go` does not call `ListSessions`. It uses a
separate `ListSessionNames` route and applies its own
`keepSessionNames` filter (lines 218-228). Adding a filter inside
`ListSessions` does not affect capture behaviour.

If `ListSessionNames` is implemented as a thin wrapper around
`ListSessions` and the new filter would change its output, the
implementation MUST preserve current behaviour for the capture path.
Two acceptable implementations:

1. Apply the underscore filter at the post-processing layer in
   `ListSessions`, and have `ListSessionNames` call the lower-level
   raw enumeration directly (bypassing the filter), OR
2. Apply the filter only in `ListSessions` because the capture path
   already filters `_*` sessions on top via `keepSessionNames` —
   double-filtering produces the same result.

The implementation chooses one and documents which.

### Diagnostic Escape Hatch (Future)

If a future caller legitimately needs unfiltered output (e.g. an
internal diagnostic command), expose a sibling `ListAllSessions` (or
`ListSessionsRaw`) on the client. This is **not** added now — it is
documented as the available extension point so the default
`ListSessions` can remain safe by construction.

### Filter Definition

A session is filtered when `strings.HasPrefix(name, "_")` is true.
The match is on the literal session name as reported by tmux. No
trimming, no case-folding.

## Fix B — Rename Bootstrap Session To `_portal-bootstrap`

### Behaviour Contract

`internal/tmux.Client.StartServer` MUST create the bootstrap session
with an explicit underscore-prefixed name. The chosen name is
`_portal-bootstrap`.

The implementation invokes `tmux new-session -d -s _portal-bootstrap`
instead of the current `tmux new-session -d`.

The reserved name MUST be exposed as an exported package-level
constant in `internal/tmux` (sibling to `PortalSaverName`), e.g.
`PortalBootstrapName = "_portal-bootstrap"`. Tests and any future
diagnostic tooling reference the constant rather than the literal
string.

### Why Rename Instead Of Kill

Three alternatives were considered for the bootstrap session:

- **Rename to `_portal-bootstrap` (chosen).** Smallest surface
  change. Hidden by Fix A's filter. Lets tmux's native `exit-empty`
  lifecycle handle reaping when other sessions exist. If Restore
  creates nothing on a given startup, the bootstrap session remains
  as the keep-server-alive anchor — exactly its intended job.
- **Add an explicit kill step in the bootstrap orchestrator after
  Restore (rejected).** Works but introduces a new orchestrator
  step plus a precondition check (must not kill the only session,
  or the server dies). More moving parts than the rename.
- **Revert `StartServer` to `tmux start-server` (rejected).** Re-
  introduces the failure mode that commit `bd659a3` fixed —
  `exit-empty on` can kill the server before restoration finishes.

### Lifecycle After The Rename

- **Server start with restorable state:** `_portal-bootstrap` is
  created. `Restore` (bootstrap step 5) creates real user sessions.
  When tmux's `exit-empty on` policy applies, `_portal-bootstrap`
  may be reaped naturally because real sessions exist. If it
  persists, Fix A's filter hides it from view.
- **Server start with no restorable state:** `_portal-bootstrap`
  is created and remains as the only session. It keeps the server
  alive (its original purpose). Fix A's filter hides it from the
  user's session list.
- **Server already running:** `StartServer` is not called.
  Bootstrap proceeds without creating the session.

### Naming Constraint

`_portal-bootstrap` is reserved. Other code MUST NOT create or
re-use a session with this name. The constant is the canonical
reference.

## Doc-Comment Cleanup

Two existing doc-comments encode incorrect or stale claims and MUST
be updated as part of this work. The edits are part of the bugfix,
not a follow-up — they document the post-fix invariants.

### `tmux.PortalSaverName`

The current comment claims `_portal-saver` is "filtered from the
TUI picker and from `sessions.json` capture." Pre-fix, the TUI
filter does not exist. After Fix A lands, the claim becomes
accurate as written. The comment may be tightened but its substance
stands.

### `tmux.StartServer`

The current comment includes the rationale: *"The unnamed session
defaults to '0', which tmux-resurrect recognizes and cleans up."*
This rationale is stale — Portal no longer relies on tmux-resurrect
for cleanup, and the user has confirmed the session persists with
the plugin removed.

After Fix B, the comment MUST:

- Drop the tmux-resurrect cleanup claim entirely.
- Document that the session is created with the reserved name
  `PortalBootstrapName` (`_portal-bootstrap`).
- Document that the session is hidden from user-facing listings by
  the underscore-prefix filter in `Client.ListSessions`.
- Retain the `exit-empty on` rationale for using `new-session -d`
  rather than `start-server` (this is still load-bearing — commit
  `bd659a3`).

## Test Requirements

The fix MUST add or update tests at three layers. Each test asserts
a single invariant; together they cover the spec gap that allowed
the bug to ship.

### Unit — `Client.ListSessions` Excludes `_*` Sessions

In `internal/tmux/tmux_test.go`, add a unit test asserting that
`Client.ListSessions` excludes any session whose name starts with
`_`. Drive the test with mocked `Commander` output containing a
mix of `_*` and non-`_*` names; assert that only the non-`_*`
names appear in the returned slice.

This single test protects every consumer of `ListSessions` (TUI
picker, `cmd/list.go`, future callers) and prevents Root Cause 1
from regressing.

### Unit — `StartServer` Uses Reserved Bootstrap Name

Update the existing `TestStartServer` in `internal/tmux/tmux_test.go`
to assert the args passed to `Commander.Run` include
`-s _portal-bootstrap` (referenced via the `PortalBootstrapName`
constant, not the literal string). This prevents accidental
regression to an unnamed session.

### End-To-End — No `_*` Sessions Visible Post-Bootstrap

Extend either a bootstrap-level test or
`cmd/bootstrap/reboot_roundtrip_test.go` to assert that, after a
full bootstrap, the session list visible via `Client.ListSessions`
contains no entries whose names begin with `_`.

This is the assertion that would have caught both root causes during
the `built-in-session-resurrection` review. It serves as the
end-to-end regression guard for this entire bugfix and for any
future `_*` session that joins the codebase.

### Capture-Path Regression Guard

The capture-path tests (`internal/state/capture_test.go:135` and
related) MUST continue to pass unchanged. Confirm that the chosen
implementation strategy from Fix A's "Interaction With The Capture
Path" section preserves capture behaviour. No new capture tests are
required, but existing ones are an explicit regression gate.

---

## Working Notes
