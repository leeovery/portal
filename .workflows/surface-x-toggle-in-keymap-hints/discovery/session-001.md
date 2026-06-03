# Discovery Session 001

Date: 2026-06-03
Work unit: surface-x-toggle-in-keymap-hints

## Description (as of session)

Surface the existing X page-toggle binding in the sessions and projects keymap footers (P/X, S/X) — display-only, no behaviour change.

## Seed

- seeds/2026-05-19-surface-x-toggle-in-keymap-hints.md (inbox:quickfix)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Origin is an inbox quick-fix. The TUI sessions page footer currently shows `P` as the hint for jumping to the projects page, and the projects page shows `S` for jumping back to sessions. A separate, undocumented `X` binding already toggles between the two pages from either side — functionally identical to the contextual `P` / `S` keys — but it has never been shown in the keymap footer, so it is effectively invisible to users.

The intent is purely a keymap-hint display change: on the sessions page, surface the projects-jump hint as both keys (`P/X` or the footer's existing grouped-binding rendering), and on the projects page surface the sessions-jump hint as both keys (`S/X`). The `X` bindings stay wired exactly as they are today — no behaviour change, nothing to diagnose, no design debate. User confirmed this framing.

Relevant code lives in the key-hint footer rendering for the sessions and projects pages under `internal/tui/`.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
