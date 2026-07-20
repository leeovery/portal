TASK: cli-verb-surface-redesign-2-2 — `-p/--path` pin — path-domain mint, dir must exist

ACCEPTANCE CRITERIA (from Phase 2 task table + phase AC):
- A dir whose name contains glob metacharacters (`~/tmp/foo[1]`) is reachable via `-p` (pin bypasses the Phase-1 glob pre-check).
- Non-existent dir hard-fails (DirNotFound), never pops the picker.
- Tilde/relative-path expansion reused from `ResolvePath`.
- `-p` never runs session/alias/zoxide matching (path-domain only).
- (Phase AC) `-p/--path <dir>` mints and requires the dir to exist; pins hard-fail on unresolvable and never pop the picker.

STATUS: Complete

SPEC CONTEXT:
Spec § Domain-pinning flags (line 103): `-p/--path <dir>` | directory path | mint new session; dir must exist. Spec § Pinned-domain contract (line 115): every domain pin hard-fails on unresolvable and never falls back to the TUI picker; `--path` mints per Axiom 2 on a hit and hard-fails on a miss; only bare positionals run the guessing chain, only `-f` opens the picker. The glob-named-dir escape hatch is the raison d'être of `-p`: a `foo[1]` dir is unreachable as a bare positional (routed to the burst as a session glob → zero match → hard-fail) but reachable via `-p` because it stats the literal path.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/resolver/query.go:325-331 — `ResolvePathPin(dir)` reuses `ResolvePath` (tilde/relative expansion + existence + is-directory validation), returns `PathResult{Domain:"path"}`; touches no session/alias/zoxide seam. Bypasses `validatedPath` by design (ResolvePath already validates).
  - internal/resolver/path.go:23-41 — `ResolvePath` (shared expansion/validation) and ExpandTilde:60-76.
  - cmd/open.go:259-272 — pinDispatch table wires `"path" → (*resolver.QueryResolver).ResolvePathPin`; dispatched via the shared `resolvePinAndOpen` helper (open.go:317-328), which returns any resolve error BEFORE `openResolved`, so a miss never reaches the picker.
  - cmd/open.go:1000 — `-p/--path` flag registered; help copy accurate ("path-domain; dir must exist").
  - cmd/open_targets.go:29 — `-p → "path"` domain mapping; cmd/open_burst.go:76-83 `globExpandableDomain` excludes "path", so a single `-p` target is never routed to the burst (falls through to the single-pin dispatch).
- Notes: The glob-bypass is a natural consequence of `ResolvePath` calling `os.Stat` on the literal path — metacharacters are never expanded. Path-domain isolation is structural (ResolvePathPin references none of the other seams). A single `-p` target correctly falls through the multi-target gate to the single-pin path. No drift from the plan.

TESTS:
- Status: Adequate
- Coverage:
  - Resolver level (internal/resolver/query_test.go:708-830, `TestQueryResolver_ResolvePathPin`) — builds the resolver with FAILING session/alias/zoxide seams (fatal if consulted), directly proving path-domain-only. Sub-cases: existing dir → PathResult{Domain:"path"}; glob-named dir `foo[1]` reachable & mints; non-existent dir → "Directory not found:" hard-fail; a file (not dir) → "not a directory:" hard-fail; `~` tilde expanded to home. Directly covers all four acceptance edge cases.
  - cmd dispatch level (cmd/open_test.go): PathPin_Mints_NoPicker (mints, never attaches, never opens picker), PathPin_GlobNamedDir_Mints, PathPin_ThreadsCommandIntoMint, PathPin_EmitsNoResolveLine (deterministic pins emit no resolve line). `-f` + `-p` mutual-exclusion covered at open_test.go:2533 and 2627.
  - Guard: open_targets_guard_test.go:82 `TestOpenTargetPinsCoverValueTakingFlags` keeps the `-p`/`--path` domain-map entry in lockstep with the live cobra flag set.
- Notes: Well-balanced — the failing-seam resolver harness is the right way to prove path-domain isolation without redundant assertions; no over-testing observed. One small symmetry gap (see non-blocking): unlike `-s` (TestOpenCommand_SessionPin_Miss_HardFailsNoPicker), there is no dedicated cmd-level `-p` non-existent → no-picker test; that behaviour is structurally guaranteed by the shared `resolvePinAndOpen` (error returned before `openResolved`) and the resolver-level hard-fail case, so coverage is adequate, not deficient.

CODE QUALITY:
- Project conventions: Followed. Small interface + method-value dispatch table matches the codebase DI pattern; the `//nolint:staticcheck` house-style user-facing-message convention is respected in ResolvePath.
- SOLID principles: Good. ResolvePathPin has a single responsibility and delegates expansion/validation to the shared ResolvePath (no duplication of the tilde/stat logic).
- Complexity: Low. ResolvePathPin is a 4-line delegate; dispatch is table-driven.
- Modern idioms: Yes. Method values in the pinDispatch table keep a new pin to one table row.
- Readability: Good. The ResolvePathPin doc comment (query.go:313-324) accurately explains the glob-bypass rationale and why it skips validatedPath.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_test.go — add a `TestOpenCommand_PathPin_Miss_HardFailsNoPicker` mirroring the `-s` pin's miss test (open_test.go:447): assert `open -p <nonexistent>` returns the "Directory not found" error and calls neither openTUIFunc nor openSessionFunc/openPathFunc. Behaviour is already structurally guaranteed and resolver-level tested; this closes the pin test-symmetry gap. Low priority.
