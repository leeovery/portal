TASK: spectrum-tui-design-1-3 — MV role-token colour layer: closed ~20-token vocabulary, pinned DARK variants, centralising scattered dark literals (tick-5517eb)

ACCEPTANCE CRITERIA (from tick-5517eb):
- Every §2.9 dark token exists with the exact pinned hex.
- The scattered dark literals in session_item.go / project_item.go / model.go are replaced by role-correct tokens; a guard test confirms no raw hex survives at those sites.
- state.green only on live/positive, state.red only destructive, text.faint only decorative — verified per re-pointed site.
- The token representation lets 1-4 add the light variant without re-pointing call sites.
- go build and go test pass; dark render structure unchanged (colour-source centralisation; hex moves to MV palette verified vs §2.9).

STATUS: Complete

SPEC CONTEXT:
§2.1 mandates designing to semantic role tokens, never scattered literal hex. §2.8 makes the role-token layer a single named built-in theme (theme-readiness) with a closed ~20-token set. §2.9 is the authoritative MV token table — pins exact DARK hexes against the dark canvas #0b0c14, defines the role-discipline rules (state.green = live/positive only, never chips/decoration; state.red = destructive only; text.faint = decorative only and never functional text; a functional-metadata grey maps to text.detail). Closed-vocabulary rule: every rendered colour is one of these tokens, no literal hex outside the token layer.

IMPLEMENTATION:
- Status: Implemented (foundation intact at HEAD; later phases evolved the renderers but the token layer is unchanged and still the single source).
- Location:
  - internal/tui/theme/theme.go — Token{Name,Light,Dark} struct; Theme struct with all 20 named role tokens; MV instance with every §2.9 dark hex pinned exactly; All() returns the 20-token slice; ColorFor(mode) resolver; Mode (Dark zero-value, Light).
  - Re-pointed sites (verified at HEAD): session_item.go:394 cursor/bar → AccentViolet, :397/:402 detail → TextDetail, :446 attached → StateGreen, :263 heading → TextDetail, :264 count → TextDim; project_item.go:115 path → TextDetail, :141 bar → AccentViolet; model.go:913 paginator active dot → AccentViolet, :917 inactive dot → TextFaint.
  - Guard: internal/tui/colour_literal_guard_test.go.
- Notes: The original 1-3 commit (58cbe923) re-pointed exactly the 9 literals the task named (cursor 212, detail #777777, attached 76, project #777777, footer key grey → accent.blue, separator grey → text.faint, signpost #888888 → text.strong, flash #888888 → text.on-warning). Subsequent phases (2-x..9-x) restructured model.go's flash/signpost/footer rendering into dedicated files (notice_band.go, section_header.go, etc.), but every colour still flows from a token — the foundation held. All 20 dark hexes match §2.9 byte-for-byte. The struct carries Light + Dark per token, so 1-4 filled light values with zero call-site edits (confirmed: light variants now present, no renderer re-pointed). Build of internal/tui/... is GREEN.

TESTS:
- Status: Adequate
- Coverage:
  - theme_test.go::TestMVDarkVariantsPinned — table-asserts all 20 dark hexes against the §2.9 values (the build target, not the PNG/old screen — correct source).
  - theme_test.go::TestMVTokenCount — pins the closed 20-token vocabulary (7 text + 6 accent + 7 surface) so the palette can't silently grow.
  - theme_test.go::TestEachTokenCarriesLightVariant — proves the Light slot is a settable seam resolving independently of Dark (the 1-4-without-re-point guarantee).
  - colour_literal_guard_test.go::TestNoRawColourLiteralAtCentralisedSites — AST-walks every non-test production .go in internal/tui (glob, not allowlist) and fails on any lipgloss.Color(<string|int literal>); the theme/ subpackage is correctly excluded as the sole sanctioned home for raw hex.
- Notes: Focused, not over-tested. Role-discipline (green/red/faint) is enforced by reading the re-pointed sites + reviewer audit rather than an automated assertion — acceptable for this task (the rule is documented at each site and the guard covers the literal-hex half). The guard models on the internal/log single-owner guard pattern as the task asked. The glob-based coverage is a strict improvement over the original 11-file allowlist (closes the later-phase blind spot). Live verification of the guard's effectiveness: zero raw lipgloss.Color hex/ANSI literals survive anywhere in internal/tui/*.go (confirmed by independent grep).

CODE QUALITY:
- Project conventions: Followed. Small interface-free value types, package doc comment present, spec-section cross-references throughout. Idiomatic Go (table tests, AST inspection via go/ast). Tests do not use t.Parallel().
- SOLID principles: Good. Single centralised token layer (SRP); Theme is the stable interface §2.1 promises; ColorFor isolates mode resolution.
- Complexity: Low. ColorFor is a one-branch resolver; All() is a flat slice; guard is a straightforward AST visitor.
- Modern idioms: Yes. go/ast guard, lipgloss.Color, image/color return type.
- Readability: Good. Every token row carries its §2.9 role comment; erratum/derivation comments document each light correction (1-4 territory, but well-documented).
- Issues: One stale doc reference (see NON-BLOCKING NOTES) — the package doc claims a "dark-pinned Color() convenience" method that does not exist (only ColorFor(mode) exists). Non-functional; pure documentation drift.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/theme/theme.go:10-11 — Remove the stale sentence referencing the non-existent "dark-pinned Color() convenience" method; only ColorFor(mode) exists in the package (confirmed: no Token.Color() method defined anywhere). Pure comment fix, zero logic impact.
- [idea] internal/tui/theme/theme_test.go — Consider a test that asserts the role-discipline rule mechanically (e.g. state.green never co-occurring with a chip/decoration call path), to lock §2.9's green/red/faint reservation the way the hex guard locks the literal-hex rule. Currently that half of the rule rests on reviewer audit + per-site comments. Requires judgement on how to express "live/positive-only" as an assertion, so not a mechanical quickfix.
