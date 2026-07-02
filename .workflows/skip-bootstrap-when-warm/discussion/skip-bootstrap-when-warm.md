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

  Discussion Map — Skip Bootstrap When Warm (12 subtopics — all decided)

  ┌─ ✓ Full vs Abridged bootstrap — classifying the 11 steps [decided]
  │  ├─ ✓ EnsureSaver on abridged path = liveness-only (version-gate → full bootstrap) [decided]
  │  ├─ ✓ Hooks cleanup home → the _portal-saver daemon [decided]
  │  └─ ✓ "Full"/"Abridged" naming & single-abridged constraint [decided]
  ├─ ✓ Latch storage & semantics — version-stamped server option [decided]
  ├─ ✓ Latch set-point & timing (final action of a successful Run) [decided]
  ├─ ✓ Latch-check placement + abridged wiring (single read; version-match branch) [decided]
  ├─ ✓ First-touch race window — end-set collapses reopen-burst; pure cold-burst tolerated [decided]
  ├─ ✓ Partial-bootstrap / soft-vs-fatal failure handling (soft latches, fatal doesn't) [decided]
  ├─ ✓ Full-bootstrap concurrent/loading-path interaction (set inside Run) [decided]
  ├─ ✓ Edge cases & latch invalidation (version-stamp; self-heal; F1/F2/F5/F8) [decided]
  └─ ✓ Test strategy for verifying the skip [decided]

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

### Naming & the single-abridged constraint (user directive)

- **Terminology:** use **full bootstrap** vs **abridged bootstrap**, *not* cold/warm — "cold/warm" collides with "is the tmux server running." The real trigger is the **latch** ("has Portal bootstrapped *this* server yet"), which usually coincides with server-was-off but isn't identical (a hand-started tmux server + `x` has no latch → gets the full bootstrap).
- **One abridged version only.** The user explicitly rejects multiple abridged variants (e.g. an `open`-flavour that cleans + an `attach`-flavour that doesn't). There is exactly one abridged path, run identically by every command against an already-bootstrapped server.
- **Same orchestrator, two invocation modes (grounding).** Full and abridged are not different programs — the full path is the existing `Orchestrator.Run`; on a cold `portal open` it runs concurrently behind the loading screen (slow: start server + restore N sessions), otherwise synchronously. The loading screen is a slow-path wrapper, not a distinct bootstrap.

### Decision (parent)

Reject the all-or-nothing skip. Split into two named paths:

- **Full bootstrap** (latch absent): all 11 steps, then set the latch.
- **Abridged bootstrap** (latch present): **EnsureSaver liveness probe only** (Class-2 protective) — the single, uniform reduced path. Everything in Class 1 is skipped; Class-3 cleanup is removed from the per-command path entirely and homed on the daemon (see child).

Confidence: high. This is the explicit "separate full from abridged" the user asked for, with a single abridged version.

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

**EnsureSaver has two duties — and the version-stamped latch splits them across the two paths (SUPERSEDES the earlier "abridged keeps full EnsureSaver" framing).** EnsureSaver = (a) *liveness* — create `_portal-saver` + daemon if absent (`BootstrapPortalSaver`); and (b) *version-gate* — if the running daemon's binary is stale, kill + recreate it on the new binary via a guarded kill-barrier (`EnsurePortalSaverVersion`).

- Originally (when the latch was going to be presence-only) we required the abridged path to run the **full** EnsureSaver, because otherwise a stale-version daemon could survive a binary upgrade for a weeks-long lifetime (that was review F6).
- **The version-stamped latch changes this.** A *satisfied* latch (present **and** version-matching) already proves the running daemon is the current binary — so on the abridged path EnsureSaver reduces to a pure **liveness** probe (`SaverPanePIDOrAbsent` + re-ensure if absent). The **version-gate lives only in the full bootstrap**, which a version bump now triggers (latch mismatch → full bootstrap → recreate daemon on new binary → re-stamp).

**Final decision: abridged EnsureSaver = liveness-only.** "Liveness-only" is **not** a reduction in crash/death recovery — it still fully covers a daemon that crashes or dies mid-lifetime: on every warm command the abridged path asks "is the `_portal-saver` daemon alive?" (`SaverPanePIDOrAbsent`) and recreates it if absent. A random daemon crash → pane process exits → `_portal-saver` session gone → next warm command's liveness probe revives it. That is the exact fail-safe from the original "keep EnsureSaver on the warm path" decision, preserved unchanged. The **only** thing dropped from the abridged path is the redundant *version* re-check (the version-gate), because a satisfied version-stamped latch already proves the daemon's binary version; the version-gate kill-barrier now runs solely in the full bootstrap that a version-mismatch triggers.

This is simpler than the earlier framing *and* strictly safer on concurrency: it dissolves review F3 (no warm command ever runs the version-gate kill-barrier, so N concurrent warm commands can never race to kill-barrier a stale daemon — the single post-upgrade full bootstrap does the recreate once). Liveness re-ensure of a genuinely-absent saver stays serialised by the daemon flock as before.

Daemon has two independent safety nets, both preserved: (1) **self-supervision** — the daemon self-ejects if it detects it is no longer the legitimate `_portal-saver` pane (split-brain guard); (2) **abridged liveness revival** on every warm command (+ full bootstrap on cold boot / upgrade). Confidence: high.

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

### Decision — daemon-owned cleanup (DECIDED, in-scope)

Steps 9 and 10: skipped on the abridged path, decided — a warm server produces none of their targets (they stay in the full bootstrap for cold-boot leftovers).

Step 11 (hooks): the **single-abridged-version constraint forces the resolution**. Command-classified cleanup (the earlier "cleanup on `open`, not `attach`" idea) is exactly the multiple-abridged-variants the user rejects — dropped. Keeping cleanup in the one abridged path means the 20× `attach` burst runs it (the anti-recommended `list-panes -a` + `hooks.json` rewrite concurrency surface). So cleanup can live in **neither** abridged variant. It moves out of the per-command path entirely and onto the **`_portal-saver` daemon** (`portal state daemon`). **User confirmed: in-scope for this feature.**

**Operational contract (resolves review F4):**

- **Home:** the existing background process inside the hidden `_portal-saver` tmux session — **not launchd** (previously rejected, not reopened).
- **Reuse, don't reinvent:** the daemon calls the existing shared `cmd/run_hook_stale_cleanup.go` `runHookStaleCleanup` helper. That helper already carries the **mass-deletion hazard guard** (`len(livePanes)==0 && hooks present` → skip + WARN, never wipe) and drives `hooks.Store.CleanStale`, which emits the existing `EmitCleanStaleSummary` **audit breadcrumb** — so no new audit event/vocabulary is invented.
- **No layering problem:** `runHookStaleCleanup` and the daemon (`cmd/state_daemon.go`) are both in package `cmd` — same package, so no new import and **no cycle**. (The "daemon stays out of the hooks store" note was a soft observation, not a hard boundary.)
- **Cadence (user directive):** *not* every 1s tick. Throttled to ~10s via a cheap `time.Since(lastCleanup) >= interval` check evaluated per tick; the cleanup body fires only when the interval has elapsed. Exact interval is a tuning detail (10s default). Rationale: the 1s tick must stay light (capture/scrollback save is the priority and can exceed 1s); stale hooks are inert so precise timing is irrelevant. (Lazier alternative noted, not chosen: trigger cleanup only when the live-session set shrinks.)
- **Priority / non-interference:** cleanup never competes with a pending capture — the tick loop is single-threaded and already skips entirely while `@portal-restoring` is set and on the `!dirty && !gap` idle fast-path; cleanup is gated so scrollback saving always wins.
- **Failure posture:** log WARN and retry next cadence (mirrors the tick loop's existing "tick failed" handling); a cleanup error never escalates or crashes the daemon.

Confidence: high. Contract fully specified; only the numeric interval is a tuning detail left to implementation.

---

## Latch storage & semantics

### Decision

`@portal-bootstrapped` as a tmux **server option**, storing the **binary version** as its value (a *version-stamped* latch, not a bare presence flag): set via `SetServerOption(@portal-bootstrapped, <version>)`, read via `TryGetServerOption`. Same server-option mechanism as `@portal-restoring` (dies with the tmux server → server restart auto-clears it → next command full-bootstraps), reusing the `internal/state/markers.go` seam vocabulary (`RestoringChecker` / `ServerOptionWriter`), but the value is load-bearing rather than presence-only.

**Latch is "satisfied" only when present *and* its stored version equals the running binary's `cmd.version`.** Absent → full bootstrap (cold or fresh). Present-but-mismatched → full bootstrap (post-upgrade). Present-and-matching → abridged. A single read (`TryGetServerOption`) drives all three: an error/down-server read or a value mismatch both resolve to "not satisfied → full bootstrap." (User: "single read is fine.")

**Why version-stamped, not presence (resolves review F4/F6/F7):** the user upgrades portal often (brew) and restarts tmux rarely (weeks). A presence-latch would keep `RegisterPortalHooks` (step 2) from ever re-running mid-lifetime, so a new binary's changed global-hook bodies would silently lag the installed version until a tmux restart — weeks. Version-stamping makes a release upgrade re-converge hooks + recreate the daemon on the first post-upgrade command, then re-stamp. The user endorsed this: "on upgrade, we will always run a full bootstrap, which will then reset the marker with the new version … it self-heals." The stored value also answers F7 (forensics): at minimum the version; optionally set-timestamp / pid as cheap additions (value shape is an implementation detail — version is the load-bearing field).

**Dev-build nuance (accepted):** local/unversioned builds carry a constant version string, so version-stamping only re-bootstraps on real version bumps (releases), not local rebuilds. The user rarely runs local builds ("it messes with the brew-installed version, not easily isolatable"), so this is a non-issue; testing local hook changes uses the escape hatch (`tmux set-option -u @portal-bootstrapped`).

Confidence: high. Decided with the user.

---

## Latch set-point & timing (the crux)

### Context

The review (F1/F2/F7) isolated this as the load-bearing decision: a full bootstrap can take seconds (it restores N sessions), so the window between "full bootstrap starts" and "latch set" is where all the concurrency/atomicity risk lives.

### Decision

**Set the latch as the final action of a *successful* `Orchestrator.Run` — after step 11, gated on no fatal error.** Three consequences, all agreed by the user:

1. **Atomic-with-success, uniform across both invocation modes (retires F2).** The latch is set *inside* `Run`, not by the two callers, so the synchronous path and the concurrent cold+TUI goroutine both get it identically — no second set-point to keep in sync. "Latch present" ⟺ "a full bootstrap ran to completion."
2. **Set at the *end*, not early — safe and sufficient.** Early-setting (e.g. right after the server is up) is **unsafe**: a concurrent command would see the latch and take the abridged path *before Restore recreated the sessions*, then attach to a session that doesn't exist yet. End-setting is **sufficient** for the target scenario because the reopen burst can't fire until the user multi-selects in the picker, and the picker only appears *after* bootstrap completes (loading screen on cold, synchronous on warm) — so by the time 20 `attach` fire, the latch is already set and they all take the abridged path.
   - **Explicitly accepted non-goal:** a *pure cold-burst* — N commands hitting a genuinely serverless tmux simultaneously, *not* via the picker — is **not** collapsed by end-setting. That isn't the reopen flow, and it's already tolerated today (daemon flock + idempotent hook convergence). We accept it rather than complicate the set-point.
3. **"Successful" = no *fatal* error; soft warnings still latch (the F1 answer).** A soft-step warning (`SaverDownWarning`, `CorruptSessionsJSONWarning`, partial restore) still sets the latch, because those either self-heal on the abridged path (EnsureSaver re-probes every command) or are non-retryable (a corrupt file won't un-corrupt next command). Requiring a totally-clean run would let one transient `SaverDownWarning` force every command back to full bootstrap for the whole server lifetime — defeating the feature. Only a **fatal** step (steps 1/2/3/8, which already abort with a non-zero exit / red TUI frame) leaves the latch **unset**, so the next command correctly retries the full bootstrap.

**Bonus (retires F5 & F8):** because the latch is set only after step 7 (EagerSignalHydrate) and step 8 (Clear `@portal-restoring`) have run, "latch present" *guarantees* hydrate signalling finished and `@portal-restoring` was cleared. So the two markers can never both be set on an abridged command (F5 two-marker inconsistency), and a late-arming skeleton pane can't be stranded unsignalled (F8) — both fall out of the ordering with no extra logic.

Confidence: high. User: "exactly the same decisions as I would have made."

---

## Latch-check placement + abridged-path wiring

### Context

Where the latch-check sits in the entry path, how the abridged path plugs into existing plumbing, and (F3/F9) how `serverStarted` and warnings behave when no full orchestrator runs.

### Decision

**Placement — a single latch-read drives a three-way branch.** In `PersistentPreRunE`, after the tmux client is built, read the latch (`TryGetServerOption(@portal-bootstrapped)`) and compare its value to the running binary version:

- **latch satisfied** (present **and** version matches) → abridged path.
- **latch not satisfied** (absent, unreadable/down-server, **or** version-mismatch) → full bootstrap: concurrent + loading screen on the TUI path (`open`, no args), synchronous otherwise.

A separate `ServerRunning()` probe is not required — the latch-read fails gracefully on a down server, so "unreadable" folds into "not satisfied → full bootstrap." Single read chosen for minimalism (user: "single read is fine").

**Loading-screen trigger moves from server-down → latch-absent (user refinement).** The concurrent/loading path now fires whenever a *full* bootstrap runs on the TUI path — keyed off latch-absent, not server-down. This retires the warm-unlatched edge as an improvement: a hand-started tmux server + `x` now gets the loading screen + progress during its first full bootstrap instead of a synchronous no-progress stall. Conceptual cleanup: "loading screen" now means exactly "a full bootstrap is in progress." No change to *what* the full bootstrap does (Restore etc. already ran on warm-unlatched today) — only the presentation improves.

| Command | Latch | Outcome |
|---|---|---|
| `open` (no args) TUI | not satisfied (absent / version-mismatch) | full bootstrap, concurrent + loading screen |
| `open` (no args) TUI | satisfied (present + version match) | abridged (sync plumbing, instant picker) |
| `attach` / `open <path>` / CLI | not satisfied | full bootstrap, synchronous |
| `attach` / CLI | satisfied | abridged (sync plumbing) |

**Abridged wiring reuses the sync plumbing (resolves F3 & F9).**

- **F3 — `serverStarted=false`** is injected (correct: the command did not start the server). Its *sole* production consumer is `openTUI`'s loading-page gate → `false` → no loading page → instant picker, which is exactly right for a warm command. No hidden "third state" to disambiguate.
- **F9 — warnings.** EnsureSaver's `SaverDownWarning` funnels into the same package-level `bootstrapWarnings` sink the sync path already uses → CLI flushes to stderr; TUI drains to the notice band. Identical to a warm command today; no new emission mechanism.
- **Shape (constraint, not prescription):** the abridged path runs through the *same* entry-path plumbing (warning sink + context injection) as the sync full path, differing only in executing a reduced step set (EnsureSaver only). This is what makes F3/F9 inherit the existing, tested handling.

Confidence: high. User confirmed placement, reuse-the-plumbing shape, and the loading-screen refinement.

---

## Edge cases & latch invalidation

### Context

`@portal-bootstrapped` is a *persistent* lifetime latch (unlike the transient `@portal-restoring`), so its failure/staleness modes need explicit treatment. Guiding principle (user): **don't program around anything that self-heals via an idempotent no-op full bootstrap.**

### Decisions

- **Auto-invalidation by design.** The latch is a server option → dies with the server → restart auto-clears it → next command full-bootstraps. No explicit invalidation code.
- **Upgrade invalidation.** Version-mismatch is treated as "not satisfied" → the first post-upgrade command full-bootstraps (re-registers hooks, recreates the daemon on the new binary) and re-stamps. Self-healing; no special-casing.
- **Two markers can't both be set.** The latch is set *last* (after step 8 clears `@portal-restoring`), so "latch satisfied" ⇒ restoring was cleared. A crash mid-bootstrap leaves the latch unset → next command full-bootstraps and re-clears any leaked restoring marker. No inconsistent state reachable on a steady server.
- **Latch-set write failure (resolves review F2).** The terminal `SetServerOption(@portal-bootstrapped, version)` is **best-effort**: on failure, log WARN and swallow. Consequence: the next command reads "not satisfied" → re-runs the (idempotent, near-no-op on warm) full bootstrap → retries the write. Self-heals; **never fatal**. Consistent with the don't-program-around-self-healing principle.
- **Manual escape hatch.** `tmux set-option -u @portal-bootstrapped` forces the next command back to a full bootstrap — handy for debugging or forcing a re-converge without a tmux restart.
- **Abridged EnsureSaver hard-failure (resolves review F8).** With the version-gate moved off the abridged path, abridged EnsureSaver is liveness-only; a failure to re-ensure an absent saver surfaces as a soft `SaverDownWarning` (via the existing sink) and the command **proceeds** — attach/switch still works; capture simply resumes on the next successful revival. No kill-barrier runs on the abridged path, so there is no kill-barrier-failure branch to handle there (it lives in the full bootstrap, already a soft step).

### Accepted residues (harmless bloat — reviewed & tolerated)

- **Cold-boot cleanup leftovers (review F1).** If a cold boot's steps 9/10 (marker/FIFO cleanup) soft-fail, that residue isn't retried until the next full bootstrap. Accepted: markers/FIFOs are inert (the daemon-merge live-set filter already prevents dead-session resurrection), and version-stamped upgrades now give *extra* full-bootstrap cleanup passes beyond just restarts.
- **Daemon-death vs cleanup home (review F5).** Step-11 hooks cleanup was relocated from an always-runs path (per-command bootstrap) to a conditionally-alive one (the daemon) — a named trade. Backstop: cleanup still runs in every *full* bootstrap (cold boot + each upgrade), and the abridged path's liveness EnsureSaver revives a dead daemon. Worst case (daemon dead *and* revival failing) leaves only inert hooks bloat until the next full bootstrap. Accepted given the misfire trace (stale hooks can't fire on the wrong session).

Confidence: high. All review-002 mechanism findings (F1–F8) resolved or explicitly accepted.

---

## Test strategy for verifying the skip

### Context

The feature's value is *not running* steps, which is harder to assert than running them, and the blast radius is load-bearing core machinery — so the test shape is worth settling before implementation (review F10 flagged that a testable design may feed back into the mechanism decisions; it does — see "design-for-test" below).

### Decision (shape approved by user)

- **Branch selection (unit, seam-mocked).** The orchestrator + steps are already `bootstrapDeps`-injected. Set `@portal-bootstrapped` on a fake client to {absent, version-match, version-mismatch} and assert: *satisfied* → only EnsureSaver invoked, Restore/Sweep/CleanStale **not** invoked; *not satisfied* → full `Run` **and** latch ends stamped with the current version. Assert via seam call-recording.
- **Set-point gating.** Inject a soft-warning step → assert latch **is** set; inject a fatal step → assert latch **unset**. Directly nails the soft-vs-fatal rule.
- **Abridged self-heal (crash-recovery regression guard).** Latch satisfied + saver dead → assert the abridged liveness EnsureSaver revives it. This is the explicit guard for the "keep the fail-safe" thread.
- **Daemon cleanup.** Unit-test the throttled cadence gate (`time.Since(lastCleanup) >= interval`); the cleanup body is the existing `runHookStaleCleanup` (already covered, guard included).
- **Integration (real tmux).** Extend `cmd/concurrent_*_test.go` + `tmuxtest` socket fixtures, **under `IsolateStateForTest`** (mandatory for daemon-spawning tests): warm+satisfied command skips restore but revives a killed saver; a version-mismatch latch triggers a full re-bootstrap that re-stamps.
- **Design-for-test.** Make the "current version" **injectable** (it is `cmd.version`) so a version-mismatch branch is unit-testable without rebuilding the binary.

Confidence: high. User: "The test strategy sounded okay to me."

---

## Summary

### Key Insights

1. **Almost all "cleanup" is restore-window (cold-boot) debris, not warm-server output.** Tracing each cleanup target (`SetSkeletonMarker`, `CreateFIFO`) to its single call site — restore — collapsed the weeks-long-server worry: steps 9/10 have zero warm workload; only step-11 hooks accrue mid-lifetime.
2. **The version-stamped latch is the linchpin.** Storing the binary version (not a bare presence flag) makes the latch a version-aware gate that (a) auto-applies release upgrades via a full re-bootstrap, (b) lets the abridged path shed the version-gate down to a pure liveness probe, and (c) carries forensic metadata — resolving three review findings (F4/F6/F7) at once and dissolving a concurrency concern (F3).
3. **Set-point-by-ordering.** Setting the latch as the *final* action of a successful `Run` (soft warnings still latch; only fatal steps don't) makes "latch satisfied ⟺ a full bootstrap ran to completion past step 8," retiring a whole cluster of atomicity/ordering/two-marker concerns for free.
4. **Motivation is concurrency + redundancy, not single-command safety.** A lone warm bootstrap is already safe today (Restore skips live sessions); the feature exists to collapse the reopen burst's N concurrent full bootstraps and to stop redundant per-command work.
5. **Self-heal principle (user).** Don't program around anything that recovers via an idempotent, no-op full bootstrap — latch-write failure, post-upgrade mismatch, and crash recovery all lean on it.

### Open Threads

- None blocking. Two implementation-tuning details deliberately left open: the daemon hooks-cleanup cadence interval (~10s default) and any optional forensic extras in the latch value beyond the version (set-timestamp / pid).

### Current State

- **Decided:** two named paths — **full bootstrap** (all 11, sets latch) vs a single **abridged bootstrap** (full EnsureSaver — liveness **and** version-gate — only). Class-1 heavy steps skipped when latched.
- **Decided:** hooks cleanup (step 11) moves to the **`_portal-saver` daemon**, in-scope for this feature. Contract fixed: reuse `runHookStaleCleanup` (inherits mass-delete guard + audit breadcrumb; same `cmd` package → no cycle), ~10s throttled cadence off the 1s tick, WARN-and-continue failure posture.
- **Decided:** naming full/abridged (not cold/warm); the latch is the switch; EnsureSaver keeps its version-gate on the abridged path (F6).
- **Decided (mechanism):** version-stamped `@portal-bootstrapped` server option; latch set as the final action of a successful `Run` (soft warnings still latch, fatal doesn't); single-read three-way branch (satisfied→abridged, else full — concurrent+loading on TUI); loading-screen keyed on latch-not-satisfied; abridged EnsureSaver = liveness-only (version-gate → full bootstrap on version-mismatch).
- **Reviews folded in:** set 001 (F1–F10) and set 002 (F1–F8) all resolved or explicitly accepted.
- **All 12 subtopics decided.** Test strategy approved. Two reviews (001, 002) incorporated; final review pass pending before conclusion.

## Triage

(none)
