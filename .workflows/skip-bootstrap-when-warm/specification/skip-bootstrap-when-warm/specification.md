# Specification: Skip Bootstrap When Warm

## Overview & Goals

### Problem

Portal's bootstrap orchestrator (`cmd/bootstrap`, run from `cmd/root.go`'s `PersistentPreRunE`) fires on **every** command not in the `skipTmuxCheck` allow-list. Its meaningful work — `EnsureServer`, `RegisterPortalHooks`, `SweepOrphanDaemons`, `EnsureSaver`, `Restore`, and the cleanup sweeps — is logically a **once-per-tmux-server-lifetime** concern, run once-per-command purely defensively: Portal has no "has this server already been bootstrapped this lifetime?" signal, so each command re-ensures the whole world. On a warm server these steps are idempotent no-ops — redundant work plus an avoidable concurrency surface when N commands hit the server near-simultaneously.

### Goal

Set a single **server-scoped latch** at the end of a successful bootstrap. Later commands in that server lifetime see the latch and take a cheap **abridged** path instead of re-running the full orchestrator. First (unlatched) touch runs the full bootstrap and sets the latch; latched commands do a cheap latch-check and skip.

### Motivation (what this is and isn't for)

- **Primary driver — collapse the concurrency surface.** This latch is the pretext for the downstream `restore-host-terminal-windows` feature, whose multi-select reopen spawns N−1 windows each running `portal attach <session>` (and `attach` is *not* in `skipTmuxCheck`). A 20-window post-crash rebuild would otherwise fire ~20 near-simultaneous full bootstraps against one server — a stability hazard. The latch collapses that to ~20 cheap latch-checks.
- **Secondary driver — stop redundant per-command work.** Every warm command stops re-running restore/sweep/clean.
- **Explicit non-goal — single-command safety.** A lone warm bootstrap is *already* safe today: `Restore` skips already-live sessions (`internal/restore/restore.go`), so on a warm server it is a near-no-op. The feature is **not** about correctness of one warm bootstrap; it is about concurrency and redundancy.

### Hard constraint — long-lived servers

The user routinely keeps a tmux server alive for **weeks**; server restarts are rare and must not be relied on for recovery. Anything that self-heals today on the *next command* (because bootstrap re-runs every command) must keep a path to self-heal within a single, possibly weeks-long, server lifetime. Recovery cannot be pushed to "next server restart."

### Scope

- A **version-stamped server-option latch** (`@portal-bootstrapped`) gating a **full** vs a single **abridged** bootstrap path.
- The abridged path runs a **liveness-only EnsureSaver** and nothing else.
- **Hooks stale-cleanup (former step 11) is removed from the orchestrator entirely** and re-homed on the `_portal-saver` daemon (orchestrator drops from 11 → 10 steps).

### Dependency

This feature is built **before** `restore-host-terminal-windows`, which depends on it landing first. It surfaced as review finding **F1** during that feature's discussion. Once landed, reopen can spawn plain `portal attach` with no special bootstrap-exempt command or hidden flag.

---

## Working Notes

_Optional - capture in-progress discussion if needed._
