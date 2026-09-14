AGENT: duplication
FINDINGS:
- FINDING: The current-session read that decides the excluded session is written twice — once for the search count, once for the picker's list
  SEVERITY: medium
  FAILURE: The sigil's match count K and the list the picker renders are derived from two independently-authored reads of "which session am I attached to". If either read changes — its source, its tolerance of a failed or empty answer, or the `InsideTmux()` gate in front of it — `/term` starts counting over a set the picker does not list. The user sees it as `x /term` attaching the session they are already sitting in (a `switch-client` to self, which looks like a keypress that did nothing), or as a picker opening on one row after the count said two. Nothing fails to compile and no test pairs the two sites.
  FILES: cmd/open.go:725-731, cmd/open_search.go:98-111
  DESCRIPTION: `internal/tui.PickerSessions` already single-sources the *exclusion* rule (both sites call it), but the step that produces its `currentSession` argument is duplicated. `openTUI` does `if tmux.InsideTmux() { name, err := client.CurrentSessionName(); if err == nil && name != "" { cfg.insideTmux = true; cfg.currentSession = name } }`, which reaches `PickerSessions` through the model's `filteredSessions()`. `searchCandidates` independently does `if !tmux.InsideTmux() { return sessions }; current, err := src.CurrentSessionName(); if err != nil || current == "" { return sessions }; return tui.PickerSessions(sessions, current)`. The two are byte-for-byte different spellings of one rule, and the specification fixes their agreement as a property ("the searched set is the set the picker lists"). The unit tests cover each side alone — `TestOpenCommand_SearchForm_ExcludesTheCurrentSessionFromTheCount` and its siblings drive `searchCandidates` through a fake source, `TestPickerSessions` drives the helper directly — so no test observes the pair.
  RECOMMENDATION: Extract the acquisition into one `cmd`-level helper beside `searchCandidates` — something like `currentPickerSession(src SearchSessionSource) string`, returning the empty string outside tmux and on a failed or empty read. Have `searchCandidates` pass its result straight to `tui.PickerSessions`, and have `openTUI` set `cfg.insideTmux`/`cfg.currentSession` from the same call (`if name := currentPickerSession(client); name != "" { … }`), so the title's copy of the name and the count's exclusion come from one read of one rule.

COMMENT_CORRECTIONS:
- cmd/open.go:734 — claims the model emits staged warnings only after the loading page dismisses, which this change set contradicted: a search attach and a command-pending picker have no loading page and get them at teardown, as `stageBootstrapWarningsOnModel`'s own (corrected) doc now says
  OLD: // Staged rather than written: the model emits them only after the loading page
       // is dismissed, because a direct write during loading corrupts the rendered UI.
  NEW: // Staged rather than written: a direct write while the TUI holds the screen
       // corrupts the rendered frame.
- cmd/root.go:141 — the widened `isTUIPath` in this same file now admits members with no loading page, so "until the loading page dismisses" names a gate those invocations never reach
  OLD: // the TUI path leaves them in the sink until the loading page dismisses.
  NEW: // the TUI path leaves them in the sink for the model to carry, so nothing is
       // written into a frame the picker is about to claim.

SUMMARY: The search feature's own rules are well single-sourced — `resolver.MatchesSearchTerm`/`SearchFields` back both the count and the picker's filter, `tui.PickerSessions` backs both session sets, `listSessionsArgs`/`parseSessionList` back both list readers, and `dismissLoadingGate` collapsed the two loading-gate tails. One duplicate survives: the current-session read feeding `PickerSessions` is authored separately in `openTUI` and `searchCandidates`, so the count and the list can silently drift apart. Two comments in the change set's own files make claims about the loading page that the change set falsified.
