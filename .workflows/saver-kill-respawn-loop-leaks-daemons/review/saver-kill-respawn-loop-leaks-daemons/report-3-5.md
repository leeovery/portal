TASK: Extract sentinelIndex + assertNoCommit helpers for captureAndCommit unchanged-pointer tests (3-5)

ACCEPTANCE CRITERIA:
- sentinelIndex(name) *state.Index exists in cmd/state_daemon_run_test.go
- assertNoCommit(t, deps, sentinel, stateDir) exists; asserts PrevIndex pointer identity + sessions.json non-existence
- Peer assertCommitReplacedPrev for the replaced-pointer case
- Four captureAndCommit tests use the helpers (no inline fixture or assertion blocks)
- All four tests preserve existing assertion semantics
- go test ./cmd/... passes

STATUS: Complete

SPEC CONTEXT: From analysis-tasks-c1 (cycle-1 duplication). Four captureAndCommit tests (Tasks 2-1/2-2/2-3/2-4 sites) constructed identical sentinelPrev fixtures and post-call assertions. Pure mechanical extraction.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - sentinelIndex: cmd/state_daemon_run_test.go:200-213
  - assertNoCommit: cmd/state_daemon_run_test.go:215-231
  - assertCommitReplacedPrev: cmd/state_daemon_run_test.go:233-245
- Call sites:
  - line 903 + 912 — replaced-pointer happy path (Task 2-1 regression)
  - line 981 + 1008 — pre-cancelled ctx (Task 2-2)
  - line 1055 + 1108 — cancel during CaptureStructure (Task 2-3)
  - line 1148 + 1182 — cancel mid-loop after k panes (Task 2-4)
  - line 1213 + 1240 — uncancelled multi-pane regression (Task 2-4 peer)
- Notes: Plan called out four sites; a fifth (TestCaptureAndCommit_UncancelledMultiPaneFixtureProcessesAllPanesAndCommits) is a peer regression added under Task 2-4 and correctly reuses assertCommitReplacedPrev. Helpers take stateDir explicitly. No lingering inline duplications.

TESTS:
- Status: Adequate
- Coverage: The five captureAndCommit tests are themselves the regression coverage per the task plan; assertion semantics preserved verbatim.
- Notes: assertCommitReplacedPrev adds a t.Fatal nil-guard for PrevIndex — strict superset of the original inline assertion, defensive, no regression risk.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(). t.Helper() correctly declared.
- SOLID principles: Good — single-purpose, intent-revealing names.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Doc comments enumerate the load-bearing invariants.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] assertCommitReplacedPrev accepts stateDir for signature parity with assertNoCommit and discards it (`_ = stateDir`). Doc comment notes this is reserved for future on-disk assertions. If no on-disk assertion lands, drop the parameter — symmetry alone is weak justification.
