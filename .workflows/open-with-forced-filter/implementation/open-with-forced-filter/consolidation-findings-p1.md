# Consolidation Findings: open-with-forced-filter (Phase 1)

## Findings

### F1: a no-probe fixture labelled "direct-path open" now carries the shape this phase redefined as a search form
- **Class**: drift
- **Failure**: `TestShouldRunConcurrentBootstrap_IssuesNoProbe` enumerates three inputs to prove the cold-path decider issues no tmux round-trip on each, and one of them is named for the branch it covers. Task 1-4 made `/dir` a search form rather than a path argument, so that row no longer exercises a direct-path positional at all: the table is left with two rows it cannot tell apart today and no case for the direct-path branch. Task 4-4 then flips a search form to `isTUIPath == true`, at which point the row exercises the picker branch under the direct-path label. A later change that made the decider probe the server for a genuine path positional — the exact regression the test exists to catch, since the decider's contract is "issues no tmux round-trip" — would pass this suite and reach the user as a cold-boot `portal open ~/Code/api` stalled on an unanswered tmux read before a single frame is painted.
- **Evidence**:
  - `cmd/concurrent_bootstrap_gate_test.go:175` — `{"direct-path open", openProbeCmd(), []string{"/dir"}}`
  - `internal/resolver/path.go:25-27` — `IsSearchSigil` accepts `/dir` (leading slash, no further slash)
  - `internal/resolver/path.go:14-16` — `IsPathArgument` now returns false for that shape
  - `cmd/concurrent_bootstrap_gate_test.go:36` and `:127` — the sibling direct-path fixtures in the same file already use `~/dir`, which is the spelling this row wants
- **Proposed shape**: change the row's argument from `/dir` to `~/dir`, matching its two siblings. One word; no assertion, name or structure changes.
- **Bank**: reviewer entry on task 1-4 — "a classification fixture still uses a single-segment absolute positional, which stops meaning what its name says once the sigil becomes a picker invocation". Confirmed against the final state: the row is unchanged at `:175` and `/dir` is a sigil as of `9edd1c221`.

## Comment Corrections

- `CLAUDE.md:210` — the session-grouping paragraph names `m.sessions` as the home of the derived directory; task 1-1 moved it to a separate grouping-only map and made `Session.Dir` recorded-only, so the claim now describes the conflation the phase exists to remove (`internal/tui/model.go:1177-1199`, `:1120`; `internal/tui/grouping.go:16-22`).
  OLD: caching the guess **in-memory** (into `m.sessions`, so later rebuilds in the same picker session skip the pane read)
  NEW: caching the guess **in-memory** (into `m.derivedDirs`, a grouping-only map keyed on session name — never the recorded `Session.Dir`, which stays exactly as tmux reported it — so later rebuilds in the same picker session skip the pane read, and a session-list refresh clears it)

- `CLAUDE.md:37` — the `open` key-command paragraph states that a bare positional runs the resolution grammar; after task 1-4 and task 1-5 one bare-positional shape never enters that chain at all (`internal/resolver/path.go:25`, `cmd/open.go:157-159`, `cmd/open_search.go:12-67`).
  OLD: A bare positional runs the resolution grammar (exact session → glob → path → alias → zoxide) and creates/attaches directly;
  NEW: A bare positional runs the resolution grammar (exact session → glob → path → alias → zoxide) and creates/attaches directly — except the search form, a positional beginning with `/` and containing no further `/` (`portal open /term`, `portal open /`), which never enters the resolution chain and is handled as a search over live sessions (`resolver.IsSearchSigil` / `cmd/open_search.go`); it composes with nothing, and `openCmd`'s `Args` validator refuses a line carrying it beside another target, a command, `-f` or a domain pin before any bootstrap runs;
