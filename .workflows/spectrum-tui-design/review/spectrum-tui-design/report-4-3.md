TASK: spectrum-tui-design-4-3 — "No tags yet" signpost reskin (accent.violet left-bar info band over the flat list, via the single-slot arbiter)

ACCEPTANCE CRITERIA (from plan phase-4-tasks.md / tick-c2c98c):
- Signpost renders as an accent.violet ▌ left-bar band with the message in text.strong; no literal hex (colours from §2.9 tokens).
- Wording is the spec-exact "No tags yet — add tags in a project's editor: press x for projects, then e to edit", sourced as a single constant.
- Shows only in By-Tag mode with zero tags anywhere, over the flat list (degrade-with-message); grouping machinery (anyTagsExist gate, ToListItems flat arm) unchanged (parity).
- Performs zero pane reads (flat/signpost path never invokes resolveSessionDirs — §5.4).
- Persistent band yields the slot to a transient flash for its duration, then returns (via the 4-1 arbiter).
- Under NO_COLOR keeps the ▌ bar + position while dropping tint + bar colour.
- vhs: testdata/vhs/sessions-no-tags-signpost.png vs Paper frame "Sessions — no tags signpost (MV)".

STATUS: Issues Found (one non-blocking spec/plan drift on the on-band text token)

SPEC CONTEXT:
§11.3 — By-Tag with zero tags anywhere shows an accent.violet left-bar signpost (message in text.strong) over the flat list — degrade-with-message, not silent flatten. §5.3 — the zero-tags-anywhere → signpost rule. §5.4 — pane reads are gated to grouped modes only; Flat and the zero-tags signpost perform ZERO pane reads. §2.5 — NO_COLOR band carve-out: bands drop tint+bar colour, keep ▌ + position + message. §2.9 — accent.violet / text.strong tokens; text-carrying tints are co-tuned with their on-band text token (both contrast ratios must clear simultaneously; when no tint value satisfies both, the text token moves too). §11 intro — single-slot rule: at most one band; transient flash wins while shown, persistent band returns.

IMPLEMENTATION:
- Status: Implemented (with one deliberate divergence from the literal spec/plan — see Notes / Blocking analysis).
- Location:
  - internal/tui/model.go:4261 — byTagSignpostText constant (spec-exact wording, apostrophe-s, matches §11.3 verbatim).
  - internal/tui/model.go:1544 — gate `m.byTagSignpost = m.sessionListMode == prefs.ModeByTag && !anyTagsExist(m.projects)` preserved byte-for-byte.
  - internal/tui/model.go:1548 — `case m.byTagSignpost: items = ToListItems(filtered)` flat-items arm preserved; no resolveSessionDirs on this arm (1549 vs the grouped arms at 1551/1553 that DO call resolveSessionDirs).
  - internal/tui/model.go:4103-4106 — viewSessionList composes header → renderSessionBandSlot() → listView → footer; the standalone insertRowBelowTitle(renderByTagSignpostRow()) call is removed (grep confirms no production references to renderByTagSignpostRow / byTagSignpostStyle anywhere).
  - internal/tui/notice_band.go:347-355 — activeNoticeBand arbiter: flashText wins, else byTagSignpost → bandInfo.
  - internal/tui/notice_band.go:86-129 — bandInfo → accent.violet bar, bg.selection tint, no status glyph; all from §2.9 tokens, no literal hex.
  - internal/tui/notice_band.go:372-377 — noticeBandOnBandText returns text.on-selection for bandInfo (NOT text.strong — the divergence).
  - internal/tui/model.go:1342-1348 — sessionBandHeight measures off renderSessionBandSlot (band + blank = 2 rows), tying the F10 reserve to exactly what is composed; applySessionListSize reserves it (model.go:1312).
- Notes: Clean routing through the shared 4-1 primitive; old #888888 italic ad-hoc style fully removed; tokens only. Build of internal/tui/... is GREEN. Visual fixture (sessions-no-tags-signpost.png) matches §11.3 — violet ▌ bar + spec-exact message on a subtle bg.selection tint, flush under the title separator, above "Sessions 4 — by tag", over the flat list.

TESTS:
- Status: Adequate (one gap — see below).
- Coverage (internal/tui/bytag_signpost_reskin_test.go — task 4-3):
  - TestSignpostReskin_VioletInfoBand — bar prefix, accent.violet bar fg seq, bg.selection tint present, bg.warning ABSENT, message present.
  - TestInfoBands_ShareSameTint — bandInfo and bandCommand both resolve bg.selection (consolidation regression guard).
  - TestSignpostReskin_SpecExactWording — pins byTagSignpostText to the literal spec string + asserts it renders in the view.
  - TestSignpostReskin_OnlyByTagZeroTags — table: Flat/By-Project/By-Tag-with-tag = no signpost; By-Tag-zero-tags = signpost (gate parity).
  - TestSignpostReskin_ZeroPaneReads — fakeStamper read counter asserts 0 reads + 0 stamp writes on the signpost/flat arm (§5.4).
  - TestSignpostReskin_GroupingMachineryUntouched — items are byte-for-byte ToListItems(sessions), no group metadata.
  - TestSignpostReskin_YieldsToFlashThenReturns — flash takes the slot (signpost hidden), clearFlash → signpost returns (single-slot arbiter).
  - TestSignpostReskin_NoColorKeepsBarAndPosition — colourless band keeps ▌ + position + message, carries no SGR (band == ansi.Strip(band)).
  - Plus the retained task 3-7 suite (bytag_zero_tags_signpost_test.go): anyTagsExist table, gate behaviour, flat-list parity, s-press advance, reopen-persisted, tag-present negatives — still GREEN because the gate/flat behaviour was preserved.
- Notes: Test balance is good — focused, behaviour-oriented (renders via the model's own band path), no redundant happy-path duplication, no over-mocking. The two files test different layers (3-7 = gate/flat behaviour; 4-3 = MV band visual + slot routing), not duplicates. GAP: every assertion on the message colour pins text.on-selection (TestSignpostReskin_VioletInfoBand line 74-77), so the test suite ENCODES the divergence from the spec's text.strong rather than catching it — a test that asserted §11.3's literal token would currently fail. This is the test mirroring the implementation choice, not an independent verification of the spec criterion.

CODE QUALITY:
- Project conventions: Followed. Tokens-only (no literal hex at the call site, §2.8/§2.9 honoured); shared primitive reused (DRY — signpost and command banner share newBandBase/renderNoticeBand); small interfaces; tests avoid t.Parallel(); no slog construction. Idiomatic Go.
- SOLID: Good. Single arbiter owns slot selection; the band primitive is open for new roles via the role enum without touching consumers.
- Complexity: Low. The signpost arm is a single switch case + a token mapping; no added branching.
- Modern idioms: Yes.
- Readability: Good — heavily commented with spec cross-references; intent is clear.
- Issues: One stale doc cross-reference in the byTagSignpost field comment (model.go:415-417) cites "spec § Mode Persistence & Empty States → Empty states → By Tag with zero tags" — an old section name; the canonical refs are §11.3 / §5.3. Cosmetic.

BLOCKING ISSUES:
- None. The on-band text token divergence (text.on-selection vs the spec/plan's text.strong) is a legibility-preserving, §2.9-co-tuning-justified decision (the info band sits on bg.selection, the same tint the selected-row name uses with text.on-selection, a pairing already contrast-validated), and is non-blocking. It does not break the band, the gate, the slot, or any behaviour. It is flagged below for a decision because it contradicts an explicit acceptance criterion and the literal spec wording without a recorded decision, and the test encodes the deviation.

NON-BLOCKING NOTES:
- [idea] internal/tui/notice_band.go:372-377 — noticeBandOnBandText returns text.on-selection for bandInfo, but both spec §11.3 AND plan task 4-3 acceptance (phase-4-tasks.md:143, tick-c2c98c) specify the signpost message in text.strong. Decide: either reconcile the implementation to text.strong (verifying it clears the 4.5 text floor on bg.selection — text.strong dark #A9B1D6 has a 9.9 canvas ratio but is unverified against the #28243a tint), or amend §11.3 / the plan to record text.on-selection as the co-tuned on-band token for the bg.selection info band (the §2.9 "text token moves too" rule). This is a decide-which-and-whether call, hence idea, not quickfix.
- [quickfix] internal/tui/bytag_signpost_reskin_test.go:74-77 — TestSignpostReskin_VioletInfoBand asserts the message foreground is text.on-selection, encoding the divergence above; once the §11.3 token question is resolved, update this assertion (and the helper noticeBandOnBandText path it implicitly relies on) to the agreed token so the test verifies the spec rather than the implementation. Tracks the idea above; mechanical once the decision lands.
- [do-now] internal/tui/model.go:415-417 — the byTagSignpost field comment cites a stale spec section name ("spec § Mode Persistence & Empty States → Empty states → By Tag with zero tags"); replace with the canonical "§11.3 / §5.3 (By Tag with zero tags anywhere)" to match the rest of the file's cross-references.
