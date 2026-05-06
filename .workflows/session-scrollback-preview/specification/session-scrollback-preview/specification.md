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

**Scroll within bounds.** The viewport (`bubbles/viewport`) renders the tail by default; the user can scroll up within the loaded N lines using the viewport's native scroll keymap. The top boundary of the slice is a hard edge — pressing scroll-up at the top silently no-ops. Deeper history (beyond N) is not reachable in v1.

**No deeper-history extend.** No `r`-to-extend, no scroll-past-top trigger, no lazy load. Reaching content older than the last 1000 lines is a "full attach" affordance, not a preview affordance.

#### Read Pipeline

The read is implemented as a **tail-N idiom at the disk layer**: open the file, seek to end, read backwards in chunks until N newlines are collected, return only those bytes. Cost is decoupled from total `.bin` file size — a 3.7MB / 50k-line file reads at the same sub-millisecond cost as a small one. No full-file read, no `strings.Split` allocation of the whole file per cycle keypress.

**Implementation shape.** ~30 LOC in a new helper, packaged with `state.ScrollbackFile` resolution. Standard Go idiom — `os.File` + `Seek(0, io.SeekEnd)` + reverse chunked scan. No external dependency.

**Output handoff.** The tail bytes are passed to `viewport.SetContent(rawAnsiBytes)` as a single string. ANSI escape sequences are preserved verbatim (see *ANSI Passthrough* below). No preprocessing, no sanitisation, no re-wrap.

**Read is synchronous.** Because the read is bounded and sub-millisecond regardless of file size, the read happens inline in `Update` for the focus-changing event. No `tea.Cmd` deferral, no generation tokens, no async I/O.

---

## Working Notes
