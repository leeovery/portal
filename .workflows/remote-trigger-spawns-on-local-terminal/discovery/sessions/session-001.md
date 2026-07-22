# Discovery Session 001

Date: 2026-07-22
Work unit: remote-trigger-spawns-on-local-terminal

## Description (as of session)

Remote-triggered multi-window spawn burst opens its windows on a host-local terminal instead of taking the honest unsupported no-op, so windows land on a machine the triggering user isn't at; fix covers both the TUI multi-select and the CLI multi-target `open` burst surfaces.

## Seed

- seeds/2026-07-15-remote-trigger-spawns-on-local-terminal.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Originated from an inbox bug capture. The symptom: firing a multi-window spawn burst from a **remote** tmux client (e.g. Blink on iPhone/iPad connected to the Mac over SSH/mosh) while a **host-local** terminal client (e.g. Ghostty on the Mac itself) is also attached to the same server opens the N−1 spawned windows on the Mac's local terminal — a screen the triggering user isn't looking at. The trigger window does self-attach to the Nth session, so from the remote side it reads as a partial/confusing success while host windows silently accumulate on the desk.

The condition is specific and mixed: it needs a remote triggering client *plus* at least one host-local client attached at detection time. The pure-remote case (no local client) already resolves NULL and correctly takes the honest unsupported no-op, so that path is fine — the mixed case is the defect.

Hypothesised mechanism (surfaced in the capture, not yet validated): inside-tmux detection in `internal/spawn/detect_inside.go` enumerates `list-clients`, filters to host-local clients (remote/mosh clients walk to a NULL identity and are excluded), and uses `client_activity` only as a tiebreak among locals. So detection answers "does any host-local terminal exist on this server?" rather than "is the client that *triggered* this burst local?" — the trigger's own locality never gates the decision.

Expected behaviour: when the triggering client is remote, the burst should resolve unsupported and take the same atomic no-op as the pure-remote case, even when local clients are attached. Windows should never open on a machine/display the triggering user isn't at.

Scope confirmed with the user: cover **both** burst trigger surfaces — the TUI multi-select picker burst and the CLI multi-target `portal open <a> <b> …` (N≥2) burst — since the CLI-verb redesign re-homed the burst under `open`, sharing the identical `detect_inside.go` gate. The broader question of ever supporting mobile terminals as spawn targets was judged infeasible elsewhere (no host→device control channel) and is independent of this — this is about the detection locality gate, not adding a Blink adapter.

The user is unsure whether the bug still reproduces after recent spawn-related bugfixes but believes it does. That current-reproduction check is the first thing the investigation phase should settle — confirm the wrong-machine spawn still reproduces against current code before tracing/changing the locality logic. (Note: `spawn --detect` retired into `portal doctor`'s host-terminal line under the redesign, so the diagnostic surface for this is now `doctor`.)

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to investigation.
