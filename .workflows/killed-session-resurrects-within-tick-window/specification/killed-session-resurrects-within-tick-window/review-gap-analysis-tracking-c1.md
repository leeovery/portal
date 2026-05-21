---
status: complete
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
The "Where the Synchronous Commit Lives" subsection said the entry-point shape was "the open design decision documented under 'Entry-Point Design Decision' below" — but the next section resolves it. Forward-reference phrasing made the resolved choice read as open.

**Proposed Addition**:
Tighten the cross-reference: "See § Entry-Point Design Decision for the resolved shape (`portal state commit-now`)."

**Resolution**: Approved
**Notes**: Applied verbatim. Forward-reference replaced with resolved-shape pointer.

---

### 2. `commit-now` failure → `save.requested` belt-and-braces touch left unresolved

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`commit-now` Failure Behaviour"

**Details**:
Spec deferred to plan whether `commit-now` should also touch `save.requested` on failure. Decision had testable consequences; needed settling in spec.

**Proposed Addition**:
Resolved as option (b): `commit-now` touches `save.requested` only on failure or skip exit paths, not on successful sync commit. Added new "`save.requested` Discipline" subsection.

**Resolution**: Approved
**Notes**: Option (b) chosen. Rationale: keeps common path (successful sync commit) free of redundant daemon work while guaranteeing bounded recovery on every error path. Without the touch on failure, the daemon's `dirty || gap` rule could delay recovery up to 30s. Subsection also addresses Finding 3 (restoring-window touch behaviour).

---

### 3. `@portal-restoring` short-circuit: does it touch `save.requested`?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`@portal-restoring` Defence (Required)"

**Details**:
Spec didn't say whether the restoring-window no-op also skipped touching `save.requested`. Without it, the daemon could skip ticks post-restoration via the gap rule.

**Proposed Addition**:
Addressed by the same "`save.requested` Discipline" subsection added for Finding 2 — the restoring-window short-circuit **does** touch `save.requested` so the daemon's first post-restoration tick captures.

**Resolution**: Approved
**Notes**: Co-resolved with Finding 2.

---

### 4. Re-entrancy fallback strategy left as plan-phase contingency

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "Hook Re-entrancy (Validate in Plan/Implementation Phase)"

**Details**:
Spec listed two possible fallback shapes (`tmux run-shell` vs subprocess detach) without choosing, conditioned on a test that hasn't run yet.

**Proposed Addition**:
Chose option (b): explicitly declared that re-entrancy test failure is a respec trigger, not an implementation-phase pivot. Pre-locking an unvalidated fallback would commit to a design whose merits can't yet be weighed.

**Resolution**: Approved
**Notes**: Test required to pass before implementation work is considered complete.

---

### 5. `CaptureStructure` call shape for `commit-now` (PrevIndex argument)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Fix Approach → "Mechanism" and § Cost Profile

**Details**:
Spec didn't pin `PrevIndex` argument or `anyScrollbackChanged` value for `state.Commit`.

**Proposed Addition**:
Pinned: `PrevIndex` is loaded from existing `sessions.json` via `state.ReadIndex` (preserves scrollback-hash fields for live sessions; dead sessions still filtered by `mergeSkippedPanes`). `anyScrollbackChanged` is hard-coded `false`.

**Resolution**: Approved
**Notes**: Reading prev from disk is the cleanest choice — preserves schema fidelity for live sessions and keeps semantic equivalence with daemon output post-fix (see Finding 11).

---

### 6. Hook migration removal mechanism not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Hook Registration Migration → "Idempotency Requirements"

**Details**:
"Remove stale notifyCommand" was framed as an extension of the existing append-if-absent discipline, but it's new logic. Implementer might invent an incompatible approach.

**Proposed Addition**:
Added new "Migration Algorithm" subsection spelling out: `ShowGlobalHooks` → scan → match body against pre-fix `notifyCommand` pattern → `UnsetGlobalHookAt` (highest-index first) → re-scan → `AppendGlobalHook(commitNowCommand)` if absent.

**Resolution**: Approved
**Notes**: Explicit highest-index-first ordering for `UnsetGlobalHookAt` to avoid index shift bugs.

---

### 7. Log component/destination for `commit-now` failures and skips unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`@portal-restoring` Defence" and "`commit-now` Failure Behaviour"; § Acceptance Criteria item 7

**Details**:
"ComponentBootstrap or equivalent component" was vague; destination for hook-subprocess stderr was undefined.

**Proposed Addition**:
Added new "Logging Discipline" subsection: file-backed state-package structured logger (same sink the daemon uses); same component constant the daemon uses for `sessions.json` captures (no renaming).

**Resolution**: Approved
**Notes**: Deliberately not naming a specific constant; the spec defers to whichever the daemon currently uses, which is unambiguous at implementation time.

---

### 8. `_portal-saver` self-kill: two timelines collapsed into one

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases → "`_portal-saver` Self-Kill"

**Details**:
Bootstrap-time (marker set → short-circuit) and steady-state (marker clear → filter) were covered in different sections without distinguishing them as a dual timeline.

**Proposed Addition**:
Restructured "`_portal-saver` Self-Kill" subsection to explicitly enumerate both timelines, each with its protecting mechanism. Stated they are orthogonal — neither subsumes the other.

**Resolution**: Approved
**Notes**: Sets up clean test design — acceptance 5/5a (Finding 10) and the integration tests can target each timeline separately.

---

### 9. Concurrent `commit-now` invocations not acknowledged

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Invariants & Edge Cases (missing edge case)

**Details**:
Rapid-fire kills spawn concurrent `commit-now` subprocesses. Atomic temp+rename makes this safe, but spec didn't acknowledge the scenario.

**Proposed Addition**:
Added new "Concurrent `commit-now` Invocations (Safe by Atomic Rename)" subsection: last-writer-wins via atomic rename; each winner reflects a real moment of post-kill tmux state; no additional locking required.

**Resolution**: Approved
**Notes**: Closes the "missing lock" question future readers might ask.

---

### 10. Acceptance criterion 5 ambiguous about which `_portal-saver` kill timeline

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Acceptance Criteria → item 5

**Details**:
Criterion 5 bundled steady-state and bootstrap-time scenarios with different mechanisms into one acceptance.

**Proposed Addition**:
Split into 5 (steady-state, `keepSessionNames` filter) and 5a (bootstrap version-upgrade, `@portal-restoring` short-circuit asserts byte-identical pre/post).

**Resolution**: Approved
**Notes**: Test author now has unambiguous separate criteria for each timeline.

---

### 11. Daemon next-tick output equivalence claim under-specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Testing Requirements → Regression Tests → "Daemon merge stability after `commit-now`"

**Details**:
"Byte-equivalent" claim may be false even when both writers are correct — the daemon may legitimately repopulate scrollback hashes that `commit-now` carries over from prev.

**Proposed Addition**:
Restated both Acceptance 10 and the corresponding regression test as **semantic equivalence on the session-name set**, not byte-equivalence. The invariant is "dead sessions stay out; live sessions stay in," not byte-for-byte file identity.

**Resolution**: Approved
**Notes**: With Finding 5's resolution (commit-now reads prev from disk), live sessions in commit-now output already carry their existing scrollback hashes — so semantic equivalence is achievable even though byte equivalence is not guaranteed across the daemon's enrichment pass.

---
