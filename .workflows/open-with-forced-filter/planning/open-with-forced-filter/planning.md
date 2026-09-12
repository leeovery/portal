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
