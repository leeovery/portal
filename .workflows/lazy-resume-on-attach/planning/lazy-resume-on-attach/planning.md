# Plan: Lazy Resume On Attach

## Phases

### Phase 1: Resume mode — storage, override and read-back
status: draft

**Goal**: Every registration resolves to eager or lazy from an install-wide default plus a per-registration override, set at `portal hook set` and readable from `portal hook list`, with `hooks.json` accepting both the string and the object value shapes.

**Why this order**: The mode is what every later phase branches on — whether the helper waits, what the panel draws, what doctor counts. It is also the feature's only change to an on-disk shape that out-of-repo scripts read, so it is the edit that most wants to land alone and early, against a system whose behaviour is otherwise unchanged.

**Acceptance**:
- [ ] A string value and an object carrying `command` plus `resume` both load; a registration with nothing to carry is written back in the string form.
- [ ] Rewriting one registration leaves every other entry exactly as it was found — attributes the reader does not model and `resume` values it cannot make sense of alike.
- [ ] An object whose `resume` is absent, empty or unrecognised carries no mode and inherits the install default; an object carrying no command, or an empty one, is not a registration and the lookup reports a miss.
- [ ] `prefs.json` holds `resume_mode` as `eager` or `lazy`, decoding tolerantly and independently of every other key, omitted on write when unset, and defaulting to lazy when missing, empty, corrupt or unrecognised.
- [ ] `portal hook set --resume-mode eager|lazy` writes the object form; `--resume-mode` with no `--on-resume`, and any value other than `eager` or `lazy`, exit non-zero and write nothing; a call passed no mode writes a registration carrying none whatever its predecessor carried.
- [ ] `portal hook list` appends a fifth tab-separated column holding `eager`, `lazy`, or empty when the registration carries no mode; the first four columns are unchanged.
- [ ] One resolution path answers eager-or-lazy for a registration from its own mode and the install default.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| lazy-resume-on-attach-1-1 | Resume mode vocabulary and resolution | unrecognised stored value carries no mode, empty value carries no mode, case and whitespace variants unrecognised rather than coerced, strict flag parse refuses every value but the two words, resolution answers lazy when neither side names a mode |
| lazy-resume-on-attach-1-2 | hooks.json accepts the object form and preserves what it did not write | unmodelled attribute survives a sibling rewrite, unrecognised resume value survives a sibling rewrite, a value that is neither string nor object never fails the load for other entries, clean-stale's recoverable value breadcrumb renders the command out of an object entry |
| lazy-resume-on-attach-1-3 | hook set writes the registration whole in the shape its content chooses | a mode-only change is a modify not a set-noop, an identical object-form rewrite is still a noop, an object-form predecessor rewritten with no mode returns to the string form and loses its unmodelled attributes |
| lazy-resume-on-attach-1-4 | The store's reads report the registration's mode | absent, empty or unrecognised resume reads as no mode, an object with no command key or an empty one is a lookup miss, a string-form entry reads as no mode, an unreadable store still degrades to no hook |
| lazy-resume-on-attach-1-5 | prefs.json carries resume_mode | missing, empty, corrupt or unrecognised gives lazy, a wrong-typed value does not zero the rest of the tolerant record, a hand-set value survives a grouping-mode toggle and a theme commit, the strict write-path decode still aborts on a malformed file |
| lazy-resume-on-attach-1-6 | portal hook set --resume-mode | --resume-mode with no --on-resume exits non-zero writing nothing, an unrecognised or empty value exits non-zero before any tmux read, token mint, pane stamp or save.requested touch, an unpassed flag writes a registration carrying no mode whatever its predecessor carried |
| lazy-resume-on-attach-1-7 | portal hook list shows the mode in a fifth column | first four columns byte-identical for a positional external parser, an unrecognised stored resume renders empty, no second tmux read is added |

**Planner's calls (structure, not product)**:
- The eager/lazy vocabulary lands in a **new stdlib-only leaf package**, not inside `internal/hooks`. Both packages that need it — `internal/hooks` (the stored `resume` attribute) and `internal/prefs` (the `resume_mode` key) — are guarded leaves forbidden from importing each other by their own leaf-guard tests, which is the situation `internal/nanoid` already exists to answer. Each package's allowlist widens by that one entry.
- `LookupOnResume` returns a **small result struct** rather than growing to four returns, because the Phase 4 call site in the hydrate helper reads the command and the mode together.


### Phase 2: The pending marker and the saver's freeze
status: draft

**Goal**: A pane carrying the pending marker is left alone by the saver's scrollback capture for as long as the marker stands, while keeping its place in the saved set.

**Why this order**: Nothing may wait before the protection exists — a wait without the freeze costs the user a screenful of their own transcript at the first tick that lands and writes a dead card into the saved history. It also carries the feature's highest-blast-radius edit: the per-pane capture read gains a column and the saver's per-tick skip gains a condition, both cheapest to verify while the rest of the system is unchanged.

**Acceptance**:
- [ ] `@portal-resume-pending` is set to `1` as a pane user-option through the same marker vocabulary the skeleton markers use, with set, read and clear all covered.
- [ ] The per-pane capture read carries the marker as an additional column and parses at the new arity; a pane that has never been marked reads as unmarked.
- [ ] The saver skips a pane's scrollback capture when it is mid-restore or when it carries the pending marker; a pane carrying neither is captured exactly as today.
- [ ] A marked pane keeps its marker and its skip through `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k`, and a session rename.
- [ ] A skipped pane is still enumerated into `sessions.json` and keeps its previous scrollback record, so it restores with the transcript it held when it was marked.
- [ ] A frozen pane's previous record is found by its durable `@portal-pane-id` rather than by its address, and the merged record keeps pointing at the scrollback file that already holds its bytes — so a rearrangement during the wait cannot orphan the transcript or let the housekeeping pass reclaim it.
- [ ] Clearing the marker returns the pane to ordinary capture on the next tick.
- [ ] Nothing about the pending state is persisted and no `sessions.json` schema version moves.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| lazy-resume-on-attach-2-1 | The pending marker: one name, set, read and clear | an already-absent option unsets as a no-op, an empty value reads as absent exactly as an unset one does, the option name is composed from one constant and never restated at a call site, the writer seam is parameterised over the target type so internal/state keeps its no-internal/tmux rule, a hand-composed `-t` argument cannot reach the unset without failing the target-composition guard |
| lazy-resume-on-attach-2-2 | The per-pane capture read carries the pending marker | a pane that has never been marked reads unmarked, a row at the old arity still fails the parse rather than short-reading, the pending state reaches no persisted field and SchemaVersion does not move, every eleven-field pane-row fixture across cmd and internal/state moves to twelve, commit-now's injected CaptureStructure seam type changes with the signature |
| lazy-resume-on-attach-2-3 | A frozen pane's previous record is found by its durable token | a pane whose address changed keeps the file its bytes are in and the housekeeping pass keeps that file referenced, a pending pane carrying no token falls back to the positional match, a previous record whose pane is absent from the live enumeration is never resurrected, a token matching more than one previous record resolves deterministically and is consumed once, the merged record takes the live address and the previous content, the merge runs off the capture's own column read so commit-now's nil skip set does not opt out, a stable merge does not make the structural comparison rewrite every tick |
| lazy-resume-on-attach-2-4 | The saver's skip gains its second condition | a pane carrying neither marker is captured exactly as today, a pane carrying both is skipped once, a skipped pane is absent from the tick summary's pane count and emits no per-pane capture breadcrumb, a cleared marker returns the pane to ordinary capture and re-files it at its current address on the next tick, the shutdown flush inherits the same skip, commit-now writes no scrollback and needs no skip of its own, no new log event or attr key is introduced |
| lazy-resume-on-attach-2-5 | The marker and the freeze travel with the pane | each of break-pane, move-pane, a window close under `renumber-windows on`, `respawn-pane -k` and a session rename is asserted after its own move rather than only at the end, `renumber-windows on` is set explicitly on the fixture server, the frozen pane's record still names the file holding its bytes after a rename, the assertions run through Portal's own capture read rather than raw tmux, unit lane on a disposable socket with no daemon and no built binary |

**Planner's calls (structure, not product)**:
- The structural capture returns the pending pane keys as a second value rather than carrying the flag on the saved pane record. A non-persisted field would still be compared against an index decoded from disk, where it is always false, so every tick would read as a structural change for as long as any pane waits. The cost is a mechanical extra return at the call sites, which the compiler finds.
- The token-matched merge is driven by the capture's own column read rather than by the caller's skip set. `portal state commit-now` — fired by the session-closed tmux hook — passes no skip set, so a skip-set-driven merge would let a commit-now landing after a rearrangement write a record naming a file that does not exist and let the housekeeping pass delete the one that does: the same loss by a second route. The specification states the rule without naming this writer; driving the merge from the column is what keeps the rule true for every writer of the saved state.
- The token match is taken from the live side — iterate the panes the enumeration returned and look each pending one's token up in the previous state — which preserves the existing guard against resurrecting a pane whose session, window or pane is gone without restating it.
- Two CLAUDE.md sentences go stale inside this phase (the capture read's field count, and the saver-skip and marker-vocabulary descriptions); the one-line edits ride with the tasks that falsify them rather than a separate documentation task, as Phase 1 did.


### Phase 3: The resume panel and its discard confirmation
status: draft

**Goal**: Both screens a waiting pane can show — the waiting panel and the discard confirmation — render as full-pane canvases carrying Portal's card grammar, at any pane size and in any theme, and can be viewed on demand for design sign-off.

**Why this order**: The panel is the feature's whole user surface and its design is fixed by reference frames, so this is where human validation is worth a checkpoint. Rendering it as a value — a function of the command, the size, the theme and whatever there is to report — lets the design be signed off before a blocking process is wired behind it, and gives the next phase something real to draw.

**Acceptance**:
- [ ] The waiting panel renders a full-pane canvas in the active theme with a centred card carrying `Resume session` and a `● PAUSED` badge, an `ON RESUME` label over the registered command, and `⏎ resume` / `d discard` hints.
- [ ] A command longer than the card's inner width wraps over at most three lines with anything beyond marked `…`, and the card's width does not change.
- [ ] Either screen carries one report row between the command and the key hints when there is something to report, and exactly its three parts when there is not.
- [ ] The discard confirmation is built through the same shared destructive-confirm builder as the picker's kill modal: `▲ Discard resume?`, the command in the destructive token, a plain-language consequence line, and `y discard   esc cancel`.
- [ ] Below the size the card needs, both screens drop the frame and stack their parts plainly on the canvas, down to the smallest pane a restore can produce; neither ever draws nothing.
- [ ] A named theme paints from the first frame; a light/dark pair runs the same detect-or-timeout appearance gate the picker runs and resolves dark with no answer. No raw hex appears at any call site.
- [ ] Under `NO_COLOR` no canvas is painted and no detection runs, and every state on both screens is carried by glyphs and words rather than colour.
- [ ] Every string either screen renders is tool-agnostic — it states the command and says nothing about what the command is.
- [ ] Both screens can be rendered on demand at a chosen theme and width for visual check against the committed reference frames.

### Phase 4: The waiting pane — hand-off, hold, and resume
status: draft

**Goal**: A restored pane whose resume is lazy comes back holding the panel and a live waiter, protected from the moment it is marked, and Enter starts the registered command over the transcript that was underneath it.

**Why this order**: This is the first phase where the feature exists; every prior phase is something it consumes. It can only follow the freeze, because the helper must mark the pane before it stops being mid-restore, and the panel, because the waiter's first act is to draw it.

**Acceptance**:
- [ ] The helper resolves the pane's mode before it clears the mid-restore marker; a pane with no registration, and one that resolves eager, is never marked and restores exactly as it does today.
- [ ] A pane that will wait is marked pending before the mid-restore marker is cleared, so it is never unprotected.
- [ ] A pane that cannot be marked does not wait: the helper fires the hook as an eager registration does and records one WARN naming the pane and the error that refused the marker.
- [ ] The process that draws hands off to a fresh minimal wait before blocking, and every later screen takes the same handover, so a pane between screens carries the wait and nothing else.
- [ ] A stream of size changes produces one redraw once the size has settled, not one per change.
- [ ] Enter and `d` are the only keys the panel acts on; everything else is swallowed, Escape is inert, and Ctrl-C, Ctrl-D and Ctrl-Z do not end the waiter.
- [ ] Enter takes the pane off the panel's screen, clears the marker, then reads the store again and runs what it holds then; an entry that has gone and an unreadable store both drop the pane to a plain shell, with the marker cleared either way.
- [ ] A clear that fails holds the answer: the panel is drawn again carrying the reason, the key can be pressed again, and neither the hook nor the shell runs while the marker stands.
- [ ] A waiter torn down with its pane exits; one that exits without handing the pane over leaves the pane on its own transcript, clears the marker, records a WARN if that clear failed, and execs the user's shell either way.
- [ ] Scrollback replays for every restored pane exactly as it does today, whether or not a resume is pending, and no bootstrap step, step ordering, eager signal pass or global hook changes.

### Phase 5: Discarding a resume from the panel
status: draft

**Goal**: `d` opens the confirmation and `y` removes the pane's registration permanently, leaving the session live and the pane on a plain shell with its transcript above it.

**Why this order**: It is the second of the panel's two answers and the only one that destroys something, so it follows a panel that already resumes correctly. The machinery it needs — the confirmation screen, the report row, the marker-clear ordering, the fall-through to a shell — is all in place by then, so this phase adds the removal and its guards rather than the plumbing.

**Acceptance**:
- [ ] `d` puts the confirmation up; while it is up only `y` and Escape act, Escape brings the waiting panel back unchanged, and Enter is swallowed there.
- [ ] Input already in flight when the confirmation opened cannot confirm it — only a keystroke arriving after it is on screen agrees.
- [ ] A confirmed discard removes that pane's `on-resume` registration and nothing else: no other entry is touched, the pane's durable token is not unstamped, and the tmux session stays live and saved.
- [ ] The removal emits one INFO line under the `hooks` component carrying the hook key and the removed command, under a new `op` of `discard` and a new `via` of `panel`, each added to its closed vocabulary.
- [ ] A discard that finds nothing to remove is still a discard: the marker clears and the pane falls through to a shell.
- [ ] A discard the store will not accept is reported on the confirmation itself and retried from there; the registration and the marker both stand, and the confirmation never closes on a failed write.
- [ ] After the removal the pane leaves the panel's screen, the marker is cleared, and the pane falls through to a plain shell; a clear that fails brings the card back as it was drawn, naming the removed command, with the reason on its report row and both hints live.

### Phase 6: Seeing what is waiting
status: draft

**Goal**: A pending decision is visible outside its own pane — as a count and an install-mode line in `portal doctor`, and as a second indicator on the picker's session row.

**Why this order**: Both surfaces report on a state that only exists once panes actually wait, so they are built and verified against a system that can produce one. Nothing earlier depends on either.

**Acceptance**:
- [ ] `portal doctor` reports the pending-pane count and the install's resume mode as informational lines that never fail, never change the exit code, and are excluded from both the passed and the total counts the summary reports.
- [ ] The picker's session row drops the word `attached`; the attached indicator stands alone as a dot.
- [ ] A row carries a second dot in the attention token when any pane in its session is waiting, whatever the number of waiting panes it holds.
- [ ] The dots pack right in a fixed order — attached, then pending — so a row with one puts it hard right and a row with both pushes the attached dot left; neither holds a reserved lane.
- [ ] Under `NO_COLOR` each indicator renders as a letter in the same cell — `A`, `P`, or `AP` — with the packing order untouched and no second row geometry.
- [ ] A row flagged gone is unaffected: the transient badge still replaces the whole trailing region.
- [ ] The help modal gains a legend for both indicators, worded tool-agnostically.
- [ ] The picker resolves pending state from a single whole-server read, and the row renders for visual check against its committed reference frame.
