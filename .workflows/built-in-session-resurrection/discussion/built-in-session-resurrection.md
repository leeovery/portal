# Discussion: Built-in Session Resurrection

## Context

Portal should own the full session lifecycle: server start → session restoration → resume hook execution. Currently the middle step depends on tmux-resurrect/continuum, which has a 100% failure rate — sessions never come back after reboot. The resume hook feature is effectively broken end-to-end despite the code being correct, because the session structure it depends on doesn't exist.

Research has confirmed full technical feasibility. tmux provides all the APIs needed for capture (`list-panes -a -F`) and restore (`new-session`, `split-window`, `select-layout`). The question is no longer *can we do this* but *how should we design it*.

Key design principles established in research:
- Portal's hook system is generic — no awareness of what consumers do with it
- Portal doesn't maintain a separate session registry — reads tmux directly
- Portal captures all sessions (Portal-created and native tmux), consistent with existing behavior
- Portal is always the entry point — bootstrap is the natural place for restoration

### References

- [Research: Built-in Session Resurrection](./../research/built-in-session-resurrection.md)

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Hook Lifecycle Redesign [decided]
  ├─ One-shot vs persistent hooks [decided]
  └─ Per-hook configurability [decided]

  Save Content & Scope [decided]
  ├─ Structural state capture [decided]
  ├─ Scrollback / pane contents capture [decided]
  ├─ Ephemeral interaction state exclusion [decided]
  ├─ History size policy (no artificial caps) [decided]
  └─ Security / file permissions [decided]

  Save-Side Architecture [decided]
  ├─ Execution model (detached tmux session host) [decided]
  ├─ Trigger mechanism (event-driven + 30s periodic; opportunistic dropped) [decided]
  ├─ Crash safety / periodic save cadence (30s max-gap) [decided]
  ├─ Signal handling (SIGHUP from PTY close, SIGTERM for direct kill) [decided]
  ├─ Debouncing / serialization (single-writer via hosted process + dirty flag) [decided]
  ├─ Save format and schema (per-pane scrollback files + sessions.json index) [decided]
  ├─ Content-hash dedup (skip unchanged scrollback writes) [decided]
  ├─ CLI surface (state status only; daemon + notify internal) [decided]
  └─ tmux hook registration lifecycle [decided]
      ├─ Fresh-server bootstrap [decided]
      ├─ Subsequent invocation on bootstrapped server [decided]
      ├─ Portal upgrade with running server [decided]
      ├─ Portal uninstall with running server [decided]
      ├─ Portal binary replaced (brew upgrade) [decided — composed from 3+4]
      ├─ User restarts tmux server [decided]
      └─ Hook collision with other plugins [decided]

  Restore-Side Architecture [decided]
  ├─ Restoration trigger (restore all, name-idempotent) [decided]
  ├─ Skeleton vs content split (skeleton-eager, scrollback-lazy via hook) [decided, amended]
  ├─ Marker coordination (awaiting-hydration + restoring-in-progress) [decided, amended]
  ├─ Scrollback restore mechanics (blocking helper pre-shell via FIFO) [decided]
  ├─ Shell readiness + hook firing redesign (helper exec chain) [decided]
  ├─ Layout restoration approach [decided]
  ├─ Fate of WaitForSessions / bootstrapWait [decided]
  └─ Bootstrap integration (full flow diagram) [decided]

  Failure Modes & Recovery [decided]
  ├─ Corrupt / partial saved state [decided]
  ├─ Missing scrollback files on restore [decided]
  ├─ Layout fit failures (terminal size drift) [decided]
  ├─ Disk full during save [decided]
  └─ User-race scenarios during restoration [decided]

  Observability & Diagnostics [pending]
  ├─ Save-state introspection command [pending]
  ├─ Logging strategy [pending]
  └─ Health signals for silent failures [pending]

  CleanStale Guard Behavior [decided]
  ├─ Guard removed (no longer needed under new design) [decided]
  └─ Stale-hook detection criteria (structural-key match against live panes) [decided]

  Session & Project Store Interaction [pending]
  ├─ Restored session naming [pending]
  └─ projects.json timestamp handling [pending]

  Ephemeral Session Opt-Out [pending]

  Scope Boundaries [pending]
  ├─ Environment / shell state (explicit non-goal) [pending]
  └─ tmux version compatibility [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent.*

---

## Hook Lifecycle Redesign

### Context

The resume hook system fires stored commands when a user enters a session. Research flagged "one-shot vs persistent" as a design decision — do hooks fire once and delete themselves, or persist across reboots until explicitly removed?

Current implementation is effectively persistent: entries live in `hooks.json` and survive reboots; volatile markers (`@portal-active-<pane>`) prevent duplicate runs within a tmux server lifetime. The research proposed making this configurable per-hook.

### Options Considered

**A: Add a `mode` field — configurable per-hook (`once` vs `always`)**
- Pros: Expressive; user declares intent; `once` auto-cleans after firing so broken hooks fail only once instead of every reboot.
- Cons: Adds API surface, CLI flag, storage field, docs, test matrix. Two more states for users to reason about.

**B: Single behavior — persistent only (status quo)**
- Pros: Minimal mechanism. Matches Portal's generic-hook principle. No new fields.
- Cons: Callers wanting one-shot semantics must implement it themselves at the command level.

### Journey

Initial framing was that `once` and `always` solve different use cases — `once` for dynamic commands like `claude --resume <uuid>` (where a wrapper re-registers on each start), `always` for static commands like `npm start`. Proposed adding a mode field, with the semantic anchor being "survives reboot yes/no."

First clarification: "session alive" meant the *logical* session (same named session across reboots), not the tmux session instance. Detach/reattach within a server lifetime was raised as an edge case but is a non-issue — existing volatile markers handle it correctly because processes are still running and nothing needs restarting. The only scenario where `once` and `always` would behave differently is reboot recovery.

Naming settled early: `once` / `always` — maps cleanly to `--mode=once` CLI flag, matches user mental model ("run this once" vs "always run this when I come back").

Then the user pushed back with a use case audit. For their Claude setup, *both* modes work — the Claude wrapper re-registers a `once` hook on resume, AND a separate exit hook removes the `always` version on explicit Claude exit. That prompted the pivotal question: if both modes work for the flagship use case, what is `once` actually for?

Audit of use cases:
- **Static dev commands** (`npm start`, `tail -f`, file watchers): `always` only; `once` makes no sense.
- **Claude resume** (dynamic UUID): both work.
- **Ephemeral one-time tasks**: `once` slightly cleaner, `always` + manual removal works.
- **Stale hook hygiene** (broken hook fails once vs every reboot): minor win for `once`.

No slam-dunk use case for `once`. The decisive argument came from re-reading the generic-hook design principle from research:

> Portal's hook system is generic. No awareness of what consumers do with it. Portal stores and fires a command string — it's the caller's responsibility to make that command correct.

One-shot vs persistent is *policy*. Portal provides the *mechanism*. If a caller wants one-shot behavior, they implement it at the command level — not inside Portal.

### False path: `&&` chaining

An initial framing proposed that one-shot callers could self-remove via shell chaining:

```
portal hooks set --on-resume "my-cmd && portal hooks rm --on-resume"
```

**This doesn't work for the flagship use case.** The canonical hook commands are long-running processes — `claude --resume <uuid>`, `npm start`, `tail -f`. These never exit, so the `&&` clause never fires, and the hook never removes itself. The proposed pattern was architecturally broken for the exact class of commands hooks exist to serve.

Verified against the codebase: the actual CLI is `portal hooks set --on-resume "..."` and `portal hooks rm --on-resume`, both inferring the current pane from `TMUX_PANE` and keying hooks by structural key (`session:window.pane`). The API shape is fine; shell chaining is not.

### The actual caller pattern: wrapper-script lifecycle management

The correct model — and the one the user already described from their Claude setup — is that long-running processes are invoked by a wrapper script which *owns* the hook lifecycle:

- Wrapper registers a Portal hook when the process starts (using current state, e.g., resume UUID)
- Wrapper re-registers on each resume if the hook command is dynamic
- Wrapper removes the hook on explicit process exit (via exit trap or explicit teardown)

Portal is never involved in deciding when to remove; it just exposes `set`/`rm` primitives that the wrapper calls at the appropriate lifecycle moments. This keeps Portal fully generic while giving callers precise control.

### Decision

**Do not add a `mode` field.** Portal keeps its single behavior: hooks persist in the store across reboots until explicitly removed via `portal hooks rm`. Callers that want one-shot or bounded-lifetime semantics manage it from a wrapper script around the target process — using set/rm as primitives at start/exit points.

**Trade-off accepted**: callers of long-running processes shoulder the responsibility of wiring up wrapper-script hook management. This is consistent with the rest of Portal's hook design — callers already own the command string entirely, and wrapping a long-running process is standard operational practice.

**Confidence**: high. YAGNI-compliant; a mode field can be added later if a concrete use case emerges where wrapper-script management is genuinely impractical.

**False paths documented**:
1. *"One-shot vs persistent as two viable models"* (original research framing) — overstated the design space. `always` (current behavior) handles every real use case with caller-side wrapping.
2. *"`&&` chaining for self-removal"* — architecturally broken for long-running processes, the exact class of commands hooks serve.

---

## Save Content & Scope

### Context

Before any save-side architecture decisions (when to save, how to write, by what mechanism), we need to decide *what* gets saved. The initial discussion had been progressing toward plumbing (daemon vs subprocess, debouncing strategy) without first nailing down the content profile — a gap the user caught by asking whether scrollback was in scope.

The answer reframes the whole save-side discussion. "Structural resurrection" (sessions + windows + panes + layouts + cwds) is functional but hollow. Every pane comes back empty; history continuity is lost. Zellij's session persistence captures *pane contents by default* and is consistently cited as one of its best features. If Portal is going to own the full lifecycle, it has to at least match that standard — otherwise the feature name is aspirational and users who know Zellij will rightly feel shortchanged.

### Journey

Initial framing implicitly excluded scrollback. I was deep in architectural plumbing and never stopped to enumerate content. User corrected with an unambiguous product directive: *"Scrollback 100% MUST be captured. This is useless without it. I want Zellij but in tmux!! Whatever we can save we should."*

That directive became the organizing principle: **capture everything that persists as meaningful state, exclude only ephemeral interaction state, accept the uncapturable as out of scope.**

**Main screen vs alternate screen — a phantom problem resolved**: A follow-up tangent worried that `capture-pane -p -S -` returns "stale" content for panes running `vim`, `htop`, `less`, `man`, etc. — because those programs use tmux's *alternate screen buffer*, not the main screen. Initial (wrong) concern: "a pane in vim for 3 hours returns empty/stale scrollback because vim's content isn't captured."

Resolution: tmux distinguishes two separate buffers per pane:
- **Main screen buffer** — the normal terminal output that scrolls. This *is* scrollback. `capture-pane -p -S -` captures this.
- **Alternate screen buffer** — what alt-screen programs draw into. It temporarily replaces the visible area while the program runs, then disappears when the program exits and the main screen becomes visible again. It is *not* part of scrollback.

So the capture is correct: the main screen buffer *is* the real shell history, just temporarily hidden by the alt-screen overlay. A pane that's been in vim for 3 hours still has the actual scrollback (everything up to and including `vim main.go`) in its main screen buffer — and that's what gets captured and restored. There is no "stale content" — there is the scrollback as it exists.

**Items removed from inventory post-review:**
- **Marks** (`<prefix>m`) — initially listed as "position markers." In reality, tmux's `<prefix>m` sets a *pane-level* marked state (used by `swap-pane -m`, one pane at a time across the server) — not a scrollback position bookmark. The useful thing (copy-mode position marks) has no tmux API to capture or restore. Neither version justifies the complexity. Removed.
- **"Deviating session options"** — initially listed as "session names and deviating session options." On inspection, nearly all tmux options are set globally via `~/.tmux.conf` and apply on restore automatically. Per-session/per-window overrides (e.g., `synchronize-panes`, `monitor-activity`) are niche. Capturing them generically requires diffing `show-options` against global defaults — complexity not worth it. Also carried a recursion risk if Portal's own `set-hook -g` definitions were captured. Dropped generic options capture entirely. If a specific flag (like `synchronize-panes`) is missed, it can be added as an explicit per-window boolean later. YAGNI.
- **Last-pane tracking** — no confirmed tmux format variable exposes "which pane is 'last' for this window." To verify during implementation; dropped from the guaranteed inventory for now.

**Implication: no special handling for alt-screen panes.** Portal captures scrollback. Programs like vim are *not* scrollback. If a user wants vim auto-relaunched on restore, they register a hook — same as Claude, same as any other process. Portal doesn't guess, doesn't infer, doesn't try to capture alt-screen contents. The user's framing: *"If I was to start something that overtook the window, like a special command like vim, I wouldn't expect you to capture that because it's outside of the scrollback."* Correct.

### Options Considered

**A: Structural-only** (original implicit framing)
- Pros: Smallest save files, fastest, simplest security story.
- Cons: Panes come back empty. No history continuity. "Resurrection" in name only. Zellij users would scoff.

**B: Structural + scrollback, opt-in** (resurrect's model)
- Pros: Safety-conscious default, users opt in if they want it.
- Cons: Most users don't opt in and never experience the full benefit. The feature exists but doesn't feel right out of the box. Fails the "Zellij in tmux" product goal.

**C: Everything capturable, on by default, ephemeral excluded** (user's directive)
- Pros: Resurrection actually feels like resurrection. Matches Zellij UX standard. Simple mental model ("whatever was there, comes back").
- Cons: Larger save files, more data on disk, security consideration for sensitive output.

### Content Inventory

**IN SCOPE** (captured on save, restored on resurrection):

*Structural:*
- Session names
- Window indices, names, layout strings, active/zoom flags
- Pane indices, current working directories, active flag

*Content:*
- Full pane scrollback with ANSI escape sequences — colors, attributes, formatting preserved via `tmux capture-pane -e -p -S - -t <pane>`
- tmux per-session environment via `show-environment -t <session>` (the tmux-level env used for initializing new panes, not live shell env). Restored in full without filtering — tmux's own `update-environment` mechanism automatically refreshes stale values (`SSH_AUTH_SOCK`, `DISPLAY`, etc.) from the attaching client's env on session attach. No Portal-side filtering needed.

*Already stored:*
- Resume hooks (already in `hooks.json`, not new)

**OUT OF SCOPE — explicitly ephemeral:**
- Copy mode state
- Active selections
- Paste buffers
- Cursor position within panes
- Scroll position within scrollback
- Per-client state (which client has which pane focused, client-specific dimensions)

**OUT OF SCOPE — uncapturable by tmux** (research-confirmed, not Portal's problem to solve):
- Live shell environment variables — tmux can't observe shell-side `export`. Callers can compensate via resume hooks if they care.
- Running process state (REPL state, interactive sessions) — hence the resume hook system exists at all
- Open file descriptors, pipes, sockets, ptrace state, etc.

### Decision

**Capture everything tmux exposes that persists as meaningful state. On by default. No opt-in.**

- Scrollback capture is non-negotiable and always on
- History size: no artificial Portal cap — save whatever tmux has in the history buffer (respects user's `history-limit`). A cap can be added later if storage becomes a real issue. YAGNI.
- Storage: saved state lives in `~/.config/portal/` alongside existing config files, resolved via the same `configFilePath` mechanism. Considered `~/.local/state/portal/` (`XDG_STATE_HOME`) for separation from synced config, but all existing Portal config (`hooks.json`, `projects.json`, `aliases`) is machine-specific too — splitting would be inconsistent. One location, no migration. Can reorganize later if needed.
- Security: state files written with `0600` permissions. Scrollback contains command *output* (potentially more sensitive than shell history — `kubectl get secret`, `gh auth token`, debug logs with API keys). Same local-filesystem trust model as shell history and debug logs users already have on disk. No encryption at rest — overkill, adds key management complexity, matches neither resurrect nor Zellij.
- Per-session opt-out for sensitive sessions is handled separately under the Ephemeral Session Opt-Out subtopic — that gives users a safety valve without compromising the default experience.

### Capture feasibility (tmux APIs)

What tmux actually exposes for each item on the in-scope list:

**Verified against research / tmux docs:**

| Content | tmux mechanism |
|---|---|
| Session/window/pane structure | `list-panes -a -F` with format variables |
| Window layout strings | `#{window_layout}` (pre-zoom form, research-verified) |
| Pane working directory | `#{pane_current_path}` |
| Pane active / zoom state | `#{pane_active}`, `#{window_zoomed_flag}` |
| Pane current command (short name) | `#{pane_current_command}` (research-verified: short name only, no args — not a Portal problem) |
| Main-screen scrollback with ANSI | `capture-pane -e -p -S - -t <pane>` (research-verified) |
| tmux per-session environment | `show-environment -t <session>` (standard tmux) |
| Session/window/pane options | `show-options -s`, `show-options -w`, `show-options -p` |

**All items on the in-scope list are verified as capturable via standard tmux APIs.** Three soft-spot items (marks, deviating session options, last-pane tracking) were removed from the inventory during review — see Journey notes above.

### Impact on Save-Side Architecture (flagged, not decided here)

Saves are now content-heavy (scrollback per pane + structural), not lightweight JSON. Implications:

- Each save does N `capture-pane` calls + a JSON write — still fast (~ms per pane), but not negligible at burst frequency.
- Debouncing matters more — avoiding storms of large saves is valuable.
- Format probably wants per-pane scrollback files referenced from a main state JSON, rather than one giant state blob. Debuggable, selectively restorable, partial-corruption tolerant.
- These concerns feed into the upcoming Save-Side Architecture and Failure Modes subtopics — noted here, decided there.

**Confidence**: High. Product direction is unambiguous, tmux capture APIs are verified, architectural ripple effects are understood and manageable.

---

## Save-Side Architecture (partial — execution model + triggers + signals)

### Context

With Save Content & Scope decided (capture structural state + scrollback + per-session env on every save), the next question is *how* saves get triggered and *what* runs them. Scrollback capture fundamentally changes the save profile: content drifts continuously as users type and processes output, so structural-event triggers alone are insufficient. Periodic saves become necessary to catch scrollback changes between structural events.

The architectural question: where does the periodic save *run*? Portal is a one-shot CLI tool today — there's no long-lived process to hang a timer off.

### Options Considered

**A: Subprocess-per-event only** (original lean)
- tmux hooks fire `portal save-state` on structural events. No periodic save.
- Pros: matches Portal's CLI architecture, no new runtime model.
- Cons: misses scrollback drift between events. User sitting in one pane with Claude outputting for hours → no save → crash loses everything.

**B: Full daemon** (the Zellij path)
- Long-running `portal state-daemon` process managed by launchd/systemd/fallback-double-fork.
- Pros: clean separation, native timers, platform-native supervision where available.
- Cons: ~500 LOC of platform-specific lifecycle code (install, supervise, PID files, IPC, upgrade). Silent-failure mode on fallback platform (the exact problem Portal exists to avoid). The "decoupled from tmux" benefit is largely theoretical — the daemon has nothing useful to do when tmux is dead.

**C: Detached tmux session hosting a long-running Go process** (chosen)
- At bootstrap, Portal creates `tmux new-session -d -s _portal-saver "portal save-state --periodic"`. The Go process inside runs an internal 30s ticker loop.
- Pros: tmux owns the lifecycle, no platform-specific service management, crash recovery via next Portal invocation, minimal new code (~50 LOC of idempotent session creation).
- Cons: session visible in `tmux ls` (filterable from Portal's own picker), pattern is niche (tmux-slay is the only public precedent).

### Journey

Initial lean was A. The user asked a sharpening question — "if I'm sitting in this pane right now with Claude outputting, how does THIS conversation get saved?" — and exposed that A misses the dominant real-world case. Structural events don't fire when content is just accumulating. Periodic saves are necessary, not optional.

That opened B as the "real" answer. Zellij solves the same problem elegantly via a tokio task inside its always-running server thread. But Zellij's architecture is client-server from day one; the daemon is *intrinsic to the tool*. Portal bolting on a daemon for one feature is a different proposition — the engineering investment is large (per-platform service management, silent-failure mode on fallback, upgrade complexity) and the daemon's value evaporates when tmux is dead.

The user's framing crystallized option C: *"It's like doing it in the documentation, saying if you want sessions to save, you need to open up a new terminal and run Portal process execute. Of course, that's a pain, but that's really what's happening here, isn't it? Except Portal is opening it itself and binds it to the same tmux."*

That reframe is honest: there IS a long-running save process; we're not pretending otherwise. We're delegating its supervision to tmux, which already owns the process lifecycle for every pane a user runs. No new infrastructure — existing tmux mechanisms, used for their normal purpose.

**Research-verified concerns and answers** (see `research/detached-session-host-verification.md`):

1. *Session lifecycle when the Go process exits*: session auto-destroys (default tmux behavior). Portal's next bootstrap sees `has-session -t _portal-saver` return false and recreates. Clean crash recovery, no `remain-on-exit` tuning needed.

2. *Signal propagation on `tmux kill-server` or server shutdown*: tmux closes the PTY master fd; the kernel delivers **SIGHUP** (not SIGTERM) to the hosted Go process. This is a subtle but important implementation detail — Portal's save loop must trap SIGHUP explicitly. Direct `kill <pid>` from outside tmux sends SIGTERM, so trap both. Handler flushes the current save atomically via `AtomicWrite` and exits. No configurable grace period.

3. *Visibility in `tmux ls`*: yes, `_portal-saver` shows up; no tmux mechanism to hide it. Portal filters it from its own picker via name-prefix check in `ListSessions`. Minor cosmetic cost.

4. *tmux 3.5/3.6 periodic primitives*: confirmed none exist. No interval hooks, no `set-hook` enhancements. The detached-session pattern is the only viable in-tmux approach.

5. *`destroy-unattached` defensive case*: a user with `set-option -g destroy-unattached on` in their `.tmux.conf` could have their global setting kill `_portal-saver` immediately on creation (since it's `-d` and has zero attached clients). Portal explicitly sets `destroy-unattached off` on the saver session after creation as a safety measure.

### Decision

**Execution model**: Option C. Portal creates a detached tmux session named `_portal-saver` during bootstrap, hosting a long-running Go process (`portal save-state --periodic`) that runs an internal 30-second ticker loop.

**Trigger mechanism** (three layers):
- *Event-driven* (immediate): `set-hook -g` on structural events (`session-created`, `session-closed`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`) triggers a save via a thin `run-shell` handoff. Catches structural changes as they happen.
- *Periodic* (every 30s): the hosted Go process's internal ticker. Catches scrollback content drift and cwd changes that structural events miss.
- *Opportunistic*: every `portal open` / `portal attach` checks last-save age and fires a save if stale. Covers "active user" cases where no structural events have fired recently.

**Crash safety cadence**: 30 seconds. Bounds worst-case data loss to ~30s of scrollback on unexpected tmux/system termination. Configurable later if needed. Matches Zellij's default (`DEFAULT_SERIALIZATION_INTERVAL = 60000ms`, was 1s pre-v0.39.2, raised due to disk-write complaints per [Zellij PR #2951](https://github.com/zellij-org/zellij/pull/2951)) — 30s is a reasonable compromise between data loss and disk write volume.

**Signal handling**: the Go process traps SIGHUP (from PTY close on tmux shutdown — the dominant path) and SIGTERM (direct kill). Handler flushes the current save via existing `AtomicWrite` (no corruption risk), exits. No mid-write corruption concerns because atomic rename guarantees either the old or new state file is always valid.

**Idempotency & bootstrap flow**: `EnsureServer()` in `PersistentPreRunE` calls `has-session -t _portal-saver`. If present, no-op. If missing, create via `new-session -d -s _portal-saver "<portal-binary> save-state --periodic"`, then `set-option -t _portal-saver destroy-unattached off` as defensive measure. Portal's own session picker filters names starting with `_` to hide it.

**Confidence**: High. All five research questions have source-level answers. The pattern has precedent (tmux-slay) and the concerns are concrete and addressed.

### Impact on remaining Save-Side subtopics

Several sub-decisions stay open:
- **Save format and schema**: scrollback per pane plus structural JSON. Exact layout (one file vs. per-session dir, pane file naming, index format) still to decide.
- **save-state CLI surface**: the `--periodic` flag is one entry point. What's the full CLI surface (`portal save-state` manual? `portal save-state status`? exit codes?) needs fleshing out.
- **tmux hook registration lifecycle**: when Portal uninstalls or upgrades, what happens to registered `set-hook -g` entries and to `_portal-saver`? Needs explicit lifecycle management.

These continue in the next round of discussion.

---

## Save-Side Debouncing / Serialization

### Context

Three trigger layers (event-driven, 30s periodic, opportunistic) can collide — a user creating a new window fires `session-created` + `window-linked` + `window-layout-changed` within ~100ms, plus the ticker could fire during any of them. Without coordination, 3+ saves could race for the state file. `AtomicWrite` prevents corruption but doesn't prevent duplicate work or inconsistent reads mid-save.

### Options Considered

**A: Everyone writes, coordinate via filesystem** — each trigger path writes directly; cooldown files or file locks prevent storms.
- Pros: each path is independent.
- Cons: concurrency-by-default; every trigger path has to implement cooldown correctly; hard to debug races.

**B: Single writer through the hosted process** — events and other triggers only *signal* "state is dirty"; the hosted process owns all writes.
- Pros: single writer by construction — no write races possible; debouncing becomes in-memory and trivial; clean ownership.
- Cons: requires a notification mechanism between trigger subprocesses and the hosted process.

### Decision

**Option B, with a file-based dirty flag** as the notification mechanism.

**How it works:**

1. tmux fires a structural event → `set-hook -g ... 'run-shell "portal save-state --notify"'`
2. `portal save-state --notify` is a ~20-line Go program: open/touch `~/.config/portal/save.requested` (the dirty flag file), exit.
3. The hosted Go process (running inside `_portal-saver`) has a 1-second ticker. Each tick checks: *is the dirty flag set, OR has it been ≥30s since the last save?* If either, capture state and clear the flag. Otherwise, wait.

**Key properties:**
- **Single writer**: only the hosted process writes state files. No filesystem coordination needed beyond the dirty flag.
- **Natural coalescing**: 5 events firing in 100ms all set the flag; the next tick does exactly one save.
- **Max-gap guarantee**: 30 seconds is the ceiling on save staleness, even during idle periods with no events.
- **Event latency**: ≤1 second from tmux event to save completion (one tick).
- **Crash coverage**: worst-case data loss is 30 seconds of scrollback on sudden tmux/system termination.

**Opportunistic trigger dropped.** Earlier framing had `portal open`/`portal attach` also firing saves. Redundant under B: if the hosted process is running, it's already saving via events + ticker. If it's not running, `portal open`'s `EnsureServer()` recreates `_portal-saver` and its first tick fires within ~1 second. Dropping opportunistic removes a code path that would race with the hosted process for no coverage benefit.

### Hosted-process loop (pseudocode)

```go
for {
    select {
    case <-ticker.C:  // 1 second
        if isDirty() || timeSinceLastSave() >= 30*time.Second {
            captureAndWrite()
            clearDirty()
        }
    case <-ctx.Done():  // SIGHUP or SIGTERM
        captureAndWrite()  // flush once on shutdown
        return
    }
}
```

### False path documented

*"Each trigger path implements its own cooldown"* — Option A. Rejected because concurrency correctness becomes distributed across every handler that might save, and every new trigger path has to re-implement the coordination primitive. Option B localizes all concurrency into one writer and makes debouncing a one-line check.

---

## Save Format & Schema

### Context

The save payload has two very different shapes: **structural state** (session/window/pane tree, cwds, env, layouts — small JSON) and **scrollback content** (binary, potentially megabytes per pane). One file vs many files is the core design fork.

### Decision

**Many files.** Per-pane scrollback files plus a single structural index JSON that references them.

**Layout:**

```
~/.config/portal/state/
├── sessions.json              # structural index — the "commit"
└── scrollback/
    ├── <session>__<window>.<pane>.bin   # raw capture-pane -e output per pane
    ├── work__0.0.bin
    ├── work__0.1.bin
    └── ...
```

- Scrollback files are raw `capture-pane -e -p -S -` output (ANSI escapes inline). Filesystem-safe pane key: `<session>__<window>.<pane>.bin`, with a simple sanitizer for special characters in session names and a hash-suffix fallback for collisions.
- `sessions.json` is the structural index: sessions → windows → panes, with cwd, active/zoom flags, layout strings, per-session environment, and `scrollback_file` paths (relative to state dir).

**Schema sketch:**

```json
{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": [
    {
      "name": "work",
      "environment": { "LANG": "en_US.UTF-8", "TERM": "xterm-256color" },
      "windows": [
        {
          "index": 0,
          "name": "main",
          "layout": "b25f,200x50,0,0{...tmux layout string...}",
          "zoomed": false,
          "active": true,
          "panes": [
            {
              "index": 0,
              "cwd": "/Users/leeovery/Code/portal",
              "active": true,
              "current_command": "zsh",
              "scrollback_file": "scrollback/work__0.0.bin"
            }
          ]
        }
      ]
    }
  ]
}
```

- `version` for future schema evolution (loader reads the field and handles known versions).
- `saved_at` for observability — `portal state status` can render "last saved 12s ago."
- No `options` field (dropped). No `marks` field (dropped).

### Cross-file atomicity via commit discipline

`AtomicWrite` gives per-file atomicity, but the state is many files. The discipline:

1. Capture all state in memory (list-panes, show-environment, capture-pane per pane).
2. Write each pane's scrollback to its file via `AtomicWrite` (temp + rename).
3. Write `sessions.json` last via `AtomicWrite` — **this is the atomic commit.**

Failure modes:
- Crash before step 3 → old `sessions.json` still points to old scrollback files, which still exist → restore works as before.
- Crash mid-step 3 → `AtomicWrite` guarantees either the old or new JSON, never partial.
- Orphaned new scrollback files → GC handles them (below).

### GC / purge logic

After every successful save, after `sessions.json` is atomically committed:

1. Read the new `sessions.json` and collect every `scrollback_file` path it references.
2. List everything in `scrollback/`.
3. Any file on disk but NOT referenced by the new index is orphaned → delete.

Handles every way files can become stale:
- Pane closed → file not in new index → deleted.
- Session renamed (`work` → `project`) → old-named files deleted, new-named ones written.
- Window renumbered → same.
- Previous save crashed mid-way leaving orphans → next successful save's GC cleans them up. Self-healing.

Idempotent. Runs synchronously, once per save.

### Content-hash dedup (skip unchanged scrollback)

The naive "rewrite every scrollback file every 30s" plan would generate ~86GB/day of writes in a heavy-scrollback scenario (power user with `history-limit 50000` and 10 panes). Most of those writes are unchanged content — wasteful, SSD-wearing.

**The hosted process holds an in-memory map** `paneKey → hash of last-written scrollback`. On each save cycle, per pane:

1. Capture scrollback (cheap — in-memory inside tmux).
2. Hash it (xxhash or similar, few ms per MB).
3. Compare to stored hash.
4. If identical → skip the disk write, no change.
5. If different → `AtomicWrite` the scrollback file, update the stored hash.

`sessions.json` is written only if anything actually changed (structural delta or at least one pane's hash differed). If literally nothing changed for a full 30s cycle, zero disk activity.

This turns worst-case 86GB/day into single-digit MB/day for realistic workloads. Only actively-changing panes incur write cost.

### Tick cadence (recap — why 1s)

The hosted process's 1s ticker is purely a **dirty-flag poll**, not a save cadence. Idle cost per tick: stat the dirty flag file + compare `time.Since(lastSave)` against the 30s threshold. Microseconds. Heavy work (capture/hash/write) only fires on dirty-flag set OR 30s max-gap elapsed. Responsiveness: event → save within 1 second.

Not load-bearing — could swap to fsnotify later for sub-10ms responsiveness at the cost of cross-platform filesystem-watcher complexity. Current polling approach is simpler and good enough.

### Retention policy

**Current state only.** Single `sessions.json`, no historical snapshots.

- `AtomicWrite` makes mid-write corruption vanishingly rare — temp + rename means the previous version is always fully intact until the new one is fully written.
- Historical snapshots would 5-10× disk use for zero restore benefit.
- If corruption becomes an issue in practice (e.g., disk-full mid-write), can add a `sessions.json.previous` backup later. YAGNI for now.

### Deferred (not decided here)

- **Compression** of scrollback files. ANSI text is highly compressible (5-10×) but adds CPU cost and makes debugging harder. Skipping for now; revisit if disk use becomes a complaint.
- **Parallel capture** for users with many panes. For now, sequential capture is fine — round-trip cost per pane is ~10ms, and realistic pane counts stay under ~20. Optimize if a complaint surfaces.
- **Schema migration** (version N → N+1). Standard practice: loader reads `version`, applies transforms or graceful fallbacks. Not a design decision now.

---

## CLI Surface

### Context

The save-side design references several Portal invocations: the long-running hosted process, the thin notifier called by tmux hooks, and potentially user-facing commands for manual save and status. What's actually exposed to users vs. hidden as implementation detail?

### Decision

**One user-facing command, two hidden internal subcommands, all under a `portal state` namespace.**

**User-facing:**
- `portal state status` — liveness check for `_portal-saver`, last-save timestamp ("saved 12s ago"), session/pane count, disk use under `~/.config/portal/state/`. Purpose: let users verify resurrection is working, diagnose when it isn't, see what's captured.

**Internal (hidden from `portal --help`):**
- `portal state daemon` — the long-running process invoked as the command of the `_portal-saver` session. Holds the in-memory hash map, runs the 1s ticker, handles signals.
- `portal state notify` — ~20-line binary invoked from `set-hook -g ... 'run-shell "portal state notify"'`. Touches `~/.config/portal/save.requested` and exits.

### Journey

Originally considered `portal state save` (manual synchronous save) as a user-facing command. On examination, every ostensible use case is already covered:

- *"Save before reboot"* — SIGHUP flush on tmux server shutdown + 30s max-gap covers it.
- *"Scripting/automation"* — speculative; no concrete workflow identified.
- *"Pre-risky-action save"* — same as reboot case.
- *"Psychological reassurance"* — not a technical need.
- *"Debugging the save mechanism"* — developer concern, not a user surface; can touch the dirty flag manually.

None of these justify a user-facing command. Dropped. If a real automation use case surfaces later, it can be added — YAGNI until then.

Namespace (`portal state`) retained even though only one command lives under it initially. Gives natural room for future user-facing commands (`portal state gc`, `portal state reset`, etc.) if they ever become necessary — though none planned now.

### Confidence

High. The internal subcommands are dictated by the architecture (the hosted process needs an entry point, the notifier needs an entry point). The user-facing surface is minimal and driven by actual use cases rather than speculation.

---

## tmux Hook Registration Lifecycle

### Context

Portal's resurrection plumbing — the `set-hook -g` entries for structural events, plus the `_portal-saver` session — is tmux server-level state. That state has to be reasoned about across a matrix of transitions: first install, subsequent invocations, upgrades, uninstalls, manual server restarts, collisions with user-installed plugins. This subtopic walks each scenario and captures the resulting bootstrap/teardown rules.

The subtopic covers seven scenarios. Each is a sub-decision. Going one at a time to keep the mental model clean.

### Scenario 1: Fresh-server bootstrap — DECIDED

The simplest case. User runs `portal open` on a machine where `tmux` isn't running. `EnsureServer()` starts the server and Portal needs to set up its plumbing.

**Decided approach** (amended after scenario 7 decision — see below):

On every `EnsureServer()` call (not just fresh-server cases):

1. **Conditionally register hooks via `set-hook -ga` with read-before-write** (not naive "always re-register"). For each target event, `show-hooks -g` is parsed to check whether any array entry already contains `portal state notify` — if present, skip; if absent, append via `-ga`. Cost per bootstrap: ~1 tmux round-trip to read, plus 0–7 round-trips for writes (zero on a bootstrapped server, seven on a fresh one). Self-healing preserved — if our entries get stripped, next bootstrap re-adds them.
2. **Conditionally create `_portal-saver`** via `has-session -t _portal-saver` check. If present, skip. If absent, `new-session -d -s _portal-saver "portal state daemon"`.
3. **Defensively set `destroy-unattached off`** on the saver session — always (even if the session already exists), via `set-option` which is idempotent. Guards against users with `destroy-unattached on` globally in `.tmux.conf`.
4. **Filter `_*` session names** from the TUI picker and from `sessions.json` capture — Portal's internal sessions don't pollute user-visible state or get "restored" on reboot.

**Note on the amendment**: earlier framing of this scenario proposed "always re-register via `set-hook -g`" on the (correct) premise that plain `set-hook -g` is idempotent. That framing used the non-`-a` (replace) variant, which would overwrite any user or other-plugin hook on the same event. Scenario 7 decided to use `-a` (append) instead for coexistence — but `-a` is *not* idempotent (identical appends accumulate duplicates). The read-before-write check replaces the "set is cheap to repeat" assumption with a content-based idempotency check, preserving self-healing without accumulation.

### Journey (scenario 1)

Initial proposal gated all setup behind the `has-session -t _portal-saver` check — skip everything if the saver was already running. User correctly flagged a failure mode: if hooks got stripped for any reason (accidental `set-hook -gu`, another plugin overwriting, obscure tmux edge case) while `_portal-saver` was still alive, a saver-only idempotency check would leave hooks broken.

Fix: separate the idempotency concerns. Hook registration is cheap and idempotent via `set-hook -g` semantics, so just always re-register. Only the session creation (which can't gracefully no-op on existing sessions) needs a guard.

The self-healing property is a small UX win: if any failure mode strips hooks, the next `portal open` restores them automatically without the user needing to know anything went wrong.

### Note carried to later scenarios

"Always re-register" also means Portal always overwrites whatever value was previously in those hook slots. If another plugin had set `set-hook -g session-created ...` to do its own thing, Portal stomps on it. That's a real concern for scenario 7 (collision with other plugins) — deferred there, not re-litigated here.

### Ordering note

Register hooks *before* creating `_portal-saver`. Creating `_portal-saver` fires a `session-created` event, which triggers our hook, which touches the dirty flag, which the hosted process picks up on its first tick — so within ~1 second of bootstrap, Portal has captured initial state. Ordering the other way would miss that initial save trigger (not critical, the 30s max-gap covers it anyway, but aesthetically cleaner the chosen way).

### Scenario 2: Subsequent invocation on bootstrapped server — DECIDED

Common path: server's been running for days, hooks registered, `_portal-saver` ticking. User runs `portal open` again.

**Decided approach:** the scenario 1 rules already cover this with one consistency tweak — **always** set `destroy-unattached off` on `_portal-saver`, not just on creation. Same self-healing principle as the hook re-registration: `set-option` is idempotent when the current value matches, so the cost is ~1ms per bootstrap, and we gain protection against the (unlikely but possible) case where something flipped that option after creation.

Net result: every `EnsureServer()` call performs a uniform ~8ms block of idempotent tmux calls to ensure plumbing consistency, regardless of whether the server is fresh or long-running. Single code path, no "first invocation vs subsequent" branching.

Explicitly *not* separately branched in code — scenario 1 and scenario 2 are the same code. The distinction only exists in this document to confirm both cases are covered by the chosen rules.

### Scenario 3: Portal upgrade with running server — DECIDED

User runs `brew upgrade portal` (or equivalent). New binary is on disk. tmux server still running, `_portal-saver` still hosting a daemon that was `exec`'d from the *old* binary. Hooks reference `portal state notify`, which resolves through `$PATH` at each invocation → they pick up the new binary automatically.

**The only stale thing is the running daemon.** It holds the old binary in memory until it exits. If the upgrade changed the daemon's save logic, schema, dirty-flag protocol, or signal handling, users won't see fixes until it restarts.

**Decided approach: version-marker-based restart detection.**

1. On startup, `portal state daemon` writes its version (e.g., `v0.4.2`) to `~/.config/portal/state/daemon.version`.
2. On every `EnsureServer()` call, Portal reads `daemon.version` and compares to `cmd.version` (the currently-invoking binary's version).
3. If they differ (upgrade occurred), Portal runs `kill-session -t _portal-saver` then recreates it with the new binary. The replacement daemon overwrites the version file on startup.
4. If the version file is absent (first-ever bootstrap), treat as mismatch and (re)create. No special case.

Cost: one file read per `portal open` (microseconds). One `kill-session` + `new-session` on an actual upgrade (~50ms). Worst-case user visibility: a brief pause on the first `portal open` after upgrade. Invisible to anyone who's not watching.

Data safety: the old daemon gets SIGHUP via PTY close when its session is killed; its signal handler flushes the final save atomically (via `AtomicWrite`) before exit. The new daemon takes over cleanly. Worst-case data loss: whatever was uncaptured in the ~1s since the last dirty-flag check.

**No backward compatibility layer needed** — since nothing's implemented, every version that ships will include the version-marker behavior from day one. The "version file missing" branch is defensive, not transitional.

### Scenario 4: Portal uninstall with running server — DECIDED

User runs `brew uninstall portal`. Binary gone. tmux server still running. Two failure surfaces:

1. **Hook-fire error spam**: every structural event tries to `run-shell "portal state notify"` → `portal: command not found` in tmux's error buffer. Noisy.
2. **`_portal-saver` eventually dies**: the in-memory binary survives until the daemon exits or the server restarts. Once gone, can't be recreated. But this is silent — Portal's uninstalled, there's nothing to do.

The real problem is #1.

**Decided approach: defensive hook shell + optional cleanup command.**

**Defensive hooks.** Every `set-hook -g` registration wraps the invocation in a binary existence check:

```
set-hook -g session-created 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'
```

If `portal` is absent, the hook fires but the `command -v` short-circuit prevents the invocation. No error, no spam. Runs `command -v` (a shell built-in, ~microseconds) on every structural event — imperceptible overhead for a big UX win: the uninstall "just works" regardless of how the user uninstalled (Homebrew, direct delete, package swap, whatever).

**Optional `portal state cleanup` command.** For users who want explicit teardown before uninstalling — kills `_portal-saver`, unsets the global hooks, optionally removes the state directory. Documented in the README's uninstall section. *Not* relied upon for correctness — the defensive hooks handle the forgot-to-run-cleanup case.

**User data left on disk.** `~/.config/portal/state/` and the existing config files (`hooks.json`, `projects.json`, `aliases`) stay put after uninstall. Standard Unix convention — uninstalling the tool doesn't destroy user data. Reinstalling picks up where the user left off.

### Scenario 5: Portal binary replaced (brew upgrade) — DECIDED (composed from 3+4)

No new rules. The transient window during `brew upgrade` is fully covered by the combination of scenarios 3 and 4:

- **During the atomic swap** (a brief window while the binary at the install path is being replaced): if a tmux hook fires and momentarily can't resolve `portal`, scenario 4's defensive `command -v` wrapper short-circuits the invocation. No error.
- **After the swap**: scenario 3's version-marker detection picks up the version change on the next `portal open` and recreates `_portal-saver` with the new binary.

Install-path migration (e.g., Intel → Apple Silicon Homebrew) is also covered — hooks reference `portal` on `$PATH`, the running daemon doesn't care about path changes (it's in memory), and the version marker triggers a restart on the next bootstrap.

Noted here for completeness; no separate code path required.

### Scenario 6: User restarts tmux server — DECIDED

Server dies via `kill-server`, `killall tmux`, reboot, crash, etc. Next `portal open` starts a fresh server.

**Server-level state** (hooks, `_portal-saver`, user sessions) — all gone. **On-disk state** (sessions.json, scrollback files, daemon.version, save.requested) — preserved.

**No new rules.** The existing bootstrap flow from scenarios 1/2 handles server restart end-to-end:

- Hooks get registered fresh on the new server (scenarios 1/2).
- `_portal-saver` is absent, gets recreated (scenario 1's `has-session` guard).
- Version check runs, matches-or-mismatches harmlessly since the daemon is newly spawned either way.
- Restoration of user sessions from `sessions.json` happens — but that's Restore-Side Architecture's problem, not hook lifecycle's.

**One defensive behavior to add in implementation:** the daemon should clear `save.requested` on startup, in case the file is left over from the previous run that didn't get to process it. Cheap belt-and-braces — prevents a stale dirty flag from immediately triggering a save of a mid-restore state. (Though even without this, the save would just capture whatever tmux looks like at that moment, which eventually converges correctly — the cleanup is about avoiding a redundant save during the restore window, not correctness.)

Cross-reference: restoration itself (recreating user sessions + replaying scrollback) is orthogonal to hook lifecycle and lives under Restore-Side Architecture.

### Scenario 7: Hook collision with other plugins — DECIDED

The naive plan (scenario 1's original framing) was to overwrite hooks via `set-hook -g`, which meant stomping on any user `.tmux.conf` hooks or other-plugin hooks on the same events. Research (see `research/detached-session-host-verification.md` continuation) confirmed:

- tmux 3.0+ supports `set-hook -a` (append) — hooks become array-indexed options with per-index entries.
- `-a` does NOT deduplicate — identical appends accumulate duplicates. "Always re-register" without a check would grow the array indefinitely.
- Major TPM plugins (continuum, resurrect, sessionist, logging, yank) don't use `set-hook` at all. Real collision risk is with users' own `.tmux.conf` hooks, not other plugins.
- Per-index removal works: `set-hook -gu 'EVENT[N]'` removes a single entry, leaves others untouched. Sparse arrays fire correctly.

**Decided approach: append + content-based idempotency + index-based removal.**

**Registration shape** (combined with scenario 4's defensive wrapper):

```
set-hook -ga session-created 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'
```

The substring `portal state notify` serves as Portal's natural identity token — it's unique enough that no other tool will ever emit that exact sequence. No separate marker comment needed.

**Idempotency check at bootstrap:**

1. Run `tmux show-hooks -g` and capture stdout.
2. For each target event, parse lines matching `^<event>\[(\d+)\] .*portal state notify` — extract indices where our entry is present.
3. If the index set is non-empty → Portal's hook is registered → skip.
4. If empty → `set-hook -ga <event> '<command>'` to append.

**Removal (uninstall / `portal state cleanup`):**

1. Run `tmux show-hooks -g`.
2. Parse for all our indices (per the regex above, for each event).
3. Remove each via `set-hook -gu 'EVENT[N]'`, in reverse index order (defensive — research showed tmux doesn't renumber after removal, but reverse-order is cheap insurance).

**Parsing lives in Go**, using the existing `Commander` interface. Compiled regex per event, table-driven tests with canned `show-hooks` output strings — no shell pipelines, no external-utility dependency.

**Quoting caveat** (from research): tmux may render the stored command with different outer quoting than we set. Doesn't affect us — we match on the `portal state notify` substring which is raw text inside the command, untouched by tmux's outer quoting.

**Preserved properties:**
- **Coexistence** — user's own hooks and any other plugin's hooks on the same events are left intact. We add; we don't replace.
- **Self-healing** — if our entries get stripped (user ran `set-hook -gu`, another tool misbehaved), next bootstrap's idempotency check finds none and re-appends.
- **Clean uninstall** — targeted removal of only our entries, per-index, without disturbing others.

### False paths documented

1. *"Use plain `set-hook -g` (replace) and rely on idempotency"* — original scenario 1 framing. Rejected because it stomps on user and other-plugin hooks on the same events. `-a` (append) is the correct primitive for coexistence.
2. *"`set-hook -a` is idempotent if the command matches"* — tested empirically by research and disproven. `-a` always appends; identical appends accumulate duplicates.
3. *"Embed a `# portal-resurrect` marker comment for identification"* — unnecessary. The `portal state notify` command name is already a unique-enough identifier; adding a marker is noise.

### Minimum tmux version implication

This scenario requires tmux 3.0+ (Feb 2020). Array-indexed hooks (the foundation for `-a` semantics) were added then. Earlier tmux versions don't support this model at all — they'd need the replace-based fallback, which we've rejected for coexistence reasons. **Min-tmux-version decision belongs in Scope Boundaries**, but noting here that it's now constrained to ≥3.0.

### False paths documented

1. *"Event-driven only is sufficient"* — true for structural state, false once scrollback is in scope. Content drift between events is the dominant case.
2. *"`run-shell -b 'while true; do ...; done'` as a poor-man's daemon"* — research found no TPM plugin uses this pattern after ~10 years. Known tmux bugs around `-b` flag ([tmux#1843](https://github.com/tmux/tmux/issues/1843), [#2306](https://github.com/tmux/tmux/issues/2306)). Detached-session hosting is more battle-tested.
3. *"Full daemon like Zellij"* — Zellij has one because it IS a multiplexer; the daemon is intrinsic. Portal bolting on a daemon for one feature is a different calculus, and the "decoupled from tmux" benefit largely evaporates given that the daemon has nothing useful to do when tmux is dead.

---

## Restore-Side Architecture (partial — trigger + skeleton/content split + coordination)

### Context

Restoration is the reverse of save: read `sessions.json` + per-pane scrollback files from disk, reconstruct tmux sessions, inject scrollback so the user returns to their work as it was. Three foundational questions need resolving before mechanical details (injection format, layout replay, shell readiness):

1. **When** does restoration trigger?
2. **What** is restored eagerly vs lazily?
3. **How** does the save process avoid destroying saved state during the restoration window?

### Decision 1: Restoration trigger — restore all, idempotent by name

Run restoration on every `portal open` invocation. No `serverStarted` gate, no explicit user command. For each entry in `sessions.json`:

- If a live session with that name already exists → **skip** (user's current reality is authoritative).
- If not → **restore the skeleton** (see decision 2).

**Why:** no concrete threat model justified defensive gating. Name-collision handling addresses the one real risk — users with manually-created tmux sessions sharing names with saved Portal sessions. Portal's `{project}-{nanoid}` naming makes collisions between Portal-created sessions practically impossible.

**Cost in steady state** (all saved sessions already live): ~20ms — one JSON read + one `list-sessions` call + diff → no-op. Invisible.

### Decision 2: Skeleton-eager + scrollback-lazy (via attach hook)

Rebuild each missing session's **structure** immediately (eager), defer **scrollback injection** until the user attaches to that session (lazy).

**Skeleton-eager:** `new-session -d`, `new-window`, `split-window`, `select-layout`, set cwds, active/zoom flags. Cost ~600ms for a heavy 10-session config. Covered by the loading page already shown during bootstrap.

**Scrollback-lazy:** scrollback bytes are NOT injected during bootstrap. Files on disk left intact. Injection happens on *attach* via a tmux `client-session-changed` hook running a Portal binary.

**Why skeleton-eager** (vs sessions-only-eager): preserves tmux self-containment. After bootstrap, native tmux commands, third-party plugins, shell aliases, and direct `tmux attach` all see the real structure. Sessions-only would leave non-Portal paths seeing broken empty sessions. The ~500ms extra cost buys Portal being additive rather than invasive.

**Why scrollback-lazy** (vs fully-eager): at realistic power-user scrollback sizes (`history-limit 50000`), `paste-buffer` injection costs 300-600ms per pane. 30 panes eagerly ≈ 15s of boot delay — unacceptable. Lazy amortizes across attaches; never-touched sessions cost nothing.

**Why hook-driven hydration** (vs Portal-attach-code-driven): `client-session-changed` fires on *any* attach path — `portal open` picker, direct `tmux attach -t NAME`, `switch-client`, anything. Universal coverage, single code path.

**Why synchronous injection** (vs async-progressive): injection completes before the user sees the pane. No "empty pane gradually filling in" UX. User clicks attach → brief pause (~1.5s heavy-case) → session appears ready. Analogous to opening a large file in an editor. If `run-shell` blocking proves annoying, can optimize with `-b` + coordination later.

**Rejected alternative — background prefetch.** Initially proposed post-bootstrap background hydration of every session. User raised race conditions (user attaches a session mid-fill). Pure-lazy + marker coordination is simpler and race-free.

### Decision 3: Marker coordination — awaiting-hydration + restoring-in-progress

Two volatile tmux server-option markers coordinate restoration and saving.

**`@portal-skeleton-<key>`** (awaiting hydration):
- Set by skeleton-restore on each pane it creates (keyed by structural position `session:window.pane`).
- Semantic: "this pane was skeleton-restored; its saved scrollback file on disk holds pre-boot state that must not be overwritten until injected."
- Save process **skips** panes with this marker — doesn't capture, doesn't update sessions.json entry. Files preserved.
- Hydration (on attach) **unsets** the marker after successful injection; normal save resumes.
- **User-created panes never get this marker** — brand-new post-boot sessions/panes capture normally from the start.
- **Inverse semantic matters**: "needs hydration" is active state set by restore; default absence means "safe to capture." Keeps new-session case working without a separate code path.
- **Volatile** (server option): cleared on server restart. On next boot, skeleton restore sets them again. No stale state across server lifetimes.

**`@portal-restoring`** (restoration in progress):
- Set at start of skeleton restore, unset after completion.
- Skeleton restore fires `session-created` / `window-linked` / `window-layout-changed` cascades. Hooks fire `portal state notify` → dirty flag → saves would capture half-built state.
- `portal state notify` and the hosted daemon's tick both check `@portal-restoring` and no-op while set.
- Volatile. Portal crash mid-restore → option gone with the server → next bootstrap starts fresh.

### Decision 4: Failure-mode behavior for hydration

Injection failure (file missing, disk read error, etc.):
- **Unset the marker anyway.** Pane stays empty; normal save resumes. Degraded, not stuck.
- **Log a warning.** Failure observable but not spam.
- **Do not retry.** Missing file is likely permanent; retrying on every attach would produce repeat errors.

Keeps one pane's broken state from poisoning forever. User gets an empty shell instead of history — disappointing but workable.

### Full bootstrap flow

`EnsureServer()` on every `portal open`:

1. Start tmux server if not running.
2. Register hooks idempotently via `set-hook -ga` + content-based check:
   - `session-created`, `session-closed`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out` → `portal state notify`
   - `client-session-changed` → `portal state hydrate` (new)
3. Create `_portal-saver` if missing; always set `destroy-unattached off` on it.
4. **Set `@portal-restoring 1`.**
5. Read `sessions.json`. For each saved session:
   - Already live → skip.
   - Else → skeleton restore. Create session, windows, panes, apply layouts, set cwds. For each created pane, set `@portal-skeleton-<key> 1`.
6. **Unset `@portal-restoring`.**
7. TUI proceeds. Hosted daemon ticks, captures state (skipping skeleton-marked panes). First attach to each skeleton-restored session triggers hydration.

### Journey highlights

- **Trigger question**: proposed gating on `serverStarted AND no sessions exist`. User asked what I was defending against. Honest audit found no concrete threat. Simplified.
- **Pane-level laziness**: scrollback sizes at `history-limit 50000` make fully-eager restoration take ~15s. Concrete math shifted the call toward lazy.
- **Sessions-only laziness (rejected)**: would break direct `tmux attach`. Self-containment won.
- **Background prefetch (rejected)**: race conditions. Pure-lazy via hook is cleaner.
- **Marker inverse semantic**: first framed as "has been hydrated." User's question about new sessions exposed the bug — new panes shouldn't need a marker. Flipped to "awaiting hydration."
- **Restore-in-progress guard**: surfaced late. Skeleton restore fires a cascade of hooks; without a guard, saves would capture partial state.

### Confirmed properties (answers to user questions)

- **Dormant session files persist indefinitely**: `@portal-skeleton-` marker prevents save-overwrite. Days/weeks of ignored sessions → files on disk intact.
- **New panes/sessions captured immediately**: no marker = normal save path.
- **Scrollback truncation at head**: tmux's history buffer is a ring; `capture-pane` returns current buffer. File size bounded by `history-limit × avg-line-bytes`. Natural.
- **Direct `tmux attach` path**: fires `client-session-changed` hook, same hydration runs. Universal.

### What's still pending under Restore-Side

- **Shell readiness detection**: for panes where hydration isn't needed (no scrollback file), whether pre-injection polling is needed before firing resume hooks.
- **Layout restoration approach**: mostly mechanical, but research flagged edge cases.
- **Fate of `WaitForSessions` / `bootstrapWait`**: existing polling becomes unnecessary; explicit refactor decision needed.
- **Bootstrap integration**: tie into existing `PersistentPreRunE` / `EnsureServer` code paths.

---

## Scrollback Restore Mechanics

### Context

With the attach-time injection plan committed, the actual mechanism for getting scrollback bytes into a pane needed resolving. Candidates on the table: `tmux load-buffer` + `paste-buffer`, `send-keys -l`, `pipe-pane -I`, shell `cat` via `send-keys`, direct `/dev/pts` write.

### The research finding that reframed the decision

**All tmux-native input commands (`paste-buffer`, `send-keys`, `pipe-pane -I`) write to the same destination: the PTY master bufferevent** — confirmed via tmux source (`cmd-paste-buffer.c`, `cmd-send-keys.c`, `cmd-pipe-pane.c` all route through `bufferevent_write(wp->event, ...)`). That destination is the **shell's stdin**, not the pane's display. ESC bytes arriving as stdin get interpreted by readline as meta-key prefixes, not rendered as ANSI colors. The paste-buffer path was fundamentally wrong for this use case.

**Only two mechanisms actually deliver bytes to the pane's output (display) path:**

1. A process *inside the pane* that writes to its own stdout. Bytes flow out through the PTY slave → tmux's VT parser → rendered into scrollback correctly. This is what tmux-resurrect does via `cat FILE; exec $SHELL`.
2. External process writes directly to the pane's slave PTY device (`/dev/pts/<N>` via `#{pane_tty}`). Fast, clean — but has positioning race issues (shell has already prompted by the time an external writer arrives).

### Decision

**Option X — blocking helper pre-shell via FIFO.**

Each skeleton-restored pane is created with a command:
```
portal state hydrate --fifo FIFO --file SCROLLBACK; exec $SHELL
```

**`portal state hydrate`** (Go binary, internal subcommand) on startup:
1. Opens FIFO for reading, blocks.
2. When signal arrives: close FIFO, `os.Remove` it.
3. Emit terminal-state reset **preamble** to stdout: `\033[?25h\033[?1049l\033[0m` (cursor visible, exit alt-screen defensively, SGR reset).
4. Copy scrollback file bytes to stdout.
5. Emit reset **postamble** + `\r\n`: `\033[?25h\033[?1049l\033[0m\r\n`.
6. Exit.

Bytes flow through the helper's stdout → PTY slave → tmux's VT parser → rendered into scrollback natively with full ANSI fidelity. The subsequent `exec $SHELL` takes over the same process, producing zero shell history pollution (the shell never sees `cat` or `portal state hydrate`).

**Signal mechanism: FIFO per pane.**
- Skeleton restore `mkfifo ~/.config/portal/state/hydrate-{paneKey}.fifo` before creating each pane.
- `client-session-changed` hook fires `portal state signal-hydrate` or similar, which for each skeleton-marked pane in the attached session writes a byte to the FIFO.
- Helper's blocked `read()` returns, helper proceeds to dump-and-exit.
- Helper unlinks its own FIFO on wake; lingering FIFOs from crashed helpers get swept by a state-dir scan on next bootstrap.

**Timeout: 3 seconds.**
- Normal signal latency: ~10-50ms.
- Slow-but-legit upper bound (NFS home, heavy load, slow hook script): ~1-2s.
- 3s = ~2× the slow-legit tail. Fast enough to degrade snappily on real failures without cutting off rare slow-legit cases.
- On timeout: helper proceeds WITHOUT dumping scrollback — just emits reset preamble + CRLF and exits. Pane degrades to empty shell. Marker `@portal-skeleton-<key>` is NOT cleared on timeout — next attach re-signals, retry happens automatically.
- Warning logged to Portal's log file so the failure is observable.

### Validation

Mechanism validated empirically on an isolated tmux socket (`tmux -L portal-hydrate-validate-<pid>`) without touching the default socket. Confirmed:

- `cat FILE; exec bash` pattern: 1000-line ANSI-laden scrollback rendered correctly; clean `bash-5.3$` prompt at end.
- Shell history check: history file contained only the post-validation `history` command — no cat, no helper, no scrollback content. Clean.
- Blocking-FIFO variant: pane empty before signal; after `echo "go" > fifo`, scrollback rendered + shell prompt appeared. Identical final state to the immediate variant.
- Default socket sessions identical before and after test. No cross-contamination.

### Why not alternatives

- **`paste-buffer` / `send-keys` / `pipe-pane -I`**: broken. Bytes go to shell stdin, ANSI corrupted, shell state polluted. Confirmed via tmux source review.
- **Direct `/dev/pts/<N>` write** (option Y): viable but positioning race — shell has prompted before the write, so content appears *after* the prompt. Mitigation via `\033[2J\033[H` clear + SIGWINCH redraw is feasible but complex. Option X's pre-shell pattern avoids the race entirely by running the helper *before* the shell.
- **Fully-eager at skeleton restore** (option Z): considered. Would eliminate attach-time latency entirely at the cost of ~2-15s boot delay. Kept lazy for the "sessions I never touch today cost zero" property. Can switch later if real-world attach latency becomes a complaint.
- **Zellij-style confirmation prompt**: Zellij pauses before re-running saved commands on restore (safety pattern). Not relevant here — Portal's resume hooks are explicit opt-in via `portal hooks set`, so registration = consent. No extra confirmation needed for scrollback injection (replaying the user's own history into their own pane).

### Marker coordination (amended)

The earlier framing had the attach hook injecting scrollback directly (via paste-buffer). That was broken. Amended flow:

- Skeleton restore sets `@portal-skeleton-<key> 1` for each pane. Same semantic as before — means "awaiting hydration, save process must skip this pane."
- `client-session-changed` hook fires `portal state signal-hydrate`, which for each pane in the attached session:
  1. Checks for `@portal-skeleton-<key>` marker.
  2. If set: writes byte to FIFO to unblock helper; unsets marker (after confirming helper likely woke — a small delay or blocking write to the FIFO is fine).
  3. If absent: no-op (pane already hydrated, or was never skeleton-restored).
- Helper wakes, dumps, exits. Shell starts.
- Marker was unset as part of the signal step, so save process starts capturing this pane normally on the next tick.

### Failure modes

- **Scrollback file missing** on helper startup: helper logs warning, emits reset preamble only, no dump, exits. Marker was already unset by signal code. Empty pane, shell prompt. Degraded, not stuck.
- **FIFO pre-opened but hook handler crashes before writing**: helper blocks until 3s timeout, proceeds to degrade to empty shell. Marker stays set, next attach retries.
- **Helper crashes during dump**: pane ends up with partial content + dead process. Shell never starts. User sees a stuck pane. Recovery: kill the pane manually; next bootstrap will skeleton-restore it again (structure) but scrollback file may have been mid-dump, so some bytes may be missing — not corruption, just truncation of the visual history.
- **Signal fires twice somehow**: second write to FIFO goes nowhere (helper already read and unlinked). Harmless.

### Implementation notes

- FIFOs are POSIX primitives; `os.Mkfifo` in Go via `syscall.Mkfifo`. Cross-platform on Linux + macOS.
- The `; exec $SHELL` chain is a shell construct — pane command is `sh -c 'portal state hydrate ...; exec $SHELL'` to parse correctly.
- Reset sequences are short strings; total preamble+postamble overhead is ~20 bytes per pane.
- The helper's blocking read must use `O_RDONLY | O_NONBLOCK`-style or `select`-based read with timeout in Go; straight blocking `ReadFile` doesn't time out. Stdlib provides `io.Reader` + `time.After` via goroutines + channels.

### Confidence

High on mechanism, medium on fine details. Validated empirically on isolated tmux. The FIFO/timeout/reset-sequence specifics are small enough that if something needs tuning (timeout length, whether to reset before as well as after), it's a localized change, not an architecture revision.

---

## Shell Readiness + Hook Firing Redesign

### Context

The existing Portal hook system fires stored commands via `tmux send-keys` on attach (via `ExecuteHooks` called from `cmd/open.go` / `cmd/attach.go`). To make this work with reboot-recovery semantics, it uses a workaround: `portal hooks set` sets a volatile `@portal-active-<pane>` server-option marker at registration time, and `ExecuteHooks` skips any pane with the marker already set. The marker is volatile (cleared on server restart), so post-reboot the hook fires on first attach. This was designed before Portal took charge of resurrection — the marker-at-registration is a hack to suppress the "fire on every attach" behavior of the underlying send-keys model.

The `send-keys` path also brings a shell-readiness problem: if the shell isn't ready to accept input when `send-keys` fires, keystrokes get queued and processed once the shell is ready — mostly fine in practice, but fragile under slow shell startup (zsh + oh-my-zsh + nvm + pyenv can be 2-5s).

Under the new restore design, both concerns collapse together.

### The insight

Hooks are for reboot recovery. A hook "firing" means "re-launch this process when the pane comes back from the dead." The only moments when a pane comes back from the dead are:

1. After a server restart — skeleton restore creates the pane fresh with the hydrate helper chain.
2. That's it.

Within a single server lifetime, a pane that still exists doesn't need its hook fired again — the process either still exists or was explicitly killed by the user. Firing `claude --resume` on a detach/reattach of a pane that already has Claude running would actively break things.

So: **hook firing belongs in the hydrate helper's exec chain**, and nowhere else.

### Decision

**Delete the existing `ExecuteHooks` attach-time firing and its `@portal-active-<pane>` marker entirely.** Replace with helper-driven hook execution as part of the restoration path.

**New hydrate helper flow:**

```
portal state hydrate --fifo F --file S:
  1. open FIFO for reading, block (3s timeout)
  2. on signal: close + unlink FIFO
  3. emit reset preamble to stdout
  4. copy scrollback file to stdout
  5. emit reset postamble + CRLF
  6. read hooks.json, look up hook for this pane's structural key
  7. if hook exists: exec `sh -c 'HOOK; exec $SHELL'`
     else:          exec $SHELL
```

When the hook command exits, `exec $SHELL` takes over (via the chained `; exec $SHELL` after `HOOK`). User is dropped into an interactive shell cleanly.

**What gets deleted:**
- `ExecuteHooks` function and its call sites in `cmd/open.go` / `cmd/attach.go`.
- `@portal-active-<pane>` marker logic in `portal hooks set` — registration becomes a pure write to `hooks.json`.
- All attach-time hook checking.

**What stays:**
- `hooks.json` storage.
- CLI: `portal hooks set`, `portal hooks list`, `portal hooks rm` — unchanged user-facing surface, just the internals are simpler.
- `CleanStale` for pruning orphaned hook entries.

### Why this isn't a regression

The only scenario where this would behave differently from the current design is "hook registered on a pane that hasn't gone through a Portal save/restore cycle yet, user reattaches before reboot." In the old design, `ExecuteHooks` would have fired the hook via `send-keys` (once per server lifetime, thanks to the marker). In the new design, the hook doesn't fire until reboot triggers skeleton restoration.

But this "live attach firing" was the *problem*, not a feature:
- Firing `claude --resume` on a live pane that already has Claude running would break things.
- The marker-at-registration workaround existed specifically to prevent this.
- The only reason it sort-of worked was that the marker made firing a one-shot per server lifetime.

Under the new design, this correctly-desired behavior falls out naturally: hooks fire exactly when a pane is freshly recreated from saved state. Not before, not after. Matches the user's mental model of "hooks are for reboot recovery."

### Coverage across attach paths

Because `client-session-changed` is registered globally by Portal's bootstrap, hook execution works regardless of attach path:

- `portal open` / `portal attach` / picker selection → client-session-changed fires → signal handler triggers helper FIFO signals
- `tmux attach -t NAME` directly → same `client-session-changed` fires → same path
- `switch-client` from inside tmux → same path

Identical UX across all paths. The user doesn't need to know "hooks only work if you use Portal to attach."

### Shell readiness — no longer a concern

Under the old model, the `send-keys` path needed the shell to be ready to accept keystrokes. Under the new model, nothing uses `send-keys` for hook firing. Hook exec happens from inside the pane's helper process, replacing itself with the hook (or shell) via `exec` — no shell needed to be "ready" because no shell is receiving anything.

There's still a shell running after the hook exits (`exec $SHELL` via the `;` chain). But by the time the user is interacting with it, it's fully initialized. No readiness detection needed.

### What this does to Portal's code shape

- `cmd/hook_executor.go` → deleted (or gutted to a no-op stub, then removed in a cleanup pass).
- `cmd/open.go` / `cmd/attach.go` → attach flow no longer calls `ExecuteHooks`.
- `internal/hooks/executor.go` → deleted.
- `cmd/hooks.go::hooksSetCmd` → no longer calls `SetServerOption` for the marker.
- `portal state hydrate` (new subcommand) → reads `hooks.json`, does the exec chain.
- `portal state signal-hydrate` (new subcommand, invoked by the `client-session-changed` hook) → iterates skeleton-marked panes in the attached session, writes to their FIFOs.

Net simplification: a whole execution path is removed, replaced by inclusion of the hook into an exec chain that was going to exist for hydration anyway.

### Confidence

High. The realization that hooks belong only in the helper exec chain is a clean mental model — and removing the attach-time firing eliminates an entire workaround layer (the marker-at-registration hack). The scenarios check out identically to current expected behavior for all real use cases.

---

## Layout Restoration Approach

### Context

For each window being restored, tmux's `select-layout` applies a saved layout string to a set of panes. Research confirmed `window_layout` (the format variable we capture) returns the pre-zoom form — correct for restoration. This sub-item is mostly mechanical; the interesting parts are ordering, edge cases, and failure handling.

### Decision

**Per-window restoration order:**

1. Create the window (`new-window` in our case — first pane is created implicitly).
2. Create remaining panes via `split-window` to reach the saved pane count. Direction arguments are arbitrary — the next step rearranges.
3. `select-layout "<saved layout string>"` — tmux parses the string and fits the panes to the saved geometry.
4. `select-pane -t <saved active pane index>` to set the active pane within the window.
5. If `window_zoomed_flag` was true at save time: `resize-pane -Z` on the active pane to re-apply zoom.

This order matches tmux-resurrect's proven approach. Zoom must come after layout because `resize-pane -Z` operates on the current layout geometry.

### Edge cases

**Pane count mismatch.** `select-layout` requires the pane count to match the saved string. Portal's restoration creates exactly the right count from `sessions.json` before applying layout. If state is corrupted (JSON says 3 panes, we only created 2), `select-layout` fails.

**Failure handling**: if `select-layout` returns an error:
- Log a warning identifying the session/window/pane count mismatch.
- Fall back to `select-layout tiled` (tmux's auto-balanced tiled layout) so panes are at least visible in a sane arrangement.
- Continue restoring other windows/sessions.

Degraded but not stuck. Consistent with the scrollback "missing file → empty pane, log, proceed" pattern.

**Terminal size drift.** Layout strings encode absolute pane dimensions. If saved on 200×50 and restored into 80×24, `select-layout` does best-effort fitting — proportions shift, some panes may be cramped. Neither Portal nor any other tool solves this at the tmux level; it's a fundamental tmux constraint.

We don't do any special handling for this. `select-layout` fits as best it can; the user can resize their terminal to match their saved layout if they care. `select-layout -E` ("spread evenly") is an option but it loses the saved proportions — trading faithful-but-cramped for even-but-different. Default to faithful.

### What about zoom state being stored in the layout string itself?

Research was explicit: `window_layout` returns the pre-zoom form, which is what we want — trying to save a zoomed layout string and re-applying it would collapse panes incorrectly. Portal captures `window_layout` (not `window_visible_layout`) and `window_zoomed_flag` separately, and re-applies zoom as step 5.

### Summary

- Order: create window → create all panes → `select-layout "<saved>"` → `select-pane -t <active>` → `resize-pane -Z` if zoomed.
- On `select-layout` failure: log warning, fall back to `select-layout tiled`, continue.
- No special handling for terminal size drift — tmux's best-effort fit is accepted as the limit.

Mechanical and low-risk. Confidence: high.

---

## Fate of WaitForSessions / bootstrapWait

### Context

Current flow (per `CLAUDE.md` and research):

> `PersistentPreRunE` calls `EnsureServer()`... If the server was just started, TUI shows a loading page; CLI commands call `bootstrapWait()` which prints to stderr and polls for session restoration (1–6s window).

`WaitForSessions` and `bootstrapWait` exist because Portal had no control over *when* resurrect/continuum would populate sessions. They were polling hedges against the upstream plugins being slow or failing. Under the new design, Portal owns restoration directly — there's nothing to wait for.

### Decision

**Delete `WaitForSessions` and `bootstrapWait` entirely.** Replace with a synchronous `Restore()` call in `PersistentPreRunE` right after `EnsureServer()`. When `Restore()` returns, skeleton-restored sessions exist because Portal just created them. No polling needed.

**Keep the loading page for the TUI path, with a minimum display time.**

Skeleton restoration typically completes in ~600ms for a heavy 10-session config — fast enough that the loading page could flash in and out too quickly to register as intentional UX. That "flash" feels like a glitch rather than a deliberate moment. Standard fix: enforce a minimum display time.

**Minimum loading-page duration: 1.2 seconds.**
- Long enough to feel intentional.
- Not so long it becomes friction.
- Middle of the 1-1.5s range we discussed.

Implementation:
```go
start := time.Now()
// show loading page ...
// skeleton restore runs ...
elapsed := time.Since(start)
if elapsed < minLoadingDuration {
    time.Sleep(minLoadingDuration - elapsed)
}
// dismiss loading page
```

If restoration happens to take longer than 1.2s (many sessions, slow disk), the loading page stays until restoration completes. If faster, it stays for 1.2s total.

**CLI path (non-TUI invocations like `portal attach NAME`): silent wait.** No loading page, no stderr progress output. Typical restoration is ~600ms — fast enough to not need user-facing progress. If someone later complains about long-feeling CLI waits, we can add a "Restoring..." line to stderr if elapsed > 2s. YAGNI until then.

### Code shape

- `internal/tmux/wait.go` (where `WaitForSessions` lives) → deleted.
- `bootstrapWait` function → deleted.
- `PersistentPreRunE` flow becomes: `EnsureServer()` → `Restore()` (sync). TUI path wraps this with loading-page + min-duration logic. CLI path just calls through.

### What stays

- `EnsureServer()` — keeps its job of starting the tmux server if not running.
- `serverStarted` flag in context — still used elsewhere (e.g., decision whether to show bootstrap messages).
- The loading page itself — retained for the TUI path with minimum-display-time padding.

### Improvement over current behavior

- Current: "waiting for sessions to populate (up to 6s)"
- New: "restoring sessions" — bounded by the minimum display time floor (1.2s) and restoration's actual cost (~600ms). No external dependency, deterministic timing.

### Confidence

High. Straightforward refactor — removing polling code that existed to hedge against an external dependency that no longer exists.

---

## Bootstrap Integration (Full Flow)

### Context

Everything we've decided so far needs to stitch together into a single, coherent bootstrap path. This sub-item isn't a new decision — it's the integration pass that checks all the pieces fit and orders correctly.

### The full flow

**`PersistentPreRunE` on every `portal open` / `portal attach` / `portal hooks set` / etc. (any Portal command that needs tmux):**

1. **`EnsureServer()`** — start tmux server if not running. Set `serverStarted=true` in context if we started it.

2. **Register global hooks via `set-hook -ga` with content-based idempotency check**:
   - Structural save triggers → `portal state notify`: `session-created`, `session-closed`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`.
   - Hydration trigger → `portal state signal-hydrate`: `client-session-changed`.
   - All wrapped in `command -v portal >/dev/null 2>&1 && ...` defensive guard.
   - Parse `show-hooks -g` first; skip events that already have the target Portal command present.

3. **Set `@portal-restoring 1`** — save process will skip all captures while set. Must happen *before* `_portal-saver` creation so any `session-created` fired by subsequent steps doesn't race the daemon's first tick.

4. **`_portal-saver` session setup** (idempotent):
   - `has-session -t _portal-saver` → if present, skip creation.
   - If absent: `new-session -d -s _portal-saver "portal state daemon"`.
   - Check `daemon.version` file against `cmd.version` — if mismatch, `kill-session -t _portal-saver` and recreate.
   - Always `set-option -t _portal-saver destroy-unattached off` (defensive).

5. **`Restore()` — skeleton-only restoration** (loading page shown for TUI path; silent for CLI):
   - If `sessions.json` doesn't exist: no-op.
   - Read `sessions.json`, parse.
   - For each saved session:
     - `has-session -t <name>` → live exists → skip this session.
     - Else: skeleton-create.
       - Compute FIFO path per pane (`~/.config/portal/state/hydrate-<paneKey>.fifo`).
       - `mkfifo` for each pane before creating the pane.
       - `new-session -d -s <name> -c <root_cwd> "portal state hydrate --fifo <F> --file <scrollback>"` for the first pane.
       - `new-window`, `split-window` for remaining windows/panes to match saved structure; each with its own `hydrate` command.
       - `select-layout "<saved>"` per window → `select-pane -t <active>` → `resize-pane -Z` if zoomed.
       - For each created pane: `set-option @portal-skeleton-<key> 1`.

6. **Unset `@portal-restoring`.** Save loop resumes normal operation on its next tick.

7. **`CleanStale()`** — prune stale entries from `hooks.json` whose structural keys don't match any currently-live pane. Runs here because by this point, live panes include both pre-existing and skeleton-restored ones.

8. **Return to the calling command.** TUI wraps steps 1-7 with loading page (1.2s min display); CLI returns immediately.

### Attach flow (after bootstrap, when user selects or attaches a session)

1. Portal's open/attach code resolves the target session.
2. `tmux switch-client -t <session>` (inside tmux) or `exec tmux attach -t <session>` (outside). This is Portal's existing AttachConnector / SwitchConnector logic.
3. `client-session-changed` hook fires automatically (tmux event — we registered the hook globally in step 2 of bootstrap).
4. Hook command runs `portal state signal-hydrate` as a subprocess, with the attached session name available via tmux format expansion.
5. Subprocess:
   - Enumerates panes in the attached session.
   - For each pane with `@portal-skeleton-<key>` set:
     - Writes a byte to its FIFO to unblock the helper.
     - Unsets `@portal-skeleton-<key>` so save loop resumes capturing this pane.
6. Helpers unblock, each does:
   - Reset preamble → dump scrollback → reset postamble.
   - Read `hooks.json`, look up hook for own pane key.
   - If hook exists: `exec sh -c 'HOOK; exec $SHELL'`.
   - Else: `exec $SHELL`.
7. User is now in the session with restored scrollback, hook running (or shell if no hook).

### Save loop (continuous, inside `_portal-saver`)

Recapped for completeness from Save-Side Architecture decisions:

- 1s ticker in the hosted Go daemon.
- Each tick: check `@portal-restoring` — if set, skip entire tick.
- Otherwise: check `save.requested` dirty flag OR `timeSinceLastSave >= 30s` max-gap. If either, capture.
- Capture: for each live pane whose `@portal-skeleton-<key>` is NOT set, capture structural + scrollback. Content-hash dedup (skip unchanged).
- Write scrollback files atomically (temp + rename), write `sessions.json` atomically last (the commit).
- GC: remove orphaned scrollback files not referenced by the new `sessions.json`.

### Ordering correctness check

The critical ordering: `@portal-restoring` must be set (step 3) **before** `_portal-saver` is created (step 4), because creating `_portal-saver` fires `session-created` which in our new flow touches the dirty flag. Without `@portal-restoring`, the daemon's first tick could try to save while restoration is mid-build.

With the ordering above:
- `@portal-restoring` set → daemon's first tick no-ops.
- Restoration runs → more `session-created`/`window-linked` events fire → dirty flag bumped many times.
- `@portal-restoring` unset → next daemon tick runs, captures the now-complete state. Clean.

Similarly, `@portal-skeleton-<key>` markers are set as each pane is created, so even after `@portal-restoring` is unset, the daemon won't try to save unhydrated panes.

### Return-to-caller timing

**TUI path**: bootstrap runs, 1.2s minimum loading-page display, TUI appears with populated picker. User selects, attach flow runs.

**CLI path** (e.g. `portal attach NAME`): bootstrap runs silently, then `attach NAME` logic resolves the target and attaches. If the target wasn't in sessions.json (fresh session), it just works. If it was in sessions.json, skeleton was restored before attach, so `has-session -t NAME` returns true when needed.

### What's decided vs. implementation

Everything on the Restore-Side decision tree is now settled at the design level. Implementation will have smaller details to pin down:
- Exact tmux command sequences for session creation (flags, error handling on each).
- Go error-propagation strategy for partial failures.
- FIFO cleanup for panes whose helpers never got signalled (state-dir scan on bootstrap to remove stale FIFOs).
- Unit tests using isolated tmux sockets for restoration correctness.

These belong in the Planning phase, not here.

### Confidence

High. All sub-items check out individually; the ordering tweak (set `@portal-restoring` before `_portal-saver` creation) is the only integration-level gotcha and it's been captured.

---

## CleanStale Guard Behavior

### Context

`CleanStale` removes entries from `hooks.json` whose structural keys don't match any currently-live pane. Under the old design it had a guard: skip cleanup when `list-panes -a` returns empty, to avoid nuking all hooks before resurrect/continuum had a chance to restore panes after reboot.

That guard was a hedge against Portal not owning restoration.

### Decision

**Delete the `livePanes empty → skip` guard.** CleanStale runs unconditionally, trusting live tmux state.

Under the new bootstrap flow, CleanStale runs in step 7 of `PersistentPreRunE`, *after* skeleton restore completes. At that point, live panes include both pre-existing panes and skeleton-restored ones — so if `list-panes -a` is empty, there genuinely are no sessions, and any hooks.json entries are genuinely orphaned.

### Where CleanStale runs

- **Bootstrap step 7**: every `PersistentPreRunE` invocation, after skeleton restore. Keeps `hooks.json` consistent with every Portal command.
- **Explicit `portal clean` command**: user-initiated. Same logic, no guard.

### Refactor scope

Delete the `if len(livePanes) == 0 { return }` early return in CleanStale's current implementation. Everything else (structural-key matching, atomic write of updated `hooks.json`) stays. A few lines removed, net simpler.

### Stale-hook detection criteria (unchanged)

Keep the existing criterion: an entry is stale if its structural key (`session:window.pane`) doesn't match any live pane enumerated by `list-panes -a`. Review-002 raised whether we should also detect stale hooks by "binary missing" or "project removed from projects.json" — not needed. Those are user-responsibility concerns (if your binary is missing, your hook fails; that's a runtime error, not a stale-hook issue). Keeping the criteria narrow matches the generic-hook design principle.

### Confidence

High. Small mechanical change removing an obsoleted workaround.

---

## Failure Modes & Recovery

### Context

Many failure cases have been handled inline as we worked through architectural decisions — scrollback file missing, hydrate timeout, layout parse errors, etc. This section consolidates the complete picture and fills gaps that weren't explicitly captured.

### Principle

**Degrade locally, log, continue.** No single failure should crash Portal or leave it stuck. User is never blocked. Logs capture context for diagnosis. Self-healing where possible.

### Consolidated failure-handling table

| Failure | Handling |
|---|---|
| Scrollback file missing at hydrate time | Helper logs warning, emits reset preamble only (no dump), exec's shell. Empty pane, not stuck. |
| `select-layout` fails (corrupt string, pane-count mismatch) | Log warning, fall back to `select-layout tiled`. Panes visible, structure approximated. |
| Hydrate signal never arrives (hook failure, FIFO issue) | 3s timeout; helper degrades to empty shell + logs warning. `@portal-skeleton-<key>` marker stays set, next attach retries. |
| AtomicWrite mid-crash | Temp file + rename guarantees either old or new file is intact. Next successful save produces fresh state. |
| Skeleton restore crashes partway | `@portal-restoring` cleared on server restart; next bootstrap retries from scratch. `sessions.json` still holds pre-crash state. Partial tmux structure from the crashed attempt doesn't block re-restore because `has-session` check skips already-live names; newly-created partial state becomes the live state. |
| Orphaned scrollback files after interrupted save | GC step at end of each successful save removes files not referenced by new `sessions.json`. Self-healing. |
| Orphan FIFO from crashed helper | State-dir scan on bootstrap sweeps stale `hydrate-*.fifo` files before re-creating new ones. |
| tmux server dies mid-save | Hosted daemon in `_portal-saver` dies with server; gets SIGHUP, flushes final save, exits. Next bootstrap re-creates saver. |
| `sessions.json` corrupt / unparseable | Log warning, skip restoration entirely, continue bootstrap. User sees empty picker. Can diagnose via file inspection. Next save will overwrite with valid content. |
| Disk full during save | `AtomicWrite` fails at write or rename step. Log error, daemon continues ticking, retries on next tick. Previous save state intact. When disk space frees, save resumes normally. Daemon never crashes from disk-full. |
| User creates new session mid-restoration | No special handling needed. Skeleton restore only touches saved sessions from `sessions.json`; pre-existing live sessions (including just-created ones) coexist. `@portal-restoring` blocks save mid-build, but user commands aren't gated. |
| Hydrate helper itself crashes mid-dump | Pane ends up with partial scrollback + dead process. Shell never starts. User sees stuck pane. Recovery: kill the pane manually; next bootstrap re-skeletons it. This is a "shouldn't happen" case but documented for completeness. |

### What's NOT handled specially

- **Terminal size drift on restoration**: `select-layout` does best-effort fit. Some panes may be cramped if terminal is smaller than save-time. Not Portal's problem to solve — documented as limitation.
- **Non-existent cwd on restore**: pane creation with `-c /path/that/no/longer/exists` — tmux's fallback is to start the shell in the user's home directory. Acceptable — no Portal-side handling needed.
- **Binary missing at runtime** (hook references `claude` but claude isn't installed): the hook fails at exec time, shell falls through via `;` chain. User sees an error message + shell prompt. Not Portal's problem; user-facing diagnostic is clear.

### User feedback on partial restore

If anything went wrong during restoration (corrupt JSON, missing files, layout fallback fired), the user should be able to find out. Two channels:

1. **Log file**: Portal writes warnings/errors to a log file in the state dir. Never silent. `portal state status` (from Observability subtopic) can surface recent warnings. Explicit diagnostic path.

2. **Visual**: no extra banners or "restoration partially failed" UI. Silent degradation is the right default — failures are rare, and nagging users about every sub-optimal restore adds friction. Log file is the diagnostic path.

### Confidence

High. All failure modes identified during architectural decisions have consistent "log + degrade locally + continue" handling. The only ambiguous case is helper-crashed-mid-dump (stuck pane requiring manual cleanup), and that's rare enough to document rather than engineer around.

---

## Summary

### Key Insights
*(To be completed during discussion)*

### Open Threads
*(To be completed during discussion)*

### Current State
- Hook Lifecycle Redesign: **decided** — no mode field; single persistent behavior; one-shot is a caller-level policy via wrapper-script lifecycle management
- Save Content & Scope: **decided** — capture structural state + scrollback + tmux per-session env on by default. Ephemeral interaction state excluded.
- Save-Side Architecture: **decided in full** — execution model (detached tmux session hosts long-running Go process), trigger mechanism (event + 30s periodic; opportunistic dropped), crash cadence (30s), signal handling (SIGHUP + SIGTERM), debouncing (single-writer via dirty flag), save format (per-pane scrollback files + sessions.json index), content-hash dedup (skip unchanged writes), CLI surface (`portal state status` user-facing; `state daemon` and `state notify` internal), tmux hook registration lifecycle (append-based coexistence via `set-hook -ga` with content-based idempotency and per-index removal; min tmux 3.0+).
- Restore-Side Architecture: **decided in full** — trigger (restore all, idempotent by name), eagerness split (skeleton-eager, scrollback-lazy), marker coordination (`@portal-skeleton-<key>` + `@portal-restoring`), scrollback injection mechanics (blocking helper pre-shell via FIFO with reset preamble/postamble and 3s timeout), hook firing redesign (delete `ExecuteHooks` + marker-at-registration, fire via helper exec chain), layout restoration (standard tmux flow with tiled fallback on failure), `WaitForSessions` deleted (replaced with synchronous `Restore()` + 1.2s min loading page), bootstrap integration ordering (set `@portal-restoring` before `_portal-saver` creation).
- Remaining: Failure Modes & Recovery, Observability & Diagnostics, CleanStale Guard Behavior, Session & Project Store Interaction, Ephemeral Session Opt-Out, Scope Boundaries
