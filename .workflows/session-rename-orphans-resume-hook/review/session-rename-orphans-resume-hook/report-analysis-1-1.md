TASK: Copy PortalID in the findOrAppendSession append branch (session-rename-orphans-resume-hook-analysis-1-1, tick-c5cde0)

ACCEPTANCE CRITERIA:
- The appended Session literal in findOrAppendSession includes PortalID: ps.PortalID.
- Windows remains an empty []Window{} (unchanged copy semantics for windows).
- No change to mergeSkippedPanes, buildLiveStructure, mergePane, or the sessionLive gate.
- go build ./... and go test ./internal/state/... pass.
- A focused unit test drives the append path asserting PortalID preservation (existing capture_test.go conventions, no t.Parallel()).

STATUS: Complete

SPEC CONTEXT:
Spec § Cross-Reboot Persistence of @portal-id (specification.md:80-107). PortalID must ride sessions.json faithfully across reboots: the resume-hook key is computed from the saved PortalID, so losing the id → the next reboot resurrects a bare shell and stale-cleanup deletes the just-fired hook. Line 107 (case (a) Re-persistence) states verbatim the failure this task guards: a session captured as PortalID == "" erases the id from the snapshot → next reboot resurrects a bare shell. Acceptance Criteria (spec:143-144) require capture to preserve PortalID and legacy payloads to decode to "". This task closes the latent partial-struct-copy seam in the append branch that would produce exactly that empty-id snapshot entry if the sessionLive gate were ever relaxed.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/capture.go:260-265 (findOrAppendSession append literal).
- Notes:
  - Line 262 adds `PortalID: ps.PortalID,` to the struct literal alongside Name (261) and Environment (263). Correct.
  - Line 264 keeps `Windows: []Window{}` empty — windows are populated by the caller via findOrAppendWindow/mergePane. Correct per task step 3.
  - The doc-comment (capture.go:250-253) promising "a shallow copy of ps (with no windows)" is now honoured — no comment change was needed and none was made.
  - The sessionLive gate (capture.go:187), mergeSkippedPanes (183-208), buildLiveStructure (216-230), and mergePane (237-248) are all unchanged. No merge-logic drift.
  - Session struct (schema.go:31-36) carries PortalID as field 2 (json:"portal_id"); the literal's field is well-formed.
  - Change is strictly the struct-copy completeness fix as scoped. No scope creep.

TESTS:
- Status: Adequate
- Coverage: New white-box unit test TestFindOrAppendSessionCopiesPortalID in internal/state/capture_internal_test.go (new file, package state) drives findOrAppendSession directly with a prev Session carrying PortalID "aB3xY9kZ" and asserts: si == 0 (appended into empty index), got.PortalID == ps.PortalID (the core assertion), got.Name and got.Environment copied, and Windows stays empty non-nil ([]Window{}, len 0, not nil) — verifying ps.Windows is intentionally NOT copied.
- Notes:
  - Test drives the append path directly rather than relaxing the sessionLive gate — the cleaner of the two options the task offered, and it pins the trap closed independently of production reachability (which no test pins, as the task notes).
  - The Windows-empty-and-non-nil pair of assertions correctly guards both halves of the intended copy semantics.
  - No t.Parallel() (correct — matches capture_test.go / cmd-package convention).
  - Not over-tested: four focused assertions, no redundant cases, no mocking, minimal setup. The Name/Environment assertions are justified — they confirm the "whole struct copy" the doc-comment promises, not just the one changed field.
  - Not under-tested for this task's scope: the append-branch PortalID preservation is the exact behaviour and it is directly asserted. Would fail if the field were dropped from the literal.
  - The doc-comment on the test file accurately explains the white-box rationale (append branch unreachable via public CaptureStructure path today).

CODE QUALITY:
- Project conventions: Followed. White-box test in `package state` uses the `_internal_test.go` suffix consistent with the codebase; no t.Parallel(); table-free single-scenario test is appropriate for a one-branch pin.
- SOLID principles: Good. Single-responsibility helper unchanged in shape; the fix is a one-line completeness addition.
- Complexity: Low. No new branches or paths.
- Modern idioms: Yes. Idiomatic struct-literal field copy.
- Readability: Good. Field order (Name, PortalID, Environment, Windows) mirrors the schema.go struct declaration order, aiding scan-comparison against the type.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
