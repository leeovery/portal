---
status: in-progress
created: 2026-04-30
cycle: 1
phase: Gap Analysis
topic: hidden-sessions-showing-on-startup
---

# Review Tracking: hidden-sessions-showing-on-startup - Gap Analysis

## Findings

### 1. Fix A strategy choice left to implementer with no decision criteria

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Interaction With The Capture Path

**Details**:
The spec offers two "equivalent" implementation strategies for the
filter placement and tells the implementer to "choose one and document
which". This is a design decision deferred into implementation. The
two strategies are not actually equivalent in code shape:

- Strategy 1 requires `ListSessionNames` to be re-pointed at a new
  lower-level raw enumeration (which is not described — does it
  already exist? must it be created? what is it called?).
- Strategy 2 relies on `internal/state` double-filtering, which is
  fine functionally but couples the capture path to the assumption
  that `_*` is filtered upstream.

A planning agent or implementer cannot proceed without picking, and
picking requires inventing detail the spec does not contain (the
name and shape of the "lower-level raw enumeration" in Strategy 1).

The spec should either:
- Pick the strategy and describe it concretely (including any new
  internal method, its name, and its signature), or
- Confirm Strategy 2 is the chosen approach and remove Strategy 1
  from the spec to avoid implementer indecision.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 2. `exit-empty` setting is unspecified — Fix B's lifecycle claims rely on it

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix B — Lifecycle After The Rename; Root Cause 2

**Details**:
The spec repeatedly invokes tmux's `exit-empty on` policy to justify
why Fix B (rename) is sufficient and why no explicit kill step is
needed. But the spec never states what `exit-empty` is currently
set to in Portal-managed tmux servers, nor whether Portal sets it.

If `exit-empty` is `off` (or user-overridden off), `_portal-bootstrap`
is never reaped — it persists indefinitely as a hidden session. That
is not a bug per se (Fix A hides it), but the lifecycle narrative
in "Fix B — Lifecycle After The Rename" reads as if reaping is the
expected outcome, which is misleading for any reader trying to
reason about long-running servers.

A planning agent will not know whether to:
- Add an explicit `set-option -g exit-empty on` somewhere, or
- Document that `_portal-bootstrap` is expected to persist for the
  server's full lifetime in the no-restorable-state case, or
- Verify Portal's existing default and call it out as a precondition.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 3. `PortalSaverName` doc-comment cleanup — mandatory or optional?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Doc-Comment Cleanup — `tmux.PortalSaverName`

**Details**:
The Doc-Comment Cleanup section opens with "Two existing doc-comments
... MUST be updated as part of this work." But the `PortalSaverName`
sub-section then says the comment "may be tightened but its substance
stands."

This is internally inconsistent: is the edit required (per the MUST
opener) or optional (per "may be tightened")? An implementer reading
the spec will not know whether shipping with the existing comment
text is acceptable.

Also, the Rollout section assigns the `PortalSaverName` doc cleanup
to commit 1, which implies a concrete edit is expected. The spec
should resolve whether the edit is in fact required and, if so, give
the target wording (or a clear directive on what should change).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 4. Pre-existing `0` session from prior installs not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix B — Behaviour Contract; Lifecycle After The Rename

**Details**:
Users upgrading from a current Portal build to the post-fix build
will, on first run, find a still-running tmux server with the legacy
`0` bootstrap session already present. The new `StartServer` will
not run (server already running), so the rename never happens and
the legacy `0` session continues to surface.

`0` does not start with `_`, so Fix A's filter does not hide it.
The spec's reproduction steps assume "fresh tmux" or "kill the
tmux server", which sidesteps the upgrade scenario.

A planning agent needs to know whether to:
- Treat upgrade as out-of-scope (with a release note telling users
  to restart their tmux server), or
- Add a one-shot cleanup that detects and renames/kills any legacy
  `0` session during bootstrap, or
- Apply Fix A's filter more broadly (e.g. also filter the literal
  name `0`) as a transitional measure.

This is a real-world rollout question that the Rollout section does
not address.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 5. `cmd/list.go` empty-output behaviour unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Behaviour Contract; Test Requirements

**Details**:
After Fix A, on a freshly bootstrapped server with no restorable
state, `Client.ListSessions` returns an empty slice (only
`_portal-bootstrap` and `_portal-saver` exist, both filtered).

`portal list` currently prints whatever `ListSessions` returns. The
spec does not say what `portal list` should output when the slice
is empty:
- An empty stdout (silent), or
- A message ("no sessions"), or
- Maintain whatever current behaviour is (which the spec doesn't
  describe).

This is small but real — scripts piping `portal list` through
counters or `wc -l` will see different results, and the
"Behavioural change beyond the visible UX" paragraph already
acknowledges scripted consumers exist.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 6. TUI behaviour when `filteredSessions` becomes empty

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Behaviour Contract

**Details**:
Same scenario as finding 5, in the TUI: a fresh server with no
restorable state means the session picker will be presented with
zero sessions. The spec does not state what the TUI should display
in that case, nor whether existing TUI code already handles the
empty-list case gracefully.

If the TUI today never sees an empty list because `_portal-saver`
and `0` always populate it, this fix may surface a latent UX gap.
A planning agent needs to know whether to:
- Verify the existing empty-list rendering and call it acceptable, or
- Add an "empty state" UX as part of this bugfix, or
- Document the empty-list rendering as out of scope.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 7. No regression guard for future unnamed `tmux new-session` callers

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix B — Sole Production Caller Verified; Test Requirements

**Details**:
The spec correctly identifies that `StartServer` is the only current
production caller of `tmux new-session` without `-s`, and warns:
"Any future contributor adding a sibling unnamed `new-session` would
re-introduce the bug."

But no test or lint rule is mandated to enforce this invariant. The
end-to-end test "No `_*` Sessions Visible Post-Bootstrap" only
catches `_*` leaks, not unnamed-session leaks (a future unnamed
session would default to `0`, `1`, etc. — none start with `_`).

A planning agent will not know whether to:
- Add a unit test or static check that asserts no production caller
  invokes `new-session` without `-s`, or
- Treat the warning as a code-review concern only.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 8. End-to-end test placement is left open

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Requirements — End-To-End — No `_*` Sessions Visible Post-Bootstrap

**Details**:
The spec says "Extend either a bootstrap-level test or
`cmd/bootstrap/reboot_roundtrip_test.go`". This leaves the test
location open — the implementer must decide. The two locations have
different test infrastructures (real-tmux fixture vs. mocked
orchestrator), and the choice changes what the test actually proves.

For planning readiness, either pin the location or describe the
selection criterion (e.g. "use the real-tmux fixture path because
the assertion is a tmux-level invariant"). Otherwise the planner
will pick somewhat arbitrarily.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 9. `Client.ListSessions` filter — empty-result semantics not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Filter Definition

**Details**:
The Filter Definition section pins down what counts as a `_*` match
but does not specify the return shape when the post-filter slice is
empty: nil slice or empty slice? Today, callers may differ in
tolerance (range over nil is fine; some explicit `len(...) == 0`
checks may behave the same; serialization to JSON differs between
`null` and `[]`).

Minor, but for a chokepoint that every consumer relies on, the
contract should be explicit.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 10. Filter ordering relative to existing post-processing in `ListSessions`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix A — Behaviour Contract; Interaction With The Capture Path

**Details**:
`Client.ListSessions` already does some parsing/post-processing of
tmux output. The spec says to apply the filter at "the post-
processing layer" but doesn't pin down where in the chain
(immediately after parse? after sort? before/after de-dup if any?).

If `ListSessions` later grows additional post-processing (sort,
attach metadata), the filter's position may matter. For an
invariant claimed to be Portal-wide and chokepoint-enforced, the
spec should say "filter as the final step before return" (or
similar) so the contract is unambiguous.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
