---
status: in-progress
created: 2026-05-10
cycle: 2
phase: Input Review
topic: killed-sessions-resurrect-on-restart
---

# Review Tracking: killed-sessions-resurrect-on-restart - Input Review

## Findings

### 1. Log-noise rationale: warnings drown out future genuine signals

**Source**: Investigation "Supporting Observations" (line 60): "The volume of warnings on every bootstrap is high enough to drown out future genuine warnings."
**Category**: Enhancement to existing topic
**Affects**: Acceptance Criteria → AC6 (Logging) and/or Risks & Rollout → Behavioural Changes for Users

**Details**:
The investigation explicitly framed the high WARN volume as not just steady-state noise but as a signal-to-noise problem that obscures *future* genuine warnings during debugging. The spec's AC6 captures the post-fix volume drop but only frames it as "drops to zero in the steady state" — it does not capture the *why-this-matters* rationale (that the noise was actively undermining future diagnostic value). This rationale strengthens AC6's motivation and is also relevant when a maintainer evaluates whether AC6 is worth the verification cost.

**Current** (AC6):
> **AC6**: `WARN | hydrate | write fifo … no such file or directory` and `WARN | hydrate | timeout waiting for signal on …` log volume on every cold-start drops to zero in the steady state. These warnings appear only when a genuine signal-flow bug occurs. **Verification is via the Manual Verification Protocol step 2** — observational, not a gated automated test. Inspect `~/.config/portal/state/portal.log` after a clean cold-start with N≥2 saved sessions; the two warning lines must be absent.

**Proposed Addition**:
Add a one-sentence rationale clause to AC6 (or as a sibling note in Risks & Rollout) explaining that the pre-fix volume on every bootstrap was high enough to drown out future genuine warnings — so the post-fix steady-state cleanliness is what restores signal-to-noise for diagnostic value, not just an aesthetic improvement.

**Resolution**: Pending
**Notes**:

---

### 2. Symptom C diagnostic-invisibility framing absent

**Source**: Investigation "Why It Wasn't Caught" (line 201): "Symptom C is even more silent. Stuck markers suppressing scrollback save produces no error, no warning, no diagnostic — the user only notices on the next reboot when scrollback is empty. By then the connection to 'marker leaked from a previous timeout' is invisible."
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Observed Symptoms (Symptom C)

**Details**:
The spec captures Symptom C's mechanism (stuck marker → daemon skips capture indefinitely → CleanStaleMarkers cannot reach it) but omits the discoverability framing: the symptom is genuinely invisible at the time of failure, only manifesting on the *next* reboot as empty scrollback, with no diagnostic trail back to the marker leak. This framing matters because (a) it justifies the upstream-trigger fix priority over a downstream cleanup, and (b) it shapes manual-verification expectations — Symptom C cannot be observed pre-fix on the same boot the marker stuck during; it requires a second cold-start.

**Current** (Symptom C bullet):
> **Symptom C — scrollback save silently skipped** for any pane whose marker is stuck (the daemon's capture loop skips marked panes indefinitely). The existing post-restore `CleanStaleMarkers` step does **not** close this gap — its predicate is "marker without a live pane", but timeout-stuck markers sit on live panes (the helper has exec'd a shell, so the pane is alive even though hydration failed). Eager signaling closes the gap at the marker-production layer rather than the cleanup layer.

**Proposed Addition**:
Append a sentence to the Symptom C bullet noting that the symptom is observable to the user only across reboots (empty scrollback on the *next* cold-start) with no diagnostic linking it back to the stuck marker — explaining why the fix targets the marker-production layer and why Manual Verification of Symptom C requires a two-boot protocol.

**Resolution**: Pending
**Notes**:

---
