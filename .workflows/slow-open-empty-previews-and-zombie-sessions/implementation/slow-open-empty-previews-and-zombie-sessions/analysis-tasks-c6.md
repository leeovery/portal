# Analysis Tasks — Cycle 6 (Phase 11 re-scan)

STATUS: tasks_proposed
TASKS_PROPOSED: 1

## Convergence note

Duplication and standards came back clean. Architecture flagged one medium-severity finding — a spec-prose residual at specification.md:365 that contradicts the T11-3-amended Component F bullet 3 / Note. The T11-3 reviewer flagged this exact line as non-blocking; cycle-5 re-scan confirms it remains.

---

## Task 1 — Reconcile Component F design-rationale prose at specification.md:365 with the amended acceptance bullet 3 / Note

- status: pending
- severity: medium
- sources: architecture-c6

### Problem

T11-3 amended Component F's acceptance bullet 3 (specification.md:391) and added a Note (specification.md:394) that establish the observable contract as **log-noise absence**, and explicitly document that on tmux 3.6b `_portal-saver` DOES disappear when the lock-loser daemon pane process exits even with `destroy-unattached=off`. The amendment demoted literal session-persistence to a future opt-in.

However, "New behaviour" step 3 at specification.md:365 still ends with: *"Even if the daemon exits immediately as lock-loser, `destroy-unattached=off` is already in effect, so the session persists for the next bootstrap to evaluate."* That trailing clause asserts the literal session-persistence outcome the amendment demoted. A top-to-bottom reader of Component F encounters the contradiction within a single component.

### Solution

Edit specification.md:365 so the design-rationale prose matches the amended observable contract (log-noise absence) rather than asserting literal session-persistence. Preserve the surrounding sentence structure and load-bearing facts (placeholder kept session alive, `destroy-unattached=off` is set, respawn replaces the placeholder). Replace only the trailing clause about persistence with language that (a) states the log-noise-absence outcome and (b) cross-references the acceptance criterion + Note for tmux-version-specific behaviour.

### Outcome

Component F reads consistently top-to-bottom. Design-rationale at line 365 and acceptance criterion at line 391 (plus the Note at line 394) assert the same observable contract. The literal-session-persistence claim is absent from the design-rationale prose and lives only in the Note as a documented future opt-in. No code changes; no other spec sections touched.

### Do

1. Open `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` at line 365.
2. Locate the trailing clause: *"Even if the daemon exits immediately as lock-loser, `destroy-unattached=off` is already in effect, so the session persists for the next bootstrap to evaluate."*
3. Replace with prose along the lines of: *"so the lock-loser cascade is quiet — every `BootstrapPortalSaver` tmux call targets an extant session and no `no such session` log entries are produced. (Literal session-persistence after daemon exit is a separate concern; see acceptance criterion 3 and the Note below for tmux-version-specific behaviour.)"*
4. Verify surrounding sentences in step 3 (about `-k` flag, placeholder replacement, pane survival) are unchanged.
5. Re-read lines 361–394 and confirm steps 3, 4, the acceptance bullets, and the Note tell a single coherent story.

### Acceptance Criteria

- specification.md:365 no longer contains "the session persists for the next bootstrap to evaluate" or any equivalent literal-persistence claim.
- specification.md:365 references the log-noise-absence contract and points readers at acceptance criterion 3 / the Note.
- Acceptance bullet 3 at specification.md:391 and the Note at specification.md:394 are unchanged.
- No other spec sections (Components A, B, C, D, E, G) are touched.
- No code files are touched.

### Tests

Spec-only edit; no executable tests. Validation is by re-reading Component F top-to-bottom and confirming internal consistency between design-rationale prose, acceptance criterion 3, and the Note.
