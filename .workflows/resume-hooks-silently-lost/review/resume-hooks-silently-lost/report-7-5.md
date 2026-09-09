TASK: resume-hooks-silently-lost-7-5 — "The Generated-Id Width Is A hooks.json On-Disk Contract Shaped As A Generic Generator Knob" (give the pane token its own named width, decoupling `hooks.json`'s key-recognition contract from the general-purpose generator width)

ACCEPTANCE CRITERIA:
- The pane-token width is a separate named constant from the generic generator width, and `IsTokenShaped` reads the pane-token one.
- The pane-token mint and `IsTokenShaped` read the same constant, so a token the mint produces is always token-shaped.
- Changing the generic `width` alone leaves every existing `hooks.json` key token-shaped and the reaper's behaviour unchanged.
- Changing the pane-token width alone fails the new literal-key test.
- The pane-token width's doc names the `hooks.json` consequence; `width`'s doc no longer claims the recognition coupling.
- Session-name suffixes and spawn/ack ids are unchanged in width and charset.

STATUS: complete

SPEC CONTEXT:
The spec's §3.2 fixes the token-shape predicate's home "beside the generator" and requires the shape be *derived* from the generator's own constants rather than restated, precisely so recognition cannot drift from generation. Its §5.2 staleness rule then makes that shape load-bearing on disk: a persisted key absent from the live set is deleted only when token-shaped or empty; any other shape is retained forever. Two corrigenda at the spec's end (2026-08-30 and 2026-09-01) are authoritative here and record exactly this task's outcome — `internal/nanoid` holds "two unexported widths — a general-purpose `width` behind `NewGenerator` and `paneTokenWidth` behind `NewPaneTokenGenerator`, which is the width `IsTokenShaped` reads", the split "decouples `hooks.json`'s on-disk key-recognition contract from the session-name suffix width", and "a source guard pins the mint to the pane-token generator, since both widths are 6 today and the swap would otherwise be invisible". The implementation matches the corrigenda.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/nanoid/nanoid.go:22` — `width`, redocumented as general-purpose ("Nothing persisted is classified by it, so it is free to move: pane tokens carry their own width"); read only by `NewGenerator` (`:29`).
  - `internal/nanoid/nanoid.go:52-58` — `paneTokenWidth`, documented as part of `hooks.json`'s on-disk contract and as a migration event, declared beside `IsTokenShaped`.
  - `internal/nanoid/nanoid.go:65-66` — `IsTokenShaped` reads `paneTokenWidth`.
  - `internal/nanoid/nanoid.go:35-49` — `NewPaneTokenGenerator` mints at `paneTokenWidth` via the shared `generatorOfWidth(n)` helper, so mint and predicate read the one constant.
  - `cmd/hooks.go:100` — the production pane-token mint (`seams.TokenMinter = nanoid.NewPaneTokenGenerator()`); this is the only pane-token minting site in the tree. (The task's commit pointed `internal/session/panetoken.go`'s `NewPaneToken` at the new generator; a later phase deleted that forwarder and moved the mint here, which the 2026-09-01 corrigendum records. The coupling the criterion asks for survives the move.)
  - `internal/hookstest/hooks.go:27-34, 123-155` — the seed-key vocabulary now sizes itself from the mint (`sync.OnceValue` over `NewPaneTokenGenerator`) with `fitPrefix` padding the legible prefix, so a width move carries the fixtures rather than breaking them.
  - Independence verified across the tree: `nanoid.NewGenerator` is reached only from `cmd/open.go:419`, `cmd/open.go:572` (session-name suffixes) and `internal/spawn/burst.go:58` (`NewID`, spent on the batch id at `burst.go:81` and each ack token at `:87`). None of those values is classified by width anywhere — the spawn marker splits on `-`, which the alphabet excludes — so moving `width` alone cannot reclassify a persisted hook key.
  - `internal/hooks/store.go:258` remains the single staleness rule and still routes shape judgement through `nanoid.IsTokenShaped`, unchanged.
- Notes: `paneTokenWidth` is declared after the function that reads it, which Go permits and which keeps it adjacent to `IsTokenShaped` as the task asked. Alphabet is untouched, so charset invariance holds for every id domain.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooks/persisted_key_width_test.go:17-44` — the data-level pin the task asked for: `persistedKey = "k3f9qz"` is a literal, not derived from `nanoid.Alphabet`, and the two subtests are exactly the named ones ("it judges a literal six-character persisted key token-shaped", "it reaps a persisted key authored at the pane-token width when the pane is gone"). The reap subtest stages the key through `hookstest.StageStore`, runs `CleanStale` against an empty live set, asserts the removed set and re-loads the file to prove the entry is gone. It would fail on both counts if `paneTokenWidth` moved, satisfying the fourth criterion on data rather than on a fixture's panic.
  - `internal/nanoid/nanoid_test.go:98-136` — "it recognises every token the pane-token mint produces" (200 mints, each asserted token-shaped) and "it rejects a key one byte short of the pane-token width"; `:59-65` carries "it rejects a key carrying a character outside the alphabet".
  - `internal/session/naming_test.go:191-200` — "it leaves session-name suffix width independent of the pane-token width", asserting the suffix width against a freshly minted general-purpose id rather than a literal, so the two widths are free to diverge.
  - `internal/hookstest/hooks_test.go:46` — "it authors every token-shaped seed at the pane-token width" pins the fixture vocabulary to the mint.
  - `cmd/hooks_pane_token_width_guard_test.go:17` (added by a later phase, after the mint moved) — an AST guard failing any `TokenMinter` assignment that draws on `NewGenerator`, plus `cmd/hooks_seams_test.go:34` asserting the production minter's output is token-shaped. Together these keep the second criterion enforced now that the two widths are numerically equal and a swap would otherwise be silent.
  - Every one of these is hermetic and unit-lane: temp dirs only, no tmux, no daemon, no binary build, no `t.Parallel()`.
- Notes: No over-testing found. The literal-key reap subtest overlaps mechanically with `internal/hooks/cleanstale_snapshot_test.go`'s derived-key reap case, but its subject is different and load-bearing — the derived case rebuilds itself around a new width and would keep passing, which is the exact blind spot this task exists to close.

CODE QUALITY:
- Project conventions: Followed. `internal/nanoid` remains stdlib-only (`crypto/rand`, `fmt`, `strings`), so `internal/nanoid/leaf_guard_test.go`'s empty-allowlist assertion still holds. `internal/hooks`'s leaf guard scans production sources only, so the new external-test-package import of `hookstest` creates no edge. CLAUDE.md's `nanoid` row and "Resume hooks" section were updated in the same commit and are still accurate against the tree after the later mint move.
- SOLID principles: Good. The shared `generatorOfWidth(n)` removes the duplication the second generator would otherwise have introduced, and the two exported constructors read as two named domains over one mechanism.
- Complexity: Low.
- Modern idioms: Yes — `for i := range len(s)` in the predicate, `sync.OnceValue` for the fixture-side width derivation.
- Readability: Good. Both width doc comments state a consequence rather than restating the value, and each names its own domain.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
