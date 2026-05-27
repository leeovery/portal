---
status: complete
created: 2026-05-27
cycle: 1
phase: Input Review
topic: bootstrap-cleanstale-wipes-hooks-on-tmux-transient
---

# Review Tracking: bootstrap-cleanstale-wipes-hooks-on-tmux-transient - Input Review

## Findings

### 1. Audit Conclusion — Blast Radius Bounded to Two Callsites

**Source**: investigation §"Blast Radius — Other `ListAllPanes` Callers" (lines 205-218); §"Audit Scope" (lines 92-94)
**Category**: Enhancement to existing topic
**Affects**: Fix Specification (Defect Class Scope subsection)

**Resolution**: Approved
**Notes**: Added new "### Defect Class Scope (Audit Result)" subsection immediately after the Fix Specification opening, naming the two bounded callsites and forward-referencing the Audited Reference Set.

---

### 2. Trigger Windows of Highest Empirical Risk

**Source**: investigation §"Environment" / "Trigger windows of highest empirical risk" (lines 43-48)
**Category**: Enhancement to existing topic
**Affects**: Problem Statement (new "Trigger Windows of Highest Empirical Risk" subsection)

**Resolution**: Approved
**Notes**: Added new subsection enumerating all four documented windows (saver-respawn, version-upgrade kill cycle, heavy load, observed Component B WARN race) plus a sentence locating step 11 inside the recovery-window tail.

---

### 3. Relationship to v0.5.11 `hooks-skip-bootstrap` Quickfix

**Source**: investigation §"Relationship to Recent Releases" (lines 96-98)
**Category**: New topic (small)
**Affects**: Notes

**Resolution**: Approved
**Notes**: Added a Notes bullet making the frequency-vs-latency distinction explicit and cross-referencing the Phase-4 subtest-inversion pattern already cited in Test Requirements.

---

### 4. Adapter Docstring Update at `cleanStaleAdapter`

**Source**: investigation §"Code Trace" (lines 116-131, esp. line 131)
**Category**: Enhancement to existing topic
**Affects**: Fix Specification — Change 3

**Resolution**: Approved
**Notes**: Added an "Adapter docstring rewrite" paragraph inside Change 3 quoting the existing misleading docstring and specifying the three new contract points the rewrite must describe.

---

### 5. Prior-Art Comment-Block Lift Alongside Code Lift

**Source**: investigation §"The Prior-Art Sibling — Why Step 9 Is Not Vulnerable" (lines 178-202, esp. lines 196-201)
**Category**: Enhancement to existing topic
**Affects**: Fix Specification — Change 3

**Resolution**: Approved
**Notes**: Extended the "mirrors the prior-art" sentence with an explicit instruction to lift the load-bearing comment block at stale_marker_cleanup.go:80-92 alongside the code, adapted to name hook entries as the protected data.

---

### 6. Deterministic Repro Mechanism — `Commander` Injection

**Source**: investigation §"Reproduction Steps" (lines 32-38)
**Category**: Enhancement to existing topic
**Affects**: Test Requirements — Integration sub-sections

**Resolution**: Approved
**Notes**: Added a new "Deterministic Repro Mechanism" subsection naming Commander-level injection as canonical, and updated the Integration — Tmux Transient Simulation entry to forward-reference it.

---

### 7. Audited `ListAllPanesWithFormat` Consumers as Non-Defective Reference Set

**Source**: investigation §"Blast Radius — Other `ListAllPanes` Callers" (lines 212-216)
**Category**: New topic (small / informational)
**Affects**: Fix Specification (new "Audited Reference Set" subsection)

**Resolution**: Approved
**Notes**: Added a new "### Audited Reference Set — Correct-by-Construction Consumers" subsection under Fix Specification listing all three ListAllPanesWithFormat consumers and grounding the architectural-consistency argument.

---

### 8. Historical Near-Miss — `saver-kill-respawn-loop-leaks-daemons` Investigation

**Source**: investigation §"Why It Wasn't Caught" (lines 245-246)
**Category**: New topic (small / historical)
**Affects**: Notes

**Resolution**: Approved
**Notes**: Added a Notes bullet recording the 2026-05-19 near-miss to anchor post-mortem timing.

---
