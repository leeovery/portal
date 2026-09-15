TASK: Keep the grouping-derived directory out of the session's recorded directory (open-with-forced-filter-1-1, tick-a5663a)

ACCEPTANCE CRITERIA:
- No production code in `internal/tui` assigns to a `tmux.Session`'s `Dir` field; the derived value lives only in `m.derivedDirs`
- A By-Project rebuild over a session with an empty recorded directory groups it under its project (not the `Unknown` catch-all) while `m.sessions[0].Dir` stays empty
- A By-Tag rebuild over the same session groups it under each of its project's tags (not `Untagged`), reading the derived value through the same helper
- Every `SessionItem` emitted by `buildByProject` / `buildByTag` carries the session's recorded directory verbatim — empty stays empty
- A second rebuild in the same picker session issues no further pane read (the derived map is the cache)
- `applySessions` clears the derived map, so a refresh re-issues the pane read on the next grouped rebuild
- Flat mode and the zero-tags signpost issue zero pane reads and zero stamp writes over unstamped sessions
- An unresolvable session (pane read returning an empty path with no error, an error, or nil seams) stores nothing, keeps both values empty and routes to the catch-all
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §4.1 ("The matched fields") fixes the match domain as the session name plus the *recorded* `@portal-dir` directory, and is explicit that a derived directory is never part of it: "A session therefore carries the two as separate values — the recorded directory, which may be absent, and the derived one, which grouping alone reads and writes. A derived value never lands in the recorded one." The stated consequence the separation buys is that neither the match (§4.4) nor the directory column (§6.1) can be moved by a regroup — a session can never become findable by a path it was not findable by a moment earlier, and a path can never appear beside a name that showed none. §4.1 also restates the standing rule that the derived value is never stamped back to tmux (pane cwd drifts from the origin dir).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:267-272` — `derivedDirs map[string]string` on `Model`, documented as the grouping-only companion to `Session.Dir`
  - `internal/tui/model.go:1207-1229` — `resolveDerivedDirs(sessions) map[string]string` replaces the old `resolveSessionDirs([]tmux.Session) []tmux.Session`: nil-seam early return preserved (`m.dirReader == nil || m.dirRunner == nil`), recorded-dir sessions skipped, cached names skipped, and only a `(dir != "", ok, err == nil)` result stored
  - `internal/tui/model.go:1150` — `applySessions` clears `m.derivedDirs` before the rebuild
  - `internal/tui/model.go:1242,1244` — `resolveDerivedDirs(filtered)` is passed into the By-Project and By-Tag arms only; the `byTagSignpost` and default (Flat) arms call neither it nor the builders
  - `internal/tui/grouping.go:16-21` — `effectiveDir(s, derived)`: recorded first, derived second
  - `internal/tui/grouping.go:29,34,40` / `:56,61` / `:78-84` — `buildByProject`, `buildByTag` and `resolveSessionTags` resolve their index key through `effectiveDir` and embed the unmodified `s` in every emitted `SessionItem`
  - `cacheSessionDir` is gone (`grep -rn cacheSessionDir --include="*.go" .` → no matches), and the only `.Dir` writes left anywhere in `internal/tui` production code are none — the sole non-test hits are reads (`internal/tui/search_filter.go:38`, `internal/tui/session_item.go:88,267,268,277`, `internal/tui/grouping.go:17-18`, `internal/tui/model.go:1213`), which is exactly the property the criterion asks for: neither the match text nor the directory column can see a derived value.
- Notes: The derived value is still never written back to tmux — the seam `m.dirReader` is `session.PaneCurrentPathReader`, whose only method is `ActivePaneCurrentPath`, so production has no route to a stamp on this path. All 45 call sites of `buildByProject`/`buildByTag` (production and test) were re-pointed to the three-argument form, passing `nil` where the case has no derived value; `resolveSessionTags` took the same parameter, which is what keeps By Tag off the raw recorded value. `CLAUDE.md`'s grouping section already describes `m.derivedDirs` in these terms, so the project record and the code agree.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/rebuild_dir_resolution_test.go` carries the task's own suite — By Project grouping with `m.sessions[0].Dir` and `SessionItem.Session.Dir` both asserted empty (`:50-82`), By Tag with two tags producing two non-catch-all rows and the same two emptiness assertions (`:84-113`), the cache-not-stamp pair asserting `m.derivedDirs["portal-abc"] == key` and zero `SetSessionOption` calls (`:115-137`), second-rebuild-zero-reads (`:139-159`), the empty-path-no-error shape routed to `Unknown` with the name absent from `m.derivedDirs` (`:164-191`), `applySessions` twice re-issuing the pane read (`:193-213`), and nil seams routing to the catch-all without a panic (`:215-231`). `internal/tui/rebuild_dir_resolution_gate_test.go:12-58` holds the Flat and `byTagSignpost` zero-read/zero-stamp gates, with `:60-102` as the positive control that the grouped arms do read. `internal/tui/rebuild_dir_resolution_test.go:233-262` additionally pins that the derived value is no part of the row's `FilterValue()` and that filtering on it returns nothing — the §4.1 property stated as an observable.
- Notes: Assertions are one-property-per-check and the fixtures are two small fakes; nothing is redundant and no test reaches into an implementation detail beyond the `m.derivedDirs` map the criteria name explicitly. The error-returning pane read is the one shape in the criteria list with no dedicated case, but the `dir == ""` arm of the same guard already catches it (`ResolveSessionDir` returns `("", false, err)` on a reader error), so no behaviour is left unobserved.

CODE QUALITY:
- Project conventions: Followed — the pointer-receiver mutator/value-receiver reader split matches the rest of `model.go`, the grouped builders stay pure functions taking their inputs, and the seam-nil early return keeps the no-DI test path alive.
- SOLID principles: Good — `effectiveDir` is the single place the recorded/derived precedence is decided, and all three grouping call sites route through it rather than restating it.
- Complexity: Low — `resolveDerivedDirs` is a flat loop with three guard clauses; `effectiveDir` is two lines.
- Modern idioms: Yes.
- Readability: Good — both new declarations carry a comment stating why the two values cannot share a field, and `applySessions`' one-line comment says what the clear is for.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — settling it needs the unit lane run (`go test ./...` from the repo root); reading establishes that every `buildByProject`/`buildByTag`/`resolveSessionTags` call site in the tree carries the new three-argument form and that the re-pointed assertions match the new field, but not that the suite executes green.
