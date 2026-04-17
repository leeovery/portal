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

  Save-Side Architecture [exploring]
  ├─ Execution model (detached tmux session host) [decided]
  ├─ Trigger mechanism (event-driven + 30s periodic; opportunistic dropped) [decided]
  ├─ Crash safety / periodic save cadence (30s max-gap) [decided]
  ├─ Signal handling (SIGHUP from PTY close, SIGTERM for direct kill) [decided]
  ├─ Debouncing / serialization (single-writer via hosted process + dirty flag) [decided]
  ├─ Save format and schema (per-pane scrollback files + sessions.json index) [decided]
  ├─ Content-hash dedup (skip unchanged scrollback writes) [decided]
  ├─ save-state CLI surface and contract [pending]
  └─ tmux hook registration lifecycle (install/uninstall/upgrade) [pending]

  Restore-Side Architecture [pending]
  ├─ Bootstrap integration [pending]
  ├─ Fate of WaitForSessions / bootstrapWait [pending]
  ├─ Shell readiness detection [pending]
  └─ Layout restoration approach [pending]

  Failure Modes & Recovery [pending]
  ├─ Corrupt / partial saved state [pending]
  ├─ Missing working directories on restore [pending]
  ├─ Layout fit failures (terminal size drift) [pending]
  └─ User feedback on partial restore [pending]

  Observability & Diagnostics [pending]
  ├─ Save-state introspection command [pending]
  ├─ Logging strategy [pending]
  └─ Health signals for silent failures [pending]

  CleanStale Guard Behavior [pending]
  ├─ Guard rationale change post-restoration [pending]
  └─ Stale-hook detection criteria (binary/dir/project missing) [pending]

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

### False paths documented

1. *"Event-driven only is sufficient"* — true for structural state, false once scrollback is in scope. Content drift between events is the dominant case.
2. *"`run-shell -b 'while true; do ...; done'` as a poor-man's daemon"* — research found no TPM plugin uses this pattern after ~10 years. Known tmux bugs around `-b` flag ([tmux#1843](https://github.com/tmux/tmux/issues/1843), [#2306](https://github.com/tmux/tmux/issues/2306)). Detached-session hosting is more battle-tested.
3. *"Full daemon like Zellij"* — Zellij has one because it IS a multiplexer; the daemon is intrinsic. Portal bolting on a daemon for one feature is a different calculus, and the "decoupled from tmux" benefit largely evaporates given that the daemon has nothing useful to do when tmux is dead.

---

## Summary

### Key Insights
*(To be completed during discussion)*

### Open Threads
*(To be completed during discussion)*

### Current State
- Hook Lifecycle Redesign: **decided** — no mode field; single persistent behavior; one-shot is a caller-level policy via wrapper-script lifecycle management
- Save Content & Scope: **decided** — capture structural state + scrollback + tmux per-session env on by default. Ephemeral interaction state excluded.
- Save-Side Architecture: **mostly decided** — execution model (detached tmux session hosts long-running Go process), trigger mechanism (event + 30s periodic; opportunistic dropped), crash cadence (30s), signal handling (SIGHUP + SIGTERM), debouncing (single-writer via dirty flag), save format (per-pane scrollback files + sessions.json index), content-hash dedup (skip unchanged writes). CLI surface and hook registration lifecycle still pending.
- Remaining: finish Save-Side Architecture sub-items, Restore-Side Architecture, Failure Modes & Recovery, Observability & Diagnostics, CleanStale Guard Behavior, Session & Project Store Interaction, Ephemeral Session Opt-Out, Scope Boundaries
