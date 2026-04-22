---
status: in-progress
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
**Affects**: CLI Surface → `portal state status` (spec line 1091)

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
Remove the `(0 ephemeral-skipped)` annotation, or replace with `(1 internal)` counting `_portal-saver` (the actually-skipped class) if a parenthetical is desired. Simplest: drop the parenthetical entirely.

**Resolution**: Pending
**Notes**:

---

### 2. Empirical validation of scrollback restore mechanism not captured

**Source**: discussion §Scrollback Restore Mechanics lines 954–961 ("Validation" subsection) and lines 771–774 (later consolidation)
**Category**: Enhancement to existing topic
**Affects**: Scrollback Restore Mechanics → Validation Reference (spec lines 769–774)

**Details**:
The spec's "Validation Reference" section exists and mentions the validation was performed on an isolated tmux socket, listing three empirical findings. But the discussion captures an additional important detail the spec omits: validation was performed on a dedicated socket name (`tmux -L portal-hydrate-validate-<pid>`) *without touching the default socket*, and the final confirmation was: "Default socket sessions identical before and after test. No cross-contamination." This is relevant for the Planning phase — it tells the implementer how to structure the validation harness.

**Current**:
```
### Validation Reference

The mechanism was empirically validated on an isolated tmux socket during discussion:
- `cat FILE; exec bash` pattern: 1000-line ANSI scrollback rendered correctly; clean `bash-5.3$` prompt at end.
- Shell history contained only post-test commands — no helper, no `cat`, no scrollback content.
- Blocking-FIFO variant: pane empty before signal; after `echo "go" > fifo`, scrollback rendered and shell prompt appeared.
```

**Proposed Addition**:
Add a final bullet: "Default-socket sessions were verified identical before and after the test — validation does not contaminate the user's live tmux state. The isolated socket pattern (`tmux -L <unique-name>`) is the recommended approach for future mechanism verification."

**Resolution**: Pending
**Notes**:

---

### 3. `run-shell -b` tmux bug history rationale omitted

**Source**: discussion §Shell Readiness + Hook Firing Redesign line 1089 (in-body reference); §Save-Side Architecture false-path line 791
**Category**: Enhancement to existing topic
**Affects**: Resume Hook Firing → `run-shell` Blocking Note (spec lines 832–836)

**Details**:
The spec's `run-shell` Blocking Note says "tmux 3.0+ has settled `-b` behavior" without explaining why the statement is load-bearing. The discussion provides the backing evidence: a false-path explicitly calls out known historical bugs ([tmux#1843](https://github.com/tmux/tmux/issues/1843), [#2306](https://github.com/tmux/tmux/issues/2306)) and the fact that no TPM plugin uses `run-shell -b` as a poor-man's-daemon after ~10 years. This is why Portal defers switching to `-b` until evidence surfaces — not purely "we don't need it," but also "the primitive had real reliability issues historically." Useful context for future maintainers considering the async switch.

**Current**:
```
tmux's `run-shell` is synchronous by default and blocks the server during hook execution. Acceptable for initial release — the user is actively attaching; sub-150ms is imperceptible at the moment of attach. If real-world use reveals problems (other clients feeling laggy during heavy attaches), switch to `run-shell -b` (async). tmux 3.0+ has settled `-b` behavior; defer the switch until there is evidence the blocking matters.
```

**Proposed Addition**:
Expand the final sentence to: "tmux 3.0+ appears to have settled earlier `-b` flag issues (see tmux#1843, tmux#2306 for the historical context); however, the primitive remains unused by mainstream tmux plugins as a poor-man's-daemon pattern. Defer the async switch until there is evidence the synchronous blocking matters in practice."

**Resolution**: Pending
**Notes**:

---

### 4. Fully-eager scrollback cost range (2–15s) not captured in rejection

**Source**: discussion §Scrollback Restore Mechanics line 967 ("2–15s added boot delay at realistic scales"); §Restore-Side line 827 ("30 panes eagerly ≈ 15s")
**Category**: Enhancement to existing topic
**Affects**: Restore-Side Architecture → Why Scrollback-Lazy (spec line 590); Rejected Alternatives (spec line 657)

**Details**:
The spec mentions "2-15s" in the Rejected Alternatives bullet (line 657) and "~15 seconds" in Why Scrollback-Lazy (line 590), but the discussion grounded the range with specific scenario numbers: `history-limit 50000` per pane × 30 panes. The spec's line 590 references the pane count but not the range — combining the two datapoints in one place would make the rejection evidence crisper for anyone reconsidering fully-eager later.

**Current**:
```
Fully-eager scrollback injection at realistic power-user sizes (`history-limit 50000` per pane × 30 panes) would add ~15 seconds to boot — unacceptable UX. Lazy hydration amortizes cost across attaches; sessions the user never touches today cost zero to hydrate.
```

**Proposed Addition**:
Amend to reference the lower bound as well: "Fully-eager scrollback injection at realistic power-user sizes (`history-limit 50000` per pane × 30 panes) would add 2–15 seconds to boot depending on pane scrollback fullness — unacceptable UX even at the low end. Lazy hydration amortizes cost across attaches; sessions the user never touches today cost zero to hydrate."

**Resolution**: Pending
**Notes**: Minor; both values are grounded in the discussion, not new.

---

### 5. Scrollback truncation-at-head property not called out

**Source**: discussion §Restore-Side Architecture line 892 ("Scrollback truncation at head: tmux's history buffer is a ring; `capture-pane` returns current buffer. File size bounded by `history-limit × avg-line-bytes`. Natural.")
**Category**: Enhancement to existing topic
**Affects**: Save Format & Schema → Scrollback Files (spec lines 288–296), or Save Content & Scope

**Details**:
The discussion explicitly confirmed (as a "confirmed property" in answer to user questions) that scrollback capture naturally truncates at head because tmux's history buffer is a ring. File size is bounded by `history-limit × avg-line-bytes`. This is a useful bound to document — it tells the user (and the implementer) that per-pane file sizes are predictable and upper-bounded without Portal doing any explicit capping. It also explains why a scrollback cap is safely YAGNI'd.

**Current**:
```
### Scrollback Files

Each live pane has its own scrollback file containing raw `capture-pane -e -p -S -` output (ANSI escape sequences preserved inline, no encoding transformation).
```

**Proposed Addition**:
Add a trailing sentence: "Scrollback size per pane is naturally bounded by `history-limit × avg-line-bytes` — tmux's history buffer is a ring that discards oldest lines at the head when the limit is exceeded. No Portal-side cap is needed to keep files bounded."

**Resolution**: Pending
**Notes**:

---

### 6. Dormant session file persistence property not documented

**Source**: discussion §Restore-Side Architecture line 890 ("Dormant session files persist indefinitely: `@portal-skeleton-` marker prevents save-overwrite. Days/weeks of ignored sessions → files on disk intact.")
**Category**: Enhancement to existing topic
**Affects**: Marker Coordination — `@portal-skeleton-<paneKey>` section (spec lines 611–619)

**Details**:
The discussion calls out a specific property as "confirmed": dormant restored sessions (user never attaches, never hydrates) keep their saved scrollback files intact for as long as they go unhydrated. The skeleton marker stays set, the save loop keeps skipping them, and the on-disk file remains the pre-boot state. This is a user-visible guarantee ("if I don't attach for a month, my history from before the reboot is still there") that the spec covers only implicitly via the marker-semantics section.

**Current**:
The `@portal-skeleton-<paneKey>` section describes the marker's behavior but does not surface the user-visible property that dormant sessions retain pre-boot scrollback indefinitely.

**Proposed Addition**:
Add a bullet under the `@portal-skeleton-<paneKey>` section: "**User-visible property**: for sessions the user never attaches to, the skeleton marker stays set indefinitely, the save loop keeps skipping, and the pre-boot scrollback file on disk remains intact. A user who reboots and then leaves a session dormant for weeks will still have their pre-boot history available the first time they attach."

**Resolution**: Pending
**Notes**:

---

### 7. Capture-cost grounding (`~10ms per pane round-trip`) not in spec

**Source**: discussion §Save Format & Schema line 559 ("sequential capture is fine — round-trip cost per pane is ~10ms, and realistic pane counts stay under ~20")
**Category**: Enhancement to existing topic
**Affects**: Deferred (Not Non-Goals, Just Not Now) — Parallel capture (spec line 82)

**Details**:
The spec lists "Parallel capture for many-pane configurations. Deferred until performance complaints surface." But doesn't capture the order-of-magnitude justification: per-pane capture round-trip is ~10ms, realistic pane counts stay under ~20, so sequential capture completes in well under 1 second — comfortably inside the 1-second tick budget. This back-of-envelope is what makes the YAGNI defensible; omitting it makes the deferral look speculative when it's actually grounded.

**Current**:
```
- **Parallel capture** for many-pane configurations. Deferred until performance complaints surface.
```

**Proposed Addition**:
Expand to: "- **Parallel capture** for many-pane configurations. Deferred until performance complaints surface. Sequential capture is adequate at realistic scale: per-pane round-trip cost is ~10ms and realistic pane counts stay under ~20, keeping capture well inside the 1-second tick budget."

**Resolution**: Pending
**Notes**:

---

### 8. 30-second cadence derivation from Zellij precedent not captured

**Source**: discussion §Save-Side Architecture line 352 ("Matches Zellij's default (`DEFAULT_SERIALIZATION_INTERVAL = 60000ms`, was 1s pre-v0.39.2, raised due to disk-write complaints per [Zellij PR #2951](https://github.com/zellij-org/zellij/pull/2951))")
**Category**: Enhancement to existing topic
**Affects**: Save-Side Architecture: Triggers & Serialization — Periodic (spec lines 191–195), or Scope Boundaries rationale

**Details**:
The spec chooses 30s as the periodic max-gap without citing precedent. The discussion explicitly references Zellij's `DEFAULT_SERIALIZATION_INTERVAL` (60s today, 1s before disk-write complaints in v0.39.2, per Zellij PR #2951) as calibration input — Portal's 30s is positioned as a "reasonable compromise between data loss and disk write volume" informed by Zellij's trajectory. Losing the reference makes the 30s figure look arbitrary instead of empirically derived from a comparable tool.

**Current**:
```
- **30-second max-gap** bounds worst-case scrollback loss on unexpected tmux/system termination at 30 seconds, even during periods with zero structural events.
```

**Proposed Addition**:
Append a short rationale bullet near the 30-second max-gap: "The 30-second figure is informed by Zellij's trajectory — Zellij originally defaulted to 1-second serialization, then raised to 60 seconds in v0.39.2 after disk-write volume complaints (Zellij PR #2951). Portal's 30s is a compromise between Zellij's pre- and post-complaint positions, reflecting both the write-volume concern and Portal's narrower YAGNI-first disk budget."

**Resolution**: Pending
**Notes**:

---

### 9. `tmux-slay` precedent reference omitted from rationale

**Source**: discussion §Save-Side Architecture line 319 ("pattern is niche (tmux-slay is the only public precedent)") and §Save-Side line 358 ("The pattern has precedent (tmux-slay)")
**Category**: Enhancement to existing topic
**Affects**: Save-Side Architecture: Execution Model → Host Process (spec line 131)

**Details**:
The spec mentions "Pattern has real-world precedent (tmux-slay)" in one bullet on line 131 but does not expand. The discussion frames `tmux-slay` as "the only public precedent" for hosting a long-running process inside a detached tmux session — a pointed claim that's useful for anyone evaluating whether the pattern is load-bearing. The spec keeps the reference but loses the "sole precedent" framing, which weakens the signal about pattern novelty.

**Current**:
```
- Pattern has real-world precedent (tmux-slay). The concerns are concrete and addressed.
```

**Proposed Addition**:
Clarify the precedent's standing: "- Pattern has real-world precedent (tmux-slay, the only public tool known to host a long-running process inside a detached tmux session). The concerns are concrete and addressed, but the pattern is niche — new territory relative to common tmux integrations."

**Resolution**: Pending
**Notes**: Minor; signals a known pattern-novelty consideration for future reviewers.

---

### 10. "86 GB/day" worst-case framing for dedup savings not captured

**Source**: discussion §Save Format & Schema line 528 ("The naive 'rewrite every scrollback file every 30s' plan would generate ~86GB/day of writes in a heavy-scrollback scenario") and line 540 ("turns worst-case 86GB/day into single-digit MB/day")
**Category**: Enhancement to existing topic
**Affects**: Content-Hash Dedup (spec lines 368–378)

**Details**:
The spec describes the dedup mechanism but loses the motivating scale: the discussion grounded the decision by computing that the naive "rewrite every tick" strategy would produce ~86 GB/day of writes in a heavy scrollback scenario (`history-limit 50000` × 10 panes), and dedup reduces that to single-digit MB/day. This order-of-magnitude framing is what justifies paying the complexity cost of an in-memory hash map and cross-tick state — without it, dedup looks like premature optimization.

**Current**:
```
### Content-Hash Dedup

To avoid rewriting unchanged scrollback on every tick (which would generate gigabytes per day for heavy-history configurations), the daemon holds an in-memory map `paneKey → hash-of-last-written-scrollback`.
```

**Proposed Addition**:
Sharpen the motivation: "To avoid rewriting unchanged scrollback on every tick — which would generate on the order of 86 GB/day of writes in a heavy-history configuration (`history-limit 50000` × 10 panes) and cause significant SSD wear — the daemon holds an in-memory map `paneKey → hash-of-last-written-scrollback`. Content-hash dedup reduces worst-case write volume to single-digit MB/day for realistic workloads."

**Resolution**: Pending
**Notes**:

---
