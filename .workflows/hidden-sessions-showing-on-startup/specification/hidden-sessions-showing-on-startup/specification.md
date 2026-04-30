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

---

## Working Notes
