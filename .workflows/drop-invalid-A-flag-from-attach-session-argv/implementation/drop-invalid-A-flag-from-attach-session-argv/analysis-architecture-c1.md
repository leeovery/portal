AGENT: architecture
STATUS: clean
FINDINGS_COUNT: 0

FINDINGS: none

SUMMARY:
Quick-fix is architecturally sound — single argv token dropped at the one `AttachConnector` seam, docstring trimmed to the surviving `=`-prefix justification, test assertion updated in lockstep, upstream spec corrected with a corrigendum. No new abstractions, no scope creep, no boundary changes; the existing execer seam + `recordingExecer` pattern remain the right shape for pinning argv at the `syscall.Exec` boundary.
