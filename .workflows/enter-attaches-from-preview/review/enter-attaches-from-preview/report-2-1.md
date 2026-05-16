TASK: enter-attaches-from-preview-2-1 — Add flash state fields and setFlash/clearFlash helpers to Sessions page model

STATUS: Complete

SPEC CONTEXT: Spec § Inline flash — feature-local infrastructure requires Sessions-page model state (flash text + tick handle/generation). Spec § Replacement on rapid successive bails requires the chosen mechanism (here: monotonic uint64 generation counter) to enforce "latest bail wins; prior pending tick must not clear the new flash early". Foundation for the rest of Phase 2.

IMPLEMENTATION:
- Status: Implemented
- Fields: internal/tui/model.go:203-213 — `flashText string` + `flashGen uint64` grouped with Sessions-page-scoped fields; godoc documents canonical-signal decision (empty string = inactive) and spec references.
- setFlash: internal/tui/model.go:778-788 — bumps gen then assigns text; godoc covers post-bump capture contract and empty-string edge case.
- clearFlash: internal/tui/model.go:790-796 — zeros text only; godoc explains idempotence and gen-preservation rationale.
- Recommendation taken: relies on `flashText != ""` as active signal (no separate flashActive bool).
- Helpers placed in model.go rather than sibling file (plan allowed either; sibling sessions_flash.go was created later for tick infra at task 2-3).
- Receiver is `*Model` — correct for mutation.

TESTS:
- Status: Adequate
- File: internal/tui/sessions_flash_state_test.go
- Coverage maps 1:1 to plan's six required tests:
  - TestModel_FlashState_ZeroValue
  - TestModel_SetFlash_SetsTextAndBumpsGen
  - TestModel_SetFlash_GenIncrementsMonotonically
  - TestModel_ClearFlash_ZerosTextLeavesGen
  - TestModel_ClearFlash_IdempotentOnAlreadyCleared
  - TestModel_FlashState_SetClearSet
  - TestModel_SetFlash_EmptyStringStillBumpsGen (additional, covers documented edge case)
- No t.Parallel.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Helpers do one thing each.
- Complexity: Low. Two-line helpers.
- Modern idioms: Zero-value-ready uint64 counter, no constructor needed.
- Readability: Good. Comments tie code to spec sections and explain the gen-preservation contract downstream tasks depend on.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
