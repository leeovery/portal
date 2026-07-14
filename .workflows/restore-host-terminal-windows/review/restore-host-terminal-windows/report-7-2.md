TASK: restore-host-terminal-windows-7-2 — Extract shared gone-session / unsupported-terminal message renderers into internal/spawn/message.go

ACCEPTANCE CRITERIA:
- The literals "gone — nothing opened" and "unsupported terminal — %s · %s — nothing opened" / "no host-local terminal — nothing opened" each appear exactly once, inside internal/spawn/message.go; no other production site hand-builds them.
- The CLI's "spawn: " prefix is applied at CLI call sites only; picker/banner sites render the bare body; the notice band still prepends ⚠ exactly once.
- Rendered output at all seven sites is byte-identical to today.

STATUS: Complete

SPEC CONTEXT: Phase-6/7 host-terminal spawn feature. The spec mandates byte-identical session naming between the CLI (cmd/spawn.go) and the picker (internal/tui) for the pre-flight gone-session and N≥2 unsupported-terminal outcome sentences. Sub-primitives (QuoteJoin/GoneVerb) were already shared in message.go; this task closes the gap by extracting the full sentence so a copy edit can only land in one place. The ⚠ glyph and the "spawn:" prefix are rendering-layer concerns owned by the notice band / CLI respectively, never the message body.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/message.go:40-42 (GoneMessage), :67-72 (UnsupportedNoopMessage) — the two new renderers, thorough doc comments, composed from QuoteJoin+GoneVerb / branch on Identity.IsNull.
  - cmd/spawn.go:138 CLI abort error → fmt.Errorf("spawn: %s", spawn.GoneMessage(gone)); :226 unsupportedSpawnMessage → "spawn: " + spawn.UnsupportedNoopMessage(id); :231-233 logSpawnGone → spawn.LogGone (which calls GoneMessage in logemit.go:103).
  - internal/tui/burst_observability.go:68-70 emitPreflightAbort → spawn.LogGone → GoneMessage.
  - internal/tui/burst_preflight_abort.go:43 picker abort banner and :78 capture-harness seed banner → spawn.GoneMessage.
  - internal/tui/burst_progress.go:450-452 unsupportedFlashText → spawn.UnsupportedNoopMessage (bare body).
- Notes: All seven sites (5 gone + 2 unsupported) route through the single renderer per the tick "Do" list. The CLI log site was wired through spawn.LogGone (logemit.go) rather than the tick's literal "logger.Info(spawn.GoneMessage(gone))" — this is an improvement, not drift: LogGone is the shared, nil-tolerant log-emission shape and it itself calls GoneMessage, so GoneMessage remains the single renderer. No inline fmt.Sprintf of these sentences remains anywhere in production. Grep for the full-sentence literals confirms they appear only in message.go (plus doc-comment mentions). The §6.2 proactive-banner labels in section_header.go:60/65 ("unsupported terminal" / "no host-local terminal", WITHOUT the "— nothing opened" suffix) are a deliberately distinct message and correctly NOT routed through the renderer. The logemit.go:92 LogUnsupported body ("unsupported terminal — nothing opened", identity as structured attrs) is the distinct log form, not the user-facing sentence — not a duplicate.

TESTS:
- Status: Adequate
- Coverage:
  - Unit (internal/spawn/message_test.go): TestGoneMessage covers one name ("'s2' is gone — nothing opened") and ≥2 names ("'s2', 's4' are gone — nothing opened"); TestUnsupportedNoopMessage covers NULL identity ("no host-local terminal — nothing opened") and a named identity with U+00B7 middot ("unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"). Exactly the tick's specified cases.
  - Regression: byte-exact assertions at all consumer sites pass unchanged — cmd/spawn_test.go:652/700/763/801/835 (CLI, "spawn: " prefix present), internal/tui/burst_unsupported_noop_test.go:95/100/139/172/219 (picker flash, bare body), internal/tui/burst_preflight_abort_test.go:118/123/352 (picker banner + ⚠-once via flashWarningGlyph + " " + body), internal/spawn/logemit_test.go:195/211/221 (log body, no "spawn:" prefix). These collectively pin byte-identical output and confirm the ⚠ glyph is prepended once by the render layer, not the body.
- Notes: The byte-exact "want" assertions in message_test.go already prove the absence of a "spawn:" prefix and ⚠ glyph in GoneMessage/UnsupportedNoopMessage output, so no dedicated no-prefix/no-glyph subtest is needed for them (PartialFailureMessage carries one, but adding symmetric guards here would be redundant over-testing given the exact-match). No under- or over-testing.

CODE QUALITY:
- Project conventions: Followed. Renderers live in message.go, the established home for cross-caller message parity alongside QuoteJoin/GoneVerb/PartialFailureMessage; log shapes stay in logemit.go. cmd/spawn.go's thin logSpawn* wrappers are consistent with the existing logSpawnUnsupported/Permission/Summary pattern.
- SOLID principles: Good. Single-responsibility renderers; the "spawn:" prefix and ⚠ glyph decisions stay at their respective render layers (open/closed — a wording change is one edit).
- Complexity: Low. Two straight-line functions; UnsupportedNoopMessage a single IsNull branch.
- Modern idioms: Yes. fmt.Sprintf composition, idiomatic Go.
- Readability: Good. Doc comments explicitly state the no-prefix/no-glyph contract and who owns each.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
