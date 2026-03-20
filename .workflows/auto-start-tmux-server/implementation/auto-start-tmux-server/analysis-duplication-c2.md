AGENT: duplication
FINDINGS:
- FINDING: Duplicate mock SessionConnector types across test files
  SEVERITY: medium
  FILES: cmd/attach_test.go:11, cmd/open_test.go:757
  DESCRIPTION: mockSessionConnector (attach_test.go) and mockConnector (open_test.go) are structurally identical types — same fields (connectedTo string, err error), same method signature (Connect(name string) error), same implementation. They implement the same SessionConnector interface but were written independently by different task executors.
  RECOMMENDATION: Keep one (mockSessionConnector in attach_test.go since it's the more descriptive name) and remove the other. Both test files are in the same package so a single definition suffices.

- FINDING: Duplicate mock SessionLister types across test files
  SEVERITY: medium
  FILES: cmd/list_test.go:13, cmd/open_test.go:671
  DESCRIPTION: mockSessionLister (list_test.go) and stubSessionLister (open_test.go) are structurally identical — same fields (sessions []tmux.Session, err error), same ListSessions method returning those fields directly. They implement the same interface but have different names.
  RECOMMENDATION: Keep one definition (mockSessionLister in list_test.go) and remove stubSessionLister from open_test.go. Update references in open_test.go to use mockSessionLister.

- FINDING: Identical validate-then-act pattern in attach.go and kill.go RunE bodies
  SEVERITY: low
  FILES: cmd/attach.go:28-41, cmd/kill.go:28-41
  DESCRIPTION: Both commands follow the exact same 4-step pattern: (1) bootstrapWait(cmd), (2) name := args[0], (3) build deps returning (action, validator), (4) if !validator.HasSession(name) return "No session found: %s" with identical error message and nolint comment. The only difference is the final action call (connector.Connect vs killer.KillSession). This is ~6 lines of duplicated logic.
  RECOMMENDATION: Severity is low because the duplication is only ~6 lines across two files and the commands have different dep types. No extraction recommended unless a third command follows this pattern. Flagging for awareness only.

SUMMARY: Two pairs of identical mock types in test files (mockSessionConnector/mockConnector and mockSessionLister/stubSessionLister) should be consolidated to single definitions. The attach/kill RunE pattern duplication is minor and does not warrant extraction at this time.
