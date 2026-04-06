# Investigation: Resume Hooks Not Firing After Server Kill

## Symptoms

### Problem Description

**Expected behavior:**
After `tmux kill-server`, reopening Portal should resurrect previous sessions (via tmux-resurrect) and fire resume hooks, resuming Claude Code sessions in their respective panes.

**Actual behavior:**
Portal boots tmux but goes straight to the projects page instead of the sessions page — sessions are not resurrected and resume hooks do not fire.

### Manifestation

- No sessions present after server restart — user lands on projects page instead of sessions page
- Resume hooks never execute because there are no sessions to trigger them
- The recent `resume-hooks-lost-on-server-restart` bugfix was supposed to address this scenario

### Reproduction Steps

1. Open Portal, initiate two Claude Code sessions — resume hooks register successfully
2. Run `tmux kill-server` to kill the tmux server
3. Open Ghostty terminal and type `x` (Portal alias) — not inside any tmux session
4. Portal shows "starting tmux server" loading page, then lands on projects page — no sessions restored
5. Running `tmux list-sessions` from another terminal shows "no server running"

**Reproducibility:** Confirmed on at least one system

**Critical finding:** After Portal's bootstrap, the tmux server is not running at all. The server either fails to start properly or starts and immediately dies. This means tmux-resurrect/continuum never get a chance to restore sessions.

### Environment

- **Affected environments:** Local (tested on a separate MacBook Pro, not the dev machine)
- **Platform:** macOS (Ghostty terminal)
- **tmux plugins confirmed:** tmux-resurrect + tmux-continuum installed with `@continuum-restore 'on'` and `@resurrect-capture-pane-contents 'on'`
- **No error output** from Portal during startup

### Impact

- **Severity:** High
- **Scope:** All users relying on resume hooks after server restart
- **Business impact:** Core workflow disruption — Claude sessions lost on tmux restart

### References

- Previous bugfix: `resume-hooks-lost-on-server-restart` (recent commits on main)

---

## Analysis

### Initial Hypotheses

- ~~tmux-resurrect may not be installed on the test machine~~ — CONFIRMED installed with auto-restore on
- Portal's EnsureServer / server bootstrap may start the server in a way that doesn't persist (e.g., server starts, Portal's TUI runs, server dies when Portal exits)
- The tmux server may start but without loading tmux.conf / TPM plugins, so resurrect never triggers
- Portal may be using `tmux new-session` in a way that creates and immediately destroys the server

### Code Trace

**Entry point:** `cmd/root.go:53` — `PersistentPreRunE` calls `bootstrapper.EnsureServer()`

**Server bootstrap path:**
1. `cmd/root.go:63` — `bootstrapper.EnsureServer()` called
2. `internal/tmux/tmux.go:137-145` — `EnsureServer()` checks `ServerRunning()` (runs `tmux info`), if false calls `StartServer()` (runs `tmux start-server`)
3. `tmux start-server` starts the server process, sources `.tmux.conf`, but creates **no sessions**
4. tmux's `exit-empty` option (on by default) causes the server to exit when there are no active sessions

**TUI loading path (no-args invocation):**
1. `cmd/open.go:90-91` — no destination → calls `openTUIFunc` with `serverStarted=true`
2. `cmd/open.go:349-406` — `openTUI` builds model with `serverStarted: true`, launches Bubble Tea
3. `internal/tui/model.go:367-374` — `WithServerStarted(true)` sets `activePage = PageLoading`
4. `internal/tui/model.go:607-629` — `Init()` on PageLoading: fetches sessions, sets minWait (1s) and maxWait (6s) ticks
5. `internal/tui/model.go:671-681` — SessionsMsg handler on PageLoading: if sessions empty, calls `pollSessionsCmd()` (re-polls every 500ms)
6. `internal/tui/model.go:696-701` — MaxWaitElapsedMsg (6s): calls `transitionFromLoading()`
7. `internal/tui/model.go:585-589` — `transitionFromLoading()` → `evaluateDefaultPage()`
8. `internal/tui/model.go:542-545` — `evaluateDefaultPage()`: 0 session items → `activePage = PageProjects`

**Why sessions are empty:**
- `ListSessions()` (`tmux.go:79-83`) swallows errors, returns `[]Session{}, nil` when server is dead
- Every 500ms poll returns empty because the server exited before or shortly after `start-server` returned
- The server never lives long enough for tmux-continuum to restore sessions

**Key files involved:**
- `internal/tmux/tmux.go:125-145` — `StartServer` / `EnsureServer` — starts server with no sessions
- `internal/tui/model.go:574-589` — loading/polling logic — polls a dead server
- `cmd/root.go:50-76` — `PersistentPreRunE` — bootstrap orchestration

### Root Cause

**`tmux start-server` combined with tmux's default `exit-empty on` causes the server to exit immediately because no sessions exist.**

Portal's `EnsureServer()` calls `tmux start-server`, which starts the tmux server process and sources `.tmux.conf`. However, with `exit-empty on` (the tmux default), the server exits as soon as it detects there are no active sessions. This creates a race condition:

1. Server starts, begins sourcing `.tmux.conf`
2. TPM plugin manager initializes via `run-shell` (potentially async)
3. tmux-continuum should detect `@continuum-restore 'on'` and trigger session restoration
4. But the server exits (or is scheduled to exit) because `exit-empty` sees 0 sessions
5. Session restoration either never starts or is aborted mid-flight

Portal's TUI polls for sessions every 500ms for up to 6 seconds, but every poll hits a dead server. `ListSessions()` swallows the error and returns an empty slice, so Portal sees "no sessions" rather than "server died." After MaxWait (6s), the TUI transitions to the projects page.

**Why this happens:** The `EnsureServer` / `StartServer` approach was designed to start a tmux server and wait for sessions to appear (via resurrect/continuum). But `tmux start-server` creates a sessionless server, and tmux's default behavior is to exit a sessionless server immediately. The 6-second polling window is meaningless when the server is already dead.

### Contributing Factors

- **`exit-empty on` (tmux default):** The server self-terminates with no sessions. Users would need to explicitly set `exit-empty off` in their `.tmux.conf` — but Portal doesn't document this requirement and most users won't have it set.
- **`ListSessions()` swallows errors:** Returns `[]Session{}, nil` on failure — Portal can't distinguish "no sessions yet" from "server is dead." The polling loop silently retries against a dead server.
- **`tmux start-server` is a fire-and-forget:** It starts the server but doesn't guarantee the server will stay alive. The `.tmux.conf` processing (including plugin initialization) may be partially async.
- **No session anchor:** The server has nothing keeping it alive. A single dummy/bootstrap session would prevent `exit-empty` from triggering while plugins initialize.
- **tmux-continuum trigger mechanism:** Continuum's auto-restore may depend on session creation events rather than bare server start, meaning `start-server` alone may not trigger restoration at all.

### Why It Wasn't Caught

- The previous bugfix (`resume-hooks-lost-on-server-restart`) focused on hook storage and keying — it assumed tmux-resurrect would handle session restoration and didn't test the server bootstrap path
- The dev machine likely has an active tmux server (user noted "too many things running"), so `EnsureServer` would return `(false, nil)` — the `start-server` path is never exercised
- The 6-second MaxWait was designed for the gap between server start and session restoration, but nobody tested what happens when the server dies within that window
- No test covers the `tmux start-server` → `exit-empty` → server dies → no restoration scenario

### Blast Radius

**Directly affected:**
- `internal/tmux/tmux.go:125-131` — `StartServer()` — the mechanism that starts the server
- `internal/tmux/tmux.go:137-145` — `EnsureServer()` — orchestrates server start
- All resume hook functionality — hooks can't fire without sessions
- TUI loading page — polls a dead server

**Potentially affected:**
- `cmd/bootstrap_wait.go` — CLI bootstrap wait also polls for sessions against a dead server
- Any command that depends on `PersistentPreRunE` starting the server (attach, open, kill, list)

---

## Fix Direction

_To be determined after findings review_

---

## Notes

- User explicitly asked not to kill tmux server on the dev machine during investigation
- tmux-resurrect and tmux-continuum are confirmed installed with auto-restore on
- The previous bugfix was about hook storage durability — this bug is about server lifecycle and session restoration
- There may also be a tmux-continuum trigger issue: continuum might only auto-restore on session creation, not on bare `start-server`. This is a secondary concern — if the server dies immediately, the trigger mechanism is moot
- The user's tmux.conf does NOT set `exit-empty off`
- **Prior art:** The `exit-empty` risk was explicitly acknowledged in the original `auto-start-tmux-server` discussion (`.workflows/auto-start-tmux-server/discussion/auto-start-tmux-server.md:67`): "tmux docs say the server exits by default with no sessions. Assumption is that continuum hooks into the process quickly enough to create sessions before this happens. If not, a keepalive session could be added later." The edge case has now materialized.
- Synthesis agent validated root cause with high confidence — all code paths independently verified
