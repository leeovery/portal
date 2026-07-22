TASK: Lock CLI Open-Burst Copy Coherence With An Explicit Literal Regression (persistent-no-host-terminal-banner-3-2, commit 54f6894c)

ACCEPTANCE CRITERIA:
- A new CLI test asserts, with a byte-literal want (not spawn.UnsupportedNoopMessage(id)), that the named unsupported no-op returns exactly `can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened`.
- The same test (or its NULL branch) asserts the NULL unsupported no-op returns exactly `can't open new windows over a remote connection — nothing opened`.
- The new test would FAIL if UnsupportedNoopMessage copy drifted from spec §5 (does not delegate to the function for its expected value).
- TestRunOpenBurst_UnsupportedTerminal_AtomicNoop remains and still passes (no burster, no half-connect, error names the identity).
- cmd/open_burst_run.go is unchanged (no block-logic edit).
- go test ./cmd/... passes on the unit lane.

STATUS: complete

SPEC CONTEXT:
Spec §5 (copy set, specification.md:126) defines the reactive no-op "Unsupported no-op" message — shared by spawn.UnsupportedNoopMessage and rendered by BOTH the TUI async-race path and the CLI open-burst: named = `can't open new windows in <name> · <bundleID> — nothing opened`, NULL = `can't open new windows over a remote connection — nothing opened`. §7 requires the CLI open-burst copy assertions to pin the new strings; §8 Non-goals forbids any CLI block-logic change (CLI still detects, resolves, returns errors.New(spawn.UnsupportedNoopMessage(id))). The task closes the "self-reference trap": the pre-existing CLI assertion computed want := spawn.UnsupportedNoopMessage(id), so it silently tracked wording drift and never pinned the literal; the NULL shape had no CLI copy coverage at all.

IMPLEMENTATION:
- Status: Implemented (test-only, exactly as scoped)
- Location: cmd/open_burst_run_test.go — new TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage (added at commit 54f6894c lines 519-593 of the diff; present in current tree at lines 550-590).
- Confirmed `git show 54f6894c --stat`: only cmd/open_burst_run_test.go changed among code files (+ .tick/tasks.jsonl and workflow manifest.json bookkeeping — not code). cmd/open_burst_run.go is NOT in the commit — production block logic unchanged.
- Production call site verified unchanged (cmd/open_burst_run.go:166-168): `if resolution == spawn.ResolutionUnsupported { spawn.LogUnsupported(deps.Logger, id); return errors.New(spawn.UnsupportedNoopMessage(id)) }`.
- Byte-literals in the test exactly match the production renderer (internal/spawn/message.go:79-84) AND spec §5 (specification.md:126): named `Apple Terminal · com.apple.Terminal`, NULL remote line — verbatim, including the U+00B7 middle-dot separator and U+2014 em-dashes.
- New test drives the required two-surface N≥2 burst: two spawn.Surface{Kind: SurfaceAttach} entries ("a","b"). At commit 54f6894c the two surfaces were inline; the current tree routes through the shared runUnsupportedOpenBurstNoOp helper (added later by task 4-1, commit 5b6352ab) which supplies the same two-attach slice — 4-1 is out of scope here and the byte-literal want assertion is preserved through that refactor.
- Notes: The `want` values are hardcoded string literals, NOT computed via spawn.UnsupportedNoopMessage(id) — the load-bearing anti-drift property is satisfied. A copy edit to message.go would break this test while the sibling AtomicNoop (self-referencing) test would silently follow — exactly the intended split.

TESTS:
- Status: Adequate
- Coverage: Named shape and NULL shape both covered via a two-entry table over {id, want}. Both branches assert the exact byte-literal error string plus the atomic-no-op structural invariants (err != nil, NewBurster never built, zero OpenWindow calls, zero Connector.Connect targets, zero LocalMint calls). This is the "nothing half-opened" contract for the N≥2 unsupported path.
- Verified by reading: bootstrap wiring routes ResolutionUnsupported → errors.New(spawn.UnsupportedNoopMessage(id)); the two literals equal the renderer output for spawn.Identity{} (IsNull) and appleTerminalIdentity(); `go test ./cmd/... -run TestRunOpenBurst_UnsupportedTerminal` reports ok (unit lane — no tmux server, no daemon, no built binary), matching the lane-purity acceptance criterion.
- Not under-tested: the previously-uncovered NULL/remote CLI copy shape is now pinned.
- Not over-tested: the structural spies (burster/OpenWindow/Connect/mint) duplicate what AtomicNoop already asserts, but the task DO section explicitly permits keeping the spies ("Keep existing spies … or at minimum assert err != nil and the exact string"), and the two-test split (computed vs byte-literal message) is the deliberate design. Not flagged as over-testing.
- Notes: AtomicNoop (behaviour test) confirmed still present and still asserting the computed message plus atomic no-op invariants (cmd/open_burst_run_test.go:534-548).

CODE QUALITY:
- Project conventions: Followed. Table-driven subtests, `t.Helper()` on shared scaffolding, error-last return ordering (ST1008) on runUnsupportedOpenBurstNoOp, no t.Parallel(), fully-injected *Deps seams (fakeTerminalDetector + fixed Resolve), poisoned-TMUX-safe (no real tmux touch). Matches golang-testing conventions.
- SOLID principles: Good — test exercises the seam boundary, no production coupling introduced.
- Complexity: Low — linear table loop, single execute + assertions per case.
- Modern idioms: Yes — idiomatic Go subtests.
- Readability: Good — the leading comment explicitly documents the self-reference trap and why `want` must be a literal, so intent survives future edits.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
