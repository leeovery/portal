TASK: Collapse dumpStateDir / dumpStateDirRaw duplication (killed-session-resurrects-within-tick-window-2-3)

ACCEPTANCE CRITERIA:
- `dumpStateDirRaw` no longer exists.
- `dumpStateDir` is the single dumper, called from both integration tests.
- Single-caller rename; no other references.

STATUS: Complete

SPEC CONTEXT: Cycle-1 duplication analysis flagged `dumpStateDir`/`dumpStateDirRaw` as near-identical with identical `(stateDir string) string` signatures. `dumpStateDirRaw` had a single caller in `symptomFixture.diagnostic`. Task 2-3 is a name-only collapse — same-package, same-signature consolidation.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Single dumper: `cmd/state_commit_now_reentrancy_integration_test.go:296` (`dumpStateDir`).
  - Caller #1 (originally `dumpStateDirRaw`): `cmd/state_commit_now_symptom_integration_test.go:423` (`symptomFixture.diagnostic`).
  - Caller #2 (pre-existing): `cmd/state_commit_now_symptom_integration_test.go:573` (`assertSessionsJSONHas`).
  - Caller #3 (pre-existing): `cmd/state_commit_now_reentrancy_integration_test.go:232`.
- Notes: Grep across repo confirms zero remaining references to `dumpStateDirRaw` in production/test code (only historical workflow artefacts under `.workflows/` and `.tick/`). Both consuming files share `package cmd_test`.

TESTS:
- Status: Adequate
- Coverage: No new test surface needed — test-helper rename whose correctness is exercised by existing integration tests calling `diagnostic()` on failure.
- Notes: Adding dedicated tests for a diagnostic helper would be over-testing.

CODE QUALITY:
- Project conventions: Followed (cmd_test package, file-level Go idioms intact).
- SOLID: Good — removed needless duplication.
- Complexity: Low — function body unchanged.
- Modern idioms: Yes.
- Readability: Good — surviving helper has clear doc-comment.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Cross-package sibling `dumpStateDirForNotifyTest` at `cmd/state_notify_six_event_eventual_consistency_test.go:155` remains as near-variant. Cycle-3 duplication analysis explicitly deferred consolidation ("leave as-is at exactly two instances; promote only if a third dumper appears"). Documented as deferred, not action.
