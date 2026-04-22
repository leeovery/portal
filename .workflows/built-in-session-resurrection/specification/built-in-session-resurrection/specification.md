# Specification: Built-In Session Resurrection

## Specification

## Overview

Portal will own the full tmux session lifecycle end-to-end: server start → session restoration → resume hook execution. The current middle step depends on tmux-resurrect/continuum, which fails to reliably restore sessions after reboot, breaking the resume hook feature despite the hook code being correct.

This replaces reliance on external plugins with a built-in save/restore mechanism.

## Product Goal

**"Zellij in tmux."** When a user reboots, their tmux sessions come back as they were — structure *and* content. Restoration is automatic, on by default, with no opt-in required.

## Organizing Principles

- **Portal owns the full lifecycle.** Save, restore, and hook execution are all internal. No external plugin dependencies for resurrection.
- **Portal's hook system is generic.** Portal stores and fires command strings. It has no awareness of what consumers do with them. Caller wrapper scripts own process-specific policy (e.g., re-registering dynamic hooks).
- **Portal does not maintain a separate session registry.** Live tmux state is read directly via `list-panes -a`, `list-sessions`, etc. Portal's saved state mirrors tmux state; it never diverges from or substitutes for it.
- **Portal captures all sessions — both Portal-created and native tmux.** Consistent with existing behavior. Sessions prefixed with `_` are reserved for Portal internals and excluded from capture, restore, and the TUI picker.
- **Bootstrap is the natural point for restoration.** Portal is always the entry point; `PersistentPreRunE` already runs before every user-facing command.
- **Degrade locally, log, continue.** No single failure may crash Portal or leave the user stuck. All failures log to a file and degrade the affected pane/session to a workable state.
- **Single-writer architecture.** All state-file writes funnel through one long-running process. Other triggers signal via a dirty flag. Eliminates write races by construction.
- **YAGNI rigorously.** Speculative features (ephemeral opt-out, background prefetch, compression, multi-host sync) are deferred until concrete user demand surfaces.

## Scope & Constraints

### Minimum Versions

- **tmux ≥ 3.0** (Feb 2020). Array-indexed hooks (`set-hook -a` semantics) require 3.0; the hook-lifecycle coexistence model depends on them.
- **Go, macOS, Linux**: inherits Portal's existing requirements. No change.

### In Scope — Captured and Restored

**Structural state:**
- Session names
- Window indices, names, layout strings, active flag, zoom flag
- Pane indices, current working directories, active flag

**Content:**
- Full pane main-screen scrollback with ANSI escape sequences — colors, attributes, formatting preserved via `tmux capture-pane -e -p -S - -t <pane>`
- tmux per-session environment via `show-environment -t <session>` — the tmux-level env used for initializing new panes (not live shell env). Restored in full without filtering; tmux's own `update-environment` refreshes stale values (`SSH_AUTH_SOCK`, `DISPLAY`) on client attach.

**Already stored elsewhere (unchanged):**
- Resume hooks (`hooks.json`)

### Out of Scope — Ephemeral Interaction State

Excluded from capture even though tmux exposes them, because they are transient:
- Copy mode state
- Active selections
- Paste buffers
- Cursor position within panes
- Scroll position within scrollback
- Per-client state (which client has which pane focused, client-specific dimensions)

### Out of Scope — Uncapturable by tmux

Not Portal's problem to solve. Users can compensate via resume hooks where meaningful:
- Live shell environment variables — tmux cannot observe shell-side `export`. Callers compensate via resume hooks if they care.
- Running process state (REPL state, interactive sessions like vim/htop/less) — alt-screen buffer contents are explicitly not scrollback. The resume hook system exists specifically for process relaunch.
- Open file descriptors, pipes, sockets, ptrace state.

### Explicit Non-Goals

- **Live shell environment variables.** Scope-boundary non-goal; see above.
- **Running process state.** Same.
- **tmux server-level options and user's `.tmux.conf` customizations.** Portal reads live tmux state via standard APIs; does not introspect or capture config files. Restoration uses the user's current `.tmux.conf` as baseline.
- **Third-party tmux plugin state.** Portal coexists with other TPM plugins via hook-append semantics but does not capture, understand, or interact with their state.
- **Multi-server, multi-host, remote sync.** Portal works with a single local tmux server per machine. No cross-machine sync, no remote tmux, no session groups distributed across hosts.
- **Non-tmux multiplexers.** Portal is tmux-specific. Zellij, GNU screen, wezterm, abduco — not supported.
- **Shell-specific behavior.** Portal is shell-agnostic. Helper `exec`s `$SHELL`. No special-casing bash/zsh/fish.
- **Mouse state / clipboard state.** Ephemeral interaction state.
- **Generic tmux option capture** (session/window/pane options). Nearly all tmux options are set globally via `~/.tmux.conf` and apply on restore automatically. Per-session overrides are niche. Capturing them requires diffing `show-options` against global defaults — complexity not justified. If a specific flag becomes important, add it as an explicit field later.
- **Marks / position bookmarks.** tmux's `<prefix>m` marks panes (not scrollback positions), and copy-mode position marks have no tmux API to capture or restore. No functional gain.
- **Last-pane tracking.** No confirmed tmux format variable exposes this.

### Deferred (Not Non-Goals, Just Not Now)

- **Ephemeral session opt-out** (per-session/per-pane exclusion from capture). Speculative without concrete user demand. `history-limit 0` on a window is the tmux-native workaround documented in the README.
- **Scrollback compression.** Deferred until disk usage becomes a complaint.
- **Parallel capture** for many-pane configurations. Deferred until performance complaints surface.
- **Schema migration (v1 → v2).** Standard practice when the time comes; not a v1 design decision.
- **Background prefetch / full-eager hydration.** Rejected in favor of pure-lazy hydration; can be reconsidered if attach-time latency becomes a complaint.

## Hook System Lifecycle Behavior

### Behavior

Portal's resume hook system has **a single persistent behavior**. Hooks registered via `portal hooks set --on-resume "cmd"` live in `hooks.json` across reboots and survive tmux server restarts. They are removed only by explicit `portal hooks rm --on-resume` invocation.

There is no `mode` field, no `once` vs `always` flag, no per-hook lifetime configuration. Portal is a mechanism; hook lifecycle policy (one-shot, bounded-lifetime, dynamic re-registration) is the caller's responsibility.

### Caller Pattern for Dynamic or One-Shot Semantics

Long-running processes that need dynamic hook management (e.g., `claude --resume <uuid>` where the UUID changes each session) are invoked via a wrapper script that owns the hook lifecycle:

- Wrapper calls `portal hooks set --on-resume "<cmd>"` when the process starts, inferring the current pane from `TMUX_PANE`.
- Wrapper re-registers on each resume if the command includes dynamic state (e.g., a new resume UUID).
- Wrapper calls `portal hooks rm --on-resume` on explicit process exit, via an exit trap or teardown routine.

Portal exposes `set` and `rm` as primitives; callers wire them into their own lifecycle hooks.

### Rejected Alternative: `&&` Shell-Chaining for Self-Removal

A pattern like `portal hooks set --on-resume "my-cmd && portal hooks rm --on-resume"` does **not** work for the flagship class of hook commands. Long-running processes (`claude --resume`, `npm start`, `tail -f`) never exit, so the `&&` clause never fires, and the hook never self-removes. This pattern is architecturally broken for the exact use case hooks exist to serve and is not supported as a recommendation.

### CLI Surface (Unchanged User-Facing Shape)

- `portal hooks set --on-resume "<cmd>"` — register or replace the resume hook for the current pane (pane inferred from `TMUX_PANE`, keyed by structural key `session:window.pane`).
- `portal hooks list` — list registered hooks.
- `portal hooks rm --on-resume` — remove the resume hook for the current pane.

The storage file (`hooks.json`) and structural-key scheme are unchanged. Internal implementation changes are covered in "Resume Hook Firing" and "CleanStale Behavior" sections below.

### Rationale

A mode field can be added later if a concrete use case surfaces where wrapper-script management is genuinely impractical. None was identified during discussion — the flagship Claude use case is satisfied by caller-side wrapping, and static commands (`npm start`, file watchers, `tail -f`) only make sense as persistent.

## Save-Side Architecture: Execution Model

### Host Process: Detached tmux Session

Portal hosts its long-running save process inside a detached tmux session named `_portal-saver`. The session's command is `portal state daemon` — a long-running Go process. tmux owns the process lifecycle: when tmux terminates, the PTY is closed, the kernel delivers SIGHUP to the daemon, and the daemon flushes its final state before exiting.

**Why a detached tmux session (not a platform-specific service and not subprocess-per-event):**

- tmux already owns process supervision. No launchd/systemd per-platform code, no install/uninstall service lifecycle, no double-fork fallback with silent-failure mode.
- The daemon has nothing useful to do when tmux is dead, so tying its lifetime to tmux's is correct.
- Portal creates `_portal-saver` idempotently at bootstrap. Crash recovery is automatic — a dead session is recreated on the next `portal open`.
- Pattern has real-world precedent (tmux-slay). The concerns are concrete and addressed.

**Why not subprocess-per-event:** structural-event-driven saves miss scrollback drift. A user sitting in one pane with a program outputting for hours produces no structural events; crash loses everything since the last event.

**Why not a full external daemon (Zellij path):** ~500 LOC of platform-specific lifecycle code (install, supervise, PID files, IPC, upgrade). Zellij's daemon is intrinsic to the tool; Portal bolting one on for one feature is a different engineering calculus.

### Session Visibility and Filtering

`_portal-saver` shows up in `tmux ls`. There is no tmux mechanism to hide it. Portal filters sessions whose names begin with `_` (underscore prefix is reserved for Portal internals) from:

- The TUI session picker.
- `sessions.json` capture (the save process skips `_*` sessions when enumerating live state).
- Any future internal-only sessions.

This keeps the internal machinery invisible in Portal's own UX while remaining inspectable via `tmux ls` for debugging.

### Defensive Session Setup

On every `EnsureServer()` call, Portal runs `set-option -t _portal-saver destroy-unattached off` unconditionally (idempotent). This defends against users with `destroy-unattached on` set globally in `.tmux.conf` — without this override, their global setting would kill `_portal-saver` immediately on creation (the session is `-d` and has zero attached clients).

### Signal Handling

The daemon traps two signals:

- **SIGHUP** — delivered by the kernel when tmux closes the PTY master fd. This is the dominant shutdown path (tmux `kill-server`, server crash, reboot). Discussion verified the kernel sends SIGHUP, not SIGTERM, in this case — Portal must trap SIGHUP explicitly.
- **SIGTERM** — delivered by direct `kill <pid>` from outside tmux. Less common but handled for completeness.

Handler behavior:

1. If the `@portal-restoring` marker is set, skip the final flush (an in-progress restore is underway; capturing now would capture mid-transition state).
2. Otherwise, flush the current state atomically via `AtomicWrite` and exit.

No configurable grace period. Atomic rename guarantees either the old or new state file is always valid on disk, so there is no mid-write corruption risk from signal-timing.

### Lifecycle Summary

- **Creation:** `EnsureServer()` calls `has-session -t _portal-saver`. If absent, `new-session -d -s _portal-saver "portal state daemon"`.
- **Auto-destroy:** when the daemon exits (normal shutdown, crash, or version-mismatch restart), tmux's default session-auto-destroy behavior removes the session. No `remain-on-exit` tuning required.
- **Recreation:** the next `portal open` after destruction runs bootstrap, finds `_portal-saver` absent, and recreates it.
- **Version-based restart:** see the "tmux Hook Registration Lifecycle" section for the version-marker restart protocol.

## Save-Side Architecture: Triggers & Serialization

### Trigger Layers

Two complementary trigger mechanisms ensure both responsiveness and bounded data loss:

**1. Event-driven (immediate response).** tmux global hooks (`set-hook -ga`) fire on structural events. Each event runs `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`, which signals that state is dirty.

The events:
- `session-created`
- `session-closed`
- `session-renamed`
- `window-linked`
- `window-unlinked`
- `window-layout-changed`
- `pane-focus-out`

These catch structural changes (session/window/pane topology, renames, layout changes, focus transitions) as they happen.

**2. Periodic (bounded worst-case).** The hosted daemon runs a 1-second ticker internally. Each tick: if the dirty flag is set OR it has been ≥30 seconds since the last save, capture and persist state; otherwise no-op.

- **Dirty-flag check** produces ≤1 second event-to-save latency.
- **30-second max-gap** bounds worst-case scrollback loss on unexpected tmux/system termination at 30 seconds, even during periods with zero structural events.

### No Opportunistic Trigger

Earlier framing proposed additional saves fired from `portal open` / `portal attach` based on last-save staleness. This is **not** included. If the hosted daemon is running, it is already saving via events + ticker. If it is not running, `EnsureServer()` will recreate `_portal-saver` and its first tick fires within ~1 second — coverage is already complete. An opportunistic trigger would only add a code path racing with the hosted process.

### Single-Writer Serialization via Dirty Flag

All state-file writes flow through the hosted daemon. Other trigger paths only *signal*. This eliminates write races by construction.

**Mechanism:**

1. tmux fires a structural event.
2. The hook command runs `portal state notify`.
3. `portal state notify` is a small binary: touch (create or bump mtime of) `~/.config/portal/state/save.requested`, exit.
4. The hosted daemon's 1-second ticker checks on each tick:
   - Is `save.requested` present? → capture, then clear the flag.
   - Has it been ≥30 seconds since the last save? → capture, then clear the flag.
   - Neither? → no-op.
5. Capture inspects `@portal-restoring` first; if set, skip the tick entirely (restoration in progress).

**Properties:**

- **Single writer by construction.** Only the hosted daemon writes scrollback files and `sessions.json`. No filesystem coordination beyond the dirty flag.
- **Natural coalescing.** Five events firing in 100ms all just set the flag; the next tick does exactly one save.
- **Max-gap guarantee.** 30 seconds is the ceiling on save staleness, even during idle periods.
- **Event latency.** ≤1 second from tmux event to save completion (bounded by the ticker interval).
- **Restoration guard.** The daemon's tick checks `@portal-restoring` at the top of the cycle. While set (during the skeleton-restore window), no capture runs, regardless of dirty-flag state.

### Daemon Tick Loop (Pseudocode)

```
for {
    select {
    case <-ticker.C:  // 1 second
        if isRestoringFlagSet() {
            continue  // skip entire tick during restore
        }
        if isDirty() || timeSinceLastSave() >= 30*time.Second {
            captureAndWrite()
            clearDirty()
        }
    case <-ctx.Done():  // SIGHUP or SIGTERM
        if !isRestoringFlagSet() {
            captureAndWrite()  // flush final state on shutdown
        }
        return
    }
}
```

### Defensive Dirty-Flag Clear on Daemon Startup

On daemon startup, the first action is to clear `save.requested` if present. This prevents a stale dirty flag from a prior (crashed or version-mismatch-restarted) daemon from triggering an immediate save of a mid-restore state.

Correctness does not depend on this — even without it, the eventual capture converges to the correct state once restoration completes and `@portal-restoring` is cleared. The clear is a belt-and-braces cleanup that avoids a redundant capture during the restore window.

### Tick Cadence Rationale

The 1-second ticker is a **poll cadence**, not a save cadence. Per-tick idle cost is a filesystem stat of `save.requested` and a `time.Since` comparison — measured in microseconds. Heavy work (capture-pane, hashing, writes) only fires when the dirty flag is set or the 30-second max-gap elapses.

This polling approach is not load-bearing. It could be swapped for `fsnotify`-style filesystem watching for sub-10ms responsiveness, at the cost of cross-platform watcher complexity. Current polling is simpler and good enough for the responsiveness target.

### Crash Safety

- **Mid-capture crash:** in-memory state discarded. On-disk `sessions.json` still points to the previous save's scrollback files. Previous save remains fully valid.
- **Mid-write crash:** `AtomicWrite` (temp file + rename) guarantees either the old or new file is intact. No partial-file states exist on disk.
- **Orphan scrollback files** from an interrupted save are cleaned up by the GC step on the next successful save (see Save Format & Schema section).

## Save Format & Schema

### Storage Location

Saved state lives at `~/.config/portal/state/`, resolved via Portal's existing `configFilePath` mechanism (per-file env var → `XDG_CONFIG_HOME/portal/` → `~/.config/portal/`). Same location as other Portal config (`hooks.json`, `projects.json`, `aliases`) — no separate XDG state directory.

All files written with mode `0600` (owner read/write only). Matches the trust model of shell history and debug logs already on the user's filesystem. No encryption at rest.

### Directory Layout

```
~/.config/portal/state/
├── sessions.json                        # structural index (the atomic commit)
├── save.requested                       # dirty flag (touched by portal state notify)
├── daemon.version                       # daemon binary version marker
├── portal.log                           # current log file
├── portal.log.old                       # previous rotated log
├── hydrate-<paneKey>.fifo               # per-pane hydration FIFOs (transient)
└── scrollback/
    ├── <session>__<window>.<pane>.bin   # raw capture-pane output per pane
    ├── work__0.0.bin
    ├── work__0.1.bin
    └── ...
```

### Scrollback Files

Each live pane has its own scrollback file containing raw `capture-pane -e -p -S -` output (ANSI escape sequences preserved inline, no encoding transformation).

**Filename scheme:** `<session>__<window>.<pane>.bin`

- `session` is the session name, passed through a filesystem-safe sanitizer (replace characters that conflict with filesystem conventions: `/`, null bytes, leading `.`, etc.). On collision (two sanitized session names map to the same file key), append a hash suffix.
- `window` is the numeric window index; `pane` is the numeric pane index.
- `.bin` extension indicates binary (non-textual) content due to embedded ANSI escapes.

### Structural Index: `sessions.json`

Single JSON file at the root of the state directory. Contains the complete structural topology plus references to scrollback files.

**Schema (version 1):**

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

**Field semantics:**

- `version` — integer. Starts at 1. Bumped on schema-breaking changes (loader reads this and applies known-version logic).
- `saved_at` — RFC 3339 UTC timestamp, used by `portal state status` to render "last saved Xs ago."
- `sessions[].name` — exact session name at save time; restored verbatim.
- `sessions[].environment` — key-value map from `show-environment -t <session>`. Written as-is; tmux's `update-environment` refreshes stale values on client attach.
- `sessions[].windows[].layout` — pre-zoom layout string from `#{window_layout}` (not `window_visible_layout`). Correct for `select-layout` replay.
- `sessions[].windows[].zoomed` — boolean from `#{window_zoomed_flag}`. Re-applied separately after layout.
- `sessions[].windows[].active` — boolean from `#{window_active}`.
- `sessions[].windows[].panes[].cwd` — from `#{pane_current_path}`.
- `sessions[].windows[].panes[].active` — from `#{pane_active}`.
- `sessions[].windows[].panes[].current_command` — short command name from `#{pane_current_command}` (no args — that's a tmux limitation). Captured for diagnostic visibility in `portal state status`; not load-bearing for restoration.
- `sessions[].windows[].panes[].scrollback_file` — path relative to the state directory.

**Omitted fields (explicitly not captured):**
- `options` (generic per-session/per-window/per-pane option capture): dropped per Scope.
- `marks`: dropped per Scope.
- `last_pane`: dropped per Scope.

### Atomic Commit Discipline

Multi-file state with per-file atomicity. The commit order:

1. **In-memory capture.** Enumerate live sessions (skipping `_*` names), call `list-panes -a -F ...`, `show-environment -t <session>` per session, and `capture-pane -e -p -S - -t <pane>` per eligible pane. All reads run to completion before any writes.
2. **Per-pane scrollback writes.** For each pane whose scrollback hash changed (see Content-Hash Dedup below), write its `.bin` file via `AtomicWrite` (temp file + rename). Unchanged panes are skipped.
3. **Structural index write.** `sessions.json` written last, via `AtomicWrite`. **This rename is the atomic commit.** If the rename succeeds, all referenced scrollback files are present on disk.

**Failure modes:**
- Crash before step 3 → old `sessions.json` still valid, still references old scrollback files. Restore works as of the previous save.
- Crash mid-step 3 → `AtomicWrite` guarantees either the old or new `sessions.json`, never a partial. Still consistent.
- Orphan new scrollback files from a partial save → cleaned by GC on the next successful save.

### Content-Hash Dedup

To avoid rewriting unchanged scrollback on every tick (which would generate gigabytes per day for heavy-history configurations), the daemon holds an in-memory map `paneKey → hash-of-last-written-scrollback`.

Per pane per capture cycle:
1. Capture scrollback bytes (cheap — tmux internal buffer).
2. Hash the bytes (xxhash or equivalent fast non-cryptographic hash).
3. Compare to the stored hash for this pane.
4. If identical → skip the disk write; no change.
5. If different → `AtomicWrite` the scrollback file, update the stored hash.

`sessions.json` is written at the end of the cycle only if *anything* changed (structural delta or at least one pane's hash differed). If a full 30-second cycle produces zero changes, zero disk activity occurs.

### GC / Orphan Cleanup

After every successful save (after `sessions.json` is atomically committed), run GC synchronously:

1. Read the freshly-written `sessions.json` and collect every `scrollback_file` path it references.
2. List everything under `scrollback/`.
3. Any file present on disk but NOT referenced by the new index → `os.Remove`.

Handles every stale-file scenario:
- Pane closed → file no longer referenced → deleted.
- Session renamed → old-name files deleted, new-name files written.
- Window or pane renumbered → same.
- Orphan files from a previous mid-save crash → cleaned on next successful save.

Idempotent. Runs once per save. Self-healing by construction.

### Retention Policy

**Current state only.** No historical snapshots. Single `sessions.json`, no `.0`/`.1` rotation.

- `AtomicWrite` makes mid-write corruption vanishingly rare (temp + rename).
- Historical snapshots would 5-10× disk use for no restore benefit.
- If real-world corruption surfaces, a `sessions.json.previous` single-slot backup can be added later. YAGNI for v1.

### FIFO Files

Per-pane FIFOs for hydration (`hydrate-<paneKey>.fifo`) live in the state directory during the restoration window. They are created just before pane creation, unlinked by the helper on signal (or timeout), and swept defensively by `os.Remove + syscall.Mkfifo` on each bootstrap. Not part of the save schema; treated as transient coordination artifacts.

## tmux Hook Registration Lifecycle

### Global Hooks Registered by Portal

Portal registers two categories of global tmux hooks:

**Save-trigger events** (fire `portal state notify`):
- `session-created`
- `session-closed`
- `session-renamed`
- `window-linked`
- `window-unlinked`
- `window-layout-changed`
- `pane-focus-out`

**Hydration-trigger events** (fire `portal state signal-hydrate #{session_name}`):
- `client-attached` (initial attach, NULL → session)
- `client-session-changed` (existing client switches sessions)

Both events are registered; every attach path is covered (double-firing on a single attach is harmless because `signal-hydrate` is idempotent via the skeleton-marker check).

### Registration Shape

All hooks use `set-hook -ga` (global, append) so Portal coexists with user `.tmux.conf` hooks and any other plugins registering on the same events. Each hook wraps its command in a `command -v portal` defensive guard so an uninstalled or missing binary produces no error spam.

```
set-hook -ga session-created 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'
set-hook -ga session-closed  'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'
# ... all save-trigger events identically ...
set-hook -ga client-attached         'run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"'
set-hook -ga client-session-changed  'run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"'
```

**Why `-ga` (append) rather than `-g` (replace):** `-g` stomps on any user or other-plugin hook registered on the same event. `-a` preserves coexistence. tmux 3.0+ requirement (array-indexed hooks) follows from this choice.

**Why the `command -v` guard:** if Portal is uninstalled or temporarily absent during a binary swap, the guard short-circuits the invocation — no "command not found" in tmux's error buffer. Runs a shell built-in on every event; cost is microseconds.

### Content-Based Idempotency (per-event, per-command)

`set-hook -ga` does **not** deduplicate. Identical appends accumulate duplicate entries in the hook array. Portal must check whether its entry is already present before appending.

For each (event, expected_command) pair Portal registers:

1. Run `tmux show-hooks -g` and capture stdout.
2. Parse lines matching `^<event>\[(\d+)\]` to find array entries for this event.
3. Look for our expected-command substring (`portal state notify` for save-trigger events, `portal state signal-hydrate` for hydration-trigger events) within those entries.
4. If any matching entry is found → skip registration for this event.
5. If none match → `set-hook -ga <event> '<full command>'` to append.

**Per-event-per-command scoping:** the check for `portal state notify` is only applied to save-trigger events; the check for `portal state signal-hydrate` is only applied to hydration-trigger events. The command substrings are distinct, so there is no cross-contamination across the two categories.

Parsing lives in Go using the existing `Commander` interface. Compiled regex per event; table-driven tests with canned `show-hooks` outputs.

**Quoting note:** tmux may render the stored command with different outer quoting than Portal supplied. The match substring (`portal state notify` or `portal state signal-hydrate`) is raw text inside the command and is not affected by tmux's outer quoting.

### Scenario-by-Scenario Behavior

#### Scenario 1: Fresh-server bootstrap

User runs `portal open` on a machine where tmux is not running.

1. `EnsureServer()` starts the tmux server.
2. Portal runs hook-registration idempotency check for every (event, command) pair. On fresh server, `show-hooks -g` returns no Portal entries → all hooks appended.
3. `has-session -t _portal-saver` → false → `new-session -d -s _portal-saver "portal state daemon"`.
4. Portal unconditionally runs `set-option -t _portal-saver destroy-unattached off`.

#### Scenario 2: Subsequent invocation on bootstrapped server

The same code path as Scenario 1. All steps are idempotent:

- Hook check finds existing entries → appends skipped.
- `has-session` returns true → session creation skipped.
- `destroy-unattached off` is a no-op when already set.

Per-bootstrap cost: ~8ms of idempotent tmux calls.

#### Scenario 3: Portal upgrade with running server

User runs `brew upgrade portal` (or equivalent). New binary on disk. tmux server still running. `_portal-saver` still hosting a daemon `exec`'d from the old binary.

**Version-marker-based restart:**

1. On daemon startup, `portal state daemon` writes its version (`cmd.version`) to `~/.config/portal/state/daemon.version`.
2. On every `EnsureServer()` call, Portal reads `daemon.version` and compares to the currently-invoking binary's `cmd.version`.
3. If they differ → `kill-session -t _portal-saver`, then recreate with the new binary. New daemon overwrites the version file on startup.
4. If the version file is absent (first-ever bootstrap) → treat as mismatch; recreate.

**Dev-build handling:**
- If `cmd.version` is empty or literally `"dev"`, treat every bootstrap as a mismatch and restart the daemon. Covers the common workflow of rebuilding Portal during development and expecting each rebuild's daemon code to take effect.
- Otherwise (release build, real semver), raw-string comparison determines mismatch.

**Data safety during restart:** tmux `kill-session -t _portal-saver` closes the PTY; kernel delivers SIGHUP to the daemon; signal handler flushes the final save via `AtomicWrite` before exit. New daemon takes over on recreation. Worst-case data loss: whatever accrued since the last dirty-flag check (≤1 second of scrollback drift).

Cost on unchanged version: one file read per `portal open` (microseconds). On upgrade: one `kill-session` + `new-session` (~50ms). Invisible to anyone not watching.

#### Scenario 4: Portal uninstall with running server

User removes the Portal binary. tmux server still running.

- **Defensive hook guard** (`command -v portal`) short-circuits every hook-fired invocation. No error spam in tmux's error buffer.
- **Dead daemon** survives in memory until it exits or the server restarts. Once gone, it cannot be recreated (binary absent). This is silent — Portal is gone, there is nothing to do.
- **`portal state cleanup`** (see CLI Surface) is the optional explicit teardown. Not required for correctness — defensive hooks and self-healing handle implicit teardown.
- **User data** (`~/.config/portal/state/`, `hooks.json`, `projects.json`, `aliases`) is left on disk. Standard Unix convention: uninstalling the tool does not destroy user data. Reinstalling picks up where the user left off.

#### Scenario 5: Portal binary replaced (brew upgrade) — composed from 3 + 4

No new rules. The atomic swap window is covered by Scenario 4's defensive guard (hooks firing during the brief swap short-circuit cleanly). After the swap, Scenario 3's version-marker detection triggers daemon restart on the next bootstrap. Install-path migrations (e.g., Intel → Apple Silicon Homebrew) are covered identically — hooks reference `portal` on `$PATH`.

#### Scenario 6: User restarts tmux server

Server dies via `kill-server`, `killall tmux`, reboot, crash, etc.

**Server-level state** (hooks, `_portal-saver`, user sessions) — all gone.
**On-disk state** (`sessions.json`, scrollback, `daemon.version`, `save.requested`) — preserved.

No new rules. Scenario 1/2 bootstrap handles end-to-end:
- Hooks re-registered fresh (append to empty arrays).
- `_portal-saver` absent → recreated.
- Version check either matches (same binary) or mismatches (daemon restarted either way since it's newly spawned).
- Restoration of user sessions is orthogonal — handled by the Restore flow.

**Defensive action on daemon startup:** clear `save.requested` if present (see Triggers & Serialization section).

#### Scenario 7: Hook collision with other plugins

Covered by the `-ga` append semantics and content-based idempotency above. Research verified:
- Major TPM plugins (continuum, resurrect, sessionist, logging, yank) do not use `set-hook` at all. Real collision risk is with user-`.tmux.conf` hooks, not other plugins.
- tmux 3.0+ stores hooks as array-indexed options; per-index removal works cleanly.
- Sparse arrays fire correctly (removed indices do not break surviving entries).

### Removal (uninstall / `portal state cleanup`)

1. Run `tmux show-hooks -g`.
2. For each (event, expected_command) pair Portal registers: parse for Portal's indices.
3. Remove each match via `set-hook -gu '<EVENT>[N]'`, in **reverse index order** (defensive — tmux does not renumber after removal, but reverse order is cheap insurance against any edge case).
4. Entries matching other commands on the same events are left alone.

### False Paths Documented

- **Plain `set-hook -g` (replace).** Stomps on user and other-plugin hooks on the same events. Rejected.
- **Assumption that `set-hook -a` is idempotent if the command matches.** Empirically disproven; identical appends accumulate. Content-based check is required.
- **Marker-comment in the hook command (e.g. `# portal-resurrect`).** Unnecessary. The `portal state notify` / `portal state signal-hydrate` command substrings are already unique identifiers.

## Restore-Side Architecture

### Restoration Trigger

Restoration runs on every `portal open` invocation (or any other Portal command that reaches `PersistentPreRunE`). No `serverStarted` gate, no explicit user command, no last-save staleness check.

**For each entry in `sessions.json`:**
- If a live tmux session already exists with that name → **skip**. User's current reality is authoritative; Portal never clobbers live sessions.
- If no live session with that name → **skeleton-restore** it (structure only; scrollback lazy).

**Steady-state cost** (all saved sessions already live): ~20ms — one JSON read + one `list-sessions` call + diff → no-op. Invisible.

Portal's `{project}-{nanoid}` naming makes collisions between Portal-created sessions practically impossible; the skip-on-live behavior addresses the only real risk, which is a user's manually-created tmux session happening to share a name with a saved Portal session.

### Skeleton-Eager + Scrollback-Lazy

**Skeleton (structure) is restored eagerly during bootstrap.** For each missing saved session:
- `new-session -d -s <name> -c <root_cwd> "<hydrate command for first pane>"`
- `new-window`, `split-window` for remaining windows/panes to match saved structure. Each pane's command is `portal state hydrate --fifo <F> --file <S>` (see Scrollback Restore Mechanics).
- `select-layout "<saved>"` per window
- `select-pane -t <active>` per window
- `resize-pane -Z` on the active pane if `zoomed` was true
- `@portal-skeleton-<paneKey>` marker set on each created pane

Cost: ~600ms for a heavy 10-session configuration. Covered by the loading page (see Bootstrap Flow).

**Scrollback (content) is NOT injected during bootstrap.** Scrollback files on disk are left intact. Injection happens at *attach time*, triggered by tmux's `client-attached` / `client-session-changed` hooks.

### Why Skeleton-Eager

Preserves tmux self-containment. After bootstrap:
- Native tmux commands (`tmux attach -t NAME`, `tmux switch-client -t NAME`, `tmux list-sessions`) see the real structure.
- Third-party tmux plugins see live sessions, not a placeholder state.
- Shell aliases and keybindings that reference specific session names work identically to before a reboot.

The ~500ms extra cost (versus a sessions-only-eager approach) buys Portal being *additive* rather than *invasive*.

### Why Scrollback-Lazy

Fully-eager scrollback injection at realistic power-user sizes (`history-limit 50000` per pane × 30 panes) would add ~15 seconds to boot — unacceptable UX. Lazy hydration amortizes cost across attaches; sessions the user never touches today cost zero to hydrate.

Mechanism details are in the Scrollback Restore Mechanics section.

### Why Hook-Driven Hydration

Both `client-attached` and `client-session-changed` are registered globally. This covers every attach path:

- `portal open` picker attach
- `portal attach NAME` (from bare shell or inside tmux)
- Direct `tmux attach -t NAME` from a bare shell
- `tmux switch-client -t NAME` from inside tmux

All run the same `portal state signal-hydrate #{session_name}` command (session name passed as argv via tmux format expansion). The subprocess enumerates panes in the specified session and signals the FIFO of any pane still carrying the `@portal-skeleton-<key>` marker. Idempotent for already-hydrated panes.

Users do not need to know "hooks only work if you attach via Portal."

### Marker Coordination

Two volatile tmux server-option markers coordinate restoration and saving. Both are **volatile** (server-option scope): they clear automatically on server restart, so stale markers cannot persist across tmux lifetimes.

#### `@portal-skeleton-<paneKey>` — "awaiting hydration"

- **Set by:** skeleton-restore, on each pane it creates. Key is the structural position `session:window.pane`.
- **Semantic:** "this pane was skeleton-restored; its saved scrollback file on disk holds pre-boot state that must not be overwritten until the pane has been hydrated."
- **Effect on save:** the daemon's capture loop **skips** panes whose marker is set. Neither the scrollback file nor the pane's `sessions.json` entry is updated. Disk file preserved.
- **Cleared by:** the hydrate helper, *after* successful content dump + 100ms settle sleep (see Scrollback Restore Mechanics). "Marker cleared" is synonymous with "helper output is complete and the pane's scrollback is in its final form."
- **User-created panes never receive this marker.** Brand-new post-boot panes are captured normally from the start.
- **Inverse semantic is deliberate:** "needs hydration" is *active state set by restore*; default absence means "safe to capture." This keeps the new-session creation path from requiring a special code branch.

#### `@portal-restoring` — "restoration in progress"

- **Set by:** bootstrap, at the start of the skeleton-restore phase.
- **Unset by:** bootstrap, after skeleton-restore completes.
- **Semantic:** "bootstrap is mid-skeleton-build; save captures would see half-built state."
- **Effect on save:** both `portal state notify` (no-op if set; does not even touch the dirty flag) and the hosted daemon's tick loop (skip the entire tick if set) honor this marker. Restore can fire a cascade of structural events (`session-created`, `window-linked`, `window-layout-changed`) without triggering partial-state saves.
- **Effect on daemon shutdown handler:** SIGHUP/SIGTERM handler skips the final flush if `@portal-restoring` is set (see Save-Side Execution Model). Prevents capturing mid-transition state during upgrade-triggered restart.

### Failure-Mode Behavior for Hydration

If scrollback injection fails (file missing, disk read error, helper timeout without signal):

1. **Unset the marker anyway.** The pane remains empty; the save loop resumes normal capture. Degraded, not stuck.
2. **Log a warning** to `portal.log` (`file not found`, `FIFO timeout`, `I/O error`, etc.). Failure is observable but not spam.
3. **Do not retry automatically** on the failure path itself. A missing file is likely permanent; the pane's future save captures whatever is on screen.

For the FIFO-timeout path specifically, the helper takes a different course that allows retry on the *next* attach (see Scrollback Restore Mechanics) — but within a single hydration attempt, failure is terminal for that attempt.

### User-Created Sessions Mid-Restore

No special handling required. Skeleton-restore only operates on saved sessions listed in `sessions.json`. Pre-existing live sessions — including sessions the user just created — are neither touched nor captured while `@portal-restoring` is set. User commands are not gated.

### Direct `tmux attach` Path

Universal coverage via `client-attached` / `client-session-changed` registration. A user who attaches directly with `tmux attach -t NAME`:

1. `client-attached` fires.
2. The hook runs `portal state signal-hydrate NAME`.
3. Portal enumerates panes in session NAME, signals any skeleton-marked pane's FIFO.
4. Helpers unblock, dump scrollback, exec hook-or-shell.

Functionally identical to `portal open` picker attach.

### Rejected Alternatives

- **Background prefetch hydration after bootstrap.** Race conditions (user attaches mid-fill), more code paths, no win over pure-lazy with marker coordination.
- **Sessions-only-eager (create sessions, skip windows/panes).** Would leave direct `tmux attach` seeing empty sessions. Breaks tmux self-containment.
- **Fully-eager scrollback injection.** 2-15s added boot delay at realistic scales. Rejected; can be revisited if attach-time latency surfaces as a user complaint.
- **Restore gated on `serverStarted` AND "no sessions exist."** No threat model justified the defensive gate. Simplified to "restore all, skip live by name."

## Scrollback Restore Mechanics

### Injection Path: Blocking Helper Pre-Shell via FIFO

Each skeleton-restored pane is created with a shell-pipeline command as its initial process:

```
sh -c 'portal state hydrate --fifo <FIFO> --file <SCROLLBACK>; exec $SHELL'
```

`portal state hydrate` is a Go subcommand that runs *inside the pane, before the shell*. Its stdout is connected directly to the pane's PTY slave. Bytes written to its stdout flow out through the PTY → tmux's VT parser → rendered into scrollback natively with full ANSI fidelity.

When the helper exits (normally or via timeout), the trailing `exec $SHELL` takes over the same process. The shell replaces the helper without spawning a new process. The shell never sees the helper's command line; no history pollution.

### Why This Mechanism Was Chosen

**All tmux-native input commands (`paste-buffer`, `send-keys`, `pipe-pane -I`) write to the PTY master bufferevent, which is the shell's *stdin*.** ESC bytes arriving as stdin get interpreted by readline as meta-key prefixes, not rendered as ANSI color sequences. The paste-buffer path is fundamentally wrong for scrollback injection.

Only two mechanisms deliver bytes to the pane's **output (display) path**:
1. A process inside the pane writing to its own stdout. Bytes flow through the PTY slave → tmux's VT parser → rendered correctly.
2. External process writing directly to the pane's slave PTY device (`/dev/pts/<N>` via `#{pane_tty}`). Faster but has positioning race issues (the shell has already prompted by the time the external writer arrives).

Option 1 (helper pre-shell) avoids the positioning race entirely: the shell does not exist yet when the bytes are written.

### Signal Mechanism: FIFO Per Pane

**FIFO path:** `~/.config/portal/state/hydrate-<paneKey>.fifo` (paneKey = `<session>__<window>.<pane>` — same sanitization as scrollback filenames).

**Creation (bootstrap, before creating the pane):**
1. `os.Remove(path)` — ignore `ENOENT`; defensive sweep of any stale FIFO from a prior crashed bootstrap or dead helper.
2. `syscall.Mkfifo(path, 0600)` — create the FIFO with owner-only permissions.

This defensive pattern eliminates the need for a separate stale-FIFO sweep step. Stale FIFOs only exist when no live helper holds them (helpers die with the tmux server, same lifetime as the FIFOs they block on).

**Signal (attach time):**
The `client-attached` / `client-session-changed` hook runs `portal state signal-hydrate <session-name>`, which:
1. Enumerates panes in the attached session (`list-panes -t <session-name>`).
2. For each pane whose `@portal-skeleton-<key>` marker is set: open the pane's FIFO for writing and write a single byte.
3. For each pane whose marker is absent: no-op (already hydrated or never skeleton-restored).

`signal-hydrate` **does not touch the marker**. The helper owns marker-unset timing to close the capture-mid-dump race (see below).

### Helper Behavior on Startup

```
portal state hydrate --fifo F --file S:
  1. Open FIFO for reading, block with 3-second timeout.
  2. On signal arrival:
     a. Close + os.Remove the FIFO.
     b. Emit reset preamble to stdout:   \033[?25h\033[?1049l\033[0m
        (cursor visible + exit alt-screen defensively + SGR reset).
     c. Copy the scrollback file's bytes to stdout.
     d. Emit reset postamble + CRLF:      \033[?25h\033[?1049l\033[0m\r\n
     e. time.Sleep(100 * time.Millisecond).
     f. Read hooks.json; look up this pane's resume hook by structural key.
     g. tmux set-option -s @portal-skeleton-<paneKey> ""  (unset marker).
     h. If hook exists: exec sh -c 'HOOK; exec $SHELL'.
        Else:           exec $SHELL.
  3. On 3-second timeout (no signal arrived):
     a. Emit reset preamble only (no content dump).
     b. Skip the 100ms sleep.
     c. Do NOT unset the skeleton marker — next attach will re-signal and retry.
     d. Log a warning to portal.log.
     e. exec $SHELL (bare shell; no hook firing on this path).
  4. On scrollback file missing / unreadable:
     a. Emit reset preamble only.
     b. Log a warning.
     c. Marker was already cleared by the signal path's step g — skip (empty pane).
     d. Continue to hook/shell exec.
```

Reset sequences are short strings (~20 bytes total preamble + postamble). Overhead is imperceptible against the 500–1500ms dump they bracket.

### Timeout: 3 Seconds

- Normal signal latency: ~10–50ms.
- Slow-but-legit upper bound (NFS home, heavy system load, slow hook script): ~1–2s.
- 3s ≈ 2× the slow-legit tail. Fast enough to degrade snappily on real failures without cutting off rare slow-legit cases.

### The 100ms Settle Sleep (Why the Helper Owns Marker-Unset)

The helper's `write()` to stdout returning does **not** mean tmux has finished parsing the written bytes. tmux runs the VT parser in a separate process with its own event loop; there is an asynchronous lag between bytes arriving at the PTY slave and those bytes being added to the pane's scrollback ring.

If `signal-hydrate` unset the marker immediately (right after writing the FIFO byte), the daemon's next tick could run while the helper is still mid-dump. `capture-pane` would return partial scrollback; content-hash dedup would compute a hash based on the partial state and overwrite the full saved scrollback file with a truncated version.

Transferring marker-unset ownership to the helper makes "marker cleared" synonymous with "helper's output is complete and the pane's scrollback is in its final form." The 100ms sleep before the unset is a safe margin against tmux's PTY-parser lag (typical lag is ~1ms; 100ms is generous without being user-perceptible against the 500–1500ms dump).

### Marker Lifecycle Summary

- **Skeleton-restore sets** `@portal-skeleton-<paneKey>` when creating each pane.
- **Save loop skips** any pane with the marker set.
- **`signal-hydrate` writes to FIFO** but does not touch the marker.
- **Helper unsets marker** after dump + 100ms sleep, or immediately if the scrollback file was missing (empty pane path).
- **Helper does NOT unset marker** on FIFO timeout — next attach re-signals, retry happens naturally.
- **Save loop resumes capturing** the pane on the next tick once the marker is cleared.

### Failure Modes (Mechanism-Level)

- **Scrollback file missing on helper startup.** Helper logs a warning, emits reset preamble only, skips content dump, unsets marker, exec's hook/shell. Empty pane, not stuck.
- **FIFO pre-opened but hook handler crashed before writing.** Helper blocks until 3-second timeout, degrades to empty shell, logs a warning. Marker stays set — next attach retries.
- **Helper crashes mid-dump.** Pane ends up with partial content + dead process. Shell never starts. User sees a stuck pane. Recovery: user kills the pane manually; next bootstrap re-skeletons the structure (scrollback file may be mid-written or corrupt, so some bytes may be missing — truncation, not corruption). Documented as a "shouldn't happen" case.
- **Signal fires twice somehow** (e.g., both `client-attached` and `client-session-changed` fire for the same logical attach). Second write to the FIFO goes nowhere (the helper has already closed and unlinked the FIFO). Harmless.

### Implementation Notes

- FIFOs are POSIX primitives; `syscall.Mkfifo` is the Go entry point. Supported on Linux and macOS, consistent with Portal's existing platform targets.
- The `; exec $SHELL` chain is a shell construct; the pane's command must be invoked as `sh -c '...'` so the shell parses the `;` correctly.
- The helper's blocking FIFO read must implement a timeout. Go's standard `os.File.Read` on a FIFO does not time out on its own. Use a goroutine + `time.After` channel + `select` (or equivalent I/O-with-deadline pattern).

### Validation Reference

The mechanism was empirically validated on an isolated tmux socket during discussion:
- `cat FILE; exec bash` pattern: 1000-line ANSI scrollback rendered correctly; clean `bash-5.3$` prompt at end.
- Shell history contained only post-test commands — no helper, no `cat`, no scrollback content.
- Blocking-FIFO variant: pane empty before signal; after `echo "go" > fifo`, scrollback rendered and shell prompt appeared.

### Rejected Alternatives

- **`paste-buffer` / `send-keys` / `pipe-pane -I`.** Broken. Target the shell's stdin, not the pane's display. Confirmed via tmux source review.
- **Direct `/dev/pts/<N>` write.** Viable but has a positioning race (shell has already prompted). Mitigation via `\033[2J\033[H` + SIGWINCH redraw is feasible but more complex. Rejected in favor of helper pre-shell.
- **Fully-eager at skeleton restore time.** Would eliminate attach-time latency entirely at the cost of 2–15s boot delay. Rejected in favor of pure-lazy.
- **Zellij-style confirmation prompt before scrollback injection.** Portal's resume hooks are already explicit opt-in via `portal hooks set`; scrollback injection is just replaying the user's own history into their own pane. No second-stage confirmation needed.

## Resume Hook Firing

### Firing Point: Inside the Helper's Exec Chain

Resume hooks fire **only** from inside the hydrate helper's exec chain, at the end of successful hydration. There is no attach-time hook firing, no `send-keys` involvement, and no shell-readiness polling.

After the helper has dumped scrollback, slept 100ms, and unset its skeleton marker (see Scrollback Restore Mechanics), the helper:

1. Reads `hooks.json`.
2. Looks up the resume hook for this pane's structural key (`session:window.pane`).
3. If a hook exists → `exec sh -c 'HOOK; exec $SHELL'`. When the hook command exits (or immediately, if it's a background process), `exec $SHELL` takes over the same process.
4. If no hook exists → `exec $SHELL` directly.

### Why Firing Belongs Only in the Helper

**Hooks are for reboot recovery.** A hook "firing" means "re-launch this process when the pane comes back from the dead." The only moments when a pane comes back from the dead are:

1. After a server restart — skeleton restore creates the pane fresh with the hydrate helper chain.

That is the complete list. Within a single server lifetime, a pane that still exists does not need its hook re-fired — the hook's process either still exists or was explicitly killed by the user. Firing `claude --resume <uuid>` on a detach/reattach of a pane that already has Claude running would actively break things.

### What Is Deleted from the Previous Design

- **`ExecuteHooks` function.** Deleted. No more attach-time hook execution.
- **Call sites of `ExecuteHooks` in `cmd/open.go` and `cmd/attach.go`.** Deleted.
- **`internal/hooks/executor.go`.** Deleted.
- **`cmd/hook_executor.go`.** Deleted.
- **`@portal-active-<pane>` volatile marker** set during `portal hooks set` as a one-shot-per-server-lifetime gate. Deleted. The registration path (`portal hooks set`) becomes a pure write to `hooks.json` with no tmux-side marker management.
- **All attach-time hook checking.** Deleted.
- **Shell-readiness polling** for `send-keys` delivery. Eliminated — nothing uses `send-keys` for hook firing any more.

### What Stays Unchanged

- **`hooks.json` storage** and its on-disk schema.
- **User-facing CLI:** `portal hooks set --on-resume "<cmd>"`, `portal hooks list`, `portal hooks rm --on-resume`. Surface unchanged; internals simplified.
- **Structural-key scheme** (`session:window.pane`) for identifying hooks.
- **`CleanStale`** for pruning orphaned hook entries (with the guard change described in "CleanStale Behavior").

### Behavior Change: No "Live Attach" Firing

The only scenario where new behavior differs from old:

- **Old:** a hook registered on a pane that has not yet gone through a save/restore cycle; user detaches and reattaches within the same server lifetime. Old design fired the hook via `send-keys` once per server lifetime (one-shot via the `@portal-active-<pane>` marker).
- **New:** the hook does not fire until the next server restart triggers skeleton restoration.

This is correct behavior by design. The old "live attach firing" was the exact misbehavior the `@portal-active-<pane>` marker existed to mitigate — firing `claude --resume` on a pane that already has Claude running breaks things. The marker-at-registration approach worked only because it happened to be a one-shot-per-server-lifetime gate; it was never the right model.

Under the new design, hooks fire exactly when a pane is freshly recreated from saved state. This matches the user's mental model of "hooks are for reboot recovery" and aligns with the semantic of the hook system being a reboot-recovery mechanism.

### `run-shell` Blocking Note

`signal-hydrate` (fired from `client-attached` / `client-session-changed` hooks) does non-trivial work: enumerate panes in the attached session, check markers, write to up to N FIFOs. For a 30-pane session, cost is ~50–150ms.

tmux's `run-shell` is synchronous by default and blocks the server during hook execution. Acceptable for initial release — the user is actively attaching; sub-150ms is imperceptible at the moment of attach. If real-world use reveals problems (other clients feeling laggy during heavy attaches), switch to `run-shell -b` (async). tmux 3.0+ has settled `-b` behavior; defer the switch until there is evidence the blocking matters.

### Net Simplification

A whole execution path is removed: attach-time hook firing + shell-readiness workaround + registration-side marker. Hook firing is folded into the hydrate helper's exec chain — an `exec` replacement that was going to exist for scrollback injection anyway. The hook fires exactly once, exactly at the right moment, with no `send-keys`, no polling, no racing.

## Layout Restoration

### Per-Window Restoration Order

For each window being restored:

1. **Create the window** (`new-window` — the first pane is created implicitly).
2. **Create remaining panes** via `split-window` to reach the saved pane count. Direction arguments are arbitrary — the next step rearranges.
3. **`select-layout "<saved layout string>"`** — tmux parses the string and fits the panes to the saved geometry.
4. **`select-pane -t <saved active pane index>`** — set the active pane within the window.
5. **`resize-pane -Z`** on the active pane if `window_zoomed_flag` was true at save time.

Zoom **must come after** layout. `resize-pane -Z` operates on the current layout geometry, and applying it before `select-layout` would produce incorrect results. This ordering matches tmux-resurrect's proven approach.

### Layout String Source

Portal captures `#{window_layout}` (pre-zoom form), not `#{window_visible_layout}`. The pre-zoom form is the correct input for `select-layout` replay. Storing the zoomed form would cause `select-layout` to collapse panes incorrectly on re-application.

Zoom state is captured separately as a boolean (`zoomed` field in `sessions.json`) and re-applied after layout, via `resize-pane -Z`.

### Pane-Count Mismatch / `select-layout` Failure

`select-layout` requires the current pane count to match the saved layout string. Portal's restoration creates exactly the right count from `sessions.json` before applying the layout, so under normal conditions there is no mismatch.

**If `select-layout` returns an error** (corrupt layout string, pane-count mismatch from partial restore, unexpected tmux behavior):

1. Log a warning to `portal.log` identifying the session, window, and the mismatch.
2. Fall back to `select-layout tiled` — tmux's built-in auto-balanced tiled layout. Panes are visible in a sane default arrangement.
3. Continue restoring the remaining windows and sessions. One broken layout does not block other restorations.

Degraded but not stuck. Consistent with the broader "degrade locally, log, continue" principle.

### Terminal Size Drift

Layout strings encode absolute pane dimensions. If a session was saved on a 200×50 terminal and restored into an 80×24 terminal, `select-layout` does best-effort fitting — proportions shift, some panes may be cramped.

Neither Portal nor any other tool solves this at the tmux level; it is a fundamental tmux constraint. Portal does no special handling for this case:

- `select-layout "<saved>"` is always applied as-is.
- tmux fits as best it can.
- If the user cares, they can resize their terminal to match the save-time dimensions.
- `select-layout -E` ("spread evenly") is not used as a fallback — it loses the saved proportions, trading faithful-but-cramped for even-but-different. Default to faithful.

### Zoom State

- Captured at save time as `window_zoomed_flag` (boolean).
- Stored in `sessions.json` as `zoomed: true|false` per window.
- Re-applied via `resize-pane -Z` on the active pane after `select-layout`, if true. `resize-pane -Z` is a toggle; applying it when already zoomed would un-zoom. The restore flow assumes zoom is off immediately after layout application (the saved layout string is pre-zoom), so an unconditional `-Z` when `zoomed` is true produces the correct state.

### Summary of Order

```
new-window <name> -c <cwd>      # window + first pane
split-window × (N-1)            # remaining panes
select-layout "<saved>"         # apply geometry
select-pane -t <active>         # set active pane
if zoomed: resize-pane -Z       # re-apply zoom
```

Mechanical and low-risk. No special logic beyond the fallback on `select-layout` failure.

## Bootstrap Flow (Integrated)

### `PersistentPreRunE` Sequence

Every Portal command that needs tmux (all commands except `version`, `init`, `help`, `alias`, `clean`) runs this sequence, in this order:

**1. `EnsureServer()`** — start tmux server if not running. Set `serverStarted=true` in `cmd.Context()` if Portal started it.

**2. Register global hooks idempotently** (`set-hook -ga` with content-based check):
- Save-trigger events (`session-created`, `session-closed`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`) → `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`.
- Hydration-trigger events (`client-attached`, `client-session-changed`) → `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`.
- For each (event, expected-command) pair, parse `show-hooks -g` and append only if the Portal entry for that event is absent.

**3. Set `@portal-restoring 1`** as a server-level option. Must happen *before* `_portal-saver` is created, because creating `_portal-saver` fires `session-created` which triggers the dirty-flag notify path; `@portal-restoring` ensures that notify is a no-op while bootstrap is still running.

**4. `_portal-saver` session setup** (idempotent):
- `has-session -t _portal-saver` — if present, skip creation.
- If absent: `new-session -d -s _portal-saver "portal state daemon"`.
- Read `~/.config/portal/state/daemon.version`; compare to `cmd.version`:
  - If `cmd.version` is empty or `"dev"` → always restart (kill + recreate) on bootstrap.
  - If version file is absent → treat as mismatch (first-ever bootstrap).
  - If stored version differs from `cmd.version` → `kill-session -t _portal-saver`, then recreate with the new binary.
  - Else → leave running.
- Always run `set-option -t _portal-saver destroy-unattached off` (defensive, idempotent).

**5. `Restore()` — skeleton-only restoration.** TUI path wraps this step (and steps 6-7) with the loading page (1.2s minimum display). CLI path runs silently.

- If `~/.config/portal/state/sessions.json` does not exist → no-op; continue to step 6.
- If `sessions.json` is unparseable (corrupt JSON) → log warning, print one-line stderr warning, skip restoration entirely; continue to step 6.
- Otherwise, parse `sessions.json`. For each saved session:
  - `has-session -t <name>` → if live, skip this session.
  - Else, skeleton-create it:
    1. For each pane: compute FIFO path (`~/.config/portal/state/hydrate-<paneKey>.fifo`); `os.Remove(path)` (ignore `ENOENT`); `syscall.Mkfifo(path, 0600)`.
    2. `new-session -d -s <name> -c <root_cwd> "sh -c 'portal state hydrate --fifo <F> --file <scrollback>; exec $SHELL'"` for the first pane.
    3. `new-window` / `split-window` for remaining windows and panes, each created with its own `hydrate` command as the pane's initial process.
    4. Per window: `select-layout "<saved>"`, `select-pane -t <active>`, `resize-pane -Z` if `zoomed`.
    5. For each created pane: `set-option @portal-skeleton-<paneKey> 1` (at the server-option level for volatility).
- On `select-layout` failure for a window: fall back to `select-layout tiled`, log, continue.
- On any per-session error: log, skip that session, continue with the next.

**6. Unset `@portal-restoring`.** Save loop resumes normal operation on its next tick. Daemon will capture new user state and skip skeleton-marked panes.

**7. `CleanStale()`** — prune stale entries from `hooks.json` whose structural keys do not match any currently-live pane. Runs unconditionally (no empty-panes guard — see CleanStale Behavior section). Runs here because by this point live panes include both pre-existing and skeleton-restored ones.

**8. Return to the calling command.** TUI: loading page is dismissed once the 1.2s minimum has elapsed AND restoration completed. CLI: returns immediately after step 7.

### Ordering Rationale

The critical ordering — `@portal-restoring` is set in step 3 **before** `_portal-saver` is created in step 4 — exists because step 4 fires `session-created`, which the hook pipeline would otherwise use to dirty the flag. Without `@portal-restoring` set first, the daemon's first tick could attempt to capture while the restoration is still building structure.

With the ordering above:
- `@portal-restoring` set → daemon's first tick no-ops.
- Restoration runs → more structural events fire → every one is a no-op on the notify side because `@portal-restoring` is still set.
- `@portal-restoring` cleared → next daemon tick captures the now-complete post-restoration state.

`@portal-skeleton-<paneKey>` markers are set as each pane is created, so even after `@portal-restoring` clears, the daemon correctly skips unhydrated panes until their helpers complete.

### Attach Flow (After Bootstrap)

When the user selects a session via the picker, runs `portal attach NAME`, or issues any other attach command:

1. Portal's open/attach code resolves the target session (alias, path, direct name, or TUI selection).
2. `tmux switch-client -t <session>` if Portal is running inside tmux; else `exec tmux attach-session -A -t <session>` (handing off the process via `syscall.Exec`).
3. tmux fires `client-attached` (bare-shell attach) or `client-session-changed` (inside-tmux switch).
4. The registered hook runs `portal state signal-hydrate <session-name>` as a `run-shell` subprocess. `<session-name>` comes from `#{session_name}` format expansion, passed as argv.
5. Subprocess work:
   - `list-panes -t <session-name>` → enumerate panes in the attached session.
   - For each pane with `@portal-skeleton-<paneKey>` set: write a byte to the pane's FIFO. Do **not** touch the marker.
   - For each pane without the marker: no-op (already hydrated or never skeleton-restored).
6. Per-pane helpers unblock, dump scrollback, sleep 100ms, unset own markers, exec hook-or-shell.
7. Daemon's next tick (sub-second away) captures now-hydrated panes normally. Content-hash dedup skips rewrites unless scrollback has legitimately changed.
8. User is in the session. Scrollback is rendered. Hook (if any) is running; shell takes over when hook exits.

### Return-to-Caller Timing

- **TUI path:** bootstrap runs. Loading page shows for minimum 1.2s (padded if restoration was faster; natural if slower). TUI appears with populated picker. User selects → attach flow runs.
- **CLI path** (e.g., `portal attach NAME`, `portal hooks set ...`): bootstrap runs silently; command-specific logic runs next. For `portal attach NAME` where the target was in `sessions.json`, skeleton was restored before the attach logic runs, so `has-session -t NAME` returns true by the time the attach needs it.

### Loading-Page Minimum Display (TUI Only)

Skeleton restoration typically completes in ~600ms for a heavy 10-session configuration. A loading page that flashes in and out sub-second reads as a UI glitch rather than a deliberate moment. Portal enforces a **minimum display duration of 1.2 seconds** for the loading page:

```
start := time.Now()
// show loading page
// bootstrap steps 1-7 run
elapsed := time.Since(start)
if elapsed < 1200*time.Millisecond {
    time.Sleep(1200*time.Millisecond - elapsed)
}
// dismiss loading page
```

- If bootstrap is faster than 1.2s → padded to exactly 1.2s.
- If bootstrap is slower than 1.2s → loading page stays until bootstrap returns.

1.2s is intentional: long enough to register as an intentional UX beat, short enough to not become friction.

**CLI path has no loading page, no "Restoring..." output.** Typical bootstrap is ~600ms; fast enough to not need a progress indicator. If long waits surface as user complaints, a stderr one-liner can be added later when `elapsed > 2s`. YAGNI for v1.

### Scope of Bootstrap Decisions vs. Implementation

Everything in this section is design-level. Implementation will pin down:
- Exact tmux command sequences (specific flags, error-handling on each call).
- Go error-propagation strategy for partial failures during restoration.
- Stale-FIFO cleanup on bootstrap (state-directory scan to remove any leftover `hydrate-*.fifo` files that do not match an active pane).
- Unit tests using isolated tmux sockets (`tmux -L <socket>`) for restoration correctness.

These belong in the Planning phase, not in this specification.

## WaitForSessions / bootstrapWait Removal

### What Is Deleted

- **`internal/tmux/wait.go`** — where `WaitForSessions` lives. Deleted entirely.
- **`bootstrapWait` function** (in the `cmd` package). Deleted.
- **All call sites** of both functions. Deleted.

### Why

`WaitForSessions` and `bootstrapWait` existed because Portal had no control over *when* resurrect/continuum would finish populating sessions after startup. They polled (1–6s window, stderr progress output) as a hedge against upstream plugins being slow or failing.

Under the new design, Portal owns restoration directly. When `Restore()` returns, skeleton-restored sessions exist because Portal just created them synchronously. There is nothing to wait for.

### Replacement

A single synchronous `Restore()` call in `PersistentPreRunE`, immediately after `EnsureServer()` and the hook-registration / `_portal-saver` setup steps. When it returns, live tmux state reflects every saved session that was not already live. No polling, no external dependency, deterministic timing.

### What Stays

- **`EnsureServer()`** — keeps its job of starting the tmux server if not running.
- **`serverStarted` flag** in `cmd.Context()` — still used by other call sites (e.g., decision whether to show bootstrap messages).
- **The loading page itself** — retained for the TUI path, with the 1.2s minimum-display-time padding described in Bootstrap Flow.

### Behavioral Improvement

- **Previous:** "Waiting for sessions to populate (up to 6s)" — unpredictable, external-dependency-driven.
- **New:** Bounded by the 1.2s minimum display time and restoration's actual cost (~600ms for heavy configs). No external dependency; timing is deterministic.

## CleanStale Behavior

### Change

**Delete the `if len(livePanes) == 0 { return }` early return** from `CleanStale`'s current implementation. `CleanStale` now runs unconditionally, trusting live tmux state whenever it is invoked.

### Why the Guard Existed

The current guard skipped cleanup when `list-panes -a` returned empty — a hedge against Portal running CleanStale before resurrect/continuum had a chance to restore panes after reboot, which would have nuked all hooks prematurely.

That guard was a workaround for Portal not owning restoration.

### Why It Is Removed

Under the new bootstrap flow, `CleanStale` runs in **step 7 of `PersistentPreRunE`** — *after* skeleton restore completes in steps 5-6. By that point, live panes include both pre-existing panes and skeleton-restored ones. If `list-panes -a` is genuinely empty at step 7, there really are no sessions, and any hooks.json entries are genuinely orphaned.

### Where CleanStale Runs

- **Bootstrap step 7** (every `PersistentPreRunE` invocation, post-restore). Keeps `hooks.json` consistent with live state on every Portal command.
- **`portal clean` command** (user-initiated). Same logic, same unconditional execution.

### Stale-Hook Detection Criteria (Unchanged)

An entry in `hooks.json` is considered stale if its structural key (`session:window.pane`) does not match any live pane enumerated by `list-panes -a`.

**Explicitly NOT criteria for staleness:**
- Hook command's binary missing. That is a runtime execution error when the hook fires, not a stale-entry condition. Portal does not validate hook commands.
- Project removed from `projects.json`. Portal's hook system is generic and has no coupling to `projects.json`.

Keeping the criteria narrow matches the generic-hook design principle: Portal stores and fires commands; validation is the caller's responsibility.

### Refactor Scope

Small mechanical change: remove the `len(livePanes) == 0` early-return branch. Everything else (structural-key matching, atomic write of updated `hooks.json`) stays as it is today.

## CLI Surface

### User-Facing Commands (Under `portal state`)

#### `portal state status`

Liveness check and diagnostic output. Readable in a terminal; exit code is scriptable.

**Output (example):**

```
Portal state:
  Save daemon: running (pid 12345, version v0.4.2)
  Last save: 12 seconds ago
  Sessions captured: 10 (0 ephemeral-skipped)
  Panes captured: 34
  State size: 18.2 MB on disk
  Recent warnings: 0 (last: none)
```

**Data sources:**
- **Daemon liveness:** `has-session -t _portal-saver` plus process verification (pane command resolves to `portal state daemon`).
- **Last save time:** `sessions.json.saved_at`.
- **Session / pane counts:** parsed from `sessions.json`.
- **State size:** total disk usage under `~/.config/portal/state/`.
- **Recent warnings:** scan `portal.log` for entries within the last hour.

**Exit code:**
- `0` — healthy (daemon running, last save recent, no recent warnings).
- non-zero — notable problem (daemon not running, last save older than 5 minutes, recent errors in the log).

Scriptable as well as human-readable.

#### `portal state cleanup`

Explicit teardown for users removing Portal or wanting a clean slate.

Actions:
- `kill-session -t _portal-saver` to terminate the daemon (SIGHUP → final flush on the way out).
- Remove Portal's `set-hook -ga` entries via index-based `set-hook -gu '<EVENT>[N]'` for each event/command pair Portal registers (see tmux Hook Registration Lifecycle for the removal protocol).
- Optionally remove `~/.config/portal/state/` (prompt or explicit flag — planning-phase decision; design intent is "offer the clean-up, don't surprise the user into losing data").

**Not required for correctness.** The defensive hook guard (`command -v portal`) and self-healing idempotency checks handle the "user uninstalled without running cleanup" case transparently. `portal state cleanup` is a first-class option for users who want a deliberate teardown.

### Internal Subcommands (Hidden from `portal --help`)

These subcommands are invoked by tmux hooks and the hosted daemon. They are Portal-internal and not intended for direct user invocation, so they are excluded from `--help` output (Cobra's `Hidden: true` pattern or equivalent).

#### `portal state daemon`

The long-running process invoked as the `command` of the `_portal-saver` session. Responsibilities:

- Write `~/.config/portal/state/daemon.version` on startup with `cmd.version`.
- Clear `save.requested` on startup (defensive).
- Hold the in-memory `paneKey → scrollback-hash` map for content-hash dedup.
- Run the 1-second ticker loop.
- Honor `@portal-restoring` (skip ticks while set).
- Trap SIGHUP and SIGTERM; flush final state (unless `@portal-restoring` is set).

#### `portal state notify`

A small binary (~20 lines of Go) invoked by tmux save-trigger hooks. Responsibilities:

- Touch `~/.config/portal/state/save.requested` (create if absent, bump mtime otherwise).
- Exit 0.

That is the entire behavior. No tmux calls, no state file reads, no logging beyond critical errors. Designed for minimum latency on the hot path of every structural event.

#### `portal state signal-hydrate <session-name>`

Invoked by `client-attached` / `client-session-changed` hooks. Responsibilities:

- `list-panes -t <session-name>` → enumerate panes in the attached session.
- For each pane with `@portal-skeleton-<paneKey>` set: open the pane's FIFO for writing, write a single byte, close.
- For each pane without the marker: no-op.
- Idempotent (safe to double-fire across `client-attached` + `client-session-changed` for a single logical attach).

Does **not** unset skeleton markers. The helper owns that.

#### `portal state hydrate --fifo F --file S`

The pane's initial command at skeleton restore time, wrapped in `sh -c 'portal state hydrate ...; exec $SHELL'`. Responsibilities per Scrollback Restore Mechanics:

- Block on FIFO read (3s timeout).
- On signal: close + unlink FIFO, emit reset preamble, dump scrollback file, emit reset postamble + CRLF, sleep 100ms, read hooks.json, unset skeleton marker, exec hook-or-shell.
- On timeout: emit reset preamble only, leave marker set, log warning, exec bare shell.
- On file missing: emit reset preamble only, log warning, clear marker (by the signal path reaching step g), exec hook-or-shell.

### No User-Facing Manual Save Command

`portal state save` (manual synchronous save) was considered and **rejected**. Every proposed use case was already covered:

- *"Save before reboot"* — SIGHUP flush on tmux server shutdown + 30-second max-gap already cover this.
- *"Scripting / automation"* — speculative; no concrete workflow identified.
- *"Pre-risky-action save"* — same as the reboot case.
- *"Psychological reassurance"* — not a technical need.
- *"Debugging the save mechanism"* — developer concern; can touch the dirty flag manually.

YAGNI. Can be added later if a concrete automation workflow surfaces.

### Namespace Rationale

All eight resurrection-related commands (two user-facing + four internal) cluster under `portal state`. This keeps related commands grouped logically in `portal --help` output (only the user-facing commands are shown), and the internal commands are all reachable via the same `state` prefix when needed for debugging.

Existing Portal CLI top-level namespaces (`portal hooks`, `portal alias`, `portal clean`, `portal attach`, `portal open`, etc.) are unchanged.

### Unchanged User-Facing Surface

- `portal open` / `portal x` — unchanged, now benefits from automatic restoration at bootstrap.
- `portal attach <name>` — unchanged.
- `portal hooks set --on-resume "<cmd>"` — unchanged surface; internals no longer set `@portal-active-<pane>` marker (see Resume Hook Firing).
- `portal hooks list` — unchanged.
- `portal hooks rm --on-resume` — unchanged.
- `portal clean` — unchanged surface; internals no longer have the `livePanes empty` guard (see CleanStale Behavior).
- `portal alias ...`, `portal init`, `portal version` — unchanged.

## Observability & Diagnostics

### Motivation

Silent failure was one of tmux-resurrect / continuum's most-cited problems — users lost data without any indication something had gone wrong. Portal replacing those plugins risks the same opacity if diagnostics are not built in deliberately.

### Log File

**Location:** `~/.config/portal/state/portal.log`. Lives alongside state data, not in the config directory.

**Format:** single line per entry.

```
timestamp | level | component | message
```

- `timestamp` — RFC 3339 UTC.
- `level` — one of `DEBUG`, `INFO`, `WARN`, `ERROR`.
- `component` — short identifier for the subsystem (`daemon`, `restore`, `hydrate`, `notify`, `hooks`, `bootstrap`).
- `message` — free-form, human-readable.

Human-readable and grep-friendly. No structured / JSON format in v1 (YAGNI).

**Log level:** warnings and errors by default. `PORTAL_LOG_LEVEL=debug` env var enables verbose tracing for debugging sessions.

**What gets logged:**

- Save failures (disk full, write errors, permission issues).
- Restoration warnings (missing scrollback file, layout fallback, corrupt `sessions.json`).
- Hydrate timeouts (3-second signal did not arrive).
- Helper crashes (in the rare cases where they can be observed).
- Bootstrap events at `DEBUG` level only.

### Log Rotation

Simple 2-file cap at **1 MB per file**.

- On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` (replacing any existing old file), then starts a fresh `portal.log`.
- Total disk usage bounded at ~2 MB.
- Portal performs rotation itself in-process. No external `logrotate` dependency.

### `portal state status` (Human-Readable Diagnostic)

Primary user-facing diagnostic entry point. Single invocation surfaces the most useful operational data without requiring the user to know where the log file lives. See CLI Surface section for the full command description.

The output includes a "recent warnings" line that scans `portal.log` for entries within the last hour, giving users a quick pointer to recent failures without needing to read the log manually.

### Proactive Health Signals

**Default: silent.** Portal does not nag about transient issues. Users opt in to visibility by running `portal state status`.

**Exception: genuinely broken states detected during `PersistentPreRunE`.** Portal emits a single line to stderr when a critical problem is detected at bootstrap:

- `_portal-saver` cannot be created after retry attempts:

  ```
  Portal save daemon failed to start — sessions won't be captured.
  Run `portal state status` for details.
  ```

- `sessions.json` exists but is unparseable:

  ```
  Portal state file is corrupt — restoration skipped.
  Check `portal state status` or ~/.config/portal/state/portal.log.
  ```

One line. No banners, no colors, no interactive UI. Just enough signal that the user knows to investigate if they care, and quiet enough not to intrude on normal use.

### What Is Explicitly NOT in Scope

- **Metrics endpoint / Prometheus exporter / telemetry.** This is a CLI tool, not a service.
- **Desktop notifications.** Users would not want macOS Notification Center alerts about tmux save status.
- **syslog integration.** Single log file, single tool, local scope.
- **Log search / filter tooling.** `grep` / `awk` over `portal.log` is sufficient.

### Confidence and Scope Rationale

Observability is deliberately modest — enough to diagnose when things break, not so elaborate that it becomes a feature in its own right. The single log file + `portal state status` diagnostic command covers the realistic inspection needs for v1. More elaborate tooling can be added later if real-world usage reveals a gap.

---

## Working Notes
