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
