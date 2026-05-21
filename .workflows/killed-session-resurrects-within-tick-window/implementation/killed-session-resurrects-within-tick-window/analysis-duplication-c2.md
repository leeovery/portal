AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 3

FINDINGS:

- FINDING: runPortalCommitNow and runPortalList remain unconsolidated subprocess shells
  SEVERITY: low
  FILES: cmd/state_commit_now_symptom_integration_test.go:453-467, cmd/state_commit_now_symptom_integration_test.go:485-499
  DESCRIPTION: Carried forward from cycle 1. The two helpers are structurally identical: same `exec.Command(binary, ...)` shape, same three-line env append, same `CombinedOutput`, same `t.Fatalf` shape. Only the positional args differ. Still below Rule-of-Three with two direct call sites.
  RECOMMENDATION: Extract `runPortalSubprocess(t, binary, f, args ...string)` helper and rewrite both as trampolines.

- FINDING: dumpStateDir (cmd_test) and dumpStateDirForNotifyTest (cmd) share per-entry format and ReadDir loop
  SEVERITY: low
  FILES: cmd/state_commit_now_reentrancy_integration_test.go:296-339, cmd/state_notify_six_event_eventual_consistency_test.go:155-170
  DESCRIPTION: Both helpers walk `os.ReadDir(stateDir)` and emit per-entry `%s (size=%d, mode=%s)`. Cmd_test version recurses one level and inlines sessions.json bytes; cmd-package version is top-level-only subset. Package boundary (`cmd` vs `cmd_test`) is real; cross-package consolidation would require promoting helper into a non-test internal package.
  RECOMMENDATION: Leave as-is for now — package-boundary cost outweighs win at two instances. If a third dumper appears, promote a `statetest.DumpDir` into a shared internal-test package.

- FINDING: defaultTouchSaveRequested is a one-line trampoline asymmetric with peer field defaults
  SEVERITY: low
  FILES: cmd/state_commit_now.go:126-133, cmd/state_commit_now.go:99
  DESCRIPTION: `defaultTouchSaveRequested` is `func(dir string) error { return state.TouchSaveRequested(dir) }`. The same pattern is NOT applied to the other five `CommitNowDeps` fields — `ReadIndex`/`CaptureStructure`/`Commit` reference `state.X` directly. `state.TouchSaveRequested` already IS a stable package-level function with the matching signature. Residue from cycle 1's promotion that kept a wrapper.
  RECOMMENDATION: Delete `defaultTouchSaveRequested` and replace `TouchSaveRequested: defaultTouchSaveRequested` with `TouchSaveRequested: state.TouchSaveRequested`. Matches the four sibling fields.

SUMMARY: Three low-severity items. Cycle-1 collapses landed cleanly. Remaining items: pre-existing carryover (runPortal*) and one new YAGNI wrapper (defaultTouchSaveRequested) introduced by cycle-1 refactor.
