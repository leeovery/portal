# Implementation Review: Open With Forced Filter

**Plan**: open-with-forced-filter
**Verdict**: Fail

## Summary

This is the second review cycle. The first covered all 58 tasks of phases 1–12 and sent two items back to planning; both became phase 13. One of them — the containment filter's map-key collision — was implemented and is verified here. The other — the `bash-completion` measurement task 5-5's shim edit was gated on — was cancelled, so that gate is still unrun and the shim still stands on a reading of cobra's and bash-completion's sources rather than on a measurement against the package a user actually has.

The delivered feature holds. Five change-set verifiers held the whole change-set against §1–§4, §5–§8, §9–§10, §11 and the test surface, each executing what the project's conventions allow; all five returned zero findings. `go build ./...`, `gofmt -l .`, `go vet ./...` and `golangci-lint run ./...` are clean over the whole tree (the lint config sets the `integration` tag, so the tagged files type-check too). The emitted `portal init bash` and `portal init zsh` shims were driven through real bash 5.3.15 and zsh over a recording stub and ask Portal for `open`'s completions; `portal open --help`, `portal open /term --help` and the README were rendered and checked line by line against what §9.2 obliges; the two committed capture frames were read as images and carry all five directory-column branches.

Task 13-1 itself is delivered: the per-key entry set, the every-entry ranking rule and the covering collision tests all hold, the edit stayed inside the two files it was scoped to, and its disagreement subtest runs both slice orders — which is what makes it a real regression guard rather than one that would have passed on the superseded last-wins map.

One finding sends the cycle back to planning. The withholding rule 13-1 installed is applied to the entries the *source* currently holds, not to the session the handed *target* came from, so the two conditions together — a filter pass running behind a list rebuild, and a filter value two distinct sessions can produce — still surface a row neither of whose fields contains the term. The doc comment at `internal/tui/search_filter.go:68-70` states the opposite as an unqualified guarantee. It arrived as a comment-accuracy nit with a comment-only remedy; the assessor re-aimed it to code, and synthesis found the code change is not one settled edit — at least three defensible shapes exist, the obvious one reverts the property task 12-2 was written to install and inverts a delivered test, and it still leaves a residual case open.

**No test suite was run in this cycle.** Every verifier read the rule that a change-set pass may run a single package only, and only to confirm a suspected defect, and none suspected one — so every "`go test ./...` passes" criterion across all 59 tasks is recorded below as not measured, alongside the integration lane, the `-race` run, the fish completion leg, the `bash-completion` measurement and the human visual gate. The first cycle did run the unit lane green over every touched package; nothing since has changed outside `internal/tui/search_filter.go` and its test file.

## QA Verification

### Specification Compliance

The change-set verification ran one agent per specification section plus one over the test surface, each holding the whole delivered change-set against its section and measuring what the project's own conventions allow. Every section returned zero findings. The coverage maps below are those agents' records verbatim; the `change-set-c2-*.md` files in this directory stay authoritative.

#### 1. Purpose and Scope + 2. The Sigil Form + 3. Resolution + 4. Match Domain and Matching Rule

- The recognition rule is exactly "leading `/` and no further `/`", filesystem-free — internal/resolver/path.go:25-27 — read: `strings.HasPrefix(arg, "/") && strings.Count(arg, "/") == 1`; every row of §2.2's table is pinned by `TestIsSearchSigil` (internal/resolver/path_test.go:109-165), including `/tmp/` → not a sigil and `./port`/`~/port`/`port` → unchanged
- A sigil is taken out of the path test and nothing else is — internal/resolver/path.go:14-16 — read: the `IsSearchSigil` early return sits ahead of the unchanged `strings.Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'`; `IsPathArgument` has exactly two production call sites (path.go:10 declaration, query.go:105), so the narrowing reaches only the bare-positional chain
- §2.4's escape still mints: `-p /tmp` does not consult `IsPathArgument` — internal/resolver/query.go:217-223 — read: `ResolvePathPin` calls `ResolvePath` directly
- The term is the text after the slash, empty for a bare slash — internal/resolver/path.go:31-33 — read: `strings.TrimPrefix(arg, "/")`, pinned by `TestSearchTerm`
- Recognition is positional-independent and bounded at `--` — cmd/open_search.go:17-34 — read: `searchFormPositionals` scans every pre-dash positional in argv order; `preDashPositionals` slices at `cmd.ArgsLenAtDash()`. `TestSearchFormPositionals` (cmd/open_search_test.go:41-64) covers sole/second/third/several positions and `TestSearchFormPositionals_StopsAtDashSeparator` covers both sides of the separator
- A sigil beside another target is a usage error, not a two-mint burst (§2.3) — cmd/open_search.go:83-96 — read: the `len(preDashPositionals(cmd, args)) > 1` arm refuses `portal open ~/Code/api /tmp`; the validator is `openCmd.Args` (cmd/open.go:160), and cobra runs `ValidateArgs` before `PersistentPreRunE`, so nothing bootstraps first
- A `/word` inside a trailing command is never a sigil (§2.3) — cmd/open_search.go:29-34 — read: `args[:dash]` excludes the command's own words; `portal open ~/Code/api -- ls /tmp` yields no form
- The bare slash is not a usage error and carries `-f`'s empty-value refusal nowhere near it (§2.5) — cmd/open.go:233-243 (`validateFilterFlag`) and cmd/open_search.go:223 — read: the empty-value refusal is gated on `cmd.Flags().Changed("filter")`, and `runSearchForm` returns on `term == ""` before any session read
- The term-less form lands with the filter focused and empty, on the whole live list (§2.5, §3.2) — internal/tui/model.go:1320-1335 — read: `state = list.Filtering` for an empty term, `SetFilterText("")` then `SetFilterState(Filtering)`; bubbles' `filterItems` short-circuits on an empty input value and returns every item, so nothing is hidden
- A term-less form takes no count — cmd/open_search.go:221-224 — read: the early return precedes `buildSearchSessionSource`, so no `ListSessionsProbe` or `CurrentSessionName` call is issued at all
- The sigil never enters the resolution chain (§3.1) — cmd/open.go:176-180 — read: the `searchFormPositionals` arm returns ahead of the `-f` arm, the multi-target gate, the pin dispatch and `qr.Resolve`, so a term carrying glob metacharacters is never read as an expandable target
- K = 1 attaches directly with no picker; every other count opens the picker (§3.2) — cmd/open_search.go:200-211, :234-248 — read: `searchDecision` answers `matches[0].Name` only at `len(matches) == 1` and `("", nil)` otherwise
- The attach uses the invocation's own connector, no third mode (§3.7) — cmd/open.go:112-114 and cmd/open.go:602-616 — read: `openSession` is `buildSessionConnector(tmuxClient(cmd)).Connect(name)`; the deferred route reaches the same `connector.Connect(selected)` through `processTUIResult`
- Portal's internal sessions are never search candidates (§3.2) — internal/tmux/tmux.go:143-148, :188-196 — read: `ListSessionsProbe` parses through the shared `parseSessionList`, whose underscore filter drops `_portal-saver` and `_portal-bootstrap` before any caller sees them
- The searched set is the set the picker lists (§3.2) — cmd/open_search.go:177-183 and internal/tui/model.go:1140-1145 — read: both route through `tui.PickerSessions` (internal/tui/picker_sessions.go:13-24), which drops the attached session and copies rather than deleting in place; outside tmux `currentPickerSession` returns "" before any read, so nothing is held back
- A failed session-list read is not a zero match (§3.7) — internal/tmux/tmux.go:143-148, cmd/open_search.go:203-205, cmd/open.go:606-611 — read: `ListSessionsProbe` wraps the `*CommandError` (whose `Error()` renders tmux's own stderr, internal/tmux/command_error.go:24-38); the warm route returns it from `RunE` and the cold route carries it as `SearchError`, checked after `FatalError` so it takes no in-TUI error frame
- The count is taken only once the whole bootstrap has completed (§3.4) — internal/tui/model.go:1509-1520, :1634-1644, :1693-1697 — read: `dismissLoadingGate` is reached only when both `minElapsed` and `bootstrapComplete` are set, dispatches the decision as a `tea.Cmd`, and returns `nil` for any repeat while `searchDecideInFlight`, so the loading page stands until `searchDecisionMsg` arrives
- The minimum loading span is untouched by the sigil (§3.4) — internal/tui/model.go:1634-1644 — read: `LoadingMinElapsedMsg` sets the flag and only then consults `bootstrapComplete`; the decision can never fire before the span has elapsed
- A decision answering after a bootstrap fatal never dismisses the error frame — internal/tui/model.go:1645-1651 and internal/tui/search_decision.go:51-63 — read: the `searchDecisionMsg` arm returns on `m.fatalActive`
- The sigil emits no `resolve` line (§3.6) — cmd/open.go:178-180 vs :219 — read: `emitResolveDecision` sits in the bare-positional arm, downstream of the sigil early return; pinned by `TestOpenCommand_SearchForm_EmitsNoResolveLine` (cmd/open_search_test.go:243)
- The matched fields are the name and the home-abbreviated recorded directory, and nothing else (§4.1) — internal/resolver/search.go:9-14 — read: `SearchFields` returns `[name]` for an empty dir and `[name, AbbreviateHome(dir)]` otherwise; `AbbreviateHome` (internal/resolver/path.go:94-106) never touches the filesystem
- The matching rule is case-folded containment with the fields tested separately (§4.3) — internal/resolver/search.go:22-36 — read: `strings.Contains(strings.ToLower(field), folded)` per field, no join; an empty term returns false rather than everything. `TestMatchesSearchTerm` (internal/resolver/search_match_test.go:10-137) pins case-folding both ways, the tilde the abbreviation introduces, the home segment it replaces, the cross-field run `"i /o"` → false, and `*`/`?`/`[` as literal characters
- A derived directory never reaches the match or the recorded value (§4.1) — internal/tui/grouping.go:16-21, internal/tui/model.go:267-272, :1206-1229 — read: `effectiveDir` reads `derived[s.Name]` without writing it back, the builders emit `SessionItem{Session: s, …}` with `s.Dir` untouched, and `resolveDerivedDirs` writes only into the grouping-only `m.derivedDirs` map
- The three filter entry points share one filter value, name then one space then the abbreviated directory, with no trailing separator when the directory is absent (§4.2, §4.3) — internal/tui/session_item.go:87-89 — read: `strings.Join(resolver.SearchFields(i.Session.Name, i.Session.Dir), " ")`, which is a one-element join when `Dir == ""`
- The searched form is the displayed form (§4.1) — internal/resolver/search.go:13 and internal/tui/session_dir_column.go:28 — read: both the match field and the rendered column go through `resolver.AbbreviateHome` on the same `Session.Dir`
- Containment holds while the committed text is character-identical to the term, and falls through to the picker's own rule for anything else (§4.4) — internal/tui/search_filter.go:74-77 — read: `if query != term { return list.DefaultFilter(query, targets) }`; a cleared filter never reaches the func at all, because bubbles' `filterItems` short-circuits on an empty input
- Containment narrows without reordering (§4.4) — internal/tui/search_filter.go:81-85 and bubbles/v2@v2.1.0/list/list.go:1251-1275 — read: ranks are appended in ascending target index and `filterItems` builds `filteredItems` in the order the Filter func returns, with no re-sort (unlike `DefaultFilter`, which calls `sort.Stable` itself at list.go:100)
- The first matching row is selected on landing (§3.2, §3.3) — internal/tui/model.go:1330-1334 and bubbles list.go:280-300 — read: both `SetFilterText` and `SetFilterState` call `GoToStart()`, and `ensureSessionRowSelected` steps off a header row
- Group headers vanish under a containment pass exactly as under the fuzzy one — internal/tui/search_filter.go:35-47, :82 and internal/tui/session_item.go:100 — read: `set` skips non-`SessionItem` rows, so a `HeaderItem`'s empty filter value is a key the source never holds and `len(held) > 0` fails
- A colliding filter value withholds every row behind it rather than ranking one (§4.4 carve-out) — internal/tui/search_filter.go:82, :92-99 — read: `everyEntryMatches` requires containment for every distinct `(name, dir)` pair recorded under the target, and `set` (:42-46) records one entry per distinct pair with `slices.Contains` deduping repeats of the same session across grouped rows
- A pass answering for a superseded generation omits rather than surfaces (§4.4) — internal/tui/search_filter.go:79-85 — read: a target the source no longer holds gives `len(held) == 0` and ranks nothing
- The containment source is written at the single list-rebuild chokepoint, before the items are handed over — internal/tui/model.go:1249-1254 — read: `m.searchItems.set(items)` precedes `m.sessionList.SetItems(items)`; `sessionList.SetItems` has exactly one call site in the package's non-test sources, inside `rebuildSessionList`, and `m.sessionList.Filter` is assigned exactly once (internal/tui/search_filter.go:108)
- The containment filter is installed for no picker but a search-opened one carrying a term (§4.4) — internal/tui/search_filter.go:103-108 — read: the `!m.searchForm || m.searchTerm == ""` guard leaves `list.DefaultFilter` in place everywhere else; `m.searchForm` is set only by `WithSearchForm`, which `Build` applies only when `deps.Search != nil` (internal/tui/build.go:132-134), and `Build` wires `InitialFilter` only when `deps.Search == nil` (:178-180)
- The source survives Bubble Tea's value copies and the filter goroutine — internal/tui/search_filter.go:28-31, :35-59 — read: `searchItemSource` is held by pointer on the model and its map is replaced under the write lock rather than mutated, so the map `current()` returns is safe to read unheld
- A search form lands on the Sessions page whatever the term matches, including zero (§3.2 K = 0) — internal/tui/model.go:1300-1306 — read: the `else if m.searchForm` arm precedes the `len(m.sessionList.Items()) > 0` test that would otherwise send an empty list to Projects
- A stale `argsLenAtDash` cannot mis-refuse a sigil line — pflag@v1.0.9/flag.go:1268, :1286 and cmd/root_test.go:67-71 — read: pflag resets the dash index only in `NewFlagSet`/`Init`, never in `Parse`, so a second parse in one process would leave a positive index; production parses `open`'s flag set once per process, and the suites' `resetRootCmd` re-`Init`s it between executions. Cobra's completion probe-parse (cobra@v1.10.2/completions.go:369) is the one double-parse, and it never runs `ValidateArgs`, so `preDashPositionals` is not reached with an out-of-range index there
- The delivered suites for this section exercise what they name, rather than asserting shape — cmd/open_search_test.go:186-742, internal/tui/search_containment_test.go:85-371, internal/tui/search_filter_test.go:39-234, internal/tui/search_decision_test.go:63-353 — read: each drives the real command body or the real `Update`/filter path through injected seams (`fakeSearchSource`, `recordingResolverSeams` counting chain consultations to prove none happened, `withFuncSeam` for `openTUIFunc`/`openSessionFunc`), and asserts observable outcomes — which seam ran, the landing carried, the ranked set, the attached name
- Toolchain state over the delivered tree — repo root — measured: `gofmt -l .` → no output; `go vet ./...` → clean; `golangci-lint run ./...` → `0 issues.`; `go build ./...` → exit 0

#### 5. Argv Composition + 6. Search Result Display + 7. Cold-Path Classification + 8. Tab Completion

- A refused line reaches no bootstrap — `cmd/open.go:160` (`Args: validateOpenArgs`) — measured: `$(go env GOMODCACHE)/github.com/spf13/cobra@v1.10.2/command.go` runs `ValidateArgs` at :968 and `PersistentPreRunE` at :985, so the refusal is decided before `EnsureServer`, restore or any frame (§5.1)
- `portal open /term --help` still prints help rather than being refused by the derived flag arm — cobra `command.go:926-934` returns `flag.ErrHelp` before `ValidateArgs` at :968 — measured: read of the execute() ordering (§5.1's carve-out)
- Every row of §5.1's table is refused by its own named arm, in one fixed order — `cmd/open_search.go:83-96` — read: second sigil (`len(forms) > 1`), second target (`len(preDashPositionals) > 1`), command (`ArgsLenAtDash() >= 0 || Changed("exec")`), `-f`, domain pin (`anyOpenDomainPin`), `--ack`
- Recognition is positional-independent and separator-bounded — `cmd/open_search.go:17-34` — read: `searchFormPositionals` scans every pre-dash positional, so `open api /term` and `open /term api` refuse alike, while a `/word` past `--` is the trailing command's
- A flag no arm names still refuses, so a future `open` flag cannot compose with the sigil silently — `cmd/open_search.go:103-105`, `:118-126` — read: `LocalFlags().VisitAll` filtered on `Changed` excludes inherited persistent flags (cobra `LocalFlags` skips `parentsPflags`), and `portal` registers no root persistent flags at all today (`grep -n "PersistentFlags()" cmd/*.go` → no production hit)
- `-f` keeps its own contract beside the new rule — `cmd/open.go:234-245`, `cmd/open_search.go:131-137` — read: `validateCommandScopeAndFilter` runs for every line the search arms passed, so `-f`'s target/pin exclusion and empty-value refusal are unchanged
- Every row in a sigil-opened list carries its recorded directory, in every grouping mode — `internal/tui/model.go:933-941` (`ShowDir: m.searchForm`), `:1231-1265` (`rebuildSessionList`, the single re-render chokepoint) — read: the delegate is rebuilt from the same constructor on regroup, refresh, marked-set mutation and restyle, and `searchForm` is set once at construction and never cleared (§6.1, §6.3)
- The displayed value is the recorded directory and never the grouping-derived guess — `internal/tui/session_item.go:267`, `:277` (both read `it.Session.Dir`), `internal/tui/model.go:1207-1228` (`resolveDerivedDirs` fills a separate grouping-only map) — read (§6.1)
- The directory begins exactly one space after the name, painted by the row background so the selected tint covers it — `internal/tui/session_item.go:286-289`, `:311` — read: `dirCell = bg.Render(" ") + …`, assembled as `name + dirCell + namePad` (§6.2)
- The directory takes the window count's token, one rung brighter on the selected row — `internal/tui/session_item.go:252-255`, `:288` — read: `countTok` is `TextMuted` / `TextSecondary`, and the same variable renders both the count and the directory (§6.2)
- The name never gives way to the directory — `internal/tui/session_item.go:271-278` — read: the name takes `max(total-used,1)` first, the directory only `remaining-1`, so a name filling its budget leaves the directory nothing
- Home-abbreviated at any width, then left-truncated on a segment boundary, then dropped below the floor — `internal/tui/session_dir_column.go:23-47` — read: `AbbreviateHome` precedes the width test; the left-to-right `/` scan returns the longest fitting `…/tail`; a width that cannot hold `…` plus one whole segment returns "" (§6.2)
- The rendered outcome at the capture geometry — `testdata/vhs/sessions-search-results.png` — measured: image read shows `~/code/portal`, `~/code/portal-gateway`, a bare `legacy-port-shim`, `…/testdata/reference/design-exports/frames` and an absolute `/opt/portal-tools`, with the directories in the count's muted rung
- The NO_COLOR row introduces no bracket, glyph or separator — `testdata/vhs/sessions-search-results-nocolor.png` — measured: image read shows the same single space with no substitute mark (§6.2's carve-out)
- A sigil invocation is classified as a picker invocation, on its shape — `cmd/root.go:173-178` — read: `len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0`, after the domain-pin veto; `open -- <command>` is the second member the corrigendum records (§7.1, §7.2, §7.6)
- The same verdict routes the soft warnings — `cmd/root.go:112-114` and `:146-148` — read: both the latch-satisfied and the full-bootstrap branches gate `bootstrapWarnings.EmitTo(stderr)` on `!isTUIPath`, so a sigil line writes nothing into the frame the picker claims (§7.2)
- The sigil takes the concurrent bootstrap and the loading page on a cold server — `cmd/root.go:189-194`, `cmd/open.go:636-643` — read: `shouldRunConcurrentBootstrap` is `isTUIPath && client != nil && !latchSatisfied`; `openTUI` starts the progress pipe and forces `serverStarted`, which parks the model on the loading page
- The count is deferred to the point the session list can answer it — `cmd/open_search.go:229-232`, `internal/tui/model.go:1509-1519`, `internal/tui/search_decision.go:40-63` — read: on the deferred route the closure rides the landing and is dispatched off the update goroutine by the loading gate, which holds `PageLoading` until it answers (§7.3)
- A K=1 sigil still delivers its warnings, after teardown and before the attach — `cmd/open.go:620-628` (`finishTUI` writes `model.WarningsOwedAtTeardown()` before `processTUIResult`), `internal/tui/model.go:486-487` — read: the accessor concatenates the buffered and staged sets, and both consumers clear their field (`internal/tui/bootstrap_warnings.go:46`, `:60`), so nothing is written twice and nothing an attach skipped is lost (§7.5)
- A warm single-match sigil writes them to the terminal itself — `cmd/open_search.go:238`, `:245` — read: both the failed-read and the attach branches drain the sink to `cmd.ErrOrStderr()` before returning (§7.5)
- The command-pending member holds its mint instead of gating the picker — `internal/tui/model.go:1844-1863` (stage while `bootstrapInFlight()`, first stage wins), `:1685-1692` (replayed through `mintSession` from `BootstrapCompleteMsg`), `internal/tui/notice_band.go:207-208` (the wait displaces the pick-a-project banner), `:1698-1710` (`BootstrapFatalMsg` quits a command-pending model) — read; both mint entry points route through `createSession` (`internal/tui/model.go:1963`, `:2881`), so neither bypasses the hold (§7.6)
- Completion looks past the sigil and keeps it on the candidate — `cmd/completion.go:70-83` — read: the term is `resolver.SearchTerm(toComplete)`, candidates are `"/" + s.Name`, and the directive is `ShellCompDirectiveNoFileComp` (§8.1)
- The offered set is the searched set, not the live enumeration — `cmd/completion.go:74` (`tui.PickerSessions(completionSessions(), completionCurrentSession())`), `internal/tui/picker_sessions.go:13-23`, `cmd/open_search.go:164-173` — read: the attached session is dropped inside tmux, and an empty/failed read drops nothing (§8.1 + its corrigendum)
- A slash-bearing session name is never offered — `cmd/completion.go:75` — read: `IsSearchSigil("/" + name)` counts slashes, so `foo/bar` fails the test while staying reachable by containment (§8.1)
- Completion is bounded by `--` and by nothing else — `cmd/completion.go:104-107`, `cmd/open_search.go:46-49` — measured: cobra appends a probe `--` at `completions.go:369` and pflag never resets `argsLenAtDash`, so a separator-free line reports `dash == len(args)` and the `<=` keeps it pre-dash; a word past a real separator answers with no candidates and `ShellCompDirectiveDefault`, leaving the shell's filenames on (§8.1 + the 2026-09-15 corrigenda)
- A line the composition rule refuses is still completed — `cmd/completion.go:95-103` — measured: cobra's completion path never calls `Args`/`ValidateArgs` (`completions.go:316-580` reaches `finalCmd.ValidArgsFunction` directly), so `open api /po<TAB>` reaches the sigil arm and `validateSearchFormCollisions` refuses only on Enter (§8.1)
- Tab after the session-opening function asks Portal for `open`'s completions — `cmd/init.go:65-73`, `:102-110` (bash), `:75-80`, `:168-176` (zsh), `:135-143` (fish) — measured: `go run . init bash` driven through bash 5.3.15 over a recording stub records `__complete|open|/po` for `x /po`, `__complete|open|` for `x `, `__complete|open|api|/po` for `x api /po` and `__complete|open|--|ls|/po` past a separator, rewriting `COMP_LINE` to `portal open /po` and `COMP_POINT` to 15; `go run . init zsh` driven through zsh records `__complete|open|…` for `x` and `__complete|` for `xctl` (§8.2, §8.3)
- The correction follows `--cmd <name>` rather than the literal `x` — `cmd/init.go:85`, `:105`, `:118`, `:135-143`, `:151`, `:171` — read: every emission interpolates `cmdName`, and the expansion is declared once at `cmd/init.go:54` (§8.3)
- A mid-line cursor is preserved by the bash shim's line rewrite — `cmd/init.go:69-70` — measured: line `x /po extra` with the cursor at offset 5 completed on `/po` (request `__complete|open|/po`) and left `portal open /po extra` with point 15
- `xctl` is untouched — `cmd/init.go:108`, `:141`, `:174` — read and measured: it still registers `__start_portal` / `_portal` / `-w portal`, and the zsh driver records a bare `__complete|` for it (§8.3)
- Tree unchanged after the pass — `git status --porcelain` → the two pre-existing entries only (`.workflows/…/manifest.json`, `report-13-1.md`); no build artefact left, all scratch files under the session scratchpad

#### 9. Documentation + 10. Out of Scope and Unchanged

- `portal open --help` states the form and its recognition rule — cmd/open.go:136-139 — measured: `go run . open --help` (isolated HOME/XDG) → "A positional beginning with / and containing no further / searches your live sessions instead of resolving:" followed by the `open /term` and `open /` examples
- The help gives the outcomes by match count — cmd/open.go:141-143 — measured (same run): "Exactly one match attaches that session outright; no match or several open the picker pre-filtered by the term — never an error", which is §3.2's three rows with the K=0 and K>=2 rows grouped on the outcome they share
- The help carries the term-less form — cmd/open.go:139 — measured: "open /          open the picker with the filter empty and ready to type"; the code matches (internal/tui/model.go:1320-1334 sets `list.Filtering` with empty text for an empty term, `list.FilterApplied` otherwise)
- The help states that the form composes with nothing — cmd/open.go:147-148 — read against cmd/open_search.go:77-105: the arms refuse a second search, another target, a command (`-e`/`--`), `-f`, a domain pin, `--ack`, and any other flag the line set
- The help distinguishes `-f` from `/term` by outcome and by use — cmd/open.go:150-152 — measured: one paragraph naming both, carrying "always opens the picker", "script or a keybinding" and "interactive form", which is §9.1's requirement verbatim in substance
- The help names the single-segment directory cost and the `-p` escape — cmd/open.go:144-145 — read: `-p` still mints there, because `ResolvePathPin` (internal/resolver/query.go:217-223) stats the literal path and never consults `IsPathArgument`
- The `-f` flag description stays a one-liner with no search wording — cmd/open.go:774 — measured: the rendered Flags block shows `-f, --filter string    open the picker pre-filtered by <text> (skips resolution)` on a single line
- The help is reachable on a sigil line, so the §9 text is not hidden behind the collision validator — cmd/open.go:160 — measured: `go run . open /term --help` → prints the Long, exit 0 (cobra returns `flag.ErrHelp` before `ValidateArgs`)
- README states the sigil form and the path shapes it leaves alone — README.md:143 — read against internal/resolver/path.go:25-27: `strings.HasPrefix(arg,"/") && strings.Count(arg,"/")==1` rejects `/tmp/`, `./port`, `~/port` and `/Users/me/Code/portal`, exactly the four the prose names
- README gives the three outcomes by match count — README.md:145-149 — read against cmd/open_search.go:221-249: K==1 hands the name to `openSessionFunc` with no picker, every other count opens the picker on the term
- README's "press `Esc` and your sessions are there" holds — README.md:149 — read: internal/tui/model.go:2576-2584 breaks out of the sessions-page Esc arm while `FilterState() == list.FilterApplied`, so the key reaches the list's own clear-filter instead of `tea.Quit`
- README carries the term-less form and that it is not an error — README.md:151 — read against internal/tui/model.go:1320-1334 (empty term ⇒ focused, empty filter; no error path exists)
- README states the match domain and that the picker's own filter stays fuzzy — README.md:145,153 — read against internal/resolver/search.go:9-35 (name and home-abbreviated recorded dir, tested separately, case-folded `strings.Contains`) and internal/tui/search_filter.go:103-109
- README states that the form composes with nothing and stops at `--` — README.md:157 — read against cmd/open_search.go:17-33 and :77-105; the README's own example `x ~/Code/api -- ls /tmp` is precisely the `preDashPositionals` bound
- README distinguishes `-f` from `/term` by outcome and names which to reach for — README.md:159 — read: matches §9.1's wording requirement
- The sigil row sits in the README's resolution table — README.md:170 — read: three columns, between the `-f, --filter` row (:169) and the `-e/--exec` row (:171), which is the pin table §9.2 names (`grep -n -- '-f, --filter' README.md` → :169)
- README states that completion after the session-opening function asks Portal for `open`'s completions — README.md:175 — measured: `go run . init bash` emits the `__start_portal_open` shim plus `complete -o default -F __start_portal_open x`; `init zsh` emits `_portal_open` plus `compdef _portal_open x`; `init fish` emits `complete -c x -f` then `complete -c x -w 'portal open'` — each routes the request to `completeOpenPositional` (cmd/open.go:784)
- README's offered-set claim for `/po<TAB>` matches the completer — README.md:175 — read: cmd/completion.go:70-82 offers `"/"+s.Name` over `tui.PickerSessions(...)`, which drops the attached session (internal/tui/picker_sessions.go:12-22), and `currentPickerSession` answers "" outside tmux or on a failed read (cmd/open_search.go:157-167), so nothing is held back there
- README's "a path argument … does not fall through to filename completion, which is the contract `portal open` has always had" is true of both trees — README.md:175 — read: `git show 6d6c38b:cmd/open.go` lines 730-732 already answered every positional with `completeSessionNames` + `ShellCompDirectiveNoFileComp`, and the new completer keeps that arm for every pre-dash word (cmd/completion.go:104-112, :32-40)
- README's bash 3.2 carve-out matches the emitted script and the 2026-09-14 corrigendum — README.md:175 — measured: the emitted bash registration is still `complete -o default -F __start_portal_open x`, and cobra's generated `compopt +o default` is gated on `type -t compopt`, so filenames still complete on a shell that cannot honour the directive
- README carries the rollout consequence (the correction reaches an install only once `portal init`'s output is re-evaluated) — README.md:175 — read
- No CHANGELOG entry was written — CHANGELOG.md — measured: `git diff --name-only 6d6c38b..811899223 | grep -i changelog` → no match
- §10.1 the bare-positional chain's ordering and domains are unchanged — internal/resolver/query.go:100-124 — measured: the range's diffstat lists no `internal/resolver/query.go`; the chain is still exact session → path → alias → zoxide → miss, and the sole edit to its path test is the sigil exclusion §2.2 requires (internal/resolver/path.go:10-17)
- §10.1's glob claim still holds — internal/resolver/glob.go:10 — measured: `grep -n 'globMeta =' internal/resolver/glob.go` → `const globMeta = "*?["`, untouched by the range
- §10.2 no `+` mint sigil was built — internal/resolver/path.go:25-32 — read: `IsSearchSigil`/`SearchTerm` are the whole sigil vocabulary added, and nothing in the range recognises any other leading character
- §10.3 no configurable sigil, and prefs still carries UI state only — internal/prefs/store.go:70-78 — measured: `git diff --name-only` over the range touches no file under `internal/prefs`; the fields remain `session_list_mode`, `appearance`, `theme`, `theme_light`, `theme_dark`, `theme_migrated`
- §10.4 session naming untouched — internal/session — measured: no file under `internal/session` appears in the range's diff, so `{project}-{nanoid}`, creation and rename are as they were
- §10.5 the domain pins keep their behaviour exactly — cmd/open.go:176-204 — read: the search arm runs only for a positional carrying the sigil (`searchFormPositionals`, cmd/open_search.go:17-24); a pin's flag value never passes through `IsSearchSigil`, so `-p /tmp` still stats and mints
- §10.5 nothing is retired, renamed or deprecated — README.md:164-171 — read: all seven pre-existing table rows survive unchanged, with the sigil row added beneath `-f` and the header widened to "Flag / form"
- §10.5 the widened match domain is the sanctioned change and reaches `-f` and the hand-typed filter alike — internal/tui/session_item.go:87-89 — read: `FilterValue()` joins name and home-abbreviated recorded dir for every picker, which is §4.4's row for both entry points
- §10.6 the rows of a picker reached any other way are untouched — internal/tui/session_item.go:115-123, :260-311 and internal/tui/model.go:938 — read: `ShowDir` is set from `m.searchForm` alone (the one production assignment in the tree — `grep -rn 'ShowDir' --include='*.go' . | grep -v _test` returns the declaration, its two uses and that assignment); with it false `dirText` stays empty, `remaining` is the old `nameWidth - width(visibleName)` and the assembled row is character-identical to the pre-change one
- §10.6 the picker's own filter rule is untouched — internal/tui/search_filter.go:103-109 — read: `installSearchFilter` returns before assigning `sessionList.Filter` unless a sigil supplied a non-empty term, so every other list keeps `list.DefaultFilter`
- §10.7 the match domain was not widened beyond name and directory — internal/resolver/search.go:9-14 — read: `SearchFields` returns the name and the recorded dir only; no project record, tag or alias is consulted on the search path
- The architecture note for `open` was updated to carry the sigil and matches the delivered parser — CLAUDE.md:37 — read against cmd/open_search.go:17-24 and :77-105
- Repo-wide gates over the delivered tree — repo root — measured: `gofmt -l .` → no output; `go vet ./...` → exit 0; `golangci-lint run ./...` → `0 issues.`; `go build ./...` → exit 0
- The tree was left as found — repo root — measured: `git status --porcelain` shows only the pre-existing manifest edit and other agents' in-flight review files; no tracked file modified, no build artefact written into the repo, no tmux server or daemon started

#### 11. Relationship to `cli-verb-surface-redesign`

- The sibling specification carries the search-sigil pre-check and exactly one dated corrigendum naming this work unit — .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md:51,:478 — measured: `grep -n 'open-with-forced-filter' <sibling spec>` → two hits, the numbered step 1 of "Target resolution precedence" and a `Corrigendum 2026-09-11` entry beneath the `## Corrigenda` heading at :464
- The correction was applied once and not duplicated by this change-set — .workflows/cli-verb-surface-redesign/ — measured: `git log --oneline <range> -- .workflows/cli-verb-surface-redesign/` → empty (the sibling was edited before this range), and `grep -c 'Search-sigil pre-check' <sibling spec>` → 1
- The sibling's step-1 text matches the delivered recognition rule verbatim in substance — internal/resolver/path.go:25-27 — read: `IsSearchSigil` is `strings.HasPrefix(arg, "/") && strings.Count(arg, "/") == 1`, which is the sibling's "begins with `/` and contains no further `/`"
- The path test in the sibling's step 3 is narrowed by exactly that shape and nothing else — internal/resolver/path.go:10-18 — read: `IsPathArgument` returns false for a sigil and is otherwise the unchanged `Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'`
- `/tmp/` and `/Users/leeovery/Code/portal` remain path targets, `/port` and `/` do not — internal/resolver/path_test.go:11-107, :109-165 — read: both tables enumerate the same six shapes with opposite verdicts, so a widening of either rule reddens one of them
- A multi-segment path positional still mints end-to-end — cmd/open_search_test.go:269-296 — read: the test fails the run if `openTUIFunc` is reached and asserts the minted path equals the temp dir
- A sigil positional never enters the precedence chain — cmd/open.go:178-180 — read: `searchFormPositionals` returns through `runSearchForm` ahead of the `-f` arm (:184), the multi-target gate (:193-196), the pin dispatch loop (:200-204) and `qr.Resolve` (:217)
- The sigil pre-check sits ahead of the glob one, as the sibling's numbering states — cmd/open.go:175-180 vs :193-196 — read: a term carrying `*`/`?` is dispatched to the search before `isMultiTarget` can read it as a K≥2-expanding target, and `MatchesSearchTerm` (internal/resolver/search.go:22-35) treats it as literal text via `strings.Contains`
- `-p /tmp` — the escape the narrowing owes the user — survives it — internal/resolver/query.go:217-223 — read: `ResolvePathPin` calls `ResolvePath` directly and never consults `IsPathArgument`, so a single-segment absolute directory still stats and mints under the pin
- The pinned-domain contract is intact: every pin hard-fails on a miss — internal/resolver/query.go:206-212 (`No session found`), :217-223 (`ResolvePath` error), :228-234 (`No alias found` / `*DirNotFoundError`), :239-248 (`ErrZoxideNotInstalled` / `No zoxide match for`) — read: each returns an error; none returns a picker result
- No pin can reach the picker — cmd/open.go:186, :207 and cmd/open_search.go:223, :231, :248 — read: those are the five `openTUIFunc` call sites in production code (`grep -rn 'openTUIFunc' cmd | grep -v _test`), and the pin dispatch loop at cmd/open.go:200-204 returns through `resolvePinAndOpen` before any of them
- A bare-positional total miss still hard-fails rather than falling back to the picker — cmd/open.go:224-226 and cmd/open_burst.go:65-67 — read: `*MissResult` returns `singleMissError`, which is the sibling's own `nothing resolved for '%s' — try -f %s`
- Axiom 2 is ratified, not changed: the sigil never mints — cmd/open_search.go:200-211, :221-248 — read: the single-match branch calls `openSessionFunc` (attach) and every other count calls `openTUIFunc`; there is no mint route on the form
- The sigil's searched set is the user-visible one, so `_portal-saver`/`_portal-bootstrap` are never matchable — internal/tmux/tmux.go:143-149 feeding :151-197 — read: `ListSessionsProbe` shares `parseSessionList` with `ListSessions`, whose :188-196 tail drops every `_`-prefixed name
- The searched set is the set the picker lists — cmd/open_search.go:177-183 and internal/tui/picker_sessions.go:13-24 — read: `searchCandidates` runs the enumeration through `PickerSessions`, which drops the attached session and returns the caller's own slice when there is none
- `-f` keeps its non-composing contract byte-for-byte — cmd/open.go:235-248 and cmd/open_search.go:131-137 — read: `validateFilterFlag` carries both refusals ("cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)", "-f/--filter value must not be empty") unchanged; the move to the `Args` validator changes when they fire, not what they say
- A refused line starts no tmux server — cobra v1.10.2 command.go — measured: `sed -n '/^func (c \*Command) execute(/,/PersistentPreRun/p' $GOPATH/pkg/mod/github.com/spf13/cobra@v1.10.2/command.go | grep -n` → `ValidateArgs` precedes the `PersistentPreRunE` loop, which is what `validateOpenArgs`'s doc comment (cmd/open_search.go:60-67) asserts
- Moving the collision checks into `Args` does not break completion of an illegal line — cobra v1.10.2 completions.go — measured: `grep -n 'ValidateArgs' completions.go` → no match, so `__complete` never runs the validator and `open api /po<TAB>` still offers a term `validateSearchFormCollisions` refuses on Enter, as the 2026-09-15 corrigendum requires
- The tab-completion principle is ratified: Portal's namespaces completed, the rest left to the shell — cmd/completion.go:104-112 — read: the sigil arm and the session-name arm both return Portal-owned candidates with `ShellCompDirectiveNoFileComp`; a post-separator word returns no candidates and `ShellCompDirectiveDefault`, which is the shell's job rather than a Portal namespace
- The sigil emits no `resolve` component line — cmd/open.go:222 and cmd/open_surfaces.go:37 — read: those are the only two `emitResolveDecision` call sites (`grep -rn 'emitResolveDecision' cmd | grep -v _test`), both downstream of `qr.Resolve`, which the sigil returns before reaching
- Post-`--` words are never targets and never sigils, on both readings of the line — cmd/open_targets.go:41-44 and cmd/open_search.go:29-34 — read: the raw-argv walk breaks at `--` and `preDashPositionals` truncates at `cmd.ArgsLenAtDash()`, so the two cannot disagree about which words are targets
- A domain-pinned invocation is still classified non-TUI, so a spawned window's warnings go to stderr rather than a frame — cmd/root.go:173-178, :182-184 — read: `isTUIPath` returns false on `anyOpenDomainPin`, which reads `openDomainPinFlags` so a new pin cannot be omitted
- The tree builds and every declared linter is clean over it — repo root — measured: `go build ./...` → exit 0; `gofmt -l .` → no output; `go vet ./...` → no output; `golangci-lint run ./...` → `0 issues.`
- The working tree was left as found — repo root — measured: `git status --porcelain` → the same two pre-existing entries (`M .workflows/open-with-forced-filter/manifest.json`, `?? …/report-13-1.md`) and nothing else

#### test surface — the change-set's test files, held against everything the specification asks them to guard (§1–§11 plus the seven corrigenda)

- §2.2's recognition rule is pinned on the shape alone, positively and negatively, and never consults the filesystem — internal/resolver/path_test.go:109 (`TestIsSearchSigil`) + :368 (`TestAbbreviateHome`) — read: `/port` and `/` true; `/Users/leeovery/Code/portal`, `/tmp/`, `./port`, `~/port`, `port`, `""` false; `IsPathArgument` re-asserted over the same table so the narrowing of the path test is guarded from both sides
- §2.2/§2.4 — a single-segment leading-slash argument reaches the resolver's miss rather than its path domain, and gives the same verdict whether or not the directory exists — internal/resolver/query_test.go:291 (`TestQueryResolver_Resolve_SearchSigil`) — read: `/tmp` and `/definitely-not-here` both `*MissResult`; `-p /tmp` still a `*PathResult` in `DomainPath`
- §2.3 — recognition is positional-independent and stops at `--` — cmd/open_search_test.go:41 (`TestSearchFormPositionals`) + :67 (`_StopsAtDashSeparator`) — read: sigils found at first, second and third positional and in argv order; `~/Code/api -- ls /tmp` yields nil while `/port -- ls /tmp` still yields `/port`. Driven through a real cobra command so `ArgsLenAtDash` carries the true separator index rather than a hand-set value
- §2.5/§3.3 — the term-less form lands on a focused, empty sessions filter with every session visible, and narrows on the first typed character; `s` is a literal filter character there — internal/tui/search_landing_termless_test.go:13 — read: `FilterState() == list.Filtering`, `FilterValue() == ""`, mode unswitched after typing `s`, and a zero-session model renders without panic
- §3.2/§3.3 — a term lands on Sessions with the filter committed (`FilterApplied`) and the cursor on the first surviving row, in Flat, By Project and By Tag, with the `-f` and command-pending landings left unchanged — internal/tui/search_landing_test.go:42 — read: the By-Project/By-Tag arms additionally assert the selection is a `SessionItem` and not a `HeaderItem`
- §3.2's K=1 / K=0 / K>=2 outcomes at the command layer — cmd/open_search_test.go:510 (`_AttachesTheSingleMatch`), :541 (`_OpensPickerWhenNothingMatches`), :560 (`_OpensPickerWhenTwoOrMoreMatch`) — read: K=0 asserts `err == nil` *and* an empty stderr buffer, which is the "never fails" half; K=1 asserts the picker seam was not reached at all
- §3.2 — a directory-only match is still a match and still attaches outright — cmd/open_search_test.go:524 — read: `api-work` recorded under `$HOME/Code/portal` attaches on `/portal` while `blog-c3d4` does not
- §3.2 — the searched set is the picker's set: internal sessions never counted, the attached session excluded, a failed or empty current-session read dropping nothing, and no read at all outside tmux — cmd/open_search_test.go:608, :623, :638, :653 + internal/tui/picker_sessions_test.go:18 — read: `PickerSessions` also asserted to leave the caller's slice unmodified, which is what the in-place-delete comment forbids
- §3.2's "no filter of its own on top" — the count is exactly the enumeration's, taken with one `ListSessionsProbe` call — cmd/open_search_test.go:707 — read: a seeded `_portal-saver` row reaches the count, proving the filtering lives in the enumerator rather than in `cmd`
- §3.2's internal-session exclusion at its real home, plus the probe/list parity the count depends on — internal/tmux/list_sessions_probe_test.go:23, :62, :131 — read: `_portal-saver`/`_portal-bootstrap` filtered; both readers parse identical output including an embedded `|` in the directory; `TestListSessionsProbe_IssuesTheSameFormatAsListSessions` compares the two recorded argvs, so a format drift that would strand `@portal-dir` outside the trailing `SplitN` slot fails
- §3.4 — the decision is held until BOTH loading gates, issued exactly once, never on a progress event, never after a fatal, and never on the update goroutine — internal/tui/search_decision_test.go:63–:353 — read: `_HeldUntilMinimumSpanElapses`, `_HeldUntilBootstrapCompletes`, `_NotIssuedOnProgress`, `_NotIssuedAfterBootstrapFatal`, `_IssuedExactlyOnce` (a duplicated gate sequence), `_HoldsLoadingPageWhileInFlight` (calls == 0 until the returned command is run) and `_MessageAfterFatalLeavesErrorFrameStanding`
- §3.4 against a real ten-step bootstrap — cmd/concurrent_search_decision_integration_test.go:167, :227 — read: the probe records `stepsAtCall`, `completeAtCall`, `IsRestoringSet` and its own `ListSessionsProbe` at the instant the decision runs, and `assertStandingOnLoadingPage` re-asserts `PageLoading` + `calls == 0` after every real step event, so acting at end-of-restore fails rather than passing. Properly isolated: `setupConcurrentColdBootEnv` calls `portaltest.IsolateStateForTest`, `RegisterStateDirTeardownGuard` and `tmuxtest.New` in the CLAUDE.md-mandated order, and the file is `//go:build integration`
- §3.6 — no `resolve` component line at any count — cmd/open_search_test.go:243 — read: a `logtest.Install` sink queried with `Matching("resolve", "resolved")` across the no-match, one-match and two-match cases
- §3.7 — the attach reuses `openSessionFunc` (the invocation's own connector) and `processTUIResult` connects on the decided name — cmd/open_search_deferred_test.go:244 — read: the read-failure arm additionally asserts the error is neither a `*UsageError` nor a `*bootstrap.FatalError` and that the connector was never called
- §3.7 — a failed session-list read is an ordinary non-zero failure carrying tmux's own words, not a zero match — cmd/open_search_test.go:724 — read: the error contains `no server running`, is not a `*UsageError`, and neither the picker nor an attach follows it
- §4.1 — the matched fields are name + home-abbreviated recorded directory, tested separately and never joined; the term is literal text — internal/resolver/search_match_test.go:10, :139 and internal/resolver/search_fields_test.go — read: `~/code` matches the abbreviation while `filepath.Base($HOME)` (which IS present in the raw path) does not, which is what makes the abbreviation load-bearing rather than decorative; `i /o` spans the joined fields and is asserted not to match, with a `strings.Contains` precondition proving the span is real; `po*`, `po?t`, `po[rs]t` all non-matching; an unresolvable `$HOME` falls back to the raw path
- §4.1's separation invariant — a grouping-derived directory never lands in the recorded one and is no part of the matched text — internal/tui/rebuild_dir_resolution_test.go:49, :233 — read: `m.sessions[0].Dir` and `SessionItem.Session.Dir` both asserted `""` after a By-Project and a By-Tag derive; the derived value lands in `m.derivedDirs` only; a filter set to the derived key leaves zero visible items; an unresolvable session is not negative-cached; a refresh discards the cache (second `applySessions` re-reads exactly once)
- §4.1's separation invariant re-armed in the two "zero pane reads" probes that previously cleared `Session.Dir` — internal/tui/apply_theme_test.go:49 and internal/tui/theme_panel_arrow_test.go:442 — read: both now clear `m.derivedDirs` instead, so the "zero reads" assertions stay non-vacuous under the new field
- §4.2 — the widened fields reach all three entry points, and the home abbreviation keeps the account name out of the match — internal/tui/model_test.go:2825, :2846, :2873 — read: `-f portal` and a hand-typed `/portal` each narrow to the directory-only session; `-f leeovery` matches nothing
- §4.2/§4.3's joined form — `FilterValue()` is exactly `resolver.SearchFields(...)` joined by one space, with no trailing separator for a session carrying no directory, and independent of the group fields — internal/tui/session_item_test.go:15, :101 — read
- §4.3/§4.4 — containment narrows without re-ranking, diverges from the picker's fuzzy rule, and holds across grouping modes — internal/tui/search_containment_test.go:85, :109, :214 — read: the `rust-tools` fixture (a genuine `p·o·r·t` subsequence that contains no run of `port`) survives `-f` and is dropped by the sigil, so the two rules are separated by a case that actually distinguishes them; order assertions use a fixture where fuzzy's rank order is the reverse of the list order
- §4.4 — the containment set is reproduced after an `s` regroup, a `Space` preview and back, and a `SessionsMsg` refresh that drops a killed session — internal/tui/search_containment_test.go:247 — read: each re-render asserts the survivors *and* that no group heading is visible
- §4.4 — the rule is keyed on the filter text, not the act: an edit returns to fuzzy, editing back restores containment, clearing shows everything — internal/tui/search_containment_test.go:312 — read
- §4.4's collision carve-out (2026-09-16 corrigendum) — where two sessions render to one filter value and disagree about the term, every row behind it is withheld; where both contain it, both rank — internal/tui/search_filter_test.go:200 — read: the fixture is a session literally named `api ~/Code/api` beside a session `api` recorded at `$HOME/Code/api`, with a `targets[0] != targets[1]` precondition proving the collision, driven in both orders
- §4.4's stale-generation and repeated-row properties of the filter — internal/tui/search_filter_test.go:39, :58, :76, :88, :103 — read: ranks index the targets the pass was handed (asserted against a generation the source never held, in an order it never held); a By-Tag session repeated under two tags ranks both rows; a target the source does not hold ranks nothing; a header's empty filter value ranks nothing; `MatchedIndexes` asserted nil
- §4.4's race surface — internal/tui/search_filter_test.go:168 — read: 500 concurrent `set` calls against 500 filter passes. Measures a race only under `-race`, which neither documented lane carries; Go's always-on concurrent-map-access check gives partial coverage without it
- §5.1's whole refusal table, each arm by its own message — cmd/open_search_test.go:333, :352, :358, :377, :383, :393, :410, :427 — read: another target at three arities and in both orders, a second sigil, `-e`, `--`, a bare `--`, `-f`, each of the four domain pins, `--ack`; plus `TestValidateSearchFormCollisions_RefusesAFlagNoNamedArmCovers` (fail-closed on an unnamed flag) and `_AdmitsASearchFormThatSetNoFlag`
- §5.1's "a refused line starts nothing" — cmd/open_search_test.go:435 and :790 (`executeOpenExpectingPreBootstrapUsage`) — read: a `recordingRunner` asserted at 0 calls and every resolver seam asserted unconsulted, with the picker seam wired to `t.Error`
- §5.1's carve-out for flags that answer first, and the pre-existing refusal precedence — cmd/open_search_test.go:450, :884, :921 — read: `open /port --help` still prints usage; a search form outranks an empty `-f` while an empty `-e` still outranks an empty `-f`; `--ack` malformed still refuses from the command body *downstream* of bootstrap (runner asserted at 1 call), which pins the one refusal that deliberately did not move
- §6.1/§6.3 — the column is on for a sigil-opened picker (term and term-less) in all three modes, off for `-f` and for a no-filter picker, and survives regroup, preview-and-back, refresh, a marked-set mutation, a theme swap, a filter edit and a filter clear — internal/tui/search_dir_column_test.go:92 — read: the regroup arm asserts `m.sessionListMode` moved, so a swallowed `s` fails rather than passing on an unchanged frame
- §6.1's "only the recorded directory is displayed" and "no pane read" — internal/tui/search_dir_column_test.go:246, :264 — read: a session whose directory is known only to `derivedDirs` renders none, with a precondition asserting the derive actually happened; a `fakeStamper` asserted at zero reads
- §6.2's placement, weight, truncation ladder and floor — internal/tui/session_row_anatomy_test.go:308, :435, :472, :527 — read: one space after the name and nothing else; the count's own colour token; the same single space with colour off (no bracket, glyph or separator standing in); dropped below the floor; the name never truncated in the directory's favour; the row width exactly the list width at every branch
- §6.2's fit function in isolation — internal/tui/session_dir_column.go:23 / internal/tui/session_dir_column_test.go:10, :121, :147, :161 — read: 15 table cases plus a width-sweep asserting every returned value is a whole separator-anchored tail of the abbreviated path and never wider than the budget, plus a CJK case proving display width rather than byte length
- §6.1/§6.2 at the render layer, as a captured fixture in the swap-and-diff completeness guard — internal/capture/fixtures.go:478 (`sessionsSearchResultsFixture`) + internal/capture/capture_test.go:1026, :1039 + internal/capture/swap_harness_test.go:65 — read: all five column branches (abbreviated home path, directory-only match, empty slot, `…/` left-truncation, unabbreviated `/opt` path) plus a non-matching session asserted absent; `fixtureHome()` reads `os.UserHomeDir()` per `FixtureByName` call, so the arbitrary-`$HOME` subtest genuinely re-derives
- §7.1/§7.2 — a search form at any positional index classifies as the TUI path, identically to `-f`; a path positional, two targets, a post-`--` `/word` and a search form beside a domain pin do not — cmd/concurrent_bootstrap_gate_test.go:28 — read: the `-f` parity subtest compares the two verdicts rather than restating a constant, so a drift in either direction fails
- §7.1's warning routing — a search-form line writes nothing to stderr and leaves the sink buffered for the picker — cmd/bootstrap_warnings_test.go:277 — read: `bootstrapWarnings.Drain()` asserted at 1 remaining
- §7.5 — the warm attach and the warm failed read each write the accumulated warnings, byte-identically to the CLI path, before the connector runs, and drain the sink so nothing is written twice — cmd/open_search_warnings_test.go:75, :193, :321 — read: `stderrAtAttach` is captured *inside* the `openSessionFunc` seam, so an ordering regression fails; `WarningsOwedAtTeardown` covers attach, failed read, warm picker, a surfaced buffer (owes nothing), a cancelled loading page and a cancel with the decision still in flight
- §7.6's command-pending hold (2026-09-15 corrigendum) — the pick is staged rather than minted while the bootstrap is in flight, replayed from the terminal event, first stage wins, the band announces the wait, a fatal mints nothing, and the warm route still mints immediately — internal/tui/command_pending_staged_mint_test.go:49, internal/tui/staged_mint_band_test.go:24, internal/tui/command_pending_bootstrap_test.go:58, cmd/open_command_pending_warnings_test.go:46 — read
- §8.1's completion rule — the term after the slash, the slash kept on the candidate, prefix-shaped, `/<TAB>` offering the whole searched set, the attached session held back, a slash-bearing session name held back, and nothing offered on a failed read — cmd/completion_test.go:350 — read: every assertion is an exact `slices.Equal` against the expected candidate list, so an over-offer fails as loudly as an under-offer
- §8.1's separator bound and its one exception (2026-09-15 corrigenda) — a post-separator word gets no candidates and `ShellCompDirectiveDefault` (so the shell's filenames survive), while a pre-separator word keeps the sigil arm even beside another target, `-f` or a pin, and the boundary word immediately after `--` keeps it too — cmd/completion_test.go:586 — read: the directive is decoded from the real `__complete` output rather than asserted on the candidate list alone, which is what makes the "filenames survive" half real
- §8.1's non-regression — `--session`'s flag completer, `kill`'s positional completer and a second `open` positional all stay on plain session names — cmd/completion_test.go:514 — read
- §8.3's correction, driven through the real emitted scripts — cmd/init_completion_shell_test.go:317, :353, :366, :378, :403 — read: the recorded stub request is asserted to be `__complete|open|<word>` for bash and zsh (fish skips: not installed), `xctl` is asserted unchanged at `__complete|<word>`, `--cmd p` is followed, the offered `/portal-a1b2` reaches `COMPREPLY`, and the file-completion switch-off is asserted on cobra's own debug channel per shell. No portal binary is built or executed — a shell-script stub shadows `portal` on a temp `PATH` — so the unit lane stays pure per CLAUDE.md; the shells run `--noprofile --norc` / `-f` / `--no-config` under a temp `HOME`
- §8.3's bash cursor handling — cmd/init_completion_shell_test.go:430, :442, :452, :461 — read: a mid-line cursor completes the word under it, `COMP_POINT` shifts by the expansion delta rather than pinning to end-of-line, the user's own spacing survives the rewrite, and a line with leading whitespace still completes. Note the environment dependency: `exec.LookPath("bash")` resolves `/opt/homebrew/bin/bash` 5.3.15 here, which is what makes the `compopt` switch-off assertion hold; the specification's own corrigendum records that macOS's stock `/bin/bash` 3.2 cannot perform it
- §9.2's `open --help` points, asserted as keywords rather than a golden string — cmd/open_help_test.go:10 — read: the recognition rule, the term-less form, the outcomes by count, the match domain, the composition rule, the single-segment cost and `-p /tmp`; the `-f` / `/term` pair is required to sit in ONE blank-line-separated paragraph naming both, with `always opens the picker`, `keybinding` and `interactive` — which is the "described by outcome" requirement rather than a token count. `TestOpenFilterFlagUsage_StaysAOneLiner` pins the negative half
- §9.2's README points — cmd/open_docs_test.go:80 — read: the assertions are anchored to the open section, to its first fenced block (so prose carrying `/port` cannot satisfy the example assertion) and to a line *beginning* `| \`/<term>\`` (so prose cannot satisfy the table row). The README's own text was read and carries every §9.2 point including the rollout consequence, which the test guards only by the weaker `portal init` token
- §10.5 — the multi-segment path positional still mints — cmd/open_search_test.go:269 — read: the picker seam is wired to `t.Error`
- The two-site terminal-background restore contract survives the new `finishTUI` hop — internal/tui/restore_source_guard_test.go:103, :162 — read: `writerArgs` resolves a writer parameter one hop out to its callers' arguments, `paramIndex` yields 2 for `canvas` in `finishTUI(model, connector, canvas, warnings)` (cmd/open.go:620) and the single caller at cmd/open.go:753 passes `os.Stdout`, so the guard still resolves to a real writer rather than to a parameter name; the vacuity tripwire in `TestRestorePath_ReadsNoTheme` is untouched
- Lane and isolation discipline across the whole added surface — read + run: `git diff … -- '*_test.go' | grep t.Parallel` → none; no `os.Setenv`, no `tmux.DefaultClient()` and no hand-rolled `go build` added in any test; the four integration-tagged files all carry `//go:build integration` on line 1; the three added `exec.Command` calls spawn only `bash`/`zsh`/`fish` with their rc files disabled, under a temp `PATH` whose first entry shadows `portal` with a shell stub
- Build and linters over the delivered tree — repo root — measured: `go build ./...` → exit 0; `gofmt -l .` → no output; `go vet ./...` → exit 0; `golangci-lint run ./...` → `0 issues.` (config sets `build-tags: [integration]`, so the integration-lane test files are type-checked in the same pass)
- The four `testdata/vhs/sessions-search-results*` artefacts are scaffolding under CLAUDE.md's retention table, not the kept `reference/` carve-out — testdata/vhs/ — read: both tapes open with an explicit "SCAFFOLDING, NOT AN ASSET … CLEARED OUT at the feature's sign-off" header naming the permanent `internal/capture` fixture as the part not covered by that clearance

### Plan Completion

- [x] Phase 1–13 acceptance criteria met, except those named below as not measured
- [x] All tasks completed or deliberately discarded — 59 of 60 tasks completed; **task 13-2 (`Run the bash-completion measurement the init.go shim edit was gated on`) was cancelled**, so the gate task 5-5 set on its own `cmd/init.go` edit has still never been run, and the shim ships on a derivation rather than a measurement
- [x] No scope creep — task 13-1's edit touched only `internal/tui/search_filter.go` and `internal/tui/search_filter_test.go`, and `git diff 2fe01e014 HEAD` over those paths is empty

**Criteria not measured**

[1-1]
- "`go test ./...` passes" — the unit lane is a whole-suite sweep; every section recorded it NOT MEASURED, the change-set pass being admitted a single package only and only to confirm a suspected defect. None suspected one.

[1-2]
- "`go test ./...` passes" — same. The `SessionItem.FilterValue()` widening reaches every picker in the tree, so its effect on pre-existing filtering suites is a suite result.

[1-3]
- "`go test ./...` passes" — same.

[1-4]
- "`go test ./...` passes" — same.

[1-5]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — the pass/fail half of both lanes. Compilation of the integration-tagged files is settled (`golangci-lint run ./...` sets `build-tags: [integration]` and reported `0 issues.`); the integration lane additionally needs `-p 1` and a real tmux server.

[1-6]
- "`go test ./...` passes" — same as [1-1].

[1-7]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].

[2-1]
- "`go test ./...` passes" — same as [1-1].

[2-2]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].

[2-3]
- "`go test ./...` passes" — same as [1-1]. The named `go test -race ./internal/tui` run is likewise a package run no section took; neither documented lane carries `-race`, so the detector half runs only when a caller supplies the flag.

[2-4]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].

[3-1]
- "`go test ./...` passes" — same as [1-1].

[3-2]
- "`go test ./...` passes" — same as [1-1].

[3-3]
- "`go test ./...` passes" — same as [1-1]. The symbol-resolution half ("every symbol the new test file references resolves to exactly one declaration in `package tui`") is settled by the clean `go vet` / `golangci-lint` type-check.
- "A `-f` picker, a no-argument picker and a command-pending picker render their session rows byte-identically to today (existing suites green unmodified)" — the substance is settled by reading (`ShowDir` is set from `m.searchForm` alone, so the row is character-identical with it false); only "existing suites green" is unsettled, and that is a suite run.
- "The column survives an `s` regroup, a `Space` preview and back, a `SessionsMsg` refresh, a marked-set mutation and a live theme swap" / "The column survives a hand edit of the committed filter text, including clearing it" — each has a subtest driving the real `Update` path (`internal/tui/search_dir_column_test.go:92`); confirming the width-220 render and the width-34 drop needs `go test ./internal/tui -run TestSearchDirColumn`.

[3-4]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].
- "The human visual check is recorded before sign-off, with the still capture and the live-view command presented" — a sign-off gate record, not a measurement any section can take. Live-view command: `go run ./cmd/capturetool --fixture sessions-search-results --theme tokyo-night`.
- "the deep path's `…/` truncation at the 120-column harness width and the arbitrary-`$HOME` abbreviation subtest" — the model layer is pinned at `internal/capture/capture_test.go:1039`, but confirming it needs the package run.

[3-5]
- "With the sessions-page key guard temporarily widened to swallow `s` under an applied filter, both subtests fail; with it reverted, both pass." — the experiment requires editing a tracked production file (`internal/tui/model.go:2547`); no change-set pass may modify the tree.
- "`go test ./...` is green." — whole-lane sweep, as [1-1].

[4-1]
- "`go test ./...` passes" — same as [1-1].

[4-2]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].

[4-3]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].
- "A warm single-match attach writes every accumulated warning line to stderr, in order, before `openSessionFunc` is called" — the in-process ordering is settled by reading (`cmd/open_search.go:245-246` emits then attaches) and asserted at `cmd/open_search_warnings_test.go:75`; the end-to-end leg needs a live install with `_portal-saver` killed, which mutates the developer's running system.

[4-4]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].
- "The warm path, every CLI path and the concurrent route's step sequence and labels are unchanged (existing suites green unmodified)" — the "unchanged" half is settled by the diff (the range touches neither `cmd/bootstrap/progress_emitter.go` nor `internal/tui/loading_progress.go`); the "green" half is a suite run.
- "On a cold server `open /port` takes the concurrent bootstrap and the honest loading page actually paints … the same verdict `open -f port` gets on the same boot" — observed only by the integration-tagged 4-5 suite against a real cold tmux server.

[4-5]
- "`go test -tags integration -p 1 ./cmd -run TestConcurrentColdBoot_SearchDecision` passes, and `go test ./...` is unaffected" — the run halves; the build tag and the absence of identifier collisions are settled by the clean type-check.
- "The decision closure is invoked exactly once across the whole boot"; "All ten real step events, and the terminal complete event, were delivered before it ran"; "`state.IsRestoringSet(client)` is false at the instant it runs"; "The closure's own live read returns the restored sessions"; "The model is on `PageLoading` after every step event"; "On the single-match term the model quits with `Selected()` set, `SearchAttached()` true, and `ActivePage()` still `PageLoading`"; "On the shared-fragment term the model transitions to `PageSessions`"; "`FatalError()` is nil on both runs and no `BootstrapFatalMsg` is observed" — each is asserted and correctly targeted (`cmd/concurrent_search_decision_integration_test.go:167`, `:227`) and each follows from `dismissLoadingGate` (`internal/tui/model.go:1509-1520`) by reading, but observing them needs the integration lane against a real cold server.

[4-6]
- "`go test ./...` is green." — whole-lane sweep, as [1-1].

[4-7]
- "`go test ./...` is green." — same.

[4-8]
- "`go test ./...` is green." — same.

[5-1]
- "`go test ./...` passes" — same as [1-1].

[5-2]
- "In fish, completion after the session-opening function asks Portal for `open`'s completions and offers live session names" — `command -v fish` resolves nothing on this machine, so every fish subtest takes the `requireShell` skip. The fish emitted-registration assertions (`complete -c x -f` then `complete -c x -w 'portal open'`) were read and hold. The bash and zsh legs were measured live against the emitted scripts and hold.
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5].

[5-3]
- "`go test ./...` passes" — same as [1-1]. The deliverable itself was settled directly: the rendered `portal open --help` was checked line by line against §9.2.

[5-4]
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — same as [1-5]. The deliverable itself was settled directly against the README's current `### \`x\` (open)` section.

[5-5]
- "A measurement contradicting the derivation closes the task with `cmd/init.go` unedited and that recorded; no further criterion applies in that case." — settling it needs a bash carrying the `bash-completion` package, which this machine does not have and which the project gives no way to provision. (The plan task that was to take this measurement, 13-2, was cancelled.)
- "`go test ./cmd` is green, including the verbatim shim expectation in `cmd/init_test.go`." — a package run no section took.

[6-1]
- "`go test ./...` is green" — whole-lane sweep, as [1-1]. The `gofmt -l` and `go vet ./...` clauses are measured clean.
- "Behaviour is unchanged on both routes: `TestOpenCommand_SearchForm_ExcludesTheCurrentSessionFromTheCount`, `…AttachesTheOtherMatchWhenTheCurrentSessionAlsoMatches`, `…CountsNothingOutWhenTheCurrentSessionReadFails`, `…ExcludesNothingOutsideTmux` and `internal/tui`'s \"excludes the current session when inside tmux\" all pass with no edit to any of them" — the "no edit" half is settled by the diff; the "pass" half is a suite result.

[6-2]
- "`go test ./...` is green" — same. The `gofmt -l` and `go vet ./...` clauses are measured clean.
- "`TestMatchesSearchTerm`, `TestMatchesSearchTermUnresolvableHome` and the existing `FilterValue` subtests pass with no edit" — the "no edit" half is settled by the diff; the "pass" half is a suite result.

[6-3]
- "`go test -race ./internal/tui` is green, including a test driving `set` concurrently with a filter pass" — a package run under `-race`, taken by no section. Reading settles the design: `internal/tui/search_filter.go:35-59` swaps a freshly built map under the write lock and `current` returns it under the read lock, so no published map is ever mutated. The `gofmt -l` and `go vet ./...` (including `copylocks`) clauses are measured clean.
- "`TestContainmentFilterFallsBackWhenTheSourceIsOutOfStep` and the `TestSearchContainment*` suites pass with no edit" — the "no edit" half is settled by the diff; the "pass" half is a suite result.

[7-1]
- "`go test ./...` passes" — whole-lane sweep, as [1-1]. The `go vet ./...`, `gofmt -l .` and `golangci-lint run ./...` clauses are measured clean.

[7-2]
- "The loading-gated routes are untouched: the existing `internal/tui` and `cmd` suites pass unchanged, with no new call reaching a real tmux server." — the "no real tmux" half is settled by reading (both new files construct only in-package fixtures and mocks; neither names `tmux.DefaultClient` nor a socket) and by the clean type-check; the "pass unchanged" half is a two-package run.

[8-1]
- "`go build ./... && go test ./...` green" — the `go build ./...` and `golangci-lint run` clauses are measured clean; the suite clause is a whole-lane sweep.
- "The existing search-count suites pass unchanged, including `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux`'s assertion that `CurrentSessionName` is read zero times outside tmux" — the assertion is present at `cmd/open_search_test.go:662` and consistent with `cmd/open_search.go:164-173`; passing is a suite result.

[10-1]
- "`go test ./internal/tui/... ./cmd/...` passes with no edit to any test file" — the "no edit" half is settled by the single-file diffstat; the "passes" half is a two-package run.

[10-3]
- "`go test ./internal/tui/... ./cmd/...` passes with no edit to any test file" — same.

[11-1]
- "Every refusal today's seven flags produce is byte-identical and still fires from its own named arm, in the current order — the collision suite at `cmd/open_search_test.go:337-401` passes with no edit." — the "no edit" and arm-ordering halves are settled by reading and the diff; "passes" needs `go test ./cmd -run 'TestValidateOpenArgs|TestValidateSearchFormCollisions|TestOpenCommand_SearchForm'`.

[11-2]
- "`go build ./...`, `go test ./...` and `golangci-lint run` are clean, and no test's semantics change beyond the three respelled calls." — the build and lint halves are measured clean and the semantics clause is settled by the diff; the `go test ./...` half, and the integration-lane `go test -tags integration -p 1 ./cmd -run TestConcurrent`, are runs no section took.

[11-4]
- "`go test ./internal/tui/...` passes with no test changed" — the "no test changed" half is settled by the single-file diffstat; "passes" is a package run.

[11-5]
- "`go test ./cmd -run TestCompleteOpenPositional` passes." — a package run no section took.
- "The four existing subtests are unedited and still pass: `-- ls /po` matches `completeSessionNames(\"/po\")` … and the probe-parse subtest still pins the `<=`." — the "unedited" half is settled by the diff; the criterion's own wording was superseded by task 12-6 (`756d46a23`), and HEAD matches 12-6's contract (`cmd/completion_test.go:586`). "Pass" needs the run.

[12-1]
- "The `commandPending` branch, `WarningsOwedAtTeardown`'s own rule and the drop of a complete arriving after dismissal are unchanged, and `go test ./...` is green." — the first three clauses are settled by reading; the green clause is a whole-lane sweep.

[12-2]
- "The `sync.RWMutex` and the single accessor stay, and the concurrent-`set` test is green under `-race`." — the first clause is settled by reading (`internal/tui/search_filter.go:28-57`); the `-race` run was taken by no section.
- "`internal/tui/search_containment_test.go` and `go test ./...` stay green." — a suite result.

[12-3]
- "A model with no decision closure dismisses exactly as it does today, and `go test ./...` is green." — the substance is settled by reading (`internal/tui/model.go:1515-1519` falls straight through to `completeLoadingDismissal` when `m.searchDecide` is nil) and asserted by `TestSearchDecision_AbsentClosureLeavesTransitionUnchanged`; the green clause is a suite result.

[12-4]
- "`completingPreDashPositional`, `validateSearchFormCollisions`'s seventh derived arm and the plain session-name completion at a second positional are untouched, and `go test ./...` is green." — the "untouched" half is settled by the diff; the green clause is a suite result.

[12-5]
- "`go test ./...` is green and `golangci-lint run` reports nothing new." — the lint clause is measured clean (`0 issues.`); the suite clause is a whole-lane sweep.
- "Every `TestWarningsOwedAtTeardown` subtest other than the cancel one passes with no edit." — the "no edit" half is settled by the commit diff; "passes" is a suite result.

[12-6]
- "`go test ./...` is green and `golangci-lint run` reports nothing new." — the lint clause is measured clean; the suite clause is a whole-lane sweep.

[13-1]
- "`go test ./internal/tui` is green, `go test -race ./internal/tui -run TestContainmentFilterIsRaceFreeAgainstAConcurrentSet` is green, and `go test ./...` is green." — three package/suite runs, none taken. The package's compilation is settled by the clean `go vet` / `golangci-lint` / `go build ./...` (the `slices` import used at `search_filter.go:44`, `searchEntry` comparable so `slices.Contains` type-checks), and the publish-and-swap design is settled by reading.

### Code Quality

No issues found beyond the one finding below. The five change-set verifiers each held the change-set against the project's own conventions and the declared linters, and all three linters plus the build are clean over the whole tree. Task 13-1's own code reads well: `everyEntryMatches` is a single-purpose unexported predicate, the `len(held) > 0` guard closes the vacuous-truth trap an `everyEntryMatches` over an empty slice would otherwise open, the source keeps its single accessor over the retained `sync.RWMutex`, and `set` still builds a fresh map and swaps it under the write hold, so the map `current()` hands back stays safe to read unheld.

### Test Quality

Tests adequately verify requirements. The test-surface verifier looked specifically for the three failure modes the finding floor admits for test files — a guard that passes while checking nothing, an isolation hole, a reproduced flake — and found none: every loop-shaped assertion has a sibling asserting its collection is non-empty, the four integration-tagged files carry `//go:build integration` on line 1, the new shell-driven suite builds and runs no portal binary, and no `t.Parallel()`, `os.Setenv`, `tmux.DefaultClient()` or hand-rolled `go build` was added anywhere. Three conditions are worth knowing without being defects:

- `TestContainmentFilterIsRaceFreeAgainstAConcurrentSet` measures a race only under `-race`, which neither documented lane carries. Go's always-on concurrent-map-access check gives partial coverage without it.
- `cmd/init_completion_shell_test.go`'s bash arm asserts the file-completion switch-off, which holds only where `bash` on PATH is 4.0+ with `compopt` as a builtin. Here it resolves to `/opt/homebrew/bin/bash` 5.3.15, so it holds; on a PATH resolving macOS's stock `/bin/bash` 3.2 — the configuration the specification's own corrigendum names as the case in the wild — that assertion would fail on correct behaviour.
- `cmd/open_docs_test.go` guards §9.2's completion-rollout point only by the token `portal init`; the README carries the consequence in full, so it is a partial guard over correct text.

### Blocking Issues

None outstanding. The one action below is routed to planning on its blast radius, not because it carries a blocking marker.

## Findings

### Needs planning

**A1 — the search filter can surface a row the term does not match** (from `13-1-1`; `internal/tui/search_filter.go`, `internal/tui/search_filter_test.go`)

`containmentFilter` ranks a handed target by whatever generation `searchItemSource` currently holds, so when a filter pass runs behind a list rebuild **and** the target's filter value is one two distinct sessions can produce, the held generation's entry decides the rank for a live row from a different generation. Verified against the code: a source holding `{name:"api ~/Code/api", dir:""}` and a pass handed targets built from `{name:"api", dir:"<home>/Code/api"}` both key on `"api ~/Code/api"`; under the term `api ~` the held entry matches at `search_filter.go:82` and the row ranks — surfacing a session neither of whose fields contains the term, the exact outcome `resolver.MatchesSearchTerm`'s field separation and the withholding rule exist to prevent. The doc comment at `search_filter.go:68-70` states the opposite as an unqualified guarantee.

**What fails**: in a picker opened by `portal open /term`, a session whose name and directory both lack the search term can appear in the narrowed list when a list rebuild overlaps a filter pass and a colliding filter value is in play — the search form's one guarantee, that every row shown contains the term, silently does not hold. Leaving the comment as written also signs that guarantee as the file's contract, so the next maintainer builds on a false invariant.

**How far the fix reaches**: it is a design decision, not one settled edit. It reaches task 12-2's delivered property and the test that pins it (`TestContainmentFilterRanksAStaleGenerationsTargetsByContainment` asserts a stale generation's targets still rank), the type comment stating the rationale for the value-keyed map, and the unanswered question of what a skewed pass should return. At least three defensible shapes exist:

- **(a)** generation-equality with rank-nothing on skew — the assessor's prescription. It reverts the shape task 12-2 deliberately removed (pre-12-2 the file held `recorded []string` plus `slices.Equal(recorded, targets)`), inverts the test above, can transiently blank the filtered list, and does not close the case where two generations yield identical ordered filter values from different field pairs.
- **(b)** generation-equality with the pre-12-2 fall-through to `list.DefaultFilter` — which surfaces non-matching rows by construction under a fuzzy rule, so it is not a safe fallback for a forced-filter picker.
- **(c)** keep the value-keyed map and have `set` union a bounded number of recent generations, so a colliding key holds both sessions' field pairs and `everyEntryMatches` withholds the row — preserves 12-2's property and closes the case, but needs a retention rule.

**Amended in prep**: re-aimed from the finding's prescribed comment narrowing to the code defect the assessor named; the finding's own "nothing here is blocking, the remedy is comment text only" judgement is not carried through, because the behaviour it describes is falsifiable.
