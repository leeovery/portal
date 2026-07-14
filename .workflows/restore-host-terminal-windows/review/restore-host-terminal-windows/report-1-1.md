TASK: 1.1 — Package scaffold + Identity model + bundle-id family matching (restore-host-terminal-windows-1-1)

ACCEPTANCE CRITERIA:
- internal/spawn compiles and `go test ./internal/spawn/...` passes.
- NewIdentity("dev.warp.Warp-Stable", "") ⇒ IsNull()==false, BundleID=="dev.warp.Warp-Stable", Name=="Warp".
- NewIdentity("", "Ghostty").IsNull() is true (empty bundle id ⇒ NULL even with an app name).
- NewIdentity("com.example.MyTerm", "") ⇒ non-NULL passthrough carrying the raw id + derived name.
- MatchesFamily("dev.warp.Warp-Stable", "dev.warp.Warp-*") true; MatchesFamily("com.apple.Terminal", "com.apple.Terminal") true; cross-family false; MatchesFamily("anything", "*") true.

STATUS: Complete

SPEC CONTEXT:
Spec *Terminal Identity & Detection → Identity resolution*: the system-blessed identity is the terminal's macOS bundle id, matched by bundle-id family (e.g. `dev.warp.Warp-*`), channel-aware; remote/mosh resolve to a NULL bundle id → unsupported → honest no-op. Spec *User-facing display: both*: the banner and `--detect` show both the friendly `.app` name and the exact bundle id, so `Identity` must carry both. Scope boundary (per phase-1-tasks.md): Phase 1 builds only the value type + the standalone family-matching primitive; the friendly-alias table and the identity→adapter resolver are Phase 2. The friendly-name derivation algorithm is an explicit implementation choice the spec does not pin beyond "non-empty, human-readable" — the Task 1.2 walk supplies the true `.app` name when available.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/doc.go (package comment); internal/spawn/identity.go:17 (Identity struct), :24 (IsNull), :39 (NewIdentity), :60 (deriveName), :84 (MatchesFamily).
- Notes: All five acceptance criteria satisfied exactly.
  - Identity{BundleID, Name} value type; IsNull() == (BundleID=="") — NULL defined solely by empty bundle id (identity.go:24-26).
  - NewIdentity trims bundleID, returns zero Identity on empty/whitespace-only regardless of appName, and always produces a passthrough for any non-empty id (identity.go:39-51). appName preferred (trimmed) when non-empty, else deriveName.
  - deriveName: last dot segment then trim channel suffix at first '-', with an empty-segment fallback to the full bundle id so Name is never empty for a non-empty id (identity.go:60-72). Matches the spec-approved derivation.
  - MatchesFamily uses path.Match semantics; the doc comment correctly justifies why `*` (which does not cross `/`) matches the whole bundle-id remainder including the channel suffix, since bundle ids contain no `/`. A path.ErrBadPattern is treated as a non-match — a defensible choice given Phase 2 patterns are Portal-controlled family globs.
  - doc.go describes the shared spawn service (detection, adapter resolution, window spawning) reached in-process by picker + `portal spawn`, and honestly scopes Phase 1 vs later phases.
  - No drift. No scope creep — only the Phase-1 surface (value type + matching primitive) was built; no Phase 2 resolver/alias table leaked in.

TESTS:
- Status: Adequate
- Coverage: internal/spawn/identity_test.go.
  - TestNewIdentity (table): channel-suffixed derived name (Warp); empty-id-with-appName NULL; whitespace-only NULL; unknown-id passthrough (MyTerm); appName-preferred; apple Terminal derivation; leading/trailing-whitespace trim. Covers every NewIdentity acceptance criterion plus the trim and derive edge cases.
  - TestNewIdentity_NeverEmptyNameForNonEmptyBundleID: pins the never-empty-Name invariant for the `com.example.-Stable` empty-segment collapse — exercises the deriveName fallback branch (identity.go:68).
  - TestMatchesFamily (table): channel-suffix vs family glob; exact literal match; channel-suffixed id rejected by exact literal; bare `*` catch-all; cross-family rejection. Covers all four MatchesFamily acceptance assertions plus the "exact pattern rejects a suffixed id" refinement.
- Notes: Tests assert observable behaviour (return values), not internals. No over-testing — each case pins a distinct branch; no redundant assertions. Would fail if the feature broke (e.g. dropping the channel-suffix trim, the NULL rule, or the family-glob semantics). Test names follow the project's "it ..." convention (the plan-listed "empty or whitespace-only" case is split into two entries — a slight, harmless improvement over the listed name).

CODE QUALITY:
- Project conventions: Followed. Idiomatic Portal/Go: value type with pointer-free methods, small pure functions, table-driven white-box tests in `package spawn` (unit lane), no external deps, comprehensive doc comments. Reuses stdlib `path.Match` rather than hand-rolling a matcher (the plan permitted either and required documenting the choice — done).
- SOLID principles: Good. Single-responsibility helpers (deriveName, MatchesFamily); the matching primitive is a free function with no premature Phase-2 coupling.
- Complexity: Low. Straight-line logic, no nesting beyond a couple of guards.
- Modern idioms: Yes. strings.TrimSpace / LastIndex / IndexByte, path.Match.
- Readability: Good. Intent is self-documenting and doc comments explain the non-obvious path.Match `*`-vs-`/` reasoning and the never-empty-Name fallback.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/spawn/identity_test.go:100 (TestMatchesFamily) — add one table case asserting a malformed pattern is a non-match (e.g. bundleID "x", pattern "[", want false) to cover the documented path.ErrBadPattern branch at identity.go:86-88, which is currently the one untested code path.
