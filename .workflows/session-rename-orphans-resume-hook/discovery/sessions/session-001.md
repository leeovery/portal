# Discovery Session 001

Date: 2026-06-30
Work unit: session-rename-orphans-resume-hook

## Description (as of session)

Renaming a tmux session silently orphans its Portal resume hook, so the
session reboots as a bare shell instead of resuming after reboot.

## Seed

- seeds/2026-06-30-session-rename-orphans-resume-hook.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Shaping opened from a single inbox bug. The symptom: renaming a tmux
session with `tmux rename-session` silently breaks its Portal reboot
resume — the session keeps running fine, but after the next restart it
comes back as a bare shell instead of resuming what was running. The
seed already traces a clean mechanism: resume hooks in `hooks.json` are
keyed by the structural key `session_name:window.pane`, so a rename
changes the session name, the stored hook key no longer matches the live
pane, stale-hook cleanup deletes the now-unmatched key, and because the
inner process never restarted nothing re-registers under the new name.
Live evidence from 2026-06-30 backs this: of 24 live sessions exactly
two lacked a resume hook, and both were the two that had been renamed.

The user raised the one genuine shape question: is this a bug or a
feature change? Resolved toward **bugfix**. The symptom is a malfunction
of an encouraged workflow (rename breaks behaviour that otherwise works),
not a missing-by-design capability — it "used to work" right up until the
rename. What made it feel feature-ish is that the fix direction ("a
stable, rename-immune session identity") is an architectural change to
how sessions are keyed, carrying design choices (what the stable id is,
how existing `hooks.json` entries migrate, how it sits against the
structural-key scheme used elsewhere). But that is the means, not the
deliverable; the deliverable is restoring correctness, and the root cause
is already established with a crisp correctness target rather than a wide
open design space. The fix's design choice still gets a home in the spec
phase. Validation of the suspected mechanism and shaping of the fix were
explicitly left to the downstream phases (investigation → specification).

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to investigation.
