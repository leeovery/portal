---
status: in-progress
created: 2026-04-21
cycle: 1
phase: Gap Analysis
topic: built-in-session-resurrection
---

# Review Tracking: built-in-session-resurrection - Gap Analysis

## Findings

### 1. CleanStale scheduling vs. `portal clean` exemption from bootstrap

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Flow (step 7), CleanStale Behavior, CLI Surface

**Details**:
Bootstrap Flow says `CleanStale()` runs as step 7 of `PersistentPreRunE`. CLAUDE.md states `PersistentPreRunE` is skipped for `clean` (among other commands). If the `clean` command is still exempt from bootstrap under the new design, then `portal clean` would never run the new unconditional CleanStale in the bootstrap path — it would only run whatever CleanStale logic the command itself invokes. The spec needs to state explicitly whether `portal clean` now participates in the bootstrap flow, or whether it invokes CleanStale directly via its own code path. Related: "CleanStale runs in step 7 *after* skeleton restore" is the rationale for removing the empty-livePanes guard; if `portal clean` does not run skeleton restore first, the safety justification collapses for that invocation path.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. `destroy-unattached` scope is unspecified

**Source**: Specification analysis
**Affects**: Save-Side Architecture — Defensive Session Setup

**Details**:
Spec says `set-option -t _portal-saver destroy-unattached off`. In tmux, `destroy-unattached` is a session option; the command needs correct scope flags (`-s` for session option targeting a specific session via `-t`). A bare `set-option -t <session> destroy-unattached off` is the correct invocation in modern tmux, but the spec doesn't clarify. An implementer could try `-g` (global) accidentally, which would override the user's global setting in the wrong direction. Pin down the exact tmux invocation.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. `_portal-saver` present but daemon dead

**Source**: Specification analysis
**Affects**: Save-Side Architecture — Lifecycle Summary, Bootstrap Flow step 4

**Details**:
Bootstrap uses `has-session -t _portal-saver` to decide whether to create the saver session. If the session exists but the daemon process inside it crashed (without tmux noticing or auto-destroying), `has-session` returns true and bootstrap skips creation — but no daemon is running, so no saves happen. Spec mentions "dead session is recreated on the next `portal open`" — but this assumes tmux auto-destroys the session on daemon exit. Depending on `remain-on-exit` or other config, an orphaned empty session could persist. `portal state status` would detect this (liveness check), but bootstrap wouldn't heal it. Need to specify: does bootstrap verify daemon liveness (e.g., pane command resolves to `portal state daemon`) or rely purely on session presence?

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. Pane indices after skeleton restore — `pane-base-index` assumption

**Source**: Specification analysis
**Affects**: Restore-Side Architecture, Layout Restoration, Bootstrap Flow step 5

**Details**:
Restoration creates panes via `new-window` then `split-window`. tmux pane indices depend on `pane-base-index` (0 or 1, user-configurable). Saved pane indices in `sessions.json` were captured from the user's running tmux, so they reflect the user's `pane-base-index` at save time. On restore:
- `select-pane -t <active pane index>` references a specific index.
- `select-layout "<saved>"` encodes pane IDs (but these are reassigned on new panes).
- FIFO paneKey (`session:window.pane`) and scrollback filename depend on the index.

If `pane-base-index` changed between save and restore (unlikely but possible), the saved indices won't match new indices. Spec doesn't cover this. Also: does `split-window` consistently produce panes in the expected order for subsequent `select-layout` to work? `select-layout` rearranges geometry but not which pane is at which index.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 5. Window indices and `base-index`

**Source**: Specification analysis
**Affects**: Save Format & Schema, Restore-Side Architecture, scrollback filename scheme

**Details**:
Analogous to finding 4 but for windows. Saved `windows[].index` is the tmux window index, which depends on user's `base-index` (commonly 0 or 1). `new-session -d -s <name>` creates the first window at the user's current `base-index`, not necessarily at the saved index. Subsequent `new-window` calls create windows at the next available index — which may or may not match saved indices.

Filename scheme `<session>__<window>.<pane>.bin` depends on the index matching. Structural key `session:window.pane` for hook lookup depends on it. If base-index or actual created indices don't align, hydration will look for the wrong FIFO / scrollback file and hooks won't match.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 6. FIFO open/write race — signal-hydrate opening for write before helper opens for read

**Source**: Specification analysis
**Affects**: Scrollback Restore Mechanics — Signal Mechanism

**Details**:
Per POSIX, opening a FIFO for write blocks until a reader opens it (unless `O_NONBLOCK` is used). `signal-hydrate` "opens the pane's FIFO for writing and writes a single byte." Case to consider: a user attaches via `tmux attach` very soon after bootstrap, before the helper process inside the freshly-created pane has reached its `os.OpenFile(fifo, O_RDONLY)` call. `signal-hydrate` then blocks on open-for-write.

Impact: `signal-hydrate` is invoked via `run-shell` which is synchronous and blocks the tmux server. A stuck `signal-hydrate` would freeze the server until the helper catches up.

Mitigations to consider: open with `O_NONBLOCK | O_WRONLY` and `EAGAIN` retry, or use a goroutine + timeout. Spec should explicitly specify the FIFO open-side behavior.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 7. Daemon's marker detection per pane — enumeration mechanism unspecified

**Source**: Specification analysis
**Affects**: Marker Coordination, Save-Side Architecture — Single-Writer Serialization

**Details**:
Spec says "the daemon's capture loop **skips** panes whose marker is set" (referring to `@portal-skeleton-<paneKey>`). But it does not specify how the daemon enumerates these markers. Options:
- Per-pane `show-option -sv @portal-skeleton-<key>` (N invocations, slow).
- Bulk `show-options -sv` and filter.
- Query via `#{@portal-skeleton-<key>}` format string per pane during `list-panes`.

This matters for daemon tick performance (the whole point of skeleton markers is not repeatedly bumping unhydrated-pane scrollback). Also: the markers are set at "server-option level for volatility" — pin down the exact syntax and retrieval mechanism.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 8. Session rename breaks hook structural keys

**Source**: Specification analysis
**Affects**: Hook System Lifecycle Behavior, Restore-Side Architecture, CleanStale Behavior

**Details**:
Hook structural key is `session:window.pane`. When a user renames a session (`tmux rename-session`), the session-renamed event fires. But hooks registered to the old session name now have a structural key that no longer matches any live pane → they become "stale" by the unchanged staleness criteria and will be pruned by the next CleanStale run (bootstrap step 7 on next `portal open`).

This silently deletes user-registered hooks when sessions rename. The spec doesn't say whether this is intentional. Options: update hook keys on `session-renamed` events, or document this as known behavior, or migrate keys during CleanStale.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 9. `current_command` captured but original command not re-executed on restore

**Source**: Specification analysis
**Affects**: Save Format & Schema, Restore-Side Architecture

**Details**:
Spec captures `current_command` (e.g., "vim", "zsh") "for diagnostic visibility in `portal state status`; not load-bearing for restoration." On restore, every pane runs `sh -c 'portal state hydrate ...; exec $SHELL'`. This means:
- A pane that had vim running at save time comes back running `$SHELL`, not vim.
- The "resume hook" mechanism is how users compensate, but only if they manually registered a hook.

This is by design (per "Out of Scope — Uncapturable by tmux") but the spec leaves open: should `portal state status` surface "N panes had non-shell current_command — consider hooks"? Is there any user-visible nudge, or is it silent? Readers of the spec will wonder whether `current_command` appears in any output or is purely an internal diagnostic field.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 10. Session name collision with live user session during restore

**Source**: Specification analysis
**Affects**: Restore-Side Architecture — Restoration Trigger

**Details**:
Spec says "if live tmux session already exists with that name → skip; Portal never clobbers live sessions." This handles the happy case. But:

- The *saved* scrollback files for that name persist in state dir. On next save cycle, the daemon captures the now-live session with that name. If the user's live session has a *different* structure than saved, the daemon overwrites saved state with live state — correct.
- But between bootstrap-skip and next save, hydration hooks registered on the live (different-structure) session reference the saved scrollback files. If any pane of the live session matches a saved paneKey (`session:window.pane`), there's no `@portal-skeleton` marker (because skeleton restore was skipped), so `signal-hydrate` no-ops. Safe.

What's unclear: does the spec handle the case where skeleton restore was *partially* applied for a session (e.g., crashed mid-restore) and then bootstrap re-runs on the next `portal open`? `has-session` returns true (partial live state exists) so full skeleton restore is skipped. Partial state sits there, not re-completed. Failure table mentions this under "Skeleton restore crashes partway" with "newly-created partial state becomes the live state" — but a partial state with some panes as helper-commanded and others missing is a degraded configuration. Spec should clarify whether this is acceptable or whether a partial-session detection is needed.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 11. Sanitized session name collisions for paneKey and hook lookup

**Source**: Specification analysis
**Affects**: Save Format & Schema, Scrollback Restore Mechanics, Resume Hook Firing

**Details**:
Scrollback filename sanitizes session name and falls back to hash suffix on collision: "On collision (two sanitized session names map to the same file key), append a hash suffix." But:
- The paneKey used for `@portal-skeleton-<paneKey>`, FIFO path (`hydrate-<paneKey>.fifo`), and hook lookup (`session:window.pane`) is defined separately.
- Hook lookup is described as "structural key `session:window.pane`" — using the raw session name? Or sanitized? If raw, hooks for a session named `work:alpha` (with colon) would be ambiguous to parse.

Need consistency: either one sanitization scheme used everywhere (filename, FIFO, paneKey, hook key), or explicitly distinct schemes for each context with collision rules for each.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 12. Save trigger event list may miss pane-creation events

**Source**: Specification analysis
**Affects**: Save-Side Architecture — Trigger Layers

**Details**:
Save-trigger events listed: `session-created`, `session-closed`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`.

Not listed:
- `pane-exited` / `pane-died` (pane closes within a window) — maybe covered by `window-layout-changed` since pane-close changes layout?
- Creation of a new pane via split (maybe covered by `window-layout-changed`).
- Window renamed (`window-renamed`) — would update `windows[].name` in sessions.json.

The comment "These catch structural changes (session/window/pane topology, renames, layout changes, focus transitions)" suggests completeness is claimed. Readers/implementers need confirmation that `window-renamed` is deliberately excluded (window name isn't restoration-critical — captured at save time via list-panes anyway and 30s max-gap catches drift).

Either pin down each event's reasoning or note that 30s max-gap is the backstop.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 13. Daemon version file absent but `_portal-saver` exists

**Source**: Specification analysis
**Affects**: Bootstrap Flow step 4 — Version-based restart

**Details**:
Step 4 reads `daemon.version` and compares to `cmd.version`. "If the version file is absent (first-ever bootstrap) → treat as mismatch; recreate."

Edge case: `_portal-saver` exists (daemon running), but `daemon.version` file was manually deleted by user or a partial state dir cleanup. "Treat as mismatch → kill-session + recreate" is applied unconditionally. This kills a working daemon unnecessarily.

Two-axis check (version file present + session present) would disambiguate: (a) file absent AND session absent → fresh start; (b) file absent AND session present → either write current version from running daemon's info, or kill + recreate. Spec doesn't discuss this path.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 14. Tmux minimum-version runtime check

**Source**: Specification analysis
**Affects**: Scope & Constraints — Minimum Versions, Bootstrap Flow

**Details**:
Spec requires tmux ≥ 3.0 (for `set-hook -ga` array semantics). No runtime check is mentioned in bootstrap. If a user runs Portal against tmux 2.x:
- `set-hook -ga` will fail silently or noisily.
- Hook registration won't work; save/restore silently broken.

Should bootstrap query `tmux -V`, compare, and emit a clear error if below 3.0? Otherwise the "mysteriously not working" UX is the exact silent-failure mode Observability section argues against.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 15. Restoration of sessions during running save-daemon cycle — first-tick race

**Source**: Specification analysis
**Affects**: Bootstrap Flow, Save-Side Architecture — Defensive Dirty-Flag Clear

**Details**:
Bootstrap step 3 sets `@portal-restoring 1` BEFORE step 4 creates `_portal-saver`. So daemon starts with `@portal-restoring` already set and its first tick skips. Good.

BUT: step 4 also handles the "daemon already running, version matches" path where `_portal-saver` is not recreated. In that case, the *existing* daemon has been ticking for however long — perhaps it already fired a tick and captured current state — *before* `@portal-restoring` was set. Then step 3 happens (`@portal-restoring = 1`). The daemon's next tick (within 1 second) would be suppressed. But what about a tick already in progress? The daemon checks `isRestoringFlagSet()` at the top of each tick iteration — if a capture cycle is mid-execution when the flag flips to 1, the in-flight capture completes and may overwrite sessions.json with pre-restore state (which is fine — pre-restore = steady state).

Less clear: the daemon's in-flight capture enumerating sessions races with bootstrap creating new skeleton sessions; could a mid-restore partial session appear. Spec says skeleton restore is sequential and happens AFTER step 3 — so if the daemon is mid-capture when step 3 runs, it completes capture of the pre-restore state, then subsequent ticks are suppressed. OK.

What's missing: explicit statement about in-flight captures vs `@portal-restoring` flag flip. Is there a per-tick atomicity guarantee, or could a capture started before the flag commit a write after the flag is set? Implementer-level question worth nailing down.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 16. Empty pane list within a window — edge case

**Source**: Specification analysis
**Affects**: Save Format & Schema, Restore-Side Architecture

**Details**:
Spec's sessions[].windows[].panes is a JSON array; tmux windows always have ≥1 pane. An edge case where panes is empty would be invalid, but what if `sessions.json` is manually edited or some corruption leaves a window with empty panes? Parse accepts it; restore calls `new-window` without any `split-window` — fine, tmux creates the default one pane. But:
- That pane has no hydrate command (nothing in the panes array to iterate).
- Layout string references a non-existent pane → `select-layout` fails → falls back to tiled. OK.

Spec's restoration pseudocode assumes `panes` non-empty. Worth stating that restore treats an empty panes array as "create just the default window" or "skip the window entirely and log warning."

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 17. TUI "loading page" during restore — what is shown?

**Source**: Specification analysis
**Affects**: Bootstrap Flow — Loading-Page Minimum Display

**Details**:
Spec mentions "TUI path wraps this step (and steps 6-7) with the loading page (1.2s minimum display)" and "a loading page that flashes in and out sub-second reads as a UI glitch." But the spec doesn't describe what the loading page looks like:
- Text/spinner?
- Progress indicator ("restoring 3/10 sessions")?
- Static message?

This might exist already from the prior design (CLAUDE.md mentions a loading page). If so, the spec should state explicitly "reuse existing loading page; no visual changes." If the page needs new copy (e.g., "Restoring sessions..."), an implementer would have to invent it.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 18. `portal state status` recent-warnings scan window

**Source**: Specification analysis
**Affects**: CLI Surface — `portal state status`, Observability & Diagnostics

**Details**:
Spec says status output shows "Recent warnings: 0 (last: none)" and the scan window is "last hour." Fine. But:
- What counts as "recent" for exit-code purposes — "recent errors in the log" for non-zero exit — same 1-hour window or a different window (like last save cycle)?
- If `portal.log` does not exist yet (first run), is that a warning or silent zero?
- If `portal.log.old` holds entries from 2 hours ago, are they scanned (no, since only current file per-hour)? Spec implies only current log file is scanned.

Minor ambiguity but worth pinning down so exit-code semantics are stable.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 19. `portal state cleanup` interaction with running daemon

**Source**: Specification analysis
**Affects**: CLI Surface — `portal state cleanup`, Observability

**Details**:
`portal state cleanup` is a regular Portal command. Does it go through `PersistentPreRunE`? If yes:
- Step 2 re-registers hooks.
- Step 4 ensures `_portal-saver` is running.
- Step 5 runs skeleton restore.
- THEN step 7 runs CleanStale.
- THEN the command body kills `_portal-saver` and removes hooks.

So bootstrap sets up everything, and the command tears it down immediately. Wasteful and weird. The "clean" command is exempt from bootstrap per CLAUDE.md — but `state cleanup` is a new command. Is it also exempt? Spec doesn't say. Implementer needs to know whether `state cleanup` (and `state status`) skip bootstrap.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 20. `portal state save.requested` path naming

**Source**: Specification analysis
**Affects**: Save-Side Architecture — Single-Writer Serialization, Directory Layout

**Details**:
Directory layout shows `save.requested` at the top of `state/`. But the detailed text uses `~/.config/portal/state/save.requested`. These agree. However, "touch (create or bump mtime of)" — spec doesn't specify the file's contents (empty? marker bytes?). Detailed behavior: stat for presence. Should be explicit that contents are irrelevant — any file with that name triggers the dirty state.

Also: what cleans up `save.requested` when daemon is not running? The spec says "defensive dirty-flag clear on daemon startup." But if no daemon ever starts again, the file lingers. Harmless, but worth noting as intentional.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 21. Priorities summary — implementer readiness

**Source**: Specification analysis
**Affects**: All sections

**Details**:
Overall the spec is dense, thoughtful, and implementation-ready for most sections. The most critical items for a planner's attention (to avoid design decisions being made at implementation time):

**Critical** (would block planning):
- Finding 1: CleanStale scheduling vs. exempt-from-bootstrap commands.
- Finding 6: FIFO open-for-write blocking semantics.
- Finding 19: Which `portal state ...` subcommands run bootstrap?
- Finding 14: tmux version runtime check.

**Important** (would force design decisions during implementation):
- Finding 2: `destroy-unattached` exact invocation.
- Finding 3: Dead-daemon / live-session detection.
- Findings 4 & 5: pane-base-index and base-index handling.
- Finding 7: Daemon marker enumeration mechanism.
- Finding 8: Session rename + hook key semantics.
- Finding 11: Sanitization consistency across paneKey, FIFO, filename, hook key.
- Finding 13: Version file absent edge case.

**Minor** (clarification):
- Findings 9, 10, 12, 15, 16, 17, 18, 20.

This meta-finding is a roadmap; remove it once others are resolved.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
