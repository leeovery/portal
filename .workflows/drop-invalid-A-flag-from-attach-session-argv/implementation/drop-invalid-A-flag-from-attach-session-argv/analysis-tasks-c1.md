# Analysis Cycle 1 — Proposed Tasks

- topic: drop-invalid-A-flag-from-attach-session-argv
- cycle: 1
- total_findings: 1
- deduplicated_findings: 1
- proposed_tasks: 1

---

## Task 1: Drop stale `-A` from cmd/reattach_integration_test.go comments

- status: approved
- severity: medium
- sources: standards

**Problem**: Two file-level documentation comments in `cmd/reattach_integration_test.go` still assert the now-invalid `attach-session -A` argv shape for the bare-shell `AttachConnector` path:
- Line 48: `//  4. \`exec attach-session -A\` (bare-shell) attach path verified`
- Line 494: `// (\`syscall.Exec\` + \`tmux attach-session -A -t NAME\`). We substitute a`

The specification's Verification step prescribes `grep -rn 'attach-session.*-A\|"-A".*attach-session' cmd/ internal/` returning only `new-session -A` matches; these two comments violate that criterion. Leaving them preserves the same false "-A-is-canonical" claim that caused the original bug.

**Solution**: Edit both comments to drop the `-A` token, leaving the surrounding text otherwise unchanged. Documentation-only.

**Outcome**: Spec grep returns no `attach-session` hits with `-A`; comments accurately describe the post-fix bare-shell attach argv.

**Do**:
1. Open `cmd/reattach_integration_test.go`.
2. Line 48 → `//  4. \`exec attach-session\` (bare-shell) attach path verified`.
3. Line 494 → `// (\`syscall.Exec\` + \`tmux attach-session -t NAME\`). We substitute a`.
4. Run `go test ./cmd/...` to confirm build/tests intact.
5. Run the spec grep to confirm zero `attach-session` + `-A` hits.

**Acceptance Criteria**:
- `cmd/reattach_integration_test.go:48` no longer contains `-A`.
- `cmd/reattach_integration_test.go:494` no longer contains `-A`.
- `grep -rn 'attach-session.*-A\|"-A".*attach-session' cmd/ internal/` returns no matches.
- `go test ./...` passes.

**Tests**:
- No new tests required (doc-only edit). Existing `go test ./...` must continue passing. Spec verification grep serves as acceptance check.
