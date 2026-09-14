AGENT: architecture
FINDINGS:
- FINDING: Sigil completion offers session names the sigil's own search cannot reach
  SEVERITY: medium
  FAILURE: Inside tmux, `x /<TAB>` (or `/por<TAB>`) offers the session the user is
    currently attached to, because the completer enumerates every live session name.
    Accepting that candidate and pressing Enter cannot attach anything — the count
    excludes the attached session — so the user lands on a picker filtered to zero
    rows, having just tab-completed a real session name off Portal's own list. The
    user notices it as a completion that reliably produces "no matches"; since the
    sigil never fails, there is no message accounting for it.
  FILES: cmd/completion.go:62-75, cmd/open_search.go:98-111, internal/tui/picker_sessions.go:13
  DESCRIPTION: "Which live sessions a search can reach" now has two independent
    implementations on the same feature. The count path composes it — `searchCandidates`
    calls `tui.PickerSessions`, the picker's own rule (enumeration less the attached
    session), which is what makes the spec's "the searched set is the set the picker
    lists" structural rather than coincidental. The completion path authors it again
    from scratch: `completeSearchTerm` walks `completionSessionNames()` (raw
    `ListSessionNames`, filtered only for `_`-prefixed internals and embedded slashes)
    and never consults the attached session. The two therefore disagree on exactly one
    element, and it is the element the user is most likely to be sitting in. The
    completer is the one surface that hands the user a term to run, so its domain is
    the one that most needs to be derived from the search's rather than written beside
    it. The divergence is silent: no test covers it (cmd/completion_test.go drives
    `completeSearchTerm` against a name-list seam with no notion of a current session),
    and the count path's own exclusion is well covered, so neither side reports the gap.
  RECOMMENDATION: Derive the offered set from the counted set. Have `completeSearchTerm`
    reach the same rule `searchCandidates` uses — enumerate sessions, drop the attached
    one via `tui.PickerSessions` when inside tmux — rather than the raw name list, so the
    words offered and the sessions counted cannot answer differently. Scope the change to
    the search completer only: `completeSessionNames`, which serves `-s` and the bare
    positional, is pre-existing surface where offering the current session is harmless.
COMMENT_CORRECTIONS:
- internal/tui/session_dir_column.go:20 — claims purity and then names the environment
  read that contradicts it, which invites treating the helper as safe to render or
  assert against without pinning $HOME (the capture suite has to pin it)
  OLD: // empty directory or a non-positive width. Pure: the only environment it reads
       // is the one AbbreviateHome consults.
  NEW: // empty directory or a non-positive width. Not environment-independent: the
       // abbreviation resolves the home directory, so one value renders differently
       // under a different $HOME.
SUMMARY: The search feature composes well across its layers — one containment rule in
  `resolver`, one picker-set rule shared by the count and the list, one landing
  chokepoint, and a warning-routing tail that reaches the terminal on every exit path I
  traced. The one seam that was not derived from an existing abstraction is tab
  completion, which enumerates its own candidate set and so offers the one session the
  search deliberately excludes.
