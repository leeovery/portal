TASK: restore-host-terminal-windows-4-1 — terminals.json store: load + tolerant decode

ACCEPTANCE CRITERIA:
- Load on a non-existent path returns an empty config, emits NO WARN, creates no file.
- Load on an unreadable file returns an empty config and emits exactly one spawn-component WARN (reason in detail).
- Load on a malformed-JSON file returns an empty config and emits exactly one spawn-component WARN.
- A valid entry with future introspect/place sub-keys parses successfully with those sub-keys ignored; Commands.Open retains the open recipe.
- Load performs reads only — never creates/truncates/writes (valid + missing cases).
- Load never returns a nil map (JSON null normalises to an empty TerminalsConfig).

STATUS: Complete

SPEC CONTEXT:
Spec §Config Schema (terminals.json) defines the user-authored escape-hatch: entry = identity key → commands → open → recipe (argv-array OR script path); future introspect/place are additive sub-keys. §Validation & error handling mandates tolerant decode consistent with Portal's other JSON stores — a malformed/unreadable file is ignored whole-file with a spawn-component WARN, unknown capability sub-keys ignored, never crash the picker. §Observability closes the spawn attr set; the opaque OS/config reason must ride the `detail` attr (no new attr key invented at call-site). Read-only at spawn time.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/terminalsconfig.go:1-83
- Notes: JSON model (Recipe {Argv/Script}, Capabilities {Open *Recipe}, TerminalEntry {Commands}, TerminalsConfig map[string]TerminalEntry) matches the spec's example JSON exactly. Open is a pointer so absent decodes to nil vs present-but-empty — correct per task. Unknown sub-keys (introspect/place) dropped for free by encoding/json (no DisallowUnknownFields), matching forward-compat intent. Load mirrors hooks.Store.Load's ENOENT/malformed structure but deliberately adds WARNs and swallows the error (returns only TerminalsConfig, never an error) — the deliberate divergence the task/spec call for since terminals.json drives command execution. Non-ENOENT read error → "terminals.json unreadable" WARN; unmarshal failure → "terminals.json malformed" WARN; both carry err text in the closed `detail` attr and reuse the existing package-level `detectLogger` (log.For("spawn"), defined in detect.go) — consistent with the sibling config WARNs in configadapter.go/recipe.go. nil-map normalisation for a literal JSON null is present. No Save/write method — read-only honoured. Recipe doc comment correctly defers exactly-one-of argv/script structural validation to a later task (4.3), so no scope creep here.

TESTS:
- Status: Adequate
- Coverage: internal/spawn/terminalsconfig_test.go:36-201 — one subtest per acceptance criterion: missing-file (empty + no WARN + os.Stat confirms no file created), unreadable (chmod 0000 with root-skip guard; empty + exactly 1 WARN + component=spawn + non-empty detail), malformed (empty + exactly 1 WARN + component + detail), valid-entry-with-unknown-sub-keys (Open recipe retained, argv verified, 0 WARN), read-only (byte-compare + mtime-compare before/after), JSON-null (non-nil + zero-length via range). White-box package spawn as required; t.TempDir() fixtures; WARNs asserted via logtest.Sink (installSpawnCapture swaps the test handler so detectLogger routes into the sink — verified the Sink captures the bound `component` attr, so the component/detail assertions are well-formed). `warnRecords` filter makes "exactly one WARN" a precise count. Would fail if the feature broke (e.g. dropping nil-normalisation fails the JSON-null subtest; writing the file fails the read-only subtest).
- Notes: Not over-tested — each subtest pins a distinct behaviour, no redundant happy-path duplication. The unreadable-file test's root-skip guard is a sound portability precaution. No behavioural gaps: valid-non-object JSON (array/number) folds into the tested malformed/unmarshal-error branch, so an extra case there would be redundant.

CODE QUALITY:
- Project conventions: Followed — mirrors hooks.NewStore(path) shape, XDG/path resolution deferred to cmd layer, tolerant-decode convention, closed spawn attr set (`detail`), reuses the Phase-1 spawn logger var.
- SOLID principles: Good — single responsibility (decode-only store), no premature abstraction; validation cleanly deferred to a later task.
- Complexity: Low — one linear function, clear branch ordering (ENOENT → read-err → unmarshal-err → nil-normalise).
- Modern idioms: Good in production code (errors.Is, os.ReadFile). One minor test-side idiom opportunity (see notes).
- Readability: Good — thorough doc comments state intent and the deliberate pointer/nil-normalisation rationale.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/terminalsconfig_test.go:191-201 — replace the hand-rolled `equalStrings` helper with `slices.Equal` (used at :125); a hand-rolled slice-equality loop is exactly what the repo's `modernize` linter flags.
- [quickfix] internal/spawn/terminalsconfig.go:68,74 — optional consistency: the two WARN message literals ("terminals.json unreadable" / "terminals.json malformed") are inline while detect.go hoists its spawn messages into named msg* constants; hoisting these to match the package convention would keep the spawn message catalog in one shape. Low value (these strings are not asserted by the tests), purely a convention alignment.
