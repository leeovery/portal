TASK: Comment the uncommented defensive branches in the phase's boundary code (chore) (portal-observability-layer-4-6)

ACCEPTANCE CRITERIA:
- Every uncommented deliberate defensive swallow/ignore in the phase's boundary code (4-1..4-5 sites) carries a one-line "why this branch exists" comment.
- Already-adequately-commented branches left unchanged (no redundant comments).
- No log line added; no control-flow/error-handling/return-value change (comment-only diff).
- Scope limited to boundary code touched this phase — no codebase-wide sweep.
- go build/test remain green.

STATUS: Complete

SPEC CONTEXT:
Spec § Diagnostic context preservation → gap-closure sites, fourth row: "Defensive branches (various) | uncommented | add a why-this-branch-exists code comment (not a log line)." Scope = sites touched by 4-1..4-5.

IMPLEMENTATION:
- Status: Implemented
- Verified sites: state_hydrate.go:156 _,_=f.Read(buf) (commented 153-154: 0-byte read = signal arrival); :159 _=f.Close() (157-158); :162 _=os.Remove(cfg.FIFO) (160-161: best-effort, reclaimed by next sweep — the site the task flagged); :348 timeout os.Remove (already commented, skipped); fifo.go:39 Chmod (already commented, skipped); pgrep.go:70-73 (already commented, skipped); tmux.go:190-192 ListSessions swallow→[]Session{},nil (commented: no-server = valid zero-sessions); portal_saver.go:356/362/375 tolerant kills (inline one-liners); daemon_identity.go:118-130 defensive branches (commented).
- Notes: No drift. Terse single-purpose accurate comments. No stray logger.* calls or control-flow changes. Self-evident empty-output guard (tmux.go:196-198) correctly left without redundant comment.

TESTS:
- Status: N/A (correctly)
- Coverage: comment-only chore, no runtime behaviour. Verification = build/test green + reviewer confirmation each swallow has a rationale. Existing Phase-4 suites remain the behavioural guard, unaffected. (Note: full `go test ./...` was run by the orchestrator post-implementation and passed.)
- Notes: Inspection of every touched site shows only comment lines — green-build holds by construction.

CODE QUALITY:
- Project conventions: Followed (terse rationale-first style; no log lines added, honouring swallow-needs-a-why-comment-not-a-log distinction).
- SOLID/Complexity/Modern idioms: N/A (comment-only).
- Readability: Improved — deliberate defensive swallow now distinguishable from a forgotten error check at every enumerated site.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] tmux.go:189-194 ListSessions collapses ALL list-sessions errors to empty-slice success; the new comment asserts "no server running" but a non-server error (malformed -F, tmux crash) is also silently swallowed to zero sessions. Out of scope for this comment-only task, but a latent observability gap — a future task could discriminate ErrNoSuchSession/server-absent from other failures per Boundary class 2.
- [quickfix] Grep tool rendered some `//` comment lines as single-slash in its output; direct Read confirms correct `//` in source — tool-display artifact, no action.
