AGENT: architecture
FINDINGS:
- FINDING: Edit project feature implemented but not wired in production
  SEVERITY: medium
  FILES: cmd/open.go:286-292, internal/tui/model.go:733-766, internal/tui/model.go:331-342
  DESCRIPTION: The `WithProjectEditor` and `WithAliasEditor` options are defined, implemented, and tested in `internal/tui/model.go` (lines 331-342), and the full edit modal logic works end-to-end in tests. However, `cmd/open.go` (the only production call site for `tui.New`) never injects either dependency (lines 286-292). The `handleEditProjectKey` method (line 738) guards on `m.projectEditor == nil || m.aliasEditor == nil` and silently returns nil, so pressing `e` on the Projects page is a silent no-op in production. The spec explicitly requires this feature: "e triggers a modal overlay with the project's name field, alias list, and full edit controls". The help bar still advertises `[e] edit` to the user.
  RECOMMENDATION: Wire `WithProjectEditor` and `WithAliasEditor` in `cmd/open.go`'s `openTUI` function, constructing the appropriate implementations from the existing `project.Store` and alias infrastructure. Alternatively, if the wiring is intentionally deferred, remove `e` from the `projectHelpKeys` so the help bar does not advertise an inoperative action.

SUMMARY: The C2 evaluateDefaultPage finding has been properly addressed. One medium finding remains: the edit project modal is fully implemented and tested but never wired in production, making the `e` key a silent no-op despite being advertised in the help bar.
