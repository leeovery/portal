TASK: restore-host-terminal-windows-7-6 — Resolve the spawn-failure/permission flash vs multi-select banner notice-slot precedence (two-row-document vs strict-single-slot suppression)

ACCEPTANCE CRITERIA:
- The chosen behaviour is explicit — either a documenting comment at the precedence seam (two-row) or a banner-suppression path (single-slot), not the current silent divergence.
- If suppression: after a partial/permission failure the notice slot shows the flash alone (no concurrent `N selected` banner), multi-select mode + marked retry set otherwise unchanged.
- If documentation: no behavioural change; the note references the spec precedence clause and the pre-flight-abort sibling for contrast.

STATUS: Complete

SPEC CONTEXT:
Spec §"Mode affordance (visual)" line 138 defines "Notice-band precedence (single slot, highest wins): filter line → in-burst `Opening n/N…` → transient error/guidance flash (pre-flight abort / spawn-failure / permission) → multi-select banner → unsupported-terminal banner → no-tags signpost." A strict literal reading puts the spawn-failure/permission flash ABOVE (replacing) the multi-select banner. The tick description records that this flash tier has no delivered design frame (a design residual in the spec) and that the implementation realises the flash and the `N selected` banner across two physical rows (co-render) rather than replacing. The task required a DECISION between (a) documenting two-row as the intended reading (no behavioural change) or (b) enforcing strict single-slot by suppressing the banner. Either satisfies acceptance.

IMPLEMENTATION:
- Status: Implemented (documentation branch chosen)
- Decision: TWO-ROW / document — commit 3b1a9a57 ("document deliberate two-row flash/banner co-render at precedence seam; Phase 7 complete"). Comment-only; zero logic change (verified via full diff: only two comment blocks added, no code lines altered).
- Location:
  - internal/tui/notice_band.go:348-360 — the precedence-seam note inside `activeNoticeBand`, directly above the `if m.flashText != ""` arm. States: do NOT collapse to strict single-slot; the flash arm takes the §11 band slot REGARDLESS of `m.multiSelectMode`; it co-renders with the `N selected` banner across two physical rows; it is "the intended reading of the spec's 'Notice-band precedence (single slot, highest wins)' clause (§ Mode affordance) for this flash tier"; references the pre-flight-abort SIBLING for contrast; explicitly warns "A later reader must NOT add `&& !m.multiSelectMode` here."
  - internal/tui/model.go:4731-4736 — cross-reference note in `applySectionHeader`'s `abortBannerText` branch clarifying that the spawn-failure/permission siblings route through setFlash → the §11 notice band (a separate row that co-renders) rather than replacing the banner, and pointing readers to the precedence-seam note in `activeNoticeBand`.
- Notes: The two comments cross-reference each other, closing the seam from both sides (the section-header claimant and the notice-band arbiter). Behaviour in burst_partial_failure.go is unchanged: `handleBurstPartialFailure` keeps `multiSelectMode` true and calls `setFlash`, and `applyBurstSelectionMutation` still unmarks only confirmed sessions (retry set preserved) — consistent with the documented two-row intent.

TESTS:
- Status: Adequate (none required)
- Coverage: The tick's Tests clause states "If documentation only: no new test required; existing render tests remain green (a comment needs no assertion)." The commit adds no test, which is correct — a comment carries no assertable behaviour, and no behaviour changed. Existing render/precedence tests continue to cover `activeNoticeBand` and `applySectionHeader`.
- Notes: The alternative (suppression) branch was NOT taken, so its required suppression unit test is correctly absent. No under-testing: there is nothing new to assert. No over-testing.

CODE QUALITY:
- Project conventions: Followed. Comment-first documentation of a load-bearing seam matches the codebase's heavy in-source-rationale convention (e.g. the surrounding §11 / §6 comment blocks). Spec-section anchoring (§6-6, §11, § Mode affordance) is consistent with house style.
- SOLID principles: N/A — no code change.
- Complexity: Low (no logic added).
- Modern idioms: N/A.
- Readability: Good. The note is precise: it names the mechanism (setFlash → activeNoticeBand, two rows), the rationale (informative co-render), the contrasting sibling (pre-flight abort as section-header claimant), the exact anti-regression instruction (do not add `&& !m.multiSelectMode`), and dates the decision (2026-07-14).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] .workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md:138 — The spec's precedence line still reads literally as strict single-slot ("highest wins", flash tier above the multi-select banner), while the code now deliberately co-renders the spawn-failure/permission flash with the `N selected` banner across two rows. Consider adding a short clause to the spec precedence line noting that the spawn-failure/permission flash co-renders (two rows) rather than replacing the banner, so the frozen spec and the in-code decision do not read as contradictory to a future spec reader. Judgment call (whether to amend a post-implementation spec artifact) — non-blocking; the in-code note already records the decision authoritatively.
