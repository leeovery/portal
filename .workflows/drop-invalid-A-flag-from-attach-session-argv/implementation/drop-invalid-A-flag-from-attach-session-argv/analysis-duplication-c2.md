AGENT: duplication
STATUS: clean
FINDINGS_COUNT: 0

FINDINGS: none

SUMMARY:
No significant duplication detected. The argv literal is pinned once in `cmd/open.go:97` and once in `cmd/open_test.go:1120` (test mirrors production contract — extracting a shared constant would obscure the test's purpose of pinning the exact argv at the syscall boundary). No cross-file repeated logic, no near-duplicate blocks, no helper-extraction opportunities introduced by this cycle.
