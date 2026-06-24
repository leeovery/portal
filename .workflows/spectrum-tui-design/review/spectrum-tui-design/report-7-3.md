TASK: spectrum-tui-design-7-3 — Scope the keymap descriptor "single source of truth" framing to DISPLAY and guard descriptor↔dispatch drift

ACCEPTANCE CRITERIA (from tick-b53df9 + plan row):
- The "single source of truth" comments no longer imply dispatch coverage; they explicitly scope the guarantee to footer + help display.
- A guard test exists asserting descriptor↔dispatch correspondence per page (every non-help descriptor Key honoured by dispatch, every bound dispatch key present in the descriptor).
- A documented display-only/help allow-list excludes intentionally display-only entries.
- The guard test passes on the current (post-Task-1) tree.
- No live dispatch behaviour changes as part of this task (descriptor + dispatch stay separate).

STATUS: Complete

SPEC CONTEXT:
Spec §8.5 (line 354) and §14.4 (line 536) frame the per-page keymap descriptor as "the single source of truth that drives footer + help" — purely the two DISPLAY surfaces (condensed footer + ? help modal). The spec never claims the descriptor drives key DISPATCH. §12.1 ("Per-screen keymaps", line 467) enumerates the audited per-page keymaps. The task's job is to align the in-source doc framing with the spec's actual (display-only) claim and to add a structural guard so the hand-coded dispatch switch and the descriptor cannot silently diverge — the Cycle-2 Projects-nav leak (Task 7-1) being the canonical instance of exactly this gap.

IMPLEMENTATION:
- Status: Implemented (commit 3e8c62b4)
- Location:
  - internal/tui/keymap.go:9, :15-21 (new SCOPE block on the type doc), :36, :48-50 (HelpKey/HelpAction docs rescoped to "single-source-of-truth-for-DISPLAY … Dispatch is out of scope"). previewKeymap doc (keymap.go:161-163) already reads "drives the §9.1 … footer and the per-page ? help reference" — DISPLAY-scoped.
  - internal/tui/help_modal.go:10-17 (rescoped to "single source of truth for the footer + help DISPLAY … The descriptor does NOT govern key dispatch … kept in sync via keymap_dispatch_guard_test.go").
  - internal/tui/keymap_dispatch_guard_test.go (new, 341 lines) — the guard.
- Notes:
  - Doc rescoping is accurate and matches the spec's claim. Both keymap.go's HelpKey/HelpAction docs and help_modal.go cross-reference the guard test and the type doc, so the "dispatch is separate" contract is discoverable from every consumer comment.
  - No live dispatch changed: the commit touches only keymap.go (comments), help_modal.go (comments), and the new test file (verified via `git show 3e8c62b4 --stat` — model.go / pagepreview.go untouched).
  - commandPendingKeymap (keymap.go:142-159) is correctly out of scope: its doc claims only "single binding source for the swapped Projects footer" (a display claim, not dispatch), and the task names only sessionsKeymap/projectsKeymap/previewKeymap. emptyFooterDescriptor's "single source of truth, §12.1" (empty_states.go:122) is a footer-copy claim, also display-only — not misleading.

TESTS:
- Status: Adequate
- Coverage:
  - Three guard tests (TestSessionsDescriptorDispatchParity, TestProjectsDescriptorDispatchParity, TestPreviewDescriptorDispatchParity) route through the shared assertDescriptorDispatchParity, which enforces BOTH directions: (1) every non-RightAligned descriptor Key must have a probe whose honour() returns true; (2) every probe key must appear in the descriptor. Both directions genuinely bite.
  - The guard is NOT vacuous. Each honour() drives the LIVE Update path and distinguishes a bound effect from a passthrough no-op against the real dispatch switches (verified model.go:2940-3029 for sessions, model.go:2255-2304 for projects, pagepreview.go:576-639 for preview):
    - Sessions: ⏎ asserts isQuitCmd(cmd) && selected=="alpha" (handleSessionListEnter); ␣ asserts activePage==pagePreview; s asserts mode changed && still PageSessions; r/k assert the right modal opens; x asserts PageProjects; n asserts a non-nil createSession cmd; q asserts a quit cmd; / asserts SettingFilter(). Removing any `case` would flip the corresponding assertion.
    - Projects: ⏎/n assert non-nil cmd; x asserts PageSessions; e/d assert modalEditProject/modalDeleteProject; esc asserts a quit cmd; / asserts SettingFilter().
    - Preview: scroll (↑/↓) and page (^↑/↓) correctly assert !handled (viewport-delegated, NOT in handlePreviewKey's switch → return false); Home/End, ←→, ⇥(Tab), ⏎, ␣ assert handled==true and ␣ further asserts a previewDismissedMsg cmd. Matches pagepreview.go exactly.
  - Allow-list is documented and minimal: the single RightAligned `?` entry per page is skipped, derived from the descriptor's own RightAligned flag (not a hand-listed glyph), so it cannot silently widen. The header comment names the dedicated suites that pin `?` dispatch elsewhere.
  - Fixtures are real: probes seed multi-row models (sessionsGuardModel / projectsNavModel) so the cursor can actually move — a single-row InfiniteScrolling list would mask a leaked nav binding (the same trap Task 7-1 documented).
- Notes:
  - Existing footer/help display tests are unaffected (comment-only changes); the guard test file was later touched only by 8-2 (footer ⏎/␣ glyph update), which is downstream and out of this task's scope.
  - Minor: the two `^↑/↓` page probes assert `KeyMap.NextPage.Keys() > 0 && KeyMap.PrevPage.Keys() > 0` (paging is bound on the list KeyMap) rather than driving a ctrl+arrow press and observing a page advance. This verifies "paging is bound" not "paging is bound to ctrl+arrows", so a mis-rebind via pinArrowOnlyNav (SetKeys to a different combo) would drift from the `^↑/↓` glyph yet still pass. Defensible — paging genuinely has no updateXxx arm; it lives entirely on the list KeyMap — but it is the one structurally-weaker probe in the suite. See non-blocking note.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); table-of-probes (map[string]dispatchProbe) is idiomatic; honour funcs build their own fixtures (t threaded through) consistent with the cmd-package DI-via-fixture pattern. Stub seams (keymapParity{Killer,Renamer,Enumerator,Reader}, sessionsGuardCreator) are minimal non-nil routers reused from sessions_keymap_dispatch_test.go — no duplication.
- SOLID principles: Good. assertDescriptorDispatchParity is a single shared correspondence engine; per-page tests supply only the probe map. dispatchProbe is a clean 2-field struct (press + honour) with a focused responsibility.
- Complexity: Low. The shared assertion is two small loops; each probe is a self-contained closure.
- Modern idioms: Yes. Glyph-keyed map, closure-per-probe, allow-list derived from data (RightAligned flag) rather than a literal list.
- Readability: Good. The file header is an exemplary explanation of the gap, the two-way contract, and the allow-list rationale; each probe carries a one-line intent comment naming the bound effect.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/keymap_dispatch_guard_test.go:108-112,193-197 — the `^↑/↓` page probes assert `KeyMap.NextPage.Keys() > 0 && KeyMap.PrevPage.Keys() > 0` (paging is bound) rather than driving a ctrl+arrow press and asserting a page change; consider strengthening to assert the bound keys are the ctrl+up/ctrl+down strings (or drive a real press against a multi-page fixture and observe Paginator.Page advance) so a mis-rebind of paging away from ctrl+arrows is caught. Requires a judgment call on how to drive list paging through a fixture, hence idea not quickfix.
- [do-now] internal/tui/keymap.go:23-26 — the Key-field doc example reads `(e.g. "↑↓", "⏎", "^↑/↓")` and references `updateSessionList`; this is accurate, but the parallel HelpKey doc (keymap.go:28-37) still narrates the historical "footer keeps the word 'space'/'enter'" switch-over in past tense — a wording tidy to drop the now-stale transition narrative would shorten the comment without changing meaning. Pure comment edit, zero logic impact.
