AGENT: duplication
FINDINGS: none
SUMMARY: The quick-fix lifted a single `t.Setenv("PORTAL_STATE_DIR", t.TempDir())` line to unconditional in TestStateUserFacingSubcommandsExitZero (cmd/state_test.go:234); it introduces no new cross-file or near-duplicate logic.

Notes:
- The lifted line also appears at line 178 (daemon case in TestStateInternalSubcommandsAcceptValidArgv) — only two contextually-distinct instances, below the Rule of Three, not an extraction candidate.
- The repeated Cobra subtest scaffolding (resetRootCmd / resetStateCmdFlags / SetOut / SetErr / SetArgs / Execute) is pre-existing boilerplate untouched by this single-line quick-fix and out of plan scope.
