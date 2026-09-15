TASK: Take the search-form shape out of the resolution chain (tick-69c098 / open-with-forced-filter-1-4)

ACCEPTANCE CRITERIA:
- `IsSearchSigil` is true for `/port` and for a bare `/`, and false for `/Users/leeovery/Code/portal`, `/tmp/`, `./port`, `~/port`, `port` and the empty string
- `SearchTerm("/port") == "port"` and `SearchTerm("/")` is empty
- `IsPathArgument` is false for the sigil shape and unchanged for every other argument, including `/tmp/` and `/Users/leeovery/Code/portal`
- Neither function stats, expands or otherwise touches the filesystem — the verdict is identical on a machine where `/port` exists and one where it does not
- `QueryResolver.Resolve("/port")` returns a `*MissResult` when nothing else claims it, rather than a path result or a directory-not-found error
- `QueryResolver.Resolve("/Users/…/portal")` still returns a `*PathResult`, and `ResolvePathPin("/tmp")` still mints
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §2.2 states the rule verbatim — "A positional argument is a sigil when it begins with `/` and contains no further `/`" — and gives the recognition table (`/port` sigil; `/Users/leeovery/Code/portal` and `/tmp/` path; `./port`, `~/port`, `port` untouched), insisting the test stays a shape test that never consults the filesystem so one command line reads identically on two machines. §2.4 records the accepted cost (single-segment absolute dirs stop minting; `-p` is the escape) and §2.5 makes a bare `/` a search form rather than a mint at root. §3.1 declares the sigil session-domain: it must reach neither the path, alias nor zoxide domain via `open` — this task only removes it from the path domain, with `open`'s interception landing in task 1-5.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/resolver/path.go:25` — `IsSearchSigil` = `strings.HasPrefix(arg, "/") && strings.Count(arg, "/") == 1`, the spec's rule with no filesystem call.
  - `internal/resolver/path.go:31` — `SearchTerm` = `strings.TrimPrefix(arg, "/")`; `"/port"` → `"port"`, `"/"` → `""`.
  - `internal/resolver/path.go:14` — the narrowing guard inside `IsPathArgument`; the rest of the rule (line 17) is byte-identical to the pre-task text (confirmed against `git show 9edd1c221 -- internal/resolver/path.go`).
  - `internal/resolver/query.go:105` — the only production consumer of `IsPathArgument`; `Resolve` therefore walks a sigil past the path branch into alias (113), zoxide (119) and the `*MissResult` tail (123).
  - `cmd/bootstrap_warnings_test.go:269` — the fixture argument re-pointed to the multi-segment `/nonexistent/path-for-test`, so that test still exercises the positional-path shape it is about.
- Notes: The pins are untouched as instructed — `ResolvePathPin` (`internal/resolver/query.go:217`) stats the literal path through `ResolvePath` and never consults `IsPathArgument`, and a repo-wide grep finds exactly two consumers of `IsPathArgument`: its own definition site's guard and `query.go:105`. Both new functions are exported package-level pure functions, which is what task 1-5 (`cmd/open.go:179`) and the completer (`cmd/completion.go:71`, `:75`, `:108`) later consume. `SearchTerm` is total rather than sigil-guarded (it would return `a/b` for `/a/b`), but both production callers gate on `IsSearchSigil` first, so no non-sigil value reaches it.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/resolver/path_test.go:109` `TestIsSearchSigil` — all eight named shapes from the criteria, including the empty string (`:152`).
  - `internal/resolver/path_test.go:168` `TestSearchTerm` — both term cases.
  - `internal/resolver/path_test.go:11` `TestIsPathArgument` — extended with the seven recognition-table rows (`:62`–`:96`), including `/tmp/` → true and `/Users/leeovery/Code/portal` → true, so the narrowing cannot widen unnoticed.
  - `internal/resolver/query_test.go:291` `TestQueryResolver_Resolve_SearchSigil` — `Resolve("/port")` → `*MissResult` with the target preserved; `/tmp` (exists) and `/definitely-not-here` (does not) both → `*MissResult`, which is the machine-independence criterion pinned as behaviour rather than prose; a multi-segment `t.TempDir()` path still → `*PathResult`/`DomainPath`; `ResolvePathPin("/tmp")` still mints.
- Notes: These tests fail if the feature breaks. Drop the guard at `path.go:14` and the `/tmp` arm of the machine-independence case returns a `*PathResult` (the directory exists) while the `/definitely-not-here` arm fatals on the `Directory not found` error — the two arms catch the regression on either kind of machine. The suite consults the filesystem only where the case is about a real path (`t.TempDir()`, `ResolvePathPin("/tmp")`); the recogniser's own table is pure. Three new `TestIsPathArgument` rows restate coverage already present with different strings (`./port` beside `./subdir` at `:36`, `~/port` beside `~/Code` at `:44`, `port` beside `myproject` at `:52`) — they are the plan's explicit recognition-table rows and cost nothing, so not raised as over-testing.

CODE QUALITY:
- Project conventions: Followed. `internal/resolver` stays a pure, log-free library (its `log_free_test.go` guard is unaffected — nothing was imported); no `t.Parallel()`; tests are external (`package resolver_test`) and named in the codebase's "it …" style.
- SOLID principles: Good — one recogniser, one extractor, the narrowing expressed as a single guard delegating to the recogniser rather than a second copy of the rule.
- Complexity: Low — two single-expression functions and one added early return.
- Modern idioms: Yes — stdlib `strings` only, no allocation, no regexp.
- Readability: Good. The `IsSearchSigil` doc comment (`path.go:20`–`:24`) states the rule, names the four shapes it rejects and asserts the filesystem independence; each claim holds against the body. No process-artifact references (no task ids, phase numbers or spec section numbers) in either comment.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — needs the unit lane executed (`go test ./...` from the repo root); reading the sources and the new tables settles compilation-shape and assertion logic but not a suite run.
