TASK: cli-verb-surface-redesign-1-2 — Directory chain (path → alias → zoxide) mint outcomes + total-miss hard-fail

ACCEPTANCE CRITERIA / EDGE CASES (from the Phase 1 task table):
- A bare project name (`api`) mints and never reattaches an `api-*` session.
- An alias resolving to a non-existent dir errors.
- zoxide-not-installed / no-match falls through silently to a miss.
- The miss message names the raw target (`nothing resolved for 'blog' — try -f blog`).
- Existing command→mint threading preserved (attach+command formalization is Phase 2).

STATUS: Complete

SPEC CONTEXT:
Spec § "Target resolution precedence" defines the bare-positional precedence chain: exact session name → path → alias → zoxide, first match wins; a directory-domain hit always mints a fresh `{project}-{nanoid}` session (Axiom 2, no find-or-create). Spec § "Bare project shorthand does not reattach" pins that `open api` never exactly-matches a running `api-x7Kd9a` session and falls through to mint. Spec § "Miss handling — total miss is a hard fail" removes the old TUI-picker-with-filter fallback and mandates the escape-hatch error `nothing resolved for 'blog' — try -f blog`. Pinned `-z` differs from the bare chain: the bare chain swallows any zoxide error and falls through silently, whereas pinned `-z` errors on absence (that pin is Phase 2).

IMPLEMENTATION:
- Status: Implemented (clean, no drift).
- Directory chain + miss: internal/resolver/query.go `QueryResolver.Resolve` (query.go:132-166) — session pre-check via `slices.Contains(names, query)`; path arm via `IsPathArgument`/`ResolvePath` → `PathResult{Domain:"path"}`; alias arm via `qr.aliases.Get` → `validatedPath(path,"alias")`; zoxide arm via `qr.zoxide.Query` with the error swallowed (`err == nil` gate) so any zoxide failure (not-installed / no-match) falls through; total miss → `MissResult{Target: query}`.
- Directory validation: `validatedPath` (query.go:404-409) returns `*DirNotFoundError` for a resolved-but-gone alias/zoxide dir — a hard error distinct from a miss.
- Bare-positional wiring: cmd/open.go `openCmd.RunE` (open.go:278-305) — resolves, emits the decision line (Task 1-4), renders `singleMissError(miss.Target)` on a MissResult, else routes hits through `openResolved`.
- Mint threading: `openResolved` (open.go:345-363) — `*PathResult` arm calls `openPathFunc(cmd, r.Path, command)`, threading the mint-scoped command into session creation.
- Miss message single-source: cmd/open_burst.go `singleMissError` (open_burst.go:108-110) — `"nothing resolved for '%s' — try -f %s"`, target substituted twice. Verified byte-for-byte: em-dash is U+2014 (confirmed via codepoint inspection), the `-f %s` suffix is present.
- Notes: The bare-positional path/alias/zoxide arms of `Resolve` are the Task 1-2 surface; the glob pre-check (Task 1-3) and pinned-domain flags (Phase 2) are correctly kept out of `Resolve`. The command-on-attach usage guard in `openResolved`'s `*SessionResult` arm is a Phase 2 (Task 2-6) addition and does not affect this task's mint threading.

TESTS:
- Status: Adequate.
- Resolver coverage (internal/resolver/query_test.go): TestQueryResolver_Resolve table covers alias hit, alias→zoxide fall-through, zoxide→miss, miss carries raw target, gone alias dir → Directory not found, gone zoxide dir → Directory not found, zoxide-not-installed → silent miss, and session-wins-over-alias precedence. TestQueryResolver_Resolve_PathLikeArguments + _PathLikeNotSentToAliasOrZoxide cover the path arm and that path-like args never touch alias/zoxide. TestQueryResolver_Resolve_SessionDomain "no session match falls through to directory chain" covers the bare-project-mint at resolver level. TestQueryResolver_Resolve_NonExistentResolvedDirectory asserts `*DirNotFoundError` via errors.As.
- cmd coverage (cmd/open_test.go): TestOpenCommand_BareProjectName_MintsNeverAttaches (mints `/Users/lee/Code/api`, asserts openSessionFunc never called with an `api-x7Kd9a` session present); TestOpenCommand_QueryResolution_AliasNotFound (gone alias dir → Directory not found); TestOpenCommand_QueryResolution_ZoxideNotFound (zoxide match to gone dir → Directory not found); TestOpenCommand_TotalMiss_HardFails (exact miss message + TUI never launched + plain error not UsageError); TestOpenCommand_CommandThreadsIntoMintedTarget (`open api -- vim .` threads `[vim .]` into the mint). Byte-identical miss wording also pinned by TestSingleMissError_ByteIdenticalFormat (open_multitarget_test.go).
- Would fail if broken: yes — each assertion targets a distinct behaviour (domain routing, error text, connector selection, command payload).
- Not over-tested overall; the layering (pure-resolver tests + cmd-integration tests) is appropriate rather than redundant. One minor duplicate table row noted below.

CODE QUALITY:
- Project conventions: Followed. Interface-based DI (SessionLister/AliasLookup/ZoxideQuerier/DirValidator), sentinel errors (ErrZoxideNotInstalled/ErrNoMatch, `*DirNotFoundError`), table-driven tests, no t.Parallel, `internal/resolver` stays a pure log-free library (the resolve log line lives only in cmd — matches CLAUDE.md and the spec amendment).
- SOLID: Good. `Resolve` is single-responsibility; the four-way outcome switch is centralised in `openResolved`; the miss string is single-sourced in `singleMissError`.
- Complexity: Low. `Resolve` is a flat first-match-wins chain; each arm is a guarded early return.
- Modern idioms: Good (`slices.Contains`, `strings.Cut`, typed result interface).
- Readability: Good. Comments accurately explain the swallow-zoxide-error rationale and the gone-dir-vs-miss distinction.
- Security/Performance: No concerns — one `ListSessionNames` fetch, no loops of note.
- Issues: none.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/resolver/query_test.go:97-116 — the two TestQueryResolver_Resolve table rows "zoxide miss falls through to miss" (query "unknown") and "miss result carries raw target string" (query "searchterm") exercise the identical code path (zoxide ErrNoMatch → MissResult carrying the raw target) and assert the same fields; merge them into a single row to drop the redundant case.
