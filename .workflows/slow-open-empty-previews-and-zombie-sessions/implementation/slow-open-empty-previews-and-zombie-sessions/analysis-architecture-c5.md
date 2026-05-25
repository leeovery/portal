# Architecture Analysis — Cycle 5 (Phase 11 re-scan)

AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 1

## Findings

- FINDING: Spec line 365 contradicts the amended Component F bullet 3 (T11-3 residual)
  SEVERITY: medium
  FILES: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md:365`, same file lines 391 and 394
  DESCRIPTION: T11-3 amended Component F's acceptance bullet 3 (line 391) and added a Note (line 394) that explicitly establish the observable contract as log-noise absence, both passages stating that on tmux 3.6b `_portal-saver` DOES disappear when the lock-loser daemon pane exits — even with `destroy-unattached=off`. The "New behaviour" step 3 at line 365 still reads: *"Even if the daemon exits immediately as lock-loser, `destroy-unattached=off` is already in effect, so the session persists for the next bootstrap to evaluate."* That trailing clause asserts the literal session-persistence outcome the amendment explicitly demoted to a future opt-in. A reader top-to-bottom encounters the contradiction within Component F: the design-rationale claims persistence; the acceptance criterion and Note disclaim it. The reviewer flagged this exact line as a non-blocking residual of T11-3 — it remains. Phase 11's amendment is incomplete: the new contract is asserted in the acceptance criteria but not yet propagated to the design-rationale prose that introduces the ordering.
  RECOMMENDATION: Edit line 365 to match the amended contract. Replace the trailing clause with something like: *"so the lock-loser cascade is quiet — every `BootstrapPortalSaver` tmux call targets an extant session and no `no such session` log entries are produced. (Literal session-persistence after daemon exit is a separate concern; see acceptance criterion 3 and the Note for tmux-version-specific behaviour.)"* Aligns design-rationale text with the acceptance criterion without re-architecting Component F.

## Non-findings (evaluated, no architectural concern)

- **T11-1 (test rename + WARN flip).** Production at `cmd/state_daemon.go:213` emits at WARN; the test now asserts WARN. Rename encodes the level in the name.
- **T11-2 ((paths, bool) return).** Test-only helper; ENOENT-at-root is a single recognised domain state, so a bool is more honest than a sentinel error. Docstring's three-shape return contract is explicit.
- **T11-3 amendment text.** Bullet 3 + Note correctly capture observed tmux 3.6b behaviour and flag remain-on-exit as a future opt-in. The residual at line 365 is the only blocker.
- **T11-4 (symmetric daemon.version stat).** Defensive add; T4-8 AST adjacency test remains the load-bearing invariant pin.

## Summary

T11-1 / T11-2 / T11-4 are architecturally clean. T11-3 left a real residual at specification.md:365 where the design-rationale prose still claims session persistence and contradicts the amended bullet 3 / Note.
