AGENT: standards
STATUS: findings
FINDINGS_COUNT: 1

FINDINGS:

- FINDING: Stale `attach-session -A` references in cmd/reattach_integration_test.go fail spec verification grep
  SEVERITY: medium
  FILES:
    - cmd/reattach_integration_test.go:48
    - cmd/reattach_integration_test.go:494
  DESCRIPTION: |
    Two file-level documentation comments still assert the now-invalid `attach-session -A` argv shape for the bare-shell `AttachConnector` path.
    - Line 48: `//  4. exec attach-session -A (bare-shell) attach path verified`
    - Line 494: `// (syscall.Exec + tmux attach-session -A -t NAME). We substitute a`

    Both are stale after the production fix. The specification's Verification section (line 24) prescribes:
        grep -rn 'attach-session.*-A\|"-A".*attach-session' cmd/ internal/
    and requires it to return only `new-session -A` matches — currently it returns these two comment hits. The spec's primary scope is `cmd/open.go` and the upstream enter-attaches-from-preview spec, but the verification criterion extends conformance to the whole `cmd/` + `internal/` tree.

    Leaving these stale comments in keeps the same false "-A-is-canonical" claim that caused the original bug visible to future re-derivers — exactly the failure mode the upstream-spec corrigendum was added to prevent.
  RECOMMENDATION: |
    Drop `-A` from both comments.
    - Line 48 → `//  4. \`exec attach-session\` (bare-shell) attach path verified`
    - Line 494 → `// (\`syscall.Exec\` + \`tmux attach-session -t NAME\`). We substitute a`
    Documentation-only change. No code or test logic affected.

SUMMARY:
Production argv at `cmd/open.go:97`, the `AttachConnector` docstring at lines 77-83, the unit-test argv assertion at `cmd/open_test.go:1120`, and the upstream-spec corrigendum and corrected lines all conform to the specification. The `new-session -A` form in `internal/session/quickstart.go:52` is correctly left untouched per spec exclusion. The only drift is the two stale documentation comments above, which fail the spec's own grep verification step.
