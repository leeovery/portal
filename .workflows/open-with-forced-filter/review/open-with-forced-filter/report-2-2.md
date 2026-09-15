TASK: open-with-forced-filter-2-2 (tick-ad2390) — Attach directly when exactly one live session matches the term

ACCEPTANCE CRITERIA:
1. With exactly one live session matching the term, `openSessionFunc` is called with that session's name and `openTUIFunc` is never called
2. The single match is connected through `openSessionFunc` — the same seam the exact-session and `-s` pin paths take — so the inside-tmux `switch-client` / outside-tmux exec choice stays `buildSessionConnector`'s and no connector type or connection call is added here
3. With zero matches over a live session list, the picker opens with the landing `{filter: term, search: true}`, `RunE` returns nil (a zero exit) and nothing is written to stderr
4. With two or more matches the picker opens with the same landing
5. A session matched on its recorded directory alone counts toward the total and is attached when it is the only one
6. `portal open /` calls neither `ListSessions` nor `CurrentSessionName` and lands the term-less picker exactly as task 1-6 leaves it
7. Inside tmux the session the user is attached to is not a candidate; outside tmux nothing is excluded
8. A glob metacharacter in the term is literal: `/po*` counts sessions containing `po*` and dispatches no burst
9. No record is emitted under the `resolve` component on any of the three outcomes
10. An enumeration error is returned to the caller rather than counted as zero matches
11. `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT:
§3.2 fixes the three outcomes by match count K (0 → pre-filtered picker, 1 → direct attach with no picker, ≥2 → picker with the cursor on the first matching row), states that a term-less sigil takes no count at all, and that the searched set is the set the picker lists — Portal's underscore-prefixed internal sessions being absent from that enumeration at source. §3.4 puts the count in `cmd` before the model is built on a warm server, and behind the whole concurrent bootstrap on a cold one. §3.6 forbids a `resolve` component line on the form. §3.7 pins the attach to the connector the invocation already selects (no third connection mode) and separates a failed session-list read from a zero match — the one non-search failure on the form. §4.3 makes the match case-folded containment over name and recorded directory tested as separate fields, with glob metacharacters literal.

IMPLEMENTATION:
- Status: Implemented (evolved past the task's literal wording by later phases, soundly)
- Location:
  - `cmd/open_search.go:144-155` — `SearchSessionSource` + `buildSearchSessionSource` (injected seam when set, `tmuxClient(cmd)` otherwise)
  - `cmd/open_search.go:157-174` — `currentSessionReader` / `currentPickerSession` (inside-tmux gate; failed or empty read drops nothing)
  - `cmd/open_search.go:177-184` — `searchCandidates`, routed through `tui.PickerSessions`
  - `cmd/open_search.go:186-196` — `searchMatches` over `resolver.MatchesSearchTerm`, enumeration order, no filter of its own
  - `cmd/open_search.go:200-212` — `searchDecision`, the closure that answers name / "" / error
  - `cmd/open_search.go:221-253` — `runSearchForm`, the three-way fork plus the deferred-bootstrap hand-off
  - `cmd/open.go:48-49` — the `SearchSessions` field on `OpenDeps`
  - `cmd/open.go:177-180` — the `RunE` search branch, sited ahead of the multi-target gate and of resolution
- Notes:
  - Two deliberate divergences from the task's literal text, both later plan tasks and both sound:
    (a) the seam method is `ListSessionsProbe()` rather than `ListSessions()` — task 2-4 (tick-5cfe10) introduced the discriminating variant at `internal/tmux/tmux.go:143-150` precisely so criterion 10 can hold; `ListSessions` swallows a failed read as an empty server (`internal/tmux/tmux.go:130-138`), which would have made criterion 10 unreachable.
    (b) the landing is `pickerLanding{search: &tui.SearchForm{Term: term}}` rather than `{filter: term, search: true}` — task 9-2 (tick-251123) moved the term onto the search form itself (`cmd/open_search.go:55-59`). Criterion 3/4 hold in substance: the picker still receives the term and the search-form marker, asserted through `shapeOfLanding` (`cmd/testhelpers_test.go:242-256`).
  - Criteria 1 and 2 hold on the cold/deferred route too, by a different mechanism task 4-2 added: when a bootstrap is in flight `runSearchForm` hands the closure to the picker (`cmd/open_search.go:229-232`), the model quits without painting a picker frame on a single match (`internal/tui/search_decision.go:49-63`), and `openTUI` connects through the same `buildSessionConnector(client)` (`cmd/open.go:686`, `cmd/open.go:621-629`). No connector type is added on either route.
  - The searched set is genuinely single-sourced with the picker's: `tui.PickerSessions` is the only exclusion rule, consumed by `searchCandidates` (`cmd/open_search.go:182`), the picker (`internal/tui/model.go:1144`) and the completer (`cmd/completion.go:74`).
  - `writeAckMarker` is correctly absent from this branch — `validateSearchFormCollisions` refuses `--ack` beside a search form (`cmd/open_search.go:94-95`).

TESTS:
- Status: Adequate
- Coverage (`cmd/open_search_test.go`):
  - c1 — `TestOpenCommand_SearchForm_AttachesTheSingleMatch:514` (name asserted, picker seam asserted unrun)
  - c2 — covered structurally: the fake replaces `openSessionFunc`, so a second connection route would show as an unset `attached`; `buildSessionConnector` is untouched by the change-set
  - c3 — `TestOpenCommand_SearchForm_OpensPickerWhenNothingMatches:545` asserts the landing, a nil error and an empty stderr buffer
  - c4 — `TestOpenCommand_SearchForm_OpensPickerWhenTwoOrMoreMatch:564`; plus `..._OpensPickerWhenNoSessionsAreLive:581`
  - c5 — `TestOpenCommand_SearchForm_AttachesASessionMatchedOnlyByItsRecordedDirectory:528` (`api-work` at `$HOME/Code/portal`, term `portal`)
  - c6 — `TestOpenCommand_SearchForm_TermLessFormTakesNoCount:594` asserts both read counters are zero
  - c7 — `..._ExcludesTheCurrentSessionFromTheCount:612`, `..._AttachesTheOtherMatchWhenTheCurrentSessionAlsoMatches:627`, `..._CountsNothingOutWhenTheCurrentSessionReadFails:642`, `..._ExcludesNothingOutsideTmux:657`, plus the direct `TestCurrentPickerSession:747` table
  - c8 — `TestOpenCommand_SearchForm_GlobMetacharactersAreLiteralText:219` (`/po*` against `po*rt-x`; `runOpenBurstFunc` asserted unrun)
  - c9 — `TestOpenCommand_SearchForm_EmitsNoResolveLine:247`, a `logtest.Install` table over all three counts
  - c10 — `TestOpenCommand_SearchForm_ReturnsAnEnumerationError:728` asserts the error surfaces, is not a `*UsageError`, and that neither seam ran
  - Spec edge cases beyond the criteria: `TestOpenCommand_SearchForm_CountsOverExactlyTheEnumeratedSessions:711` pins that no second internal-session filter was restated on this path (§3.2's "restating that filter here would be a second home"), and `TestSearchCandidates_LeavesTheEnumerationItWasHandedUnmodified:689` guards `PickerSessions`' documented never-delete-in-place contract.
- Notes: Not over-tested. `TestCurrentPickerSession` overlaps the behavioural exclusion tests at the surface, but `currentPickerSession` now has three production consumers (`cmd/open_search.go:182`, `cmd/completion.go:26`, `cmd/open.go:733`), so a direct test of the helper earns its place. The fakes are minimal (two methods, call counters) and assert on behaviour — the attached name and which seam ran — rather than on internals.

CODE QUALITY:
- Project conventions: Followed. Small (2-method) DI interface on `OpenDeps` with a `build*` resolver falling back to the production client; seams staged through `withFuncSeam` / `withOpenDeps`, never assigned directly, so `cmd/seam_guard_test.go` stays green. No tmux-touching dependency is left uninjected in the tests that Execute the real `open` body.
- SOLID principles: Good. `SearchSessionSource` is segregated to exactly what the count needs; `currentSessionReader` is narrowed further so `currentPickerSession` is reusable from the completer without dragging the enumeration along.
- Complexity: Low. `runSearchForm` is a flat four-branch dispatch; the count, the exclusion and the match rule are each one small function.
- Modern idioms: Yes. The rule itself lives in `internal/resolver` (`MatchesSearchTerm`/`SearchFields`) with one declaration of the searchable field list.
- Readability: Good. Every non-obvious choice carries its reason — why the term-less form skips the read, why a failed current-session read drops nothing, why the decision is a closure.
- Comment accuracy: Verified against the code. `SearchSessionSource`'s "discriminates a failed read from an empty server" holds against `ListSessionsProbe` (`internal/tmux/tmux.go:143-150`); `searchMatches`' "in enumeration order" holds; `runSearchForm`'s deferred-bootstrap paragraph matches `deferredBootstrapFromContext`. No process-artifact references (no task ids, phases or § numbers) in the source.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — settling it requires executing both lanes; reading the suite cannot. The integration lane must be run with `-p 1` from the project root.
