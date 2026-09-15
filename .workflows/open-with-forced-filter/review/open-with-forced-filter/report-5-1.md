TASK: Complete the search term after the slash against live session names (open-with-forced-filter-5-1, tick-6c7035)

ACCEPTANCE CRITERIA:
- `/po` offers the live session names prefixed by `po`, each returned with the leading `/` — `/portal-a1b2`, never `portal-a1b2`
- `/` offers every live session name, each carrying the slash
- `/ort` offers nothing (the offer is prefix-shaped) while the term itself still matches `portal-a1b2` by containment at run time
- A live session whose name contains a `/` is never offered, for any term including the empty one
- `_portal-saver` and `_portal-bootstrap` are never offered, with no filter of their own added here
- No directory, path fragment or filesystem entry is ever offered on this branch
- A non-search word completes byte-identically to today: `/tmp/`, `~/Code/pro`, `.`, a bare word, and the empty word all route to `completeSessionNames` unchanged
- Every branch — including the zero-candidate ones — returns `cobra.ShellCompDirectiveNoFileComp`
- A tmux read failure yields no candidates and no error, with the directive still `NoFileComp`
- The completer builds its own client through the existing seam and needs no context-injected client, so the bootstrap-exempt `__complete` path still works
- Prefix matching stays a byte prefix — no case folding and no containment on the offer side
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §8.1 ("Completion looks past the sigil") requires `/po<TAB>` to complete the term after — and excluding — the `/` against live session names, with the sigil kept on every offered word, because the shell discards any candidate that is not an extension of the word being completed. §8.1 as it now stands carries two rules added after this task was authored and both are honoured by the delivered code: the offered set is "the set the sigil can reach, not the whole live enumeration" (the picker's set, which omits the session the caller is attached to, degrading to holding nothing back when the read fails or answers nothing), and completion is "bounded by the `--` separator, as recognition is (§2.3), and by nothing else" — a `/word` past the separator is answered with no candidates and the shell's own filename completion left on, while the word immediately after the separator keeps the sigil arm because the flag layer cannot distinguish it from the next ordinary positional. §2.2 supplies the no-second-slash rule that makes a slash-bearing session name unofferable, and §4.3 the containment rule the offer side deliberately does not adopt.

IMPLEMENTATION:
- Status: Implemented (moved past the task text by two later tasks in the same feature, both spec-recorded)
- Location:
  - `cmd/completion.go:70` — `completeSearchTerm`: term via `resolver.SearchTerm`, set via `tui.PickerSessions(completionSessions(), completionCurrentSession())`, slash-bearing names held back by asking `resolver.IsSearchSigil("/" + s.Name)`, byte-prefix match on `s.Name`, each candidate returned as `"/" + s.Name` with `ShellCompDirectiveNoFileComp`.
  - `cmd/completion.go:104` — `completeOpenPositional`: pre-dash guard (`completingPreDashPositional`, `cmd/open_search.go:46`) → sigil arm → plain `completeSessionNames` arm.
  - `cmd/open.go:785` — `openCmd.ValidArgsFunction = completeOpenPositional`.
  - `cmd/completion.go:15`/`:25` — the `completionSessions` / `completionCurrentSession` seams, each building its own `tmux.DefaultClient()` and degrading a failed read to nil / `""`.
  - `internal/tui/picker_sessions.go:13` — `PickerSessions`, the single home of "the set the picker lists", shared with the search count in `cmd/open_search.go`.
- Notes: Two divergences from the task's literal "Do" text, both sound and both recorded in the spec rather than lost:
  1. The offered set is the picker's set rather than the raw enumeration, so the attached session is not offered (task tick-430d00, §8.1's "What is offered is the set the sigil can reach"). Degrades correctly: `currentPickerSession` returns `""` outside tmux and on a failed read, and `PickerSessions("")` drops nothing.
  2. `completeOpenPositional` takes `(cmd, args, toComplete)` rather than `toComplete` alone and answers a post-separator word with `nil, ShellCompDirectiveDefault` (the separator-bound task, §8.1's "bounded by the `--` separator … and by nothing else"). This is the one place a non-search word does not complete byte-identically to today, and it is the change §8.1 asks for, not a loss.
  The name-exclusion rule is derived from the parser (`IsSearchSigil("/"+name)`) rather than restated as `strings.Contains(name, "/")`, so the offer side cannot drift from §2.2. `_portal-*` filtering is inherited from `ListSessions` with no second filter added here, exactly as the criterion requires. `completeSessionNames`, `killCmd`'s completer and the `--session` / `--alias` flag completers are untouched; `--path` / `--zoxide` still register nothing.

TESTS:
- Status: Adequate
- Coverage: `cmd/completion_test.go:350` `TestCompleteSearchTerm` covers the slash-kept candidate (`/po` → `["/portal-a1b2"]`), the bare slash offering every name, a term prefixing no name, a slash-bearing name held back for both `/foo` and the empty term, the attached session held back for both terms, the empty/failed current-session read holding nothing back, and the nil session read — each asserting `NoFileComp`. `:463` `TestCompleteOpenPositional` tables the non-search words (`/Users/lee`, `/tmp/`, `~/Code/pro`, `.`, a bare word, the empty word), asserting both the concrete expectation and byte-identity with `completeSessionNames`. `:514` `TestSearchCompletionWiring` drives `openCmd.ValidArgsFunction` directly, the end-to-end `completionCandidates(t, "__complete", "open", "/po")`, the `--session` flag completer (unchanged), `killCmd` (unchanged) and a second positional. `:586` `TestCompleteOpenPositionalSeparatorBound` pins the separator rule including the boundary word and the pflag dash-index carry-over. `:253` `TestCompletionExcludesInternalSessions` pins the `_`-prefixed exclusion for the search branch against a real disposable tmux socket. The edge case the task names as the reason the slash must ride on the candidate is proven at the shell layer by `cmd/init_completion_shell_test.go:366` (`TestInitCompletion_CarriesSlashPrefixedCandidateIntoReply`, bash/zsh/fish), with `:378` proving no filename is offered for `/tm`.
- Notes: No over-testing found. The three layers (`completeOpenPositional` directly, `openCmd.ValidArgsFunction`, `__complete` end to end) are the three the plan asked for and each fails for a different reason. The one doubled assertion — `slices.Equal(names, plain)` beside `slices.Equal(names, tt.want)` at `:502`/`:503` — asserts routing and value separately and earns its place. Tests stage the seams through `withCompletionSessions` / `withCompletionCurrentSession`, never assigning a seam directly, so `cmd/seam_guard_test.go` stays satisfied. Subtests that leave `completionCurrentSession` on its production value rely on `TestMain`'s `TMUX` poison to make the read fail closed to `""`; that is the package's stated enforcement mechanism, not an accident, and it contacts no real server.

CODE QUALITY:
- Project conventions: Followed. Function-var seams consumed through `withFuncSeam` helpers; no `t.Parallel()`; the real-tmux assertion sits in the sanctioned fast client-test set (disposable `ptl-*` socket, no daemon, no binary built).
- SOLID principles: Good. `completeOpenPositional` routes and does nothing else; `completeSearchTerm` owns one rule; the searched-set rule and the sigil-shape rule are each read from their single owner (`tui.PickerSessions`, `resolver.IsSearchSigil`) rather than restated.
- Complexity: Low — one loop, two guards.
- Modern idioms: Yes.
- Readability: Good. Both doc comments explain why rather than restating the code.
- Comment accuracy: The comments hold against the code. `completeSearchTerm`'s claim that the set comes from `tui.PickerSessions` and the shape question is asked of `resolver.IsSearchSigil` matches line 74/75; `completeOpenPositional`'s account of the post-separator arm matches the `nil, ShellCompDirectiveDefault` return, and its boundary-word exception matches `completingPreDashPositional`'s `len(args) <= dash`. No task ids, phases or spec-section references in the changed code.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — needs the unit lane run from the project root; reading cannot settle a suite result.
