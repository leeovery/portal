# Consolidation Findings: Lazy Resume On Attach (Phase 16)

## Findings

### F1: The store's mutations now answer a refused load two ways — `Set` leaves no record and no `ErrStoreRead`
- **Class**: behaviour
- **Failure**: Before this phase `Set` and `removeEntry` handled a refused load identically, returning `failed to load hooks: %w` with no breadcrumb. The phase moved `removeEntry` (Remove and Discard) to wrap `ErrStoreRead` and write its op's WARN, and left `Set` alone. `portal hook rm` and `portal hook set` run against the same unreadable or malformed `hooks.json` now differ. rm leaves a `hooks: rm` WARN carrying the whole chain. set leaves nothing in `portal.log` beyond `process: exit code=1`, because `main` prints a command error to stderr and never logs it. `hook set` is machine-invoked; CLAUDE.md keeps `hooks` as a silent alias for machine-written `portal hooks set …`, and nobody reads the stderr on that route. So a registration refused this way vanishes with no trace, and an operator grepping `hooks: set` to find out why a hook never landed finds nothing. The same failure on rm and discard is now recorded. This also breaks the audit-trail rule that every `hooks.json` mutation leaves its breadcrumb from the store method. `Set` already follows that rule for its acquire and save refusals; the load refusal is the gap. A caller also can no longer classify a refused read the same way across the three mutations: `errors.Is(err, ErrStoreRead)` holds for rm and discard but not for set.
- **Evidence**:
  - `internal/hooks/store.go:150-153`: Set's load failure is bare, with no WARN and no sentinel.
  - `internal/hooks/store.go:225-230`: removeEntry's load failure wraps `ErrStoreRead` and writes its WARN.
  - `internal/hooks/store.go:143-146` and `:219-221`: the two acquire-failure WARNs, the same line in each method.
  - `internal/hooks/store.go:326-331`: `ErrStoreRead`'s doc lists the sites that carry it. The phase already had to amend that list once.
  - `cmd/hooks_malformed_store_test.go:11-15`: the two CLI mutations are meant to refuse alike.
  - `main.go:53-70`: a command error is printed, never logged.
- **Proposed shape**: Declare the refused-load handling once in the store and have `Set` and `removeEntry` both call it. For example, `s.loadForMutation(op, key, via)` wraps `ErrStoreRead` and writes `logger.Warn(op, "op", op, "hook_key", key, "via", via, "error", err)`. `Set` passes its method op, `set`, just as its acquire-failure WARN does, because `classifySet`'s verdict needs the file the load failed to read. The identical acquire-failure WARN in both methods can use the same helper. `ErrStoreRead`'s doc then stops listing sites: it becomes a read of `hooks.json` that a mutation or a clean could not complete, which a failed write never carries. `internal/hooks/store_test.go:231-240` ("refuses to write over a malformed hooks.json") gains the `ErrStoreRead` assertion and the one-WARN assertion. `cmd/hooks_malformed_store_test.go` is unaffected, because `errors.Is(err, ErrMalformed)` still holds.

### F2: The waiting pane's "disable terminal signals failed" record names it by the positional key alone
- **Class**: behaviour
- **Failure**: The phase gave every record on the wait and recovery paths the pane's durable token (`resumePaneRef.logAttrs`: `hook_key` beside `pane_key`), because the positional key restore baked names another pane or none after a rename, a renumber or a moved pane. The same key can already differ from the pane's live address at restore, since a renumbered restore places panes away from their saved address. One waiting-pane record was left out: the hydrate helper's WARN when it cannot turn the parked pane's kill keys into bytes. That record is the counterpart of `enable terminal signals failed`, which now carries both names. It explains why a waiting pane's Ctrl-C ended the wait unanswered and dropped the pane to a shell with its hook never run. An operator who follows the pane by its token, the name `portal hook list` resolves to a live location and the name every other record of that pane's wait now carries, finds the enable record and the tail's records but not the record that explains the drop.
- **Evidence**:
  - `cmd/state_hydrate.go:269-271`: `cfg.Logger.Warn("disable terminal signals failed", "pane_key", payload.PaneKey, "error", err)`. The function only runs for a parked pane, and `payload` is in hand.
  - `cmd/state_resume_chain.go:198-202`: `enableTTYSignalsOrLog` now logs through `pane.logAttrs`.
  - `cmd/state_resume_chain.go:62-77`: `paneRef()` and `logAttrs`.
  - `cmd/state_resume_signals_test.go:115`: the only assertion on the disable record.
- **Proposed shape**: `cfg.Logger.Warn("disable terminal signals failed", payload.paneRef().logAttrs("error", err)...)`. The signals test adds a `hook_key` assertion beside its `pane_key` one, the same assertion `TestResumeWait_SignalGenerationOnHandingThePaneOn` makes for the enable side.

### F3: The report-row fit guard checks literal copies of the rows, not the rows production emits
- **Class**: duplication
- **Failure**: `TestResumeReportRows_FitTheCard` is the only guard that renders the refusal rows through the card and requires each to appear whole. Its list is hand-copied literals, not the `resumeReport*` constants that produce the rows. Suppose a constant is reworded or lengthened, for example `resumeReportLockHeld`. The exact-match tests pinning that row fail and get updated in the same edit. Nothing forces the fit list to move with them, so it goes on rendering the old string and passing while the new row is cut on the 52-cell card. The guard then checks a string nothing emits. It also covers no `resumeReportDrop` row at all. That row is 27 cells before its cause, and with the ENOTTY cause the draw tests use it reads `can't clear pending input: inappropriate ioctl for device`, 57 cells, which the card cuts inside the cause. The design accepts that cut, because the act comes first. But the guard is silent about it rather than covering it.
- **Evidence**:
  - `cmd/state_resume_report_test.go:312-342`: the guard, with its literal list at `:313-324`.
  - `cmd/state_resume_report.go:17-28`: the constants that produce the rows.
  - `cmd/state_resume_drop_input_test.go:269`: the drop row with its ENOTTY cause.
- **Proposed shape**:
  - Build the guard's rows from the production constants.
  - Fixed rows: `resumeReportMalformed`, `resumeReportLockHeld`, `resumeReportTmuxAbsent`, `resumeReportFallback`.
  - Prefixed rows: `resumeReportRead`, `resumeReportLock`, `resumeReportWrite`, `resumeReportLocate`, `resumeReportUnpause` and `resumeReportDrop`, each with a representative cause. An edit to a constant then changes what the guard measures.
  - The drop row needs either a cause that fits or an explicit assertion that the cut falls inside the cause, not the act.

## Comment Corrections

- cmd/state_resume_chain.go:14-16: a cardinality claim that this phase has already had to falsify once ("twice" became "three times").
  OLD: // The flags the resume chain's subcommands are addressed by. A pane is named
// three times: --pane is the pane id the marker writes need, --pane-key the
// positional key restore baked, and --hook-key the pane's durable token.
  NEW: // The flags the resume chain's subcommands are addressed by. --pane is the pane
// id the marker writes need, --pane-key the positional key restore baked, and
// --hook-key the pane's durable token.

- cmd/state_hydrate.go:282-297: narrates the argument ("— but not … So the backstop opens with …") and restates the shell steps the code composes. Every reason is kept, and the narration and the step list are dropped.
  OLD: // parkedChainBackstop leaves the pane at a shell when the tail could not start
// at all. It keys on the shell's could-not-run statuses, not on any failure: a
// tail that recovered the pane exec'd the user's shell, whose exit status
// arrives here, and a second shell after it would make the pane take two exits
// to close. The executable check tells a tail that never started from a shell
// that exited 126 or 127 itself — but not an answered pane from an abandoned
// one, since the binary can leave the baked path while an answered pane's shell
// is still running. So the backstop opens with the tail's own gate: a pending
// marker that reads back clear means the pane was answered and has had its
// shell, and the chain ends with the could-not-run status. A read that fails
// counts as still pending, as the tail counts it. A plain format read serves:
// the one pane it misreads as clear is a gone one, which wants no shell either.
// Past the gate it takes the tail's own steps in the tail's order — leave the
// panel's screen, clear the pending marker, lift the alternate-screen pin once
// tmux reports the leave, restore the terminal — since no Portal binary is left
// to take them.
  NEW: // parkedChainBackstop leaves the pane at a shell when the tail could not start
// at all. It keys on the shell's could-not-run statuses, not on any failure: a
// tail that recovered the pane exec'd the user's shell, whose exit status
// arrives here, and a second shell after it would make the pane take two exits
// to close. The executable check tells a tail that never started from a shell
// that exited 126 or 127 itself. The binary can leave the baked path while an
// answered pane's shell runs, so the tail's own gate comes first: a marker that
// reads back clear ends the chain, and a read that fails counts as still
// pending. A plain format read serves: the one pane it misreads as clear is a
// gone one, which wants no shell either. Past the gate it takes the tail's own
// steps in the tail's order, since no Portal binary is left to take them.
