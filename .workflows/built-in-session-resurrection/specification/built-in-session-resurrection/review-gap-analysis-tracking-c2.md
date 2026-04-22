---
status: complete
created: 2026-04-21
cycle: 2
phase: Gap Analysis
topic: built-in-session-resurrection
---

# Review Tracking: built-in-session-resurrection - Gap Analysis

## Findings

### 1. `portal state notify` behaviour is internally contradictory

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Save-Side Architecture — Single-Writer Serialization (lines ~213-215); Marker Coordination — `@portal-restoring` (line 677); Resume Hook Firing — Session Rename (line 903); CLI Surface — `portal state notify` (lines 1209-1216)

**Details**:
Four sections describe `portal state notify` inconsistently:
- Line 215: "a small binary: touch … exit. No tmux calls, no state file reads, no logging beyond critical errors" (hot path).
- Line 677: `portal state notify` "is a no-op if set [i.e., when `@portal-restoring` is set]; does not even touch the dirty flag." Checking a server option requires a tmux call.
- Line 903: The `session-renamed`-registered hook "(which also fires `portal state notify`)" is "augmented" with hook migration logic rewriting `hooks.json`.
- Line 907: migration "can live either inside `portal state notify` … or as a separate `portal state migrate-rename` internal subcommand invoked by a dedicated hook. Planning-phase decision."

An implementer cannot determine whether `portal state notify`:
(a) is literally 20 lines that just touch a file, or
(b) also reads `@portal-restoring` to decide whether to touch, or
(c) also performs hook-rename migration on `session-renamed`.

Each variant changes the latency profile on the hot path. The "Planning-phase decision" escape hatch on (c) is fine, but combined with variant (b), the spec has three different operational descriptions of the same binary. Pin down which tmux reads (if any) `notify` performs, and whether the rename path is inside `notify` or a separate subcommand (including its registration as a distinct hook if so).

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 2. Session environment restoration mechanism is not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Restore-Side Architecture — Skeleton-Eager + Scrollback-Lazy; Bootstrap Flow — step 5; Layout Restoration — Summary of Order

**Details**:
`sessions[].environment` is captured per-session via `show-environment -t <session>` and stored in `sessions.json`. On restore, the spec lists the operations performed per session (new-session, new-window, split-window, select-layout, select-pane, resize-pane -Z) but never mentions applying the captured environment. An implementer must decide:
- Apply via `set-environment -t <session> KEY VAL` per key, and if so, before or after panes are created? (Panes inherit env at creation — ordering matters.)
- Skip env restoration entirely and rely on shell-level rc files? (Contradicts the stated intent of capturing it.)
- What to do when a variable was stored with `-r` (scheduled for removal) form, vs. a plain set?

Without this, the captured `environment` field is either dead data or implemented ambiguously.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 3. paneKey index source (saved vs. live) at restore time is ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Save Format & Schema — Canonical paneKey; Index Semantics and base-index / pane-base-index; Scrollback Restore Mechanics — Signal Mechanism; Marker Coordination

**Details**:
The paneKey is `sanitize(session) + "__" + window_index + "." + pane_index` and drives scrollback filename, FIFO filename, and `@portal-skeleton-<paneKey>` marker name. Index Semantics (lines 319-327) establishes that on restore the live tmux indices may differ from saved indices (e.g., `base-index`/`pane-base-index` changed), so Portal re-queries `list-panes` to remap. Unresolved:
- The scrollback file on disk is named with the **saved** paneKey (created at save time). 
- The helper is invoked with `--file <scrollback>` — passed path at bootstrap, so Portal can plug in the saved-indexed file name. OK.
- The helper is invoked with `--fifo <F>` — at which indexing? FIFO is created fresh per bootstrap, so Portal can use live indices. But then `signal-hydrate` must enumerate live panes and compute paneKey from LIVE indices to locate the FIFO, meaning marker names must also use live indices.
- The daemon's `@portal-skeleton-*` enumeration and `list-panes` cross-reference (lines 665-666): marker key must match live paneKey.
- Hook lookup in `hooks.json` uses raw (session, window, pane) key — but saved session had one set of indices; live recreated session may have different indices. Does the helper look up hooks by saved-structural key (from sessions.json) or live-structural key (from its own env)?

Three consistent options exist (all-saved, all-live, mixed with explicit mapping), and the spec doesn't pick one. An implementer must choose and risk any of the save/restore/signal/hydrate/hook paths going out of sync.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 4. Helper "file missing" branch contradicts its own marker-unset ordering

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Scrollback Restore Mechanics — Helper Behavior on Startup (pseudocode, lines 760-785)

**Details**:
Helper pseudocode on signal arrival runs steps a-h in order; step g ("unset marker") is AFTER step c ("copy the scrollback file's bytes"). Branch 4 ("On scrollback file missing / unreadable") instructs:
  c. "Marker was already cleared by the signal path's step g — skip (empty pane)."

This is incorrect: if step c (file copy) fails because the file is missing, step g has not yet run. The marker-unset ordering in branch 4 is therefore self-contradictory. Two real interpretations exist — (i) on file-missing, unset the marker inline (fall through to normal empty-pane completion); or (ii) do not unset, leave for next attach — and the spec's current wording tries to assert both.

An implementer must decide: on file-missing, does the pane stay in "awaiting hydration" state (marker set, next attempt may succeed if some other file appears? unlikely) or transition to "hydrated-empty" (marker cleared, daemon resumes normal capture)? The later failure-modes table (line 814, line 1356) suggests the latter but contradicts pseudocode branch 4.c literally.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 5. Stderr bootstrap warnings interact badly with the TUI loading page

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Flow — step 5 (line 998); Observability & Diagnostics — Proactive Health Signals (lines 1317-1332); CLI Surface — Loading-Page Minimum Display (line 1072)

**Details**:
Step 5 says on corrupt `sessions.json`, Portal "print[s] one-line stderr warning, skip[s] restoration entirely." Observability lists two such stderr-emitting conditions (corrupt state file, saver-creation failure). The TUI path wraps bootstrap steps 5-7 with the Bubble Tea loading page, which owns the terminal during that window. A stderr write during the Bubble Tea program phase will either:
(a) intermix into the rendered UI (visible corruption), or
(b) be buffered and appear only after the program exits (unclear ordering), or
(c) be swallowed if the TUI captures stderr.

The CLI path has no conflict. The spec treats "emit to stderr" uniformly across both paths. An implementer needs explicit guidance: queue warnings to display after the TUI dismisses the loading page? Use a Bubble Tea message to surface them? Suppress them entirely on TUI path and rely on `portal state status`? Each option has UX consequences.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 6. Content-hash dedup map after daemon restart triggers full rewrite

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Save Format & Schema — Content-Hash Dedup; CLI Surface — `portal state daemon` responsibilities

**Details**:
The in-memory `paneKey → hash` map is the only cache preventing redundant scrollback writes. On daemon startup (including version-mismatch restart at every `portal open`, and every server restart), the map is empty. The first tick after startup will rehash every pane's scrollback and write all files that are "different" from the missing map entry — effectively, write all of them unconditionally.

For users running dev builds (spec says "always restart on bootstrap for `dev`/empty version"), this means every `portal open` triggers a full rewrite of all scrollback files. On a heavy config (30 panes × avg 500KB scrollback), that's ~15MB of writes per `portal open` invocation during development. Not catastrophic but unexpected given the "single-digit MB/day" budget rationale.

Options: (i) persist the hash map to disk alongside sessions.json (small JSON add-on); (ii) hash the existing on-disk scrollback file at startup to seed the map; (iii) document that daemon restarts force a full rewrite and accept it. The spec should pick one (or explicitly defer to YAGNI) so the dev-build experience isn't a surprise.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 7. `portal state cleanup` exit-code and partial-failure semantics unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Surface — `portal state cleanup`

**Details**:
The spec lists actions for `portal state cleanup`: kill the saver session, remove registered hooks, optionally remove the state directory. No contract is specified for:
- Exit code on success, partial-success, or failure. (Compare to `portal state status`, which spells out 0 vs non-zero meaning.)
- Behavior when the saver session is already absent (idempotent success, or note it?).
- Behavior when one hook removal succeeds and another fails (tmux error mid-loop).
- Whether the optional state-dir cleanup is interactive (prompt) or flag-gated (`--remove-state-dir`), and what happens if the flag is set but `state/` doesn't exist.

An implementer must design the failure model from scratch. Given cleanup is "deliberate teardown," users running the command in scripts will rely on the exit code to branch. Leaving this implicit means different implementations will behave differently.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 8. Log rotation under concurrent writers is unaddressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Observability & Diagnostics — Log Rotation (lines 1299-1305)

**Details**:
Spec describes rotation: "On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` (replacing any existing old file), then starts a fresh `portal.log`." It says "Portal performs rotation itself in-process."

Multiple Portal processes write concurrently to `portal.log`:
- The daemon (long-running, in `_portal-saver`).
- Any Portal CLI/TUI command that reaches `PersistentPreRunE` and logs during bootstrap (e.g., "restoration warning").
- `portal state signal-hydrate` subprocesses during attach hooks.
- `portal state hydrate` helpers inside each pane at restore time (can log on FIFO timeout / file-missing).

Race scenarios:
- Two processes both observe file size ≥ 1MB → both rename. Second rename overwrites first's `portal.log.old`, losing the older log history.
- Process A renames, process B still holds an open fd on the old file and writes, creating an orphan file on Linux (overwritten on next rotation) or inconsistent data.
- No locking scheme is mentioned.

The spec's "in-process" rotation is silently assuming a single writer. Either pin down a lock file / advisory lock protocol, or restrict rotation to a designated writer (e.g., only the daemon rotates; CLI appenders just write and let the daemon handle size).

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 9. Hook lookup by structural key when restored indices differ from saved indices

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Resume Hook Firing — Firing Point (line 848); Save Format & Schema — Canonical paneKey / Hook structural keys (line 343); Scrollback Restore Mechanics — Helper Behavior on Startup (line 770)

**Details**:
Hooks are keyed by `session:window.pane` with raw (un-sanitized) session name and raw indices. On restore, when `base-index` or `pane-base-index` differs between save and restore, live indices do not match saved indices. Two gap-adjacent questions arise:

(a) Does the helper look up hooks using the **live** paneKey (from its own tmux context) or the **saved** paneKey (embedded in a flag like `--hook-key`)? The spec says the helper "looks up the resume hook for this pane's structural key" — but does not define which indices form that key.

(b) Is there any automatic migration of `hooks.json` keys when live indices diverge from saved indices? Finding 8 in cycle 1 addressed session *renames*; it did not address index drift. Without migration, hooks registered against saved indices would be missed on the restored pane, then pruned as stale by CleanStale in step 7 — silently deleting user-registered hooks on any `base-index`/`pane-base-index` change.

Either the helper receives the saved key from bootstrap at pane creation (safe) or indices must always match (contradicts cycle-1 resolution) or stale pruning must be gated on something more than index mismatch.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 10. Loading-page dismissal on unrecoverable bootstrap errors

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Bootstrap Flow — steps 1-8 and Return-to-Caller Timing (lines 1045-1068)

**Details**:
The spec handles per-session restore errors gracefully (log, skip, continue). It handles corrupt `sessions.json` (skip restoration, continue). It handles `_portal-saver` creation failure (retry, then warn, continue without the daemon). But for failures earlier in the chain, behavior is unspecified:
- `EnsureServer()` fails (e.g., `tmux` binary missing despite tmux 3.0+ check, or permission error starting the server).
- `set-hook -ga` calls fail (e.g., tmux version mismatch slipped past the 3.0 check because parsing tmux -V returned something unexpected).
- `@portal-restoring` set-option fails.

Under the TUI path, the loading page is active when these failures occur. Does the TUI:
- Dismiss the loading page, show the error in-TUI, exit the program?
- Tear down the loading page and drop the user back to the shell with a stderr message?
- Stay on the loading page forever because "restoration completed" is never reached?

For CLI path, does the command propagate an error (non-zero exit) or continue in degraded mode? The spec's "degrade locally, log, continue" principle applies clearly for per-session errors but is underspecified for foundational setup errors.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 11. `pane_current_command` liveness check for `_portal-saver` is ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Save-Side Architecture — Lifecycle Summary (line 170, resolved in cycle 1)

**Details**:
Cycle 1's resolution added: "Bootstrap additionally verifies the first pane's `#{pane_current_command}` resolves to the daemon binary (e.g., contains `portal` and the pane is still active)."

`#{pane_current_command}` returns the short process name of the foreground process (not the full argv). Expected value when the daemon is healthy: `portal` (from `portal state daemon`). Ambiguities:
- On macOS, when a Go binary is invoked, the short name is the binary name — but if Portal is installed via `brew` and symlinked, depending on platform and exec model, the name could vary.
- If the daemon briefly shells out (never should, but defensive): `#{pane_current_command}` would transiently report `sh` or the child's name.
- If the user somehow invokes `portal <anything>` manually inside `_portal-saver` (not a real workflow, but theoretically possible via `send-keys`): the short name is `portal` even though the daemon is dead.

The spec says "contains `portal`" which is a substring check. An implementer needs to know whether:
- Exact match `portal` is required.
- Substring `portal` (any argv[0] containing it) is enough.
- Whether additional verification (e.g., `#{pane_pid}` matches the PID in a PID file) is preferred.

Without a precise predicate, false positives (treating a non-daemon pane as live daemon) are possible, which would leave the user without a running saver.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---

### 12. Scrollback file written with mode 0600 but existing parent dirs are not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Save Format & Schema — Storage Location (line 286); Directory Layout

**Details**:
Spec says "All files written with mode `0600` (owner read/write only)." Directories are not addressed. On first-ever bootstrap, Portal must create `~/.config/portal/state/` and `~/.config/portal/state/scrollback/`. If these directories are created with a permissive umask (e.g., 0755 default), scrollback content (potentially sensitive) could be listed by other users on a multi-user system, even if individual file contents are 0600.

Not a critical flaw — the file contents are the sensitive part — but the Privacy Considerations README section (line 1462) promises 0600 file mode. A corresponding commitment to 0700 directory mode would match the intent. An implementer should know whether to explicitly `os.Mkdir(path, 0700)` or accept umask default.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Resolution synthesized and applied in auto mode.

---
