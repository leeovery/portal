TASK: De-duplicate hook command-body and fingerprint test literals within the tmux_test package (state-notify-cascade-on-binary-upgrade-2-3)

STATUS: Complete

ACCEPTANCE CRITERIA: notify command body has exactly one test-package declaration (realtmux references it); each fingerprint substring declared once (or sourced from shared full-body constant); production unexported-constant mirroring untouched; full internal/tmux suite incl. real-tmux passes.

SPEC CONTEXT: Analysis-cycle TEST-ONLY de-duplication — two byte-for-byte copies of the notify body in the same package tmux_test; NOT the exempt import-boundary mirroring.

IMPLEMENTATION:
- Status: Implemented
- hooks_register_test.go:31,37,42 — single declarations of notifyFingerprint / commitNowFingerprint / signalHydrateFingerprint. :49,57,66 — expected* bodies COMPOSED from their fingerprint substring (fingerprint is the single source). hooks_register_realtmux_test.go:39-45 comment-only block (notifyCommandBody gone); file references shared symbols (managedEventFingerprints uses the three consts; body cross-checks use expectedNotifyCommand / expectedCommitNowCommand).
- notifyCommandBody no longer exists in source. Production unexported constants in hooks_register.go untouched (notifyCommand/commitNowCommand/signalHydrateCommand/*Substring). Legitimately-distinct literals remain distinct (staleSignalHydrateCommand, user-hook markers, notifyDebugBody).

TESTS:
- Status: Adequate. Assertions unchanged in meaning (pure literal-reference substitution). TestSignalHydrateCommand_HasEndOfFlagsSeparator (:855) retains an INDEPENDENT non-composed `want` and asserts equality — correct anchor pattern preventing silent composition drift. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (external test pkg, no t.Parallel, idiomatic const composition). Complexity: Low. Readability: Good (comments document single-home decision + exempt-boundary rule).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
