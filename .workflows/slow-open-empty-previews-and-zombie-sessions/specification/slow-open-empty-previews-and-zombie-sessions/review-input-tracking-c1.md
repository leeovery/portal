---
status: in-progress
created: 2026-05-22
cycle: 1
phase: Input Review
topic: slow-open-empty-previews-and-zombie-sessions
---

# Review Tracking: slow-open-empty-previews-and-zombie-sessions - Input Review

## Findings

### 1. Pre-v0.5.6 zombie-session behaviour contrasted with current

**Source**: Investigation, Symptoms / Problem Description (line 16): "Pre-v0.5.6, they would briefly reappear within a 'tick window' then disappear after ~5 s; now they never disappear."
**Category**: Enhancement to existing topic
**Affects**: Problem Statement (Symptom 3) and/or Root Cause symptom mapping

**Details**:
The investigation records that the zombie-session symptom has a notable shape change at v0.5.6 — pre-v0.5.6 the killed session would reappear briefly inside a "tick window" and then disappear after ~5 s; post-v0.5.6 it never disappears. This is informative about how the kill-barrier and merge-filter interact with multi-daemon contention across the v0.5.5 → v0.5.6 boundary. The spec's Symptom 3 simply says "reappear on the next `portal open` and persist indefinitely" without noting the regression-shape change.

**Current**:
> 3. **Killed sessions resurrect** — Sessions removed via `K` in the picker (or via the user's `Option-Q` tmux shortcut) reappear on the next `portal open` and persist indefinitely. Caused by multiple daemons independently committing `sessions.json` every tick — the legitimate daemon's post-kill commit (without the dead session) is overwritten seconds later by a competing daemon whose stale `prev` state still includes it. Restore on next bootstrap reconstructs the dead session as a skeleton pane. Expected: `K` removes the session permanently.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 2. `daemon.version` mismatch observation not preserved

**Source**: Investigation, Reporter's local diagnostic observations (line 62): "`daemon.version` file content was `0.5.5`."
**Category**: Enhancement to existing topic
**Affects**: Root Cause / Trigger description, or End-State Verification

**Details**:
The investigation notes that on the reporter's install the `daemon.version` file content was `0.5.5` even though the user had upgraded to 0.5.6. This is direct evidence that the version-guard / saver-upgrade path was not running cleanly — and corroborates that the `EnsurePortalSaverVersion` step couldn't replace the saver because the kill-barrier was timing out. The spec mentions the kill-barrier timeout but does not surface the `daemon.version` mismatch as a concrete diagnostic artefact that should disappear post-fix.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 3. Regression window framing within v0.5.x line

**Source**: Investigation, Constraints & Confirmed Context (line 74): "Regression window is within the v0.5.x line. Reporter is confident the session preview was working under some v0.5.x version. Investigation should establish the precise within-v0.5.x regression point rather than treating this as a long-standing latent fragility."
**Category**: Enhancement to existing topic
**Affects**: Root Cause (latency / regression attribution)

**Details**:
The investigation explicitly framed the bug as a within-v0.5.x regression for the preview-empty symptom (not a long-standing latent fragility). The spec attributes the `CaptureStructure` abort-on-error path to commit `7dc990be4` (2026-04-27) and v0.5.x line, but does not say anything about WHEN inside v0.5.x the preview started failing — i.e. it does not establish the regression point the investigation was directed to find. This may matter for choosing the version bump (v0.5.x patch vs v0.6.0) and for the changelog narrative.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 4. Hydrate WARN noise (`scrollback file not found`) as a downstream symptom

**Source**: Investigation, Symptoms / Manifestation block (line 25): `WARN | hydrate | scrollback file not found for --hook-key=A:0.0 --file=/Users/.../scrollback/A__0.0.bin`
**Category**: Enhancement to existing topic
**Affects**: End-State Verification (quiet-log assertion)

**Details**:
The investigation captures hydrate-side WARN log lines about scrollback files not being found as part of the observed broken state. The spec's End-State Verification lists the daemon-side log lines that should disappear (`another daemon holds the lock`, `prior daemon did not exit within 5s`, `no such session: _portal-saver`) but does not include the hydrate-side `scrollback file not found` warnings. Those should also quiet naturally once previews work and scrollback is stable across ticks.

**Current**:
> - **Daemon log is quiet under steady state.** No `"another daemon holds the lock"` entries, no `"prior daemon did not exit within 5s"` entries, no `"no such session: _portal-saver"` entries.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 5. Component C's lock-file open mode (`O_EXCL|O_CREAT`) divergence from investigation

**Source**: Investigation, Options Explored — C (line 303): "Open with `O_EXCL|O_CREAT`, then `fstat` the fd and `stat` the path, and refuse if inodes differ — with a bounded retry loop on stat-mismatch to absorb the small race window."
**Category**: Gap/Ambiguity
**Affects**: Component C — Stabilise the daemon.lock Singleton Against Inode Replacement

**Details**:
The investigation's described shape of C was "Open with `O_EXCL|O_CREAT`" followed by fstat/stat inode comparison. The spec keeps the existing `os.OpenFile(..., O_RDWR|O_CREATE, ...)` open and only adds the post-flock fstat/stat cross-check (plus a pre-acquire `daemon.pid` liveness gate). This is a deliberate-looking design choice — the pre-`daemon.pid` check serves the singleton role differently — but the spec does not explicitly note that O_EXCL|O_CREAT was considered and rejected, nor why. A reader returning to this spec from the investigation will wonder why the open mode change disappeared.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 6. Investigation's ruled-out items not preserved in spec

**Source**: Investigation, Dead Ends / Ruled Out (lines 224-227)
**Category**: Enhancement to existing topic
**Affects**: Root Cause or Risk Summary

**Details**:
The investigation explicitly ruled out three plausible-looking adjacent causes:
- TOCTOU on `ShowEnvironment` for session "A" (manual probe always succeeded; the daemon-log entry was noise from a different daemon).
- Merge-filter regression from `daemon-merge-reintroduces-dead-sessions` (filter is intact; the resurrect symptom is competing-daemon overwrite, not merge regression).
- Missing ctx-cancellable fix from `saver-kill-respawn-loop-leaks-daemons` (fix is present; orphan survival is reachability, not cancellation honouring).

The spec does not preserve these rule-outs. Anyone re-investigating after the spec exists may re-tread the same dead ends. At minimum the merge-filter rule-out is load-bearing for the Symptom-3 mechanism narrative (it says "merge-filter is NOT the cause; competing daemons are").

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 7. Upgrade-path inode-replacement as a landmine

**Source**: Investigation, Blast Radius / Potentially affected (line 255): "Any user upgrading across binaries while a daemon from a prior version is still running — the singleton's inode-replacement gap is the upgrade-time landmine."
**Category**: Enhancement to existing topic
**Affects**: Component C acceptance criteria or End-State Verification

**Details**:
The investigation flags binary upgrades as the realistic, user-facing trigger for the inode-replacement gap (the in-flight daemon was launched by the old binary; the new binary spawns a daemon that lands on a different inode). The spec describes C's mechanics but does not call out the upgrade scenario as the prototypical real-world reproduction — i.e. an acceptance test or scenario covering "v0.5.x daemon still alive when v0.5.(x+1) bootstraps" is not enumerated.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 8. Empirical measurement of N for Component D before tuning

**Source**: Investigation, Risk Assessment (line 339): "Main residual risk is D — overly-tight hysteresis would false-positive-exit the legitimate daemon during normal bootstrap kill-and-recreate. Mitigation: measure the legitimate transient duration empirically before picking N."
**Category**: Enhancement to existing topic
**Affects**: Component D — Daemon Self-Supervision / Risk Summary

**Details**:
The investigation explicitly requires measuring the legitimate `_portal-saver` create/recreate transient duration before picking N. The spec mentions "If implementation measurement during the planning phase reveals real-world transient durations longer than ~3 ticks, N can be increased" — but this is conditional on measurement, not a mandate to measure. The investigation framed it as a mitigation that MUST happen, not optional. The Risk Summary should probably promote this measurement to a required planning-phase activity rather than an "if revealed" qualifier.

**Current** (Risk Summary):
> - **Component D's hysteresis (N=3)** is the only tuning knob. Planning phase should verify empirically that the legitimate daemon does NOT observe N consecutive saver-absences during any normal operation (steady-state ticking, attach/detach cycles, hook-driven `client-attached` events). If a real-world transient is found to span 3 ticks, N is raised — the spec target is "single-digit ticks" not a fixed value.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 9. `portal clean` interaction with orphan sweep

**Source**: Investigation, Blast Radius / Directly + Potentially affected (lines 250, 253-254): bootstrap orchestrator needs a new step; `portal clean` listed as potentially affected because it reads `sessions.json`.
**Category**: Gap/Ambiguity
**Affects**: Component B (or new sub-component) — interaction with `portal clean`

**Details**:
The investigation notes `portal clean` as potentially affected by the bug (it reads `sessions.json` and trusts its contents). The spec adds the orphan sweep at bootstrap time only. It does not state whether `portal clean` should also gain orphan-sweep behaviour, or whether `portal clean` is expected to be a no-op for orphan daemons. Given that `portal clean` is the user's escape hatch for stuck state, it may be worth explicitly stating its scope here (either "out of scope — bootstrap sweep is sufficient" or "include in clean as well").

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 10. TUI preview adapter as the read-side of the symptom

**Source**: Investigation, Key Files (line 221): "`internal/tui/preview_adapter.go` — Reads scrollback `.bin` per paneKey; surfaces 'no saved content' when the file is missing (which is most of the time under the GC race)."
**Category**: Enhancement to existing topic
**Affects**: Root Cause / Symptom-2 mapping

**Details**:
The investigation explicitly maps the user-visible "no saved content" string to `internal/tui/preview_adapter.go` and the `state.ScrollbackFile(stateDir, paneKey)` read path. The spec's Symptom 2 explanation references the GC race but never names the TUI read site. This is minor — the fix doesn't touch the TUI — but the symptom narrative is incomplete without it (a future reader chasing "where does the 'no saved content' string come from?" needs to find this themselves).

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 11. Component F doom-loop framing (recovery doom-loop)

**Source**: Investigation, Options Explored — F (line 309): "produces the observed `no such session: _portal-saver` log noise and the recovery doom-loop where each bootstrap creates the saver, the daemon exits as lock-loser (because A/B haven't yet swept), the session self-destroys, and the next bootstrap finds it absent again."
**Category**: Enhancement to existing topic
**Affects**: Component F — Saver Creation Sets `destroy-unattached=off` BEFORE Daemon Starts

**Details**:
The spec describes F's "current behaviour" as the create-then-set race but does not articulate the *doom-loop* dynamic the investigation called out: each bootstrap recreates the saver, the daemon exits as lock-loser, the session self-destroys, and the next bootstrap re-enters the same loop. This loop is what makes F's fix non-trivially load-bearing in the multi-daemon state, not just a clean-up cosmetic. The spec's Goal mentions "recovery doom-loop" once but doesn't trace the loop itself.

**Current** (F, Goal):
> **Goal:** Eliminate the race in which a newly-created `_portal-saver` session is destroyed by tmux before its `destroy-unattached=off` option can be set, producing the observed `no such session: _portal-saver` log noise and the recovery doom-loop where each bootstrap creates the saver, the daemon exits as lock-loser (because A/B haven't yet swept), the session self-destroys, and the next bootstrap finds it absent again.

**Proposed Addition**:
(leave blank until discussed — Goal does state the loop succinctly; this finding may be redundant. Flagging for confirmation.)

**Resolution**: Pending
**Notes**: On re-read, the Goal does already capture the doom-loop. This finding may be withdrawn during discussion.

---
