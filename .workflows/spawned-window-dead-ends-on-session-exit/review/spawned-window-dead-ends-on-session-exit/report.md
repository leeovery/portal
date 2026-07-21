# Implementation Review: Spawned Window Dead-Ends On Session Exit

**Plan**: spawned-window-dead-ends-on-session-exit
**QA Verdict**: Approve

## Summary

The fix is correct, tightly scoped, and adequately tested. The native Ghostty adapter (`internal/spawn/ghostty.go`) now wraps the composed open argv as `bash -lc '<composed open argv>; exec "$SHELL" -il'` via a pure `wrapWithShellFallback` helper rendered through the shared `renderCommandString`, and `wait after command` is dropped from the osascript template — exactly the two changes the spec prescribes. The quote-nesting seam at the heart of the fix is verified by a hand-authored golden literal (independent of the production escaping functions) that both verifiers re-derived byte-for-byte, so a symmetric encode/decode escaping bug cannot pass falsely. The stale `realAttachArgv()` fixture was corrected to the real `open --session … --ack …` shape. The only non-test production file changed across the entire work unit is `internal/spawn/ghostty.go`; the shared composition, attach path, ack ordering, trigger path, and custom `terminals.json` adapters are all untouched. Build succeeds and the full unit lane is green. One optional non-blocking quick-fix remains (a residual tautology in a pre-existing structural test), which is fully subsumed by the new embed-level golden.

## QA Verification

### Specification Compliance

Implementation aligns with the specification.

- **AC2** (wrapper form + no `wait after command`): `ghosttyEmbed` renders `renderCommandString(wrapWithShellFallback(command))` with the load-bearing AppleScript-escape order (backslash-doubling before quote-escaping) preserved; `ghosttyScriptTemplate` reduced to `new window with configuration {command:"%s"}`. `wait after command` appears nowhere in production — only in comments and the negative test assertion.
- **AC3** (composed argv unchanged, PATH preserved): `wrapWithShellFallback` consumes `command` verbatim, so the `/usr/bin/env -u TMUX -u TMUX_PANE PATH=<…>` prefix survives inside the payload for both attach and mint surfaces.
- **AC7** (correctly-escaped golden, quote-sensitive fixture): `TestGhosttyEmbedGoldenLiteral` pins hand-written `'\''`-nested, AppleScript-escaped literals for the canonical attach argv and the quote-sensitive mint fixture (`echo 'a';$x"b"` — carrying single quote, `;`, `$`, `"`), exercising the double-single-quote escaping path. Both verifiers independently re-derived the goldens from the production primitives and confirmed byte-for-byte matches.
- **Scoping** (AC5/6): shared `composeOpenArgv`/`renderCommandString`/`shellQuote` bodies unchanged; `AttachConnector`/`syscall.Exec` attach path, connector selection, ack ordering, trigger path, and `terminals.json` adapters untouched. The wrap lives at the adapter, so both burst entry points inherit it via the shared `Adapter.OpenWindow` seam.

No deviations found.

### Plan Completion

- [x] Phase 1 acceptance criteria met (Task 1-1: wrapper + dropped `wait after command`)
- [x] Phase 2 acceptance criteria met (Task 2-1: golden literal pinned, stale attach fixture corrected)
- [x] All tasks completed (2/2)
- [x] No scope creep — sole production change is `internal/spawn/ghostty.go`

### Code Quality

No issues found. `wrapWithShellFallback` is a 2-line pure helper with a single responsibility; `ghosttyEmbed` composes it with the existing render/escape layers without reaching into their internals. Doc comments were rewritten to explain the wrapper layer, the load-bearing escape order, and why the explicit `bash -lc` wrapper (not implicit-append) is required under Ghostty's `exec -l` model. No `t.Parallel` (per CLAUDE.md); unit-lane placement correct.

### Test Quality

Tests adequately verify requirements — neither under- nor over-tested.

- Golden-literal assertions serve as the primary independent oracle (frozen `const` raw strings, no production-function recomputation); the decoder round-trips are retained as complementary supplementary coverage, exactly as the task prescribes. AC4 (round-trip) and AC7 (golden) assert different properties, so both are justified.
- Coverage spans both surface kinds, PATH preservation, `wait after command` absence across multiple input shapes, percent-inertness, and the quote-sensitive double-escaping path.
- The osascript exec boundary is correctly left to the `//go:build manual` test per spec — no new automated real-Ghostty lane, as required.

### Required Changes (if any)

None.

## Recommendations

### Quick-fixes

1. `internal/spawn/ghostty_command_test.go:166,186` — remove residual tautology in `TestWrapWithShellFallback` (Report 2-1)
   - The test asserts `wrapped[2]` against `renderCommandString(cmd) + shellFallbackSuffix`, a production-recomputed expected value (analysis concern #2). Not blocking — the new embed-level golden (`TestGhosttyEmbedGoldenLiteral`) subsumes the wrapped payload and satisfies criterion #7. Could be tightened by asserting `wrapped[2]` against a hand-written golden payload literal (the pre-AppleScript-escape inner region). Low value given the embed golden already pins the same bytes downstream.
