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

---

## Working Notes
