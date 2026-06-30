# Discussion: Skip Bootstrap When Warm

## Context

Portal's eleven-step bootstrap orchestrator (`cmd/bootstrap`, run from `cmd/root.go`'s `PersistentPreRunE`) fires on **every** command not in the `skipTmuxCheck` allow-list. Its meaningful work — `EnsureServer`, `RegisterPortalHooks`, `SweepOrphanDaemons`, `EnsureSaver`, `Restore` (reboot recovery), and the `CleanStale` sweeps — is logically a *once-per-tmux-server-lifetime* concern, not a once-per-command one. It runs on every command purely defensively: Portal has no "has this server already been bootstrapped this lifetime?" signal, so each command re-ensures the whole world. On a warm server these steps are all idempotent no-ops, so it is pure redundant work plus an avoidable concurrency surface when N commands hit the server near-simultaneously.

**Shaped intent (from discovery):** set a single server-scoped latch (a tmux *server* option that dies with the server, e.g. `@portal-bootstrapped`) at the end of a successful bootstrap. Later commands in that lifetime see the latch and fast-skip the orchestrator while still building their tmux client (which is separate from the orchestrator). First (cold) touch bootstraps and sets the latch; warm commands do a cheap latch-check and move on.

**Payoff:** every warm `portal` command stops re-running restore/sweep/clean; the concurrency surface of N near-simultaneous commands collapses from N full bootstraps to N cheap latch-checks.

**Blast radius (respect it):** load-bearing core machinery. The cold/warm flip (`shouldRunConcurrentBootstrap`), the concurrent-TUI bootstrap path (`cmd/bootstrap_progress.go`), the daemon singleton (`AcquireDaemonLock`), and the `@portal-restoring` window all live near here. The cold+TUI concurrent path must set the latch at completion too. A benign first-touch race (two commands hitting a fresh server both bootstrap) already exists today and is already tolerated via the daemon flock plus idempotent hook convergence — the latch only reduces its frequency, it need not eliminate it.

**Dependency / origin:** surfaced as review finding F1 during the `restore-host-terminal-windows` feature discussion. That feature's multi-select reopen spawns N−1 windows each running `portal attach <session>`, and `attach` is *not* in `skipTmuxCheck` — so a 14-window post-crash rebuild would fire 13 near-simultaneous full bootstraps against one server. This latch dissolves that and lets reopen spawn plain `portal attach` with no special bootstrap-exempt command or hidden flag. Intended as its own feature, built **before** `restore-host-terminal-windows`, which depends on it landing first.

### References

- Seed: `seeds/2026-06-30-warm-command-bootstrap-latch.md` (inbox:idea)
- Discovery: `discovery/session-001.md`
- Downstream dependent: `restore-host-terminal-windows` (review finding F1)

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
