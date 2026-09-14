# Analysis Tasks: Open With Forced Filter (Cycle 1)

## Task 1: Single-source the session set the sigil counts and the picker lists

severity: duplication
sources: duplication, architecture

**Problem**: "The searched set is the set the picker lists" is one contract implemented as two hand-written computations in two packages. `searchCandidates` (`cmd/open_search.go:99-112`) reads `tmux.InsideTmux()`, then `CurrentSessionName()`, treats a failed or empty read as "exclude nothing", and deletes the current session from the enumeration. `Model.filteredSessions` (`internal/tui/model.go:1115-1126`) re-states the same rule over `m.insideTmux` / `m.currentSession`, whose values come from a third copy of the same guard in `openTUI` (`cmd/open.go:722-728`). They agree today only because three separately-written guards happen to fold the same way. A later change to which sessions the picker shows — hiding a gone-flagged row, listing the current session with a marker, filtering on any new predicate — lands in one copy, and `/term` then takes K over a different set than the list it hands the user: it attaches a session the picker would never have offered, or paints a one-row picker after promising to attach when exactly one matched. Nothing fails to compile, `cmd`'s exclusion test and `internal/tui`'s grouping suites both stay green against their own copy, and the user meets it as "portal attached me to a session I did not pick". The underscore-prefixed exclusion is already shared correctly (both routes reach it through `parseSessionList`), which makes this remaining hand-authored exclusion the odd one out rather than an unavoidable split.

**Solution**: Declare the rule once as a pure exported helper in `internal/tui` — `func PickerSessions(sessions []tmux.Session, currentSession string) []tmux.Session`, holding "drop the session the caller is in; an empty `currentSession` drops nothing", which collapses the inside/outside-tmux branch into the argument — and route both `Model.filteredSessions` and `searchCandidates` through it. `internal/tui` rather than `internal/tmux` (the architecture agent's alternative home) because the rule is what the *picker* shows, and `internal/tmux` is the tmux CLI wrapper: it must not learn a presentation policy, while `cmd` already imports `internal/tui` and takes no new edge. `searchCandidates` keeps its own `tmux.InsideTmux()` / `CurrentSessionName()` read and its error policy, passing the resolved name in, so only the set rule becomes single-sourced and the seam stays where it is. The helper allocates rather than deleting in place — `searchCandidates` today uses `slices.DeleteFunc`, which mutates and zeroes the tail of the slice it was handed; it owns that enumeration outright, so no caller observes the change.

**Outcome**: One declaration of the picker's visible set, called from both sides, so a future change to the exclusion cannot land on one route only.

**Do**:
- Add `internal/tui/picker_sessions.go` holding `func PickerSessions(sessions []tmux.Session, currentSession string) []tmux.Session`: return `sessions` unchanged when `currentSession == ""`; otherwise build a fresh slice (`make([]tmux.Session, 0, len(sessions))`) carrying every session whose `Name != currentSession`, in enumeration order. It allocates — it must never delete in place, because `searchCandidates` hands it an enumeration whose tail `slices.DeleteFunc` would zero.
- Rewrite `Model.filteredSessions` (`internal/tui/model.go:1115-1126`) as `return m.sessions` when `!m.insideTmux`, else `return PickerSessions(m.sessions, m.currentSession)`. Keep the `m.insideTmux` guard at the call site: `Build` only ever sets the pair together, but a hand-built model can carry a name without the flag, and today's code returns the whole set for it.
- Rewrite the tail of `searchCandidates` (`cmd/open_search.go:99-112`) as `return tui.PickerSessions(sessions, current), nil`, leaving the `ListSessionsProbe` call, the `tmux.InsideTmux()` gate, the `CurrentSessionName()` read and its "a failed or empty read drops nothing" policy exactly where they are. Drop the `slices` import — line 111 is its only use in the file.
- Leave the underscore-prefixed exclusion alone: both routes already share it through `parseSessionList`, and it is no part of this helper.

**Acceptance Criteria**:
- [ ] `PickerSessions` returns every session whose name differs from `currentSession`, in enumeration order, and returns the whole set for an empty `currentSession` or a name no session carries
- [ ] `PickerSessions` leaves the slice it was handed untouched — same length, same elements, no zeroed tail — so a caller holding the enumeration elsewhere reads it unchanged
- [ ] `searchCandidates` and `Model.filteredSessions` each reach the exclusion only through `PickerSessions`: `grep -n 'DeleteFunc' cmd/open_search.go` returns nothing, and neither file carries a remaining hand-written loop or delete over `s.Name`/`currentSession`
- [ ] Behaviour is unchanged on both routes: `TestOpenCommand_SearchForm_ExcludesTheCurrentSessionFromTheCount`, `TestOpenCommand_SearchForm_AttachesTheOtherMatchWhenTheCurrentSessionAlsoMatches`, `TestOpenCommand_SearchForm_CountsNothingOutWhenTheCurrentSessionReadFails`, `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux` and `internal/tui`'s `"excludes the current session when inside tmux"` all pass with no edit to any of them
- [ ] `go test ./...` is green, `gofmt -l` reports nothing and `go vet ./...` is clean

**Tests**:
This is a pure refactor: behaviour is unchanged, every existing test stays green untouched, and the new tests below cover only the newly exported helper and the non-mutation property the move introduces.
- `"it drops the named session and keeps the rest in enumeration order"`
- `"it drops nothing when the current session name is empty"`
- `"it drops nothing when no session carries the name"`
- `"it leaves the caller's slice unmodified"`
- `"searchCandidates leaves the enumeration it was handed unmodified"`

## Task 2: Derive the searchable field list from one declaration

severity: duplication
sources: duplication, architecture

**Problem**: The field set a session is searchable by — its name plus its recorded directory in home-abbreviated form, with an empty directory falling back to the name alone — is authored twice. `resolver.MatchesSearchTerm` (`internal/resolver/search.go:12-27`) states it as an enumeration of two `strings.Contains` arms; `SessionItem.FilterValue` (`internal/tui/session_item.go:81-93`) states it again as a join, `Name + " " + AbbreviateHome(Dir)`. The matching *rule* diverging by entry point is deliberate (containment for `/term`, fuzzy for the picker's own filter); the *fields* are meant to be one thing all three entry points share, and the joined form is derivable from the enumerated one rather than needing its own authorship. Widen or narrow the domain in one place — the specification names narrowing back to names alone as an anticipated reversal if the false-positive rate proves intolerable, and adding a project name or a tag is the ordinary additive change — and `/term` and `-f text` return different session sets for the same text: the user types `/acme`, the sigil reports no match and opens a picker showing nothing while `-f acme` on the same machine lists the session, or `/acme` opens a picker on two rows where `-f` lists three. No compile error, no test failure: `internal/resolver`'s containment tests and `internal/tui`'s `FilterValue` tests each keep passing against their own copy. The coupling is carried only by a prose warning on `FilterValue` ("widening or narrowing what a filter matches means changing both") — a comment doing a derivation's job, and itself falsified by any additive edit to either file.

**Solution**: Declare the field list once in `internal/resolver` — `func SearchFields(name, recordedDir string) []string`, returning the name and, when `recordedDir` is non-empty, `AbbreviateHome(recordedDir)` — and derive both consumers from it. `MatchesSearchTerm` becomes a case-folded containment loop over the returned slice, which preserves the per-field test exactly (a run spanning the end of one field and the start of the next is still not a match). `FilterValue` becomes `strings.Join(resolver.SearchFields(...), " ")`, where the one-element slice joins to the bare name, so the no-trailing-separator case falls out rather than being branched on. The `FilterValue` doc comment then shrinks to the one thing still worth saying — that the two entry points read the same fields by different rules, and why the join order matters.

**Outcome**: Widening or narrowing the search domain is one edit both entry points inherit.

**Do**:
- Add `func SearchFields(name, recordedDir string) []string` to `internal/resolver/search.go`: `[]string{name}` when `recordedDir == ""`, else `[]string{name, AbbreviateHome(recordedDir)}`. The abbreviation belongs to the field, so every consumer gets the displayed form without asking for it.
- Rewrite `MatchesSearchTerm` (`internal/resolver/search.go:12-27`) over it: keep the empty-term guard returning false, fold the term once with `strings.ToLower`, then loop the slice testing `strings.Contains(strings.ToLower(field), folded)` and returning true on the first hit. The fields stay tested separately — nothing joins them on this path.
- Rewrite `SessionItem.FilterValue` (`internal/tui/session_item.go:89-94`) as `strings.Join(resolver.SearchFields(i.Session.Name, i.Session.Dir), " ")`, deleting the `Dir == ""` branch: a one-element slice joins to the bare name, so the no-trailing-separator case falls out of the join. Add the `strings` import.
- Trim the `FilterValue` doc comment's "widening or narrowing what a filter matches means changing both" warning — the derivation now carries it. Keep one line on why the join order matters; the wording is yours.
- Leave the two display-side `AbbreviateHome` calls alone (`session_item.go`'s row render, `session_dir_column.go`) — they abbreviate for the column, not for the match. Accept that `SearchFields` abbreviates eagerly where `MatchesSearchTerm` used to short-circuit on a name hit: `FilterValue` already pays that per item per filter pass, so add no cache.

**Acceptance Criteria**:
- [ ] `SearchFields` returns the name alone for an empty recorded directory, and the name plus the home-abbreviated directory otherwise
- [ ] `MatchesSearchTerm` still tests each field separately: a term spanning the end of the name and the start of the directory is not a match, and an empty term matches nothing
- [ ] `FilterValue` for a session with no recorded directory is the bare name with no trailing space; with one it is `"<name> <home-abbreviated dir>"`, name first
- [ ] Neither consumer restates the field list: `internal/tui/session_item.go`'s `FilterValue` names no directory field of its own, and the only `AbbreviateHome` calls left in `internal/tui` are the two display ones
- [ ] `TestMatchesSearchTerm`, `TestMatchesSearchTermUnresolvableHome` and the existing `FilterValue` subtests pass with no edit
- [ ] `go test ./...` is green, `gofmt -l` reports nothing and `go vet ./...` is clean

**Tests**:
This is a pure refactor: the matched set is unchanged on all three entry points, every existing test stays green untouched, and the new tests below pin the new declaration and the derivation of the joined form from it.
- `"it returns the name alone when no directory is recorded"`
- `"it returns the name and the home-abbreviated directory, name first"`
- `"FilterValue is the SearchFields slice joined by a single space"`
- `"FilterValue for a session with no recorded directory has no trailing separator"`
- `"a term spanning the name and the directory is not a containment match"`

## Task 3: Make the containment filter's item source self-contained

severity: medium
sources: architecture

**Problem**: `searchItemSource` (`internal/tui/search_filter.go:12-22`) is a mutable cell written on Bubble Tea's Update goroutine by `rebuildSessionList` (`internal/tui/model.go:1230-1235`) and read on a command goroutine by the filter closure (`internal/tui/search_filter.go:34-52`). `list.SetItems` returns `filterItems(m)` whenever the filter is applied (`bubbles/v2@v2.1.0/list/list.go:380-392`), and bubbletea runs every `Cmd` in its own goroutine (`bubbletea/v2@v2.0.7/tea.go:721`) — while the write is reachable from the very next `Update` after the one that returned the cmd. Two concrete windows exist on every search-opened picker: `refetchSessionsAfterRestore`'s `SessionsMsg` rebuild dispatches the filter cmd, then `ProjectsLoadedMsg` (or a second `SessionsMsg`) rebuilds and re-points the source underneath it. The correctness of the whole containment rule therefore rests on the Update loop and the command goroutine never overlapping, which nothing enforces. The `len(items) != len(targets)` guard is a proxy for staleness rather than a test of it: it catches a regroup (headers appear or vanish, so the count moves) and misses every same-length replacement — a rename, a refresh with the same session count — which is exactly what this race produces, and `bubbles` then maps each rank into the items *its* closure captured, so the narrowed list shows rows that do not contain the term and the user Enters on the wrong session. The unsynchronised slice-header read can also tear. Nothing in the unit lane drives the real program loop, so `-race` cannot see it today; the first end-to-end test that does will abort the run.

**Solution**: Make the source define the access rather than relying on caller discipline. Guard `set`/`current` with a `sync.RWMutex` — it is already an accessor-wrapped pointer type, so the change is confined to those two methods — and have `set` store the item slice together with the `[]string` of `FilterValue()`s it was built from, returning both from `current()` under the same hold. The filter then compares the `targets` it was handed against that recorded companion (`slices.Equal`) instead of against a length, which turns the fallback into a real identity check and closes the same-length window the mutex alone leaves open. The fallback behaviour is unchanged in kind: a source out of step still yields `list.DefaultFilter`, so the picker's own rule stands rather than a lookup against the wrong rows.

**Outcome**: The containment filter is correct on its own terms, not on the schedule of the Update loop, and a future `-race` end-to-end test can drive a search-opened picker.

**Do**:
- Give `searchItemSource` (`internal/tui/search_filter.go:12-22`) a `sync.RWMutex` and a second field beside `items`: the `[]string` of filter values those items were built from.
- `set(items []list.Item)` takes the write lock and stores both — the slice it was handed, and a freshly allocated `make([]string, len(items))` filled with each `item.FilterValue()` in order. Build a new companion on every call; never mutate one a previous `current` handed out.
- `current()` returns both under one read-lock hold, so a reader can never pair one call's items with another call's values.
- In `containmentFilter` (`internal/tui/search_filter.go:34-52`), take both from `current()` and fall through to `list.DefaultFilter(query, targets)` when `query != term` **or** `!slices.Equal(recorded, targets)` — the length comparison goes. Leave the rest as it stands: the `SessionItem` type assertion that skips headers, the rank loop over the source's own items in list order, and the nil `MatchedIndexes`.
- The doc comment above `containmentFilter` states the fallback as a length mismatch. Restate it for the identity check in one line; the wording is yours.
- Add the `slices` and `sync` imports. `rebuildSessionList`'s re-point (`internal/tui/model.go:1230-1235`) stays exactly where it is — it is the writer the lock now covers, not something to move.

**Acceptance Criteria**:
- [ ] Every read and write of `searchItemSource`'s fields happens inside `set` or `current` under the mutex — no other file touches them
- [ ] `current` returns the items and the recorded filter values from a single hold
- [ ] The filter falls through to `list.DefaultFilter` whenever the recorded values differ from `targets`, **including a same-length replacement** — a rename, or a refresh returning the same number of sessions
- [ ] A source holding exactly the slice the targets were built from still narrows by containment, in the list's own order, skipping `HeaderItem` rows, with `MatchedIndexes` nil
- [ ] A never-`set` source still behaves as today: an empty target list yields no ranks, a non-empty one falls through to the picker's own rule
- [ ] `go test -race ./internal/tui` is green, including a test driving `set` concurrently with a filter pass
- [ ] `TestContainmentFilterFallsBackWhenTheSourceIsOutOfStep` and the `TestSearchContainment*` suites pass with no edit
- [ ] `gofmt -l` reports nothing and `go vet ./...` is clean

**Tests**:
- `"it falls back to the picker's own rule when the source was replaced at the same length"`
- `"it falls back to the picker's own rule when the source is out of step by length"`
- `"it narrows by containment when the recorded values match the targets"`
- `"it falls back when the filter text is no longer the supplied term"`
- `"it is race-free with a set running against a concurrent filter pass"`

## Task 4: Deliver the soft bootstrap warnings the warm picker route drops

severity: medium
sources: standards

**Problem**: `isTUIPath` (`cmd/root.go:169-176`) now answers true for `portal open /term`, so a soft bootstrap warning — a failed `_portal-saver` revive adding `SaverDownWarning` to the sink — is deliberately withheld from stderr and staged onto the TUI model instead (`cmd/open.go:733` → `SetPendingBootstrapWarnings`). On a warm install whose bootstrap latch is satisfied, the model is built with `activePage == PageSessions`, so `Init`'s loading branch (`internal/tui/model.go:1522-1540`) never runs, no `BootstrapCompleteMsg` is synthesized, `m.bufferedWarnings` is never populated, and `surfaceBufferedWarnings` — whose one call site is `dismissLoadingGate` — is never reached. The staged slice dies on the model: the warning reaches neither the notice band nor stderr. For a user whose state daemon is down, `x /port` lands them in the picker with no sign that nothing is being captured; they find out at the next reboot, when `sessions.json` and every pane's scrollback are gone. The same silent drop now applies to `portal open -- ls`, which the p4 classification change also moved onto the picker path — both were lines that previously carried a positional and so emitted to stderr from `PersistentPreRunE`. The hole predates this work (bare `portal open` and `-f` already fall into it), but the search form's classification is what newly routes these lines into it, so the guarantee the specification states for this form is not met on the warm route. The classification is correct and is not in question; the delivery path behind it is what is missing — and `finishTUI` already owns a post-teardown writer and already writes there for the K=1 attach and read-failure teardowns, so the alt-screen corruption the classification exists to prevent cannot occur at that point.

**Solution**: Make the pending→buffered handoff a move, then write whatever is still owed at teardown. In the `BootstrapCompleteMsg` arm of `Update` (`internal/tui/model.go:1626-1632`), clear `m.pendingBootstrapWarnings` in the same breath as `m.bufferedWarnings = msg.Warnings` — the arm holds `m` by value and returns it, so the clear propagates where `Init`'s value receiver could not. `PendingBootstrapWarnings()` then reads as "warnings no loading gate ever consumed", which is non-empty on exactly the warm no-loading-page route. In `finishTUI` (`cmd/open.go:610-618`), after the terminal background restore and alongside `emitSearchTeardownWarnings`, write `model.PendingBootstrapWarnings()` to the warnings writer. The two sources stay disjoint — `emitSearchTeardownWarnings` keeps writing `BufferedWarnings()` for the attach and read-failure teardowns, where the decision quit before `surfaceBufferedWarnings` could empty the buffer — so nothing is written twice on the cold route. Post-teardown stderr rather than a warm-route notice band, following the precedent the specification already sets for the K=1 attach: the warm picker paints from its first frame with no gate to hang a band on, and the band would be surfacing a warning about a bootstrap that finished before the process reached the TUI at all. Leave the concurrent route's notice band exactly as it is.

**Outcome**: A down state daemon is reported on every picker invocation again — warm and cold, sigil, `-f` and bare — and the warning lands after the alt-screen is gone rather than into the frame the picker is about to claim.

**Do**:
- In the `BootstrapCompleteMsg` arm of `Update` (`internal/tui/model.go:1618-1633`), clear `m.pendingBootstrapWarnings` inside the existing `if m.activePage == PageLoading` block, in the same breath as `m.bufferedWarnings = msg.Warnings`. The arm holds `m` by value and returns it, so the clear reaches the caller where `Init`'s value receiver could not; putting it inside the block is what keeps the cancelled loading page owed nothing, since its warnings became buffered and the existing teardown rule already declines to report them.
- Give `PendingBootstrapWarnings` (`internal/tui/model.go:463-465`) a one-line doc: it now reads as the warnings no loading gate consumed. Wording is yours.
- In `finishTUI` (`cmd/open.go:610-618`), after `tui.RestoreTerminalBackground` and alongside `emitSearchTeardownWarnings`, add `tui.WriteBootstrapWarnings(warnings, model.PendingBootstrapWarnings())`. Both writes must stay ahead of `processTUIResult` — the outside-tmux attach execs and never returns.
- Change nothing else: the concurrent route's notice band, `stageBootstrapWarningsOnModel`, `surfaceBufferedWarnings`, `emitSearchTeardownWarnings`'s own `BufferedWarnings()` write and `isTUIPath`'s classification all stay as they are.
- A warm picker model for a test is `tui.Build(tui.Deps{Lister: …})` with `ServerStarted` left false — that is what lands on `PageSessions` from frame one with no gate — then `SetPendingBootstrapWarnings` before `finishTUI`.

**Acceptance Criteria**:
- [ ] On the warm picker route — model on `PageSessions` from frame one, no progress receiver, no loading gate — `finishTUI` writes the staged warnings to its warnings writer, byte-identical to `warning.WriteLines`' rendering on the CLI path
- [ ] The `BootstrapCompleteMsg` arm clears the pending slice whenever it buffers it, and the cleared value is on the model the arm returns
- [ ] Nothing is written twice: on the warm loading-page route `surfaceBufferedWarnings` surfaces them and `finishTUI` adds nothing, and on the cold concurrent route the notice band still owns them and `finishTUI` adds nothing
- [ ] On the K=1 decision attach and the failed-read teardown, `emitSearchTeardownWarnings` still writes `BufferedWarnings()` exactly once and the new write contributes nothing
- [ ] A cancelled loading page still writes nothing
- [ ] `finishTUI` writes nothing when no warnings accumulated
- [ ] Both writes land before the connector runs
- [ ] A warm `portal open -f term` and a warm bare `portal open` report a down saver at teardown too, not only the sigil form

**Tests**:
- `"it writes the staged bootstrap warnings at teardown when no loading gate consumed them"`
- `"it writes the same lines as the CLI path"`
- `"it writes nothing at teardown when the loading gate already surfaced them"`
- `"it clears the pending warnings when the complete message buffers them"`
- `"it writes nothing at teardown when no warnings accumulated"`
- `"it writes the staged warnings before the connector runs"`
- `"it writes the staged warnings for a warm picker opened by -f and by a bare open"`
