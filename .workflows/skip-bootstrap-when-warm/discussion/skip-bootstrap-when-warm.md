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

  Discussion Map — Skip Bootstrap When Warm (9 subtopics — 1 decided · 1 converging · 7 pending)

  ┌─ → What "skip" means — classifying the 11 steps [converging]
  │  ├─ ✓ Protective steps stay on the warm path (EnsureSaver) [decided]
  │  └─ → Cleanup steps over a long-lived (weeks) server [converging]
  ├─ ○ Latch storage & semantics [pending]
  ├─ ○ Where & when the latch is set [pending]
  ├─ ○ Latch-check placement in the entry path [pending]
  ├─ ○ First-touch race tolerance [pending]
  ├─ ○ Partial-bootstrap / failure handling [pending]
  ├─ ○ Interaction with the cold+TUI concurrent path [pending]
  └─ ○ Edge cases & invalidation [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## What "skip" means — classifying the 11 steps

### Context

The seed framed the latch as all-or-nothing: latch set ⇒ skip all 11 steps. The first real design question is whether that's safe. It isn't uniformly — the 11 steps are not the same *kind* of work. Some are genuinely once-per-server-lifetime and categorically pointless on a warm server; others exist as ongoing safety nets that protect against mid-lifetime failure.

**The driving motivation (from the user):** this latch is the pretext for `restore-host-terminal-windows`' multi-select reopen — opening, say, 20 sessions at once each in its own Ghostty window. Opening *one* new window that runs a full bootstrap is fine; opening *20 simultaneously*, each firing the full orchestrator against one server, is a stability hazard. The goal is **not** shaving nanoseconds off a warm command — it's collapsing that concurrency surface so simultaneous warm commands do cheap checks instead of N concurrent restore/sweep/clean passes.

**Grounding — current warm-path reality (important):** today *every* warm command runs the full 11 steps synchronously (no loading screen — that's cold-only). The user runs `x` (= `portal open`) hundreds of times/day, each a full bootstrap, and it's fine. So a *single* warm bootstrap is **not** unsafe — the heavy steps are guarded/idempotent: Restore silently **skips already-live sessions** (`internal/restore/restore.go:170`, "steady-state common case"), so on a warm server it's a near-no-op that does not churn sessions. (An earlier framing that repeating these steps is "actively unsafe" was **overstated** and corrected — the only latent edge is a ~1s resurrection race if a session is killed *outside* the picker and `x` is run before the daemon's next tick captures the kill; pre-existing, rare, not this feature's concern.) The real drivers for skipping on warm are therefore (1) the **concurrent** 20× reopen burst, and (2) redundant per-command work — **not** correctness of a lone warm bootstrap.

**Hard constraint — long-lived servers.** The user routinely keeps a tmux server alive for **weeks**; server restarts are rare and must not be relied on for recovery. Anything that today self-heals on the *next command* (because bootstrap re-runs every command) must keep a path to self-heal within a single, possibly weeks-long, server lifetime. We cannot push recovery to "next server restart."

### The classification

Three classes, not two:

| Class | Steps | Warm-path behaviour |
|---|---|---|
| **1 — Cold-only** (genuinely once-per-lifetime, idempotent no-op when warm) | 1 EnsureServer, 2 RegisterPortalHooks, 3 SetRestoring, 4 SweepOrphanDaemons, 6 Restore, 7 EagerSignalHydrate, 8 ClearRestoring | **Skip when latched.** Server is up (latch died with it otherwise); hooks converged once and nothing re-adds them mid-lifetime; restore is a cold-boot concern; orphan-daemon sweep targets *prior-lifetime* leftovers (within a lifetime the daemon flock + self-supervision keep N≤1). |
| **2 — Protective liveness** (safety net against mid-lifetime death) | 5 EnsureSaver | **Keep on every command** as a cheap probe + re-ensure if down. Decided — see child below. |
| **3 — Cleanup / hygiene** (accrues over the lifetime) | 9 CleanStaleMarkers, 10 SweepOrphanFIFOs, 11 CleanStale (hooks) | **Open** — the weeks-long-server constraint makes "once per lifetime" insufficient. See child subtopic. |

### Decision (parent)

Reject the all-or-nothing skip. The latch gates **Class 1** only; **Class 2** stays on the warm path; **Class 3** is under discussion. This is the explicit "separate cold-start from warm-start" the user asked for. Confidence: high on Classes 1 and 2; Class 3 open.

---

## Protective steps stay on the warm path (EnsureSaver)

### Context

EnsureSaver (step 5) bootstraps/version-upgrades the `_portal-saver` session that hosts `portal state daemon`. Today it runs on *every* command, so it silently revives the daemon if it died mid-lifetime — the daemon's own self-supervision can `os.Exit(0)`, which tears down its pane and kills the `_portal-saver` session, and the next command's EnsureSaver brings it back. A naive latch (skip all 11) would remove that per-command safety net.

### Options Considered

- **A — Pure latch.** Skip all 11; saver revived only at server restart + the daemon's self-supervision.
  - Cons: with weeks-long servers, a self-ejected daemon could stay dead for *weeks* → silent loss of scrollback capture and resurrection-state. Directly violates the hard constraint.
- **B — Latch gates everything except a cheap saver-liveness check.** Warm commands skip Class 1 but still probe saver/daemon liveness (e.g. `SaverPanePIDOrAbsent`) and re-ensure if absent.
  - Pros: preserves today's self-healing; the probe is ~1 tmux call; the expensive re-create path only fires on the rare failure case, and the daemon flock serialises concurrent re-creation correctly.
- **C — Pure latch + harden the daemon so it never needs external revival.**
  - Pros: cleanest entry path. Cons: "never dies" is unachievable in practice ("all sorts of things can happen"); betting stability on it is fragile.

### Decision

**Option B.** Keep EnsureSaver (saver/daemon liveness) on the warm path as a cheap probe + conditional re-ensure. The user is emphatic: keep the fail-safe ("Our fail-safe is great to keep") *and* separately pursue making the daemon as robust as possible (belt **and** suspenders — B does not preclude C's hardening as ongoing work). Deciding factor: weeks-long server lifetimes mean we cannot lean on restart for recovery, and the probe's cost is negligible even under the 20-simultaneous-windows burst (a healthy saver ⇒ 20 cheap probes; a dead one ⇒ flock-serialised single re-create).

Note: on the warm path EnsureSaver runs **outside** the `@portal-restoring` window (no restore in flight), which is correct — the revived daemon should capture normally, not suppress.

---

## Cleanup steps over a long-lived (weeks) server

### Context

The weeks-long-server constraint raised a worry: cleanup steps 9 (CleanStaleMarkers), 10 (SweepOrphanFIFOs), 11 (CleanStale hooks) are framed as once-per-lifetime, but if cruft *accrues* during a weeks-long warm lifetime, skipping them on warm commands would let it pile up for weeks (the daemon does **not** clean these — confirmed: the daemon's only GC is `gcOrphanScrollback`, scrollback `.bin` files, inside `Commit`; markers/FIFOs/hooks cleanup live only in bootstrap + `portal clean`).

So the real question isn't "is cleanup important" — it's **"does a warm server actually produce new cleanup targets mid-lifetime?"** Traced each:

### The trace (what produces each cleanup target)

- **Skeleton markers (`@portal-skeleton-*`)** — `SetSkeletonMarker` is called from exactly **one** place: `internal/restore/session.go` during bootstrap step 6 restore. Nowhere else. A warm server creates **zero** new skeleton markers. Any stale ones are cold-boot restore leftovers, already cleaned by step 9 *during that same cold boot*. ⇒ Step 9 has **no mid-warm-lifetime workload**.
- **Hydrate FIFOs (`hydrate-*.fifo`)** — `CreateFIFO` is called from exactly **one** place: `internal/restore/session.go:217` during restore. A warm server creates **zero** new FIFOs. ⇒ Step 10 has **no mid-warm-lifetime workload**.
- **Hook entries (`hooks.json`)** — created by `portal hooks set` (user action, any time) and go stale when the keyed pane/session is killed (normal warm-server activity). This is the **only** Class-3 target a warm server genuinely produces over time.

### Options Considered (for the hooks step only — 9 & 10 are moot on warm)

- **Skip step 11 on warm too.** Dead hook entries (for killed sessions) accrue in `hooks.json` over weeks.
  - Harm: low — dead entries don't fire (their pane doesn't exist); they're plain JSON bloat. Cleaned at next cold boot, and `portal clean` is an explicit manual sweep. **Bonus:** skipping step 11 on warm *reduces* exposure to the known `bootstrap-cleanstale-wipes-hooks-on-tmux-transient` bug (which only triggers inside a bootstrap when `list-panes -a` returns transiently-empty).
- **Keep step 11 on warm.** Cleans dead hook entries promptly.
  - Cons: re-introduces the hooks-wipe bug surface on every warm command; runs a `list-panes -a` diff-and-delete on commands that mostly have nothing to clean; and most users have *zero* resume-hook entries (opt-in feature), so it's pure overhead in the common case.
- **Move cleanup into the daemon.** Make the lifetime-resident daemon prune stale hooks on its tick.
  - Cons: scope expansion; the daemon already deliberately stays out of the hooks store; only buys prompt cleanup of low-harm bloat. Better as a separate consideration if it ever matters.

### Can a stale hook *misfire*? (the "side effect" question)

The user's concern with skipping hook cleanup is **side effects**, not bloat. So: can a genuinely-stale hook entry ever fire on the *wrong* target?

The hook key is the structural key `#{session_name}:#{window_index}.#{pane_index}` (`tmux.StructuralKeyFormat`, e.g. `myproj-AbC123:0.0`). Session names are `{project}-{nanoid}` and `GenerateSessionName` **guarantees uniqueness**. A "stale" entry = a key not present in the live pane set. For that key to become live again, a session with that exact nanoid-bearing name must exist again — which only happens when Portal **restores that same saved session** (same identity) after a reboot, and firing then is the hook's *intended* behaviour, not a misfire.

- A different, newly-created session gets a **new** nanoid ⇒ new key ⇒ never collides with the stale entry.
- Within-session index reuse (`window.pane` recycled by a new pane in a *surviving* session) keeps the key **live**, so it's never classed as stale — that's a separate positional-key property of the hooks feature, orthogonal to cleanup timing, and unfixable by cleaning stale entries anyway.

**Conclusion: a genuinely-stale hook entry cannot fire on the wrong session.** The only cost of leaving it is inert JSON bloat. (Confidence: high, modulo a user manually recreating a session under an old nanoid name by hand — not a realistic path.)

### Decision (parent still open — hooks-cleanup home under discussion)

Steps 9 and 10: **skip on warm**, decided — a warm server produces none of their targets.

Step 11 (hooks): the user wants cleanup **not to wait**. Given the misfire trace above the risk is bloat, not misfiring — but the user's preference stands. With the corrected grounding (cleanup runs on every `x` today and is harmless), the decision is just *which commands keep running it once we optimize the warm path*:

- **Command-classified — cleanup on `open`, skipped on `attach`.** `open`/`x` is run hundreds of times/day, single-invocation (never bursted), and already makes the user wait for a UI. Keeping a cleanup pass there is essentially **status quo for `open`**; the change is dropping it (and the heavy steps) from the bursty `attach` path. Cleanup stays prompt; zero daemon scope; the only cost is a small command-name branch in the entry path. Bounds: cleanup cadence is coupled to how often `open` is run (a non-issue for this user).
- **Keep cleanup on *all* warm commands (incl. `attach`).** ⚠️ **Anti-recommended** — re-inflates the exact concurrency surface the feature removes: 20 burst `attach` each running a `list-panes -a` diff + atomic `hooks.json` rewrite (the `bootstrap-cleanstale-wipes-hooks-on-tmux-transient` surface).
- **Daemon-owned cleanup.** Lifetime-resident single writer that already has the live pane set each tick; prompt cleanup, uniform light path for *all* warm commands (optimizes `open` fully too), no `hooks.json` write contention. Scope addition (daemon gains a guarded hooks-store write duty); most robust, decouples cadence from command usage.

Leaning: **command-classified (cleanup on `open`)** — proportionate to the low stakes, near-zero scope, strictly fewer cleanup runs than today, and it fully satisfies "don't wait" because `x` runs constantly. **Daemon-owned** is the pick only if we want cleanup fully decoupled from command usage and are willing to grow the daemon.

---

## Summary

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- **Decided:** reject all-or-nothing skip; EnsureSaver (step 5, protective liveness) stays on the warm path.
- **Converging (awaiting confirm):** skip all of Class 3 (steps 9/10/11) on warm — 9 & 10 produce nothing on a warm server; 11 (hooks) is low-harm + bug-reducing to skip. Net scope: **latch skips everything except step 5**.
- Mechanism (server-option latch), set-point, check placement, race/failure handling, concurrent-path interaction, and edge cases all still pending.

## Triage

(none)
