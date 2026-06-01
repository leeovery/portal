---
status: in-progress
created: 2026-06-01
cycle: 2
phase: Gap Analysis
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Gap Analysis

## Findings

### 1. Daemon self-eject's `os.Exit(0)` cannot emit `process: exit`, contradicting the forensic guarantee and the four-way terminal classification

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Defensive invariants § "Externally-killed-process footnote" (line 586) + "Mechanical rule — `process: exit` and the `main` exit shape" (lines 518–560); Saver/daemon lifecycle taxonomy § daemon lifecycle catalog (line 889) and § reason value spaces (line 899)

**Details**:
The defensive-invariants footnote asserts (line 586): "The daemon's clean self-eject uses `os.Exit(0)` and still emits its own `process: exit`." But the spec defines exactly one mechanism for emitting `process: exit`: `log.Close(code)`, called only from the `main` exit shape on the `rootCmd.Execute()` return path (lines 534–553). The daemon's self-supervision self-eject calls `os.Exit(0)` directly from inside the tick loop (per the codebase's documented self-supervision design — `os.Exit(0)` bypassing `daemonShutdownFunc`), which never unwinds back through `rootCmd.Execute()` → `main` → `log.Close`. `os.Exit` skips all deferred and post-Execute code. So an implementer following the specified `main` exit shape literally would produce **no** `process: exit` line on self-eject — directly contradicting line 586's "still emits its own `process: exit`" claim.

This is load-bearing because the four-way terminal classification (lines 577–584) is a core deliverable: a self-ejecting daemon whose `process: start` is never paired falls into the table's "*nothing* → Genuinely alarming" row, yet line 586 promises it IS paired. The spec asserts the outcome without specifying the mechanism, and the only specified mechanism is unreachable on this path.

Compounding the ambiguity: the daemon lifecycle catalog (line 889) lists a `daemon: shutdown` INFO event for "Daemon shutdown (any reason)" with `reason ∈ {sighup, self-eject, signal, exit}` — i.e. `self-eject` is a sanctioned `shutdown reason`, implying `daemon: shutdown reason=self-eject` fires on self-eject. But if `daemon: shutdown` is emitted from the shutdown path that self-eject's `os.Exit(0)` explicitly bypasses, then either (a) `self-eject` is a dead value in the reason space (never emitted), or (b) the self-eject path must emit `daemon: shutdown` (and/or `log.Close`) *before* its `os.Exit(0)`. The spec gives conflicting signals across three sections (line 586 footnote, line 888 `daemon: self-eject`, line 889 `daemon: shutdown reason=self-eject`), so an implementer cannot determine what the self-eject path emits: only `daemon: self-eject`? plus `daemon: shutdown reason=self-eject`? plus `process: exit`? The bare-`os.Exit`-prohibition (line 557, "prohibited outside `main`") further conflicts with the self-eject using `os.Exit(0)` outside `main` without a carve-out.

**Proposed Addition**:
Pin the self-eject emission sequence explicitly: state whether the self-eject path calls `log.Close(0)` (and/or emits `daemon: shutdown reason=self-eject`) *before* `os.Exit(0)`, so the `process: exit` pairing the footnote promises is actually produced; OR, if no pairing is intended, correct line 586 and add the daemon-self-eject case to the four-way classification (an `os.Exit(0)`-without-Close pairing rule) and reconcile the `self-eject` value in the `daemon: shutdown` reason space (remove it, or specify it fires before the exit). Also carve out the daemon self-eject from the "bare `os.Exit` prohibited outside `main`" rule.

**Resolution**: Pending
**Notes**: Priority: Critical.

---

### 2. `saver: placeholder died` has three conflicting emitter attributions and no resolvable calling location

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Saver/daemon lifecycle taxonomy § saver lifecycle catalog (line 880) and § "Calling code locations" (lines 911–913); Defensive invariants § "Externally-killed-process footnote" (line 586)

**Details**:
Three sections name different emitters for the single `saver: placeholder died` line:
- Lifecycle catalog (line 880): Site = "**Daemon self-supervision** observes the saver pane's host process exited" → implies the daemon process (`cmd/state_daemon.go`) emits it.
- Defensive-invariants footnote (line 586): "**Bootstrap** emits `saver: kill-barrier started/escalated target_pid=X` and `saver: placeholder died`." → says the bootstrap process (`cmd/bootstrap/`) emits it.
- Calling-code-locations (lines 911–913): `cmd/bootstrap/` is listed for "most saver lifecycle events (placeholder creation, respawn, kill-barrier, daemon-ready observation)" — `placeholder died` is conspicuously **absent**; and `cmd/state_daemon.go` is listed only for the `daemon:`-component events (`lock acquired`, `self-eject`, `shutdown`), not the `saver:`-component `placeholder died`.

So `saver: placeholder died` is assigned to the daemon by one section, to bootstrap by another, and to neither by the enumeration that is supposed to pin the calling file. An implementer cannot determine which process/file emits it — a planning-blocking dangling reference, since bootstrap (`cmd/bootstrap/`) and the daemon (`cmd/state_daemon.go`) are entirely different call sites with different observers. The two scenarios are also semantically distinct: bootstrap killing a *prior* daemon (the kill-barrier context, line 586) vs. the *current* daemon's self-supervision noticing its saver-pane host changed (line 880) — yet both are mapped to the same line with the same `target_pid`/`reason` attrs. The externally-killed-process forensic reasoning (line 586) explicitly leans on bootstrap being the recorder, so getting the emitter right is load-bearing for the unpaired-`process: start` triage rule.

**Proposed Addition**:
Pin a single emitter (process + file) for `saver: placeholder died`, reconcile all three sections, and — if both the bootstrap-kill-barrier scenario and the daemon-self-supervision scenario genuinely need a marker — either give each its own catalog row (distinct msg/component) or state explicitly that the same line is emitted from both sites with the same shape. Add the chosen calling location to lines 911–913.

**Resolution**: Pending
**Notes**: Priority: Important.

---

### 3. Cycle-summary `<unit>` examples include `orphans` and `files`, which are not in the closed attr vocabulary

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Cycle-level summary § mechanical rule `<unit>` list (line 833); cross-refs Subsystem taxonomy § closed cycle-summary attr space (line 196) and § extension policy (lines 244–247)

**Details**:
The cycle-summary mechanical rule lists `<unit>` example values as "`sessions`, `panes`, `entries`, `orphans`, `steps`, `files`, etc." (line 833). The closed cycle-summary attr vocabulary (line 196) is: `sessions`, `panes`, `entries`, `steps`, `step`, `windows`, `skipped`, `warnings`, `natural_churn`, `anomalous`, `reaped`, `killed`, `unset`, `entries_failed`. Two of the suggested `<unit>` values — **`orphans`** and **`files`** — are not in that closed list, nor anywhere else in the 49-key vocabulary.

This directly conflicts with the spec's emphatically closed attr vocabulary and its hard rule (line 246): "Spec writers and code reviewers MAY NOT introduce new component or attr names ad hoc." The concrete cycle catalog (lines 845–854) never actually uses `orphans` or `files` as a unit — the orphan sweeps report `reaped`/`killed`/`skipped`/`unset` instead — so the two example values are not only un-vocabularied but also unused. An implementer authoring a new cycle summary, told `<unit>` may be `orphans` or `files`, would emit `orphans=N`/`files=N` keys that PR review is mandated to reject (line 382). The trailing "etc." compounds it by inviting arbitrary unit keys, undercutting the closed guarantee for this attr group.

**Proposed Addition**:
Replace the `<unit>` example list with values drawn only from the closed cycle-summary vocabulary (`sessions`, `panes`, `entries`, `steps`, `windows`, …) — drop `orphans` and `files`. If a meaningful sweep genuinely needs an `orphans`/`files` count key, add it to the closed vocabulary via the amendment process (and update the 14-count); otherwise remove the "etc." or scope it to "drawn from the closed cycle-summary keys above" so it cannot be read as a license to invent unit keys.

**Resolution**: Pending
**Notes**: Priority: Important.

---

### 4. Hydrate-helper signal-receipt lines render `hydrate:` but the component table assigns signal receipt to `signal`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Subsystem taxonomy § component ownership table (`signal` row, line 166); Hook-firing observability limit § failure-mode lines (line 959, `signal timeout`)

**Details**:
The component-ownership table (line 166) assigns the `signal` component ownership of "FIFO signaling — `EagerSignalHydrate`, **hydrate-helper signal receipt**." But the Hook-firing catalog emits the hydrate helper's signal-receipt-failed event via `hookLogger` (line 959: `hookLogger.Info("signal timeout", "took", signalTimeout)`), and `hookLogger` is the hydrate helper's logger bound to component `hydrate` (the section's grep guarantee, line 923, is `grep "hydrate:"` reconstructs every helper invocation). So the `signal timeout` line — literally the hydrate helper's signal-receipt outcome — renders `hydrate:`, not `signal:`, even though the component table says signal receipt is owned by `signal`.

A reviewer applying the closed-component table mechanically would expect a `signal:` prefix; the catalog produces `hydrate:`. Forensically, `grep "signal:"` would not surface the hydrate-helper signal-receipt event the component table implies it owns. The line-350 precedence rule ("the subtopic catalogs ... are authoritative for their real call sites") does resolve which prefix wins (the catalog → `hydrate:`), so this is not a hard implementation fork — but the "hydrate-helper signal receipt" phrasing under `signal` is a genuine cross-section ownership tension that should be tightened so the component table and the catalog agree on where the hydrate helper's signal-wait outcome lives.

**Proposed Addition**:
Tighten the `signal` component description (line 166) to scope it to the signaling *mechanism* (e.g. `EagerSignalHydrate` and the lower-level FIFO signal-send/receive plumbing in `internal/state`), and clarify that the hydrate helper's own terminal signal-receipt outcome lines (`signal timeout`, etc.) render under `hydrate` per the Hook-firing catalog — so the two sections no longer appear to claim the same event for different components.

**Resolution**: Pending
**Notes**: Priority: Minor. Line-350 precedence rule already resolves the prefix; this is a clarity/forensic-completeness wart, not an implementation fork.

---
