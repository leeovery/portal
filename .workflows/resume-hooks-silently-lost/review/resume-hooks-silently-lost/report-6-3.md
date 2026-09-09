TASK: resume-hooks-silently-lost-6-3 — Restore internal/hooks To A Leaf By Moving The Id Vocabulary Into Its Own Package (tick-548309)

ACCEPTANCE CRITERIA:
- `go list -deps ./internal/hooks` returns no `tmux`, `state`, `session`, `project` or `resolver`.
- The new package imports nothing outside the standard library.
- The shape predicate and the generator's alphabet/width remain in one package, so a width change cannot desynchronise them.
- Every existing call site of `NewPaneToken` / `IsTokenShaped` behaves identically.

STATUS: complete

SPEC CONTEXT:
The specification's §3.2 requires the token-shape predicate's home to be "beside the generator", with the shape "derived from the generator's own constants, not restated" — the invariant that keeps `hooks.json`'s staleness rule and the pane-token mint from drifting (a restated `^[A-Za-z0-9]{6}$` in `internal/hooks` beside a movable width is the named failure mode: every persisted key silently stops being judgeable and the reaper starts retaining what it should delete, with nothing failing anywhere). The spec body originally named `internal/session` as that home and explicitly sanctioned an `internal/hooks` → `internal/session` import; two corrigenda supersede it and are authoritative:
- Corrigendum 2026-08-30 (specification.md:550): the home is `internal/nanoid`, a stdlib-only leaf, and `internal/hooks/leaf_guard_test.go` now forbids the `internal/session` import the body had sanctioned.
- Corrigendum 2026-09-01 (specification.md:560): the `internal/session/panetoken.go` forwarder was later deleted, `cmd/hooks.go` reaches the leaf directly, and the leaf holds two unexported widths — general-purpose `width` behind `NewGenerator`, `paneTokenWidth` behind `NewPaneTokenGenerator` (the width `IsTokenShaped` reads).
The delivered state matches the corrigenda exactly. CLAUDE.md's architecture table already carries the `nanoid` row describing this shape.

IMPLEMENTATION:
- Status: Implemented (delivered at 56a245d5; the pane-token/general width split and the forwarder deletion landed in later phase-8 tasks, per the 2026-09-01 corrigendum — the current tree is judged, per "the code is the source of truth").
- Location:
  - internal/nanoid/nanoid.go:1-6 — package doc, following the `internal/shellquote` leaf precedent verbatim in form (why-it-is-a-leaf stated).
  - internal/nanoid/nanoid.go:17 `Alphabet`, :22 `width`, :26 `Generator`, :29 `NewGenerator`, :35 `NewPaneTokenGenerator`, :39 `generatorOfWidth`, :58 `paneTokenWidth`, :65 `IsTokenShaped`.
  - internal/nanoid/nanoid.go imports: `crypto/rand`, `fmt`, `strings` — stdlib only (nanoid.go:8-12).
  - internal/hooks/store.go:15 imports the leaf; :258 is the sole shape test in the staleness rule (`key == "" || nanoid.IsTokenShaped(key)`).
  - internal/session/naming.go:7,14 — `IDGenerator` survives as a type alias for `nanoid.Generator`; `NanoIDAlphabet`, `suffixLen` and `NewNanoIDGenerator` are gone from `internal/session`.
  - Call sites all route through the leaf: cmd/hooks.go:46,100; cmd/open.go:419,572; internal/spawn/ackid.go:30, burst.go:58; internal/session/naming.go:14; internal/hookstest/hooks.go:30,130-139,153.
- Notes:
  - AC1 verified structurally (I do not run commands beyond the report rename): the only module-internal imports in `internal/hooks`'s non-test sources are fileutil, log, nanoid, storelog (store.go:12-16); `internal/storelog`'s only internal imports are fileutil and log (clean_stale.go); `internal/fileutil`, `internal/log` and `internal/nanoid` import no module-internal package at all. The transitive set is therefore exactly {fileutil, log, storelog, nanoid} — no tmux, state, session, project or resolver. `internal/hooks/leaf_guard_test.go` pins this both ways (direct source imports + `go list -deps`), so the criterion is enforced rather than merely met.
  - AC3: `NewPaneTokenGenerator` (nanoid.go:36) and `IsTokenShaped` (nanoid.go:66) both read `paneTokenWidth`, and `generatorOfWidth` (nanoid.go:46) and `IsTokenShaped` (nanoid.go:70) both read `Alphabet` — derivation, not restatement. No second declaration of the shape survives anywhere: a repo-wide search for a restated width or charset test (`{6}` regexp, `len(key) == 6`) finds none, and `internal/hookstest` sizes its seed vocabulary from the mint itself (hooks.go:27-34) rather than from a literal.
  - AC4: at the delivering commit `session.NewPaneToken` forwarded to `nanoid.NewGenerator()` — same alphabet, same width 6 — so behaviour was identical; the later width split moved the mint to `NewPaneTokenGenerator` with both widths still 6, so no call site's observable behaviour changed at any point. No dangling references to the removed `session.NewNanoIDGenerator` / `session.NanoIDAlphabet` / `session.IsTokenShaped` / `session.NewPaneToken` / `suffixLen` remain in Go sources or in README/CLAUDE.md (only historical `.workflows/` documents of prior work units mention them, which is correct — they are records, not references).

TESTS:
- Status: Adequate
- Coverage:
  - internal/hooks/leaf_guard_test.go:29-48 — the dependency guard the task asked for, in the repo's established style and driven by `sourceguardtest`. Two arms: a source-walking check that no non-test source imports outside the allowlist (:30-42), and `AssertDepsWithin` over the transitive `go list -deps` set across both lanes (:44-48). `AssertDepsWithin` refuses to pass vacuously (fatal on an empty set, on a set not holding the package itself, and on an allowlist that matches nothing), so a guard that stopped looking fails rather than reports green.
  - internal/nanoid/leaf_guard_test.go:14-18 — the stdlib-only assertion for the new package: an empty allowlist plus `ForbiddingThirdParty()`, over both lanes. This is the direct test of AC2.
  - internal/nanoid/nanoid_test.go:40-96 — the `IsTokenShaped` table moved with the predicate: accepts, wrong widths, out-of-alphabet bytes, empty, multi-byte (both a six-byte and a six-rune fixture, with the fixtures' own byte/rune counts asserted first so the case cannot silently stop testing what it names), and every old-format `<name>:<w>.<p>` key shape.
  - internal/nanoid/nanoid_test.go:98-136 — the derive-from-the-generator property in its new home: every one of 200 minted pane tokens is token-shaped, a token one byte short is rejected (which is what would fail if the two widths ever separated), and the mint is distinct per call.
  - internal/nanoid/nanoid_test.go:10-21 — `Alphabet` pinned against an independent literal plus the load-bearing absence of `.`, `:`, `-`, ` ` (the `<batch>-<token>` marker split depends on it).
  - Beyond the task's own list, the width contract is covered where it is actually consumed: internal/hooks/persisted_key_width_test.go:17-44 judges a *literal* six-character key exactly as a shipped Portal wrote it (so a width move fails on real persisted data, not only where a fixture rebuilds itself), and cmd/hooks_pane_token_width_guard_test.go:17-52 pins the production `TokenMinter` to `nanoid.NewPaneTokenGenerator` by AST — necessary precisely because both widths are 6 today, so swapping the generator would otherwise be invisible. That guard is fatal if it finds no `TokenMinter` assignment at all.
- Notes:
  - Not under-tested: each acceptance criterion has a test that would fail if the criterion broke. The one criterion with no direct assertion is AC4 ("call sites behave identically"), which is correct — it is a refactor-equivalence claim, carried by the unchanged existing suites at each call site (cmd/hooks_seams_test.go:34, cmd/hooks_pane_token_test.go:45, internal/spawn/ackid_test.go:139, internal/session/naming_test.go:193) rather than by a new test.
  - Not over-tested: no redundant restatement of the table in the old location (internal/session/tokenshape.go and tokenshape_test.go were deleted, not copied), and the alphabet assertion lives in exactly one place.
  - Collision-based assertions (200 draws from 62^6) carry a ~3.5e-7 flake probability — negligible, and they are the pre-existing shape moved unchanged.
  - Lane rule respected: every test here is unit-lane, builds no binary and spawns no daemon; both leaf guards nevertheless take a reading under the integration tag too via `sourceguardtest.Lanes()`, so a dependency admitted only behind `//go:build integration` cannot slip past.

CODE QUALITY:
- Project conventions: Followed. The new package mirrors the repo's established leaf pattern (`internal/shellquote`, `internal/tmuxerr`, `internal/tmuxout`, `internal/xdg`, `internal/storelog`) down to the package-doc form that states why the package is a leaf and which mutually-unimportable packages share it. CLAUDE.md's architecture table carries the `nanoid` row. No logging is introduced into a leaf (correct — nanoid is outside the closed component vocabulary and must not import `internal/log`).
- SOLID principles: Good. One reason to change (the id vocabulary), and the dependency inversion is strengthened: `internal/state` and `internal/tmux` — the packages that own `PortalPaneIDOption` and the capture column carrying the token — can now reach the predicate, which the previous `hooks → session` edge had structurally locked out.
- Complexity: Low. `generatorOfWidth` is a single closure shared by both constructors, so the two mints cannot diverge in anything but width.
- Modern idioms: Yes. `for i := range len(s)` (range-over-int), `strings.IndexByte` for the byte-wise membership test, type alias rather than a wrapper for `session.IDGenerator`.
- Readability: Good. The comments carry the reasoning that is not visible in the code and nothing else: why `-` is absent from `Alphabet` (nanoid.go:14-16), why the general-purpose `width` is free to move while `paneTokenWidth` is a migration event (nanoid.go:19-21, :52-58), and why length and membership are both counted in bytes (nanoid.go:62-64). Each of those claims holds against the code as written; none restates it, and none references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
