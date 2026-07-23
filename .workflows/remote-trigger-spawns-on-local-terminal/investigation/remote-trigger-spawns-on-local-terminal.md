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

**Checkpoint depth:** check-ins

{Live ledger — statuses evolve through the analysis:}

- **H1 — `detectInsideTmux` gates on "any host-local client on the current session" not "the triggering client's locality"** [**confirmed**]
  Basis: `internal/spawn/detect_inside.go` enumerates `ListClients(session)`, NULL-filters to host-local clients, and picks the highest `client_activity` *among the locals*. The triggering client's own locality never enters the decision.
  Evidence: The loop (`detect_inside.go:86-105`) walks each client; a NULL walk (remote/mosh) hits `continue` **before** the activity comparison at line 100. So in a session holding `[remote-trigger @high-activity, local-Ghostty @lower-activity]`, the remote is dropped first and `best` becomes Ghostty regardless of the remote's higher activity. The burst then drives Ghostty — the wrong machine. Order is the defect: locality is a *pre-filter*, activity is a *post-filter tiebreak among survivors*.

- **H2 — the exact repro precondition is "a host-local client shares the triggering session"** [**confirmed**]
  Basis: `tmux.ListClients` runs `list-clients -t <session>` (`internal/tmux/clients.go:29`); `session = CurrentSessionName` = `display-message -p '#{session_name}'` (`internal/tmux/tmux.go:313`, no `-t` → resolves the *triggering pane's* session from `$TMUX`/`$TMUX_PANE`).
  Evidence: tmux 3.7b `man` — "If target-session is specified, list only clients connected to that session." So detection sees exactly the clients viewing the triggering pane's session. The bug fires iff a host-local client is *currently attached to that same session* as the remote trigger. If the local client is on a different session, it isn't listed → clean NULL → correct no-op. (Precondition is real and natural: a `tmux attach` with no `-t` lands on the most-recently-used session, so a remote + local client commonly mirror one session.)

- **H3 — the triggering client IS identifiable (heuristically) — the fix gates locality on it, not a locals pre-filter** [**confirmed**]
  Basis: portal inside tmux has `$TMUX` (`socket,server_pid,session_id`) + `$TMUX_PANE` (`%N`) but **no** client id of its own.
  Evidence (read-only probe, tmux 3.7b): a plain pane-run `display-message -p '#{client_pid}'` (no `-c`) resolves to a **specific** client — and empirically it is the **highest-`client_activity`** client attached to the pane's session (`89076`@`…933` chosen over `87367`@`…151`, `14920`@`…161`). This is tmux's documented best-client-for-session resolution (`cmd_find_current_client` → `server_client_best`: most-recently-active client on the session). Since the burst is triggered by the user *acting on their client* immediately before detection (picker startup for TUI; command entry for CLI), that most-active client **is** the triggering client. The signal is a strong tmux-native heuristic, not a hard binding — activity is epoch-seconds-granular, so a local client active in the same/later second is a residual edge (arguably not "wrong machine" then). Fix shape (fix-exploration, not decided here): pick the triggering (max-activity, all clients) client first, THEN locality-check the winner — flipping the current filter-then-tiebreak order.

- **H4 — recent spawn bugfixes did not fix this** [**confirmed**]
  Basis: `detect_inside.go` has one commit ever (original impl `45010cf3`, `restore-host-terminal-windows`). The recent `spawned-window-dead-ends-on-session-exit` fix touched `ghostty.go` window lifecycle; the `persistent-no-host-terminal-banner` fix is TUI-side. Neither touched detection locality. Bug still lives in current `main`.

**Trace lines (agreed order):**
1. `detectInsideTmux` / `Detect` / `resolve` — formalise the locality gate (H1)
2. `tmux.ListClients` scoping + `CurrentSessionName` — pin repro precondition (H2)
3. `$TMUX` / `$TMUX_PANE` / `client_tty` / `client_pid` — triggering-client identifiability (H3, feasibility scouting)
4. Both surfaces share `Detect()` [done: `cmd/open_burst_run.go:158`, `internal/tui/spawn_detect.go:90`] + no detection change since (H4) [done: git log]

### Code Trace

**Entry point (two surfaces, one gate):**
Both burst surfaces resolve the host terminal through the same `Detector.Detect()`:
- CLI multi-target burst: `cmd/open_burst_run.go:158` — `id := deps.Detector.Detect()`.
- TUI multi-select burst: `internal/tui/spawn_detect.go:90` — `detector.Detect()` (run once per picker session, cached at startup).

**Execution path (inside tmux):**
1. `internal/spawn/detect.go:108` `resolve()` — `d.insideTmux()` true → reads `session, _ := d.currentSession()`.
2. `internal/tmux/tmux.go:313` `CurrentSessionName()` → `display-message -p '#{session_name}'` (no `-t`) → the **triggering pane's** session (from `$TMUX`/`$TMUX_PANE`).
3. `internal/spawn/detect.go:114` → `detectInsideTmux(session, lister, walker, reader)`.
4. `internal/spawn/detect_inside.go:74` → `lister.ListClients(session)` → `internal/tmux/clients.go:29` → `list-clients -t <session> -F '#{client_pid} #{client_activity}'` — **only** clients connected to that session.
5. `internal/spawn/detect_inside.go:86-105` — per-client loop: `walkToBundle(client.PID, …)`; a **NULL** walk (remote/mosh — ancestry never reaches a local `.app`) hits `continue` at line 98 **before** the activity comparison at line 100; a non-NULL walk sets `best` (highest activity **among survivors**).
6. `internal/spawn/detect_inside.go:107-117` — `localFound` true → returns the local client's `Identity`. `Detect()` (`detect.go:94`) returns the resolved (non-NULL) identity → the burst treats the host terminal as **supported** and drives it via the resolved adapter.

**The defect (ordering):** locality is applied as a **pre-filter** (drop remotes), and `client_activity` is only a **tiebreak among the local survivors**. The *triggering* client — the most-active client on the session, which in the repro is the **remote** one — is discarded before its locality can gate the outcome. Correct behaviour requires selecting the triggering (most-active, all clients) client **first**, then locality-checking that winner (NULL → honest no-op; local → drive).

**Key files involved:**
- `internal/spawn/detect_inside.go` — the locality gate (filter-then-tiebreak order = the bug). Lines 86-105 loop; 96-99 NULL-drop; 100-104 activity tiebreak among locals.
- `internal/spawn/detect.go` — `Detector.Detect()` / `resolve()`; folds the resolved identity into supported-vs-NULL. Shared by both surfaces.
- `internal/tmux/clients.go` — `ListClients` = `list-clients -t <session>` (session-scoped enumeration).
- `internal/tmux/tmux.go:312` — `CurrentSessionName` = the triggering pane's session.
- `cmd/open_burst_run.go:158`, `internal/tui/spawn_detect.go:90` — the two `Detect()` call sites (both surfaces).
- `cmd/doctor.go:406` — third `Detect()` consumer (`portal doctor` host-terminal line) — same resolution, so `doctor` would also misreport a driveable host terminal for a remote client with a local client attached (diagnostic parity, not a burst).

### Root Cause

`detectInsideTmux` (`internal/spawn/detect_inside.go:86-105`) decides host-terminal locality in the **wrong order**: it treats client *locality* as a pre-filter (drop every remote/mosh client) and client *activity* as a tiebreak applied only **among the surviving local clients**. It therefore answers **"is there any host-local client attached to the triggering pane's session?"** rather than **"is the client that triggered this burst host-local?"**

In the mixed case — a remote client triggers the burst while a host-local client is also attached to the same session — the remote (triggering) client is dropped by the NULL walk at line 98 *before* its higher `client_activity` can be consulted at line 100, so `best` becomes the local client. `Detect()` returns that non-NULL identity, the burst treats the host terminal as **supported**, and the N−1 windows open on the local machine the triggering user is not at.

**Why this happens:** the correct discriminator — "which client is the user acting through?" — is exactly the most-recently-active client on the session (tmux's own `server_client_best` heuristic, confirmed via the `display-message -p '#{client_pid}'` probe). The code has that signal (`client_activity`) but applies it *after* locality instead of *before*, so the one client whose locality actually matters is discarded first. The fix is to select the triggering client first (max activity across **all** clients) and locality-check that single winner (NULL → honest no-op; local → drive).

### Contributing Factors

- **Over-broad proxy baked into the original design.** The inside-tmux client-walk (introduced by `restore-host-terminal-windows`, commit `45010cf3`) correctly solved NULL-filtering for the *pure-remote* case, but framed activity as a *local-only tiebreak* — encoding the implicit assumption "if any local client is attached, the user is at a local terminal." That assumption holds for a single-user-at-the-desk model and fails for the remote-trigger-plus-idle-local-client model.
- **tmux exposes no hard triggering-client binding for a pane-run command.** A process inside a pane knows its pane (`$TMUX_PANE`) and session (`$TMUX`) but not "the client that launched me," so the design leaned on client enumeration + a heuristic. The available heuristic (most-active client) was under-used (as a tiebreak) rather than used as the primary discriminator.
- **`client_activity` is epoch-seconds-granular** — not a cause, but a residual edge the fix inherits: a local client active in the same/later second as the remote trigger could still win. Acceptable (a local terminal actively used in that second is arguably not "the wrong machine").

### Why It Wasn't Caught

- **The buggy outcome is codified as intended behaviour in a unit test.** `internal/spawn/detect_inside_test.go:133` — subtest *"it drops remote clients but still resolves a mixed local+remote client set"* — deliberately seeds `{PID:601, Activity:9999}` (remote) + `{PID:501, Activity:1}` (local) and asserts the **local** wins, with the comment *"proving the NULL-filter runs first and activity is only a local tiebreak"* and the error message *"want the local … despite the remote client's higher activity."* The mixed case was considered and the wrong answer was chosen and locked in. **The fix must invert this test's assertion** (mixed set with the remote as most-active → NULL/no-op).
- **No test models "the remote client is the user."** The suite frames remote clients uniformly as noise to filter out, never as the triggering actor. There is no "remote trigger + local bystander" scenario asserting a no-op.
- **Not reproducible without a real multi-client setup.** Reproduction needs an actual remote client (SSH/mosh) plus a host-local client on the same session — outside unit-test reach and easy to miss in manual testing (the developer is usually sitting at the local terminal, i.e. the local *is* the trigger, which resolves correctly).

### Blast Radius

**Directly affected:**
- **Both burst surfaces, inside tmux, mixed remote-trigger + host-local-client-on-same-session:**
  - CLI multi-target `portal open <a> <b> …` (N≥2) — `cmd/open_burst_run.go:158`.
  - TUI multi-select picker burst — `internal/tui/spawn_detect.go:90` (detection cached at picker startup).
- Windows open on the local machine; the trigger window self-attaches remotely (partial-success illusion); host windows linger (Portal never tears down host windows).

**Also affected (same `Detect()`):**
- **`portal doctor` host-terminal line** (`cmd/doctor.go:406`) — would report a driveable host terminal for a remote session with a local client attached. Read-only diagnostic misreport, not a spawn, but the fix corrects it in lockstep (single gate).

**Not affected:**
- **Outside-tmux detection** (`internal/spawn/detect_outside.go`) — walks the portal process's *own* ancestry (env fast-path / self-walk), which reflects the actual launching terminal. No client enumeration, no locality-ordering bug. Only the inside-tmux client-walk is defective.
- **Pure-remote case** (no local client on the session) — already resolves clean NULL → correct honest no-op. Unchanged by the fix.
- **Single-local-client case** (developer at the desk) — the trigger *is* the local client (most-active), resolves local → drives. Unchanged.

**Interaction to carry into spec (not a defect):**
- **`persistent-no-host-terminal-banner`** (spec 2026-07-22) splits detection outcomes into supported / named-unsupported / NULL-remote, dropping the persistent banner for NULL/remote and keeping it for named-unsupported. After **this** fix, a remote trigger with a local client attached resolves **NULL** (instead of the local's *named/supported* identity), so the mixed case now flows into that bug's NULL/remote branch — the two fixes compose cleanly (remote users get the honest no-op + no noise banner). That bug's own note already anticipates this ("once its trigger-locality gate is fixed, every remote login resolves NULL"). No conflict; worth a coherence check at spec time.

---

## Fix Direction

{Written only after the fix discussion concludes.}

---

## Notes

- Scope confirmed in discovery: cover **both** burst surfaces (TUI multi-select picker burst and CLI multi-target `portal open` N≥2 burst) — they share the identical `internal/spawn` detection gate.
- Out of scope: adding a mobile-terminal (Blink) spawn adapter — judged infeasible elsewhere (no host→device control channel). This bug is about the detection locality gate only.
