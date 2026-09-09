TASK: resume-hooks-silently-lost-7-21 — "Two Pairs Of Line-Identical Subtests In cmd/hooks_test.go"
(delete the two duplicate `hook rm` subtests, keep the better-named survivors)

ACCEPTANCE CRITERIA:
- `:761` ("it removes the verbatim key on rm --pane-key without consulting the resolver") and `:791`
  ("it errors when TMUX_PANE is unset for the rm fallback") are deleted; `:705` and `:550` are untouched.
- The `--pane-key`-without-`TMUX_PANE` removal is still asserted, in `cmd/hooks_test.go:705` and
  `cmd/hooks_rm_exit_test.go:143`.
- The unset-`TMUX_PANE` rm error is still asserted, in `cmd/hooks_test.go:550`.
- `go test ./cmd` passes and the commit names the reduction.

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context: phases 6–10 are consolidation/quality cycles the implementation
generated). The subject matter is the `hook rm` command's exit contract — `hook rm` exits 0 iff it removed an
entry, the `--pane-key` pass-through removes a verbatim key without consulting tmux at all, and an unset
`TMUX_PANE` on the fallback path is a Portal-worded refusal ("must be run from inside a tmux pane"). Nothing in
that contract may lose an assertion to the de-duplication; the task's whole claim is "two subtests fewer and not
one assertion fewer".

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/hooks_test.go (the deletions), cmd/hooks_rm_exit_test.go (the independent coverage relied on)
- Notes:
  * Both named duplicates are gone. `TestHooksRmCommand` (cmd/hooks_test.go:477) now holds exactly seven
    subtests — :478 "returns error when TMUX_PANE is not set", :498 "returns error when on-resume flag is not
    provided", :518 "cleans up pane key when last event removed", :544 "it aborts hooks rm when the hook-key read
    fails and leaves the entry intact", :571 "--pane-key flag removes specified key without requiring TMUX_PANE",
    :600 "--pane-key unset falls back to resolveCurrentPaneKey", :623 "--pane-key help describes a verbatim hook
    key". Neither deleted name ("it removes the verbatim key on rm --pane-key without consulting the resolver",
    "it errors when TMUX_PANE is unset for the rm fallback") appears anywhere in the file.
  * The two survivors named by the criteria are present and intact. `:705`'s survivor is cmd/hooks_test.go:571,
    and `:550`'s is cmd/hooks_test.go:478 — the latter identified by the discarded-return `hooksFileInTempDir(t, nil)`
    at :479, which is what distinguishes the rm-side subtest from its identically-named set-side neighbour at
    cmd/hooks_test.go:328 (that one binds `_, hooksFile :=` at :329 and additionally asserts the file was never
    created). The set-side twin was correctly left alone: the task deletes the rm-side duplicate only.
  * The survivor keeps the distinguishing claim of the twin it absorbed: cmd/hooks_test.go:578 injects the poisoned
    pair via `paneKeyPathSeams()` (declared cmd/hookkey_vocabulary_test.go:183) and cmd/hooks_test.go:597 calls
    `assertNoPaneTmuxCalls` (cmd/hookkey_vocabulary_test.go:187), so "without consulting the resolver" — the whole
    subject of the deleted `:761` — is still asserted, on both the resolver and the stamper.
  * The independent `--pane-key` coverage the task leans on exists: cmd/hooks_rm_exit_test.go:274 ("it exits 0 and
    removes on the --pane-key path") stages an empty `TMUX_PANE` at :279, the poisoned pair at :281, removes an
    old-format key at :286 and asserts `assertNoPaneTmuxCalls` at :294. (The task cited it as
    `cmd/hooks_rm_exit_test.go:143`; line numbers in that file have moved since the task was authored — the case
    itself is at :274 today.)
  * No orphaned scaffolding was left by the deletions. Every helper the removed subtests would have used is still
    read by live callers: `paneKeyPathSeams` at cmd/hooks_test.go:578 and cmd/hooks_rm_exit_test.go:68, :233, :281;
    `assertNoPaneTmuxCalls` at cmd/hooks_test.go:597 and cmd/hooks_rm_exit_test.go:104, :294; `mockKeyResolver`
    throughout both files. cmd/hooks_test.go's whole import block (bytes, errors, fmt, log/slog, os, path/filepath,
    strings, testing, hookstest, logtest, state, tmux) still has at least one live use after the deletions —
    `fmt` at :550, `log/slog` at :741/:743, `path/filepath` at :774, `state` at :729, `logtest` at :738 — so the
    package still compiles, which is the compile-time half of "`go test ./cmd` passes".
  * The subtest name at cmd/hooks_test.go:600 names `resolveCurrentPaneKey`, which exists at cmd/hooks.go:67 — the
    surviving names describe the code as it stands.

TESTS:
- Status: Adequate (this task's deliverable *is* the test change; the verification is that coverage is unchanged)
- Coverage:
  * `--pane-key` removal without `TMUX_PANE`, and reaching tmux for nothing: cmd/hooks_test.go:571 (removal,
    neighbouring entry untouched, no resolver/stamper calls) and cmd/hooks_rm_exit_test.go:274 (exit 0 on an
    old-format key, no resolver/stamper calls). Both survive.
  * Unset `TMUX_PANE` on the rm fallback: cmd/hooks_test.go:478 asserts the non-nil error and the
    "must be run from inside a tmux pane" substring — the exact pair the deleted `:791` asserted. The behaviour is
    also observed from a different angle by the "unset TMUX_PANE" row of the byte-identity table at
    cmd/hooks_rm_exit_test.go:311, which asserts hooks.json is left byte-identical on that route.
  * Nothing else in the two files depended on the deleted subtests, so no assertion was lost with them.
- Notes: The residual overlap between cmd/hooks_test.go:571 and cmd/hooks_rm_exit_test.go:274 is deliberate and
  named in the task body ("nothing rests on `:761` alone"), and the criteria require `:571` to stay untouched —
  the two differ in subject (the command suite's removal behaviour vs. the exit-code contract) and in fixture (a
  token-shaped key removed beside an old-format one, vs. an old-format key removed). Not a leftover duplicate.

CODE QUALITY:
- Project conventions: Followed. The change is a pure deletion inside `//go:build`-untagged unit-lane test files;
  no `t.Parallel()`, no tmux reach (both surviving `--pane-key` cases inject `HooksDeps` seams through
  `withHooksDeps`, satisfying CLAUDE.md's rule that a test Executing a real command body injects every
  tmux-touching seam), no new logger construction, no hand-rolled hooks.json path (both go through
  `hooksFileInTempDir`, which delegates to `hookstest.StageStore`).
- SOLID principles: N/A for a test deletion; the seam boundaries are untouched.
- Complexity: Low — two subtests removed, nothing restructured.
- Modern idioms: N/A (no code added).
- Readability: Good. The surviving names state their subjects, and the better-named subject the task wanted kept
  (`cmd/hooks_rm_exit_test.go:274`) reads against the exit contract rather than against an implementation detail.
- Comment accuracy: The comments adjacent to the survivors hold. cmd/hookkey_vocabulary_test.go:180-182 explains
  why `paneKeyPathSeams` and `assertNoPaneTmuxCalls` are used as a pair, which is exactly how
  cmd/hooks_test.go:578/:597 use them; cmd/hooks_rm_exit_test.go:284-285's "the pass-through validates nothing"
  matches cmd/hooks.go:270-271, where a non-empty `--pane-key` is taken verbatim with no shape check.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.

VERIFICATION NOT PERFORMED:
- The commit-message half of the fourth criterion ("the commit names the reduction") was not checked: my remit
  permits no shell use beyond renaming this report, so the git history was out of reach. It is recorded here as
  unverified rather than as a finding — a commit message is not a code defect, and a note whose entire remedy is
  prose could not block in any case. The code-side half of that criterion (the package still compiles after the
  deletions, with no orphaned helper and no now-unused import) was verified by reading, and holds.
