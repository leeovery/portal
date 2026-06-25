---
status: in-progress
created: 2026-06-26
cycle: 2
phase: Gap Analysis
topic: cold-boot-restore-lands-on-projects
---

# Review Tracking: cold-boot-restore-lands-on-projects - Gap Analysis

## Findings

### 1. Interim Sessions page: input handling during the (async I/O) interim window is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Constraints & Invariants ("Valid interim page", "Interim render content"); AC7; Fix Approach ("the window is a single Update cycle because the refetch is dispatched in the same handler return"); Testing Requirements case 5 (Interim page)

**Details**:
The fix *introduces a new user-observable state* that does not exist in today's buggy
code. Today, `transitionFromLoading()` calls `evaluateDefaultPage()` immediately and
lands directly on Projects (against the stale empty list) — the user never sees an
interim Sessions page. Under the fix, on the cold route the model sets the interim
`activePage = PageSessions` and *defers* the decision to the post-restore refetch's
`SessionsMsg`. The spec accepts this interim as "valid (non-blank)" and even accepts
that it "may briefly show the Sessions empty-state signpost."

The spec repeatedly characterises this interim as "a single Update cycle." That is
true in *message-count* terms (exactly one decision-bearing `SessionsMsg` follows the
transition), but `refetchSessionsAfterRestore()` returns `fetchSessionsCmd()`, whose
body performs a real async tmux enumeration (`ListSessions()` → `tmux list-sessions`).
During that I/O the Bubble Tea event loop is live and the model is sitting on the
interim **empty** Sessions page. The wall-clock duration of the interim window is
therefore the duration of a tmux enumeration, not an instant.

The spec does not state what happens if the user presses a key during this interim
window — e.g. `Enter`/`x`/`k`/`/` on an empty Sessions page before the refetch
`SessionsMsg` lands and re-decides the page. Possible concerns an implementer would
otherwise have to resolve by guessing:
- `Enter` on an empty Sessions list (no selection) — no-op? Does it risk acting on a
  nil/zero item?
- `x` toggling to Projects mid-interim, then the refetch `SessionsMsg` arriving and
  running `evaluateDefaultPage()` — does the deferred decision override the user's
  explicit `x`, snapping them back to Sessions? (`evaluateDefaultPage` sets
  `activePage` directly, and the latch is still unset at that point.)
- A `/` filter started during interim against the empty list.

The "TUI is inert during loading" race-containment guarantee from the loading page
does *not* extend to this new interim Sessions page (it is a live picker page). Either
the spec should state that the interim window is short enough / input during it is
out of scope and accepted as-is, or it should specify the intended behaviour (and
ideally whether the deferred decision should defer to a user page-toggle that occurred
in the interim). Without this, AC7 and test case 5 only assert the *page identity*
during the interim, leaving input behaviour to implementer discretion.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 2. "Decision always resolves" invariant attributes resolution solely to the SessionsMsg handler, but resolution can land in the ProjectsLoadedMsg handler

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Constraints & Invariants ("Decision always resolves on the cold route"); cross-reference with Fix Approach "Ordering contract (cold route)"

**Details**:
The "Decision always resolves on the cold route" invariant states: "the `SessionsMsg`
handler re-invokes `evaluateDefaultPage()` whenever `activePage != PageLoading` — so
the deferred decision is guaranteed to be taken (no path strands the picker on the
interim page)." This phrasing attributes the guarantee specifically to the
post-restore **`SessionsMsg`** handler.

But the post-restore `SessionsMsg` handler's `evaluateDefaultPage()` call is a no-op
unless `projectsLoaded` is *also* true at that moment (the `!sessionsLoaded ||
!projectsLoaded` early-return). If `ProjectsLoadedMsg` arrives *after* the refetch
`SessionsMsg`, the actual page decision is taken in the **`ProjectsLoadedMsg`**
handler (which also calls `evaluateDefaultPage()`), not in the `SessionsMsg` handler.

The "Ordering contract" paragraph already gets this right ("the first call that finds
`sessionsLoaded && projectsLoaded` both true runs against the already-repaired
post-restore list"), so the spec is not self-contradictory — but the invariant's
narrower wording could mislead an implementer or test author into believing the
`SessionsMsg` handler is always the decision point. This matters because test case 6
(late `ProjectsLoadedMsg`) deliberately exercises the path where the decision is in
fact resolved by `ProjectsLoadedMsg`, not `SessionsMsg`.

Recommend aligning the invariant's wording with the ordering contract: the decision
is guaranteed to be taken by whichever of (post-restore `SessionsMsg`,
`ProjectsLoadedMsg`) lands *second*, since both handlers call `evaluateDefaultPage()`
and the latch is still unset — so no path strands the picker on the interim page.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
