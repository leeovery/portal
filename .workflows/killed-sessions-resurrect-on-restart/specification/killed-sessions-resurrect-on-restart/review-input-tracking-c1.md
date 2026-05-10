---
status: in-progress
created: 2026-05-10
cycle: 1
phase: Input Review
topic: killed-sessions-resurrect-on-restart
---

# Review Tracking: killed-sessions-resurrect-on-restart - Input Review

## Findings

### 1. Duplicate `client-attached` ENOENT warnings — addressed-by-side-effect of Fix 1

**Source**: Investigation "Supporting Observations" (lines 57-58) and "Contributing Factors" (lines 192-193) and "Discussion" (line 252)
**Category**: Enhancement to existing topic
**Affects**: Fix 1 → Relationship to Existing Hook-Driven Signaling, or Fix 2 → Logging

**Details**:
The investigation explicitly documents that `signal-hydrate` "write fifo … no such file or directory" entries appear twice at the same timestamp for the same paneKey on some bootstraps, traced to both `client-attached` and `client-session-changed` firing near-simultaneously when the user attaches. The investigation's discussion section explains this phenomenon vanishes on its own under the chosen fix because eager signaling unsets markers before either event fires. The specification's logging section (AC6) covers the volume drop generally but does not specifically explain that the duplicate-fire ENOENT pattern is incidentally resolved.

**Current**:
"After the eager step has run, the user's subsequent attach (which is what causes the bare-CLI handoff or `tmux switch-client`) fires its hook against panes whose markers have already been unset by the helpers' success path; `signal-hydrate` enumerates the now-empty marker set for that session and exits cleanly. This is the desired 'second-fire is a no-op' behaviour."

**Proposed Addition**:
{Note that this also incidentally resolves the duplicate-timestamp ENOENT warnings observed in the investigation — both `client-attached` and `client-session-changed` can fire near-simultaneously on attach, and under the pre-fix flow each invocation logged ENOENT against the now-unlinked FIFO. With markers cleared by the eager step before either event fires, both invocations enumerate empty marker sets and exit silently.}

**Resolution**: Approved
**Notes**: Added to Fix 1 → Relationship to Existing Hook-Driven Signaling section.

---

### 2. `CleanStaleMarkers` cannot address timeout-stuck markers on live panes

**Source**: Investigation "Symptom C" (lines 174-178)
**Category**: Enhancement to existing topic
**Affects**: Problem Statement → Observed Symptoms (Symptom C), or Fix 1 → Failure Posture / motivation

**Details**:
The investigation explicitly documents that the companion-fix's step 7 (`CleanStaleMarkers`) cannot close the Symptom C gap because its predicate is "marker without a live pane" — but timeout-stuck markers are ON live panes (a parked sh + bare shell remains a live tmux pane). This is load-bearing context for why eager signaling at the marker-production layer is the right fix rather than expanding cleanup. The spec mentions Symptom C and the daemon skipping marked panes, but does not state why existing cleanup doesn't help.

**Current**:
"**Symptom C — scrollback save silently skipped** for any pane whose marker is stuck (the daemon's capture loop skips marked panes indefinitely)."

**Proposed Addition**:
{Add a clarifying note (in Symptom C itself, or in Fix 1's motivation) that the existing post-restore `CleanStaleMarkers` step does not address this case because its predicate is "marker without a live pane" — but timeout-stuck markers sit on live panes (the helper has exec'd a shell, so the pane is alive even though hydration failed). Eager signaling closes the gap at the marker-production layer instead of expanding cleanup.}

**Resolution**: Pending
**Notes**:

---

### 3. "Why It Wasn't Caught" insights useful for test plan motivation

**Source**: Investigation "Why It Wasn't Caught" (lines 196-202)
**Category**: Enhancement to existing topic
**Affects**: Test Plan, or Risks & Rollout → Regression Risk

**Details**:
The investigation enumerates five distinct reasons the bug escaped review — most relevantly: "Integration tests cover only the happy path. The `built-in-session-resurrection` integration tests verify the signal-arrived flow end-to-end (skeleton + signal + dump + hook + shell). They don't model the case where multiple sessions are skeletoned and only one is attached." This directly motivates the multi-session cold-start integration test the spec already lists, but framing it as a deliberate gap-closure rather than just a new test would be more durable. The "manual reproduction requires multiple saved sessions" point is also load-bearing context for why this test shape was missing.

**Current**:
"**Multi-session cold-start**: Boot with N≥2 saved sessions. Assert all `@portal-skeleton-*` markers are unset within reasonable time post-bootstrap (no client attach required)."

**Proposed Addition**:
{Add a brief note to the multi-session cold-start test plan entry that this test closes a specific gap in existing coverage — prior integration tests verified the signal-arrived flow only for the attached session, never modelled the N≥2 case where the bug's deterministic behaviour surfaces.}

**Resolution**: Pending
**Notes**:

---

### 4. Single-saved-session user is unaffected — useful blast-radius framing

**Source**: Investigation "Blast Radius → Not affected" (lines 215-217)
**Category**: Enhancement to existing topic
**Affects**: Scope Boundary, or Risks & Rollout → Behavioural Changes for Users

**Details**:
The investigation explicitly notes that single-saved-session users never see any of the visible bugs (no race, one session = one attach = one signal). Likewise, hot-path `portal open <existing-session>` after cold-start is unaffected (Restore skips live sessions). The spec captures the cold-start scoping but does not surface the single-session-user case, which is useful framing for understanding who is affected and who already sees correct behaviour.

**Current**:
"The bug surfaces only on **cold-start** bootstrap (first `portal` invocation after the tmux server starts), because `internal/restore/restore.go` skips already-live sessions on subsequent invocations. The cardinality is 'once per tmux-server lifetime, affecting all-saved-sessions-minus-one' — not 'once per `portal open`'."

**Proposed Addition**:
{Add a sentence noting that single-saved-session users are unaffected (one session = one attach = one signal; no panes left unsignaled), and that hot-path `portal open <existing-session>` after the cold-start bootstrap completes is also unaffected (Restore skips live sessions; no skeleton, no helpers).}

**Resolution**: Pending
**Notes**:

---

### 5. Rejected wrapper-redesign options not captured in out-of-scope

**Source**: Investigation "Options Explored" (lines 244-245)
**Category**: New topic (or Enhancement to "What is explicitly out of scope")
**Affects**: Fix Scope → What is explicitly out of scope, or Fix 3 → Why the Outer Wrapper Is Removable

**Details**:
The investigation explicitly enumerates and rejects two wrapper-redesign alternatives that the spec does not record:
- "Wrapper redesign — keep wrapper, use `exec` inside (`sh -c 'exec portal state hydrate ...'`)" — rejected because same correctness as dropping entirely with no upside.
- "Status quo signaling; only fix what happens after timeout" — rejected because Symptom C is structurally hard to fix from the timeout side (marker has to outlive timeout to mean anything).

The spec captures the panic-resilience wrapper and "remove a hook registration" rejections but not these two. Recording them prevents future re-litigation.

**Proposed Addition**:
{Add to "What is explicitly out of scope":
- Wrapper redesign that keeps the outer `sh -c` envelope (e.g. `sh -c 'exec portal state hydrate ...'`) — same correctness as dropping the wrapper, no upside, more complex.
- Status-quo per-session signaling with timeout-path-only corrections — rejected because Symptom C cannot be fixed from the timeout side; the marker has to outlive the timeout to suppress scrollback save, so the production-layer fix (eager signaling) is required to close that gap.}

**Resolution**: Pending
**Notes**:

---

### 6. Helper success-path also leaves parked `sh` parent (wrapper drop benefit broader than timeout)

**Source**: Investigation "Defect D" (lines 181-185, especially line 184: "Same on the success path — the helper exec's $SHELL inside `execShellAndExit` / `execShellOrHookAndExit`, so the wrapper sh is also parked across normal hydration.")
**Category**: Enhancement to existing topic
**Affects**: Fix 3 → Side Effects, or Problem Statement → Defect D

**Details**:
The spec's Defect D framing is "orphan `sh -c` wrapper parked on every restored pane". The investigation explicitly clarifies the orphan parent persists on the **success path too**, not only on timeout — both `execShellAndExit` and `execShellOrHookAndExit` `syscall.Exec` $SHELL inside the wrapper, leaving the wrapper sh as a parked parent for the lifetime of the pane on every hydration outcome. The spec's Fix 3 → Side Effects line ("every restored pane currently leaves a parked `sh` parent under tmux for the lifetime of the pane") captures the breadth, but the Problem Statement framing ("Defect D — orphan `sh -c` wrapper parked on every restored pane") could be sharper if it explicitly states this is independent of hydration outcome.

**Current**:
"**Defect D — orphan `sh -c` wrapper** parked on every restored pane: the wrapper's trailing `; exec $SHELL` is unreachable on success, breaks pane-close-on-`exit`, and leaves a parked `sh` parent under tmux for the lifetime of the pane."

**Proposed Addition**:
{Tighten Defect D's wording to make explicit that the parked `sh` parent appears on **every** hydration outcome (success, file-missing, timeout) — not only on the timeout path the investigation addendum first surfaced it on. Both `execShellAndExit` and `execShellOrHookAndExit` exec the user's shell from inside the wrapper, parking the outer sh as the pane's lifetime-bound parent regardless of which exit path the helper took.}

**Resolution**: Pending
**Notes**:

---

### 7. Reproduction steps from the investigation not surfaced as a verification protocol

**Source**: Investigation "Reproduction Steps" (lines 33-42)
**Category**: Gap/Ambiguity
**Affects**: Test Plan (or Risks & Rollout → Empirical Reconfirmation)

**Details**:
The investigation provides a six-step manual reproduction protocol (boot to trigger restore → observe portal.log warnings → inspect server-options for stuck markers → kill an affected session → run `portal open` → check on-resume hook). The spec's "Empirical Reconfirmation Before Implementation Starts" mentions checking Symptom A against current `main` but does not surface the step-by-step protocol. As a manual verification gate (in addition to the automated test plan), this reproduction protocol is the canonical way to confirm pre-fix state and post-fix resolution on a developer's own machine.

**Proposed Addition**:
{Add a "Manual Verification Protocol" subsection to either Test Plan or Risks & Rollout, restating the investigation's 6-step reproduction so post-implementation verification on a real machine has a documented checklist. Pre-fix: the steps reproduce all four symptoms; post-fix: every step's failure mode is absent (no stuck markers, no warnings, killed sessions stay killed, on-resume hook fires, single `exit` closes the pane).}

**Resolution**: Pending
**Notes**:
