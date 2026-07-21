TASK: Wrap Ghostty window command in shell fallback and drop wait after command (spawned-window-dead-ends-on-session-exit-1-1)

ACCEPTANCE CRITERIA:
1. ghosttyOpenScript(command) emits the wrapper form — AppleScript-escaped rendering of the 3-element argv [bash, -lc, renderCommandString(command) + suffix `; exec "$SHELL" -il`], built via wrapWithShellFallback + renderCommandString (close-escape-reopen nesting, never naive concatenation).
2. ghosttyOpenScript(command) output contains no `wait after command` for any input.
3. Composed open argv inside the wrapper byte-identical to today's renderCommandString(composeOpenArgv(...)) for BOTH surface kinds (attach + mint incl. `-- <command...>` passthrough), retaining the `/usr/bin/env ... PATH=<...> -u TMUX -u TMUX_PANE` prefix; PATH not stripped.
4. Quote-sensitive mint fixture (single quote, semicolon, dollar, double-quote) round-trips uncorrupted through the bash -lc layer, exercising the double-single-quote escape path (doubled-backslash signature).
5. composeOpenArgv, renderCommandString, shellQuote bodies unchanged; AttachConnector/syscall.Exec attach path, connector selection, @portal-spawn ack ordering, trigger path, custom terminals.json adapters untouched.
6. Full suite green: go build -o portal . && go test ./...

STATUS: complete

SPEC CONTEXT:
The bug: a burst-spawned (N-1 external) native-Ghostty window dead-ends ("Process exited. Press any key to close the terminal.") when its session exits/detaches, because the window's only process is a one-shot exec chain (bash -c -> env -> portal open -> syscall.Exec into tmux attach) with no parent interactive shell, compounded by the adapter's `wait after command:true`. The trigger window is exempt (self-attaches in-process from a Portal child of the login shell). Spec §"The Fix" scopes the fix entirely to internal/spawn/ghostty.go: wrap the composed argv as `bash -lc '<composed argv>; exec "$SHELL" -il'` and drop `wait after command`. Spec is explicit that the wrapper is schematic, not a byte template: the inner per-element single quotes must nest correctly via the shared shell-quote helper's `'\''` idiom, not naive concatenation. Shared composeOpenArgv/renderCommandString are explicitly out of scope (a shell wrap there would inject `;`/`exec` into the {command} handed to direct-exec terminals.json adapters). Spec AC 2/3/7 are the code-verifiable criteria; the live-window behaviour (AC 1) was already validated live during investigation and no manual deliverable is gated.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/ghostty.go:56-59 — wrapWithShellFallback: `payload := renderCommandString(command) + `; exec "$SHELL" -il``; returns ["bash", "-lc", payload]. Payload left un-quoted here (the outer renderCommandString owns the nesting), exactly as the Do step prescribes.
  - internal/spawn/ghostty.go:80-85 — ghosttyEmbed: `embedded := renderCommandString(wrapWithShellFallback(command))` then the two unchanged AppleScript-escape passes in load-bearing order (backslash-doubling BEFORE quote-escaping).
  - internal/spawn/ghostty.go:26-28 — ghosttyScriptTemplate reduced to the single-field record `new window with configuration {command:"%s"}`; `wait after command:true` removed.
  - Doc comments (ghostty.go:8-25, 30-55, 61-79) fully rewritten to describe the wrapper layer and why `wait after command` is dropped, and why the explicit wrapper (not implicit-append) is required under Ghostty's `exec -l` model. Manual-test doc (ghostty_openwindow_manual_test.go:19-41) updated for the new behaviour.
- Notes:
  - I hand-derived both golden literals through the two escaping layers (renderCommandString over [bash -lc PAYLOAD] with `'\''` nesting, then AppleScript backslash-double + quote-escape) and both match the committed constants byte-for-byte — the escaping seam is real, not a mirrored recomputation.
  - Scope confirmed via git: the only non-test production file changed across the entire work unit is internal/spawn/ghostty.go (task commit 3a275753). command.go/recipe.go (composeOpenArgv/renderCommandString/shellQuote) were last touched by the unrelated cli-verb-surface-redesign feature — bodies unchanged. AttachConnector/ack ordering/trigger path/terminals.json adapters untouched (AC5 satisfied).
  - wrapWithShellFallback is argv-agnostic and sits at the adapter, so both burst entry points (picker multi-select + `portal open` multi-target) inherit the fix via the shared Adapter.OpenWindow seam (AC — both surfaces). No external consumers of ghosttyEmbed/ghosttyOpenScript/wrapWithShellFallback exist outside ghostty.go + its tests.
  - PATH not stripped: wrapWithShellFallback consumes `command` verbatim, so the `/usr/bin/env -u TMUX -u TMUX_PANE PATH=<...>` prefix survives inside the payload (AC3).

TESTS:
- Status: Adequate
- Coverage (internal/spawn/ghostty_command_test.go):
  - TestWrapWithShellFallback — exact [bash -lc <render>+suffix] shape for attach and the argv-agnostic mint case.
  - TestGhosttyOpenScript — single `command` property / no stale `surface configuration`; NO `wait after command` across 4 input shapes incl. specials and a backslash/quote element (AC2); the bash -lc wrapper opening + AppleScript-escaped `exec \"$SHELL\" -il` tail; percent-inert; spaced-session-name preservation.
  - TestGhosttyEmbed — attach round-trip (decode of reversed-AppleScript-escape == wrapWithShellFallback(cmd)); quote-sensitive mint round-trip asserting the doubled-backslash `'\\''` signature AND peeling the suffix to decode the inner argv back to `cmd` byte-identically (AC4); PATH/-u TMUX prefix preserved inside the wrapper (AC3).
  - TestGhosttyEmbedGoldenLiteral — the PRIMARY independent oracle: hand-authored frozen literals for attach and the quote-sensitive mint fixture, never recomputed from a production function, so a symmetric encode/decode escaping bug cannot pass falsely (AC7). Also asserts the golden lands verbatim inside `command:"..."`.
- Notes:
  - mintArgvWithSpecials uses `echo 'a';$x"b"` — carries all four required specials (single quote, `;`, `$`, `"`), so the `'\''` double-escaping path is genuinely exercised, not an escape-neutral attach argv (AC4/AC7 met precisely).
  - Not over-tested: the round-trip decoder (decodeRenderedArgv, ~45 lines) coexists with the golden literals, but they assert different properties — round-trip proves no corruption, golden pins the exact escaped bytes. The task (AC4 + AC7) explicitly requires BOTH, and the golden is the deliberate independent oracle against symmetric-encode/decode false-passes. Justified, not bloat.
  - Not under-tested: both surface kinds, PATH preservation, wait-absence over multiple shapes, and the quote-sensitive path are all covered. The osascript exec boundary is correctly left to the //go:build manual test per spec (no new automated real-Ghostty lane).
  - shellFallbackSuffix is duplicated as a test-local const (not imported) so the test pins the exact expected bytes independently of the production literal — appropriate.

CODE QUALITY:
- Project conventions: Followed. Pure helper with a small, single-responsibility surface; DI seams (osascriptRunner) untouched; leaf-package discipline intact. Doc comments are thorough and explain the load-bearing escape order and the exec-l rationale.
- SOLID principles: Good. wrapWithShellFallback is a pure, single-purpose function; ghosttyEmbed composes it with the existing render/escape layers without reaching into their internals. Nesting responsibility correctly delegated to the shared renderCommandString.
- Complexity: Low. wrapWithShellFallback is 2 lines; ghosttyEmbed is a 3-line pipeline. No branching added.
- Modern idioms: Yes. Idiomatic Go; raw-string literals used for the suffix to keep the escaping legible.
- Readability: Good. Intent and the escaping invariants are documented at the exact point they matter.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (AC6 "full suite green" was not executed here: the verifier role is read-only — Bash is limited to the output-file rename and test execution is prohibited — so I verified the code and tests statically. The implementation is well-formed and self-consistent, the touched package has no dangling references, and suite-green is covered by the build/test lanes. This is a scope note, not a finding, and proposes no change.)
