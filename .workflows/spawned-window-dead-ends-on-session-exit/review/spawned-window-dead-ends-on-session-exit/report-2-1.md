TASK: spawned-window-dead-ends-on-session-exit-2-1 — Pin an escaped golden literal in the Ghostty command-composition tests and correct the stale attach fixture

ACCEPTANCE CRITERIA:
1. realAttachArgv() emits `open --session proj-abc123 --ack batch1:tok1` (no `attach` verb, `--ack` present) and its doc comment matches the composed shape.
2. At least one wrapper-shape assertion pins a hand-written, correctly-escaped golden literal ('\''-nested, AppleScript-escaped) for the attach argv and one for the mint fixture, per spec acceptance criterion #7.
3. The golden literals are independent of renderCommandString / wrapWithShellFallback / decodeRenderedArgv — no production function recomputes the expected value.
4. No production code changes; the wrap-under-test behaviour is unchanged.

STATUS: complete

SPEC CONTEXT:
The fix (spawned-window-dead-ends) wraps the native Ghostty adapter's window command in `bash -lc '<composed open argv>; exec "$SHELL" -il'`. Spec §"Constraints" and acceptance criteria #2/#3/#7 are load-bearing here: the wrapper is a SCHEMATIC form, not a byte template — the inner per-element single quotes must be re-escaped for the outer -lc single-quote layer via the POSIX '\'' close-escape-reopen idiom, then AppleScript-escaped for the osascript `command:"…"` double-quoted literal. Acceptance criterion #7 explicitly requires unit tests to "assert the wrapper shape against the correctly-escaped expected string (the '\''-escaped nesting, not the schematic form)" and to exercise the '\'' double-escaping path with a quote-sensitive fixture (the mint `-- <command…>` passthrough). This task is the test-quality hardening that makes that assertion a genuine (non-tautological) oracle and fixes the stale attach fixture that still used the retired `attach` verb.

IMPLEMENTATION:
- Status: Implemented (correct, matches the preferred approach in the task's "Do")
- Location: internal/spawn/ghostty_command_test.go
  - realAttachArgv() (lines 17-25): now derives the fixture via composeOpenArgv(Surface{Kind: SurfaceAttach, Value: "proj-abc123"}, "batch1", "tok1", nil) — the PREFERRED approach mirroring mintArgvWithSpecials(), not the minimal string swap. Produces `/usr/bin/env -u TMUX -u TMUX_PANE PATH=… /abs/portal open --session proj-abc123 --ack batch1:tok1`. The retired `attach` verb is gone and `--ack batch1:tok1` is present. AC1 met.
  - Doc comment (lines 9-16): rewritten to accurately describe the composed `open --session <name> --ack <batch>:<token>` shape and explicitly states "(the retired `attach` verb is never emitted)". The false "as Task 2.3 builds it" / escape-neutral-only claim is gone. AC1 met.
  - New golden constants wantAttachCommandBody (line 61) and wantMintCommandBody (line 63): raw string `const`s. AC2 met.
  - New TestGhosttyEmbedGoldenLiteral (lines 348-375): two subtests asserting ghosttyEmbed(realAttachArgv()) == wantAttachCommandBody and ghosttyEmbed(mintArgvWithSpecials()) == wantMintCommandBody, plus a strings.Contains check that the golden lands verbatim inside the `command:"…"` property. AC2/AC3 met.
- Notes: The task commit (c2f95fa0) touches ONLY internal/spawn/ghostty_command_test.go (plus workflow bookkeeping .tick/tasks.jsonl + manifest.json). No production file (ghostty.go / command.go / recipe.go) changed → AC4 met. The existing decoder round-trips (TestGhosttyEmbed, reverseAppleScriptEscape, decodeRenderedArgv) are retained unchanged as supplementary coverage per the task's explicit instruction.

INDEPENDENT GOLDEN-LITERAL VERIFICATION (hand-derived, not trusting the test):
I re-derived both goldens from the production primitives (shellQuote in recipe.go:98, renderCommandString recipe.go:110, wrapWithShellFallback ghostty.go:56, ghosttyEmbed ghostty.go:80, composeOpenArgv command.go:47, FormatSpawnAckFlag ackid.go:77):
- Attach: composeOpenArgv → argv → wrapWithShellFallback payload (each element single-quoted, `; exec "$SHELL" -il` appended) → renderCommandString over ["bash","-lc",payload] (payload's single quotes each become '\'') → AppleScript escape (every `\` doubled → the '\\'' signature, then `"` → \"). The derived byte string matches wantAttachCommandBody exactly.
- Mint: same pipeline over the quote-sensitive passthrough element `echo 'a';$x"b"`. I derived the passthrough region char-by-char; the doubled-escape run `'\\''\\'\\'''\\''` and the AppleScript-escaped `;$x\"b\"` match wantMintCommandBody exactly (17-char inner-quote run verified byte-for-byte), and the `--path`/`/abs/dir`/`--` env-prefix pattern matches the attach derivation.
Both goldens are correct.

GUARD-STRENGTH (independence) VERIFICATION:
The task requested an empirical guard-strength check (perturb renderCommandString's single-quote escaping, run the suite, confirm the golden assertion FAILS, then revert). I did NOT execute that step: as the review-task verifier I am bound by a hard no-test-execution constraint and a no-source-mutation constraint (my only sanctioned file write is this report; temporarily editing production source and running `go test` violates both). I performed the equivalent verification statically, which establishes the same property with certainty:
- wantAttachCommandBody / wantMintCommandBody are `const` raw string literals. Neither calls renderCommandString, shellQuote, wrapWithShellFallback, ghosttyEmbed, or decodeRenderedArgv.
- TestGhosttyEmbedGoldenLiteral compares `ghosttyEmbed(...)` against those frozen consts with `!=`. Because the expected side is fixed bytes and the actual side flows through the escaping code, perturbing shellQuote/renderCommandString's single-quote escaping changes only the actual side → the `!=` fires → the assertion FAILS. This is definitionally a non-tautological, independent oracle; the perturbation would fail exactly as the task predicts.
- Contrast the pre-existing TestWrapWithShellFallback (lines 166, 186), which computes `wantPayload := renderCommandString(cmd) + shellFallbackSuffix` — still tautological — but it is a structural shape test, not the anti-tautology oracle. The new embed-level golden fully subsumes the wrapped payload (ghosttyEmbed is a deterministic transform of wrapWithShellFallback's output), so criterion #7's "correctly-escaped expected string" requirement is satisfied.

TESTS:
- Status: Adequate
- Coverage: Golden-literal assertions for both the escape-neutral canonical attach argv AND the quote-sensitive mint passthrough fixture (the '\'' double-escaping path). Supplementary decoder round-trips retained. shellFallbackSuffix const (line 44) independently pins `; exec "$SHELL" -il`, matching production ghostty.go:57.
- Notes: Not over-tested — the golden test and the decoder round-trip are intentionally complementary (primary independent oracle vs. supplementary symmetric round-trip), exactly as the task prescribes. Both subtests also verify the golden lands verbatim in the `command:"…"` property, tying the embed to the actual osascript template. No t.Parallel (per CLAUDE.md). Unit-lane pure-function tests (no daemon, no build tag) — correctly placed.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel; TestXxx + descriptive t.Run subtests consistent with the surrounding file; unit-lane placement correct. Raw-string goldens documented with an accurate rationale comment.
- SOLID principles: N/A (test code); good separation between fixture builders, oracle constants, and assertions.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — the golden constants carry a thorough comment explaining the two escaping layers and the '\\'' doubled-backslash signature, which materially aids future maintainers reading an otherwise dense escaped literal.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/ghostty_command_test.go:166,186 — TestWrapWithShellFallback still asserts against `renderCommandString(cmd) + shellFallbackSuffix` (a production-recomputed, tautological expected value). The analysis that spawned this task flagged this as concern #2. It is not blocking — the new embed-level golden subsumes the wrapped payload and satisfies criterion #7 — but the residual tautology could be removed by asserting `wrapped[2]` against a hand-written golden payload literal (the pre-AppleScript-escape form of the wantAttachCommandBody/wantMintCommandBody inner region). Optional; low value given the embed golden already pins the same bytes downstream.
