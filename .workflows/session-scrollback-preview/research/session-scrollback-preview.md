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

**Leaning (post-conversation):** sub-page, with in-preview navigation between candidate sessions (i.e. step from one preview to the next without exiting back to the list). User confirmed the failure mode is *scanning through several lookalikes*, not inspecting one in isolation. Cited prior art: Claude Code's resume-session picker, which uses a full-screen preview with arrow-key stepping across recent sessions. That hybrid — full-screen preview + in-preview cursor — captures the "scan several" use case without paying the column-view cost (wide-terminal requirement, layout sizing battles with `bubbles/list`).

This shifts the next fork from *"which shape?"* to *"how does in-preview navigation behave?"* — see T1a.

### T1a. In-preview navigation (only relevant if T1 settles on sub-page-with-stepping)

If preview supports stepping across candidates without returning to the list, several sub-decisions:

- **Stepping key** — arrow up/down (matches Claude Code), Tab/Shift-Tab, or j/k (vim-style). Arrows are the obvious default; j/k is a free shortcut that doesn't conflict because preview owns its keymap.
- **List cursor sync** — when the user steps from session A to session B inside the preview, does the underlying list cursor move too? Two options:
  - **Sync on step**: Esc returns to whichever session was *last previewed*. Simplest mental model for the user — preview navigation is just "moving the list cursor with extra rendering".
  - **No sync**: Esc returns to the original highlighted session; preview navigation is a transient inspection. More complex but preserves "where you were".

  Sync-on-step is clearly cheaper to implement and matches the Quick Look mental model.
- **Wraparound** — at end of list, does stepping wrap or stop? `bubbles/list` defaults to wrap-around behaviour; matching that is consistent.
- **Filtered-list interaction** — if a filter is active on the sessions page, in-preview nav should step through *filtered* items only, not all items. Otherwise the user could step into a session they explicitly hid.
- **Capture timing** — stepping rapidly through 6 candidates means 6 capture calls. Snapshot must run as a `tea.Cmd` (off the event loop) and the result must include a generation token so a stale capture for the previous selection doesn't clobber the current view (cancel-on-step pattern). Even at 30ms per capture, holding arrow-down would visibly lag without this.

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

## Empirical Findings

### F1. `tmux capture-pane` latency (measured against 5 live sessions)

Methodology: 3 runs each per session, on the user's running tmux server, against sessions ranging from 706 to 50,330 history lines. `/usr/bin/time -p`.

| Session             | History | `-S -` (full)    | `-S -200` (last 200 lines) |
|---------------------|---------|------------------|----------------------------|
| infra-terraform     |     706 | <10ms / 44 KB    | <10ms / 13 KB              |
| portal              |  15,454 | 50–170ms / 1 MB  | <10ms / 21 KB              |
| agentic-workflows   |  28,388 | 110ms / 2.4 MB   | <10ms (after warm) / 18 KB |
| codeintel           |  50,330 | 160ms / 3.7 MB   | <10ms / 20 KB              |
| pigeon              |  49,812 | 160–290ms / 3.8 MB | <10ms / 18 KB            |

**Interpretation:**
- Full-buffer capture scales with history size — at 50k lines, ~160ms typical, occasionally 290ms under load. Stepping through 6 such sessions = ~1s of cumulative lag. **Not viable for stepping UX.**
- Last-N capture (here N=200) is essentially free (sub-10ms) regardless of total history. Output is 13–22 KB — small. **Comfortably within the stepping budget** (~60ms total for 6 sessions = imperceptible).

**Design implication (not a decision):** the stepping interaction model implies a *bounded capture* (e.g. last viewport-height ± buffer lines). If users want to look further back than the bounded snapshot, that becomes a separate read-more interaction (manual `r`-refresh, or page-down extends the capture window). Single full-buffer capture per preview-step is off the table for sessions with large history.

**Secondary observation:** even bounded capture is 70–110 bytes/line on `-e`-decorated content. A 200-line preview is ~20 KB of ANSI text — not noticeable for Bubble Tea to render, but worth knowing for memory/event-message sizing.

### F2. Page-state-machine integration cost

`internal/tui/model.go::Update` (line 785) routes by `activePage` via a switch — `PageLoading | PageSessions | PageProjects | pageFileBrowser`. Adding a fifth `pagePreview` is one `case` arm + one method (`updatePreviewPage`). Each page handler has the same shape:
- Top-of-handler check `if m.modal != modalNone { return m.updateModal(msg) }` (modal routing is per-page, not global)
- `tea.KeyCtrlC` → quit
- Page-specific keymap

The Esc-progressive-back semantics live inside each page handler (each decides what Esc means in its own context) — there is no central "esc stack" to extend. This means a preview page would own its own Esc handling: dismiss → sessions list. Clean.

Filter-typing protection pattern (`m.projectList.SettingFilter()`) lets a page conditionally swallow keys while a filter is being entered — applies the same way for preview's Space trigger on the sessions page (don't open preview while user is mid-filter).

**Cost rating:** small. The page-state machine is built for this; no architectural pressure.

### F3. `bubbles/viewport` availability and ANSI behaviour

- Already transitively available — `bubbles v1.0.0` is in `go.mod`; `viewport` is a sibling sub-package of `list`. No new direct dependency required.
- Width measurement in viewport uses `lipgloss.Width`, which is ANSI-aware (delegates to `charmbracelet/x/ansi.StringWidth`). Already used in `internal/tui/modal.go`.
- Known gotcha (Bubble Tea community): when content wraps inside viewport, ANSI sequences spanning a wrap boundary can render as literal escape characters because naive `strings.Split` on `\n` fragments styled spans. Mitigations: (a) use `tmux capture-pane -J` to join wrapped lines so the source has only logical newlines, (b) match the viewport width to the source pane width (queryable via `#{pane_width}` per pane), or (c) ANSI-aware re-wrap on display.
- **Unverified end-to-end** — the failure mode is content-dependent; a representative Claude session capture rendered through viewport would prove or disprove this. Worth a small spike before committing to the rendering approach.

### F4. `capture-pane` target resolution under "active pane" assumption

`tmux capture-pane -t <session>` (no window/pane suffix) resolves to the session's *current* (active) window's *active* pane — verified empirically against the test runs above (no errors when only session name is supplied). This eliminates the need for portal to enumerate panes and pick the active one for the v1 case where preview shows "what you'd see if you attached now". `tmux.Client.CapturePane(target)` already accepts a target string, so passing just the session name works.

For multi-pane / multi-window inspection (deferred per inbox), `ListPanesInSession` plus a per-pane format that includes `pane_active` would let portal pick or compose. Not needed for v1.

## Threads from Conversation

### T9. Preview as session metadata, not just scrollback content

User raised: "if the user has registered a hook to resume Claude Code, previewing the session would ideally show this somehow." Pushes the preview from "raw terminal pixels" to "what is this session?" — multiple data sources can answer that, with different freshness and semantics.

**Available data sources for session metadata:**

- **Current process per pane** — `tmux list-panes -F "#{pane_current_command}"` returns the foreground process name (e.g. `claude`, `nvim`, `node`). Live, cheap, single tmux call. Tells the user *what's running right now*.
- **Pane title** — `#{pane_title}` is settable by escape sequences (`OSC 0` etc.); some shells/programs set it to the current command line or context. Often empty in plain shells, occasionally rich in TUIs.
- **Registered on-resume hooks** — `internal/hooks/lookup.go::LookupOnResume(store, hookKey)` returns `(cmd, ok, err)` keyed by `session:window.pane`. Tells the user *what'll fire on the next hydrate (reboot recovery)* — **not** on every attach. Per CLAUDE.md: "hooks fire on reboot recovery, not on every detach/reattach inside the same tmux server lifetime."
- **Captured scrollback** — already covered (F1).
- **Working directory per pane** — `#{pane_current_path}`. Cheap. Disambiguates same-named sessions if they're in different sub-paths.

**Important semantic precision** for hooks display: showing `claude --continue abc123` next to a session label *without* a "(on next reboot)" qualifier could mislead the user into expecting that command to run on attach. It won't. Two framings:

- "Last hook: `claude --continue abc123` (runs on next reboot)" — explicit.
- Treat the hook as a *labelling hint* (this session is the one that'll resume conversation abc123) rather than a behavioural promise.

The second framing aligns with the user's stated need ("show this somehow" = recognise the session, not "tell me what'll happen").

**Ergonomic implication unrelated to scrollback:** session metadata may be useful even *before* preview is opened — a small inline marker in the sessions list (e.g. `*` for "has on-resume hook" or showing the current process beside the name) could disambiguate without any preview interaction at all. That's a different feature surface — possibly subsumes some of preview's job for the most common case (recognise by current process / hook). Worth flagging as an alternative *complement* to preview, not a replacement.

### T10. Alt-screen capture behaviour for TUI sessions

`capture-pane` against a pane currently running an alt-screen program (vim, htop, etc.) captures the *alt-screen* contents — the live redraw frame — not the main-screen scrollback that exists "underneath". Several open questions:

- **Does Claude Code's TUI use alt-screen?** Bubble Tea programs only use alt-screen if `tea.WithAltScreen()` is set; default is in-line render. If Claude Code is in-line, capture sees the conversation; if alt-screen, capture sees the live UI frame (which may be visually rich but not what the user reads to identify the conversation).
- **Empirical check needed** — capture against a live Claude session and inspect the output. None of the user's currently running sessions are in alt-screen (verified via `pane_in_alt_screen=` empty across the board), so this needs a deliberate test.
- **`-a` flag** — `tmux capture-pane -a` captures the *alternate* screen contents specifically when alt-screen is *not* active, or vice-versa. Doesn't directly help "give me the recent terminal output" if the active screen is the alt-screen.
- **Connecting to T9** — for sessions running alt-screen TUIs (Claude included if it uses alt-screen), the *registered hook* and *current process* metadata may be more disambiguating than the captured pixels. The preview could degrade gracefully: scrollback for shell sessions, metadata-prominent for alt-screen sessions.

This is verifiable. A small spike: start a Claude conversation in a tmux session, run `tmux capture-pane -e -p -S -200 -t <session>` and inspect what comes back. Until that's done, the rendering pipeline is shaped by an unverified assumption about what `capture-pane` returns for the most important target case (Claude sessions are the very ones the user said are hardest to disambiguate).

## Open Questions (deferred per inbox note)

- Which pane to capture when a session has multiple panes/windows — assume active-pane-of-active-window for v1.
- How much history — full vs last-N — drives whether preview is scrollable.
- Scrollable vs fixed snapshot — depends on history-depth choice and terminal size.
- ANSI rendering specifics — pass-through with `bubbles/viewport` is the leading candidate but unverified end-to-end.
