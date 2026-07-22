# Investigation: Remote Trigger Spawns On Local Terminal

## Symptoms

### Problem Description

**Expected behavior:**
When the client that *triggered* a multi-window spawn burst is remote (e.g. Blink on iPhone/iPad over SSH/mosh), the burst should resolve **unsupported** and take the same atomic no-op as the pure-remote case (`⚠ no host-local terminal — nothing opened`) — even when host-local clients are attached to the same tmux server. Windows must never open on a machine/display the triggering user isn't at.

**Actual behavior:**
Firing a multi-window spawn burst from a remote tmux client while a host-local terminal client (e.g. Ghostty on the Mac) is *also* attached to the same server opens the N−1 spawned windows on the host-local terminal — a screen the triggering user isn't looking at. The trigger window self-attaches to the Nth session, so from the remote side it reads as a partial/confusing success while host windows silently accumulate on the desk at home.

### Manifestation

- N−1 host-terminal windows silently open on the local Mac terminal (Ghostty) when the burst is triggered from a remote client.
- The remote (trigger) window *does* self-attach to the Nth session, so the remote user sees a partial success with no indication the other windows went elsewhere.
- Portal deliberately never tears down host windows it opened, so the misplaced windows linger until manually closed.

### Reproduction Steps

1. On the Mac, leave a host-local terminal client (Ghostty) attached to the tmux server.
2. Connect from a remote client (Blink on iPad over SSH/mosh) to the same tmux server.
3. From the remote client, trigger a multi-window spawn burst (N≥2) via **either** surface:
   - TUI multi-select picker burst (`m` → select ≥2 → Enter), or
   - CLI multi-target `portal open <a> <b> …` (N≥2).
4. Observe: the N−1 spawned windows open on the Mac's local Ghostty, not on / not blocked for the remote client.

**Reproducibility:** Believed reproducible (mixed remote-trigger + local-client-attached case). User is unsure whether it still reproduces after recent spawn-related bugfixes but believes it does — **confirming current reproduction against current code is the first investigation task.**

### Environment

- **Affected environments:** Local (developer's real macOS + tmux setup); the user routinely leaves a local terminal attached on the Mac and connects from Blink on iPad.
- **Browser/platform:** macOS host; remote clients over SSH/mosh (Blink); host-local terminal Ghostty.
- **User conditions:** The **mixed case** — a remote triggering client PLUS at least one host-local client attached to the same tmux server at detection time. The pure-remote case (no local client) already resolves NULL and correctly no-ops, so that path is fine; the mixed case is the defect.

### Impact

- **Severity:** Moderate. Nothing is destroyed, but host windows spawn invisibly on the wrong machine and linger (Portal never tears down host windows), and the triggering user gets no indication anything went to the wrong place.
- **Scope:** Real-world for this user's routine remote workflow. Both burst trigger surfaces affected.
- **Business impact:** Trust / correctness — silent wrong-machine action.

### References

- Seed: `.workflows/remote-trigger-spawns-on-local-terminal/seeds/2026-07-15-remote-trigger-spawns-on-local-terminal.md` (inbox:bug)
- Discovery session: `.workflows/remote-trigger-spawns-on-local-terminal/discovery/sessions/session-001.md`
- Diagnostic surface (post cli-verb redesign): `portal doctor` host-terminal line (`spawn --detect` retired).

---

## Analysis

### Hypotheses

**Checkpoint depth:** {TBD at investigation-plan step}

{Live ledger — statuses evolve through the analysis:}
- **Inside-tmux detection (`internal/spawn/detect_inside.go`) answers "does any host-local terminal exist on this server?" rather than "is the triggering client local?"** [suspected]
  Basis (from discovery, unvalidated): detection enumerates `list-clients`, filters to host-local clients (remote/mosh clients walk to a NULL identity and are excluded), and uses `client_activity` only as a tiebreak *among locals*. The remote trigger's own locality never gates the decision, so the burst resolves to whatever local terminal happens to be attached and drives it.

### Code Trace

**Entry point:**
{TBD}

**Execution path:**
{TBD}

**Key files involved:**
- `internal/spawn/detect_inside.go` - hypothesised location of the locality gate
- `cmd/open_burst.go` / `cmd/open_burst_run.go` - CLI multi-target burst surface
- `internal/tui` (multi-select) - picker burst surface

### Root Cause

{TBD}

### Contributing Factors

{TBD}

### Why It Wasn't Caught

{TBD}

### Blast Radius

{TBD}

---

## Fix Direction

{Written only after the fix discussion concludes.}

---

## Notes

- Scope confirmed in discovery: cover **both** burst surfaces (TUI multi-select picker burst and CLI multi-target `portal open` N≥2 burst) — they share the identical `internal/spawn` detection gate.
- Out of scope: adding a mobile-terminal (Blink) spawn adapter — judged infeasible elsewhere (no host→device control channel). This bug is about the detection locality gate only.
