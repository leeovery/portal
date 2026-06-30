# Discussion: Skip Bootstrap When Warm

## Context

Portal's eleven-step bootstrap orchestrator (`cmd/bootstrap`, run from `cmd/root.go`'s `PersistentPreRunE`) fires on **every** command not in the `skipTmuxCheck` allow-list. Its meaningful work — `EnsureServer`, `RegisterPortalHooks`, `SweepOrphanDaemons`, `EnsureSaver`, `Restore` (reboot recovery), and the `CleanStale` sweeps — is logically a *once-per-tmux-server-lifetime* concern, not a once-per-command one. It runs on every command purely defensively: Portal has no "has this server already been bootstrapped this lifetime?" signal, so each command re-ensures the whole world. On a warm server these steps are all idempotent no-ops, so it is pure redundant work plus an avoidable concurrency surface when N commands hit the server near-simultaneously.

**Shaped intent (from discovery):** set a single server-scoped latch (a tmux *server* option that dies with the server, e.g. `@portal-bootstrapped`) at the end of a successful bootstrap. Later commands in that lifetime see the latch and fast-skip the orchestrator while still building their tmux client (which is separate from the orchestrator). First (cold) touch bootstraps and sets the latch; warm commands do a cheap latch-check and move on.

**Payoff:** every warm `portal` command stops re-running restore/sweep/clean; the concurrency surface of N near-simultaneous commands collapses from N full bootstraps to N cheap latch-checks.

**Blast radius (respect it):** load-bearing core machinery. The cold/warm flip (`shouldRunConcurrentBootstrap`), the concurrent-TUI bootstrap path (`cmd/bootstrap_progress.go`), the daemon singleton (`AcquireDaemonLock`), and the `@portal-restoring` window all live near here. The cold+TUI concurrent path must set the latch at completion too. A benign first-touch race (two commands hitting a fresh server both bootstrap) already exists today and is already tolerated via the daemon flock plus idempotent hook convergence — the latch only reduces its frequency, it need not eliminate it.

**Dependency / origin:** surfaced as review finding F1 during the `restore-host-terminal-windows` feature discussion. That feature's multi-select reopen spawns N−1 windows each running `portal attach <session>`, and `attach` is *not* in `skipTmuxCheck` — so a 14-window post-crash rebuild would fire 13 near-simultaneous full bootstraps against one server. This latch dissolves that and lets reopen spawn plain `portal attach` with no special bootstrap-exempt command or hidden flag. Intended as its own feature, built **before** `restore-host-terminal-windows`, which depends on it landing first.

### Code Anchors (confirmed via code map, 2026-06-30)

- **Entry point:** `cmd/root.go` `PersistentPreRunE` (143–221). `skipTmuxCheck` (38–46) = `alias, clean, help, hooks, init, state, version`. `attach` is **not** in it (the F1 dependency).
- **Routing:** `shouldRunConcurrentBootstrap` (257–264) returns true **only** for `portal open` with zero args **and** server not running (`client.ServerRunning()` = one `tmux info` probe). Everything else — including every warm command — runs the **synchronous** path. ⇒ *The latch only ever short-circuits the warm/synchronous path; the concurrent (cold+TUI) path never sees a latch to check — it only ever sets one.*
- **In-process memoisation already exists:** `runBootstrap` wraps `runner.Run` in a `sync.Once` (`bootstrapOnce`, 86–91). The latch is the cross-*process* equivalent of that gate.
- **Orchestrator:** `cmd/bootstrap/bootstrap.go` `Run(ctx) (bool serverStarted, []Warning, error)` (274–474). Fatal steps = 1 EnsureServer, 2 RegisterPortalHooks, 3 SetRestoring, 8 ClearRestoring (return `*FatalError`). Soft steps = 4 SweepOrphanDaemons, 5 EnsureSaver (`SaverDownWarning`), 6 Restore (`CorruptSessionsJSONWarning`), 7 EagerSignalHydrate, 9 CleanStaleMarkers, 10 SweepOrphanFIFOs, 11 CleanStale.
- **Server-option API** (`internal/tmux/tmux.go`): `SetServerOption(name,value)` (`set-option -s`), `TryGetServerOption(name) (val, found, err)`, `UnsetServerOption(name)` (`set-option -su`, idempotent). Seam interfaces already in `internal/state/markers.go`: `RestoringChecker` (TryGet), `ServerOptionWriter` (Set/Unset).
- **Direct precedent:** `@portal-restoring` is **already a presence-latch** — set step 3 (`SetServerOption(@portal-restoring,"1")`), cleared step 8, read via `IsRestoringSet` → `TryGetServerOption`. The new `@portal-bootstrapped` latch copies this shape exactly.
- **Context injection on skip:** the sync path injects `serverStartedKey` + `tmuxClientKey` into `cmd.Context()` (203–206). A latch-skip must still do this injection (with `serverStarted=false`) — it just doesn't run the orchestrator.

### References

- Seed: `seeds/2026-06-30-warm-command-bootstrap-latch.md` (inbox:idea)
- Discovery: `discovery/session-001.md`
- Downstream dependent: `restore-host-terminal-windows` (review finding F1)
- Prior art: `daemon-merge-reintroduces-dead-sessions` (spec) — server-scoped marker lifecycle, bootstrap soft-warning posture, daemon already ticking after step 4.

## Discussion Map

A living index of subtopics tracked during the discussion.

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — Skip Bootstrap When Warm (8 subtopics, all pending)

  ┌─ ○ Latch storage & semantics [pending]
  ├─ ○ Where & when the latch is set [pending]
  ├─ ○ Latch-check placement in the entry path [pending]
  ├─ ○ What "skip" means — scope of the fast path [pending]
  ├─ ○ First-touch race tolerance [pending]
  ├─ ○ Partial-bootstrap / failure handling [pending]
  ├─ ○ Interaction with the cold+TUI concurrent path [pending]
  └─ ○ Edge cases & invalidation [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## Summary

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- Discussion just initialized; no subtopics decided yet.

## Triage

(none)
