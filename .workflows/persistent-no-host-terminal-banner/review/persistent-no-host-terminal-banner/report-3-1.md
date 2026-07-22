TASK: Rewrite UnsupportedNoopMessage (Both Shapes) + Lockstep Copy Assertions (persistent-no-host-terminal-banner-3-1 / tick-45dc48)

ACCEPTANCE CRITERIA:
- UnsupportedNoopMessage(Identity{}) returns exactly "can't open new windows over a remote connection — nothing opened".
- UnsupportedNoopMessage(Identity{Name:"Apple Terminal", BundleID:"com.apple.Terminal"}) returns exactly "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened" (U+00B7 middle dot, U+2014 em-dash, U+0027 apostrophe).
- unsupportedFlashText(id) (unchanged) returns the two new strings via delegation.
- spawn suite passes: TestUnsupportedNoopMessage asserts new strings; TestGoneMessage/TestPartialFailureMessage/TestQuoteJoin/TestGoneVerb unchanged.
- tui suite passes: TestUnsupportedFlashText, TestBurstUnsupported_NonNullAtomicNoOp, TestBurstUnsupported_NullFlash, TestBurstUnsupported_DeferredThenUnsupported assert new strings; TestBurstUnsupported_SupportedStillDispatches unchanged.
- No production/test file still contains the two old literals.
- logemit.go Info("unsupported terminal — nothing opened", …) + its test assertion UNCHANGED.
- GoneMessage / PartialFailureMessage / permission-guidance copy UNCHANGED.
- Function edit + both suites in one green commit.

STATUS: complete

SPEC CONTEXT:
Spec §5 (Plain-Language Rewrite) golden table (line 126) mandates exactly the two new strings this task ships:
NULL → "can't open new windows over a remote connection — nothing opened"; named → "can't open new windows in <name> · <bundleID> — nothing opened". §5 rationale (line 131): "can't open new windows" is the plain statement, "nothing opened" stays to honestly signal an attempt occurred (distinguishing the reactive no-op from the pre-emptive banner). §8 keeps message.go in scope; adjacent spawn copy (GoneMessage, permission guidance, the persistent banner, the logemit static line) out of scope. Verified the implementation strings are byte-for-byte identical to the §5 golden copy.

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria and spec §5 exactly)
- Location:
  - internal/spawn/message.go:79-84 — UnsupportedNoopMessage rewritten. NULL branch (L81) and named fmt.Sprintf template (L83) both migrated; id.Name/id.BundleID args, the · separator and the trailing "— nothing opened" clause preserved. Doc comment (L68-78) updated to the plain-language copy, retaining the "both callers render through it" + "no spawn: prefix / no ⚠ glyph" notes and adding the accurate "only place the user sees the bundle id (terminals.json key)" note.
  - internal/tui/burst_progress.go:460-462 — unsupportedFlashText delegates verbatim to spawn.UnsupportedNoopMessage; NOT touched by the commit (confirmed via git show --name-only), tracks the new copy automatically. Correct per task.
- Byte-exactness (hexdump-verified in both source and every test want):
  - "can't" uses straight ASCII apostrophe U+0027 (0x27) in both branches — NOT typographic U+2019.
  - Middle dot between name and bundle id is U+00B7 (c2 b7).
  - Em-dash before "nothing opened" is U+2014 (e2 80 94), single.
- Commit scope (81d09740) is exactly right: message.go, message_test.go, burst_unsupported_noop_test.go (+ .tick/manifest bookkeeping). Does NOT touch logemit.go/logemit_test.go, burst_progress.go, or unsupported_banner_test.go.
- Regression greps (—include=*.go, .workflows planning docs excluded):
  - Old NULL literal "no host-local terminal — nothing opened": NOT found in any .go file.
  - Old named literal "unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened": NOT found in any .go file.
  - Persistent named banner "⚠ unsupported terminal — Apple Terminal · com.apple.Terminal" (no "— nothing opened"): PRESENT and UNCHANGED in internal/capture/fixtures.go:455 and internal/tui/unsupported_banner_test.go:57,136,145 — the naïve "unsupported terminal — " replace did NOT hit it.
  - logemit static Info line: PRESENT and UNCHANGED at internal/spawn/logemit.go:130 and its assertion at internal/spawn/logemit_test.go:305.
- Notes: None. No drift from plan or spec.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/message_test.go:106-121 TestUnsupportedNoopMessage — NULL subtest (L107-112) and named subtest (L114-120) pin both new strings byte-exact. NULL input Identity{} pins both the remote/mosh case and the transient-detection-error-folded-to-Identity{} edge in one assertion (correct per the edge-case note; no redundant transient-error test). Subtest names describe the new copy, no stale old-copy names. TestGoneMessage/TestPartialFailureMessage/TestQuoteJoin/TestGoneVerb left intact.
  - internal/tui/burst_unsupported_noop_test.go — all five expected occurrences updated: named literal at the flash-table row (L97), NonNullAtomicNoOp (L145), DeferredThenUnsupported (L229); NULL literal at the flash-table row (L102) and NullFlash (L182). Adjacent stale comment prose ("honest no-host-local line" → "plain remote-connection line") updated at the file header (L10-12), the TestUnsupportedFlashText doc comment (L86-87), and the NullFlash error string (L184). Test structure / entry-timing established by Phase 2 (markTwo-before-resolve async ordering, deferred-Enter path) is unchanged.
- Notes: Coverage is focused, not over-tested — the pure-function assertions plus the reactive-backstop behavioural assertions (flashText set correctly on direct, deferred, and NULL paths) are each distinct and load-bearing. TestBurstUnsupported_SupportedStillDispatches (the supported-path guard) is unchanged. Would fail if the copy broke (byte-literal want comparison). Unit-lane appropriate — package tui white-box, no daemon, no built binary.

CODE QUALITY:
- Project conventions: Followed. Idiomatic Go, single-responsibility shared renderer, thorough doc comment consistent with the sibling GoneMessage/PartialFailureMessage renderers. Copy single-sourced so both burst surfaces (picker flash + CLI open-burst) cannot drift.
- SOLID principles: Good. One function, one reason to change; both callers delegate.
- Complexity: Low (single if/else, one Sprintf).
- Modern idioms: Yes.
- Readability: Good — intent-revealing doc comment; discriminator ("— nothing opened" suffix) documented in the edge-case reasoning.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
