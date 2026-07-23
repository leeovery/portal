# Plan: remote-trigger-spawns-on-local-terminal

## Phases

### Phase 1: Gate Locality on the Triggering (Most-Active) Client
status: approved
approved_at: 2026-07-23

**Goal**: Invert `detectInsideTmux` (`internal/spawn/detect_inside.go`) so it selects the triggering client — the one with the highest `client_activity` across ALL enumerated clients (local and remote alike, first-listed winning an exact tie) — and then walks ONLY that winner, branching on the winner's locality. A remote-triggered burst with a host-local client also on the session now resolves NULL (the same atomic no-op as the pure-remote case) instead of driving the N−1 windows onto a machine the user is not at, while a legitimate local trigger still drives. The owned trade — winner-only walk drops the "one flaky ps cannot mask a resolvable local" resilience for the winner (fail-safe to NULL + WARN on a transient winner walk) — is made explicit in the rewritten docstring contract and locked in by the reframed regression test.

**Why this order**: This is a single-root-cause, single-file bugfix; per the bugfix single-phase model it is one contained phase. It begins with a failing test that reproduces the reported bug (remote most-active + host-local idle → must be NULL), fixes the root cause (the filter-then-tiebreak ordering inversion), owns the deliberately-dropped resilience property in both the docstring and a regression test rather than slipping it in, and re-verifies every pinned invariant. The three affected surfaces (CLI burst, TUI picker burst, `portal doctor` host-terminal line) and the re-armed `m`-entry safeguard all inherit the correction with no separate change, so there is no downstream phase to sequence.

**Acceptance**:
- [ ] A regression test reproducing the reported bug (remote client most-active + host-local client idle on the same session) fails before the code change and passes after, asserting NULL identity / honest no-op
- [ ] `detectInsideTmux` computes the winner as the max-`client_activity` client over the existing `ListClients(session)` enumeration across all clients (local and remote), with first-listed winning an exact activity tie, and walks only that winner (no extra tmux round-trip; the `detectInsideTmux(session, lister, walker, reader)` signature and `clientLister` seam unchanged)
- [ ] Winner resolves to a local `.app` → returns its `Identity` (drives); winner walks to clean NULL → NULL identity, nil error; winner walk transient-fails → NULL + `ErrDetectTransient`-wrapped error; empty client list → clean NULL, nil error
- [ ] The two subtests that codify the bug are transformed, not deleted: the ~`:133` "mixed local+remote" test is inverted to expect NULL for a most-active remote + idle local; the ~`:196` resilience test is reframed so a transient walk on the most-active winner yields NULL + `ErrDetectTransient`-wrapped error
- [ ] A net-new subtest covers local most-active + remote idle bystander → the local drives (guarding against over-correction that would refuse a legitimate local spawn)
- [ ] All pinned existing invariants remain green: pure-remote → NULL; single-local → drives; 2+ all-local → highest-activity local (first-listed on tie); list-clients enumeration failure → NULL + transient; single-client walk failure → NULL + transient (retained, not deleted as a supposed duplicate of the reframed `:196`); empty client list → clean NULL
- [ ] The `detect_inside.go` docstring contract — both the algorithm description and the Outcomes list — is rewritten to describe most-active-winner selection and winner-only walk with fail-safe-to-NULL-on-transient-winner; the two directly-inverted sentences ("NULL-filtering is the primary signal" and "client_activity is used ONLY to disambiguate among host-local clients — never as a cross-client primary signal") are gone, leaving no stale contract text
- [ ] The fast hermetic unit lane (`go test ./...`) passes; `internal/spawn/detect.go` and the `Detect()` consumers (CLI burst, TUI picker burst, `portal doctor`) are unchanged
- [ ] Manual end-to-end verification in the reported reproduction setup: trigger a burst (either surface) from a remote SSH/mosh client while a host-local terminal is attached to the same session, and confirm the honest no-op — no windows open on the host machine

#### Tasks
status: draft

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| remote-trigger-spawns-on-local-terminal-1-1 | Reproduce the bug and invert the locality gate in detectInsideTmux | empty client list → clean NULL nil-error, winner walk transient-fail → NULL + ErrDetectTransient, exact activity tie → first-listed wins, list-clients enumeration failure → NULL + transient, single-client walk failure → NULL + transient (retained) |
| remote-trigger-spawns-on-local-terminal-1-2 | Guard the fix against over-correction with a local-most-active regression test | remote idle bystander attached but local still drives |
| remote-trigger-spawns-on-local-terminal-1-3 | Manually verify the honest no-op end-to-end in the reported reproduction setup | none |
