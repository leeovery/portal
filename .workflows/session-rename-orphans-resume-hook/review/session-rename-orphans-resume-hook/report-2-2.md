TASK: Switch resolveCurrentPaneKey (cmd/hooks.go) to resolve the hook key (tick-94ec77, session-rename-orphans-resume-hook-2-2)

ACCEPTANCE CRITERIA:
- resolveCurrentPaneKey() resolves via ResolveHookKey(paneID) — no ResolveStructuralKey call remains in cmd/hooks.go.
- hooksDeps.KeyResolver seam is a hook-key resolver interface with single method ResolveHookKey(paneID string) (string, error); production *tmux.Client satisfies it.
- portal hooks set stores the resolved hook key (mock 'tok123:0.0' -> written under 'tok123:0.0').
- ResolveHookKey read failure aborts registration: set returns user-facing error (contains 'resolve') and writes NO hooks file; rm returns the same error, existing entry untouched. No name-based key synthesized.
- rm --pane-key <key> removes verbatim <key> without consulting the resolver (even with TMUX_PANE unset).
- Missing TMUX_PANE still errors 'must be run from inside a tmux pane' for set and the rm fallback (--pane-key exempt).
- go build succeeds; go test ./cmd -run 'TestHooks' passes.

STATUS: Complete

SPEC CONTEXT:
Spec §"Hook-Key Derivation" Stage 1 — Registration (cmd/hooks.go). resolveCurrentPaneKey() must switch from the name-based ResolveStructuralKey/StructuralKeyFormat read to the hook-key read via HookKeyFormat (ResolveHookKey), so hooks store under the immutable @portal-id when a session is stamped (rename-immune), else the session name. Failure contract: ResolveHookKey is a single display-message read; stamped-vs-unstamped resolves inside tmux (no Go-side "id absent" branch). If the read fails, registration ABORTS with the error and MUST NOT synthesize a name-based key (that would silently orphan a stamped session's hook). --pane-key literal pass-through on rm is unchanged. Acceptance Criteria 6: no external/UI change.

IMPLEMENTATION:
- Status: Implemented (matches spec + acceptance exactly)
- Location:
  - cmd/hooks.go:16-18 — HookKeyResolver interface, single method ResolveHookKey(paneID string) (string, error), doc-comment describes <@portal-id or session_name>:window.pane via HookKeyFormat.
  - cmd/hooks.go:25-27 — HooksDeps.KeyResolver field typed HookKeyResolver.
  - cmd/hooks.go:48-67 — resolveCurrentPaneKey(): requireTmuxPane() gate unchanged (line 49-52), resolver selection now HookKeyResolver (54-59), calls keyResolver.ResolveHookKey(paneID) (61), wraps error as "failed to resolve hook key for current pane: %w" and returns "" (62-64), local renamed hookKey.
  - cmd/hooks.go:104-119 (set) — resolveCurrentPaneKey() then store.Set; control flow unchanged.
  - cmd/hooks.go:144-166 (rm) — branches --pane-key verbatim (151-152, resolver NOT consulted) vs resolveCurrentPaneKey() fallback (153-158); unchanged.
  - internal/tmux/tmux.go:344-350 — (*Client).ResolveHookKey (Task 2-1) satisfies the seam; production buildHooksTmuxClient() returns *tmux.Client assigned to the HookKeyResolver var at hooks.go:58 (structural compile-time check).
- Notes: grep confirms zero ResolveStructuralKey / StructuralKeyResolver / structuralKey references remain in cmd/hooks.go or cmd/hooks_test.go. The only remaining ResolveStructuralKey in the tree is the still-live internal/tmux client method + its own tests + a bootstrap comment — out of this task's scope and intentionally retained (spec Stage 2 keeps StructuralKeyFormat available for non-hook structural use).

TESTS:
- Status: Adequate
- Coverage (cmd/hooks_test.go):
  - mockKeyResolver.ResolveHookKey renamed (line 144).
  - "it stores the hook under the resolved hook key" (351) — mock 'tok123:0.0' -> written under 'tok123:0.0'. Covers criterion 3.
  - "it aborts hooks set when the hook-key read fails" (323) — resolver err, asserts error contains 'resolve' AND os.Stat shows no file created (no side effects). Covers criterion 4 (set).
  - "it aborts hooks rm when the hook-key read fails and leaves the entry intact" (606) — seeds entry, resolver err, asserts error contains 'resolve' AND entry still present. Covers criterion 4 (rm).
  - "it removes the verbatim key on rm --pane-key without consulting the resolver" (703) — TMUX_PANE unset, resolver rigged to fail loudly, verbatim sess:0.1 removed, other entry preserved. Covers criterion 5.
  - "it errors when TMUX_PANE is unset for set" (375) and "it errors when TMUX_PANE is unset for the rm fallback" (738) — both assert 'must be run from inside a tmux pane'. Covers criterion 6.
  - Pre-existing tests retained and still green: overwrite idempotency, JSON structure, silent no-op, multi-key removal, last-event cleanup, --pane-key fallback branch.
- Notes: Tests verify observable behaviour (stored/removed key, error text, filesystem side effects), not implementation details. Failure paths assert BOTH the error and the no-side-effect invariant — exactly the edge cases the spec calls out. Not over-tested: some overlap between the older "sets hook for current pane"/"writes correct JSON structure" and the new "stores under resolved hook key", but each exercises a distinct facet (write, JSON shape, resolved-key routing) — acceptable, not redundant. package cmd, no t.Parallel(), mocks injected via package-level hooksDeps with t.Cleanup restore — matches the mandated pattern.

CODE QUALITY:
- Project conventions: Followed. Small (1-method) DI interface, package-level *Deps injection, t.Cleanup teardown, no t.Parallel() — per CLAUDE.md and golang-dependency-injection skill. Error wrapping with %w per golang-error-handling.
- SOLID principles: Good. Interface segregation (single-method seam), dependency inversion (cmd depends on HookKeyResolver, not *tmux.Client).
- Complexity: Low. resolveCurrentPaneKey is a linear gate -> select -> resolve -> wrap.
- Modern idioms: Yes. errors wrapped with %w; nil-check on both hooksDeps and KeyResolver before use.
- Readability: Good. Doc-comments updated to describe hook-key semantics; local renamed structuralKey -> hookKey.
- Issues: None functional.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/hooks_test.go:167,192,195,305,310,313,407,424,427,431,453,456,565,599 — residual "structural key" wording in comments, t.Error messages, and one subtest name ("reads pane ID from TMUX_PANE and resolves structural key") carried over from before the rename; the resolver now produces a hook key. Reword "structural key" -> "hook key" so the test vocabulary matches the seam. Documentation/message-only, no logic impact.
- [do-now] cmd/hooks.go:58 — production *tmux.Client satisfies HookKeyResolver only via the implicit assignment in resolveCurrentPaneKey; consider adding a compile-time assertion `var _ HookKeyResolver = (*tmux.Client)(nil)` near the interface to make the contract explicit and fail fast if the method signature drifts. (Optional; current structural check is sufficient.)
