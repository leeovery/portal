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

---

## Working Notes
