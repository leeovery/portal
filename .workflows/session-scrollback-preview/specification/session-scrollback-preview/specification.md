# Specification: Session Scrollback Preview

## Specification

### Overview

A Quick Look-style preview of a session's scrollback, opened from the Sessions page of the TUI. Lets the user disambiguate similarly-named sessions (e.g. `pigeon-AbCdEf` vs `pigeon-XyZw12`) by glancing at terminal content, without paying the attach/detach cost.

**Use case framing.** Disambiguation is a recognition task — the user is glancing to identify which session is which, not watching content change in real time. Every downstream decision (staleness ceiling, history depth, refresh model, visual fidelity) is anchored to recognition, not live monitoring.

**Side-effect-free contract.** Opening and dismissing the preview leaves session state byte-identical: no hydration, no resume-hook firing, no tmux marker mutation, no FIFO consumed. Preview is read-only with respect to portal state and tmux.

### Trigger and Entry Point

- **Page binding.** Bound to the **Sessions page only** (`updateSessionsPage` in `internal/tui/model.go`). Projects, FileBrowser, and the Loading page have no preview entry point.
- **Open.** `Space` on the highlighted session in the Sessions list opens preview.
- **Attach.** `Enter` continues to attach as today (unchanged).
- **Dismiss.** `Esc` returns to the Sessions list at the same cursor position the user was on when `Space` was pressed.
- **Self-exclusion.** The current session (when running inside tmux) and the `_portal-saver` detached session are already excluded from the Sessions list by existing logic; preview inherits this exclusion — no new suppression layer required.

### Interaction Shape

- **Sub-page peer of `pageFileBrowser`.** Preview is a full-screen page in the TUI's page state machine, with its own keymap. Implemented as a new `pagePreview` arm in `internal/tui/model.go::Update`.
- **Bound to one session per open.** Preview shows the session that was highlighted when `Space` was pressed. To preview a different session, the user `Esc`s back to the list, moves the cursor, and presses `Space` again. There is no in-preview between-session stepping.

### Source of Preview Bytes

Preview reads pane content **only from disk** — the `.bin` scrollback files written by the `portal state daemon`. There is no live `tmux capture-pane` path, no fork on `@portal-skeleton-<paneKey>` marker, and no new tmux wrapper.

**Read API.** `state.ScrollbackFile(stateDir, paneKey)` resolves the on-disk path for a given pane key. Preview reads from this path directly for every pane it renders, regardless of whether the pane is currently hydrated or still skeletal.

**Single read path consequences.**
- No marker check; no rendering fork.
- No per-preview tmux IPC.
- The same path that already serves skeleton panes during restore is reused for hydrated panes.
- The rapid-stepping race between two in-flight tmux captures cannot occur (file reads are microseconds and synchronous).
- No new tmux wrapper. The existing `tmux.Client.CapturePane` hardcodes `-S -` (full scrollback) and is shared with save-daemon semantics; a bounded variant (e.g. `CapturePaneTail(target, n)`) would have been net-new code. Always-disk avoids that addition entirely.

**Snapshot semantics — not live.** Preview is a snapshot at the moment each pane is read. Worst-case staleness is bounded by the daemon's per-tick interval (small but not strictly bounded by 1s under heavy load). This is acceptable for the recognition use case at both ends of the bandwidth spectrum:
- Slow output (e.g. Claude TUI): staleness invisible because content moves slowly.
- Fast output (e.g. tailing logs): staleness invisible because the user identifies by overall shape, not individual lines.

**Concurrent attach.** Portal targets a single-user-per-machine environment; concurrent attach contention is not a design constraint.

**Surface label honesty.** Preview is a snapshot, not "what attaching now would show". Any user-facing labelling must not promise liveness.

### Multi-pane Rendering Shape

Sessions can contain multiple windows, each with multiple panes. Preview renders the structure as a **sequential, window-grouped** flat presentation: one pane shown at a time, with hierarchical cycling between panes within a window and between windows.

The literal `window_layout` is **not** rendered. No layout parser. Visual fidelity is discharged by chrome (counts, names, position) rather than by reproducing the spatial grid.

#### Within-preview Key Bindings

| Key | Action |
|-----|--------|
| `]` | Next window (wraps from last → first) |
| `[` | Previous window (wraps from first → last) |
| `Tab` | Next pane within current window (forward-only with wraparound) |
| `Esc` | Dismiss preview, return to Sessions list |

**Why bidirectional for windows but not panes.** Windows are typically purposeful (editor / logs / repl) and overshoot is costly, so `[` and `]` are both bound. Pane counts are small; forward-only `Tab` with wraparound is sufficient for the dominant case.

**Degenerate cases.** In single-window single-pane sessions (the dominant ~95% case), all three cycle keys silently no-op. No flicker, no error feedback, just nothing.

**Position on session re-entry.** Stepping out via `Esc` and re-opening preview on the same session re-opens at **window 1 / pane 1**, not at the last viewed position. Per-session position state is not retained — the use case is disambiguation, and fresh-view matches recognition better than memory.

#### Chrome Floor (v1 must-show)

Chrome must show structural overview and current position so the user can identify "which window am I in" and "how many siblings does this pane have" at a glance. The minimum content is:

- **Window M of N** — without this, users have no signal that the session has multiple windows.
- **Pane X of Y** — same logic within a window.
- **Window name** — tmux's `#W` / window name; adds disambiguation signal for users who name their windows.
- **Keystroke hints** — visible cycle-key reminders (`] [ Tab Esc`), matching Portal's existing UI convention elsewhere.

**Chrome data source.** Window/pane counts and names come from **tmux structural enumeration** (e.g. `list-panes -F`), not from `.bin` content. Chrome is computed once at preview-open and cycled in place; no live re-enumeration mid-preview.

**Above the floor (rejected for v1, additive later if wanted):**
- Per-pane current-command (e.g. `nvim`, `claude`).
- Pane position hint (e.g. `(top-right)`).

**Open spec-pinning items handed to the build phase:**
- Exact chrome wording, header vs footer, single-line vs two-line.

### History Depth

Preview renders a **bounded snapshot, scrollable within bounds**, of each pane's saved scrollback. Per-pane `.bin` files on disk can be large (~3.7MB / 50k+ lines for busy sessions); preview never feeds the full file into the renderer.

**Slice size.** The last **N lines** of the pane's `.bin` file are read on each focus event, where **N = 1000**. This pins the working figure of "generous N (e.g. ~500–1000 lines)" at the upper end. The exact value is a constant in the read pipeline.

**Memory trade-off.** Holding N lines per previewed pane while focused is the dominant memory cost of preview. At N ≤ 1000, this footprint is negligible (typical line widths × 1000 ≪ a few MB) — the cap on N is set by this trade-off as much as by the recognition use case. If a future revision wants deeper history, raising N has a real, if still small, memory cost that should be re-justified rather than silently bumped.

**Scroll within bounds.** The viewport (`bubbles/viewport`) renders the tail by default; the user can scroll up within the loaded N lines using the viewport's native scroll keymap. The top boundary of the slice is a hard edge — pressing scroll-up at the top silently no-ops. Deeper history (beyond N) is not reachable in v1.

**No deeper-history extend.** No `r`-to-extend, no scroll-past-top trigger, no lazy load. Reaching content older than the last 1000 lines is a "full attach" affordance, not a preview affordance.

#### Read Pipeline

The read is implemented as a **tail-N idiom at the disk layer**: open the file, seek to end, read backwards in chunks until N newlines are collected, return only those bytes. Cost is decoupled from total `.bin` file size — a 3.7MB / 50k-line file reads at the same sub-millisecond cost as a small one. No full-file read, no `strings.Split` allocation of the whole file per cycle keypress.

**Implementation shape.** ~30 LOC in a new helper, packaged with `state.ScrollbackFile` resolution. Standard Go idiom — `os.File` + `Seek(0, io.SeekEnd)` + reverse chunked scan. No external dependency.

**Output handoff.** The tail bytes are passed to `viewport.SetContent(rawAnsiBytes)` as a single string. ANSI escape sequences are preserved verbatim (see *ANSI Passthrough* below). No preprocessing, no sanitisation, no re-wrap.

**Read is synchronous.** Because the read is bounded and sub-millisecond regardless of file size, the read happens inline in `Update` for the focus-changing event. No `tea.Cmd` deferral, no generation tokens, no async I/O.

### Refresh Semantics

Preview is **stateless with respect to byte content** — the disk is the canonical source of truth, and no pane content is cached in the model between focus events. Every focus-changing event triggers a fresh disk read of the newly-focused pane.

**No timer, no polling.** Reads happen only when the user acts. There is no background refresh loop, no `r`-to-refresh key, no auto-tick.

#### Read Trigger Events

A fresh tail-N read is performed when:

- **Initial preview-open (Space)** — lazy per focus: reads only the currently-focused pane (window 1 / pane 1 by reset rule). Other panes are read on first focus via `]` / `[` / `Tab`.
- **`]` or `[`** — re-reads the newly-focused pane after a window cycle.
- **`Tab`** — re-reads the newly-focused pane after a within-window pane cycle.

Chrome counts (window M of N, pane X of Y, window name) come from tmux structural enumeration captured at preview-open and **do not** force eager `.bin` reads for unfocused panes.

**Between-session step is not a trigger** because in-preview between-session stepping is not part of the feature. To preview a different session, the user `Esc`s back to the list, moves the cursor, and presses `Space` again — which fires the initial-open trigger fresh on the new session.

#### Viewport-internal Scroll Does Not Re-read

While focus is held on a pane, the viewport holds its current N lines as the active rendering buffer. Scrolling up/down within those bounds is a pure viewport operation — no disk read. The "no content cache" framing applies *across focus changes*; the currently-focused viewport is the active rendering buffer, not a cache.

#### Scroll Position Resets on Focus Change

If the user scrolls 50 lines up in window 1 / pane 1, then `Tab` to pane 2, then `Tab` back to pane 1 — pane 1 re-renders at scroll-tail (default), not at scroll-offset 50. Scroll position is **ephemeral per focus session**, consistent with the position-reset rule for re-opening preview on the same session.

#### Dwell Behaviour

Sitting on one pane for an extended period shows a snapshot that grows progressively staler. There is no in-place refresh. The natural recovery is to step away and back (`Tab` away then `Tab` back, or `]` / `[` and back) — any focus change triggers a re-read.

### Read-Failure Handling

Three failure modes, all handled benignly without aborting the preview:

- **Daemon mid-write while preview reads.** Closed by atomicity: the daemon writes via `fileutil.AtomicWrite0600` (tempfile + rename), so the reader observes either the previous full content or the new full content — never torn bytes. No special handling required.
- **`.bin` deleted between two consecutive focus events** (pane killed, daemon cleanup, etc.). The viewport renders a placeholder for that pane (see *Placeholder* below). Other panes in the session continue to render normally.
- **OS-level read error** (permissions, disk full, etc.). The viewport renders a brief error string in place of content. Should never occur given mode 0600 / same-user guarantees from the save daemon, but handled defensively rather than crashing the TUI.

#### Placeholder

A single placeholder shape is used across all "no content available" cases — read failures, deleted `.bin`, and the brand-new-session edge case (covered separately below). Working label: **"(no saved content)"**.

Chrome (window M of N, pane X of Y, window name) is unaffected by content placeholders: the structural counts come from tmux enumeration, so the user always sees the correct shape with placeholders only where content is missing.

**Open spec-pinning items handed to the build phase:**
- Exact placeholder wording.
- Exact error-string wording for OS-level read failures.

### Esc Level Tree

`Esc` is **progressive**: each press discharges exactly one level of state. The user model is "Esc backs out of one thing at a time".

| Context | `Esc` action |
|---------|--------------|
| In preview | Return to Sessions list. Filter (if any) stays committed; cursor stays on the previewed session. |
| Sessions list with **committed filter** | Clear filter. Return to unfiltered list; cursor preserved. (Standard `bubbles/list` behaviour.) |
| Sessions list, **mid-typing-filter** | Cancel filter input; return to unfiltered list. (Standard `bubbles/list` behaviour.) |
| Sessions list with **no filter** | Quit Portal (or return to caller). Existing Portal behaviour. |

A typical "filter, then preview, then quit" path therefore takes **three Escs**: preview → committed-filter list → unfiltered list → quit.

### No In-preview Between-Session Stepping

Preview is bound to **one session per open**. There are no key bindings inside preview that move between candidate sessions.

**Cascading consequences (anchored here so the spec is self-contained):**

- **No between-session keymap.** Arrow keys, `j`/`k`, `n`/`p`, etc. inside preview are unbound or no-op (TBD by build phase keymap design); they do not advance to the next session.
- **List cursor sync is not a question.** Preview cannot move the underlying list cursor, so on `Esc` the cursor is exactly where it was when `Space` was pressed. There is no sync mode to design.
- **Filter set boundary is not a question.** "Stepping iterates filtered set vs all sessions" does not arise — preview never iterates.
- **Refresh trigger list does not include between-session step** (already reflected above).

**Reversibility.** Adding in-preview between-session stepping later is additive (a new keymap entry plus cursor-sync semantics), not a rewrite. It is intentionally out of scope for v1.

**Override traceability.** Earlier research had locked "in-preview between-session stepping" (Claude Code resume-style) as part of the feature shape. This spec deliberately overrides that constraint in favour of the simpler `Esc → arrow → Space` loop documented above. This is the only material deviation from the research-locked shape; recorded here so future readers (or future spec revisions) do not silently re-introduce the original assumption from the research source.

### Filter Behaviour with Preview

`bubbles/list`'s filter mode has two phases that interact differently with `Space`:

1. **Filtering** (`SettingFilter()` true) — every keypress is text input; `Space` inserts a literal space into the filter.
2. **Filter committed** (after `Enter`) — list shows the filtered set, arrows move the cursor, and `Space` is free to open preview.

**Decision: default `bubbles/list` semantics.** Preview does **not** intercept `Space` while filtering. There is no "magic Space" that commits and previews in one step. There is no second binding for "open preview" while filtering.

The user must commit the filter (`Enter`) before `Space` opens preview.

#### Canonical User Flow

1. Type filter (e.g. `pigeon`).
2. Press `Enter` to commit filter — list narrows to matches.
3. Arrow to candidate.
4. `Space` to preview.
5. `Esc` back to list — filter still committed; cursor stays in narrowed set.
6. Arrow + `Space` to preview next candidate.
7. `Esc` to clear filter, then `Esc` again to quit (per Esc level tree).

**Why this rather than magic Space.** Text input and list navigation are distinct interaction modes; `Space` changing role between them is the universal text-input convention, not an inconsistency. Magic Space would also make a literal space character impossible to type into the filter without awkward workarounds.

### Brand-new-session Edge Case

Two related "no `.bin` content yet" scenarios are handled by the same placeholder:

- **Whole-session.** A session created within the last save tick — the daemon hasn't captured any of its panes yet. Every pane reads as missing-content; preview shows the placeholder for each pane.
- **Per-pane.** A multi-pane session where one pane was just split-windowed in but the daemon hasn't ticked it yet. Other panes have `.bin` and render normally; the new one shows the placeholder.

**Behaviour.** Per-pane "(no saved content)" placeholder rendered in the viewport for any pane whose `.bin` is missing or unreadable (same shape as read-failure handling above). Other panes in the same session render normally.

**Chrome integrity.** Window M of N, pane X of Y, and window name come from tmux structural enumeration, so the user always sees the correct structure even when content is missing. Cycle keys (`]` / `[` / `Tab`) work normally and traverse all structural entries regardless of which have content.

**No live capture fallback.** Preview never falls back to `tmux capture-pane` for missing-`.bin` panes. That would contradict the always-disk decision and would only succeed for hydrated panes anyway.

### Privacy / Threat Model

**Decision: no design response.** Preview ships as a sharp tool. There is no opt-out toggle, no redaction layer, no end-user documentation about preview's exposure surface, and no automatic suppression of any session content based on heuristics.

**Rationale.** Portal is a single-user developer tool; the user is the operator and the audience. Mitigation of secret-exposure during sharing contexts (screen-shares, demos, OBS recording, pairing) is the user's responsibility — accomplished simply by not pressing `Space`. Redaction would create a false sense of security that is worse than no protection.

**Reversibility.** A future opt-out toggle (e.g. `portal config set preview-disabled true`) is additive — a config flag check at preview-open. It is intentionally not built in v1; it can be added later without rework if real users report concern.

**Build-phase consequence.** The build phase must not introduce any "safety" fallback (e.g. blocking preview on sessions whose content matches certain patterns). The feature surface is intentionally uniform.

### Cross-cutting Seams

These are integration boundaries that the build phase must respect. None require new architectural work; they document how the feature composes with existing portal subsystems.

#### Bootstrap Restore-Window Interaction

The `cmd/bootstrap` 9-step orchestrator sets `@portal-restoring` across step 5 (Restore), recreates skeleton panes with their saved `.bin` files on disk, and clears the marker in step 6. The Loading page is shown during this window; the Sessions page transitions in afterward.

Preview composes naturally with this without new constraints:

- **Loading page has no preview entry point.** `Space` is unbound at the Loading page level, so there is nothing to gate.
- **Always-disk path is marker-agnostic.** Even if `@portal-restoring` lingers briefly when the Sessions page first appears, preview's always-disk read works regardless of marker state. Skeleton panes have `.bin` files on disk *because* they're being restored from saved bytes — those are exactly the bytes hydrate will replay. Reading them is harmless and informative.
- **Inside-tmux invariant excludes the current session.** Brand-new sessions during restore that lack `.bin` simply hit the per-pane placeholder.

No new bootstrap steps, no preview-aware bootstrap gating, no marker checks in the read pipeline.

#### Externally-Killed Session During Preview

If session A is killed externally (another tmux client, `tmux kill-session`, `portal clean`) while the user holds preview open on it:

- **Content reads.** The `.bin` files persist briefly, then get cleaned by the daemon. Read-failure / deleted-`.bin` is already covered by the placeholder behaviour. `]` / `[` / `Tab` will increasingly land on placeholders as files are cleaned.
- **Chrome.** Window/pane structural counts and names were captured at preview-open. Cycle keys cycle the captured shape; no live re-enumeration is performed mid-preview, so chrome stays stable.
- **Esc back to list.** The Sessions list re-fetches the live session list on return — the killed session simply isn't there anymore. Cursor lands on a neighbouring session via `bubbles/list`'s default behaviour.

No new graceful-degradation logic is required; the pieces already pinned (placeholder, chrome captured at open, list re-fetch) handle this case.

#### `_portal-saver` Self-Reference

The `_portal-saver` detached session that hosts `portal state daemon` must not appear in the Sessions list at all. Existing list-population logic and the inside-tmux invariant should already exclude it.

The build phase must **confirm** during implementation that `_portal-saver` is excluded from the list passed to the Sessions page; if any code path ever leaks it into the list, no preview-layer suppression is required as long as the list filter is fixed at its source. Preview itself does not introduce a name-based blacklist.

#### ANSI Passthrough vs Viewport Width

The read pipeline is straight passthrough — `viewport.SetContent(rawAnsiBytes)` with no preprocessing, no sanitisation, no re-wrap.

Behavioural consequences from `bubbles/viewport` and tmux output shapes:

- **`bubbles/viewport` does not auto-wrap.** Long lines are handled by ANSI-aware horizontal cut (`ansi.Cut`) when content is wider than the viewport.
- **tmux `capture-pane -e` output is hard-wrapped.** Output is wrapped to the source pane's display width at capture time.

Resulting render behaviour:

- If preview viewport ≥ source pane width: no truncation, full lines rendered.
- If preview viewport < source pane width: clean ANSI-aware horizontal cut; no corruption, no garbled escape sequences.

The build phase must not introduce wrapping, sanitisation, or escape-stripping in the read pipeline.

#### State Package API Reuse

Preview's read pipeline uses the existing `state` package surface:

- **`state.ScrollbackFile(stateDir, paneKey)`** — resolves the on-disk path. Reused unchanged.
- **Pane-key resolution** — preview must look up structural pane keys consistent with how the daemon writes them (see `internal/state` paneKey helpers).
- **Tail-N read helper** — new addition (~30 LOC), packaged in `internal/state` alongside `ScrollbackFile`. Naming and exact placement TBD by build phase.

No new methods on `tmux.Client`. No new daemon work. No marker mutations. No hook firing. The feature is purely additive on top of existing surfaces.

### Architecture Summary

The feature decomposes into a small, well-bounded set of additions:

**Read pipeline** (in `internal/state`)
- Reuse: `state.ScrollbackFile(stateDir, paneKey)` for path resolution.
- New: tail-N read helper (~30 LOC) — `os.File` + `Seek(0, io.SeekEnd)` + reverse chunked scan to collect last 1000 lines.
- Output: raw ANSI bytes, straight passthrough to `viewport.SetContent`.

**Page state machine** (in `internal/tui`)
- New: `pagePreview` arm in the page state machine, peer of `pageFileBrowser`.
- New: preview model owning a `bubbles/viewport`, current focus indices (window index, pane index), structural enumeration captured at open, and minimal keymap.
- Modified: `updateSessionsPage` binds `Space` to open preview.

**Within-preview keymap**
- `]` next window, `[` previous window (both wrap).
- `Tab` next pane within window (forward-only with wrap).
- Viewport's native scroll keys (passed through to the embedded `bubbles/viewport`).
- `Esc` returns to Sessions list.

**Chrome rendering**
- Computed from tmux structural enumeration (`list-panes -F`-style call) at preview-open.
- Floor: window M of N + pane X of Y + window name + keystroke hints.

**No changes to:**
- `tmux.Client` (no new commands).
- `state` daemon (no new save/replay logic).
- `restore` engine.
- `cmd/bootstrap` orchestrator.
- `hooks` store or hydrate helper.
- Save format or `.bin` file shape.

### Out of Scope (v1)

Documented for traceability. Each is reversible — adding any of these later is additive, not a rewrite.

- **Live capture** (`tmux capture-pane` for hydrated panes). Marker-branched read path is not built; could be added as a per-pane source override later.
- **Literal `window_layout` rendering.** Sequential window-grouped is the only shape; layout parser is not built.
- **In-preview between-session stepping.** No keymap to advance to the next/previous candidate session without dismissing.
- **Deeper history beyond N=1000 lines.** No `r`-to-extend, no scroll-past-top trigger, no lazy load.
- **Auto-refresh / live tail / timer-driven reload.** Reads happen only on focus change.
- **Position memory across re-opens.** Re-opening preview on the same session resets to window 1 / pane 1.
- **Per-pane current-command in chrome** (e.g. `nvim`, `claude`).
- **Pane position hint in chrome** (e.g. `(top-right)`).
- **Privacy / threat-model design response.** No opt-out toggle, no redaction layer, no documentation gating.
- **Preview entry point on Projects or FileBrowser pages.**
- **Preview-layer `_portal-saver` suppression** (excluded at list-population layer instead).

### Open Items Handed to the Build Phase

These are spec-pinning decisions intentionally deferred to implementation:

- **N value pinned at 1000** by this spec; build may surface as a named constant.
- **Chrome layout details** — exact wording, header vs footer placement, single-line vs two-line.
- **Placeholder wording** — working label is "(no saved content)".
- **Error string for OS-level read failures** — short, viewport-renderable.
- **Tail-N helper name and exact package location** within `internal/state`.
- **Confirm `_portal-saver` exclusion** is already applied at the Sessions-list source. If not, fix at the source — not in preview.
- **Within-preview keymap collisions** — confirm `]`, `[`, `Tab` do not collide with any inherited `bubbles/viewport` or page-level bindings; if they do, preview's binding wins inside the preview page.

---

## Working Notes
