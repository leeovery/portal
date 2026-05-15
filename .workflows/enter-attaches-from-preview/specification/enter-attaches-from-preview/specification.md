# Specification: Enter Attaches From Preview

## Overview

Add an `Enter` binding to the scrollback preview page that attaches to the previewed session, honouring any `(window, pane)` focus the user navigated to inside preview. Today preview's `Update` handler has no `tea.KeyEnter` case — `Enter` falls through to the embedded viewport as a no-op, forcing the user to press `Esc` to dismiss and then `Enter` on the Sessions list. This feature adds the single-keystroke commit path the user's mental model expects.

This is an **intentional behaviour extension**, not a bug fix. The prior `session-scrollback-preview` specification explicitly scoped preview's owned keymap to `]`, `[`, `Tab`, `Esc`; this feature lifts `Enter` out of "everything else is unbound/no-op" as the single new exception. Preview remains a verification surface — the new Enter binding does not open the door to inheriting other Sessions-page action keys (see *Keymap expansion policy*).

### Spec scope

This specification is **additive**. The prior `session-scrollback-preview/specification.md` is referenced for unchanged surfaces (open trigger, layout, viewport, esc level tree, scrollback read pipeline) but is not edited. Specs from completed work are frozen historical records.

This specification covers:

- The new preview-page `Enter` binding and what it commits.
- The pre-select + attach sequence (`has-session` → `select-window` → `select-pane` → connector) and best-effort failure semantics.
- The session-killed-externally refresh-and-bail path with a feature-local minimal inline flash on the Sessions list.
- The discoverability update to the preview chrome line.
- The keymap expansion policy in its general form, so future binding proposals have a clear test to argue against.

---

## Enter binding behaviour

Preview's `Update` handler gains a `tea.KeyEnter` case. When the user presses `Enter` while preview is the active page, preview commits an attach to the previewed session, applying any `(window, pane)` focus the user navigated to inside preview before handing off to the existing connector path.

### What Enter commits to

Preview's `]`, `[`, and `Tab` keys already build up a real `(window, pane)` focus — they are not viewport chrome, they are navigation state. Enter treats that state as **intent**, not a viewport-rendering concept:

- **Session**: the session the preview was opened on (captured by name at preview open, not by list-row position).
- **Window**: the window index the user navigated to via `]`/`[`, defaulting to the captured window if the user did not navigate.
- **Pane**: the pane index the user navigated to via `Tab`, defaulting to the captured pane if the user did not navigate.

The user paid keystrokes to focus a specific coordinate; Enter honours those keystrokes. The "walked-away peek" scenario (user previously had pane 2 active in the session, walked away, came back, previewed, Tab'd to pane 0 to peek, then Enter) is the accepted cost: Enter lands on the last-focused preview pane rather than tmux's prior current pane. The user's most recent `]`/`[`/`Tab` press is their stated intent — if they only wanted to peek, they would have used `Esc`.

### Transition mechanics

The pre-select and attach sequence is issued **as one logical unit from preview's `Update`**. No intermediate render. No round-trip to the Sessions page. Preview does not dismiss to Sessions and then re-fire Enter — the Sessions-page `Enter` handler does not know about preview's `(window, pane)` focus, so the sequence must be authored as a single unit in preview's update.

Implementation shape (e.g. `tea.Sequence` vs a single combined connector wrapper) is a build-phase detail. The spec-level constraint is: the four-call sequence (`has-session` → `select-window` → `select-pane` → connector) must complete in order, with selects completing before the connector hands off the terminal.

---

## Pre-select + attach sequence

On `tea.KeyEnter` in preview, the following four-call sequence runs in order. Each step has defined semantics for both success and failure paths.

### 1. `tmux has-session -t <session>`

A proactive existence check, run **before** the pre-select calls.

- **Zero exit (session exists)**: proceed to step 2.
- **Non-zero exit (session gone)**: bail. The session has been killed externally (by `tmux kill-session`, `portal clean`, the daemon, or another tmux client) between preview open and Enter. Dispatch the refresh-and-bail path — see *Session-killed-externally bail path* below. Steps 2–4 do not run.
- **OS-layer error (missing binary, exec failure)** — distinct from a non-zero exit: treat as "session present" and proceed to step 2. An OS-layer error is not a tmux-state signal; the connector will fail in the same shape it would have without the check, and `EnsureServer` already validates tmux is invocable in bootstrap.

This is one extra tmux round-trip per Enter. Negligible — sub-millisecond locally, well within UI responsiveness.

### 2. `tmux select-window -t <session>:<window_index>`

Best-effort. Uses the window index preview captured at open and walked with `]`/`[`.

- **Zero exit**: proceed.
- **Non-zero exit (window no longer exists)**: log and swallow. Do not block, do not warn the user, do not abort. Proceed to step 3 (which will also fail), then step 4.
- **No re-enumeration**: do NOT call `list-panes -F` or any structural enumeration on Enter. Re-enumeration would cost a round-trip on every Enter for an edge case that is bounded and self-correcting.

### 3. `tmux select-pane -t <session>:<window_index>.<pane_index>`

Best-effort. Uses the pane index preview captured at open and walked with `Tab`.

- **Zero exit**: proceed.
- **Non-zero exit (pane no longer exists)**: log and swallow. Same shape as step 2.

### 4. Connector handoff

The existing connector path runs, unchanged:

- **Outside tmux**: `AttachConnector` issues `syscall.Exec` to hand off the process to `tmux attach-session -A -t <session>`.
- **Inside tmux**: `SwitchConnector` issues `tmux switch-client -t <session>` (two-step: create detached session if needed, then switch).

If the pre-select calls all succeeded, the connector lands the user on the focused `(window, pane)`. If the pre-selects failed (steps 2 and/or 3), tmux's last-current pane in the session wins as a natural fallback — equivalent to the pre-existing Sessions-page Enter behaviour. No regression.

### Captured coordinate freshness

Preview captures structural enumeration at preview-open and walks `]`/`[`/`Tab` purely locally — no mid-preview re-enumeration. Pre-select acts against those captured-then-walked coordinates. This model is inherited from the prior preview spec and is restated here for completeness.

### Hook firing

The pre-select sequence does not trigger any tmux hook events — `select-window` and `select-pane` are plain tmux commands. The connector handoff WILL trigger tmux's `client-attached` hook (which Portal registers to run `portal state signal-hydrate`), but post-bootstrap there are no armed `@portal-skeleton-*` panes, so `signal-hydrate` is a no-op and no on-resume hooks fire. On-resume hooks fire only inside the hydrate helper's exec chain during restore (bootstrap step 5), per `cmd/state_hydrate.go`'s `execShellOrHookAndExit`. This feature does not change hook semantics.

---

## Working Notes
