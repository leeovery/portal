---
status: in-progress
created: 2026-05-21
cycle: 1
phase: Gap Analysis
topic: killed-session-resurrects-within-tick-window
---

# Review Tracking: killed-session-resurrects-within-tick-window - Gap Analysis

## Findings

### 1. Stale "open design decision" framing for entry-point shape

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Fix Approach → "Where the Synchronous Commit Lives"

**Details**:
The "Where the Synchronous Commit Lives" subsection says: "The exact entry-point shape (new sibling subcommand vs new flag on `notify`) is the open design decision documented under 'Entry-Point Design Decision' below." But the very next top-level section resolves that decision (`portal state commit-now`). The forward-reference phrasing makes it read as if the decision is still open when in fact it's settled. A future reader skimming the Fix Approach will be confused about whether they're allowed to pick.

**Proposed Addition**:
Tighten the cross-reference: replace "the open design decision documented" with something like "see § Entry-Point Design Decision for the resolved shape (`portal state commit-now`)".

**Resolution**: Pending
**Notes**:

---

### 2. `commit-now` failure → `save.requested` belt-and-braces touch left unresolved

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`commit-now` Failure Behaviour"

**Details**:
The spec says: "this implies `commit-now` may also touch `save.requested` as a belt-and-braces fallback before exiting — open implementation detail flagged for the plan phase." This is a real decision (does the synchronous path *also* set the dirty flag on failure, yes/no?) that an implementer would have to make before writing code. Per the project's discussion-vs-spec discipline, decisions should be settled in spec, not deferred to plan/implementation. The two answers have different testable consequences (acceptance 7 + the regression-test interaction with `save.requested`).

**Proposed Addition**:
Resolve: either (a) `commit-now` touches `save.requested` unconditionally before its own work as a pre-commit belt-and-braces (so a panic/crash still leaves the daemon with a dirty flag to act on), or (b) `commit-now` touches `save.requested` only on its own failure exit path, or (c) `commit-now` never touches `save.requested` (the daemon's tick still re-queries tmux on schedule and would notice the stale `sessions.json` independently). Pick one; capture rationale in the spec.

**Resolution**: Pending
**Notes**:

---

### 3. `@portal-restoring` short-circuit: does it touch `save.requested`?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`@portal-restoring` Defence (Required)"

**Details**:
When `commit-now` no-ops because the restoration marker is set, the spec says it returns immediately and logs. It does not say whether `save.requested` is touched in that path. If skipped, a kill that fires during restoration produces no synchronous write *and* no dirty flag — the daemon's next tick will still capture from live tmux and notice the kill, so correctness is preserved by re-query, but the dirty flag is normally what schedules an *earlier* tick. Worth being explicit about this: the no-op is total (no file work, no flag touch) and that's fine because the daemon's tick is unconditional.

**Proposed Addition**:
Add one sentence to the `@portal-restoring` defence subsection clarifying whether the no-op also skips touching `save.requested`, and why that's safe (the daemon's tick runs unconditionally on its 1-second period and will pick up the post-kill state on its next pass anyway).

**Resolution**: Pending
**Notes**:

---

### 4. Re-entrancy fallback strategy left as plan-phase contingency

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "Hook Re-entrancy (Validate in Plan/Implementation Phase)"

**Details**:
The spec says: "If re-entrancy issues surface, the fix must defer the tmux calls via `tmux run-shell` or detach the subprocess differently — the user-visible behaviour must remain ..." This is a conditional design decision: the spec lists two possible fallback shapes (`tmux run-shell` vs. subprocess detachment) without choosing between them, and conditions the choice on a test outcome that hasn't been run yet. An implementer hitting a re-entrancy hang would not know which fallback to reach for. Either: (a) commit to one fallback now so the implementer doesn't have to redesign mid-flight, or (b) acknowledge that on test failure the work-unit returns to the discussion/spec phase, not the implementation phase.

**Proposed Addition**:
Pick one of:
- Specify a primary fallback shape (e.g., "`tmux run-shell` is the chosen fallback if re-entrancy issues surface; the synchronous path becomes a fire-and-forget `run-shell` invocation").
- State explicitly that re-entrancy test failure is a re-spec trigger, not an implementation-phase pivot.

**Resolution**: Pending
**Notes**:

---

### 5. `CaptureStructure` call shape for `commit-now` (PrevIndex argument)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Fix Approach → "Mechanism" and § Cost Profile

**Details**:
The spec says `commit-now` "captures the current structural index via the existing `state.CaptureStructure`" and reuses `state.Commit(dir, idx, anyScrollbackChanged, logger)`. But `CaptureStructure` (as used by the daemon) takes a `PrevIndex` for dedup / merge behaviour. `commit-now` is a short-lived subprocess with no in-memory `PrevIndex` — does it pass nil? Does it read the existing `sessions.json` to seed `PrevIndex`? Does it call a variant that doesn't take one? Similarly, what value does it pass for `anyScrollbackChanged`? (Presumably `false`, since the sync path is structural-only and writes no `.bin` files — but the spec doesn't say.) An implementer would need to read daemon source to guess. Pinning these call-site details closes the gap.

**Proposed Addition**:
Add to "Mechanism": specify the `CaptureStructure` call shape `commit-now` uses (PrevIndex = nil, or = freshly-loaded from disk, or = zero value), and the `state.Commit` arguments (specifically `anyScrollbackChanged` = `false` because no `.bin` writes happen in this path).

**Resolution**: Pending
**Notes**:

---

### 6. Hook migration removal mechanism not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Hook Registration Migration → "Idempotency Requirements"

**Details**:
The spec says `RegisterPortalHooks` "must remove the stale `notifyCommand` registration from `session-closed`" during upgrade. The existing `RegisterPortalHooks` only knows how to *append* idempotently — adding a "remove an existing matching hook line" operation is new logic, not "the same idempotency discipline ... extended." The codebase exposes `UnsetGlobalHookAt(event, index)` (per CLAUDE.md), which implies removal must happen by index after a scan. The spec should commit to the algorithm: scan `show-hooks -g session-closed`, identify the entry whose body matches the pre-fix `notifyCommand`, call `UnsetGlobalHookAt` for that index, then append `commitNowCommand` if absent. Otherwise an implementer might invent a different (incompatible) approach.

**Proposed Addition**:
In "Idempotency Requirements", spell out the migration algorithm in terms of existing tmux client methods (`ShowGlobalHooks` + match by hook body + `UnsetGlobalHookAt` + `AppendGlobalHook`), so the upgrade path is unambiguous.

**Resolution**: Pending
**Notes**:

---

### 7. Log component/destination for `commit-now` failures and skips unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`@portal-restoring` Defence" and "`commit-now` Failure Behaviour"; § Acceptance Criteria item 7

**Details**:
Two places refer to logging:
- The `@portal-restoring` skip "log[s] at INFO under `ComponentBootstrap` or an equivalent component" — the "or an equivalent component" is vague; the implementer must pick. Production uses structured component constants; a wrong pick clutters logs.
- Acceptance 7 says failures are "logged and Portal proceeds." But `commit-now` runs as a tmux hook subprocess — there's no attached terminal or in-process logger consumer. The spec doesn't define where its stderr/log goes (state-logger file? tmux's `display-message` buffer? lost to the void?).

The fix is small but specifically: pick a component constant (likely `ComponentState` or `ComponentDaemon` — `ComponentBootstrap` is questionable since this path runs outside bootstrap), and confirm the logger destination is the existing state-logger sink (a file under the state dir), not stdio.

**Proposed Addition**:
Specify the logger component constant `commit-now` uses (one constant, no "or equivalent") and confirm the log destination is the existing structured state logger (file-backed), not the hook subprocess's stdio. Reference this from acceptance criterion 7.

**Resolution**: Pending
**Notes**:

---

### 8. `_portal-saver` self-kill: two timelines collapsed into one

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`_portal-saver` Self-Kill (Documented, No Code Change)" vs. "`@portal-restoring` Defence"

**Details**:
The spec describes the `_portal-saver` self-kill safely in two places that imply different timelines:
- "`@portal-restoring` Defence" rationale: "`_portal-saver` version-upgrade (bootstrap step 4) can fire `session-closed` while restoration is still in progress. A synchronous commit at that moment would write a partial skeleton state ..." → handled by short-circuit.
- "`_portal-saver` Self-Kill" subsection: "Steady-state user-kill of `_portal-saver` triggers the `session-closed` hook, which runs `commit-now`. `state.CaptureStructure`'s `keepSessionNames` filter excludes underscore-prefixed sessions ..." → handled by filter.

Both are correct but cover *different* timelines: bootstrap-time (marker set, short-circuit fires) vs. steady-state (marker clear, filter fires). A reader could mistakenly conclude one mechanism handles both, or that the two are alternatives. Worth one explicit sentence calling out the dual timeline.

**Proposed Addition**:
In the "`_portal-saver` Self-Kill" subsection, add a sentence distinguishing the two timelines: during bootstrap step 4 the `@portal-restoring` short-circuit prevents the write; in steady-state (marker clear) the `keepSessionNames` filter ensures `_portal-saver` is omitted. Both protections are required and orthogonal.

**Resolution**: Pending
**Notes**:

---

### 9. Concurrent `commit-now` invocations not acknowledged

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases (missing edge case)

**Details**:
Rapid-fire kills (e.g., a user killing two sessions in quick succession, or a script-driven mass kill) will fire `session-closed` twice in rapid succession, spawning two concurrent `commit-now` subprocesses. Each calls `state.CaptureStructure` (reads live tmux — fine; tmux server serializes these) and `state.Commit` (temp file + rename — atomic on POSIX). The atomic rename means the on-disk file is always one of N consistent snapshots, but the spec doesn't acknowledge this scenario at all. Worth a sentence under Invariants & Edge Cases so future readers don't worry about a "missing lock" between concurrent commit-now processes.

**Proposed Addition**:
Add a short subsection acknowledging concurrent `commit-now` invocations: the atomic temp+rename of `state.Commit` makes concurrent writes safe (last writer wins; on-disk file is always one consistent snapshot of live tmux at some recent moment). No additional locking required.

**Resolution**: Pending
**Notes**:

---

### 10. Acceptance criterion 5 ambiguous about which `_portal-saver` kill timeline

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Acceptance Criteria → item 5

**Details**:
Acceptance 5: "The `session-closed` hook firing for `_portal-saver` (during steady-state user kill or bootstrap step 4 version-upgrade) must not corrupt `sessions.json`." It bundles two scenarios with very different mechanisms (filter vs. short-circuit) into one criterion. A test author would need to write two separate tests with two different setups. The criterion should split or explicitly call out that both timelines are covered by the test plan.

**Proposed Addition**:
Either split into 5a (steady-state, marker clear, filter must omit `_portal-saver`) and 5b (bootstrap step 4, marker set, short-circuit no-ops), or rewrite criterion 5 to enumerate the two scenarios with their respective expected mechanisms.

**Resolution**: Pending
**Notes**:

---

### 11. Daemon next-tick output equivalence claim under-specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Testing Requirements → Regression Tests → "Daemon merge stability after `commit-now`"

**Details**:
Regression test: "After a `commit-now` write, the daemon's next tick must produce a `sessions.json` that is byte-equivalent to the `commit-now` output." Byte-equivalence is a strong claim that may be false even when both writers are correct. The daemon's tick writes scrollback hashes (`anyScrollbackChanged`-related fields, content-hashes, possibly timestamps), whereas `commit-now` produces a structural-only output with no scrollback content. If the schema includes scrollback-hash fields that `commit-now` leaves empty (or zeroed), the daemon's first tick after `commit-now` *will* differ from the `commit-now` output. The spec should clarify whether: (a) the schema is uniform and `commit-now` writes the same shape, or (b) the regression test compares only structural fields, or (c) the daemon overwrites with a richer file and the symptom (killed-session-absent) is the actual invariant, not byte equivalence.

**Proposed Addition**:
Restate the regression-test invariant as "the killed session does not reappear in any subsequent tick output" (a semantic equivalence on the session set), not "byte-equivalent." If true byte-equivalence is intended, specify how `commit-now` produces a file that includes scrollback-hash fields without doing scrollback work (e.g., by reading the previous on-disk file's hash values and preserving them).

**Resolution**: Pending
**Notes**:

---
