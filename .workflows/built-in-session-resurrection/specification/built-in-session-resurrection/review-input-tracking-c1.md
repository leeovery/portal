---
status: complete
created: 2026-04-21
cycle: 1
phase: Input Review
topic: built-in-session-resurrection
---

# Review Tracking: built-in-session-resurrection - Input Review

## Findings

### 1. `ephemeral-skipped` counter contradicts the YAGNI decision

**Source**: discussion §CLI Surface (`portal state status` example output, line ~1440); §Ephemeral Session Opt-Out (lines 1504–1531, decided SKIPPED)
**Category**: Enhancement to existing topic
**Affects**: CLI Surface → `portal state status`

**Details**:
The spec reproduces the discussion's `portal state status` example output verbatim, including the line `Sessions captured: 10 (0 ephemeral-skipped)`. But the discussion decided not to build ephemeral opt-out for v1 (§Ephemeral Session Opt-Out — SKIPPED (YAGNI)). The only "skipped" class in v1 is the `_*`-prefixed internal sessions (e.g., `_portal-saver`). Leaving `ephemeral-skipped` in the status example implies a feature that isn't built and will confuse implementation.

**Current**:
```
Portal state:
  Save daemon: running (pid 12345, version v0.4.2)
  Last save: 12 seconds ago
  Sessions captured: 10 (0 ephemeral-skipped)
  Panes captured: 34
  State size: 18.2 MB on disk
  Recent warnings: 0 (last: none)
```

**Proposed Addition**:
Remove the `(0 ephemeral-skipped)` annotation.

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 2. Empirical validation of scrollback restore mechanism not captured

**Source**: discussion §Scrollback Restore Mechanics lines 954–961 ("Validation" subsection) and lines 771–774 (later consolidation)
**Category**: Enhancement to existing topic
**Affects**: Scrollback Restore Mechanics → Validation Reference

**Details**:
The spec's "Validation Reference" section exists and mentions the validation was performed on an isolated tmux socket, listing three empirical findings. But the discussion captures an additional important detail the spec omits: validation was performed on a dedicated socket name (`tmux -L portal-hydrate-validate-<pid>`) *without touching the default socket*, and the final confirmation was: "Default socket sessions identical before and after test. No cross-contamination." This is relevant for the Planning phase — it tells the implementer how to structure the validation harness.

**Proposed Addition**:
Add a final bullet: "Default-socket sessions were verified identical before and after the test — validation does not contaminate the user's live tmux state. The isolated socket pattern (`tmux -L <unique-name>`) is the recommended approach for future mechanism verification."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 3. `run-shell -b` tmux bug history rationale omitted

**Source**: discussion §Shell Readiness + Hook Firing Redesign line 1089 (in-body reference); §Save-Side Architecture false-path line 791
**Category**: Enhancement to existing topic
**Affects**: Resume Hook Firing → `run-shell` Blocking Note

**Details**:
The spec's `run-shell` Blocking Note says "tmux 3.0+ has settled `-b` behavior" without explaining why the statement is load-bearing. The discussion provides the backing evidence: a false-path explicitly calls out known historical bugs ([tmux#1843](https://github.com/tmux/tmux/issues/1843), [#2306](https://github.com/tmux/tmux/issues/2306)) and the fact that no TPM plugin uses `run-shell -b` as a poor-man's-daemon after ~10 years.

**Proposed Addition**:
Expand the final sentence to: "tmux 3.0+ appears to have settled earlier `-b` flag issues (see tmux#1843, tmux#2306 for the historical context); however, the primitive remains unused by mainstream tmux plugins as a poor-man's-daemon pattern. Defer the async switch until there is evidence the synchronous blocking matters in practice."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 4. Fully-eager scrollback cost range (2–15s) not captured in rejection

**Source**: discussion §Scrollback Restore Mechanics line 967; §Restore-Side line 827
**Category**: Enhancement to existing topic
**Affects**: Restore-Side Architecture → Why Scrollback-Lazy

**Details**:
The spec mentions "2-15s" in the Rejected Alternatives bullet and "~15 seconds" in Why Scrollback-Lazy, but the discussion grounded the range with specific scenario numbers: `history-limit 50000` per pane × 30 panes.

**Proposed Addition**:
Amend to reference the lower bound as well: "Fully-eager scrollback injection at realistic power-user sizes (`history-limit 50000` per pane × 30 panes) would add 2–15 seconds to boot depending on pane scrollback fullness — unacceptable UX even at the low end."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 5. Scrollback truncation-at-head property not called out

**Source**: discussion §Restore-Side Architecture line 892
**Category**: Enhancement to existing topic
**Affects**: Save Format & Schema → Scrollback Files

**Details**:
The discussion explicitly confirmed that scrollback capture naturally truncates at head because tmux's history buffer is a ring. File size is bounded by `history-limit × avg-line-bytes`.

**Proposed Addition**:
Add a trailing sentence: "Scrollback size per pane is naturally bounded by `history-limit × avg-line-bytes` — tmux's history buffer is a ring that discards oldest lines at the head when the limit is exceeded. No Portal-side cap is needed to keep files bounded."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 6. Dormant session file persistence property not documented

**Source**: discussion §Restore-Side Architecture line 890
**Category**: Enhancement to existing topic
**Affects**: Marker Coordination — `@portal-skeleton-<paneKey>` section

**Details**:
The discussion calls out a specific property as "confirmed": dormant restored sessions (user never attaches, never hydrates) keep their saved scrollback files intact for as long as they go unhydrated.

**Proposed Addition**:
Add a bullet: "**User-visible property**: for sessions the user never attaches to, the skeleton marker stays set indefinitely, the save loop keeps skipping, and the pre-boot scrollback file on disk remains intact."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 7. Capture-cost grounding (`~10ms per pane round-trip`) not in spec

**Source**: discussion §Save Format & Schema line 559
**Category**: Enhancement to existing topic
**Affects**: Deferred — Parallel capture

**Details**:
The spec lists "Parallel capture for many-pane configurations. Deferred until performance complaints surface." But doesn't capture the order-of-magnitude justification: per-pane capture round-trip is ~10ms, realistic pane counts stay under ~20.

**Proposed Addition**:
Expand to: "- **Parallel capture** for many-pane configurations. Deferred until performance complaints surface. Sequential capture is adequate at realistic scale: per-pane round-trip cost is ~10ms and realistic pane counts stay under ~20, keeping capture well inside the 1-second tick budget."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 8. 30-second cadence derivation from Zellij precedent not captured

**Source**: discussion §Save-Side Architecture line 352
**Category**: Enhancement to existing topic
**Affects**: Save-Side Architecture: Triggers & Serialization — Periodic

**Details**:
The spec chooses 30s as the periodic max-gap without citing precedent. The discussion explicitly references Zellij's `DEFAULT_SERIALIZATION_INTERVAL` (60s today, 1s before disk-write complaints in v0.39.2, per Zellij PR #2951) as calibration input.

**Proposed Addition**:
Append a short rationale bullet: "The 30-second figure is informed by Zellij's trajectory — Zellij originally defaulted to 1-second serialization, then raised to 60 seconds in v0.39.2 after disk-write volume complaints (Zellij PR #2951). Portal's 30s is a compromise between Zellij's pre- and post-complaint positions."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 9. `tmux-slay` precedent reference omitted from rationale

**Source**: discussion §Save-Side Architecture line 319 and line 358
**Category**: Enhancement to existing topic
**Affects**: Save-Side Architecture: Execution Model → Host Process

**Details**:
The spec mentions "Pattern has real-world precedent (tmux-slay)" in one bullet but does not expand. The discussion frames `tmux-slay` as "the only public precedent."

**Proposed Addition**:
Clarify the precedent's standing: "- Pattern has real-world precedent (tmux-slay, the only public tool known to host a long-running process inside a detached tmux session). The concerns are concrete and addressed, but the pattern is niche — new territory relative to common tmux integrations."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---

### 10. "86 GB/day" worst-case framing for dedup savings not captured

**Source**: discussion §Save Format & Schema line 528 and line 540
**Category**: Enhancement to existing topic
**Affects**: Content-Hash Dedup

**Details**:
The spec describes the dedup mechanism but loses the motivating scale: the discussion grounded the decision by computing that the naive "rewrite every tick" strategy would produce ~86 GB/day of writes in a heavy scrollback scenario.

**Proposed Addition**:
Sharpen the motivation: "To avoid rewriting unchanged scrollback on every tick — which would generate on the order of 86 GB/day of writes in a heavy-history configuration (`history-limit 50000` × 10 panes) and cause significant SSD wear — the daemon holds an in-memory map `paneKey → hash-of-last-written-scrollback`. Content-hash dedup reduces worst-case write volume to single-digit MB/day for realistic workloads."

**Resolution**: Approved
**Notes**: Applied in auto mode.

---
