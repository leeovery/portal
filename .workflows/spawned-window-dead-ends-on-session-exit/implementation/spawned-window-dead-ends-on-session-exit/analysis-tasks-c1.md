---
topic: spawned-window-dead-ends-on-session-exit
cycle: 1
total_proposed: 1
---
# Analysis Tasks: Spawned Window Dead-Ends On Session Exit (Cycle 1)

## Task 1: Pin an escaped golden literal in the Ghostty command-composition tests and correct the stale attach fixture
status: approved
severity: medium
sources: standards, architecture

**Problem**: The Ghostty command-composition tests in `internal/spawn/ghostty_command_test.go` are weaker than the spec prescribes at the fix's central quote-nesting seam, and one fixture is stale.
(1) `realAttachArgv()` (lines 9-17) builds its fixture from the retired `attach` verb and omits the `--ack <batch>:<token>` element, so it cannot be what `composeOpenArgv` actually builds (`open --session <name> --ack <batch>:<token>`). Its doc comment falsely claims it is "a representative composed attach argv (as Task 2.3 builds it)," while CLAUDE.md and spec Acceptance Criterion 3 fix the spawned verb as `open --session … --ack …`. The sibling `mintArgvWithSpecials()` fixture already routes correctly through `composeOpenArgv`, making the attach fixture the inconsistent one; left as-is it can mislead a maintainer into thinking `attach` is still the spawned verb.
(2) The wrapper-shape assertions never pin an independent, correctly-escaped golden: `TestWrapWithShellFallback` asserts `wrapped[2] == renderCommandString(cmd) + shellFallbackSuffix` (tautological — reproduces any `renderCommandString` escaping bug on both sides), and `TestGhosttyEmbed`'s round-trips (lines 36-101, 124-164, 249-292) compare against `wrapWithShellFallback(cmd)` after decoding through `decodeRenderedArgv`, a ~47-line hand-rolled, itself-untested POSIX-quote parser that must stay in lockstep with production quoting — so a symmetric encode/decode bug passes falsely. The only independent anchors are two substring fragments. Acceptance criterion #7 explicitly requires asserting the wrapper shape against the correctly-escaped expected string (the `'\''`-escaped nesting, not the schematic form) — exactly the seam most likely to regress.

**Solution**: Correct `realAttachArgv()` so the attach fixture matches the real composed shape and doc comment, then add hand-written golden-literal assertions that pin the full `'\''`-escaped, AppleScript-escaped `command:"..."` body for both the canonical attach argv and the quote-sensitive mint fixture, keeping the existing decoder round-trip as a supplementary (not primary) oracle.

**Outcome**: The Ghostty command-composition tests pin the exact escaped bytes an independent human wrote, so a subtle change in `renderCommandString`'s single-quote / AppleScript escaping is caught rather than mirrored; the attach fixture reflects the real `open --session … --ack …` verb, consistent with the mint fixture and the retired-`attach` reality.

**Do**:
- Rebuild `realAttachArgv()` (internal/spawn/ghostty_command_test.go:9-17) to derive the fixture via `composeOpenArgv` for an attach surface — mirroring how `mintArgvWithSpecials()` is built (e.g. `composeOpenArgv(exePath, path, Surface{Kind: SurfaceAttach, Value: "proj-abc123"}, "batch1", "tok1", nil)`) — or, at minimum, replace `"attach", "proj-abc123"` with `"open", "--session", "proj-abc123", "--ack", "batch1:tok1"`. Confirm the exact `composeOpenArgv` signature and `Surface` constructor in the source before wiring.
- Correct the `realAttachArgv()` doc comment so it accurately describes the composed `open --session … --ack …` shape.
- Add at least one assertion in the wrapper-shape tests (near lines 36-101 / 124-164 / 249-292) that compares the rendered `command:"..."` body against a hand-written string literal containing the full `'\''`-escaped, AppleScript-escaped nesting — one for the canonical attach argv, one for the quote-sensitive mint fixture. The golden literals, not `decodeRenderedArgv` or re-called production functions, must be the primary oracle.
- Keep the existing round-trip decoder checks as supplementary coverage; do not remove them.

**Acceptance Criteria**:
- `realAttachArgv()` emits `open --session proj-abc123 --ack batch1:tok1` (no `attach` verb, `--ack` present) and its doc comment matches the composed shape.
- At least one wrapper-shape assertion pins a hand-written, correctly-escaped golden literal (`'\''`-nested, AppleScript-escaped) for the attach argv and one for the mint fixture, per acceptance criterion #7.
- The golden literals are independent of `renderCommandString` / `wrapWithShellFallback` / `decodeRenderedArgv` — no production function recomputes the expected value.
- No production code changes; the wrap-under-test behaviour is unchanged.

**Tests**:
- `go test ./internal/spawn` passes with the corrected fixture and the new golden-literal assertions.
- Guard-strength sanity check: temporarily perturbing `renderCommandString`'s single-quote escaping makes the new golden-literal assertion fail (it must not pass via symmetric mirroring); revert the perturbation afterward.
