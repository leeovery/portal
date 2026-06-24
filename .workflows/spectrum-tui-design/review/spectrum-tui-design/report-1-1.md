TASK: spectrum-tui-design-1-1 — `vhs` capture harness: install/verify, committed tapes dir, fixture-seeded foundation capture, Paper reference PNG export pipeline (tick-09ba3b)

ACCEPTANCE CRITERIA:
- Two vhs runs of testdata/vhs/sessions-flat.tape from a clean checkout yield byte-comparable captures (determinism is the gate; NOT a frame compare).
- Committed testdata/vhs/ holds the runnable tape, the captured PNG (current design), and the committed Paper reference export; capture tool + fakes + fixtures committed under cmd/capturetool + internal/capture.
- The shipped portal binary is unchanged: no new command, and it does not import the capture fakes/fixtures package (assert via import check).
- Capture never touches the real tmux server or real ~/.config/portal: opens no server; any config dir is isolated/none.
- The capture tool builds the SAME TUI model production uses (shared tui.Build constructor; no bespoke render path).
- Harness doc records install/verify (both paths), the sandbox + quoted-path gotchas, and the paper-MCP re-export command.
- Compare mechanism documented as agent/user-judged layout/structure/colour-role match, NOT a pixel-diff CI gate.

STATUS: Complete

SPEC CONTEXT:
§15 mandates a permanent vhs capture harness as the objective visual gate every later reskin task self-verifies through. §15.2 prescribes vhs + one .tape per canonical screen committed under testdata/vhs/, fixed terminal size + known fixture seeding. §15.4/§15.5 mandate committed per-task PNGs and a committed Paper reference export so no live MCP is needed at implement/CI time, with the compare being agent/user-judged (layout/structure/colour-role), never a pixel diff. The tick task supersedes the spec's literal "Type portal" tape with a SEPARATE harness binary (Option A) that skips bootstrap and fakes every tmux seam — the safety decision that prevents bootstrap step-4 SweepOrphanDaemons from SIGKILLing the developer's real daemon. The §1 parity bar applies to the shared-constructor refactor (provably cosmetic / behaviour-preserving).

IMPLEMENTATION:
- Status: Implemented (fully)
- Location:
  - cmd/capturetool/main.go — separate package main harness binary; --fixture / --appearance flags; resolveProgram → resolveModel → tui.Build; alt-screen run; no bootstrap, no tmux server, no config read.
  - internal/capture/fakes.go — every tmux seam faked (lister/projectStore/enumerator/reader return canned data; killer/renamer/creator/editors are no-ops).
  - internal/capture/fixtures.go — FixtureByName / FixtureNames + the deterministic sessions-flat set (exact 12-session Paper-mock list with window counts + attached flags); later-phase fixtures co-located.
  - internal/tui/build.go — the shared tui.Build(Deps) constructor (the 1-1 refactor); compiler-enforced Deps struct.
  - cmd/open.go:380 buildTUIModel — production path routes through tui.Build (field-for-field mapping, Build owns option assembly).
  - testdata/vhs/sessions-flat.tape, sessions-flat.png, README.md, reference/sessions-modern-vivid-v2.png, trail/sessions-flat/1-1.png — all git-tracked.
- Notes: The shared-constructor lift is behaviour-preserving: build.go mirrors the legacy inline cmd/open.go construction one-for-one (same nil-guards, always-injected InitialMode, same post-construct WithCommand/WithInitialFilter/WithInsideTmux order). The ScrollbackReader fake's Tail(string)([]byte,error) matches the real seam exactly. The harness has since grown well past 1-1 scope (12+ fixtures, swatch for 1-9), but every 1-1 foundation deliverable is present and correct.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/capturetool/import_guard_test.go — TestPortalBinaryDoesNotImportCapture (production stays clean) PLUS the positive control TestCaptureToolDoesImportCapture (guards against a vacuous pass). Uses `go list -deps` anchored at project root.
  - cmd/capturetool/shared_constructor_test.go — asserts BOTH cmd/open.go AND cmd/capturetool call tui.Build( (anti-drift guard).
  - cmd/capturetool/main_test.go — resolveModel known/unknown/empty-fixture + invalid-appearance error paths; resolveAppearance dark/light/invalid.
  - internal/capture/capture_test.go — TestFixtureByName pins the exact ordered 12-session set; TestFakeSeamsAreInert asserts mutators are no-ops and read seams return canned data; plus per-fixture builders verified against the production model via tui.Build.
- Notes: Determinism (the headline acceptance gate) is verified structurally — the fakes return copies of fixed in-memory data and read no external state — and documented as a runnable shasum check in the README. The byte-comparable two-run gate is a runtime/vhs property (cannot be unit-tested without vhs), which is the correct boundary: the test suite proves the *inputs* are deterministic; the README documents the *runtime* verification. Not over-tested: each fixture test asserts one observable contract (page/title/seam data), not implementation detail.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (per CLAUDE.md mandate). Seams are small interfaces with fakes; harness scaffolding kept out of production (internal/capture import-guarded). Doc comments are thorough and spec-cross-referenced.
- SOLID principles: Good. Single shared construction chokepoint (DRY); Deps struct is the compiler-enforced seam contract (ISP/DIP); fixtures depend on the tui.Deps abstraction, not concrete tmux.
- Complexity: Low. resolveProgram/resolveModel/resolveAppearance are flat and linear; the one branch (contrast-validation swatch bypassing tui.Build) is documented and justified.
- Modern idioms: Yes. Idiomatic flag parsing, error-wrapping with %q context, env via os.LookupEnv per no-color.org convention.
- Readability: Good. Intent is self-documenting and every non-obvious decision (alt-screen, OSC 11 restore, swatch exception, copy-on-read for determinism) carries a comment.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] testdata/vhs/README.md:96 — the determinism gate is documented as a manual shasum check; consider a future make target / scripted helper that runs the tape twice and cmp's the PNGs so the byte-comparable gate is reproducible without copy-paste. (Decide whether worth the maintenance; out of 1-1 scope.)
- [do-now] testdata/vhs/sessions-flat.tape:33 — the `Sleep 4s` first-paint pad is a fixed magic number with only an inline rationale; add a one-word note that it covers `go run` compile + first render (the README explains it, the tape could echo it) so a future editor doesn't trim it and flake the compile-on-cold-cache case.
