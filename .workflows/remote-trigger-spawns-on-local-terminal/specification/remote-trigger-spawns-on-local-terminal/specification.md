# Specification: remote-trigger-spawns-on-local-terminal

## Specification

## Background

Portal's multi-window spawn burst (both the TUI multi-select picker burst and the CLI multi-target `portal open <a> <b> …` N≥2 burst) resolves the host terminal to spawn windows into through a single shared gate: `spawn.Detector.Detect()`. When Portal runs **inside tmux**, `Detect()` cannot walk its own process ancestry (that leads to the tmux server, not the launching terminal), so it enumerates the clients attached to the triggering pane's session and walks each client's process tree to decide whether a host-local terminal is present.

## The Bug

When a spawn burst is triggered from a **remote** tmux client (e.g. Blink on iPhone/iPad over SSH/mosh) while a **host-local** terminal client (e.g. Ghostty on the Mac) is *also* attached to the same session, the N−1 spawned windows open on the host-local terminal — a screen the triggering user is not at. The trigger window self-attaches to the Nth session on the remote side, so it reads as a partial/confusing success while host windows silently accumulate on the machine at home (Portal never tears down host windows, so they linger until closed manually).

The expected behaviour: a remote-triggered burst must resolve **unsupported** and take the same atomic no-op as the pure-remote case — windows must never open on a machine/display the triggering user isn't at.

**Precondition (when it fires):** a remote triggering client **plus** at least one host-local client attached to *the same session* at detection time. If the local client is on a different session it isn't enumerated → clean NULL → correct no-op. The precondition is natural: a `tmux attach` with no `-t` lands on the most-recently-used session, so a remote + local client commonly mirror one session.

## Root Cause

`detectInsideTmux` (`internal/spawn/detect_inside.go`) decides host-terminal locality in the **wrong order**. It treats client *locality* as a pre-filter (drop every remote/mosh client whose walk resolves NULL) and client *activity* (`client_activity`) as a tiebreak applied only **among the surviving local clients**. It therefore answers *"is there any host-local client attached to the triggering session?"* rather than *"is the client that triggered this burst host-local?"*

In the mixed case the remote (triggering) client — which has the highest `client_activity` — is dropped by the NULL walk *before* its activity is ever consulted, so `best` becomes the local client, `Detect()` returns that non-NULL identity, and the burst treats the host terminal as supported.

The correct discriminator — *"which client is the user acting through?"* — is exactly the most-recently-active client on the session (tmux's own `server_client_best` heuristic). The code already has that signal but applies it *after* locality instead of *before*, so the one client whose locality actually matters is discarded first.

**Validated mechanism:** `client_activity` tracks a client's **sent input**, not the **received redraws** it gets from mirroring another client's session. A trigger keystroke on the remote client bumps only the remote's activity; a passively-mirroring local client stays stale. So "most-active client on the session" reliably fingers the remote trigger.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
