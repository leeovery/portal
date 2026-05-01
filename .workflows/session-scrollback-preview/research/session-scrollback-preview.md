# Research: Session Scrollback Preview

Quick Look-style preview of a session's scrollback from the portal open panel, so users can disambiguate similarly-named sessions (especially Claude/team-up sessions in the same project) without paying the attach/detach cost.

## Starting Point

What we know so far:
- Prompted by: TUI picker shows multiple sessions per project named `{directory}-{nanoid}` that are visually indistinguishable. User currently loops attach → realise wrong → detach → guess again. Wants Quick Look-style preview without committing to attach.
- Interaction model is borrowed from macOS Finder Quick Look — Space previews highlighted session, Enter attaches, Escape returns. Attach/switch behaviour on Enter is unchanged from today.
- Surface area: `internal/tui` (sessions page of the page state machine) and `internal/tmux` (likely a new `tmux.Client` method around `capture-pane` to read the active pane's scrollback).
- Starting point: technical feasibility — how to capture session scrollback via tmux, how to render it inside Bubble Tea (including ANSI colour), how the preview pane fits the current page state machine.
- Constraints: AI-based auto-renaming of sessions is explicitly out of scope.
- Open questions to defer (don't answer in research): which pane to capture for multi-pane/multi-window sessions, how much history, scrollable vs fixed snapshot, ANSI colour rendering approach.

---

## Existing Surface Area (already in the codebase)

The build-side primitives the feature would compose are already present — no new tmux wrapper is strictly required, only TUI work.

- `tmux.Client.CapturePane(target string) (string, error)` (`internal/tmux/tmux.go:535`) — runs `capture-pane -e -p -S - -t <target>`. Flags: `-e` preserves ANSI escape sequences, `-p` writes to stdout, `-S -` starts from the very top of the history buffer. Returns verbatim raw output. Currently used by the save-side state daemon to dump per-pane scrollback to disk during `portal state daemon`.
- `tmux.Client.ListPanesInSession(session) ([]PaneCoord, error)` (`internal/tmux/tmux.go:385`) — enumerates `(window, pane)` coords across all windows. Sorted by window then pane.
- TUI page state machine in `internal/tui/model.go` already supports four pages: `PageLoading | PageSessions | PageProjects | pageFileBrowser`. Adding a preview page (or another modal state) is structurally cheap.
- Modal overlay infrastructure already exists at `internal/tui/modal.go`: `renderListWithModal` composites `lipgloss`-styled content over the list view ANSI-aware via `charmbracelet/x/ansi`. Used for kill confirm, rename, project edit, delete project.
- `charmbracelet/x/ansi` is already an import (used in modal.go for `ansi.StringWidth` / `ansi.Truncate` / `ansi.TruncateLeft`) — ANSI-aware width and truncation are available without a new dependency.

Implication: feasibility risk is concentrated in the *rendering* and *interaction* layers, not in fetching the data.

## Threads to Explore (parked — not yet investigated)

### T1. Interaction model: modal overlay vs sub-page vs side pane

Three structurally different shapes; the rest of the feature design changes around the choice.

- **Modal overlay** — reuses the `renderListWithModal` infrastructure verbatim. Quick to build. List stays visible behind preview. Border + padding consume usable screen — preview content is bounded by the modal box, not the terminal. Best when preview is *informational* (recent ~20 lines).
- **Sub-page** (peer of `pageFileBrowser`) — full-screen preview with its own keymap. Maximum content area. Loses the surrounding list but Esc returns instantly. Matches the user's word *"preview screen"*. Slightly more code than a modal because it adds a fourth page, but the page-state machine is built for it.
- **Side pane / column view** — list on left, live preview on right (Finder Column View shape). Preview updates as cursor moves. Most ergonomic for fast disambiguation, but: (a) needs a wide terminal, (b) complicates layout sizing with `bubbles/list`'s built-in dimensions, (c) likely re-captures on every cursor move (need a debounce + cancel pattern).

### T2. Capture target & content shape

`capture-pane` needs a pane target. Plausible defaults:

- **Active pane of active window** in the session. tmux supports omitting the pane suffix (`-t <session>` resolves to the session's active pane), or we can list panes and pick the one with `pane_active = 1`.
- **Active window, all panes joined** — useful if the session is a tmux split, but visually noisy (we'd need to compose multiple captures with separators).
- **All windows + panes** — overkill for a Quick Look-style peek.

The user has explicitly deferred multi-pane handling, so v1 likely sticks with active-pane-of-active-window. Worth confirming the defer.

History depth: `-S -` (full buffer) can be 10k+ lines. For preview the *recent* state matters most. tmux supports `-S -<n>` for "n lines back from bottom". A natural cap is "what fits the preview viewport plus a small overshoot to allow scrolling". Trades off:

- Fixed snapshot — capture a fixed window (e.g. last 200 lines), no scrolling.
- Scrollable — capture more (or full) and use `bubbles/viewport` to scroll within the preview.

User has parked this. Worth noting as a fork in the road.

### T3. ANSI rendering inside Bubble Tea

`capture-pane -e` emits raw ANSI escapes. Three approaches:

- **Pass-through** — render the captured text verbatim inside the View() string. Bubble Tea writes to an ANSI-aware terminal so this *generally* works. But: any code that measures width or truncates the content (for line wrapping, viewport sizing, modal composition) must use ANSI-aware utilities. The `charmbracelet/x/ansi` package already used in `modal.go` provides these. Risk: malformed ANSI sequences or bracketed-paste markers leaking into the buffer could confuse rendering.
- **Strip ANSI** — feed plain text into a viewport. Loses colour, which is the very thing that makes Claude/team-up sessions distinguishable. Probably a non-starter.
- **`bubbles/viewport`** — Bubble Tea's idiomatic scrollable content widget. Accepts arbitrary string content (including ANSI). Combines well with the pass-through approach.

`bubbles/viewport` is the obvious primitive if scrolling is in scope. It is *not* currently a dependency — `go.mod` includes `bubbles/list` and `bubbles/textinput` but viewport is a sibling package in the same module so the import is already transitively available.

### T4. Capture timing & responsiveness

`tmux capture-pane` is fast (single process, typically <30ms even on large buffers) but synchronous. Two patterns:

- **Lazy on Space** — capture only when preview is invoked. Matches user's stated UX. Simplest. Single `tea.Cmd` returns a `previewCapturedMsg`.
- **Eager on cursor change** (column-view style) — capture on every selection change, debounced. More responsive but: more tmux process churn; need cancellation / staleness handling so a slow capture for session N doesn't render after the user has moved to session N+1.

Capture cost is bounded — even a 10k-line buffer with `-J` collapsing is small in absolute terms, but a 10ms × 60 keypress/sec arrow-hold is real cost. Mitigation is debounce + cancel-on-select.

### T5. Inside-tmux invariant simplifies ownership

When portal runs inside tmux (the common case), the tui-session-picker spec excludes the current session from the list (`internal/tui/model.go` filters it before `SetItems`). So preview is *never* asked to capture the session whose UI is currently rendering portal. Eliminates the most awkward edge case (recursive capture, tmux client recursion) for free. Worth confirming this still holds in the v1 design.

### T6. Esc / progressive-back integration

The tui-session-picker spec defines a 4-step progressive Esc unwind: modal → filter → file browser → exit. Preview needs a slot in that order — most naturally as **layer 0** (preview dismisses before filter clears, before browser, before exit), in the same position as a modal. Confirms preview should be modelled as a modal-like overlay or a top-of-stack page, not as a sibling page.

### T7. Status / framing of the preview view

Quick Look has a header band with the file name. Preview equivalent: small header showing the session name + a footer hint `[Enter] attach  [Esc] back  [r] refresh`. Trivial to render. Worth keeping the framing minimal so terminal width does not steal preview content.

### T8. Refresh semantics

If the user holds Space on a session running an active stream (Claude generating), the snapshot at preview time is what they see. Should preview:

- Stay frozen at the snapshot (simple — what user described)
- Live-tail (poll capture-pane every Ns)
- Provide manual refresh (`r`)

The cheapest useful v1 is snapshot + manual `r`. Live-tail adds capture churn and a polling timer — defer.

## Open Questions (deferred per inbox note)

- Which pane to capture when a session has multiple panes/windows — assume active-pane-of-active-window for v1.
- How much history — full vs last-N — drives whether preview is scrollable.
- Scrollable vs fixed snapshot — depends on history-depth choice and terminal size.
- ANSI rendering specifics — pass-through with `bubbles/viewport` is the leading candidate but unverified end-to-end.
