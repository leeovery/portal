TASK: Align bootstrap step-count nomenclature (nine-step vs ten-step) (3-4)

ACCEPTANCE CRITERIA:
- `bootstrap.go` doc/Run/comments switch to nine-step framing with CleanStaleMarkers as step 7.
- CLAUDE.md "Server bootstrap" section updated.
- No remaining "ten-step" wording in `cmd/bootstrap/`.
- Existing tests pass without modification.

STATUS: Complete

SPEC CONTEXT: Standards-finding remediation surfaced in analysis-standards-c1.md. Prior to this task, bootstrap.go framed as "ten-step" with "Return" as step 10, while CLAUDE.md and bootstrap_test.go used nine-step framing. Realigned to nine-step framing.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/bootstrap.go:1` — package doc opens with "nine-step PersistentPreRunE sequence".
  - `cmd/bootstrap/bootstrap.go:1-23` — package doc enumerates steps 1-9 with "Return" as post-step boundary.
  - `cmd/bootstrap/bootstrap.go:98-101` — MarkerCleaner doc identifies as "Step 7".
  - `cmd/bootstrap/bootstrap.go:113-117` — FIFOSweeper doc identifies as "Step 8".
  - `cmd/bootstrap/bootstrap.go:161` — Orchestrator type doc says "nine-step bootstrap sequence".
  - `cmd/bootstrap/bootstrap.go:177` — Run doc says "nine bootstrap steps".
  - `cmd/bootstrap/bootstrap.go:208-292` — Step-entry Debug logs labelled "step 1" through "step 9".
  - `cmd/bootstrap/bootstrap.go:294-297` — Final Debug log labelled "Return: exiting".
  - `CLAUDE.md:71-83` — uses nine-step framing.

TESTS:
- Status: Adequate (no new tests required; doc-only).
- Coverage: Test files already used nine-step framing (cmd/bootstrap/bootstrap_test.go:111,645,734).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: N/A (doc-only).
- Complexity: Low.
- Readability: Good — step numbering consistent across docs.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Historical planning artifacts under `.workflows/daemon-merge-reintroduces-dead-sessions/planning/` (phase-2-tasks.md:203,207,211,246; review-traceability-tracking-c1.md:79) still contain "ten-step" wording. Out of scope per acceptance criterion.
