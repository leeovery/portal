# Analysis — Duplication (cycle 1)

STATUS: findings
FINDINGS_COUNT: 2
SUMMARY: Two low-severity duplication findings — one in-scope (assertion-loop overlap in tmux_test.go), one pre-existing (seed-session helper opportunity).

## Findings

### F1 — TestListSessions and TestListSessionsFiltersUnderscorePrefixed share an identical session-equality assertion loop

- SEVERITY: low
- FILES: `internal/tmux/tmux_test.go:120-135`, `internal/tmux/tmux_test.go:180-194`
- DESCRIPTION: The new `TestListSessionsFiltersUnderscorePrefixed` repeats the same field-by-field `[]tmux.Session` comparison block that already lives in `TestListSessions` — same length-check, same per-index Name/Windows/Attached `t.Errorf` trio. Both tests are table-driven over identical `(name, output, want)` shapes (the new test additionally has a non-nil-slice guard but is otherwise structurally the same). Two tests verify distinct contracts (parse correctness vs filter correctness), so this is borderline.
- RECOMMENDATION: Optional. Either (a) collapse the three filter cases into `TestListSessions`' existing table (add a `wantNonNilEmpty` flag for the empty-slice non-nil guard), or (b) extract `assertSessionsEqual(t, got, want)` and call it from both. If neither feels worth the churn, leave it — the duplication is shallow.

### F2 — Test seed-session bootstrap pattern repeated across two test files

- SEVERITY: low
- FILES: `cmd/bootstrap/reboot_roundtrip_test.go:237-238, 320-321, 927-928`; `internal/restore/integration_test.go:280-281, 359-360`
- DESCRIPTION: Five call sites pair `ts.Run(t, "new-session", "-d", "-s", "_seed"|"_bootstrap")` with `ts.WaitForSession(t, name, 2*time.Second)` to seed a placeholder underscore-prefixed session. The sites pre-date this work unit; this work unit consumed the convention via the new `verifyPostBootstrapSessionSet` allowedReserved arg.
- RECOMMENDATION: Defer. Mark as follow-up housekeeping for `internal/tmuxtest`. If touched in a future tmuxtest pass, introduce `tmuxtest.SeedSession(t *testing.T, ts *Socket, name string, timeout time.Duration)` and migrate the five sites in one pass.
