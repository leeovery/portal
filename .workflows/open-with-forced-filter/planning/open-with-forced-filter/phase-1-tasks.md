---
phase: 1
phase_name: Sigil Recognition and the Pre-Filtered Picker
total: 7
---

# Phase 1: Sigil Recognition and the Pre-Filtered Picker — 7 tasks

## open-with-forced-filter-1-1

### Task 1-1: Keep the grouping-derived directory out of the session's recorded directory

**Problem**: `resolveSessionDirs` (`internal/tui/model.go:1155`) writes the pane-derived directory straight into `tmux.Session.Dir` — the field that holds the recorded `@portal-dir` stamp — and `cacheSessionDir` (`:1173`) freezes that guess into `m.sessions[i].Dir` for the rest of the picker session. A session therefore carries one directory value with two meanings. The moment the matched text (task 1-2) and, later, the directory column read the recorded directory, that conflation becomes a defect: an `s` regroup would make a session findable by a path it was not findable by a moment earlier, and would put a path beside a name that showed none. The two values must be carried separately, with the derived one read and written by grouping alone and never landing in the recorded one.

**Solution**: Keep `tmux.Session.Dir` recorded-only inside the TUI. Move the derived value into a model-side name-keyed map that only the grouped render paths consult, and have grouping read an "effective directory" (recorded, else derived) for group membership while the `SessionItem` it emits still carries the untouched recorded value.

**Outcome**: After a By-Project or By-Tag rebuild resolves an unstamped session's directory, the session groups under its project or tags exactly as it does today, `m.sessions[i].Dir` is still empty, and a `SessionsMsg` refresh discards the derived value; Flat and the zero-tags signpost still issue zero pane reads.

**Do**:
- Add `derivedDirs map[string]string` to `Model` (`internal/tui/model.go`), keyed on session name — the grouping-only companion to the recorded `Session.Dir`.
- Replace `resolveSessionDirs(sessions []tmux.Session) []tmux.Session` with `resolveDerivedDirs(sessions []tmux.Session) map[string]string`: for each session whose `Dir == ""`, reuse an existing `m.derivedDirs` entry when present, otherwise call `session.ResolveSessionDir(s.Name, m.dirReader, m.dirRunner)` and store a successful non-empty result in `m.derivedDirs`; return `m.derivedDirs`. Keep the nil-seam early return (both seams nil → return with nothing resolved) and keep storing nothing for an unresolvable session. Delete `cacheSessionDir`; no code path writes `m.sessions[i].Dir` any more.
- Add `func effectiveDir(s tmux.Session, derived map[string]string) string` to `internal/tui/grouping.go`: `s.Dir` when non-empty, else `derived[s.Name]`.
- Thread the map through the grouped builders: `buildByProject(sessions []tmux.Session, idx project.Index, derived map[string]string)` and `buildByTag(...)` (and `resolveSessionTags(s, idx, derived)`) resolve their lookup key via `effectiveDir` instead of reading `s.Dir` directly, and keep embedding the unmodified `s` in every `SessionItem` they emit.
- In `rebuildSessionList`, pass `m.resolveDerivedDirs(filtered)` into the two grouped arms only; the Flat and `byTagSignpost` arms call neither it nor the builders.
- Clear `m.derivedDirs` in `applySessions` before the rebuild, so a refreshed session list re-derives rather than inheriting a stale guess.
- Re-point the two existing assertions that read the old conflated field: `internal/tui/rebuild_dir_resolution_test.go`'s `"caches the derived directory into m.sessions and never stamps tmux"` (assert `m.sessions[0].Dir == ""` and `m.derivedDirs["portal-abc"] == key`) and `"it treats an empty current path as unresolved in the grouped render"` (assert the name is absent from `m.derivedDirs`).

**Acceptance Criteria**:
- [ ] No production code in `internal/tui` assigns to a `tmux.Session`'s `Dir` field; the derived value lives only in `m.derivedDirs`
- [ ] A By-Project rebuild over a session with an empty recorded directory groups it under its project (not the `Unknown` catch-all) while `m.sessions[0].Dir` stays `""`
- [ ] A By-Tag rebuild over the same session groups it under each of its project's tags (not `Untagged`), reading the derived value through the same helper
- [ ] Every `SessionItem` emitted by `buildByProject` / `buildByTag` carries the session's recorded directory verbatim — empty stays empty
- [ ] A second rebuild in the same picker session issues no further pane read (the derived map is the cache)
- [ ] `applySessions` clears the derived map, so a refresh re-issues the pane read on the next grouped rebuild
- [ ] Flat mode and the zero-tags signpost issue zero pane reads and zero stamp writes over unstamped sessions
- [ ] An unresolvable session (pane read returning `("", nil)`, an error, or nil seams) stores nothing, keeps both values empty and routes to the catch-all
- [ ] `go test ./...` passes

**Tests**:
- `"it groups an unstamped session under its project without writing the recorded directory"` — By Project; assert `GroupKey`/`GroupHeading` and `m.sessions[0].Dir == ""`
- `"it groups an unstamped session under each of its tags from the derived value"` — By Tag, two tags, two rows, neither a catch-all
- `"it leaves the item's recorded directory empty when the group came from a derived value"` — read `SessionItem.Session.Dir` off the built row
- `"it reuses the derived value on a second rebuild and issues no further pane read"`
- `"it discards derived values when the session list is refreshed"` — `applySessions` twice, asserting a second pane read is issued
- `"it issues no pane read in Flat mode"` and `"it issues no pane read under the zero-tags signpost"`
- `"it stores nothing for a session whose pane read returns an empty path"` — the `("", nil)` shape, routed to `Unknown`
- `"it does not panic with nil dir seams and routes the session to the catch-all"`
- `"it never stamps tmux with a derived directory"` — zero `SetSessionOption` calls

**Edge Cases**:
- Flat and the `byTagSignpost` arm must stay off the resolution path entirely — the pass is called from the grouped arms only, so a pane read there is a regression
- A `SessionsMsg` refresh replaces `m.sessions`; the derived map must be cleared in the same breath or the guess outlives the list it was derived for
- An unresolvable session is not negative-cached: nothing is stored, both values stay empty, and the next grouped rebuild tries again — exactly today's behaviour
- By Tag reads the effective directory too, through `resolveSessionTags`; leaving it on the raw recorded value would silently move every legacy session to `Untagged`
- The derived value is canonicalised by `session.ResolveSessionDir` while the recorded value is the raw `@portal-dir` string; they are not interchangeable, which is the second reason they cannot share a field

**Context**:
> A session carries the two as separate values — the recorded directory, which may be absent, and the derived one, which grouping alone reads and writes. A derived value never lands in the recorded one. Neither the match nor the directory column ever reads it, so a regroup can never make a session findable by a path it was not findable by a moment earlier, and can never put a path beside a name that showed none.
>
> The derived value is still never stamped back to tmux: a pane's cwd can drift from its origin dir, and freezing that drift would permanently mis-group the session.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §4.1

## open-with-forced-filter-1-2

### Task 1-2: Match sessions on their home-abbreviated recorded directory as well as their name

**Problem**: All three of the picker's filter entry points — `-f/--filter`, a `/` typed by hand, and (from task 1-5) the search form — narrow the sessions list through `SessionItem.FilterValue()`, which returns the session name alone (`internal/tui/session_item.go:80`). A term aimed at a directory therefore matches nothing, which is the whole complaint the feature answers: the user cannot recognise sessions by name, so the search has to reach the place the session was opened in. The matched text must widen to the session's recorded directory, and it must be the *displayed* form of that directory — matching the raw `/Users/<account>/…` path would make every session on the machine answer to a term hitting the home prefix while every returned row showed no such text.

**Solution**: Add the inverse of `resolver.ExpandTilde` — a pure `AbbreviateHome` that rewrites a path under the user's home to `~/…` and leaves every other path alone — and return the session name plus one space plus that abbreviated recorded directory from `SessionItem.FilterValue()`, name alone when the session carries no recorded directory.

**Outcome**: `portal open -f portal` and a `/portal` typed inside the picker both return a session named `api-work` that lives in `~/Code/portal`, and a term hitting the home prefix (`lee`, `Users`) matches nothing that does not literally display those characters; a session with no recorded directory still matches on its name, with no trailing separator in the matched text.

**Do**:
- Add `func AbbreviateHome(path string) string` to `internal/resolver/path.go`, beside `ExpandTilde`: resolve the home via `os.UserHomeDir()` and return `path` unchanged on error or on an empty home; return `"~"` when `path == home`; return `"~/" + path[len(home)+1:]` when `path` has the prefix `home + "/"`; otherwise return `path` unchanged. It is a pure string test — no `filepath.EvalSymlinks`, no `os.Stat`, no filesystem access of any kind.
- Change `SessionItem.FilterValue()` (`internal/tui/session_item.go`) to return `i.Session.Name` when `i.Session.Dir == ""`, else `i.Session.Name + " " + resolver.AbbreviateHome(i.Session.Dir)`.
- Leave `HeaderItem.FilterValue()` returning `""`, and leave `ToListItems`, `buildByProject`, `buildByTag` and the delegate untouched — the value is derived from the item's own session, so every construction site (including a hand-built item in a test or fixture) gets it with no stamping.
- Update the two existing subtests in `internal/tui/session_item_test.go` that assert `FilterValue()` equals the session name (`"FilterValue returns session name"`, `"returns the session name from FilterValue regardless of group fields"`) to cover both shapes — with and without a recorded directory.

**Acceptance Criteria**:
- [ ] `AbbreviateHome` abbreviates a path under the home directory to `~/<rest>`, returns `~` for the home directory itself, and returns any other path byte-identical — including a path whose prefix merely looks like the home directory (`/Users/leeoveryX/Code` under home `/Users/leeovery`)
- [ ] `AbbreviateHome` returns its input unchanged when the home directory cannot be resolved, and touches the filesystem on no path
- [ ] `SessionItem.FilterValue()` for a session with a recorded directory is `"<name> <home-abbreviated dir>"`; for one without, it is the name alone with no trailing space
- [ ] `-f <directory fragment>` narrows the sessions list to the sessions whose recorded directory carries that fragment
- [ ] A `/` filter typed by hand inside the picker narrows on the same widened text
- [ ] A term matching the user's account name or home prefix matches no session on the strength of a path the row does not display
- [ ] `HeaderItem.FilterValue()` is still `""`, so headings still vanish the moment a query is typed
- [ ] A session whose directory is known only to grouping (recorded empty, derived present) is not matchable by that directory
- [ ] `go test ./...` passes

**Tests**:
- `"it abbreviates a path under the home directory"` — `<home>/Code/portal` → `~/Code/portal`
- `"it abbreviates the home directory itself to a bare tilde"`
- `"it leaves a home-prefix lookalike path unchanged"` — home `/Users/leeovery`, path `/Users/leeoveryX/Code`
- `"it leaves an unrelated absolute path unchanged"` — `/opt/tools`
- `"it returns the path unchanged when the home directory cannot be resolved"` — `t.Setenv("HOME", "")`
- `"it returns the session name alone when the session carries no recorded directory"` — no trailing separator
- `"it returns the name, one space and the abbreviated directory"`
- `"it narrows -f to a session found by its directory"` — model-level: `WithInitialFilter("portal")` over a session named `api-work` with `Dir: <home>/Code/portal`
- `"it narrows a hand-typed filter to a session found by its directory"` — drive `/` plus the term through `Update`
- `"it does not match a session on the user's account name"` — a term equal to the home directory's last segment returns nothing
- `"it does not match a session on a grouping-derived directory"` — recorded empty, derived populated by a grouped rebuild, filter on the derived path returns nothing
- `"it keeps a group heading's filter value empty"`

**Edge Cases**:
- A recorded directory exactly equal to the home directory renders `~` rather than `~/`
- A recorded directory carrying a trailing separator (`<home>/`) abbreviates to `~/` — acceptable, since the value is whatever was stamped and nothing normalises it
- Home lookup failure degrades to the raw path in the matched text rather than failing the filter
- The widened text reaches `-f` and the hand-typed filter in every picker, including pickers this feature does not open; the visible effect is rows appearing, never rows the user expected going missing
- A session created before the directory stamp shipped matches on name alone, unconditionally, in every view

**Context**:
> The searched form is the displayed form. A recorded directory under the user's home is searched home-abbreviated (`~/Code/portal`), not as tmux recorded it — otherwise a term hitting the home prefix would match every session the user has while every returned row displayed no such text, and on a lone survivor would attach outright with nothing on screen accounting for the choice.
>
> Widening the matched fields widens them for all three filter entry points deliberately: the same list narrowing on different *fields* depending on how the user arrived at it is harder to predict than one wider rule. What diverges between the entry points is the rule (containment versus the picker's fuzzy), not the fields — and that divergence arrives in a later phase.
>
> On the fuzzy routes the two fields are joined, as the picker's stock matcher expects, and the joined text is exactly the session name, one space, then the recorded directory in its home-abbreviated form — the two values the row is built from, so what the matcher scores and what the eye reads are the same text. A session carrying no recorded directory joins to its name alone, with no trailing separator. The row's count and attached slots are no part of the matched text, and the join is taken before the row is laid out, so width-driven truncation never narrows what is matched.
>
> `internal/tui` already reads `$HOME` transitively on every grouped rebuild (`project.CanonicalDirKey` → `resolver.ExpandTilde`), so deriving the abbreviation inside `FilterValue` introduces no new kind of dependency for that package.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §4.1, §4.2, §4.3

## open-with-forced-filter-1-3

### Task 1-3: Land a search-form term as a committed sessions filter in the picker

**Problem**: `-f`'s landing sends its text to whichever page `evaluateDefaultPage` chose (`internal/tui/model.go:1238-1273`), and that choice falls through to the Projects page whenever the session list is empty. A search form is session-domain by declaration: a term matching nothing, and a machine holding no live sessions at all, must still land on the Sessions page with the term committed as the sessions filter — landing on the mint page would have the form contradict itself. The model has no way to express "this filter is a session search" today, so the page pin and the term cannot be requested.

**Solution**: A search-form landing on the model — a nil-tolerant `Deps.Search` applied through a `WithSearchForm(term)` option — that pins `activePage` to Sessions in `evaluateDefaultPage` and applies the term to the session list as filter text in the committed (`FilterApplied`) state, with the cursor re-anchored onto the first surviving session row.

**Outcome**: A model built with a search-form term lands on the Sessions page in every grouping mode, with the filter text set and committed rather than focused, the cursor on the first surviving session row, and arrows / `Space` / `Enter` working immediately on the narrowed list — including when nothing survives the term and when no sessions are live at all; `-f`'s own landing and the command-pending Projects redirect are byte-for-byte unchanged.

**Do**:
- Add `type SearchForm struct { Term string }` and `Search *SearchForm` to `Deps` (`internal/tui/build.go`), documented as: nil means the picker was not reached by a search form. In `Build`, `if deps.Search != nil { opts = append(opts, WithSearchForm(deps.Search.Term)) }`.
- Add `searchForm bool` and `searchTerm string` to `Model`, and `func WithSearchForm(term string) Option` setting both. Do not clear them after the landing — they describe the invocation, not a pending action.
- In `evaluateDefaultPage`, pin `m.activePage = PageSessions` when `m.searchForm`, ahead of the existing item-count branch; leave the `commandPending` branch and the non-search branches exactly as they are.
- Add `applySearchLanding()` called from `evaluateDefaultPage` in `applyInitialFilter`'s slot (a search form never carries `InitialFilter`, so exactly one of the two applies): for a non-empty `m.searchTerm`, `m.sessionList.SetFilterText(m.searchTerm)`, `m.sessionList.SetFilterState(list.FilterApplied)`, then `m.ensureSessionRowSelected()`.
- Leave `applyInitialFilter` and `applyInitialCursor` untouched.

**Acceptance Criteria**:
- [ ] A model built with `Deps.Search = &SearchForm{Term: "…"}` lands on `PageSessions` with `FilterState() == list.FilterApplied` and `FilterValue()` equal to the term
- [ ] The visible set is the sessions matching the term and the cursor is on a `SessionItem`, never a `HeaderItem`, in Flat, By Project and By Tag
- [ ] With zero live sessions the page is still Sessions, the term is on the session list, and the project list's filter is empty
- [ ] With live sessions but nothing surviving the term the page is still Sessions and the no-matches state renders; the project list's filter is empty
- [ ] `Deps.Search == nil` changes nothing: the `-f` landing, the no-filter launch and the command-pending Projects redirect behave exactly as before (existing suites green unmodified)
- [ ] A search form never routes its term to the project list, on any page-choice path
- [ ] `go test ./...` passes

**Tests**:
- `"it lands a search term on the Sessions page with the filter committed"` — assert page, `FilterApplied`, filter value, visible set
- `"it puts the cursor on the first surviving session row"` — Flat
- `"it puts the cursor on a session row rather than a group header in By Project"` and `"… in By Tag"`
- `"it pins the Sessions page when no sessions are live"` — empty `SessionsMsg`; assert page and that the project filter is empty
- `"it pins the Sessions page when nothing survives the term"` — assert the no-matches state and an empty project filter
- `"it leaves the -f landing unchanged"` — the existing `-f` expectations re-asserted with `Search` nil
- `"it leaves the command-pending Projects redirect unchanged"` — `WithCommand` plus `InitialFilter`, Projects page, project filter set
- `"it applies no filter for an empty search term"` — the term-less shape reaches the landing inert at this task's boundary

**Edge Cases**:
- Zero live sessions: `ensureSessionRowSelected` runs against an empty list — `SelectedItem()` returns nil and the step must be a no-op, not a panic
- Nothing survives the term: `VisibleItems()` is empty and the page must stay Sessions, so the no-matches body (which requires a non-empty query and a filtered state) is what the user sees rather than the empty-sessions body
- Under a committed non-empty filter the group headings drop out (a header's filter value is empty), so the first visible row is already a session row; the cursor step stays as the guard for the empty-query case
- `evaluateDefaultPage` is latched by `defaultPageEvaluated`, so the landing applies exactly once even though the cold route evaluates it after a post-restore session refetch
- A search form and a pending command cannot co-occur — a line carrying both is refused before the picker is built (task 1-7) — so the two landings never contend

**Context**:
> Whenever a term opens the picker it lands exactly as `-f` already lands it: filter text set and filter state committed rather than focused, cursor re-anchored onto the post-filter visible set, so arrows, `Space` and `Enter` work immediately on the narrowed list. The user is not left inside a live filter input.
>
> A count of zero is a filter result rather than an error: it is indistinguishable from typing `/` inside the picker and filtering to nothing, and `Esc` clears the filter to reveal the session list. The picker reopens in whichever grouping mode the user last left it in, so the narrowed list lands in Flat, By Project or By Tag depending on that history — the landing must be identical in all three.
>
> In this phase the narrowed set is still the picker's own fuzzy set; the stricter containment rule that decides a match count arrives in the next phase. This task delivers the landing, not the rule.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §3.2, §3.3, §5.2

## open-with-forced-filter-1-4

### Task 1-4: Take the search-form shape out of the resolution chain

**Problem**: A leading `/` makes an argument a path unconditionally (`internal/resolver/path.go:14` → `strings.Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'`), so `/port` is resolved as a directory and fails with `Directory not found: /port`. The shape has to stop being a path before anything can read it as session-search text, and the narrowing must be exact: every other path argument — a multi-segment absolute path, a single segment with a trailing slash, `./port`, `~/port` — has to keep resolving exactly as it does today, and the test must never consult the filesystem, or the same command would be read differently on two machines.

**Solution**: One recogniser beside `IsPathArgument` — an argument beginning with `/` and containing no further `/` — plus the term extractor, with `IsPathArgument` narrowed to return false for that one shape.

**Outcome**: `resolver.IsSearchSigil` accepts `/port` and `/` and rejects every other argument shape; `IsPathArgument("/port")` is false, so the single-target chain no longer routes it to the path domain and no longer reports a missing directory for it; every other path argument's classification and resolution are unchanged.

**Do**:
- Add to `internal/resolver/path.go`: `func IsSearchSigil(arg string) bool` — true iff `strings.HasPrefix(arg, "/") && strings.Count(arg, "/") == 1` — and `func SearchTerm(arg string) string` returning everything after the leading `/` (empty for a bare `/`).
- Narrow `IsPathArgument` with a leading `if IsSearchSigil(arg) { return false }`, leaving the rest of its rule byte-identical.
- Extend `internal/resolver/path_test.go`'s `TestIsPathArgument` table with the shapes from the recognition table: `/port` (false), `/Users/leeovery/Code/portal` (true), `/tmp/` (true), `/` (false), `./port` (true), `~/port` (true), `port` (false).
- Pin the chain's behaviour in `internal/resolver`: `Resolve("/port")` with no live session of that name walks past the path domain to alias, then zoxide, then a `*MissResult` — no `*PathResult`, no `Directory not found` error.
- Leave every pin untouched: `ResolvePathPin` stats the literal path (so `-p /tmp` still mints), and `-s`, `-a` and `-z` never consulted `IsPathArgument`.
- Re-point `cmd/bootstrap_warnings_test.go:269`, whose fixture argument `/nonexistent-path-for-test` is a single-segment absolute path: use a multi-segment path (e.g. `/nonexistent/path-for-test`) so the test keeps exercising the positional-path shape it is about.

**Acceptance Criteria**:
- [ ] `IsSearchSigil` is true for `/port` and for a bare `/`, and false for `/Users/leeovery/Code/portal`, `/tmp/`, `./port`, `~/port`, `port` and the empty string
- [ ] `SearchTerm("/port") == "port"` and `SearchTerm("/") == ""`
- [ ] `IsPathArgument` is false for the sigil shape and unchanged for every other argument, including `/tmp/` (the trailing slash is the second one) and `/Users/leeovery/Code/portal`
- [ ] Neither function stats, expands or otherwise touches the filesystem — the verdict is identical on a machine where `/port` exists and one where it does not
- [ ] `QueryResolver.Resolve("/port")` returns a `*MissResult` when nothing else claims it, rather than a path result or a directory-not-found error
- [ ] `QueryResolver.Resolve("/Users/…/portal")` still returns a `*PathResult`, and `ResolvePathPin("/tmp")` still mints
- [ ] `go test ./...` passes

**Tests**:
- `"it recognises a single-segment leading-slash argument as a search form"` — `/port`
- `"it recognises a bare slash as a search form"`
- `"it rejects a multi-segment absolute path"` — `/Users/leeovery/Code/portal`
- `"it rejects a single segment carrying a trailing slash"` — `/tmp/`
- `"it rejects ./port, ~/port and a bare word"`
- `"it rejects the empty string"`
- `"it returns the term after the slash"` and `"it returns an empty term for a bare slash"`
- `"it no longer classifies the search-form shape as a path argument"` — the extended table
- `"it does not route the search-form shape to the path domain"` — `Resolve("/port")` → `*MissResult`, no error
- `"it returns the same verdict for an existing and a non-existing single-segment directory"` — `/tmp` (exists) and `/definitely-not-here`, both sigils
- `"it still resolves a multi-segment absolute path to the path domain"`
- `"it still mints for -p on a single-segment directory"` — `ResolvePathPin("/tmp")`

**Edge Cases**:
- `/tmp/` stays a path: the trailing slash is the second one, and it is always a slash the user typed (Portal's completion never adds one)
- A bare `/` is a search form, not a mint at the filesystem root
- Recognition never consults the live session set: the predicate takes no session list, so a live session named literally `/port` does not change the shape's reading. The chain's own exact-session branch still precedes the path test, so a caller that hands `/port` to `Resolve` while such a session lives still gets a session result — but `open` stops handing the shape over at all (task 1-5), which is what makes `/port` a search on every command line
- `-p`, `-s`, `-a` and `-z` values are flag values rather than positionals and are untouched by this narrowing
- The single-segment absolute directories the rule takes out of the minting domain — `/tmp`, `/opt`, `/srv` — are reachable with `-p`, which is the deliberate escape

**Context**:
> A positional argument is a sigil when it begins with `/` and contains no further `/`. That narrows the path test for one shape and leaves every other path argument exactly as it is. The rule stays a test of the argument's shape and never consults the filesystem — a rule that minted when the single-segment path happened to exist would read the same command differently on two machines.
>
> The accepted cost is that single-segment absolute directories typed without a trailing slash stop minting and start filtering, which is affordable because minting a session directly in a root-level directory is not something the user does. The escape is the pin that exists for exactly this: `portal open -p /tmp`.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §2.2, §2.4, §2.5, §3.1

## open-with-forced-filter-1-5

### Task 1-5: Route `portal open /term` to the search-form landing

**Problem**: `open`'s `RunE` (`cmd/open.go:140-206`) has no branch for the shape. A lone `/port` positional falls through the multi-target gate — where a term carrying glob metacharacters (`/po*`) is treated as a K≥2-expandable target and bursts (`cmd/open_burst.go:35-44`) — and then into the resolution chain, which after task 1-4 walks it to a miss and hard-fails with `nothing resolved for '/port'`. The term must bypass resolution entirely and open the picker pre-filtered, with no decision line in the `resolve` log component, because a search form declares its domain the way a pin does rather than guessing it.

**Solution**: A search-form branch at the front of `RunE`, driven by a positional scan that stops at a `--` separator, handing the term to the picker through a landing value that replaces `openTUI`'s bare filter-string parameter — so one seam carries all three ways the picker is opened.

**Outcome**: `portal open /port` (and `x /port`) opens the picker on the sessions list pre-filtered by `port`, having built no query resolver, resolved nothing, expanded no glob, dispatched no burst and emitted no `resolve` line; `-f`, the no-argument picker, every domain pin and the multi-target burst are untouched.

**Do**:
- Add `cmd/open_search.go` with `func searchFormPositionals(cmd *cobra.Command, args []string) []string`: take `args` up to `cmd.ArgsLenAtDash()` when that is `>= 0` and all of `args` otherwise, and return the values satisfying `resolver.IsSearchSigil` in argv order.
- Add `type pickerLanding struct { filter string; search bool }` (same file) and replace `openTUIFunc`/`openTUI`'s `initialFilter string` parameter with it, threading it into `buildTUIModel(cfg tuiConfig, landing pickerLanding, command []string)`, which sets `Deps.InitialFilter = landing.filter` for a non-search landing and `Deps.Search = &tui.SearchForm{Term: landing.filter}` for a search one.
- Update the existing call sites to the new shape: the `-f` branch passes `pickerLanding{filter: filterVal}`, the no-target branch `pickerLanding{}`. The seam's parameter change is mechanical across its test closures — `cmd/open_test.go`, `cmd/open_multitarget_test.go`, `cmd/bare_root_test.go`, `cmd/abridged_route_test.go`, `cmd/abridged_integration_test.go`, `cmd/reattach_integration_test.go` (including its `var _ func(*cobra.Command, string, []string, bool) error = openTUIFunc` type assertion), `cmd/concurrent_bootstrap_gate_test.go`, `cmd/concurrent_bootstrap_route_test.go`, `cmd/concurrent_coldboot_integration_test.go`, `cmd/bootstrap_warnings_test.go`, `cmd/seam_staging_test.go`, plus the `buildTUIModel` callers in `cmd/open_theme_construction_test.go`, `cmd/open_theme_commit_test.go`, `cmd/open_initial_mode_test.go`, `cmd/open_nocolor_test.go`, `cmd/open_spawn_detect_test.go` and `cmd/theme_source_test.go`.
- In `RunE`, immediately after the `--ack` shape validation (which must stay ahead of every branch that touches tmux) and before the `filter` block: `if forms := searchFormPositionals(cmd, args); len(forms) > 0 { return openTUIFunc(cmd, pickerLanding{filter: resolver.SearchTerm(forms[0]), search: true}, nil, serverWasStarted(cmd)) }`.
- Do not call `emitResolveDecision`, `buildQueryResolver`, `orderedOpenTargets` or `writeAckMarker` on this path, and change neither `isTUIPath` nor `shouldRunConcurrentBootstrap` (`cmd/root.go:168-186`).

**Acceptance Criteria**:
- [ ] `portal open /port` reaches the picker with the landing `{filter: "port", search: true}` and no other branch of `RunE` runs
- [ ] No query resolver is constructed and no resolution is attempted on this path — an injected session lister / alias lookup / zoxide querier records zero calls
- [ ] `portal open '/po*rt'` opens the picker on the literal term `po*rt`: no glob expansion, no burst dispatch, no window spawned
- [ ] A term equal to a live session's name still opens the picker rather than attaching
- [ ] No record is emitted under the `resolve` component for a search-form invocation
- [ ] `searchFormPositionals` finds the shape wherever it sits among the positionals, and never inspects a word after a `--` separator
- [ ] `-f <text>`, `portal open` with no arguments, all four domain pins, the bare-positional chain and the multi-target burst behave exactly as before
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it opens the picker pre-filtered by the term after the slash"` — stage `openTUIFunc`, assert the landing
- `"it resolves nothing for a search form"` — injected `OpenDeps` resolver seams record zero calls
- `"it treats glob metacharacters in the term as literal text"` — `/po*rt`: the landing carries `po*rt`, `runOpenBurstFunc` is never called
- `"it opens the picker for a term that equals a live session name"` — no connector call
- `"it emits no resolve component line"` — `logtest.Install`, assert no record for `resolve`/`resolved`
- `"it finds the search form at any positional index"` — `searchFormPositionals` unit table
- `"it never inspects words after a -- separator"` — `searchFormPositionals` over an argv whose post-dash words include `/tmp`
- `"it leaves -f routing unchanged"` and `"it leaves the no-argument picker launch unchanged"`
- `"it leaves a multi-segment path positional minting"` — regression pin on the untouched bare chain

**Edge Cases**:
- A term carrying `*`, `?` or `[` must not reach `isMultiTarget`, which would treat the single positional as glob-expandable and burst it — hence the branch sits ahead of the multi-target gate
- A bare `/` routes here with an empty term; its landing behaviour is task 1-6's, and until that lands the empty term applies no filter
- The `--ack` malformed-value check stays ahead of this branch, since this branch opens the picker and therefore touches tmux
- A line carrying anything besides the search form is refused before `RunE` runs (task 1-7), so this branch never has to decide what to do with a second target — its own tests use well-formed lines and exercise positional-independence through the scan helper
- Cold-boot classification and warning routing are deliberately out of scope: this task must not touch `isTUIPath`, so a cold-boot search form still takes the synchronous bootstrap in this phase

**Context**:
> A sigil argument never enters the bare-positional resolution chain. It reaches neither the path, alias nor zoxide domain, and it can never mint a session. Its term is a search over live sessions and nothing else.
>
> The term is literal text: `*`, `?` and `[` are characters to find rather than wildcards. The glob forms answer ambiguity by opening a window per match; the search form answers it by narrowing, and the two rules are not mixed.
>
> The search form emits no `resolve` component line. That component records one INFO line per bare positional resolved through the guessing chain; deterministic targets — globs and domain pins — emit none, and a search form declares its domain in the same way a pin does.
>
> The command payload is not searched for the shape. Recognition applies to the arguments `open` parses as targets and stops at a `--` separator: the words after it are the trailing command's own, so a `/word` among them is that command's argument and never a search form.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §2.3, §3.1, §3.2, §3.6, §4.3

## open-with-forced-filter-1-6

### Task 1-6: Open an empty focused filter for the term-less search form

**Problem**: `portal open /` — the search form with no term — must open the picker with the filter open, empty, focused and the cursor in it, ready to type: it is `x` followed by `/`, reached from the shell. After task 1-5 it routes to the picker with an empty term, and after task 1-3 that lands inert: the list stays unfiltered with no input open, so the user still has to press `/` themselves. Flipping the list into the filtering state naively is worse than inert — `bubbles/list` serves `VisibleItems()` from its filtered slice whenever the state is not `Unfiltered`, so a bare state flip with no filter pass behind it would show an empty list where every live session should be.

**Solution**: In the search landing, set the session-list filter text to the empty string first (which runs the filter pass and admits every item) and then flip the state to `Filtering`, which focuses the input; step the cursor off a group-header row afterwards.

**Outcome**: `portal open /` lands on the Sessions page with the filter input focused and empty, every live session visible beneath it, the cursor on a session row, the first typed character narrowing through the picker's own rule — and it is not an error, while `-f ""` keeps its refusal.

**Do**:
- Extend `applySearchLanding()` (`internal/tui/model.go`): when the search form carries an empty term, call `m.sessionList.SetFilterText("")`, then `m.sessionList.SetFilterState(list.Filtering)`, then `m.ensureSessionRowSelected()`. Add one comment on the ordering — the text call is what populates the filtered set, so the state flip must not come first.
- Add nothing to the render layer: the focused-filter footer, the untouched section-header row and the `SettingFilter` key guard already cover the `Filtering` state.
- Leave the `-f` empty-value refusal (`cmd/open.go:161`) exactly as it is.

**Acceptance Criteria**:
- [ ] A model built with `Deps.Search = &SearchForm{Term: ""}` lands on `PageSessions` with `FilterState() == list.Filtering` and `FilterValue() == ""`
- [ ] Every live session is present in `VisibleItems()` under that empty focused filter — in Flat, By Project and By Tag
- [ ] The cursor is on a `SessionItem`, never a `HeaderItem`, in the grouped modes where headings are still present under the empty query
- [ ] `portal open /` exits through the picker rather than returning a usage error; `-f ""` is still refused
- [ ] The first typed character narrows the list, and `s` typed while the input is focused is a literal filter character rather than a grouping switch
- [ ] With zero live sessions the landing does not panic: the page is Sessions, the filter is focused and empty, and the body carries no rows
- [ ] `go test ./...` passes

**Tests**:
- `"it lands the term-less form on a focused empty sessions filter"` — page, `Filtering`, empty filter value
- `"it keeps every live session visible under the empty focused filter"` — the regression that a bare state flip would break
- `"it keeps every session visible in By Project and By Tag"`
- `"it puts the cursor on a session row rather than a group header"` — grouped mode with headings present
- `"it narrows on the first typed character"` — drive a rune through `Update`
- `"it treats s as a filter character while the input is focused"`
- `"it does not panic with zero live sessions"`
- `"it is not a usage error"` — `portal open /` through the command, asserting the picker seam ran and no error returned
- `"it still refuses an empty -f value"`

**Edge Cases**:
- The ordering is load-bearing: `SetFilterState(Filtering)` alone leaves the filtered slice nil, and `VisibleItems()` then returns nothing while the list holds every session
- Grouped modes keep their heading rows under an empty query (a header is only dropped once a non-empty query is typed), so the cursor step is required rather than defensive here
- `-f`'s refusal of an empty value does not transfer: `-f ""` is a flag given no argument, while `/` on its own is the gesture that opens a filter
- Zero live sessions: neither the empty-sessions body (which requires the `Unfiltered` state) nor the no-matches body (which requires a non-empty query) claims the frame, so the list body renders with no rows. The specification settles the landing and says nothing about copy for this case, so this task introduces none

**Context**:
> `x /` — the sigil with no term — opens the picker with the filter open and empty, the cursor in it, ready to type. It is `x` followed by `/`, not plain `x`, and not an error. It does not mean "mint at root". A term-less search form takes no match count: it carries nothing to match, so it opens on the whole live session list, on a machine holding one live session as on a machine holding twenty.
>
> The term-less form supplies nothing for the containment rule to hold on to — the first character typed there is already a value the search form did not supply, so the picker's own rule applies from that keystroke on.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §2.5, §3.2, §3.3, §4.4

## open-with-forced-filter-1-7

### Task 1-7: Refuse a search form that shares its command line

**Problem**: The search form is a whole-invocation mode, the way `-f` is: any other target, a trailing command, `-f`, any domain pin and the hidden `--ack` are each a malformed line rather than a composition. That refusal has to be decided from the arguments alone and start nothing — which rules out `RunE`, because cobra runs `PersistentPreRunE` first, so by the time `RunE` could refuse the line the tmux server has been started, hooks registered and every saved session restored. Today such a line is not refused at all: `portal open /term extra` would route to the picker on `/term` and silently drop `extra`, and `portal open /term -- ls` would open the picker and discard the command.

**Solution**: An `Args` validator on `openCmd` — cobra validates args before any `PersistentPreRunE` and answers `--help` before either — that refuses a line carrying a search form beside anything else, with one `*UsageError` naming the element that collided.

**Outcome**: Every composition in the refusal table exits 2 with a message naming the search form and the offending element, having started no tmux server, restored nothing and painted no frame; `portal open /term --help` still prints help, root persistent flags still apply, words after `--` are never inspected for the shape, and every other `open` line validates exactly as before.

**Do**:
- Add `func validateOpenArgs(cmd *cobra.Command, args []string) error` to `cmd/open_search.go`: return nil when `searchFormPositionals(cmd, args)` is empty, otherwise return the first collision in this fixed order, so the message is deterministic:
  1. a second search form among the pre-`--` positionals → `cannot use a /term search with another /term search`
  2. any other pre-`--` positional → `cannot use a /term search with another target`
  3. `cmd.ArgsLenAtDash() >= 0` or `cmd.Flags().Changed("exec")` → `cannot use a /term search with a command (-e/--)`
  4. `cmd.Flags().Changed("filter")` → `cannot use a /term search with -f/--filter`
  5. `anyOpenDomainPin(cmd)` → `cannot use a /term search with a domain pin (-s/-p/-z/-a)`
  6. `cmd.Flags().Changed("ack")` → `cannot use a /term search with --ack`
  Each is a `NewUsageError`, so `main.classify` maps it to exit 2.
- Set `Args: validateOpenArgs` on `openCmd` (`cmd/open.go:139`) in place of `cobra.ArbitraryArgs`; every line carrying no search form returns nil, so arity is otherwise unconstrained.
- Scan only `args[:cmd.ArgsLenAtDash()]` when a dash separator is present — the trailing command's own words are never inspected.
- Keep `RunE`'s existing usage errors (`--ack` shape, the `-f` exclusions, `parseCommandArgs`) exactly as they are.

**Acceptance Criteria**:
- [ ] Each of these exits 2 with a message naming the search form and the collided element: `/term <other-target>` (at every arity), `<other-target> /term`, `/term /other`, `/term -e <cmd>`, `/term -- <cmd>`, `/term -f <text>`, `/term -s|-p|-a|-z <value>`, `/term --ack <batch>:<token>`
- [ ] `portal open /term --` with nothing after the separator is refused as a command collision rather than reaching `RunE`'s "no command specified after --"
- [ ] A refused line runs no bootstrap: an injected recording orchestrator records zero `Run` calls, and the picker seam is never invoked
- [ ] `portal open /term --help` prints help and exits 0; a root persistent flag on a search-form line still applies
- [ ] `portal open ~/Code/api -- ls /tmp` is unaffected — the post-`--` `/tmp` is the command's own argument and no refusal fires
- [ ] `portal open /term` alone and `portal open /` alone validate (nil) and reach the picker
- [ ] Every pre-existing `open` line still validates, including two or more positional targets for the multi-target burst
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it refuses a search form beside another target"` — table over `/term x`, `x /term`, `/term x y`, asserting exit-code-2 error type and the named element
- `"it refuses a second search form"`
- `"it refuses a search form with -e"` and `"… with a -- command"`
- `"it refuses a search form with an empty -- separator"` — `/term --`
- `"it refuses a search form with -f"`
- `"it refuses a search form with each domain pin"` — table over `-s`, `-p`, `-a`, `-z`
- `"it refuses a search form with --ack"`
- `"it starts no bootstrap for a refused line"` — recording orchestrator with zero `Run` calls; picker seam never invoked
- `"it still answers --help on a search-form line"`
- `"it ignores a slash word after the -- separator"` — `~/Code/api -- ls /tmp` validates
- `"it admits a lone search form and a lone bare slash"`
- `"it still admits two positional targets"` — the multi-target burst line

**Edge Cases**:
- Words after `--` are never inspected for the shape, so a command's own `/tmp` argument cannot make the line a refusal
- `portal open /term --` carries the separator with no command; the separator itself is the collision, which keeps the "a refused line starts nothing" property (the existing empty-command error lives in `RunE`, after bootstrap)
- Recognition is positional-independent, so the refusal fires in either order and at every arity — no error message could explain a rule that depended on which argument came first
- `--help` is answered before args are validated, so it still works on a line that would otherwise be refused; root persistent flags are likewise outside the rule
- A flag value that happens to look like a search form (`-s /port`) is a value rather than a positional and is never in reach of the rule — it stays a session-pin miss
- Exit code 2 comes from `*UsageError`; a `nil` return for every non-search line keeps `open`'s otherwise-unconstrained arity, which the existing surface test asserts by calling the validator with two positionals

**Context**:
> `/term` composes with nothing. Any other target, any trailing command, and any of `open`'s own flags on the same line is a usage error. A refusal names what collided — a single message naming the search form and the element beside it that may not be there, the shape `-f`'s own mutual-exclusion refusal already takes — rather than a generic complaint about the arguments that leaves the user to find the offending word themselves.
>
> A refused line starts nothing. The refusal is decided from the arguments alone, so it needs no tmux server and takes no loading page: on a cold machine as on a warm one, a refused line prints its usage error and exits without starting the server, restoring a session or painting a frame. That is what separates a usage error from everything else on this path: a malformed line is knowable from the argv, while the match count needs a live session list.
>
> A trailing command is refused rather than redirected because it declares mint: `-f` answers a pending command by routing its term to the Projects page, which is correct for `-f` since it never promised session-domain. Inheriting that would mean typing a glyph whose entire job is to declare "search my live sessions, never mint" and landing on the mint page.
>
> The specification fixes the *shape* of the message — it names the form and the collided element, mirroring `-f`'s refusal — but not its literal wording; the strings above are this task's own and are the ones to review against that requirement.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §2.3, §5.1, §5.2, §5.3, §5.4
