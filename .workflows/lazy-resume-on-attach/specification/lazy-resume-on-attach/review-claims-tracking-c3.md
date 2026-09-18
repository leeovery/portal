# Review Tracking: Lazy Resume On Attach - Claims Verification

## Findings

### 1. `portal open` does not run bootstrap every time

**Source**: Tree measurement — `awk 'NR>=104 && NR<=120' cmd/root.go`; `tmux show-options -s | grep portal-bootstrapped`
**Category**: Source defect
**Move**: route
**Affects**: §7.3 (the freeze is held by the pane, not by the pane's position), §9.1 (what was measured and needs nothing)

**Problem**:
The specification has the reader believe that every `portal open` re-runs Portal's ten-step bootstrap — that restore runs inside it on each open, and that the stale-marker sweep therefore reaches a waiting pane "constantly". Against the tree, an `open` on a server already bootstrapped by the running binary version returns after a saver-liveness check and runs no orchestrator at all: no restore, no stale-marker sweep, no orphan-FIFO sweep. Those run only when the server is cold or the binary's version has moved — which is the state of the machine the feature was designed on right now (`@portal-bootstrapped 0.12.0` is set on the live server, so the next `open` of that version short-circuits).

Two things follow for the user. The sweep that is said to destroy a positionally-addressed freeze marker mid-wait actually fires at the next *full* bootstrap — the first open after a reboot or an upgrade — not on the opens the user runs all day, so the exposure that argues for carrying the freeze on the pane is real but far rarer than stated. And the assurance that a second bootstrap cannot respawn a waiting pane out from under the user is checked against a path that the ordinary warm `open` never takes: an implementer confirming that edge on a warm install exercises nothing, while the case that can actually reach a waiting pane — a full bootstrap running with forty-odd panes holding a pending decision, e.g. the first open after `brew upgrade` — goes unexercised. A pane respawned there loses both the pending decision and the transcript sitting under the panel.

**Evidence**:
Claim, §9.1: "**A second bootstrap does not disturb a waiting pane.** Every `portal open` runs the orchestrator, and restore runs inside it — but it skips any saved session whose name is already live (`internal/restore/restore.go:118`)."

Claim, §7.3: "…and the next `portal open` — which runs bootstrap, and which the user runs constantly — sweeps the marker away as stale."

Carried by the source: `.workflows/lazy-resume-on-attach/discussion/lazy-resume-on-attach.md` line 454 ("Every `portal open` runs the orchestrator, and restore runs inside it…") and line 157 ("the next `portal open` — which runs bootstrap, and which the user runs constantly — sweeps the marker away as stale"). Both sentences appear in the discussion verbatim, so the specification carries the source's claim faithfully.

Measurement 1 — the warm short-circuit, `awk 'NR>=104 && NR<=120 {printf "%d: %s\n", NR, $0}' cmd/root.go`:
```
105: 		latchSatisfied := client != nil && state.BootstrappedLatchSatisfied(client, version)
109: 		if latchSatisfied {
110: 			stateDir, _ := state.Dir()
111: 			ensureSaverLiveness(client, stateDir)
112: 			if !isTUIPath(cmd, args) {
113: 				bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
114: 			}
115: 			ctx := context.WithValue(cmd.Context(), serverStartedKey, false)
116: 			ctx = context.WithValue(ctx, tmuxClientKey, client)
117: 			cmd.SetContext(ctx)
118: 			return nil
119: 		}
```
`PersistentPreRunE` returns at line 118 before any orchestrator route (`shouldRunConcurrentBootstrap` at :123, the synchronous `runBootstrap` below it) is reached.

Measurement 2 — what satisfies the latch, `sed -n '/func BootstrappedLatchSatisfied/,/^}/p' internal/state/*.go`:
```
func BootstrappedLatchSatisfied(c RestoringChecker, runningVersion string) bool {
	val, found, err := c.TryGetServerOption(BootstrappedMarkerName)
	if err != nil { return false }
	if !found { return false }
	return val == runningVersion
}
```
and its write, `awk 'NR>=243 && NR<=252' cmd/bootstrap/bootstrap.go`:
```
243: 	emitStep(10, stepSweepOrphanFIFOs)
248: 	if o.Latch != nil {
249: 		if err := o.Latch.SetServerOption(state.BootstrappedMarkerName, o.Version); err != nil {
```
— the latch lands as the orchestrator's last action, so a completed bootstrap silences every subsequent same-version `open`.

Measurement 3 — the latch on the live install, `tmux show-options -s | grep -i portal-bootstrapped`:
```
@portal-bootstrapped 0.12.0
```

**Resolution**: Routed
**Notes**: Re-measured independently: `cmd/root.go:108-118` returns after `ensureSaverLiveness` when the latch holds, so no orchestrator, restore or sweep runs; the live server carries `@portal-bootstrapped 0.12.0`. Both conclusions leaning on the claim re-land from the corrected value unchanged — a positional marker stops matching on any rearrangement whether or not a sweep reaches it, and a waiting pane is disturbed less often rather than more — so the repair went in place in the discussion (both restatements sit outside a Decision block), and the specification's §7.3 and §9.1 were aligned to it.

---

## Observations

- Four code citations land one line short of the statement they name: §1's `cmd/state_hydrate.go:172-196` (the hook exec is at `:197`, the function runs 172-198), §7.2's `cmd/state_hydrate.go:139-148` (the exec is at `:149`), §6.4's `internal/hooks/store.go:221` (the `op=rm` INFO emission is at `:222`; `:217` carries the failed-write Warn), and §3.2's `internal/hooks/store.go:28-31` (`type Snapshot map[string]map[string]string` is at `:32`). Every one still lands the reader in the right function.
- §7.3's "closing an earlier window renumbers" holds only under `renumber-windows on`, which is the developer's setting rather than tmux's default; the paragraph's point is carried unconditionally by `break-pane`, `move-pane` and `rename-session`, all three measured in `resume-hooks-silently-lost`.
- The live install has drifted from the recorded figures since they were taken — 39 hook keys today against the 41 recorded on 2026-09-18, and 42 live sessions (41 single-pane, 1 two-pane) — which is exactly the point-in-time framing §1 states; every shape claim built on them still holds, including that all live keys are token-shaped and none old-format.
