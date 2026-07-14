TASK: restore-host-terminal-windows-3-3 — `--spawn-ack <batch>:<token>` flag on the existing `portal attach`

ACCEPTANCE CRITERIA:
1. `attach s1 --spawn-ack b1:t1` with existing s1 calls `AckWriter.Write("b1","t1")` exactly once and *then* `Connector.Connect("s1")` — write strictly before connect.
2. Ack writer scripted to error → `attach` still calls `Connect("s1")` (best-effort) and returns no error to caller.
3. `attach s1 --spawn-ack bogus` / `b1:` / `:t1` return a `*cmd.UsageError` (exit 2) and never call Write or Connect.
4. `attach ghost --spawn-ack b1:t1` with `HasSession("ghost")` false returns `No session found: ghost`, Write 0 times, no Connect.
5. `attach s1` (no flag) calls `Connect("s1")`, never touches the ack writer.

STATUS: Complete

SPEC CONTEXT:
Spec "Ack delivery & `portal attach` contract" (§ Burst & Partial-Failure): the carrier is a `--spawn-ack <batch>:<token>` flag on the composed argv; write ordering is "abridged bootstrap → confirm the session exists → write `@portal-spawn-<batch>-<token>` (value opaque; presence is the signal) → exec into tmux; the write is the last action before the exec handoff." Best-effort: attach still execs if the marker write fails (folds to a picker ack-timeout, the safe failed classification); a session that fails to resolve at attach time produces NO marker. Spec "Reporting & exit codes": usage error → exit 2 — a malformed `--spawn-ack` is the same class (`*cmd.UsageError`, which `main.classify` maps to exit 2, confirmed at main.go:105-108).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/attach.go:20-27 (AckWriter added to AttachDeps), :33-73 (RunE spawn-ack logic), :81-90 (buildAttachDeps wires spawn.NewServerOptionAckChannel(client, client)), :92-95 (flag registration in init).
- Notes: Order of operations matches spec exactly — flag parse+validate (fail-fast usage error before touching tmux) → HasSession check → best-effort Write → Connect. Marker write is strictly after the session-exists check (attach.go:54-70) and immediately before Connect (:72). Write failure logs DEBUG under the spawn component and falls through to Connect — does not return (best-effort). `ackRequested := ackVal != ""` treats an empty flag value as absent, so plain attach is byte-for-byte unchanged. UsageError → exit 2 mapping verified end-to-end via main.classify. `ParseSpawnAckFlag` (internal/spawn/ackid.go:83) correctly rejects missing-colon / empty-batch / empty-token. No drift from plan; the flag description string ("...@portal-spawn-<batch>-<token>...") is actually more correct than the plan's suggested text (uses the real hyphen-delimited marker name, not the colon).

TESTS:
- Status: Adequate
- Coverage: cmd/attach_test.go — TestAttachSpawnAck has all 5 planned subtests plus one extra (attr-vocabulary) subtest:
  * "it writes the ack marker after the session-exists check and before connect" — asserts Write("b1","t1") once, Connect("s1"), and order == ["write","connect"] via a shared call-order recorder (covers AC1).
  * "it still execs the attach when the marker write fails (best-effort)" — scripted Write error, asserts still Connect("s1") and no returned error (covers AC2).
  * "it returns a usage error (exit 2) for a malformed --spawn-ack value" — table over "bogus"/"b1:"/":t1", asserts errors.As(*UsageError), Write 0, Connect none (covers AC3).
  * "it writes no marker and takes the no-session path when the session is gone" — HasSession false, asserts "No session found: ghost", Write 0, no Connect (covers AC4).
  * "it leaves plain attach unchanged when --spawn-ack is absent" — asserts Connect("s1"), Write 0 (covers AC5).
  * "it routes the write-failure DEBUG through the enumerated detail attr" — asserts the DEBUG line carries session/batch/detail and NOT a non-enumerated `error` attr (guards the closed attr-key vocabulary invariant).
- Notes: Would fail if the feature broke (order recorder catches write-after-connect regressions; UsageError type-assert catches misclassification; Write-count assertions catch write-on-gone-session). Flag state is properly reset between subtests via resetRootCmd (cmd/root_test.go resets spawn-ack value + Changed), so no cobra package-level flag leakage. The DEBUG attrs (session/batch/detail) are all within the spawn component's already-established closed vocabulary (internal/spawn/logemit.go uses session/batch/detail). The 6th subtest is not redundant — it verifies a distinct project invariant (never-invent-at-call-site attr keys), not the same happy path.

CODE QUALITY:
- Project conventions: Followed. Interface-based DI via *AttachDeps seam (AckWriter spawn.AckWriter), package-level nil-injected production path, buildAttachDeps chokepoint. Unit-lane test with no t.Parallel(), bootstrapDeps short-circuit + full tmux-seam injection (TMUX poison satisfied). Component logger (spawnLogger = log.For("spawn")) used, not a hand-rolled slog.
- SOLID principles: Good. Single seam per responsibility (connect/validate/write); attach depends on the narrow AckWriter.Write(batch,token) interface, not a concrete client.
- Complexity: Low. One added branch plus a best-effort write block; linear, clear.
- Modern idioms: Yes. strings.Cut in ParseSpawnAckFlag; errors.As in tests.
- Readability: Good. Comments explain the write-point/ordering and best-effort rationale inline, tracking the spec.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/attach.go:92-95 — the `--spawn-ack` flag is described as "internal:" but is not marked hidden, so it appears in `portal attach --help`. Consider `_ = attachCmd.Flags().MarkHidden("spawn-ack")` in init to keep the internal spawn carrier out of user-facing help. Judgment call (recipe/argv authors composing the command may want it visible), hence idea rather than a mechanical fix.
