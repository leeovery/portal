# Analysis Tasks: Open With Forced Filter (Cycle 6)

## Task 1: Give the bootstrap's terminal event one warnings provenance

severity: medium
sources: architecture

**Problem**: `BootstrapCompleteMsg.Warnings` means two different things depending on which route produced it, so no single merge rule fits both and the two arms that read it already disagree. On the warm route `Init` synthesizes the message from the model's own staged set (`internal/tui/model.go:1555-1556` — `pending := m.pendingBootstrapWarnings; … BootstrapCompleteMsg{Warnings: pending}`), so the field *is* the staged set. On the concurrent route the progress pipe supplies the orchestrator's own warnings, which are disjoint from anything staged. The `PageLoading` arm resolves this by assigning and then nil-ing (`model.go:1648-1660`: `m.bufferedWarnings = msg.Warnings`, then `m.pendingBootstrapWarnings = nil`) — lossless on the first route only because the two sets are the same slice, and on the second only because the sink happens to be empty there today (`cmd/root.go:143-148` adds to the sink solely on the synchronous branch; the concurrent branch returns before it). The `commandPending` arm two lines below reaches the opposite conclusion for the same message — `append`, with a comment stating the two sets are disjoint and both owed. Neither arm states the invariant it depends on, and the invariant lives in a different package. Add any pre-bootstrap warning to the sink on the deferred route and the loading arm silently eats it: the user is never told the saver is down or that restore degraded, `WarningsOwedAtTeardown` (`model.go:485-490`) then has nothing to write on the search-attach exit, and nothing on any surface records that a warning existed. The design fault is the message field, not the arms.

**Solution**: Stop routing the staged set through the synthesized message, so the terminal event carries one provenance — the orchestrator's warnings and nothing else. `Init`'s warm-route synthesis (`internal/tui/model.go:1555-1556`) emits a warnings-free `BootstrapCompleteMsg{}`, and the `PageLoading` arm reads the staged set off the model itself, applying the same disjoint-union rule the `commandPending` arm already applies: buffer the staged set followed by the message's own, then clear the staged set. Build the union into a fresh slice rather than appending onto the staged one, so the two never share a backing array. Both arms then run one rule with no unstated cross-package invariant behind either, and the `m.progressReceiver == nil` test that decides whether the synthesis happens at all stays exactly as it is. Behaviour-preserving on every route that exists today: on the warm route the union is the staged set (the message carries nothing), on the concurrent route it is the orchestrator's set (the sink is empty), and on the command-pending route nothing changes at all. This extends cycle 1's approved Task 4, cycle 3's Task 3 and cycle 4's Task 3 rather than reversing any of them — `PendingBootstrapWarnings()` still reads as "warnings no loading gate ever consumed", the teardown's answer still lives in `WarningsOwedAtTeardown`, nothing is written twice, and the disjointness cycle 1 asserted in a comment becomes the arm's actual rule.

**Outcome**: One message field with one meaning, both arms of the switch running the same merge rule, and a warning staged before a concurrent launch reaching the user instead of disappearing.

**Do**:
- In `Init`'s warm-route branch (`internal/tui/model.go:1549-1557`), drop the `pending := m.pendingBootstrapWarnings` capture and emit `BootstrapCompleteMsg{}`; the `m.progressReceiver == nil` test that decides whether the synthesis happens at all stays exactly as it is.
- In the `BootstrapCompleteMsg` arm's `PageLoading` branch (`model.go:1648-1652`), set `m.bufferedWarnings` to a fresh slice holding the staged set followed by the message's own — `slices.Concat(m.pendingBootstrapWarnings, msg.Warnings)`, with `slices` already imported — then clear `m.pendingBootstrapWarnings`. Leave the `commandPending` branch's `append` (`model.go:1653-1660`) alone.
- Correct `SetPendingBootstrapWarnings`'s doc comment (`model.go:492-493`), which says a loading page folds the staged set into the `BootstrapCompleteMsg` from `Init`'s first tick — after this change no route does.
- Re-point the drivers that hand-simulate the warm route by passing the staged set through the message, so they stage it and deliver `BootstrapCompleteMsg{}`: `internal/tui/model_test.go:6900` and `:7095`, `cmd/open_search_warnings_test.go:401` and `:427`.

**Acceptance Criteria**:
- [ ] The message `Init` synthesizes on the warm route carries no warnings, and no production path reads the staged set into a `BootstrapCompleteMsg`.
- [ ] A warning staged before a concurrent launch reaches `bufferedWarnings` alongside the orchestrator's own, the staged one first, and `PendingBootstrapWarnings()` is empty afterwards.
- [ ] The warm route still buffers exactly the staged set — one copy of each warning, not two.
- [ ] `bufferedWarnings` and `pendingBootstrapWarnings` never share a backing array: appending to the buffered union writes nothing into the staged slice.
- [ ] `WarningsOwedAtTeardown` returns that union on a search-attach exit, so a warning staged before a concurrent launch is still written when the gate quits without painting a picker.
- [ ] The `commandPending` branch, `WarningsOwedAtTeardown`'s own rule and the drop of a complete arriving after dismissal are unchanged, and `go test ./...` is green.

**Tests**:
- `"it buffers the staged set and the message's own, staged first"`
- `"it empties the staged set once the loading gate has taken it"`
- `"its warm-route synthesized complete message carries no warnings"`
- `"it buffers a warm route's staged set exactly once"`
- `"the buffered union does not share a backing array with the staged slice"`
- `"it still owes a staged warning at teardown when the gate quits on a search attach"`
- `"it leaves a command-pending model appending the message's warnings onto the staged set"`

## Task 2: Hold the containment rule through overlapping list rebuilds

severity: low
sources: standards

**Problem**: On a picker opened by `x /port`, two list rebuilds dispatched close together leave two filter commands in flight against different item generations — a `Space`-preview dismiss queues a sessions refresh and the user presses `s` before it lands, which is two of the three events the containment guarantee names by name. `rebuildSessionList` (`internal/tui/model.go:1250-1255`) re-points the single-generation `searchItemSource` and then hands the items to `SetItems`, whose filter command captures that generation's targets (`bubbles/v2/list.go:1243-1273`); the older command finds `recorded != targets` and falls through to `list.DefaultFilter` (`internal/tui/search_filter.go:57-58`). `FilterMatchesMsg` is applied unconditionally (`list.go:828-830` — no generation check), so if the stale result is delivered last the narrowed list ends up showing subsequence matches: sessions whose name and recorded directory contain no run of "port" — the `~/Projects/rust-tools` case the containment rule exists to exclude. Nothing re-filters afterwards, so the wrong set stands until the next rebuild, and `Enter` on one of those rows attaches a session the search never matched. The user sees rows they cannot account for in a list they were told was narrowed by the term they typed.

**Solution**: Make the containment rule answerable from the `targets` a pass was handed, so a stale source can never force the fallback. `searchItemSource` retains, beside the slice, a map keyed on the item's filter value holding the name and recorded directory that value was built from (rows sharing a session share one entry, so By-Tag duplicates collapse harmlessly; a `HeaderItem`'s empty filter value is not in the map, so headers rank nothing as they do today). `containmentFilter` then walks `targets`, resolves each through that map, and emits a rank at the target's own index — which is the index the list resolves against its own generation's items, so the rank is correct for whichever generation the pass belongs to. With the answer no longer depending on the source holding the same slice, drop the `slices.Equal(recorded, targets)` gate and the `list.DefaultFilter` fall-through with it, leaving `query != term` as the single route back to the picker's own rule — which is the test the specification states. A target the map does not hold yields no rank, so the worst a stale pass can do is omit a row already gone, never show a row the term does not match. The `sync.RWMutex` and the both-under-one-hold accessor stay as cycle 1 settled them.

This corrects one clause of cycle 1's approved Task 3, which settled that "a source out of step still yields `list.DefaultFilter`". The ground is the specification: it states the containment set holds "for as long as the sigil's filter text stands untouched", names a `Space` preview and back, an `s` regroup and a refresh after an external kill as events that each "reproduce the containment set", and states that "only a hand edit of the filter text returns the list to the picker's own rule, and the test is the text rather than the act". A scheduling overlap between two of those very events is not a hand edit, so the escape hatch that task preserved is reachable on a line the specification says is closed. That task's actual direction — the source defines the access rather than relying on caller discipline, and a pass never resolves ranks against the wrong rows — is what this completes.

**Outcome**: The containment set survives every rebuild the specification names, whatever order two filter passes land in, and the picker's fuzzy rule returns only when the filter text itself changes.

**Do**:
- Replace `searchItemSource`'s `items []list.Item` / `recorded []string` pair (`internal/tui/search_filter.go:18-41`) with one map keyed on the item's filter value, holding the `Session.Name` and `Session.Dir` that value was built from. `set` populates it from the `SessionItem`s it is handed under the existing write lock — a `HeaderItem`'s empty `FilterValue()` contributes no entry, and rows sharing a session collapse onto one — and the accessor answers with it under the read lock, keeping the `sync.RWMutex` and the single both-under-one-hold shape cycle 1 settled.
- Rewrite `containmentFilter` (`search_filter.go:54-72`) to walk `targets`, resolve each through that map, and emit `list.Rank{Index: i}` at the target's own index when `resolver.MatchesSearchTerm(query, entry.name, entry.dir)` holds; a target the map does not hold ranks nothing, and `MatchedIndexes` stays nil.
- Drop the `slices.Equal(recorded, targets)` gate and the `list.DefaultFilter` fall-through with it, leaving `query != term` as the single route back to the picker's own rule, and correct the `searchItemSource` and `containmentFilter` doc comments where they state the dropped rule.
- Replace the pins that assert that fall-through — `TestContainmentFilterFallsBackWhenTheSourceIsOutOfStep`, `TestContainmentFilterFallsBackWhenTheSourceWasReplacedAtTheSameLength` and the non-empty-targets case of `TestContainmentFilterOnASourceThatWasNeverSet` (`internal/tui/search_filter_test.go:13-72`, `:113-138`) — with the rule that replaces it.

**Acceptance Criteria**:
- [ ] A filter pass handed the targets of a generation the source no longer holds ranks those targets by containment; a session matching only as a subsequence (`~/Projects/rust-tools` for `port`) never appears.
- [ ] `list.DefaultFilter` is reached on exactly one condition — the committed query is no longer the term.
- [ ] Ranks index the `targets` slice the pass was handed, so the list resolves each against its own generation's items.
- [ ] A target the map does not hold ranks nothing, so the worst a stale pass can do is omit a row already gone.
- [ ] A header row's empty filter value ranks nothing, and every row of a session listed under more than one tag ranks.
- [ ] The `sync.RWMutex` and the single accessor stay, and the concurrent-`set` test is green under `-race`.
- [ ] `internal/tui/search_containment_test.go` and `go test ./...` stay green.

**Tests**:
- `"it ranks a stale generation's targets by containment rather than the picker's fuzzy rule"`
- `"it ranks every row of a session that appears under more than one tag"`
- `"it ranks nothing for a target the source does not hold"`
- `"it ranks nothing for a header row's empty filter value"`
- `"it returns to the picker's own rule only once the filter text is no longer the term"`
- `"it stays race-free against a concurrent set"`

## Task 3: Take the search decision off the render goroutine

severity: low
sources: architecture

**Problem**: `resolveSearchDecision` (`internal/tui/search_decision.go:38-56`) calls a `cmd`-supplied closure synchronously from inside `Update`, at the one transition the loading page exists to cover. The closure (`cmd/open_search.go:177-211`) performs two blocking tmux round-trips — `ListSessionsProbe`, then `CurrentSessionName` through `currentPickerSession` — so at the instant the loading page would lift, the event loop stops: the spinner freezes and queued input, including Ctrl-C, is not processed until both subprocesses return. Against a tmux server that answers bootstrap and then turns slow, the user sees a frozen loading page with no way out but killing the terminal, which is precisely the "reads as a hang" failure the loading page was introduced to prevent. Every other tmux read the model performs on this path goes through a `tea.Cmd` and lands as a message (`fetchSessionsCmd`, `loadProjects`, `refetchSessionsAfterRestore`), and the ten-step bootstrap runs in a goroutine for the same reason. The comment explains the synchronous call as the price of pre-empting the transition, and `internal/tui` has no control over how long a closure `cmd` supplies may take or whether it can be cancelled. One of the two round-trips is also for a value the model is already holding: `openTUI` resolved the attached session name and handed it over as `CurrentSession`.

**Solution**: Make the decision a third condition on the dismissal gate rather than a synchronous call at dismissal. When `minElapsed` and `bootstrapComplete` are both satisfied and a decision is still outstanding, dispatch the closure as a `tea.Cmd` and hold the loading page; act on the message it returns — attach or read failure quits as it does now, and any other outcome runs the existing `dismissLoadingGate` sequence unchanged. The single-shot property then comes from the gate's own state rather than from clearing a field mid-call, and the ordering the helper holds (the decision precedes `surfaceBufferedWarnings`, which empties the buffer the teardown still owes) is preserved as gate ordering rather than statement ordering. A slow server leaves the page animating with Ctrl-C live. This extends phase 4's approved "fold the two loading-gate dismissal blocks into one helper" — both gates still dismiss through one place that holds the sequence — and leaves cycle 4's and cycle 5's directions on `searchAttached`/`searchErr` and the value-in/value-out shape exactly as they stand.

**Outcome**: No unbounded subprocess I/O runs on the Bubble Tea update goroutine, and the loading page keeps animating and accepting Ctrl-C until the decision answers.

**Do**:
- Add the decision's own message and command to `internal/tui/search_decision.go`: a `searchDecisionMsg` carrying the closure's `(name, error)` pair, and a `tea.Cmd` that runs the closure and returns it.
- Make the decision a third condition on the dismissal gate — `dismissLoadingGate` (`internal/tui/model.go:1508-1516`) returns that command with the page still on `PageLoading` when a decision closure is set and has not yet been dispatched, recording the dispatch on the model so a repeated `LoadingMinElapsedMsg` / `BootstrapCompleteMsg` pair cannot dispatch twice; `resolveSearchDecision`'s synchronous call and its clear-the-field guard go with it.
- Add the `searchDecisionMsg` arm to `Update`'s cross-view switch beside the `BootstrapCompleteMsg` arm: `m.fatalActive` returns early, a read failure records `searchErr` and returns `tea.Quit`, a named session sets `selected` + `searchAttached` and returns `tea.Quit`, and any other outcome runs the rest of today's dismissal — `transitionFromLoading`, then the `surfaceBufferedWarnings` / `refetchSessionsAfterRestore` / `maybeDispatchDetectionCmd` batch — so the sequence the helper held becomes gate ordering.
- Correct the doc comments that state the dropped rule: `resolveSearchDecision`'s "the closure runs synchronously … deferring it through a `tea.Cmd` would let the transition it is meant to pre-empt fire first" and its clearing-is-the-single-shot-guard note (`search_decision.go:29-37`), and `dismissLoadingGate`'s ordering note (`model.go:1504-1507`).
- Re-point the drivers that call the gate and read the result straight back — `internal/tui/search_decision_test.go`'s `transitionWithWarnings` path and `cmd/open_search_warnings_test.go`'s `searchTeardownModel` (`:55-74`) — so they run the dispatched command and feed its message into the model.

**Acceptance Criteria**:
- [ ] No `cmd`-supplied closure is invoked from inside `Update`: the decision runs only from a `tea.Cmd`.
- [ ] While the decision is in flight the page stays `PageLoading` and Ctrl-C quits with nothing selected.
- [ ] The closure is invoked exactly once across a duplicated `LoadingMinElapsedMsg` / `BootstrapCompleteMsg` sequence, and not at all before both gates are satisfied or after a `BootstrapFatalMsg`.
- [ ] An attach decision records `Selected()` and `SearchAttached()` and quits with no picker frame composed; a read failure records `SearchError()`, quits, and sets no fatal.
- [ ] A picker decision dismisses through today's sequence with the decision still ahead of `surfaceBufferedWarnings`: `WarningsOwedAtTeardown` returns the buffered set on the attach and read-failure exits and nothing on the picker exit.
- [ ] A decision message arriving after a fatal leaves the error frame standing.
- [ ] A model with no decision closure dismisses exactly as it does today, and `go test ./...` is green.

**Tests**:
- `"it holds the loading page while the dispatched decision is in flight"`
- `"it processes Ctrl-C while the decision is in flight"`
- `"it invokes the decision closure exactly once across a duplicated gate sequence"`
- `"it never invokes the closure before both gates are satisfied"`
- `"it quits on the named session an attach decision returns"`
- `"it records a read failure without raising a fatal"`
- `"it dismisses into the picker and surfaces the buffered warnings on a picker decision"`
- `"it ignores a decision message that arrives after a bootstrap fatal"`
- `"it leaves a model with no decision closure dismissing unchanged"`

## Task 4: Decide whether completion may offer a word for a line the validator will refuse
severity: medium
sources: standards

**Problem**: The search form composes with nothing — a second target, `-f`, a domain pin, `-e`, `--ack` on the same line are each a usage error. Completion knows about exactly one of those cases. `completeOpenPositional` (`cmd/completion.go:93-98`) gates its sigil arm on the word's shape and on the `--` separator bound, and on nothing else, so `x api /po<TAB>` offers `/portal-a1b2` for a line `validateSearchFormCollisions` (`cmd/open_search.go:77-107`) then refuses with `cannot use a /term search with another target`. The user accepts Portal's own suggestion, presses Enter, and is refused — which reads as the tool contradicting itself. The specification never settles the question: it states the offered set unconditionally and bounds the sigil arm in exactly one place, the separator, with a reason ("offering one there would complete to a term the parser will not honour") that applies verbatim to every other case the composition rule refuses. The 2026-09-15 corrigendum that added that bound did not extend it. So the code is faithful to the rule as written and cannot be called wrong against a rule nobody wrote, and the divergence is pinned twice — `cmd/completion_test.go:621-629` asserts it as intended, and cycle 5's approved Task 3 carried it as an acceptance criterion.

**Solution**: Completion completes the word in front of it. Settled to side 1 at the walk, on the asymmetry between the two bounds: past the `--` separator the word genuinely is not a search form and offering session names there also suppresses the filenames the user wanted, so that bound corrects two wrongs; before the separator the word *is* a search form and only the surrounding line is illegal, which the user may fix by deleting the other target before pressing Enter. Making the completer read the rest of the line to decide what one word may be is more than completing the word being typed.

The behaviour therefore stands as it is, and what the task lands is the record of it:
- The specification's completion section takes a dated correction stating the bound it writes is the only one — a search form is offered wherever one could be typed, the `--` separator is the single exception and why, and the validator is what refuses an illegal line. The corrigendum records that the reason given for the separator bound does not extend to the composition rule's other cases, and why.
- `completeOpenPositional`'s doc comment states the same, so a reader meeting `x api /po<TAB>` finds the behaviour accounted for rather than looking like an oversight.
- The existing pin at `cmd/completion_test.go:621-629` gains a sibling covering one non-positional case — a search form offered beside a set `-f` — so the rule is pinned as a rule rather than at one arity.

Left alone: `validateSearchFormCollisions` and its seventh derived arm, `completingPreDashPositional`, and the plain session-name completion at a second positional, which stays because multi-target bursts are legal and a plain name there is a real completion.

**Outcome**: What completion offers and what the validator refuses are two stated rules rather than one rule and one gap, so the line Portal helps assemble and then rejects is a documented consequence of completing the word in front of the cursor.

**Do**:
- Edit §8.1's separator-bound paragraph in `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` so it states the bound it writes is the only one: a search form is offered wherever one could be typed, the `--` separator is the single exception and why, and `validateSearchFormCollisions` is what refuses a line the composition rule (§5.1) does not allow.
- Append one entry to that file's `## Corrigenda` section in the standing form — `> **Corrigendum {YYYY-MM-DD}** (from `implementation/open-with-forced-filter`): {quoted claim} — corrected: …` — quoting §8.1's unconditional offered-set statement and the reason it gives for the separator bound, and recording that the reason does not extend to the composition rule's other cases: past the separator the word is not a search form and offering session names there also suppresses the filenames the user wanted, while before it the word *is* a search form and only the surrounding line is illegal, which the user may fix before pressing Enter. Take the date from `date` on this machine.
- Re-index the corrected file: `node .claude/skills/workflow-knowledge/scripts/knowledge.cjs index .workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md`.
- Extend `completeOpenPositional`'s doc comment (`cmd/completion.go:85-92`) with that same rule, so a reader meeting `x api /po<TAB>` finds the behaviour accounted for.
- Add the sibling pin beside `cmd/completion_test.go:621-629`, in `TestCompleteOpenPositionalSeparatorBound`: with `-f` set on the line, a pre-dash `/po` still takes the sigil arm and is offered `/portal-a1b2`.

**Acceptance Criteria**:
- [ ] `completeOpenPositional` and `validateSearchFormCollisions` are behaviourally unchanged: `portal __complete open api /po` still offers `/portal-a1b2`, and `portal open api /port` is still refused with `cannot use a /term search with another target`.
- [ ] §8.1 states the `--` separator as the single bound on the sigil arm and names the validator as what refuses an illegal line.
- [ ] The `## Corrigenda` section carries exactly one new entry, dated from this machine and attributed to `implementation/open-with-forced-filter`, quoting the claim it corrects.
- [ ] The re-index ran against that specification path, and the knowledge store's changes are committed with the task rather than left dirty.
- [ ] `completeOpenPositional`'s doc comment accounts for a search form offered on a line the validator refuses.
- [ ] The new test pins the `-f` case, so the rule is pinned at two arities rather than one.
- [ ] `completingPreDashPositional`, `validateSearchFormCollisions`'s seventh derived arm and the plain session-name completion at a second positional are untouched, and `go test ./...` is green.

**Tests**:
- `"it still offers sigil completions for a pre-dash word on a line carrying -f"`
- `"it still offers sigil completions for a pre-dash word beside another target"` — the existing pin, unchanged
- `TestValidateOpenArgs_RefusesSearchFormWithFilter` — the other half of the pair, unchanged and green
