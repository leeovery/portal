TASK: Hooks.json Accepts the Object Form and Preserves What It Did Not Write (lazy-resume-on-attach-1-2, tick-711d73)

ACCEPTANCE CRITERIA:
- A string value loads as a registration carrying that command and no mode; an object carrying `command` and `resume` loads as that command and that mode.
- After `Set` rewrites one entry, every other entry's value is byte-equivalent to what it held — an unmodelled attribute (e.g. "nickname"), an unrecognised `resume` value (e.g. "LAZY"), and a value that is neither string nor object alike. Key order inside a preserved object is unchanged; file-level indentation is the store's and is not part of the contract.
- A value that is neither string nor object (number, array, boolean, null) does not fail the load: the other entries load normally and that entry round-trips untouched.
- A registration built by a mutation and carrying no mode is written as a plain JSON string; one carrying a mode is written as an object holding `command` and `resume`.
- `clean-stale`'s recoverable `value` breadcrumb carries the command out of an object-form entry, exactly as out of a string-form one, and a key holding several events still renders every `event=command` pair in event order.
- `doctor`'s stale-hook count, the daemon's sweep and `StaleKeys` behave exactly as before across the existing suites.
- Re-registering the same command over a hand-written object-form entry that carries a mode is a `modify` that writes, not a `set-noop`; the entry comes back as a plain string carrying no mode while every other entry keeps its bytes.

STATUS: issues_found

SPEC CONTEXT: Section 3.2 makes an event's stored value either the command as a string or an object carrying `command` plus `resume`. Both shapes are permanent, with no migration between them, and the writer picks by whether there is anything to carry. A value the reader cannot make sense of never fails a pane, and nothing is rewritten to correct it. A rewrite of one registration leaves every other entry exactly as it found it: an unmodelled attribute and an unrecognised `resume` value both survive. Section 2.2 says a registration is written whole: what a call is handed is all that entry keeps. The `SessionStart` hook rewrites the whole file roughly hourly, so preservation happens on every rewrite, not as a rare edge case.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/hooks/registration.go:14-24 — `Registration{Command, Resume, raw}`.
  - internal/hooks/registration.go:31-48 — `UnmarshalJSON` never errors and always retains a copy of the bytes. A string becomes the command. For an object, `command` and `resume` are read through `stringAttribute` and `resumemode.Parse`. Any other JSON value sets neither field, including `null`: decoding null into a string succeeds with no error, which leaves `Command` empty and `raw` holding "null".
  - internal/hooks/registration.go:53-64 — `MarshalJSON` on the value receiver emits `raw` when present. Otherwise it emits a string when `Resume` is unset and the `{"command","resume"}` object when it is set.
  - internal/hooks/registration.go:68-70 — `sameRegistration` compares command and mode only.
  - internal/hooks/store.go:34 — `Snapshot` is `map[string]map[string]Registration`.
  - internal/hooks/store.go:162 — `Set` stores a registration rebuilt from its command and mode, with no raw bytes, so the rewritten entry is written from its own content.
  - internal/hooks/store.go:174-187 — `classifySet` goes through `sameRegistration`.
  - internal/hooks/store.go:416-434 — `removedValue` renders `.Command`.
  - internal/hooks/lookup.go:37-41 and internal/hooks/store.go:262-270 — `LookupOnResume` and `List` read `.Command`.
  - internal/hooks/leaf_guard_test.go:26 — `resumemode` is added to `hooksMayImport`.
  - The CLAUDE.md `hooks` row and the README `hooks.json` row (README.md:420) carry the prescribed text, and the lock sentence is kept.
- Notes:
  - The code has moved past this task's text, and soundly. Later tasks changed `Set` to take a `Registration` (store.go:134) and made `List` and `LookupOnResume` carry the mode. These are the planned follow-ons, not drift.
  - Not flagged: re-registering the same command and mode over an object that also carries an unmodelled attribute is a `set-noop`, so that attribute survives. This is what the plan prescribes for `classifySet`, and the reader sees no difference.
  - The one divergence that matters: the preserved bytes are not emitted verbatim. See FINDINGS.

TESTS:
- Status: Adequate
- Coverage: All 11 named tests exist:
  - internal/hooks/registration_test.go:40, 59 and 78 (the table over 42, ["x"], true, null), 116, 126, 159, 163, 167 and 175.
  - internal/hooks/store_test.go:1028 and 1044.

  Preservation is asserted on compacted stored bytes, not on a re-decoded view (the `storedValue` helper, registration_test.go:18-37). The modify case checks all three effects: the `modify` record, the entry rewritten as a plain string, and the sibling's bytes unchanged. The malformed-file edge is still covered by existing tests (store_test.go:63, 231, 463). The `Snapshot` literals in hooksweep were updated (for example internal/hooksweep/decline_error_test.go:19), and the `StaleKeys` tests were re-pinned over the new value type (store_test.go:806-858).
- Notes: The key-order clause of the second criterion is not observed by any test. See FINDINGS.

CODE QUALITY:
- Project conventions: Followed. The leaf guard was updated, `resumemode` is used as the single vocabulary, and the store stays the logging chokepoint.
- SOLID principles: Good. The codec concern is contained in `Registration`.
- Complexity: Low
- Modern idioms: Yes (`bytes.Clone`). The value-receiver `MarshalJSON` is required here, because map elements are not addressable.
- Readability: Good
- Issues: Only the escaping behaviour behind the "verbatim" claim; see FINDINGS.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/hooks/store.go:121 — `save` encodes through `json.MarshalIndent`, which HTML-escapes. The encoder re-compacts every `MarshalJSON` result with HTML escaping on, including the retained bytes `Registration.MarshalJSON` returns (internal/hooks/registration.go:54-55). So a preserved entry is not re-emitted verbatim: each `<`, `>`, `&` (and U+2028/U+2029) inside its strings comes back as a `\u00XX` escape. The fix is to encode in `save` with a `json.Encoder` configured with `SetEscapeHTML(false)` and `SetIndent("", "  ")`. The retained bytes then pass through compaction unescaped, and freshly built registrations still escape as today, because their `MarshalJSON` escapes internally through `json.Marshal`. Land it together with a preservation fixture whose value contains `&&` in `TestRegistrationPreservationThroughASiblingRewrite` (internal/hooks/registration_test.go:137-172). — FAILS: a hand-written object entry whose command contains `&&` (the shape of section 3.2's own example, `cd "…" && claude --resume …`) comes back from the first sibling rewrite as `&&`, within the hour on this install. That breaks the criterion that preserved values are byte-equivalent, and falsifies the "re-emitted verbatim" claims at internal/hooks/registration.go:17-19 and in CLAUDE.md's `hooks` row. The decoded value is unchanged, which is why this is not blocking.
- [in-scope] [contained] internal/hooks/registration_test.go:160 — both preserved-object fixtures, `{"command":"npm start","nickname":"dev server","resume":"lazy"}` (line 160) and `{"command":"npm start","resume":"LAZY"}` (line 164), already list their keys in alphabetical order. Reorder the line-160 fixture to a non-sorted order, for example `{"resume":"lazy","nickname":"dev server","command":"npm start"}`. — FAILS: a regression that re-marshals a preserved object through `map[string]json.RawMessage` keeps every attribute but sorts the keys, and would still pass every current test. That leaves the "key order inside a preserved object is unchanged" clause of the second criterion unobserved.

UNSETTLED:
- "`doctor`'s stale-hook count, the daemon's sweep and `StaleKeys` behave exactly as before across the existing suites." — needs a run of `go test ./internal/hooks/... ./internal/hooksweep/... ./cmd/...` plus the integration lane's daemon stale-hook suites. Reading shows that `StaleKeys` (store.go:292-307) and `narrowToSnapshot` (store.go:310-318) read keys only and are unchanged, but whether the suites pass can only be settled by running them.
