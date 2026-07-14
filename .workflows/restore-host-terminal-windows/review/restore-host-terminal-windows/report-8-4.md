TASK: restore-host-terminal-windows-8-4 — Extract the shared partial-failure leave-what-opened message renderer into internal/spawn/message.go

ACCEPTANCE CRITERIA:
- The partial-failure sentence appears exactly once, inside internal/spawn/message.go; neither cmd/spawn.go nor internal/tui/burst_partial_failure.go hand-builds it.
- The CLI and picker render the same one-line body for the same failed-session set (CLI additionally carries its "spawn: " prefix; picker additionally carries the notice band's ⚠).
- The rendered message honours the spec's "same one-line message" contract for the failed-session naming.
- Tests: unit PartialFailureMessage for one name and ≥2 names (QuoteJoin quoting + verb agreement), bare body no prefix/glyph; cross-caller parity (CLI exit-1 body == picker flash body for same failed set); regression assertions updated to single canonical wording.

STATUS: Complete

SPEC CONTEXT: specification.md:63 pins the CLI contract — a partial spawn failure exits 1 "with the same one-line message the picker would show, on stderr; nothing self-execs." §156/§183/§374 describe leave-what-opened: once past the pre-flight gate, a per-window spawn hiccup (adapter spawn-failed or ack timeout) leaves opened windows in place and names the failed set. The task closes the last gap in cycle 1's message-parity abstraction: the gone/unsupported sentences were already centralised in internal/spawn/message.go; the partial-failure line was hand-built differently in each caller (CLI: "failed to open window(s) for 's2' — others left open"; picker: "'s2' failed to open — others left open"), a broken cross-caller contract.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/message.go:54-56 — new PartialFailureMessage(failed []string) string, bare body "%s failed to open — others left open" composed from QuoteJoin; no spawn: prefix, no ⚠ (documented at 44-53).
  - cmd/spawn.go:210 — CLI renders through it: fmt.Errorf("spawn: %s", spawn.PartialFailureMessage(failed)); prefix applied only at this site.
  - internal/tui/burst_partial_failure.go:114 — picker's burstPartialFailureFlash returns spawn.PartialFailureMessage(failed) bare; the ⚠ is added by the warning notice band (statusGlyph), confirmed by the surrounding comment (96-106) and the pre-spawn/permission/empty-failed branches that correctly bypass it.
- Notes: grep confirms the sentence is built in exactly one place (message.go:55); both callers reference the function, none hand-build it. The old CLI "failed to open window(s) for …" wording is fully gone (grep exit 1). The single canonical wording adopts the picker's phrasing form. Number-agnostic "failed to open" needs no count-aware verb (unlike GoneMessage/GoneVerb), documented at message.go:51-52 — a correct reading of the "verb agreement" acceptance note.

TESTS:
- Status: Adequate
- Coverage:
  - Unit (internal/spawn/message_test.go:64-88): one name → "'s2' failed to open — others left open"; two names → "'s2', 's3' failed to open — others left open" (QuoteJoin quoting + list order); bare-body subtest asserts no "spawn:" prefix and no ⚠ rune. Matches the task's unit test spec.
  - Cross-caller parity: CLI (cmd/spawn_test.go:1012, 1042) asserts err.Error() == "spawn: " + spawn.PartialFailureMessage(...); picker (internal/tui/burst_partial_failure_test.go:142, 245) asserts rm.flashText == spawn.PartialFailureMessage(...). Asserting through the shared renderer (rather than a duplicated literal) makes parity structural — a copy edit to the renderer moves both expectations in lockstep, so the assertions cannot silently rot into divergence.
  - Regression / degenerate guards: pre-spawn error and empty-failed cases assert the flash does NOT contain "failed to open" (burst_partial_failure_test.go:166, 288), pinning the no-degenerate-empty-named-body contract.
- Notes: Well-balanced — three focused unit subtests, no redundant assertions. The "several names" unit subtest title mentions list order and the s2,s3 → 's2', 's3' expectation does verify order via QuoteJoin. No over-testing.

CODE QUALITY:
- Project conventions: Followed — leaf renderer in internal/spawn composed from the existing QuoteJoin primitive, sibling to GoneMessage/UnsupportedNoopMessage; the prefix/glyph split (bare body; CLI adds prefix; band adds ⚠) is applied identically to the established renderers. Test naming/structure matches the package's it-does-X subtest convention.
- SOLID principles: Good — single-responsibility renderer, single source of truth for the sentence; callers depend on the abstraction not a literal.
- Complexity: Low — one Sprintf line.
- Modern idioms: Yes.
- Readability: Good — the doc comment (44-53) is precise about the no-verb / no-prefix / no-glyph contract and why.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
