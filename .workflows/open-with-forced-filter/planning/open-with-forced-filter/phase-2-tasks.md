---
phase: 2
phase_name: The Single-Match Shortcut Under Containment Matching
total: 4
---

# Phase 2: The Single-Match Shortcut Under Containment Matching — 4 tasks

## open-with-forced-filter-2-1

### Task 2-1: Decide a session match by case-folded containment over name and directory

**Problem**: The sessions list filters through `bubbles/list.DefaultFilter` — `sahilm/fuzzy`'s subsequence match, anywhere in the text, rank-sorted — and Phase 1 left the search form on exactly that rule. It is harmless inside the picker, where the user reads the rows and skips the nonsense ones. It is not harmless for the single-match shortcut this phase adds, which acts on a lone survivor without ever showing it: under subsequence matching a session at `~/Projects/rust-tools` satisfies `port` (**p** in `projects`, **o** in `projects`, **r** in `rust`, **t** in `tools`, the four letters never appearing together), so `/port` could attach a session the user never saw and would not have chosen. `/port` finding nothing is a better failure than `/port` attaching `rust-tools`. The eager attach cannot land until a strict rule exists to decide it, and the two deciders — the count taken in `cmd` and the narrowed list in the picker — must reach identical verdicts from one implementation rather than two.

**Solution**: One pure predicate in `internal/resolver`, testing a term for case-folded containment against a session's name and, separately, against its home-abbreviated recorded directory, with the two fields never joined.

**Outcome**: `resolver.MatchesSearchTerm` answers true when the term appears as a contiguous run in either field case-folded, false for a run that exists only across the two fields, treats `*`, `?` and `[` as ordinary characters to find, and judges a session carrying no recorded directory on its name alone.

**Do**:
- Add `internal/resolver/search.go` with `func MatchesSearchTerm(term, name, recordedDir string) bool`: return false for an empty term; fold the term once with `strings.ToLower`; return true when `strings.ToLower(name)` contains it; otherwise return false when `recordedDir == ""`, else return whether `strings.ToLower(AbbreviateHome(recordedDir))` contains it.
- Test the two fields with two separate `strings.Contains` calls against two separate folded strings — build no joined value, and call neither `filepath.Match` nor `HasGlobMeta`, so `*`, `?` and `[` reach `strings.Contains` as ordinary characters.
- Take the directory's matched form from `AbbreviateHome` (`internal/resolver/path.go`, added in task 1-2) rather than re-deriving the home prefix, so the text this rule searches is the same value `SessionItem.FilterValue()` joins.
- Add `internal/resolver/search_match_test.go` as a table over the shapes in **Tests**, with every home-relative case anchored on `t.Setenv("HOME", <t.TempDir()>)` so the verdict does not depend on the developer's account name.
- Wire the rule nowhere: its two callers are the count (task 2-2) and the picker's narrowed list (task 2-3).

**Acceptance Criteria**:
- [ ] A term appearing as a contiguous run in the session name is a match, in either case direction (`PORT` against `portal-a1b2`, `port` against `PORTAL-A1B2`)
- [ ] A term appearing as a contiguous run in the recorded directory's home-abbreviated form is a match, and the abbreviation is applied before the comparison (`Code` matches `<home>/Code/portal`; the account name and `Users` match nothing)
- [ ] A directory outside the home directory is compared unabbreviated (`opt` matches `/opt/tools`)
- [ ] A run that spans the end of the name and the start of the directory is **not** a match — the two fields are never joined for this rule
- [ ] A session with an empty recorded directory is judged on its name alone and never panics or matches vacuously
- [ ] `*`, `?` and `[` in the term are literal: a session named `po*rt-x` matches the term `po*rt`, and a session named `portal-a1b2` does not match the term `po*`
- [ ] An unresolvable home directory degrades to comparing the raw recorded path rather than failing
- [ ] An empty term returns false, so a caller that forgets its own guard fails closed rather than matching every session
- [ ] The predicate touches no filesystem beyond `AbbreviateHome`'s `$HOME` read and issues no tmux call
- [ ] `go test ./...` passes

**Tests**:
- `"it matches a term contained in the session name"`
- `"it matches case-insensitively in both directions"` — upper-cased term against a lower-cased name, and the reverse
- `"it matches a term contained in the home-abbreviated recorded directory"` — name `api-work`, dir `<home>/Code/portal`, term `portal`
- `"it does not match the home prefix the abbreviation replaced"` — term equal to the home directory's last segment, and the term `Users`, both false
- `"it leaves a directory outside the home directory unabbreviated"` — dir `/opt/tools`, term `opt`
- `"it does not match a run that spans the name and the directory"` — name `api`, dir `/opt/tools`, term `i /o`
- `"it matches on the name alone when the recorded directory is empty"`
- `"it does not match on an empty recorded directory"` — a term that would have matched the directory returns false
- `"it treats a glob metacharacter in the term as a literal character"` — term `po*rt` matches name `po*rt-x`; term `po*` does not match name `portal-a1b2`
- `"it falls back to the raw recorded path when the home directory cannot be resolved"` — `t.Setenv("HOME", "")`, dir `/Users/leeovery/Code/portal`, term `Users` matches
- `"it returns false for an empty term"` — against a populated name and directory

**Edge Cases**:
- An empty term is never handed to the rule in production: the term-less form takes no count at all (task 2-2) and the picker's filter machinery short-circuits an empty query before any filter func runs (task 2-3). Returning false rather than true is the fail-closed choice — with an empty term matching everything, a machine holding exactly one live session would be attached to it, which is the one outcome `x /` must never produce
- A sigil term can never contain `/` (the recognition rule refuses a second slash), so the rule never sees a path fragment with separators; it is nonetheless plain containment and needs no special case for one
- Case folding is `strings.ToLower` on both sides, matching the property the sigil keeps from the picker's own rule; the two rules diverge on subsequence-versus-containment and on nothing else
- A recorded directory that is exactly the home directory abbreviates to `~`, so a term matching `~` matches it; a recorded directory carrying a trailing separator abbreviates to `~/` — whatever was stamped is what is searched, and nothing normalises it
- A session created before the directory stamp shipped carries no recorded directory and is matchable by name alone, unconditionally and in every view

**Context**:
> The sigil matches by containment — the typed characters appearing as a run in the matched text — case-folded. The two fields are tested separately for the sigil: a session matches when the term appears as a run in its name, or as a run in its recorded directory. They are never joined into a single string on this path — a term matching across the join would return a session that neither field contains, and the sigil can act on a lone match without showing it.
>
> The term is literal text. `*`, `?` and `[` are characters to find rather than wildcards: `/port*` matches a session whose name or recorded directory contains `port*`, and nothing else does. The glob forms answer ambiguity by opening a window per match; the sigil answers it by narrowing, and the two rules are not mixed.
>
> The searched form is the displayed form. A recorded directory under the user's home is searched home-abbreviated (`~/Code/portal`), not as tmux recorded it — otherwise a term hitting the home prefix (`/lee`, `/user`) would match every session the user has while every returned row displayed no such text, and on a lone survivor would attach outright with nothing on screen accounting for the choice.
>
> Never a derived directory: the recorded value that rides back with the session list is the only one the count can read, and matching it alone makes the answer identical everywhere — the count and the list, the shell and the picker.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §4.1, §4.3, §4.4

## open-with-forced-filter-2-2

### Task 2-2: Attach directly when exactly one live session matches the term

**Problem**: After Phase 1 every search form opens the picker, whatever it matches. The form's whole point is the flow it collapses — `x`, wait, `/`, type, `Enter`, preview the survivors, `Enter` — and the shortcut past it is the lone match: when exactly one live session answers the term there is nothing to choose between, so showing a list of one is ceremony. The count has to be taken in `cmd` before the model is built, because a model that took its own count would paint a loading or empty frame on the warm path before deciding to tear itself down for an attach. Nothing on the path knows what the searched set is either: `resolver.SessionLister` returns names only, and the rule needs each session's recorded directory.

**Solution**: A search-form runner in `cmd` that enumerates the live sessions the picker would list, counts the matches with `resolver.MatchesSearchTerm`, and forks three ways — attach through the existing session-connector seam on exactly one, open the Phase 1 picker landing on zero or two-plus, and take no count at all for the term-less form.

**Outcome**: `portal open /port` attaches directly when one live session matches, with no picker and no new connection mode; it opens the pre-filtered picker on zero matches with nothing on stderr and a zero exit, and on two or more with the cursor on the first matching row; `portal open /` still opens the whole live list without enumerating anything for a count.

**Do**:
- Add to `cmd/open_search.go` (the file task 1-5 created): `type SearchSessionSource interface { ListSessions() ([]tmux.Session, error); CurrentSessionName() (string, error) }`, satisfied by `*tmux.Client`; a `SearchSessions SearchSessionSource` field on `OpenDeps` (`cmd/open.go`); and `func buildSearchSessionSource(cmd *cobra.Command) SearchSessionSource` returning the injected seam when non-nil and `tmuxClient(cmd)` otherwise.
- Add `func searchCandidates(src SearchSessionSource) ([]tmux.Session, error)`: return the enumeration, propagating its error unchanged; then, only when `tmux.InsideTmux()`, drop the session `src.CurrentSessionName()` names — a failed or empty read drops nothing, matching how `openTUI` already treats that read.
- Add `func searchMatches(term string, sessions []tmux.Session) []tmux.Session` returning, in enumeration order, the sessions satisfying `resolver.MatchesSearchTerm(term, s.Name, s.Dir)`. Add no filtering of its own — Portal's `_`-prefixed internal sessions are already absent from the enumeration.
- Add `func runSearchForm(cmd *cobra.Command, term string) error`: an empty term returns `openTUIFunc(cmd, pickerLanding{search: true}, nil, serverWasStarted(cmd))` without touching the source at all; otherwise enumerate (returning any error to the caller), match, and either `openSessionFunc(cmd, matches[0].Name)` for exactly one or the same picker landing carrying `filter: term` for any other count.
- Point task 1-5's `RunE` search branch at `runSearchForm(cmd, resolver.SearchTerm(forms[0]))`, leaving `emitResolveDecision`, `buildQueryResolver`, `orderedOpenTargets`, `writeAckMarker`, `isTUIPath` and `shouldRunConcurrentBootstrap` untouched.
- Re-point task 1-5's `"it opens the picker for a term that equals a live session name"` to the superseding expectation: that line now attaches, because such a term matches exactly one live session.

**Acceptance Criteria**:
- [ ] With exactly one live session matching the term, `openSessionFunc` is called with that session's name and `openTUIFunc` is never called
- [ ] The single match is connected through `openSessionFunc` — the same seam the exact-session and `-s` pin paths take — so the inside-tmux `switch-client` / outside-tmux exec choice stays `buildSessionConnector`'s and no connector type or connection call is added here
- [ ] With zero matches over a live session list, the picker opens with the landing `{filter: term, search: true}`, `RunE` returns nil (a zero exit) and nothing is written to stderr
- [ ] With two or more matches the picker opens with the same landing
- [ ] A session matched on its recorded directory alone counts toward the total and is attached when it is the only one
- [ ] `portal open /` calls neither `ListSessions` nor `CurrentSessionName` and lands the term-less picker exactly as task 1-6 leaves it
- [ ] Inside tmux the session the user is attached to is not a candidate, so the count is taken over the same set the picker lists; outside tmux nothing is excluded
- [ ] A glob metacharacter in the term is literal: `/po*` counts sessions containing `po*` and dispatches no burst
- [ ] No record is emitted under the `resolve` component on any of the three outcomes
- [ ] An enumeration error is returned to the caller rather than counted as zero matches
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it attaches the single matching session without opening the picker"` — stage `openSessionFunc` and `openTUIFunc`, assert the name and that the picker seam never ran
- `"it attaches a session matched only by its recorded directory"` — name `api-work`, `Dir: <home>/Code/portal`, term `portal`
- `"it opens the picker when nothing matches"` — assert the landing, a nil error and an empty stderr buffer
- `"it opens the picker when two or more sessions match"`
- `"it opens the picker when no sessions are live"`
- `"it takes no count for the term-less form"` — a source recording every call, asserting zero
- `"it excludes the session the user is currently in from the count"` — inside tmux (`t.Setenv("TMUX", …)`), the only matching session is the current one → the picker opens
- `"it attaches the other match when the current session also matches the term"` — two matches inside tmux, one of them current → attach the other
- `"it counts nothing out when the current-session read fails"` — `CurrentSessionName` erroring leaves the candidate set whole
- `"it counts over exactly the sessions the enumeration returns"` — no second filter of its own
- `"it treats a glob metacharacter in the term as literal and dispatches no burst"` — `/po*` against `po*rt-x` and `portal-a1b2`; assert `runOpenBurstFunc` never ran
- `"it attaches a session whose name equals the term"` — the expectation task 1-5 pinned the other way
- `"it emits no resolve component line on any outcome"` — `logtest.Install`, all three counts
- `"it returns an enumeration error rather than opening the picker"` — an erroring source; assert the error surfaces and neither seam ran

**Edge Cases**:
- The term-less form takes no count: `x /` opens on the whole live list on a machine holding one live session as on a machine holding twenty, so the enumeration must be skipped entirely rather than run and ignored
- `_portal-saver` and `_portal-bootstrap` are absent from the enumeration (`internal/tmux` filters `_`-prefixed names at source), so they can never be counted or attached. Restating that filter here would be a second home for a Portal-wide invariant — the search branch must not
- Inside tmux the picker's own list omits the session the user is in (`filteredSessions`), so the count omits it too; counting it would let a lone "match" resolve to the session already on screen, or hand the user a two-match picker showing one row
- The count read and the picker's own session read are two separate tmux reads, so a session can vanish between them; the picker lists what is live when it loads, and a session that dies between the count and a K=1 attach fails with tmux's own words from the connector
- A term carrying `*`, `?` or `[` reaches this branch ahead of the multi-target gate (task 1-5), so it can never be read as a glob and burst
- Any composition beside the form is refused by the `Args` validator before `RunE` (task 1-7), so this runner never sees a command, a pin or a second target
- An enumeration failure is returned rather than reported as zero matches, but until task 2-4 the underlying `ListSessions` only errors on malformed output — a failed read still reads as an empty server on this path until then

**Context**:
> Let K be the number of live sessions matching the term. K = 1 attaches the matching session directly with no picker; K = 0 opens the picker on the Sessions page with no session surviving the term; K ≥ 2 opens it with the cursor on the first matching row. The sigil never fails — the form is the pre-filtered picker, and the single-match attach is the one shortcut past it; a count of zero is a filter result rather than an error, nothing is written to stderr and the exit status is not a failure.
>
> A term-less sigil takes no count. `x /` carries nothing to match, so no count is evaluated and none of the rows apply: it opens the picker on the whole live session list, on a machine holding one live session as on a machine holding twenty.
>
> The searched set is the set the picker lists. Portal's own internal sessions — the `_portal-saver` daemon host and the `_portal-bootstrap` server anchor — are absent from that list and are never search candidates: no term counts one toward K, and none can be attached by a sigil.
>
> An attach under K = 1 uses the connector the invocation already selects — `syscall.Exec` into `tmux attach-session` outside tmux, `switch-client` inside it. The sigil introduces no third connection mode.
>
> The sigil emits no `resolve` component line: that component records one INFO line per bare positional resolved through the guessing chain, and a sigil declares its domain the way a pin does.
>
> The count is taken in `cmd` before the model is built. A model-side count would have the warm path paint one frame of a picker it is about to tear down for an attach.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §3.2, §3.4, §3.6, §3.7, §4.3

## open-with-forced-filter-2-3

### Task 2-3: Narrow a search-opened list by containment while the term stands untouched

**Problem**: The count and the list must agree. Task 2-2 decides K by containment, but the picker it opens on K = 0 and K ≥ 2 still narrows by `list.DefaultFilter`'s fuzzy subsequence rule, so a term that counted two would show five — rows the user never searched for, sitting beside the ones they did. The list a user chooses from has to be the list the count was taken over. Installing the rule is not a one-line filter swap either: `list.FilterFunc` is handed only a term and `[]string` built from `FilterValue()`, which joins the name and the abbreviated directory with a single space — a character a tmux session name may legally contain — so the fields cannot be recovered by splitting that string, and the items the ranks index into are rebuilt on every regroup, preview-dismiss and refresh rather than fixed at construction.

**Solution**: A containment `list.FilterFunc` installed on the sessions list only for a picker a search form opened, resolving each `Rank.Index` back to the `SessionItem` through a pointer-held item source that `rebuildSessionList` re-points on every rebuild, and falling through to `list.DefaultFilter` the moment the committed text is anything other than the term the form supplied.

**Outcome**: A search-opened picker shows exactly the sessions the count matched, in the order the list already gives them with nothing re-ranked, reproduced across a regroup, a preview and back, and a refresh that drops an externally-killed session; editing the filter text to any other value returns the list to the picker's own fuzzy rule, and editing it back to the identical term restores containment.

**Do**:
- Add `internal/tui/search_filter.go` with a pointer-held `searchItemSource` (a `[]list.Item` field plus `set` / `current` methods) and `func containmentFilter(term string, src *searchItemSource) list.FilterFunc`. The closure returns `list.DefaultFilter(query, targets)` whenever `query != term` or `len(src.current()) != len(targets)`; otherwise it walks the source in index order and appends `list.Rank{Index: i}` for every element that is a `SessionItem` satisfying `resolver.MatchesSearchTerm(query, si.Session.Name, si.Session.Dir)`, leaving `MatchedIndexes` nil.
- Add `searchItems *searchItemSource` to `Model` and a `func (m *Model) installSearchFilter()` called at the end of `New` (`internal/tui/model.go:814`, after the options loop): when `m.searchForm && m.searchTerm != ""` it allocates the source and assigns `m.sessionList.Filter = containmentFilter(m.searchTerm, m.searchItems)`; otherwise it does nothing, so every other picker keeps `list.DefaultFilter`.
- In `rebuildSessionList` (`internal/tui/model.go:1184`), call `m.searchItems.set(items)` when the source is non-nil, immediately before `m.sessionList.SetItems(items)`. Comment that the source must be re-pointed before the items are handed over, since the filter pass `SetItems` defers into a `tea.Cmd` resolves its ranks against that slice.
- Leave `applySearchLanding`, `evaluateDefaultPage`, the delegate, `ensureSessionRowSelected` and the grouping builders untouched — the landing already commits the term, and headers drop out on their own because a `HeaderItem` is not a `SessionItem`.
- Add `internal/tui/search_containment_test.go` (external `package tui_test`, driving models through `tui.Build` with `Deps.Search`), plus the source-out-of-step case as an internal `package tui` test calling `containmentFilter` directly.

**Acceptance Criteria**:
- [ ] A model built with `Deps.Search = &SearchForm{Term: t}` shows exactly the sessions matching `t` by containment over name and recorded directory; a session the fuzzy rule would have returned on a scattered-letter subsequence is absent
- [ ] Surviving rows keep the order the session list already gives them — nothing is re-ranked, and a set the fuzzy rule would have rank-sorted differently proves it
- [ ] Group headers are absent under a non-empty term in By Project and By Tag, and the containment set is identical in all three grouping modes
- [ ] An `s` regroup, a `Space` preview and back, and a refresh that drops an externally-killed session each re-render the containment set rather than a fuzzy one
- [ ] Committing any other filter value returns the list to `list.DefaultFilter`; clearing the filter shows every session; committing the identical term again restores containment
- [ ] The term-less form is on the picker's own rule from its first keystroke — no containment filter is installed for it
- [ ] A picker opened by `-f`, or with no filter at all, narrows by `list.DefaultFilter` exactly as before
- [ ] A session whose name contains a space is matched on its own two fields, never by splitting the joined filter value
- [ ] The rebuilt item slice is what the ranks resolve against, so a row can never be matched against another row's fields after a regroup
- [ ] `go test ./...` passes

**Tests**:
- `"it narrows a search-opened list to the containment set"` — assert the visible names
- `"it excludes a session the picker's fuzzy rule would have matched"` — a session at `<home>/Projects/rust-tools` under the term `port`, absent here and present under `-f`
- `"it keeps the list's own order and re-ranks nothing"` — two matching sessions whose fuzzy ranking would invert them
- `"it matches a session found only by its recorded directory"`
- `"it drops group headers under a non-empty term"` — By Project, then By Tag
- `"it reproduces the containment set after an s regroup"` — drive the `s` key through `Update`
- `"it reproduces the containment set after a preview and back"` — `Space` then dismiss
- `"it reproduces the containment set after a refresh that drops a killed session"` — a second `SessionsMsg` without one of the matches
- `"it returns to the fuzzy rule when the filter text is edited"` — `SetSessionListFilter("other")`, then a rank-sorted fuzzy expectation
- `"it restores containment when the text is edited back to the supplied term"`
- `"it shows every session when the filter is cleared"`
- `"it leaves a -f picker on the fuzzy rule"` and `"it leaves a picker opened with no filter on the fuzzy rule"`
- `"it leaves the term-less form on the fuzzy rule from its first keystroke"` — build with `Term: ""`, type one rune, assert a fuzzy-only survivor is present
- `"it matches a session whose name contains a space on its own fields"` — name `my session`, dir `/opt/api`, term `session /opt` (contained in the joined value, in neither field)
- `"it falls back to the picker's rule when the item source is out of step with the targets"` — internal test, mismatched lengths

**Edge Cases**:
- The item source must be a pointer: `Model` is a value type that Bubble Tea copies on every `Update`, so a slice field would go stale in whichever copy the filter closure was built against, while a pointer is shared by every copy
- `SetFilterText` runs the filter pass synchronously while `SetItems` defers it into a `tea.Cmd`; the source is re-pointed before `SetItems` so both orders resolve against the same slice, and the landing is safe because `applySessions` (and its rebuild) always precedes `evaluateDefaultPage`
- `Rank.Index` indexes the **unfiltered** item slice, which includes header rows in the grouped modes — the lookup must be by index into that slice, never by counting session rows
- The list's filter machinery short-circuits an empty query before calling the filter func at all, so a cleared filter shows every item without the containment rule being consulted, and the rule's own empty-term refusal is never reached
- `MatchedIndexes` is left nil because no delegate reads `MatchesForItem`; a delegate that ever highlights matched runes would have to supply them
- The containment set holds on the text rather than on the act: opening the filter input and leaving it untouched, or editing away and back, keeps containment, because the comparison is against the committed value
- A refresh re-anchors the cursor before the deferred filter pass has run, so the re-anchor reads the pre-refresh visible set — pre-existing behaviour, unchanged by this task

**Context**:
> The containment set holds for as long as the sigil's filter text stands untouched. The narrowed list does not sit still — a `Space` preview and back, an `s` regroup, a refresh after a session is killed elsewhere all re-render it — and every one of those reproduces the containment set. The list the user is choosing from is the list they were handed. Only a hand edit of the filter text returns the list to the picker's own rule, and the test is the text rather than the act: while the committed filter value is character-identical to the term the sigil supplied, containment stands. Any other value, a cleared filter included, is the picker's own rule.
>
> Containment narrows the list; it does not reorder it. Rows keep the order the sessions list gives them in whatever grouping mode is current, with non-matching rows removed and nothing re-ranked — the picker's rank-sorting is a property of its fuzzy rule, which the sigil does not use. The first matching row is the first surviving row of that existing order.
>
> The term-less form supplies nothing for that test to hold: `x /` lands with an empty, focused filter — the picker's own filter gesture, reached from the shell — so the first character typed there is already a value the sigil did not supply, and the picker's own rule applies from that keystroke on.
>
> On the two fuzzy routes the fields are joined, as the picker's stock matcher expects — the session name, one space, then the recorded directory in its home-abbreviated form. That join is what the sigil's own rule must not reach through: a session name may legally contain a space, so the fields can only be recovered from the item.
>
> The picker's own filter is untouched by this work. Once the user edits that text the picker's rule applies and the row set can widen. The divergence is therefore visible only by rows appearing, never by rows the user expected going missing.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §4.2, §4.3, §4.4

## open-with-forced-filter-2-4

### Task 2-4: Report a failed session-list read rather than counting it as no matches

**Problem**: `Client.ListSessions` answers a failed `list-sessions` with `([]Session{}, nil)` — the swallow is deliberate and load-bearing, because for the picker and `portal list` that failure *is* the no-server signal (`internal/tmux/tmux.go:129-132`). The search form cannot live with it. K = 0 says the search ran and found nothing, which is a filter result and opens the picker; a read that failed has searched nothing, and opening an empty picker on it tells the user their sessions are gone while they are running. Task 2-2 already returns an enumeration error to the caller, but the enumeration it consumes can only produce one for malformed output — every transport failure still arrives as an empty server.

**Solution**: A discriminating sibling of `ListSessions` on `*tmux.Client` — the same read and the same parse, with a failed command returned as an error carrying tmux's own stderr — consumed by the search form alone, leaving the swallowing original exactly as it is for every existing caller.

**Outcome**: A `list-sessions` that fails makes `portal open /port` exit non-zero with tmux's words, painting no frame and attaching nothing, while an empty list from a live server is still zero matches and opens the picker; the picker, `portal list` and `ListSessionNames` keep the swallow they depend on.

**Do**:
- In `internal/tmux/tmux.go`, factor today's parse into `func parseSessionList(output string) ([]Session, error)` — the line split, the four-field `SplitN` with `@portal-dir` in the trailing slot, the two `Atoi`s and the `_`-prefix filter, moved verbatim — and hoist the `list-sessions -F …` argv into one shared value so both readers issue the identical format.
- Keep `ListSessions` contract-identical: a `Run` error still returns `([]Session{}, nil)` behind its existing "the error is the no-server signal" comment; everything else routes through `parseSessionList`.
- Add `func (c *Client) ListSessionsProbe() ([]Session, error)` beside it, documented as the discriminating variant of `ListSessions` in the shape `HasSessionProbe` (`internal/tmux/tmux.go:96`) already sets: the same read, with a `Run` error returned as `fmt.Errorf("failed to list tmux sessions: %w", err)` so the `*CommandError` the commander built — argv and captured stderr — survives `errors.As`.
- Re-point `SearchSessionSource`'s enumeration method from `ListSessions` to `ListSessionsProbe` (`cmd/open_search.go`), and update the test doubles that satisfy it; `*tmux.Client` satisfies the interface either way, so the production wiring is unchanged.
- Extend `internal/tmux/tmux_test.go` (or a sibling `list_sessions_probe_test.go`) with the probe's own cases and the re-pinned swallow, using `commandertest.Fails(&tmux.CommandError{Stderr: "no server running on /tmp/tmux-501/default", Err: …}, "list-sessions")`.

**Acceptance Criteria**:
- [ ] `ListSessionsProbe` returns the same sessions as `ListSessions` for identical output — window and attached counts, the `@portal-dir` column including an embedded `|`, and the `_`-prefix filter that keeps `_portal-saver` and `_portal-bootstrap` out
- [ ] A failed `list-sessions` returns a non-nil error from `ListSessionsProbe` that unwraps to `*tmux.CommandError` with tmux's stderr intact, and a nil session slice
- [ ] The same failure still returns `([]Session{}, nil)` from `ListSessions`, so the picker, `portal list` and `ListSessionNames` are unchanged
- [ ] Empty output from a live server is `([]Session{}, nil)` from both — never a failure
- [ ] A malformed line is an error from both, the same failure class as a failed read for the search form's purposes
- [ ] `portal open /term` over a failing read returns that error: no picker is opened, no session is connected, and the error is not a `*UsageError` (exit 1, printed by `main.classify`)
- [ ] `portal open /term` over a live server holding no sessions still opens the picker with a nil error
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it returns tmux's error when the session list cannot be read"` — probe; assert `errors.As` to `*tmux.CommandError` and that its `Stderr` is carried in the rendered message
- `"it still returns an empty slice and no error when ListSessions cannot read"` — the swallow, re-pinned
- `"it returns the same sessions as ListSessions for identical output"` — one table driving both readers
- `"it filters Portal's internal sessions"` — probe over output carrying `_portal-saver` and `_portal-bootstrap`
- `"it parses the recorded directory including an embedded pipe"` — probe
- `"it returns an empty slice and no error for a live server with no sessions"` — probe over empty output
- `"it reports a malformed line as an error"` — probe and `ListSessions`
- `"it reports a failed session-list read rather than opening the picker"` — `cmd`; an erroring source, asserting the error surfaces, `openTUIFunc` and `openSessionFunc` never run, and the error is not a `*UsageError`
- `"it still opens the picker for a live server with no sessions"` — `cmd`; empty slice, nil error

**Edge Cases**:
- A genuinely empty server is not a failure: the search form's count over it is zero, which opens the picker exactly as a term matching nothing does
- On the search path a failed read means the server went away after bootstrap ensured one, so tmux's own words are the honest report; the swallow stays everywhere else precisely because there the same failure is the no-server signal that yields an empty list
- The error carries the full `list-sessions` argv, format string included, because `CommandError` renders the argv it was handed — the stderr is the part that matters and it is preserved verbatim
- The two readers must keep issuing the identical format string: `@portal-dir` has to stay the last field, since a directory may contain a literal `|` and only the trailing `SplitN` slot survives one
- A parse failure and a transport failure are one class for this form — both mean the search never ran — while they stay distinguishable to any caller that recovers the `*CommandError`
- This task changes no exit code or message for any pre-existing command; the only new failure surface is the search form

**Context**:
> A session list that could not be read is not a zero match. K = 0 says the search ran and found nothing, which is a filter result and opens the picker; a failed read has searched nothing, and opening an empty picker on it would tell the user their sessions are gone when they are running. A tmux read failure is therefore reported in tmux's own terms and exits non-zero — the one failure path on this form, and it belongs to tmux rather than to the search.
>
> The usage errors of the composition rule are ordinary usage errors, carrying the same shape as `-f`'s own mutual-exclusion refusal. They are refusals of a malformed command line, not outcomes of a search — which is why this failure is not one of them and takes exit 1 rather than exit 2.
>
> `ListSessions`' swallow is not a defect to fix: for the picker and `portal list` the failed read is the no-server signal, and both render an empty list from it. The discriminating variant is additive for exactly that reason.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §3.2, §3.7
