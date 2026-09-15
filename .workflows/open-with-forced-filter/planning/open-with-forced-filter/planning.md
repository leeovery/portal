# Plan: Open With Forced Filter

## Phases

### Phase 1: Sigil Recognition and the Pre-Filtered Picker
status: draft

**Goal**: `portal open /term` (and the `x /term` shell form) is recognised as session-search intent, refuses to share its command line with anything else, and opens the picker on the sessions list pre-filtered by the term — with the term-less `/` opening an empty focused filter — while every other path argument keeps its current meaning.

**Why this order**: the recognition rule and the widened matched fields are what every later phase consumes: the match count, the picker's narrowed set, the directory column, the cold-boot decision and tab completion all key off the same shape and the same two fields. Widening the fields here is also what keeps this phase's own picker honest — a term that hit a directory would otherwise open a list with nothing in it. The phase delivers the dominant flow's collapse on its own, without the eager attach that needs the stricter matching rule.

**Acceptance**:
- [ ] A positional beginning with `/` and containing no further `/` is read as session-search text and never reaches the path, alias or zoxide domain; `/Users/leeovery/Code/portal`, `/tmp/`, `./port`, `~/port` and `port` resolve exactly as they do today.
- [ ] The shape is recognised wherever it sits among the targets, and words after a `--` separator are never inspected for it.
- [ ] A search form sharing its line with another target (at any arity, in either order), a second search form, a trailing command (`-e` or `--`), `-f`, any domain pin, or `--ack` is refused with one usage error naming the search form and the element beside it; the refusal is decided from the arguments alone, starts no tmux server, restores nothing and paints no frame.
- [ ] `--help` and root persistent flags still answer on a line carrying the form.
- [ ] A form carrying a term opens the picker on the Sessions page with the filter text set and committed rather than focused, and the cursor on the first surviving row in Flat, By Project and By Tag.
- [ ] `portal open /` opens the picker with the filter open, empty and focused, and is not an error.
- [ ] A session's recorded directory, home-abbreviated, is matched alongside its name by `-f/--filter` and by a filter typed by hand in the picker; a session with no recorded directory matches on its name alone with no trailing separator in the matched text.
- [ ] No `resolve` component log line is emitted for a search-form invocation.
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both pass.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-1-1 | Keep the grouping-derived directory out of the session's recorded directory | Flat mode still issues zero pane reads, a sessions refresh discards derived values, an unresolvable session keeps both values empty, By Tag reads the derived value too |
| open-with-forced-filter-1-2 | Match sessions on their home-abbreviated recorded directory as well as their name | no recorded directory (name alone, no trailing separator), directory exactly the home directory, home-prefix lookalike path not abbreviated, home lookup failure, header rows keep an empty filter value, hand-built session item still matches on its directory, grouping-derived directory never enters the matched text |
| open-with-forced-filter-1-3 | Land a search-form term as a committed sessions filter in the picker | Sessions page pinned with zero live sessions, Sessions page pinned when nothing survives the term, Flat / By Project / By Tag, existing -f landing unchanged, command-pending Projects redirect unchanged |
| open-with-forced-filter-1-4 | Take the search-form shape out of the resolution chain | bare /, multi-segment absolute path, single segment with a trailing slash, ./port, ~/port, bare word, a live session named literally /port, -p/-s/-a/-z unchanged |
| open-with-forced-filter-1-5 | Route `portal open /term` to the search-form landing | *, ? and [ in the term stay literal (no glob expansion, no burst), term equal to a live session name still opens the picker, no resolve component log line |
| open-with-forced-filter-1-6 | Open an empty focused filter for the term-less search form | -f ""'s empty-value refusal does not transfer, whole live list stays visible under an empty focused filter, cursor must not start on a group header, zero live sessions |
| open-with-forced-filter-1-7 | Refuse a search form that shares its command line | words after -- never inspected, search form second on the line, second search form, -e, --, -f, four domain pins, --ack, -- with no command, --help still answers, root persistent flags still apply, exit code 2, no server started and no frame painted |

### Phase 2: The Single-Match Shortcut Under Containment Matching
status: draft

**Goal**: the number of live sessions matching the term decides the outcome — one match attaches directly through the connector the invocation already selects, zero or two-plus open the pre-filtered picker — with the match decided by case-folded containment over the session name and the home-abbreviated recorded directory tested separately, and the sigil-opened picker's narrowed list showing exactly that set for as long as the term stands untouched.

**Why this order**: eager attach is the half of the form that can act without ever showing the user what it chose, so it may only land once the strict containment rule exists to decide it. It builds on Phase 1's recognition and landing and depends on nothing built later; the risk profile shift is what makes this a checkpoint of its own.

**Acceptance**:
- [ ] Exactly one matching live session is attached directly with no picker, through the existing outside-tmux exec and inside-tmux switch-client connectors; no third connection mode exists.
- [ ] Zero matches opens the picker with no session surviving the term, writes nothing to stderr and exits with a non-failure status.
- [ ] Two or more matches opens the picker as Phase 1 lands it, cursor on the first matching row.
- [ ] A term matches when its characters appear as a contiguous run, case-folded, in either the session name or the home-abbreviated recorded directory; the two fields are tested separately and never joined for this rule, and `*`, `?` and `[` are literal characters to find.
- [ ] The term-less form evaluates no count and opens the picker on the whole live session list, on a machine holding one live session as on a machine holding twenty.
- [ ] `_portal-saver` and `_portal-bootstrap` are never candidates and can never be attached by this form.
- [ ] The narrowed list in a search-opened picker is the containment set rather than the picker's fuzzy set, in every grouping mode, and is reproduced across a preview and back, an `s` regroup, and a refresh after a session is killed elsewhere; surviving rows keep the order the list already gives them with nothing re-ranked.
- [ ] Editing the filter text to any other value — cleared included — returns the list to the picker's own fuzzy rule; a value character-identical to the supplied term keeps containment; the term-less form is on the picker's own rule from its first keystroke.
- [ ] A session-list read that fails is reported in tmux's own terms and exits non-zero, distinct in both message and status from a zero-match result.
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both pass.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-2-1 | Decide a session match by case-folded containment over name and directory | empty recorded directory matches on name alone, case-folded on both sides, glob metacharacters literal, a run crossing the name–directory join is not a match, home lookup failure degrades to the raw recorded path, a term hitting the home prefix matches nothing, a directory outside home stays unabbreviated, an empty term is never handed to the rule |
| open-with-forced-filter-2-2 | Attach directly when exactly one live session matches the term | term-less form takes no count, `_portal-saver` and `_portal-bootstrap` never candidates, the session the user is currently in (count uses the set the picker lists), a term equal to a live session name now attaches (supersedes a Phase 1 expectation), glob metacharacters literal and no burst, inside-tmux switch versus outside-tmux exec, zero live sessions, K=0 exits non-failure with nothing on stderr, no resolve component line, a session vanishing between the count read and the picker's own read |
| open-with-forced-filter-2-3 | Narrow a search-opened list by containment while the term stands untouched | filter edited to any other value including cleared reverts to fuzzy, edited back to the identical term restores containment, term-less form on the picker's own rule from its first keystroke, group headers still drop under a non-empty term, surviving rows keep their existing order with nothing re-ranked, regroup / preview and back / external-kill refresh each reproduce the set, -f and every other picker keep fuzzy, per-item field lookup must track the rebuilt items rather than a stale capture, a session name containing a space (fields never recovered by splitting the joined filter value) |
| open-with-forced-filter-2-4 | Report a failed session-list read rather than counting it as no matches | a genuinely empty server is still zero matches and opens the picker, tmux's own stderr is the message, no picker paints and nothing is attached, `ListSessions`' no-server swallow stays intact for the picker and `portal list`, a malformed-output parse error is the same failure class, an empty session list from a live server is never a failure |

### Phase 3: The Directory Column in a Search-Opened Picker
status: draft

**Goal**: every row of a search-opened sessions list shows its recorded directory beside the session name, so a row returned on the strength of its path accounts for itself.

**Why this order**: the column exists to explain the directory matching Phase 2 delivered, and it shares that work's scope — the picker session a search opened. It is also the feature's only visual change, so it carries a human visual check, which is cleanest once the list's content is final.

**Acceptance**:
- [ ] Every row in a search-opened list — rows found by name and rows found by directory alike — shows the recorded directory beginning one space after the session name, in the same colour token as the row's window count, with no new colour token and no raw colour at the call site.
- [ ] A directory under the user's home renders home-abbreviated at every width; a session carrying no recorded directory shows an empty slot, and no pane read is issued on this path for either the match or the display.
- [ ] The directory takes the width remaining after the name and the fixed count, attached and right-margin slots; when width is short it is shortened from the left so its tail survives, and the session name never gives way to it.
- [ ] When the remaining width cannot hold an ellipsis plus one whole path segment the directory is dropped and the row shows the session name alone.
- [ ] The column renders in Flat, By Project and By Tag, for the term-less form as well, and for as long as that picker stays open including after a hand edit of the filter text; a picker reached any other way renders its rows unchanged.
- [ ] The rendering never narrows the search: a row can survive on a directory segment the width pushed out of view or the floor dropped altogether.
- [ ] With colour off the row is unchanged beyond the directory text itself — no bracket, glyph or separator stands in for the weight.
- [ ] A capture fixture renders the search-opened list and is enumerated by the fixture-coverage assertion; the rendered frame is checked against the specified placement, weight and truncation before sign-off.
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both pass.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-3-1 | Fit a session's recorded directory into a column width | no recorded directory renders nothing, a directory exactly the home directory becomes a bare `~`, a directory outside home stays unabbreviated, the abbreviation applies at every width and is never a truncation rung, truncation lands on a segment boundary so the tail survives whole, a value carrying no separator at all, the floor drops rather than truncating to noise, a last segment alone wider than the column, zero or negative width, a stamped value carrying a trailing separator, display width rather than byte length, the verdict must not depend on the developer's account name |
| open-with-forced-filter-3-2 | Render the recorded directory beside the session name in a search-opened row | the column off renders today's row byte-identical, an absent directory leaves an empty slot with no separator and no trailing space, the name never gives way (a name alone wider than the region truncates as today and the directory drops), count and attached slots stay column-aligned across differing name lengths, the row width stays exactly the list width, a grouped row's indent comes out of the same budget, an unsized list has no budget to flex against, the selected row takes the brighter rung the count already takes, colourless renders the same single space with no bracket / glyph / separator, no new colour token and no raw colour literal |
| open-with-forced-filter-3-3 | Turn the directory column on for a search-opened picker and nowhere else | the gate is the search-form flag alone and not the containment gate, so the term-less form carries the column, Flat / By Project / By Tag, header rows carry no directory, survives an `s` regroup and a `Space` preview and back and a sessions refresh and a theme restyle, survives a hand edit of the filter text including clearing it, a `-f` picker and a no-filter picker render unchanged, a width that truncates or drops the directory never removes the row from the list, a session whose directory is known only to grouping shows none and issues no pane read |
| open-with-forced-filter-3-4 | Capture the search-opened list as a fixture and check the rendered frame | the capture must pin `$HOME` or the frame shows raw absolute paths on one machine and `~/…` on another, one frame must carry a home-abbreviated path and a directory-only match and an absent directory and a path long enough to left-truncate, the fixture is enumerated by the coverage assertion and must never be exempted from it, the other grouping modes are reached live with `s` rather than by a second fixture, a `NO_COLOR` capture for the colour-off row, the tape and PNG are committed now and cleared at feature sign-off while the fixture stays |
| open-with-forced-filter-3-5 | Make the s-regroup assertions read the mode they claim to reach | the frame alone carries no mode indicator under a committed filter, so the capture site must read the settled model's list title rather than the frame; the tui site must pin all three presses including the wrap back to Flat; `searchResultsFrame` keeps its signature and its seven other call sites stay byte-unchanged with the drive loop declared exactly once; the regression check widening the sessions-page key guard is temporary and must not reach the commit; test-side only, no production file edited |

### Phase 4: Cold-Boot Classification
status: draft

**Goal**: a search-form invocation is classified as a picker invocation — taking the concurrent bootstrap and the honest loading page — with the match count taken only once the whole bootstrap has completed, and soft bootstrap warnings routed away from the screen the picker is about to claim.

**Why this order**: this phase changes when Phase 2's count is taken and where warnings land, so both the count and the picker landing must already exist before they can be re-timed and re-routed. Nothing built later depends on it.

**Acceptance**:
- [ ] On a cold server a complete search-form invocation takes the concurrent bootstrap and shows the loading page, exactly as a `-f` invocation on the same boot does; a line refused for composition still starts no server and shows no loading page.
- [ ] The count is taken only after every bootstrap step has completed, never at the end of restore; the restoring marker is cleared before anything the count decides fires.
- [ ] The loading page stands until the count can be taken, and where the count resolves before the page's minimum display span has elapsed the page stands for the remainder of it before the attach or the picker follows.
- [ ] Soft bootstrap warnings on this path ride the progress channel to the post-load notice band and are never written to stderr while the alternate screen is live.
- [ ] Where the count is one, the TUI tears down, the accumulated soft warnings are written to the terminal, and the attach follows — on a warm-server attach as well as a cold one.
- [ ] A session-list read failure on a cold boot reaches the user after teardown, is not a bootstrap fatal, and takes no in-TUI error frame.
- [ ] The warm path, the CLI paths and every non-search invocation keep their current classification, and the concurrent bootstrap's own behaviour is unchanged.
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both pass.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-4-1 | Hold the loading page until the search decision resolves | a bootstrap fatal never issues the decision and keeps the error frame, a decision resolving before the minimum display span leaves the page standing for the remainder, a decision resolving after it holds the page until it arrives, no decision seam leaves every other picker's transition byte-identical, no decision is issued on a progress event including the restore step's, a single match quits without painting the picker and leaves the buffered warnings unsurfaced, a read failure sets no fatal state and takes no in-TUI error frame, Ctrl-C during loading before the decision resolves selects nothing |
| open-with-forced-filter-4-2 | Defer the search count to the loading page when a bootstrap is in flight | a warm or latched invocation still counts up front with no seam supplied, the term-less form supplies no decision on either route, the deferred closure takes the same discriminating read and candidate set as the warm count including the inside-tmux current-session exclusion, a failed read exits non-zero without being a usage error or a bootstrap fatal, the single-match attach uses the connector built before the TUI ran, `-f` and the no-argument picker pass no decision |
| open-with-forced-filter-4-3 | Deliver the accumulated soft bootstrap warnings when a search ends without a picker | an already-drained sink writes nothing so no path double-writes, no warnings is a silent no-op, the write precedes the exec'd attach that never returns, the lines are byte-identical to the CLI path's, the concurrent write follows the terminal-background restore and precedes the connect, a zero-or-two-plus picker still routes them to the notice band with nothing written after teardown, a cancelled picker writes nothing new |
| open-with-forced-filter-4-4 | Classify a search form as a picker invocation | a line refused by the args validator never reaches the classification and still starts no server, words after a `--` separator are never inspected, a domain pin still reads as non-picker, parity with a `-f` invocation on the same boot, the warm and CLI paths keep their current classification, the term-less form classifies identically to a term-carrying one, the concurrent bootstrap's own step sequence and labels are unchanged |
| open-with-forced-filter-4-5 | Pin the search decision behind the whole bootstrap against a real cold server | the decision must not fire at the end of restore, `@portal-restoring` is clear before anything the decision drives fires, the loading page stands for every step, the fixture uses an isolated socket and state dir and never touches the developer's server or daemon, the suite carries the integration build tag |
| open-with-forced-filter-4-6 | Classify a command-only line with no target as a picker invocation | every other classified line keeps the verdict the phase already pins (bare `open`, `-f text`, `-e cmd`, `~/dir`, `api blog`, a pre-dash target before `--`, `/port`, `/port -s api`), the existing dash subtest stays unedited and green, the projects-picker landing test passes unchanged with only its cold-path classification moved, no consumer of `isTUIPath` is touched |
| open-with-forced-filter-4-7 | Fold the two loading-gate dismissal blocks into one helper | a pure refactor so no test file is edited or renamed, each guard stays in the arm that owns it, the decision-before-warnings ordering rule is stated once on the helper, the per-`Update` model copy must be addressed explicitly because the evaluation order of a non-call operand beside a call is unspecified |
| open-with-forced-filter-4-8 | Make openTUI's teardown ordering reachable by a test | a picker that opened still writes nothing, a cancelled loading page still writes nothing, the reach is proved by moving each writer below the connect and reverting, `os.Stdout` and `cmd.ErrOrStderr()` are named at the `openTUI` call site alone, the other `TestEmitSearchTeardownWarnings` subtests stay unedited |

### Phase 5: Completion and Documentation
status: draft

**Goal**: Tab completes the form against live session names, the session-opening shell function reaches Portal's completer at all, and the help text and README describe the form and the flag it is most easily confused with.

**Why this order**: completion depends on the form's final shape, and the documentation must describe the whole surface at once — including the completion correction and its rollout consequence, which only exist once that fix has landed. Without the completion fix the form is reachable only by typing `portal open /term` in full, so this phase is what makes it muscle memory.

**Acceptance**:
- [ ] `/po<TAB>` completes the term after — and excluding — the slash against live session names and leaves the slash in place, offering `/portal-a1b2` rather than `portal-a1b2`; `/<TAB>` offers every live session name; a term prefixing no name offers nothing while still matching by containment on Enter.
- [ ] Directories are never offered, and a live session whose name contains a `/` is never offered while staying reachable by a term stopping short of that slash.
- [ ] Tab after the session-opening function asks Portal for `open`'s completions in bash, zsh and fish, honouring the name chosen with `portal init --cmd <name>`; the control function's completion is unchanged.
- [ ] Filename fallback stays off on that word, so Tab on a partial single-segment absolute directory never gains a trailing slash.
- [ ] `portal open --help` carries the form and its recognition rule, the three outcomes by match count, the term-less form, that the form composes with nothing, and the distinction between `-f` and the search form stated by outcome and by which to reach for; the `-f` flag description stays a one-liner.
- [ ] The README's open section carries those points plus the single-segment absolute-directory cost and the `-p` escape, what the search matches and that the picker's own filter stays fuzzy, the completion behaviour after the session-opening function, and that an existing install picks the completion up only once the output of `portal init` is re-evaluated; the resolution table carries a row for the form alongside the domain pins and `-f`.
- [ ] No CHANGELOG entry is written.
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both pass.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-5-1 | Complete the search term after the slash against live session names | bare `/` offers every live session name each carrying the slash, a term prefixing no name offers nothing while still matching by containment on Enter, a session name containing `/` is never offered but stays reachable by a term stopping short of it, directories are never offered, `_portal-saver` and `_portal-bootstrap` are never offered, a non-sigil word (`/tmp/`, `~/Code/pro`, a bare word, the empty word) completes exactly as today, NoFileComp on every branch including zero candidates, a tmux read failure degrades to no candidates rather than an error, the completer keeps building its own client on the bootstrap-exempt `__complete` path, prefix matching stays byte-prefix as today |
| open-with-forced-filter-5-2 | Ask Portal for `open`'s completions after the session-opening function | the correction follows `portal init --cmd <name>` rather than the literal `x`, the control function is untouched because its request already resolves, bash and zsh build the request from the typed word while fish inherits through its wrap target, filename fallback stays off once Portal answers so `/tm<TAB>` never gains a trailing slash and a path argument loses its filenames deliberately, the emitted function still opens sessions unchanged on every non-completion invocation, the shells are driven with a recording `portal` stub so no portal binary is built or exec'd and the suite stays in the unit lane, a shell binary absent from the machine skips rather than fails, a `--cmd` name that shadows a real command still emits consistently |
| open-with-forced-filter-5-3 | Describe the search form in `portal open --help` | the `-f` flag usage string stays a one-liner, the distinction is stated by outcome and by which of the two to reach for rather than by input shape, the term-less form is named as not an error, the composition refusal is stated without restating the whole refusal table, the existing help-metadata keyword assertions and the `Use` line's multi-target admission both keep passing |
| open-with-forced-filter-5-4 | Document the search form and the completion correction in the README | the resolution-table row is a positional rather than a flag so it must not read as a domain pin, the single-segment absolute-directory cost and the `-p` escape, containment versus the picker's own fuzzy rule, the completion correction belongs to `portal init` rather than to `open`, an existing install picks the completion up only on a new shell or a re-run of `portal init` while the form itself works the moment the binary is new, the example block gains `x /port` and `x /`, no CHANGELOG entry is written |
| open-with-forced-filter-5-5 | Keep the user's cursor position through the bash completion shim | the measurement comes first and a measurement contradicting the derivation voids the finding and leaves `cmd/init.go` unedited, the existing `_init_completion` stand-in models away the cursor derivation so the guard must be taught it before the shim is touched, the mid-line case must fail against the pre-edit shim, a line with leading whitespace where a plain prefix strip removes nothing is handled or its limit stated beside the shim, the rewritten line preserves the user's own spacing, `zshOpenCompletionShim` and the fish emission and both `__start_portal` registrations stay byte-unchanged, the verbatim shim expectation in `cmd/init_test.go` moves with the shim, no portal binary is built or run and no tmux server is contacted |
| open-with-forced-filter-5-6 | Corrections | prose only so no source, test or specification file is edited and no behaviour changes, the durable rollout fact survives the de-versioning as a property of `portal init`, the paragraph keeps what `x <TAB>` and `x /po<TAB>` offer and the deliberate filename-fallback loss and the bash-version parenthetical, the domain-pin sentence names `-f` and `/<term>` and `-e`/`--` rather than counting rows, the section keeps the tokens its guard reads and the commented `x /port` and `x /` example lines, `TestReadmeDocumentsSearchForm` stays green unmodified |

### Phase 6: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-6-1 | Single-source the session set the sigil counts and the picker lists | an empty current session name drops nothing, a name no session carries drops nothing, the caller's slice is left unmutated (no `slices.DeleteFunc` tail-zeroing), the `m.insideTmux` guard stays at the call site, the underscore-prefixed exclusion stays in `parseSessionList`, `searchCandidates` keeps its own read and its failed-or-empty-read policy |
| open-with-forced-filter-6-2 | Derive the searchable field list from one declaration | an empty recorded directory yields the name alone and a `FilterValue` with no trailing separator, the fields stay tested separately so a run spanning the name-directory join is not a match, an empty term matches nothing, the two display-side `AbbreviateHome` calls stay as they are, eager abbreviation is accepted with no cache |
| open-with-forced-filter-6-3 | Make the containment filter's item source self-contained | a same-length replacement (a rename, a same-count refresh) must fall back as well as a length change, a never-`set` source behaves as today, header rows are still skipped and `MatchedIndexes` stays nil, a fresh companion slice is built on every `set`, items and recorded values come from one hold, `-race` with `set` driven against a concurrent filter pass |
| open-with-forced-filter-6-4 | Deliver the soft bootstrap warnings the warm picker route drops | the clear sits inside the `PageLoading` block so a cancelled loading page is owed nothing, nothing is written twice on the warm loading-page route or the cold concurrent route, the K=1 attach and failed-read teardowns keep writing `BufferedWarnings()` exactly once, no warnings means no write, both writes precede the connector that execs and never returns, a warm `-f` and a warm bare `open` are covered too |

### Phase 7: Ad Hoc

**Goal**: Ad hoc additions.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-7-1 | Deliver the concurrent bootstrap's terminal event to the command-pending picker | a warm command-pending model with a nil receiver issues nothing extra and its batch is otherwise unchanged, the `BootstrapCompleteMsg` arm's `PageLoading` buffering gate stays exactly as it is so the warnings stay pending and reach `finishTUI` once rather than twice, a fatal must set `fatalActive` whatever page the model is on, a fatal must not mint the session so `processTUIResult` is stopped if it can still run the command, the loading page's behaviour stays byte-identical including the notice band and the destructive error frame, `isTUIPath` and `shouldRunConcurrentBootstrap` are untouched |
| open-with-forced-filter-7-2 | Hold the command-pending picker's mint until the bootstrap's terminal event arrives | a warm command-pending model (nil `progressReceiver`) mints on the keypress exactly as today with no staging and no band change, first stage wins so a second Enter or `n` changes neither the staged directory nor the band, a staged empty directory stays distinguishable from nothing staged, both call sites assign the returned command to a local before returning, no staged mint is ever issued from the `BootstrapFatalMsg` arm nor by a `BootstrapCompleteMsg` arriving after a fatal, a live flash still claims the Projects slot ahead of the wait band, `projectBandHeight` matches the rendered slot and the list budget is re-measured at both the staging and the issuing, Esc and Ctrl-C still quit while a mint is staged, the loading-gated routes stay untouched with no new call reaching a real tmux server |

### Phase 8: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-8-1 | Route every search surface's session set through one attached-session read | outside tmux the helper takes no read at all so the zero-call assertion in `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux` holds, a failed or empty current-session read drops nothing on any of the three surfaces, the bare `/` completion holds back the attached name too, a held-back name never reaches the `"/"+name` append, only the search branch consults the helper so `completeSessionNames` offers the same set as today including the attached session, the new completion seam is staged in tests only through `withFuncSeam` so the derived function-var guard stays green, the `SearchSessionSource` seam and `tui.PickerSessions` and the `openDeps.SearchSessions` wiring are untouched, the two connector-selection `tmux.InsideTmux()` uses stay as they are |
| open-with-forced-filter-8-2 | Corrections | documentation text only so no Go file is touched and no behaviour moves, `cmd/completion.go`'s hold-back and the untouched `completeSessionNames` are both correct as they stand, the edit is confined to `README.md:175` and the rest of that line stays byte-intact including the `portal init` re-evaluation clause the guard reads, the two completers' sets are named separately rather than equated, the slash-stays-in-place record survives, the ``### `x` (open)`` section keeps the tokens `` `/<term>` `` and `-p /tmp` and `portal init` and `-f, --filter` plus the commented `x /port` and `x /` example lines, the new clause is pitched in the README's register rather than quoting the specification, `TestReadmeDocumentsSearchForm` stays green unmodified and no test is added, removed or reworded |

### Phase 9: Analysis (Cycle 3)

**Goal**: Address findings from Analysis (Cycle 3).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| open-with-forced-filter-9-1 | Refuse every malformed open line before any bootstrap runs | each refusal's message text stays byte-identical and still arrives as a `*UsageError` (exit 2), a line colliding several ways names the refusal it names today (search form, then `-e`/`--`, then `-f`), `parseCommandArgs` stays the single home of the command-scoping refusals with nothing restated, `--help` still prints help on a malformed `-f` line, the `--ack` shape check stays in `RunE`, completion on the `__complete` path is unaffected |
| open-with-forced-filter-9-2 | Make pickerLanding carry the search form itself | a search landing leaves `filter` empty and a `-f` landing leaves `search` nil, all five routes (bare sigil, warm term, deferred term, `-f`, no-args picker) hand the model what they hand it today, `cmd` takes no new import, every converted assertion asserts the same fact with the term changing field rather than meaning |
| open-with-forced-filter-9-3 | Let the model answer what the teardown still owes the terminal | a cancelled loading page is owed nothing (no decision recorded, pending set already emptied), route parity across search attach / search read failure / warm picker / picker after a loading gate / cancelled page, nothing written twice on any route, the decision reads the model's own fields rather than `SearchAttached`/`SearchError`, `surfaceBufferedWarnings` and the concurrent route's notice band unchanged, the teardown's lines still match the CLI path's byte for byte |
| open-with-forced-filter-9-4 | Declare the session-opening function's expansion once | the emitted text stays byte-identical for bash, zsh and fish, for the default `x` and for `--cmd p`, the shims render through a format at their emission site and neither carries a `%` character, the bash shim's `expansion=` and `COMP_WORDS=` derive from one const, the sites naming the binary (`portal "$@"`, `-w portal`, `compdef _portal %s`, the `__start_portal`/`_portal` names) are left alone, no shell suite is edited |
