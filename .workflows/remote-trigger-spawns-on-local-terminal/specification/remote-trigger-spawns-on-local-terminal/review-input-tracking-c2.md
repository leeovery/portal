---
status: complete
created: 2026-07-23
cycle: 2
phase: Input Review
topic: remote-trigger-spawns-on-local-terminal
---

# Review Tracking: remote-trigger-spawns-on-local-terminal - Input Review

## Findings

### 1. Temporal premise underpinning "most-active client = triggering client"

**Source**: investigation.md — H3 (line 68): "Since the burst is triggered by the user *acting on their client* immediately before detection (picker startup for TUI; command entry for CLI), that most-active client **is** the triggering client."
**Category**: Enhancement to existing topic
**Affects**: Root Cause section (and coherently, the "cached once at picker startup" note in Scope item 2)

**Details**:
The whole fix rests on the equivalence "most-recently-active client == the client that triggered the burst." The spec establishes this via the mirroring mechanism (activity tracks sent input, not received redraws), which explains why a *passive* local mirror stays stale. But it omits the second, distinct supporting premise from H3: detection runs **immediately after the user's trigger action** — picker startup for the TUI, command entry for the CLI — so the just-bumped client is still the freshest (most-active) at detection time. This is the piece that closes the reasoning gap "what if the local client happened to be active more recently?" It also ties in cleanly with the spec's own note that TUI detection is *cached at picker startup* (Scope item 2): the cache is taken at exactly the moment the remote trigger action bumped the remote client's activity, so the cached winner is the trigger. Without the temporal premise stated, the most-active heuristic reads as an unproven correlation rather than a timing-anchored inference.

**Current**:
> **Validated mechanism:** `client_activity` tracks a client's **sent input**, not the **received redraws** it gets from mirroring another client's session. A trigger keystroke on the remote client bumps only the remote's activity; a passively-mirroring local client stays stale. So "most-active client on the session" reliably fingers the remote trigger.

**Proposed Addition**:
Added a temporal-premise sentence to the Validated mechanism paragraph: detection runs immediately after the trigger action (picker startup / CLI command entry), so the just-bumped triggering client is freshest at detection time.

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 2. `portal doctor` corrected-output string is not grounded in source

**Source**: investigation.md — Code Trace (line 104) and Blast Radius (line 135): both describe only that `doctor` "would report a driveable host terminal for a remote session with a local client attached" and that the fix "corrects it in lockstep." The corrected output string is never specified.
**Category**: Gap/Ambiguity
**Affects**: Scope: Affected Surfaces — item 3 (`portal doctor` host-terminal line)

**Details**:
The spec asserts a concrete post-fix output: the mixed case "now reports 'unsupported (remote session)' instead of misreporting a driveable host terminal." That specific string appears nowhere in the investigation — the source only characterises the *before* state (misreports a driveable terminal) and that the diagnostic is corrected in lockstep, never the exact *after* text `checkHostTerminal` renders. An implementer taking the quoted string literally could hard-code / assert wording that differs from what `checkHostTerminal` actually produces. Either the string should be grounded (verified against `cmd/doctor.go` `checkHostTerminal`) or softened to a behavioural claim (reports unsupported rather than a driveable terminal) to match what the source actually supports.

**Proposed Addition**:
Reworded Scope item 3 to ground the string as checkHostTerminal's existing NULL-branch output, verified against current code (short-circuits on IsNull()).

**Resolution**: Approved
**Notes**: Auto-approved.

---

### 3. Testing Requirements omit the source's "outside unit-test reach" constraint

**Source**: investigation.md — Why It Wasn't Caught (line 124): "Not reproducible without a real multi-client setup. Reproduction needs an actual remote client (SSH/mosh) plus a host-local client on the same session — outside unit-test reach and easy to miss in manual testing."
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements

**Details**:
The spec's Testing Requirements are entirely unit-level (invert `:133`, reframe `:196`, add the local-most-active case, preserve invariants) — which correctly locks the *selection/locality-ordering logic* via the seeded `clientLister`/`walker`/`reader` fakes. But the investigation explicitly flags that the actual defect — a real remote client resolving NULL and the N-1 windows genuinely not opening on the host machine — is "outside unit-test reach and easy to miss in manual testing." The spec never acknowledges this limit: it does not note that the end-to-end wrong-machine behaviour (the user's real remote + host-local workflow) is not exercised by the unit suite, nor whether a manual verification in the real multi-client setup is expected. Given this is the reported bug's actual reproduction condition, the testing section reads as complete when a whole verification dimension (the real-environment fix confirmation) is silently unaddressed. Worth either an explicit "unit tests cover the selection logic; the real multi-client scenario is out of unit-test reach — verify manually" caveat, or a conscious decision to accept unit coverage as sufficient.

**Proposed Addition**:
Added a 'Verification scope' note to Testing Requirements: unit tests cover the selection/locality logic; the real multi-client end-to-end scenario is out of unit-test reach and must be verified manually in the reproduction setup.

**Resolution**: Approved
**Notes**: Auto-approved.

---
