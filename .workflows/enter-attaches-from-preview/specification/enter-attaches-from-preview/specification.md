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

## Working Notes
