AGENT: duplication
FINDINGS:
- FINDING: Repeated tmux.NewClient(&tmux.RealCommander{}) construction across cmd files
  SEVERITY: medium
  FILES: cmd/root.go:42, cmd/bootstrap_wait.go:22, cmd/kill.go:52, cmd/attach.go:52, cmd/list.go:103, cmd/open.go:69, cmd/open.go:227, cmd/open.go:334
  DESCRIPTION: The expression `tmux.NewClient(&tmux.RealCommander{})` appears 8 times across 6 files. Each command's buildXxxDeps function independently constructs a new client. The open.go file alone has 3 separate constructions (buildSessionConnector, openPath, openTUI).
  RECOMMENDATION: Extract a package-level helper in the cmd package, e.g. `func newTmuxClient() *tmux.Client { return tmux.NewClient(&tmux.RealCommander{}) }`. This consolidates the construction site so a future change to client creation (e.g. adding options) only needs one edit.

- FINDING: Identical validate-then-act pattern in attach.go and kill.go RunE bodies
  SEVERITY: low
  FILES: cmd/attach.go:29-41, cmd/kill.go:29-41
  DESCRIPTION: Both commands share the same 5-line pattern: call bootstrapWait, extract name from args[0], build deps, validate session with HasSession, and return the identical "No session found: %s" error on failure. The structural shape and error message are identical, only the final action differs (Connect vs KillSession).
  RECOMMENDATION: Low severity because the pattern is only 5 lines and appears exactly twice. If a third command adopts this pattern, extract a `validateSession(cmd, name, validator)` helper that returns the validated name or error. For now, keep as-is per the Rule of Three.

- FINDING: Near-duplicate mock types across test packages (cmd vs internal/tui)
  SEVERITY: low
  FILES: cmd/list_test.go:11, internal/tui/model_test.go:210, cmd/kill_test.go:9, internal/tui/model_test.go:720, cmd/open_test.go:218, internal/tui/model_test.go:754, cmd/attach_test.go:9, cmd/open_test.go:744
  DESCRIPTION: mockSessionLister, mockSessionKiller, mockSessionCreator, and mockConnector/mockSessionConnector are independently defined in both the cmd test package and the internal/tui test package. The implementations are structurally identical (same fields, same method bodies). This is expected in Go since _test packages cannot share unexported types, but the duplication is notable in volume (4 mock types x 2 packages = 8 definitions).
  RECOMMENDATION: This is a Go test packaging constraint, not a design flaw. Shared test helpers via an internal/testutil package could consolidate these, but that adds coupling between test packages. No action recommended unless mock count grows further.

SUMMARY: One medium-severity finding: tmux client construction is repeated 8 times across cmd files and should be extracted to a single helper. Two low-severity findings (validate-then-act pattern in attach/kill, and cross-package test mock duplication) are within acceptable thresholds.
