---
status: complete
created: 2026-05-21
cycle: 2
phase: Gap Analysis
topic: killed-session-resurrects-within-tick-window
---

# Review Tracking: killed-session-resurrects-within-tick-window - Gap Analysis

## Findings

### 1. `state.ReadIndex` failure / missing-file behaviour unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Fix Approach → Mechanism (step 1); § `commit-now` Failure Behaviour

**Details**:
Mechanism step 1 didn't address (a) `sessions.json` not existing yet (fresh install), or (b) `sessions.json` existing but unreadable/corrupt.

**Proposed Addition**:
Added inline subsection "`PrevIndex` resolution failure modes" to Mechanism step 1: both missing-file and decode-error cases fall through to zero-value `PrevIndex`, WARN-level log, and successful commit. `ReadIndex` failure is **not** a `commit-now` failure exit.

**Resolution**: Approved
**Notes**: Rationale: the synchronous path's primary goal (removing killed session from sessions.json) is satisfied regardless of PrevIndex availability. Daemon repopulates scrollback hashes on next tick.

---

### 2. Migration pattern-match strictness for stale `notifyCommand` removal

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Hook Registration Migration → Migration Algorithm step 2

**Details**:
"Matches the pattern" was ambiguous between exact-string and loose/regex match.

**Proposed Addition**:
Chose **exact-string match** against the historical `notifyCommand` literal. Same exact-string discipline for the `commitNowCommand` presence check in step 3. Added rationale paragraph explaining why (avoid false-positive removal of user-customised hooks).

**Resolution**: Approved
**Notes**: Spec assumes a single historical `notifyCommand` literal; if a future audit reveals multiple historical literals, the match set is extended.

---

### 3. Short-circuit exit code unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § `@portal-restoring` Defence; § `commit-now` Failure Behaviour; § `save.requested` Discipline

**Details**:
Failure exit was non-zero; short-circuit exit code was unstated.

**Proposed Addition**:
Added "Exit Code Summary" table: successful commit = 0, short-circuit = 0, failure = non-zero. Inline exit codes added to `save.requested` Discipline bullets.

**Resolution**: Approved
**Notes**: Deliberate skip is not an error condition; exit 0 matches semantics.

---

### 4. `save.requested` touch failure not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § `commit-now` Failure Behaviour; § `save.requested` Discipline

**Details**:
What happens if the `save.requested` touch itself fails (disk full, permission denied).

**Proposed Addition**:
Added "`save.requested` Touch Failure Handling" subsection: best-effort touch, log via state logger, do not nest error handling, do not panic, do not propagate. Original `commit-now` outcome dominates the exit code (failure paths still exit non-zero; short-circuit paths still exit 0). Daemon's `gap` rule is the worst-case fallback if both layers fail.

**Resolution**: Approved
**Notes**: Recovery-of-last-resort. Spec accepts that if both layers fail, the next Portal bootstrap captures fresh state.

---

### 5. `ShowGlobalHooks` primitive choice left open

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Hook Registration Migration → Migration Algorithm step 1

**Details**:
"Or the existing per-event variant" left implementer to guess which primitive to use.

**Proposed Addition**:
Pinned: `ShowGlobalHooks` is the enumeration primitive; filtering by event happens in-process. Removed the speculative "per-event variant" reference.

**Resolution**: Approved
**Notes**: Per CLAUDE.md, `ShowGlobalHooks` is the listed primitive; no per-event variant is assumed.

---

### 6. Acceptance criterion numbering: duplicate `5`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Acceptance Criteria → Functional Acceptance

**Details**:
Criteria `5` and `5a` shared a number; subsequent criteria stayed at 6–12.

**Proposed Addition**:
Renumbered `5a` → `6` and shifted all subsequent criteria up by one (6 → 7, 7 → 8, 8 → 9, 9 → 10, 10 → 11, 11 → 12, 12 → 13). Result: clean monotonic numbering 1–13.

**Resolution**: Approved
**Notes**: Plan/implementation phases now have unambiguous references.

---

### 7. Bootstrap-step-4 `save.requested` side effect not called out as benign

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § `_portal-saver` Self-Kill; § `save.requested` Discipline

**Details**:
Every bootstrap step 4 version-upgrade now queues a daemon commit for post-restoration. Worth explicitly stating this is benign and desired.

**Proposed Addition**:
Added paragraph to Timeline 1 of "`_portal-saver` Self-Kill" subsection: the dirty-flag touch during the short-circuit promotes the earliest post-restoration daemon tick from "skip" to "commit," shortening worst-case stale window from 30s (gap rule) to 1s (next ticker fire). No user-visible extra cost.

**Resolution**: Approved
**Notes**: Heads off "isn't every bootstrap now triggering an extra tick?" reader concern.

---
