TASK: 9-4 — Promote ReadPortalLogSafe to internal/portaltest

STATUS: Complete

SPEC CONTEXT: c3 duplication — two byte-identical wrappers around `os.ReadFile(state.PortalLog(stateDir))` with identical failure placeholder strings.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/portaltest/portal_log.go` (new, 43 LOC; `ReadPortalLogSafe` at line 36)
  - `cmd/state_daemon_self_supervision_integration_test.go` — 11 call sites migrated
  - `cmd/bootstrap/composition_e2e_self_eject_integration_test.go` — 2 call sites migrated
- Placeholder `(read portal.log failed: %v)` preserved verbatim
- Helper deliberately omits `*testing.T`; rationale documented in file header (13-17): pure read-and-format, no test-state side effects
- No leftover local defs

TESTS:
- Status: Adequate (refactor)
- Helper exercised by every migrated call site (13 sites across 2 integration files)
- No dedicated unit test — given four-line wrapper and placeholder string not asserted as load-bearing, would be over-testing

CODE QUALITY:
- Project conventions: Followed
- SOLID: Single responsibility
- Complexity: Minimal
- Modern idioms: `os.ReadFile`
- Readability: High; header explains purpose and `*testing.T` omission

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `internal/portaltest/doc.go` states `*testing.T` parameter "enforces this structurally"; `ReadPortalLogSafe` is exception. One-word softening in doc.go ("…on most exported helpers…") would prevent future contributor reading contract as inviolable
