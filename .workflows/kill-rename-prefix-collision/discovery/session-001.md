# Discovery Session 001

Date: 2026-06-09
Work unit: kill-rename-prefix-collision

## Description (as of session)

KillSession and RenameSession issue bare `-t <target>` argv without tmux's `=` exact-match prefix, risking prefix-collision that kills or renames the wrong session.

## Seed

- seeds/2026-05-16-kill-rename-prefix-collision.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Work originated from an inbox bug surfaced during review of the `enter-attaches-from-preview` work. That earlier work established that every `-t <session>` argv against tmux must use the exact-match `=` prefix, because tmux's default target resolution is prefix-match: with a live `foo-2` coexisting with a killed-then-recreated `foo`, `-t foo` silently binds to `foo-2`. Five sites (`has-session`, `select-window`, `select-pane`, `switch-client`, `attach-session`) were fixed at that time.

Two destructive callers were left bare and carry the same hazard: `KillSession(name)` (`kill-session -t <name>`) and `RenameSession(oldName, newName)` (`rename-session -t <oldName>`). The kill path is the dangerous one — destructive with no undo, and a wrong-session kill is silent. Confirmed during shaping that both are still un-fixed in `internal/tmux/tmux.go` (KillSession ~line 353, RenameSession ~line 362), and that the `=` prefix is consistently applied at the five named sites (has-session lines 136/166, switch-client 378, PaneTarget formatter 547, line 936). No `exactTarget` centralising helper exists yet.

The shape is a clear bugfix: a known malfunction mode with the root cause already located — root-cause confirmation and the fix belong to the investigation phase onward. An open scope question carried forward (not decided here): whether to introduce the optional centralising `exactTarget` helper and optionally migrate existing inline-`=` sites onto it to prevent drift, versus a minimal two-caller patch. Out of scope per the seed: `PaneTarget` (the no-prefix hooks.json key formatter) must stay as-is — changing it would silently invalidate existing hook entries.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
