TASK: killed-sessions-resurrect-on-restart-10-1 — Fix gofmt drift on internal/restore/session.go docstring

ACCEPTANCE CRITERIA:
- Docstring-only edit with no behavioural change.
- Sanitization rationale + reference to sanitizeSessionName preserved.
- Lint gate verified by `gofmt -l internal/restore/session.go` returning empty.
- Repo-root gofmt-flagged set matches the pre-fix list minus this file.

STATUS: Complete

SPEC CONTEXT: gofmt is one of three declared linters. Cycle 7 standards finding: Go 1.26's gofmt smart-punctuation rule rewrites the literal `'\''` token in buildHydrateCommand docstring. Five other repo-root gofmt-flagged files predate the work unit and are out of scope.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/restore/session.go:408-436
- Notes:
  - Literal `'\''` token removed and replaced with prose at line 425.
  - Grep for `'\''` against the file returns zero matches.
  - Sanitization rationale preserved verbatim — docstring still explains apostrophe-bearing inputs would break shell parsing, still references sanitizeSessionName in internal/state/panekey.go, still enumerates filtered character set (`/`, `\`, `\0`).
  - Function body byte-identical to pre-fix.

TESTS:
- Status: Adequate (lint-gate verification, no new unit test required)
- Coverage: Cycle 8 standards analysis confirms `gofmt -l .` from repo-root now reports only the five pre-existing baseline files. Existing unit tests covering buildHydrateCommand continue to pass.

CODE QUALITY:
- Project conventions: Followed.
- Readability: Good — reworded prose is clearer than original literal-token form.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
