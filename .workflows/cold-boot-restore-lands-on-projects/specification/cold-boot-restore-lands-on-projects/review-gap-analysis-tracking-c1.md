---
status: complete
created: 2026-06-25
cycle: 1
phase: Gap Analysis
topic: cold-boot-restore-lands-on-projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Gap Analysis

## Findings

### 1. AC7 (interim valid-page) has no corresponding required test case

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria (AC7), Testing Requirements

**Details**:
AC7 is a first-class acceptance row ("Cold route, interim window between loading-page dismissal and the post-restore refetch landing → A valid page is shown (interim Sessions); no blank, undefined, or loading page flashes"). The Constraints & Invariants section reinforces it ("Valid interim page"). But the Testing Requirements section lists only four required test cases (1: AC1, 2: AC2, 3: AC3, 4: AC4/AC5). There is no required test case mapped to AC7, and the closing line "Each test asserts the active page, not just list contents" speaks only to the four listed cases.

The spec states "These criteria are the observable contract; the fix is correct only if every row holds." An implementer told that every AC row must hold, but given a test matrix that omits AC7, would have to guess whether AC7 is meant to be (a) verified by an explicit test asserting the interim page between transitionFromLoading and the refetch SessionsMsg, (b) considered implicitly covered by test case 1 (which transits through the interim state), or (c) a manual/demo-only check. AC7 is the one criterion describing a transient intermediate frame, which is the hardest to assert and most likely to be skipped silently. Without a stated expectation, coverage of the named interim invariant is left to implementer discretion.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Added Testing Requirements case 5 — a deterministic interim-page assertion (assert PageSessions after dismissal, before the refetch SessionsMsg). Auto-mode.

---

### 2. Cold-route SessionsMsg-arrival precondition (`activePage != PageLoading`) is asserted but the ordering that guarantees it is not made explicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Approach (second bullet), Constraints & Invariants ("Decision always resolves on the cold route")

**Details**:
The fix hinges on the post-restore SessionsMsg running the `evaluateDefaultPage()` branch of the SessionsMsg handler, which fires only when `activePage != PageLoading` (the handler early-returns while on PageLoading). The spec asserts this precondition holds: "its post-restore SessionsMsg arrives when activePage != PageLoading, so the existing SessionsMsg handler path runs evaluateDefaultPage()." But it never states *why* the precondition is guaranteed — i.e. that `transitionFromLoading()` runs first and sets `activePage = PageSessions` before `refetchSessionsAfterRestore()` is dispatched (both happen in the same Update return: transition mutates the model, then the refetch cmd is batched, so the SessionsMsg it produces necessarily arrives after the interim Sessions page is set).

This is the load-bearing ordering claim of the entire fix. The interim-Sessions assignment in transitionFromLoading is precisely what lifts the SessionsMsg handler out of its PageLoading early-return. An implementer who, for instance, decided to move the interim-page assignment or change the order of the batched cmds could break the fix while believing they were honouring the spec. The spec should make this dependency explicit as part of the fix contract rather than leaving the reader to reconstruct it from the code.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Added an explicit "Ordering contract (cold route)" paragraph to Fix Approach making the same-handler-return ordering (transition mutates interim page first, then batches the refetch cmd) the load-bearing contract. Auto-mode.

---

### 3. State of `sessionsLoaded` / latch when the interim page is set on the cold route is under-specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Approach (first bullet)

**Details**:
The first Fix Approach bullet says on the cold route transitionFromLoading "sets a valid interim activePage = PageSessions and marks sessionsLoaded = true, but does not call evaluateDefaultPage()." The third bullet (warm route) and the deferred-decision narrative state the decision later runs "With sessionsLoaded and projectsLoaded both already true and the latch still unset."

Two related under-specifications:
(a) The SessionsMsg handler itself sets `m.sessionsLoaded = true` before calling evaluateDefaultPage(). So on the cold route, sessionsLoaded ends up set in two places (transitionFromLoading AND the SessionsMsg handler). The spec says transitionFromLoading should still set it, but does not say whether that is required, harmless-redundant, or load-bearing for some interim behaviour. An implementer deciding to drop the `sessionsLoaded = true` from the cold-route transition (since the SessionsMsg handler re-sets it) cannot tell from the spec whether that is safe.
(b) The narrative says the deferred decision finds "the latch still unset." This is the critical assumption: nothing between the cold-route transition (which now skips evaluateDefaultPage) and the post-restore SessionsMsg may latch defaultPageEvaluated. The spec asserts the outcome but does not enumerate which other evaluateDefaultPage() call sites could otherwise fire in that window (e.g. a ProjectsLoadedMsg arriving late, or any other handler that calls evaluateDefaultPage). If projectsLoaded is already true when the cold transition happens, the deferral is safe; if a ProjectsLoadedMsg can still arrive after the transition but before the refetch SessionsMsg, that handler would call evaluateDefaultPage against the still-empty interim session list and latch on Projects — re-introducing the bug. The spec does not state that ProjectsLoadedMsg is guaranteed to have arrived before the loading-page dismissal on the cold route (the Testing Requirements implies it must be delivered "before the loading-page transition," which suggests this is the intended ordering, but the Fix Approach / Invariants do not state it as a contract).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: CORRECTION to the fix design. Fix Approach bullet 1 amended: on the cold route transitionFromLoading does NOT set sessionsLoaded (leave false) and does NOT decide — this closes the window where a late ProjectsLoadedMsg would latch on Projects against the stale list. Ordering contract paragraph documents independence from ProjectsLoadedMsg arrival order. Guard test added as Testing Requirements case 6. Auto-mode.

---

### 4. "Cold route" detection predicate is stated two ways and tied to an implementation field without a single canonical definition

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Context (Affected path), Fix Approach (first/third bullets), Constraints & Invariants (Scope), Testing Requirements (case 4)

**Details**:
The spec gates all cold-route behaviour on `progressReceiver != nil` (Fix Approach: "On the cold concurrent route (progressReceiver != nil)"; warm route: "progressReceiver == nil"; Testing case 4: "refetchSessionsAfterRestore() stays nil on the warm route"). The Affected-path section equally describes the route as "a fresh tmux server and the TUI picker, where the orchestrator runs in a goroutine (progressReceiver != nil)."

These are presented as equivalent, but the spec never states that `progressReceiver != nil` is *exactly* and *only* set on the cold concurrent route — it is taken as given. The CLAUDE.md architecture notes that the route is decided by `shouldRunConcurrentBootstrap` (a `tmux has-server` probe), which is upstream of progressReceiver being wired. If there is any path where progressReceiver is nil yet the bootstrap ran concurrently (or vice versa), the fix's branch would mis-fire. The spec should state explicitly that progressReceiver being non-nil is the authoritative, sufficient, and necessary discriminator for the deferral branch (it appears to be, from refetchSessionsAfterRestore's existing guard), so an implementer does not introduce an alternative predicate (e.g. re-probing serverStarted or has-server).

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Added "Canonical cold-route predicate" invariant — progressReceiver != nil is the single authoritative discriminator (matches refetchSessionsAfterRestore's guard); no alternative predicate may be introduced. Auto-mode.

---

### 5. Behaviour when `transitionFromLoading` is reached via both gates / multiple times is not addressed for the new cold-route branch

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Approach (final paragraph), Constraints & Invariants

**Details**:
The spec correctly identifies transitionFromLoading as "the single chokepoint reached by both the LoadingMinElapsedMsg and BootstrapCompleteMsg handlers (whichever closes the second gate)." Both handlers, after calling transitionFromLoading, batch refetchSessionsAfterRestore(). Both arms guard on `activePage == PageLoading`, so the second gate-closer is the one that transits — that part is sound.

What is not addressed: with the cold-route change, transitionFromLoading no longer calls evaluateDefaultPage, so the *only* thing that decides the page on the cold route is the refetch SessionsMsg. The refetch is dispatched once (by whichever handler runs transitionFromLoading). The spec's invariant "refetchSessionsAfterRestore() always emits a SessionsMsg on the cold route" guarantees one SessionsMsg. But the spec does not state what guarantees that SessionsMsg is dispatched on the same path that performed the deferral — i.e. that the cold-route deferral and the refetch dispatch are inseparable. If a future change gated the refetch differently from the transition, the cold route could transit (interim Sessions) without ever dispatching the refetch, permanently stranding the picker on the interim page with no decision ever taken. The spec asserts "no path strands the picker on the interim page" as an invariant but does not tie the deferral and the refetch dispatch together as a unit to make that invariant enforceable by construction.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Captured by the ordering contract (deferral + refetch are a single unit, same handler return) plus the "Decision always resolves" invariant. Auto-mode.

---

### 6. "Interim Sessions (briefly empty)" rendering during the one-frame window is asserted safe but its visible content is not described

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Constraints & Invariants ("Valid interim page"), Out of Scope (one-frame interim render), AC7

**Details**:
The Out-of-Scope section accepts "an interim Sessions page (briefly empty before the refetch populates it, or flipping to Projects in the rare empty-restore case)." AC7 requires "no blank, undefined, or loading page flashes." There is a subtle internal tension worth resolving: on the cold route the interim Sessions page renders against the stale empty Init snapshot for one frame. An empty Sessions page in this TUI has its own empty-state rendering (empty_states.go) — e.g. a "no sessions" signpost. So for the duration of the interim frame, a cold boot that restored N>0 sessions could momentarily flash the *empty-Sessions* signpost before the refetch populates the list.

The spec says the interim page must not be "blank" — but does not clarify whether the empty-Sessions empty-state render counts as an acceptable "valid page" or as a disallowed "blank frame." For a 12-session restore this could be a visible "you have no sessions" flash for one frame. The implementer needs to know whether this is acceptable (consistent with "lowest-risk behaviour, not a target for further polish") or whether the interim render should suppress the empty-state. This is the practical observable consequence of AC7 and the spec leaves the acceptable interim *content* (vs. merely the page identity) undefined.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Added "Interim render content" invariant — the one-frame empty-Sessions empty-state render is an accepted valid (non-blank) page; must not be special-cased to suppress. Auto-mode.

---

### 7. `commandPending` on the cold concurrent route: interaction with the deferral is not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria (AC6), Constraints & Invariants (commandPending branch preserved)

**Details**:
AC6 and an invariant both require the commandPending → Projects branch to be preserved, stating it "is independent of session count." But the spec frames the deferral purely in terms of the cold route (progressReceiver != nil) without intersecting it with commandPending. On the cold route under the new logic, transitionFromLoading sets interim activePage = PageSessions and skips evaluateDefaultPage; the decision is deferred to the refetch SessionsMsg, whose handler calls evaluateDefaultPage, which has a commandPending → PageProjects arm.

The unaddressed question: can commandPending and the cold concurrent route co-occur? If they can, then under the new flow a commandPending cold boot would (a) show interim PageSessions for one frame, then (b) the refetch SessionsMsg's evaluateDefaultPage would correctly flip to PageProjects via the commandPending arm. That is a *new* one-frame Sessions flash for a commandPending launch that previously (warm, or pre-fix) went straight to Projects. The spec asserts AC6 ("Lands on Projects as today") but does not state whether "as today" tolerates that interim Sessions flash on the cold+commandPending intersection, nor whether that intersection is even reachable. If commandPending cold boots are impossible, the spec should say so to close the case; if reachable, AC6's "as today" needs the interim caveat.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Added "commandPending does not intersect the deferral" invariant — verified in code: Init's commandPending branch returns before wiring loading-page dismissal machinery, so transitionFromLoading is never invoked for a commandPending launch; no intersection, AC6 unchanged, no interim flash. Auto-mode.

---

### 8. Error-carrying SessionsMsg on the cold route quits before the deferred decision — the resulting end state is unspecified

**Source**: Specification analysis
**Category**: Edge case within scope

**Details**:
An invariant states: "A SessionsMsg carrying an error continues to quit, exactly as today." On the cold route the deferred landing decision rides entirely on the post-restore refetch's SessionsMsg. If that refetch SessionsMsg carries an error, the handler quits (tea.Quit) before reaching the deferred evaluateDefaultPage call. The spec correctly notes the quit behaviour is unchanged. However, with the new deferral the model would, in that error case, have transited to the interim PageSessions and then quit without ever running the landing decision — which is fine (the app exits). But the spec does not state the user-visible outcome of that path: does the user see the interim (possibly empty) Sessions page for the frame before quit, or is the quit immediate? This is a genuine edge case the fix introduces (the warm route never had a separate refetch that could fail). It is low-impact, but the spec introduces the refetch-as-decision-point and should confirm that a failing refetch on the cold route degrades to the same quit-with-error UX as the warm route rather than, say, leaving the picker on a stale interim page.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Approved
**Notes**: Added "Failing refetch degrades to today's quit" invariant — an error-carrying refetch SessionsMsg runs tea.Quit (unchanged); must not strand the picker on the interim page; same error UX as warm. Auto-mode.

---
