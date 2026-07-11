---
status: in-progress
created: 2026-07-11
cycle: 1
phase: Input Review
topic: restore-host-terminal-windows
---

# Review Tracking: Restore Host Terminal Windows - Input Review

## Findings

### 1. Rollback "close the windows" mechanism is undefined (no adapter close primitive)

**Source**: Discussion §3 "Spawn, then self-attach LAST" and the all-or-nothing decision — *"Any fails → **roll back**: close the windows that opened (safe — it just detaches the client; the tmux sessions persist)"*; cross-referenced against §8 Adapter Contract (*"Adapters implement exactly one capability in scope: open-window-with-command"* — no close capability).
**Category**: Gap/Ambiguity
**Affects**: `Burst & Partial-Failure Contract` (rollback), `Adapter Contract & Extensibility`

**Details**:
The all-or-nothing contract is load-bearing: on a post-pre-flight spawn failure the picker must "close the windows that opened." Both the discussion and the spec describe the *effect* ("it detaches the client; the tmux sessions persist") but neither defines the *mechanism*. The adapter's only in-scope capability is `OpenWindow(command)` — there is no close/kill primitive, and the picker doesn't own the spawned host windows' processes (they're separate host windows launched via `osascript`). So how the picker targets and closes a specific already-confirmed spawned window is unspecified.

The implied mechanism seems to be "detach the tmux client for each spawned session" (the `@portal-spawn-*` markers know which sessions were spawned), relying on the host window to close when its `portal attach → tmux attach` command exits on detach. But that cleanliness *depends on the terminal closing the window when the command exits* — if a terminal keeps the window open after command exit, rollback leaves a leftover window with a dead shell, violating the net-N-windows anti-requirement. This dependency is not stated. This is a blind spot in the sources, surfaced because it is a concrete build-time hole in a core contract (worth confirming rather than discovering during implementation).

**Proposed Addition**:
(Leave blank until discussed — likely: specify the rollback close mechanism, e.g. "the picker detaches the tmux client for each confirmed spawned session (targeting via the batch's `@portal-spawn-*` set); window teardown relies on the host terminal closing the window when its attach command exits," plus a note on the per-terminal close-on-command-exit assumption as a build-time residual.)

**Resolution**: Pending
**Notes**:

---

### 2. Ghostty `wait after command` surface-configuration property dropped from the validated API shape

**Source**: Discussion §12 "Validated live" — *"On 1.3.1, `Ghostty.sdef` exposes `new window` + a `surface configuration` record with a **`command`** property ('Command to execute instead of the configured shell') + `wait after command`."*
**Category**: Enhancement to existing topic
**Affects**: `Dependencies, Deferred Scope & Build-Time Residuals` (Ghostty AppleScript API residual); relevant to `Spawn Architecture` window lifecycle

**Details**:
The discussion's live validation of the Ghostty AppleScript API recorded two properties on the `surface configuration` record: `command` **and** `wait after command`. The spec's Build-Time Residuals note reproduces the validated shape but keeps only `command`, dropping `wait after command`. That property controls whether Ghostty keeps the spawned window open after its command exits — directly relevant to the window's post-detach lifecycle and to the rollback-closes-window behaviour in Finding 1 (whether detaching leaves a dead-shell window vs. closing it). It is a concrete validated fact the Ghostty-adapter implementer would want; its omission loses that knowledge.

**Current**:
> - **Ghostty AppleScript API** is a preview API (may churn in 1.4) — pin/watch. Real shape (validated on 1.3.1): make a `surface configuration` record with a `command` property, then `new window` with it.

**Proposed Addition**:
(Leave blank until discussed — likely: restore mention of the `wait after command` property alongside `command`, e.g. "make a `surface configuration` record with a `command` property (and a `wait after command` property governing post-command window persistence), then `new window` with it.")

**Resolution**: Pending
**Notes**:

---
