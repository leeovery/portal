# Investigation: Cold-Boot Restore Lands on the Projects Page, Not Sessions

## Symptoms

### Problem Description

**Expected behavior:**
On a cold start (no tmux server yet) through the TUI picker, after the concurrent bootstrap restores every saved session, the picker should open on the **Sessions** page — matching the warm path, which lands on Sessions.

**Actual behavior:**
On the cold concurrent-bootstrap path, the picker opens on the **Projects** page instead of Sessions despite N sessions being restored correctly. The loading screen reports `Restoring sessions N/N` accurately, but the user must press `x` to reach the restored sessions.

### Manifestation

- Loading screen shows `✓ Restoring sessions 12/12 · ✓ Replaying scrollback · ✓ Running resume commands` (accurate).
- Picker opens on **Projects** (e.g. 10 projects), footer shows `x sessions`.
- Pressing `x` reveals the **Sessions** page with all 12 restored sessions (correct names, scrollback intact).
- So restore itself is fully correct — only the **initial page selection** is wrong.

### Reproduction Steps

1. Cold container, no tmux server, `sessions.json` + scrollback present (demo harness: `demo/`, sandboxed Linux container with a baked restore seed of 12 sessions).
2. `portal open` (the TUI picker) → loading screen shows `Restoring sessions 12/12`.
3. Picker opens on **Projects** (10 projects), footer `x sessions`.
4. Press `x` → **Sessions** page lists all 12 restored sessions.

**Reproducibility:** Repeatable in the demo harness (cold path). Warm path (server already running) lands on Sessions as expected — defect is specific to the cold concurrent-bootstrap landing decision.

### Environment

- **Affected environments:** Cold start (no tmux server) via the TUI picker, in the `demo/` sandboxed Linux container.
- **Browser/platform:** Linux container (demo harness `demo/portal-cold.tape`).
- **User conditions:** Saved `sessions.json` + scrollback present; tmux server not yet running so the concurrent cold-path bootstrap fires.

### Impact

- **Severity:** Low (UX). Not a data/correctness issue — sessions and scrollback restore fine.
- **Scope:** Anyone who cold-boots (reboot/fresh container) into the picker with restorable sessions.
- **Business impact:** Mildly surprising; costs an extra keypress (`x`) after a reboot to reach the just-resurrected sessions.

### References

- Seed: `seeds/2026-06-25-cold-boot-restore-lands-on-projects.md` (inbox:bug)
- Discovery: `discovery/session-001.md`
- Observed while building the cold-boot resurrection demo (`demo/portal-cold.tape`) for spectrum-tui-design, 2026-06-25.

---

## Analysis

### Initial Hypotheses

(Seed hypothesis — to verify, not asserted) The Loading → page transition chooses Sessions-vs-Projects from a session count captured *before* the restored sessions are visible to `ListSessions` — an ordering/race between restore completion on the `BootstrapCompleteMsg` path and the "no sessions yet → fall back to Projects" landing rule. Suggested to compare the cold-path landing decision in `internal/tui/model.go` (the `BootstrapCompleteMsg` handler / first non-loading page selection) against the warm path, which sees sessions at init and lands on Sessions.

Not personally reproduced by the user — observed by an agent running portal + tmux in the sandboxed container.

### Code Trace

**Entry point:** the Loading → picker transition on the cold concurrent-bootstrap route, in `internal/tui/model.go`.

**The landing decision** — `evaluateDefaultPage()` (`internal/tui/model.go:1615`):

```go
m.defaultPageEvaluated = true
if m.commandPending {
    m.activePage = PageProjects
} else if len(m.sessionList.Items()) > 0 {
    m.activePage = PageSessions
} else {
    m.activePage = PageProjects        // ← lands here when the list is empty
}
```

It is **one-shot latched**: the first line of the function is `if m.defaultPageEvaluated { return }` (`model.go:1616`), and `defaultPageEvaluated` is set to `true` exactly once (`model.go:1626`) and never reset. So whichever call first sees both `sessionsLoaded && projectsLoaded` true makes the decision **permanently**.

**Execution path — cold concurrent route** (`progressReceiver != nil`):

1. **`Init()`** (`model.go:1854`) batches `fetchSessionsCmd()` + `loadProjects()`. On the cold route the orchestrator runs in a goroutine, so this frame-one `ListSessions()` enumerates tmux **before** Restore (bootstrap step 6) creates the saved sessions — the snapshot is **stale/empty** (confirmed by the maintainers' own comment at `model.go:1804-1806`).
2. `ListSessions()` (`internal/tmux/tmux.go:193`) additionally filters out `_` -prefixed names, so `_portal-saver` never counts (`tmux.go:243-256`). At Init time there are **zero** user sessions → empty result.
3. The stale **`SessionsMsg`** arrives while `activePage == PageLoading` → `applySessions(msg.Sessions)` runs and **sets the session list to the empty snapshot** (`model.go:1982`), but deliberately does **not** flip `sessionsLoaded` (`model.go:1990-1992`). `rebuildSessionList` confirms "Zero live sessions yields an empty list in every mode" (`model.go:1424`).
4. **`ProjectsLoadedMsg`** arrives (local file read, fast) → `projectsLoaded = true`; `evaluateDefaultPage()` is called (`model.go:2098-2099`) but bails on the `!sessionsLoaded` guard (still on PageLoading) — **does not** latch.
5. Restore finishes in the goroutine → **`BootstrapCompleteMsg`** (paired with `LoadingMinElapsedMsg`). The handler calls **`transitionFromLoading()`** (`model.go:2047`):
   ```go
   func (m *Model) transitionFromLoading() {
       m.activePage = PageSessions      // tentative
       m.sessionsLoaded = true
       m.evaluateDefaultPage()          // ← now both flags true → decides on the STALE empty list
   }
   ```
   `evaluateDefaultPage` proceeds: `len(m.sessionList.Items()) == 0` → **`activePage = PageProjects`**, and `defaultPageEvaluated = true` **locks it**.
6. The same handler batches **`refetchSessionsAfterRestore()`** (`model.go:2051`, `model.go:1818`) — which exists *specifically* to repair the stale snapshot — issuing a fresh `fetchSessionsCmd()`.
7. The fresh **`SessionsMsg`** (12 restored sessions) arrives in a later Update cycle. `activePage` is now `PageProjects` (≠ PageLoading), so it takes the `model.go:1994-1995` path: `m.sessionsLoaded = true; m.evaluateDefaultPage()`. But `evaluateDefaultPage` hits the `defaultPageEvaluated` guard and **returns immediately**. The session **list contents** are updated to 12 sessions, but the **active page stays PageProjects**.

**Result:** the picker opens on Projects; pressing `x` reveals the (correctly populated) Sessions list — exactly the reported symptom.

**Warm route contrast** (`progressReceiver == nil`, `serverStarted == false`): the orchestrator ran synchronously in `PersistentPreRunE` **before** the model was built, so Init's `ListSessions()` snapshot is already post-restore and non-empty. `evaluateDefaultPage` runs on a populated list → `PageSessions`. `refetchSessionsAfterRestore()` returns `nil` on this route (`model.go:1819-1821`). Hence the warm path lands correctly.

**Key files involved:**
- `internal/tui/model.go` — `evaluateDefaultPage` (`:1615`, the latched decision), `transitionFromLoading` (`:1828`), the `SessionsMsg` / `LoadingMinElapsedMsg` / `BootstrapCompleteMsg` / `ProjectsLoadedMsg` handlers (`:1975`–`:2100`), `refetchSessionsAfterRestore` (`:1818`), `Init` (`:1854`).
- `internal/tmux/tmux.go` — `ListSessions` (`:193`) and the `_`-prefix saver filter (`:243`).

### Root Cause

On the cold concurrent-bootstrap route, the default-page decision (`evaluateDefaultPage`, which lands on Sessions only when `len(m.sessionList.Items()) > 0`) is made inside `transitionFromLoading` at loading-page dismissal **using the stale, empty session list** that Init's frame-one `fetchSessions` captured *before* Restore created the sessions. That decision is then permanently latched by the one-shot `defaultPageEvaluated` guard. The post-restore re-fetch (`refetchSessionsAfterRestore`), added to repair exactly this stale snapshot, only updates the **list contents** — it arrives in a later Update cycle when `evaluateDefaultPage` is already a guarded no-op, so it never re-decides the page. Net: the landing page is computed against an empty list → Projects, and the corrected session count arrives too late to move it.

**Why this happens:** the page decision and the latch are coupled to the session *count*, but the count is only correct *after* the refetch — which is dispatched as a separate command that lands one Update cycle after the decision was already taken and latched.

### Contributing Factors

- **Concurrent cold-boot flip runs enumeration before restore.** By design (race containment — the TUI is inert during loading), Init's `ListSessions` runs before bootstrap step 6, so the frame-one session snapshot is empty on the cold route.
- **The refetch fix addressed contents, not the decision.** `refetchSessionsAfterRestore` was introduced for the "empty-previews / slow-open" prior incident to make the *list* reflect post-restore state; it did not account for the *page-landing decision* that depends on the same count.
- **`evaluateDefaultPage` is one-shot latched.** `defaultPageEvaluated` is set once and never cleared, so a later-corrected session count cannot move the page even though `evaluateDefaultPage` is re-invoked on the refetched `SessionsMsg`.
- **`transitionFromLoading` decides synchronously, then batches the refetch.** The decision necessarily precedes the corrected data, which arrives as a separate `tea.Cmd` result.

### Why It Wasn't Caught

- **Cold route is the once-per-reboot path.** Startup-ordering tests (`cmd/concurrent_*_test.go`) cover warm/cold parity and step ordering, but the landing-page assertion on a *pre-restore-empty* cold fetch (Init sees 0 sessions, restore then creates N) appears not to be exercised.
- **The refetch's own test likely asserts list contents, not the active page.** `refetchSessionsAfterRestore` was validated by "the list is populated after dismissal," which passes here — the list *is* correct; only the page is wrong.
- **Low severity masks it.** UX-only (one extra keypress), no crash or data loss, so it didn't surface through error tracking — it was only noticed building the demo harness.

### Blast Radius

**Directly affected:**
- The initial page selection on the cold concurrent-bootstrap TUI path (`portal open` with restorable sessions after a reboot / fresh server).

**Potentially affected (to weigh during fix):**
- Any future consumer of `evaluateDefaultPage` / `defaultPageEvaluated` that assumes the decision reflects post-restore state.
- The `initialFilter` application inside `evaluateDefaultPage` (`model.go:1635-1646`) also keys off `activePage == PageSessions` — a cold-boot launch *with* a filter would likewise apply the filter to the wrong list. Same root cause; worth confirming the fix covers it.
- Warm path and CLI/direct-path: **not** affected (decision runs on a post-restore snapshot there).

---

## Fix Direction

> High-level only — the chosen approach is refined in specification. Confirmed with the user in findings review (Discussion below).

The landing-page decision must be evaluated against the **post-restore** session list on the cold route, not the stale Init snapshot. The fix is fundamentally about *ordering / latch timing*: defer (or re-open) the page decision until the corrected session count is in hand.

### Options Explored (high-level)

- **A — Don't latch the decision in `transitionFromLoading` on the concurrent route; let the post-restore refetch drive it.** On the cold route, `transitionFromLoading` would flip to the loading-exit state but NOT call `evaluateDefaultPage`; the refetched `SessionsMsg` (which already calls `evaluateDefaultPage` when not on PageLoading) would make the landing decision against the correct, post-restore list. Warm route unchanged (no refetch, decision still made synchronously as today).
- **B — Allow exactly one re-evaluation after the post-restore refetch.** Keep `transitionFromLoading` as-is but let the refetch's `SessionsMsg` re-open the latch once (e.g. the concurrent route clears/bypasses `defaultPageEvaluated` for the single post-restore refetch) so the page is re-decided on the corrected count.
- **C — Gate the cold-route transition on the refetch completing first.** Restructure so the refetch lands *before* `transitionFromLoading` evaluates the page, making the decision against fresh data in one shot.

**Leaning:** Option A — it removes the premature decision rather than patching the latch, keeps the warm path byte-identical (the §10.1 zero-new-risk contract), and reuses the existing post-refetch `evaluateDefaultPage` call site. To be finalised in specification.

### Testing Recommendations

- Add a cold-route test asserting the **active page is Sessions** when Init's `ListSessions` returns empty and the post-restore refetch returns N>0 sessions (the exact ordering of this bug). Distinct from existing "list is populated" assertions.
- Confirm warm-route parity test still lands on Sessions and that `refetchSessionsAfterRestore` stays `nil` on warm.
- Cover the empty-restore case: cold boot with genuinely zero sessions restored must still land on Projects (don't over-correct to always-Sessions).
- Confirm `initialFilter` on a cold-boot launch applies to the Sessions list (same `evaluateDefaultPage` code path).

### Risk Assessment

- **Fix complexity:** Low (localised to the cold-route transition/decision ordering in `model.go`).
- **Regression risk:** Low–Medium — touches load-bearing startup ordering with prior-incident history (slow-open / zombie-session); the warm path must stay untouched. Race review of the live event loop during loading still applies.
- **Recommended approach:** Regular release (UX-only, no hotfix urgency).

---

## Notes

- The seed's hypothesis was correct: the landing decision is captured before the restored sessions are visible to `ListSessions`. Investigation pins the *exact* mechanism — the one-shot `defaultPageEvaluated` latch in `evaluateDefaultPage`, decided inside `transitionFromLoading` against the stale Init snapshot, with the post-restore `refetchSessionsAfterRestore` landing too late to re-decide.
- Not a race in the classic non-deterministic sense — it is **deterministic** on the cold route: Init always enumerates before restore, so the count is always 0 at decision time → always Projects (consistent with the "always Projects" observation).
- The fix should be scoped to the concurrent cold route only; warm/CLI paths are correct and must remain untouched (zero-new-risk contract from `spectrum-tui-design`).
