TASK: cli-verb-surface-redesign-7-7 — Sweep stale removed-surface references in code comments and the process-role doc

ACCEPTANCE CRITERIA (plan row 7-7):
- Comments/docs only — NOT the bootstrap warning strings (Task 7-1) or CLAUDE.md (Task 7-6)
- Repoint `internal/spawn` "cannot drift" anchors to the two surviving bursts (picker + open)
- Annotate now-single-caller split helpers with their sole consumer
- Note the dead `case "clean"` process-role arm; keep the closed-space value in place

STATUS: Complete

SPEC CONTEXT:
This is an Analysis-Cycle-1 chore. The spec (specification.md §"clean deleted" / §"doctor" / §"uninstall") confirms `portal clean` and its `--logs` flag are deleted, `state status`/`state cleanup` subsumed, and `portal spawn`/`attach` retired (Phase 5). The `spawn` service (internal/spawn) is retained and reached only via the two bursts. Post-redesign the only removed-surface references that survive in the tree are stale COMMENTS pointing at the deleted CLIs — this task sweeps them. The process_role taxonomy is a governed closed space (per CLAUDE.md "Closed taxonomy … amend spec, never invent at call-site"), so pruning the now-unreachable `clean` value is out of scope for a comment sweep — the retained-value decision aligns with that governance.

IMPLEMENTATION:
- Status: Implemented
- Location (commit 804cc5e1):
  - internal/spawn/doc.go:6-14 — package "cannot drift" anchor repointed from "the TUI picker … and the `portal spawn` CLI … `--detect`" to "two burst callers — the TUI picker … and the multi-target open burst (cmd/open_burst_run.go)"; adds the accurate carve-out that the detector alone is reused by `portal doctor`.
  - internal/spawn/message.go:9-11, :35, :53-60, :73-76 — QuoteJoin/GoneMessage/PartialFailureMessage/UnsupportedNoopMessage doc anchors repointed from "the CLI (cmd/spawn.go)" / "--detect echo" to the picker + open burst; "the CLI adds the prefix" → "the log emitters add the prefix".
  - internal/spawn/split.go:3-16 (SplitNetN) and :27-33 (SplitTriggerFirst) — each now names its SOLE consumer (SplitNetN → the picker's dispatchBurst; SplitTriggerFirst → the open burst's runOpenBurstWithDeps) and states the deliberate trailing-vs-leading-trigger convention distinction; the prior "legacy `portal spawn` CLI" reference is gone. Function bodies unchanged.
  - internal/spawn/classify.go:6-11 and internal/spawn/logemit.go:10-16 — the two other "cannot drift" chokepoint anchors likewise name both burst callers (picker + cmd/open_burst_run.go).
  - internal/log/process_role.go:5-16 (const block header), :29-31 (ResolveProcessRole doc), :39-44 (mapping table), :86-90 (the `case "clean"` arm) — the dead arm and unreachable `roleClean` value are explicitly documented as retained-by-design (closed-space pruning needs a taxonomy amendment); the mapping doc updated to `hook / hooks … -> hooks_cli`.
  - internal/log/retention.go — one inline stale-flag comment repointed `--logs removes it` → `doctor --fix removes it`.
  - ~40 further files (cmd/doctor.go, cmd/open.go, cmd/open_burst_run.go, internal/log/symlink.go/retention.go, internal/state/status.go, internal/tui/burst_*, internal/transienttest/commander.go, and the paired _test.go comment references) received the same comment-only sweep.
- Notes: The doc mapping in process_role.go accurately reflects the live switch (hook/hooks→hooks_cli, clean→roleClean dead, open/x→tui, bare→tui, default→bootstrap). The const-block "CLOSED 6-value space" claim + "every reachable result is one of the other five" is internally consistent. No drift between comment and code.

TESTS:
- Status: Adequate (no new test warranted — comment/doc-only task, mirroring Task 7-6's "docs-only, no automated test")
- Coverage: The change alters zero behaviour (confirmed below), so existing suites remain the coverage. The retained dead `case "clean"` arm is not orphaned: internal/log/process_role_test.go:26-27,94,111 still exercise the `clean` argv → roleClean mapping, so removing the arm would fail the drift-tripwire — the retained value stays test-guarded.
- Notes: A whole-diff scan of all 47 .go files shows every added/removed line is a comment except one, and that one is itself an inline `//` comment (`// … doctor --fix removes it`). No string literal, signature, or executable line changed — so the Task 7-1 bootstrap warning strings and all behavioural assertions are untouched.

CODE QUALITY:
- Project conventions: Followed. Honours the closed-taxonomy governance (retain the process_role value, document the deadness rather than silently prune) and the codebase's dense, provenance-naming comment house-style (comments cite concrete files + function names, e.g. runOpenBurstWithDeps / dispatchBurst).
- SOLID principles: N/A (comment-only).
- Complexity: N/A (no logic touched).
- Modern idioms: N/A.
- Readability: Improved. The split.go helpers now make the trailing-vs-leading-trigger convention legible as a deliberate per-caller choice rather than an accident; the "cannot drift" anchors now name real consumers instead of deleted CLIs.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The dead `case "clean"` arm / `roleClean` value is retained by explicit task decision, governed by the closed-taxonomy convention, documented in-place, and guarded by process_role_test.go — no concrete action is warranted; its eventual removal is a deliberately-deferred taxonomy amendment, out of scope here.)
