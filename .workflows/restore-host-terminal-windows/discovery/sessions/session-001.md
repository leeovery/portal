# Discovery Session 001

Date: 2026-06-29
Work unit: restore-host-terminal-windows

## Description (as of session)

Portal restores the host-local terminal layer after reboot: track which terminal windows were attached to which sessions on this machine, then re-spawn and re-attach them on demand. macOS Spaces placement is out of scope.

## Seed

(none)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Origin: the user's machine crashed with ~32 Claude sessions running under Portal/tmux. On reboot, Portal's resurrection layer worked well — the loading screen showed sessions resuming and ~28 of 32 reattached and resumed their Claude sessions correctly. A small number (~4) did not resume, suspected to be old sessions or a gap in the hook/Portal logic.

The limitation that motivates this work: resurrection restores the tmux/server layer, but not the **host-local terminal layer**. The user works across ~14 macOS Spaces, one project zone per Space, each window holding a few sessions. After reboot they had to manually rebuild every window — open Ghostty, Cmd+N, drag to the right Space, press X, navigate to the project, preview to confirm, attach — roughly an hour to rebuild working state.

The shaped intent: Portal tracks what was actually attached **on this host** when it last ran (distinct from tmux-level attachment, which a mobile or other client could also hold), then on demand re-spawns those terminal windows and re-attaches them to their sessions, so the user doesn't rebuild windows by hand.

Scope boundary: the user explicitly chose to bite this off in chunks — "reopen the windows" is its own job. The macOS Spaces placement (dropping each reopened window back onto its original Space) is deferred as a separate, very Mac-specific follow-up and is out of scope here.

Feasibility is genuinely uncertain ("I have no idea if it's possible") — both for distinguishing host-vs-other-client attachment and for programmatically spawning + attaching terminal windows — which is expected to pull the first phase toward research.

Two tangential items surfaced for the inbox rather than scope creep: the macOS Spaces placement follow-up, and the ~4 sessions that didn't resume on reboot.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to research.
