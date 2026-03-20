AGENT: standards
FINDINGS:
- FINDING: Double session wait on open command fallback path
  SEVERITY: medium
  FILES: cmd/open.go:89, cmd/open.go:107
  DESCRIPTION: When a destination is provided but resolution falls back to the TUI, both the CLI session wait (bootstrapWait at line 89) and the TUI loading interstitial (serverWasStarted passed at line 107) would execute. The spec defines two-phase ownership where session wait belongs to either the CLI path or the TUI path, not both. If the server was just bootstrapped, the user experiences two sequential waits: first the CLI wait (up to 6s), then the TUI loading page (up to 6s more). Practically, the second wait would be short since sessions likely appeared during the first, but it violates the spec's clean separation.
  RECOMMENDATION: When falling back to TUI after bootstrapWait already ran, pass false for serverStarted to openTUI on line 107, e.g. `return openTUI(r.Query, command, false)`. The CLI wait already handled the session wait; the TUI should not repeat it.
SUMMARY: One medium-severity finding: the open command's fallback-to-TUI path can trigger both CLI and TUI session waits when the server was just bootstrapped, violating the spec's two-phase ownership model. All other spec decisions (tmux start-server command, PersistentPreRunE placement, timing constants, detection via tmux info, loading interstitial UX, stderr messaging, one-shot with no retry, skipTmuxCheck commands, plugin-agnostic design) are correctly implemented.
