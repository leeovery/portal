AGENT: standards
CYCLE: 1
STATUS: clean
FINDINGS: none
SUMMARY: Implementation conforms to specification and project conventions.

Verification details:
- The diff matches the spec's two prescribed edits verbatim: sessionHelpKeys() at internal/tui/model.go:529 changes WithHelp("p", "projects") -> WithHelp("p/x", "projects"), and projectHelpKeys() at line 576 changes WithHelp("s", "sessions") -> WithHelp("s/x", "sessions").
- Display-only as required: WithKeys("p") and WithKeys("s") left unchanged.
- The x runtime toggles remain wired exactly as before — sessions-page x sets PageProjects (model.go:1602), projects-page x sets PageSessions (model.go:1256).
- Excluded surfaces correctly untouched: commandPendingHelpKeys() (model.go:587), brightenHelpStyles palette, three-column footer chunking. No new entries added.
- Verification gates pass: go build succeeds; go test ./internal/tui/... passes.
- No project-convention or golang-pro skill conflicts.
