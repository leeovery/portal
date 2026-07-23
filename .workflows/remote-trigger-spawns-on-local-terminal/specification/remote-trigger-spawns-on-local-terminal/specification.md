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

## The Fix: Gate Locality on the Triggering (Most-Active) Client

`detectInsideTmux` (`internal/spawn/detect_inside.go`) must select the **triggering client first, then locality-check that single winner** — inverting the current filter-then-tiebreak order.

1. Enumerate the clients attached to the triggering pane's session (unchanged — `ListClients(session)`).
2. Select the **triggering client** = the client with the highest `client_activity` across **all** enumerated clients (local and remote alike). On an exact activity tie, the **first-listed** client wins (deterministic rule; preserves the existing multi-local tie-break behaviour).
3. Walk **only that winner's** process tree and branch on the result:
   - Winner resolves to a local `.app` bundle → **drive it** (supported host terminal — return its `Identity`).
   - Winner walks to a clean NULL (remote/mosh — ancestry never reaches a local `.app`) → **honest no-op** (NULL identity, nil error — the same atomic no-op as the pure-remote case).
   - Winner's walk fails transiently (`ps`/`defaults` error) → **NULL + transient error** (`ErrDetectTransient`-wrapped), so `Detect()` emits a `spawn` WARN and folds to the unsupported no-op. **Never open windows on uncertainty.**
4. **Empty client list** (no clients on the session) → clean NULL, nil error (no winner to select).

This selects the client the user is acting through and gates the burst on *that* client's locality: a remote trigger → no-op (bug fixed); a local trigger → drives (legitimate local spawn preserved). The change is behaviourally *"sometimes no-op where it used to drive, never drive where it shouldn't"* — no new false-drive is possible.

### Behavioural outcomes by scenario

| Scenario | Selected winner | Result |
|---|---|---|
| Pure remote (no local client on session) | remote | NULL → no-op (unchanged) |
| Single local client (developer at desk) | local | drive (unchanged) |
| **Mixed: remote most-active, local idle** | remote | **NULL → no-op (bug fixed)** |
| **Mixed: local most-active, remote idle** | local | **drive (legitimate local spawn preserved)** |
| 2+ all-local clients | highest-activity local (first-listed on tie) | drive that one (unchanged) |
| Winner's walk transient-fails | — | NULL + WARN (fail-safe) |
| Empty client list | — | clean NULL (unchanged) |

### Implementation approach

Compute the most-active winner over the **existing `ListClients(session)` enumeration** — it already returns each client's PID and `client_activity` — selecting the max-activity client (first-listed winning an exact tie) in Go, then walking **only that winner**. This reuses the data already fetched (no extra tmux round-trip) and keeps the existing `clientLister` DI seam and the `detectInsideTmux(session, lister, walker, reader)` signature intact, so the unit tests and their deterministic tie-break assertions remain meaningful. Delegating to tmux's own best-client resolution (`display-message -p '#{client_pid}'`) was considered and rejected: it adds a round-trip, cannot expose a controllable tie-break, and would restructure the seam.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
