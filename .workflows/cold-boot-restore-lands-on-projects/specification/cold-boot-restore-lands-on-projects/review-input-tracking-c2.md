---
status: in-progress
created: 2026-06-25
cycle: 2
phase: Input Review
topic: cold-boot-restore-lands-on-projects
---

# Review Tracking: cold-boot-restore-lands-on-projects - Input Review

## Findings

### 1. Init-time stale `SessionsMsg` deliberately leaves `sessionsLoaded` false — the precondition the fix's "do not set `sessionsLoaded`" rests on

**Source**: Investigation — Analysis › Code Trace, step 3 ("The stale **`SessionsMsg`** arrives while `activePage == PageLoading` → `applySessions(msg.Sessions)` runs and **sets the session list to the empty snapshot** (`model.go:1982`), but deliberately does **not** flip `sessionsLoaded` (`model.go:1990-1992`)").
**Category**: Enhancement to existing topic
**Affects**: Fix Approach (the first cold-route bullet, line 44) and/or Root Cause (the cold-route execution narrative)

**Details**:
The fix's cold-route bullet states that `transitionFromLoading()` "does **not** set `sessionsLoaded`" and that "Leaving `sessionsLoaded` false is load-bearing." But this only leaves the flag false if nothing *before* the transition has already set it. The investigation documents the precondition that makes this hold: the Init-time stale `SessionsMsg` (which arrives while `activePage == PageLoading`) updates the list *contents* to the empty snapshot but deliberately does **not** flip `sessionsLoaded`. That existing behaviour is exactly why `sessionsLoaded` is still false at transition time on the cold route, which is what permits the fix to simply "not set it" rather than having to actively reset it.

Without this precondition stated, a reader could reasonably wonder why `sessionsLoaded` would be false at `transitionFromLoading()` time at all (the warm route, by contrast, has no such interim and sets it in the transition). Capturing it closes that gap and makes the "leaving it false" invariant self-evidently safe rather than asserted. It also confirms the fix does not need to defend against an earlier accidental latch via an Init-path `sessionsLoaded` flip.

**Current**:
On the **cold concurrent route** (`progressReceiver != nil`), `transitionFromLoading()` sets a valid interim `activePage = PageSessions` but does **not** set `sessionsLoaded` and does **not** call `evaluateDefaultPage()`. Leaving `sessionsLoaded` false is load-bearing: it keeps every `evaluateDefaultPage()` invocation a no-op (via the `!sessionsLoaded` early-return) until the post-restore refetch lands, so nothing can latch against the stale interim list. The decision is left to the refetch.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
