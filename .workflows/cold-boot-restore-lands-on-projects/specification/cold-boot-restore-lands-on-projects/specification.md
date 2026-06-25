# Specification: Cold-Boot Restore Lands on Projects

## Specification

### Context: Defect & Root Cause

#### Defect

On a **cold start** (no tmux server yet) launched through the TUI picker (`portal open`), after the concurrent bootstrap restores every saved session the picker opens on the **Projects** page instead of **Sessions** — even though all N sessions restored correctly (names and scrollback intact). The user must press `x` to reach the just-restored sessions. The warm path (server already running) lands on Sessions correctly; only the cold concurrent-bootstrap route is affected.

A second defect with the **same root cause** rides along: a cold-boot launch carrying a resolution-chain filter (`initialFilter`, e.g. a `portal open <query>` that fell through to the picker) applies that filter to the **Projects** list instead of Sessions, and the filter value is consumed (zeroed) in the process — so it is never re-applied to Sessions even after the session list is repaired.

Both are **UX-only** (low severity): restore itself is fully correct — only the initial page selection and filter routing are wrong. The behaviour is **deterministic** on the cold route, not a non-deterministic race.

#### Affected path

The cold concurrent-bootstrap route only — a fresh tmux server *and* the TUI picker, where the orchestrator runs in a goroutine (`progressReceiver != nil`). The warm path, the CLI / direct-path, and inside-tmux paths are correct and must remain unaffected.

#### Root cause

`evaluateDefaultPage()` chooses the landing page from the live session count: it lands on Sessions only when `len(m.sessionList.Items()) > 0`, otherwise Projects. It is **one-shot latched** by `defaultPageEvaluated` — set `true` on the first call where both `sessionsLoaded && projectsLoaded` hold, and never reset.

On the cold route the bootstrap orchestrator (including Restore, which creates the saved sessions) runs in a goroutine concurrently with the TUI. `Init`'s frame-one `fetchSessions` therefore enumerates tmux **before** Restore has created any sessions — the snapshot is empty. When the loading page dismisses, `transitionFromLoading()` calls `evaluateDefaultPage()` against that **stale empty list** → lands on Projects and **latches the decision permanently**. The post-restore re-fetch (`refetchSessionsAfterRestore`), which exists to repair exactly this stale snapshot, lands one Update cycle later and updates the list **contents** — but `evaluateDefaultPage` is by then a guarded no-op, so it never re-decides the page.

The filter co-defect shares this mechanism: the `initialFilter` application lives **inside** `evaluateDefaultPage` and is gated on the chosen page being Sessions. Because the latched decision is Projects, the filter is routed to the project list and `initialFilter` is zeroed in the same one-shot call.

### Fix Approach: Defer the Landing Decision to the Post-Restore Refetch

The fix changes **when** the landing-page decision is made on the cold route, not the decision logic itself. The single `evaluateDefaultPage()` call that today fires prematurely inside `transitionFromLoading()` against the stale empty list is deferred — on the cold route — to the post-restore refetch's `SessionsMsg`, which already calls `evaluateDefaultPage()` and arrives carrying the correct post-restore session list.

Concretely:

- On the **cold concurrent route** (`progressReceiver != nil`), `transitionFromLoading()` sets a valid interim `activePage = PageSessions` and marks `sessionsLoaded = true`, but does **not** call `evaluateDefaultPage()`. The decision is left for the refetch.
- `refetchSessionsAfterRestore()` is already dispatched at the transition on this route; its post-restore `SessionsMsg` arrives when `activePage != PageLoading`, so the existing `SessionsMsg` handler path runs `evaluateDefaultPage()` — now against the **post-restore** list. With `sessionsLoaded` and `projectsLoaded` both already true and the latch still unset, it decides correctly and latches.
- On the **warm / CLI route** (`progressReceiver == nil`), `transitionFromLoading()` still calls `evaluateDefaultPage()` synchronously exactly as today. There is no refetch on this route, so the decision must be made at the transition against the already-post-restore `Init` snapshot.

`transitionFromLoading()` is the single chokepoint reached by both the `LoadingMinElapsedMsg` and `BootstrapCompleteMsg` handlers (whichever closes the second gate), so gating the `evaluateDefaultPage()` call there covers every transition trigger without touching either handler.

Because the `initialFilter` application lives inside `evaluateDefaultPage()`, deferring that one call also resolves the filter co-defect with no extra code: on the cold route the filter is applied during the post-restore decision, so when the page resolves to Sessions the filter is routed to — and consumed by — the **session** list.

The one-shot `defaultPageEvaluated` latch is left untouched; only the **timing** of the single `evaluateDefaultPage()` call changes on the cold route. This keeps the blast radius minimal and the warm-path startup ordering byte-identical (the zero-new-risk contract from `spectrum-tui-design`).

### Acceptance Criteria

| # | Scenario | Required behaviour |
|---|----------|--------------------|
| AC1 | Cold boot, TUI picker, **N>0** sessions restored | Picker opens on **Sessions**, listing all N restored sessions. No `x` keypress required to reach them. |
| AC2 | Cold boot, TUI picker, **zero** sessions restored | Picker opens on **Projects** (the fix must not over-correct to always-Sessions). |
| AC3 | Cold boot with an `initialFilter`, **N>0** sessions restored | Picker opens on **Sessions** with the filter applied to the **session** list (and consumed there, not against Projects). |
| AC4 | Warm path (server already running), N>0 sessions | Picker opens on **Sessions**, byte-identical to today — unchanged. |
| AC5 | Warm path, zero sessions | Picker opens on **Projects**, byte-identical to today — unchanged. |
| AC6 | Command-pending launch (`commandPending`) | Lands on **Projects** as today — the `commandPending` branch of the landing decision is independent of session count and must be preserved. |
| AC7 | Cold route, interim window between loading-page dismissal and the post-restore refetch landing | A valid page is shown (interim **Sessions**); no blank, undefined, or loading page flashes. |

These criteria are the observable contract; the fix is correct only if every row holds.

### Constraints & Invariants

- **Warm / CLI / direct-path untouched.** The synchronous route (`progressReceiver == nil`) keeps making the landing decision at `transitionFromLoading()` time against its already-post-restore `Init` snapshot. No new enumeration, no behaviour change, no new ordering dependency. This is the zero-new-risk contract — the warm-path startup sequence has prior-incident history (slow-open / zombie-session) and must not be perturbed.
- **No over-correction.** The deferred decision runs the *same* `len(Items()) > 0` test against the post-restore list, so a genuine zero-session cold boot still lands on Projects (AC2). The fix changes *when* the test runs, never *what* it tests.
- **Valid interim page.** Between loading-page dismissal and the post-restore refetch landing, the cold route must hold a valid `activePage` (interim **Sessions**). It must never sit on an undefined page, re-enter the loading page, or render a blank frame.
- **Latch preserved.** The one-shot `defaultPageEvaluated` latch is not modified, removed, or re-opened. The decision is still made exactly once; only its timing moves on the cold route. (Option B — re-opening the latch — was explicitly rejected as patching the symptom rather than the ordering.)
- **Decision always resolves on the cold route.** `refetchSessionsAfterRestore()` always emits a `SessionsMsg` on the cold route, and the `SessionsMsg` handler re-invokes `evaluateDefaultPage()` whenever `activePage != PageLoading` — so the deferred decision is guaranteed to be taken (no path strands the picker on the interim page). A `SessionsMsg` carrying an error continues to quit, exactly as today.
- **`commandPending` branch preserved.** The `commandPending → Projects` arm of `evaluateDefaultPage()` is independent of the deferral and must remain correct.
- **Scope: cold concurrent route only.** The change is confined to the cold concurrent-bootstrap landing-decision ordering in `internal/tui/model.go`. No changes to the bootstrap orchestrator, restore engine, tmux enumeration, or `evaluateDefaultPage`'s decision logic.

---

## Working Notes
