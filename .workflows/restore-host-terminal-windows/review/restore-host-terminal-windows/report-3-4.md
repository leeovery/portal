TASK: restore-host-terminal-windows-3-4 — Pre-flight has-session gate in the spawn orchestrator

ACCEPTANCE CRITERIA:
- spawn s1 s2 s3 with s2 gone returns a plain error naming s2, calls zero FakeAdapter.OpenWindow and zero Connect, never reaches detect/resolve.
- Multiple gone sessions (s2, s3) all named in the single one-line message.
- spawn s1 (N=1) with s1 gone aborts with the gone-session message and no self-attach (pre-flight not skipped for N=1).
- All-present (spawn s1 s2 s3, all exist) proceeds unchanged — gate returns empty, pipeline runs to normal outcome.
- A probe returning false (transient tmux fault) aborts conservatively (no window, no self-attach).
- PreflightMissing is pure (no I/O beyond exists) and preserves list order.

STATUS: Complete

SPEC CONTEXT:
Spec § "Stance: pre-flight + all-or-nothing" (spec:154-162) mandates all-or-nothing at the pre-flight gate: before opening a single window, verify every selected session still exists via quick has-session checks; if any is gone, nothing opens and a one-line error names the gone session(s) (design copy `⚠ '<session>' is gone — nothing opened`). Spec § "Trigger-Context Matrix" (spec:225) lists "Selected session vanished between picker-load and Enter" → atomic abort, nothing opens. Spec:63 fixes exit-1-on-stderr with the same one-line message the picker shows. The selection-pruning half is a Phase-6 picker concern; the CLI has no persistent selection, so it simply aborts (exit 1).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/preflight.go:13 (PreflightMissing), internal/spawn/message.go:13/25 (QuoteJoin / GoneVerb, this task's new shared helpers), cmd/spawn.go:136-139 (first-gate wiring in runSpawn).
- Notes:
  - PreflightMissing is a pure order-preserving accumulator returning nil when all present (preflight.go:14-20). Matches the AC exactly.
  - Wired as the FIRST step in runSpawn — ahead of the N=1 direct-attach branch and the N≥2 unsupported gate (cmd/spawn.go:136), so a gone session aborts even on an unsupported terminal and for N=1. Probes the whole `sessions` slice (external + trigger), confirming the trigger self-attach target is included.
  - Exists seam added to SpawnDeps defaulting to client.HasSession (cmd/spawn.go:53-56, 299, 352-354). HasSession folds ANY error to false (internal/tmux/tmux.go:135-138), giving the required conservative "probe fault → gone → abort". This is a deliberate fail-closed contrast to the sibling HasSessionProbe (fail-open on OS fault) used by the preview path — the correct choice for this task per its spec stance.
  - The task's literal `fmt.Errorf("spawn: %s %s gone …", QuoteJoin, GoneVerb)` was consolidated into a single `spawn.GoneMessage(gone)` renderer (message.go:40, used at cmd/spawn.go:138 and LogGone at logemit.go:102). Functionally identical, and a cleaner single-source-of-copy consolidation consumed by both the CLI error and the log line. Not drift — an improvement consistent with the "copy edit lands in one place" intent.
  - Shared-helper contract holds: PreflightMissing is reused verbatim by the Phase-6 picker (internal/tui/burst_progress.go:190,428) and QuoteJoin/GoneVerb underpin GoneMessage/PartialFailureMessage — declared once in internal/spawn, no cross-package re-declaration.
  - Minor note: the task-to-verify brief cited "its use in internal/spawn/burst.go"; the gate correctly lives in cmd/spawn.go's runSpawn (per the plan's Key-Do), not burst.go. burst.go has no PreflightMissing reference, which is correct — the burster owns only the post-pre-flight external half.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/preflight_test.go exercises the pure helper: nil-when-all-present, single gone collected, multiple-gone in input order (deliberately unsorted input), and probes-each-once-in-order (purity/ordering). Covers AC6.
  - cmd/spawn_test.go TestSpawnPreflight covers the wired pipeline: single gone (message, non-UsageError, zero OpenWindow, zero Connect, zero Detect — AC1), multiple gone named in one line (AC2), N=1 sole-session gone aborts no self-attach (AC3), all-present proceeds to 2 externals + self-attach s3 (AC4), conservative abort on probe fault (AC5), and one INFO outcome line with no opened/total/ack/batch summary attrs leaking.
  - The spyDetector asserting detector.calls == 0 is a strong guard that the gate precedes detect/resolve.
- Notes:
  - Well-balanced, not over-tested — each subtest pins a distinct AC. The AC5 "probe fault" case is behaviourally identical to AC1's single-gone (both model Exists→false for s2); it is justified as it explicitly documents the fail-closed HasSession-folds-error semantic rather than duplicating coverage, so no change needed.
  - Would fail if the feature broke: reordering the gate after detect, skipping N=1, or letting a spawn/connect through would all trip these assertions.

CODE QUALITY:
- Project conventions: Followed. Injectable 1-method Exists seam with production default in buildSpawnDeps; cross-package helpers exported per the codebase pattern (matches PreflightMissing / SpawnMarkerName). TMUX-poison / *Deps-injection discipline honoured in the tests (nopRunner + full injection).
- SOLID principles: Good. PreflightMissing is single-responsibility and pure; message helpers are small focused renderers; the gate delegates rendering to GoneMessage rather than inlining copy.
- Complexity: Low. Single linear loop; gate is one guarded branch at the top of runSpawn.
- Modern idioms: Yes. Idiomatic nil-slice accumulation, function-value seam.
- Readability: Good. Thorough godoc on preflight.go and the runSpawn gate comment explains ordering/precedence rationale.
- Issues: None. QuoteJoin does not escape embedded single quotes, but session names are portal-generated and the output is display copy (not shell), so there is no injection risk.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
