AGENT: architecture
FINDINGS:
- FINDING: evaluateDefaultPage does not respect commandPending when choosing the active page
  SEVERITY: medium
  FILES: internal/tui/model.go:460-476
  DESCRIPTION: When commandPending is true, evaluateDefaultPage still unconditionally evaluates the session-list-items check at lines 472-476 to decide the active page. Correctness depends on Init() never fetching sessions in command-pending mode (so sessionList.Items() is empty and the check falls through to PageProjects). If a SessionsMsg ever arrives before evaluateDefaultPage runs -- e.g., from a future code path or a race in message delivery -- the method would set activePage to PageSessions, breaking the command-pending contract. The commandPending guard at lines 464-467 only controls the readiness gate, not the page decision itself.
  RECOMMENDATION: Add an early return or explicit branch inside evaluateDefaultPage for commandPending mode. When commandPending is true, always set activePage to PageProjects and skip the session-items check. This makes the invariant self-contained rather than relying on Init() behavior: `if m.commandPending { m.activePage = PageProjects } else if len(m.sessionList.Items()) > 0 { m.activePage = PageSessions } else { m.activePage = PageProjects }`

- FINDING: Session creation errors silently swallowed with no user feedback
  SEVERITY: low
  FILES: internal/tui/model.go:576-578, internal/tui/model.go:91-94
  DESCRIPTION: When sessionCreateErrMsg is received, the handler returns (m, nil) with a comment "return to current page" but the error in msg.Err is never surfaced to the user. There is no status message, no modal, and the status bar is disabled on both lists. The user presses Enter (or n) to create a session, nothing happens, and they have no indication why. The edit project modal demonstrates the pattern for surfacing errors (editError field), but session creation has no equivalent. Similarly, ProjectsLoadedMsg with a non-nil Err silently keeps the stale list.
  RECOMMENDATION: Either enable the list status bar and use it to display transient error messages, or store the error on the model and render it as a brief status line (similar to how editError works in the edit modal). Alternatively, surface it via lipgloss styling in the list title area.

SUMMARY: The cycle 1 findings (ANSI overlay, modal dispatch duplication) have been properly addressed. Two remaining concerns: evaluateDefaultPage does not explicitly guard command-pending mode in its page-selection logic (relying on Init() behavior for correctness), and session creation errors are silently dropped without user feedback.
