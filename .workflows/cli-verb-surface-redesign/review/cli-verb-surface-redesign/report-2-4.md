TASK: cli-verb-surface-redesign-2-4 — `-z/--zoxide` pin — zoxide-domain mint, explicit not-installed error, hard-fails on no match

ACCEPTANCE CRITERIA (from Phase 2 task table + phase AC):
- zoxide not installed → explicit `ErrZoxideNotInstalled` (distinct from the bare chain's silent fall-through)
- no match → hard-fail, never pops the picker
- resolved dir validated to exist before mint
- `-z` never runs session/path/alias matching (zoxide-domain only)
- (phase AC) `-z/--zoxide <query>` mints at the best match and hard-fails on no match; every pin hard-fails on unresolvable and never pops the TUI picker

STATUS: Complete

SPEC CONTEXT:
Spec § Domain-pinning flags (line 104, 109): `-z/--zoxide <query>` pins to zoxide best match — "mint at matched dir; hard fail on no match; **explicit error if zoxide not installed**." The note is explicit: "`-z` differs from the guessing chain on zoxide-absence: pinned `-z` **errors** when zoxide is not installed (`ErrZoxideNotInstalled`), whereas the bare-target chain treats any zoxide error as 'continue to next domain' (falls through silently)." Spec § Pinned-domain contract (line 114-115): every domain pin hard-fails on unresolvable and never falls back to the TUI picker — a spawned window or script must never pop a TUI. Spec § Burst exec-argv (line 202): mint targets reduce to a literal existing directory at resolve time, which is why the resolved dir is validated before mint.

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria exactly)
- Location:
  - internal/resolver/query.go:376-389 — `ResolveZoxidePin`: queries zoxide-domain only, surfaces `ErrZoxideNotInstalled` verbatim via `errors.Is` branch, maps any other query error to a plain `"No zoxide match for: <query>"` hard error, and validates the best-match dir on disk via `validatedPath` (→ `*DirNotFoundError` if gone).
  - internal/resolver/query.go:404-409 — `validatedPath`: disk existence check before returning a `PathResult{Domain:"zoxide"}` (mint) — this is the "resolved dir validated to exist before mint" guarantee.
  - internal/resolver/query.go:158-160 (Resolve, bare chain) — swallows every zoxide error (`if path, err := qr.zoxide.Query(query); err == nil`), so the bare chain silently falls through to the miss tail. This is the deliberate contrast the pin diverges from.
  - internal/resolver/zoxide.go — `ErrZoxideNotInstalled` sentinel + `Query` returns it when `lookPath("zoxide")` fails, `ErrNoMatch` on non-zero exit.
  - cmd/open.go:259-272 — `pinDispatch` table wires `{"zoxide", (*resolver.QueryResolver).ResolveZoxidePin}`; the shared `resolvePinAndOpen` (309-328) reads the flag, resolves in-domain, and hands a hit to `openResolved` (mint via `openPathFunc`). A pin miss returns the error (exit 1) and never reaches the picker; no resolve log line is emitted for a pin.
  - cmd/open.go:1002 — `-z/--zoxide` flag registered with help text naming the explicit-not-installed behaviour; no completion func registered (delegates to shell, per spec Tab Completion).
  - cmd/open_targets.go:30 — `-z`/`--zoxide` mapped to the "zoxide" domain in `openTargetPins`; single `-z` target is not glob-expandable (`globExpandableDomain` excludes zoxide, cmd/open_burst.go) so it routes to the single-target pin path, not the burst.
  - cmd/open_surfaces.go:84-96 — burst path (Phase 3) reuses `ResolveZoxidePin`; `ErrZoxideNotInstalled` aborts the whole resolve, no-match/gone-dir become collected misses — consistent with the pin primitive.
- Notes: `-z` is strictly zoxide-domain: `ResolveZoxidePin` touches only `qr.zoxide` and `qr.dirValidator`, never `qr.sessions` or `qr.aliases`. The `dirValidator.Exists` call is disk validation of the resolved zoxide dir (an AC requirement), not path-domain matching — correctly scoped.

TESTS:
- Status: Adequate (well-layered, focused, not redundant)
- Coverage:
  - Resolver unit (internal/resolver/query_test.go:980-1070, `TestQueryResolver_ResolveZoxidePin`): best-match → `PathResult{Domain:"zoxide"}`; not-installed → `ErrZoxideNotInstalled` (errors.Is); no-match → exact `"No zoxide match for: nope"` AND asserts it is NOT `ErrZoxideNotInstalled` (distinguishes the two failure modes); gone best-match dir → `*DirNotFoundError` "Directory not found: /gone/dir". The resolver factory (`newZoxidePinResolver`) injects `failingSessionLister` + `failingAliasLookup` that fail the test if consulted — so every sub-case doubles as the "zoxide-domain only, never session/path/alias" guard.
  - Bare-chain contrast (internal/resolver/query_test.go:135-144, "zoxide not installed skipped silently"): the bare `Resolve` swallows `ErrZoxideNotInstalled` → `MissResult`. This is the exact contrast that proves the pin's divergence is real and tested on both sides.
  - cmd integration (cmd/open_test.go): `ZoxidePin_Mints_NoPicker` (mints, never attaches, never picker), `ZoxidePin_NotInstalled_ErrorsNoPicker` (errors.Is ErrZoxideNotInstalled + no picker/no mint), `ZoxidePin_NoMatch_HardFailsNoPicker` (exact message + no picker + not a `*UsageError` → exit 1), `ZoxidePin_ThreadsCommandIntoMint` (command threading), `ZoxidePin_EmitsNoResolveLine` (deterministic — no resolve decision line).
  - Guard: `TestOpenTargetPinsCoverValueTakingFlags` (cmd/open_targets_guard_test.go) keeps `-z`/`--zoxide` in the argv-scan pin map in lockstep with the live flag set.
- Notes: The resolver-layer and cmd-layer both exercise not-installed and no-match, but at different layers (resolution logic vs. wiring + never-pops-picker contract) — legitimate layering, not redundancy. Every AC edge case has a direct test; each would fail if the behaviour broke (message strings, sentinel identity, picker-not-called flags all asserted). Not over-tested.

CODE QUALITY:
- Project conventions: Followed. Small-interface DI (`ZoxideQuerier`, `DirValidator`), method-value pin dispatch table, house-style capitalised user-facing messages with the `//nolint:staticcheck` directive matching sibling pins ("No session found" / "No alias found"). `internal/resolver` stays a pure log-free library; the resolve line lives only in cmd.
- SOLID principles: Good. `ResolveZoxidePin` is single-responsibility; the four pins share the `resolvePinAndOpen` body and a one-row dispatch table (adding a pin touches one place). Sentinel returned directly (documented) so callers can `errors.Is` it.
- Complexity: Low. `ResolveZoxidePin` is a linear query → classify → validate.
- Modern idioms: Yes — `errors.Is` sentinel discrimination, method values.
- Readability: Good. Doc comments state the zoxide-domain-only contract, the explicit-vs-silent divergence, and the DirNotFound-vs-no-match distinction.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
