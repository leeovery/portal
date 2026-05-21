AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 5

FINDINGS:

- FINDING: save.requested touch sequence is duplicated four-ways
  SEVERITY: medium
  FILES: cmd/state_commit_now.go:122-132, cmd/state_notify.go:50-60, cmd/state_commit_now_test.go:119-134, cmd/state_commit_now_daemon_merge_integration_test.go:151
  DESCRIPTION: The exact `O_WRONLY|O_CREATE|O_TRUNC + close + os.Chtimes(now,now)` pattern on `state.SaveRequested(dir)` now lives in (1) `defaultTouchSaveRequested` (production), (2) `stateNotifyCmd`'s inline body (production — the original source the reviewer noted), (3) the test seam `TouchSaveRequested` mock body in `state_commit_now_test.go`, and (4) the daemon-merge integration test's `os.WriteFile(state.SaveRequested(...), nil, 0o600)` — a near-variant. The reviewer note flagged (1)↔(2) only; (3) is a byte-for-byte third copy that exists solely to keep the test's mock aligned with production. Three production-shape copies passes the Rule of Three.
  RECOMMENDATION: Promote `defaultTouchSaveRequested` (or a sibling `state.TouchSaveRequested(dir string) error`) to a package-level helper. Have `stateNotifyCmd.RunE` call it (one log-component substitution stays inline). Have the test mock either delegate to that helper or assert against its side effects rather than re-implementing the body.

- FINDING: dumpStateDir / dumpStateDirRaw are near-identical
  SEVERITY: medium
  FILES: cmd/state_commit_now_reentrancy_integration_test.go:354-397, cmd/state_commit_now_symptom_integration_test.go:657-695
  DESCRIPTION: `dumpStateDir` (reentrancy file) and `dumpStateDirRaw` (symptom file) are line-by-line equivalents — same ReadDir loop, same one-level recursion into subdirs, same `(size=%d, mode=%s)` formatting, same 2048-byte sessions.json truncation, same `--- sessions.json contents ---` banner. The symptom file's header explicitly acknowledges the duplication ("Kept here rather than refactoring the sibling test because the two files are idiomatically self-contained"). They sit in the same package (`cmd_test`) and could share the symbol with zero callsite change. The "idiomatically self-contained" rationale is contradicted by the fact that the symptom file already calls `sessionNames` defined in the reentrancy file (line 341) — cross-file sharing within `cmd_test` is already the pattern.
  RECOMMENDATION: Delete `dumpStateDirRaw` and have `symptomFixture.diagnostic` call `dumpStateDir` directly. The `(stateDir string) string` signature is identical, so this is a name-only collapse.

- FINDING: pollSessionsJSON and pollSessionsJSONForKill share their loop body
  SEVERITY: medium
  FILES: cmd/state_commit_now_reentrancy_integration_test.go:294-334, cmd/state_commit_now_symptom_integration_test.go:518-550
  DESCRIPTION: Two implementations of the same two-consecutive-consistent-reads poll loop over `state.ReadIndex(stateDir)`: identical ticker setup, identical ENOENT/skip/default switch with `consecutive = 0` reset semantics, identical `consecutive >= N` early-return, identical ctx cancellation handling. The only behavioural difference is the shape predicate: `expectKept present AND expectKilled absent` vs `every name in mustHave present AND every name in mustOmit absent`. The first is a strict subset of the second.
  RECOMMENDATION: Delete `pollSessionsJSONForKill` and update its single caller to call `pollSessionsJSON(ctx, stateDir, []string{"A"}, []string{"B"})`. Collapse the duplicate constants while you're there.

- FINDING: sessionNames helper exists in three incompatible shapes
  SEVERITY: low
  FILES: cmd/state_commit_now_reentrancy_integration_test.go:341-347, cmd/state_commit_now_symptom_integration_test.go:570-576, cmd/state_commit_now_test.go:161-167
  DESCRIPTION: `cmd_test`'s `sessionNames(idx) map[string]bool` and `indexSessionNameSet(idx) map[string]struct{}` differ only by value type. The `cmd` package's `sessionNames(idx) []string` is a third variant.
  RECOMMENDATION: Pick one shape (`map[string]struct{}` is canonical) and have both `cmd_test` integration files use it.

- FINDING: Subprocess env-building is duplicated across runPortal* helpers
  SEVERITY: low
  FILES: cmd/state_commit_now_symptom_integration_test.go:460-474, cmd/state_commit_now_symptom_integration_test.go:492-506
  DESCRIPTION: `runPortalCommitNow` and `runPortalList` are structurally identical: same `exec.Command(binary, ...)` + same three-line env append + same CombinedOutput + same Fatalf shape. Only the command args differ. Below Rule-of-Three threshold today but flagged for the obvious near-future third caller.
  RECOMMENDATION: Extract a `runPortalSubprocess(t, f, args ...string)` helper.

SUMMARY: Five duplication clusters, all introduced during this work unit. Two medium-severity cmd_test items are name-only collapses with same-package precedent. The save.requested touch sequence has reached three production-shape copies and warrants promotion to a shared helper.
