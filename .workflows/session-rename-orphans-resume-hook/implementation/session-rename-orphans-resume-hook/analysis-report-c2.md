---
topic: session-rename-orphans-resume-hook
cycle: 2
total_findings: 4
deduplicated_findings: 4
proposed_tasks: 2
---
# Analysis Report: Session Rename Orphans Resume Hook (Cycle 2)

## Summary
Cycle 2 confirms the fix is well-converged: standards found nothing, and production code is soundly consolidated (hook-key derivation centralised in `tmux.HookKey`/`HookKeyFormat`, stale-cleanup in one `runHookStaleCleanup`, `@portal-id` source-of-truth in `session.PortalIDOption`). Two findings survive rigorous filtering: a real defence-in-depth gap where nothing ties the source-of-truth `@portal-id` constant to the two format-string embeddings in a tmux-less path, and a true in-package test-helper redundancy. Two duplication findings — cross-file integration setup/reboot preambles and a cmd stale-cleanup seed-and-assert spine — are discarded as over-coupling extractions that would reduce isolated test readability and diagnostic value.

## Discarded Findings
- Restore integration-test harness re-inlined across task boundaries (duplication, medium) — Extracting the setup preamble and reboot half across the three independently-authored task files (3-5/3-6/3-7) would couple them via divergent per-file constants (`ptl-3-5-`/`ptl-3-6-`/`ptl-3-7-` prefixes, distinct `renameOldName`/`renameNewName`/`renamePortalID`) and reduce isolated readability. The codebase deliberately keeps these integration files self-contained across build-tag boundaries — the `verifyRenameHookFiredOnce` doc-comment states this intent explicitly ("kept local so this file stays self-contained across build-tag boundaries"). Higher coupling risk than maintenance benefit for a converging cycle-2. Not high-severity; discarded.
- cmd stale-cleanup survival tests share a near-identical seed-and-assert body (duplication, medium) — The two tests already share the correct leaf helpers (`assertLiveHookKeyPresent`, `newTempHooksStore`, `keysOf`). Extracting the seed→guard→run→assert spine would collapse the distinct, semantically-meaningful per-chain error messages ("name-based live key coincides ... no-migration upgrade" vs "re-stamped live @portal-id ... chain (b)") into one generic message — losing the self-documenting diagnostic value — or require passing them as parameters, which reintroduces the duplication. The inline assertion bodies are the tests' value. Not high-severity; discarded.
