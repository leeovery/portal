TASK: Prefs.json Carries Resume Mode (lazy-resume-on-attach-1-5, tick-923e20)

ACCEPTANCE CRITERIA:
- LoadResumeMode returns Eager for "eager" and Lazy for "lazy".
- It returns Lazy for an absent file, an absent key, an empty value, an unrecognised value ("LAZY", " lazy", "off"), a non-string value, and a wholly corrupt file.
- A prefs.json that exists but cannot be read at all answers Lazy too — the value is the shipped default alongside whatever error is propagated.
- A wrong-typed resume_mode does not zero the rest of the tolerant record: the theme keys and the grouping mode beside it still load.
- A hand-set resume_mode is still on disk, with its value unchanged, after Save(ModeByTag) and after SaveTheme/SaveThemeSlot.
- An install that never set the key has no resume_mode in the file after any of those writes.
- The strict write-path decode is unchanged: a malformed prefs.json still aborts a write rather than merging over it, and the file is left as it was.
- internal/prefs still depends on internal/fileutil and internal/resumemode alone, across both lanes.

STATUS: complete

SPEC CONTEXT: Spec 3.1 puts the install-wide default in prefs.json as `resume_mode` (`eager`|`lazy`). It decodes tolerantly, like the other fields: missing, empty, corrupt or unrecognised gives the shipped default. The key is `omitempty` on write. Per the 2026-09-19 corrigendum, a file that cannot be read at all resolves to that same default by the ordinary route. Spec 2.1 sets that default to lazy, the safe direction. Nothing in Portal writes the key, and no UI exists for it. The field must stay declared, because every prefs writer re-encodes the whole struct. internal/prefs stays a leaf so internal/tui can import it.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/prefs/store.go:70-83: `resumeModeValue` is a tolerant named string type modelled on `migrationMarker`. Its `UnmarshalJSON` takes a JSON string as the value and absorbs anything else, including null, number, bool, array or object, as "" with no error.
  - internal/prefs/store.go:96: `ResumeMode resumeModeValue` with tag `json:"resume_mode,omitempty"` on `prefsFile`. It uses the default string marshal, so an unrecognised string round-trips verbatim and an empty value is omitted.
  - internal/prefs/store.go:209-222: `LoadResumeMode` reads through the tolerant `readFile`. It returns `resumemode.Default` beside a non-ErrNotExist read error, returns `resumemode.Parse`'s answer when the stored string is recognised, and returns `Default` otherwise.
  - internal/prefs/store.go:1-3, 15: the package doc and imports are widened to include internal/resumemode.
  - internal/prefs/leaf_guard_test.go:11-17: `prefsMayImport` gains internal/resumemode, and the guard runs over `sourceguardtest.Lanes()`.
  - README.md:421: the prefs.json row names `resume_mode`, says it holds eager or lazy, and says it is the install-wide default that a hook naming no mode inherits.
  - CLAUDE.md prefs row: `resume_mode` is added to the literal key shape; the row states the tolerant decode (including the unreadable-file case with the error propagated) and that nothing in Portal writes the key. The leaf clause now reads "stdlib + fileutil + internal/resumemode only, no internal/log", and the no-log rationale is unchanged.
- Notes:
  - No writer exists. I found no assignment to `f.ResumeMode` anywhere in the tree.
  - The strict path (`readFileStrict`, store.go:163-184) is untouched. The field's own decoder never raises an `UnmarshalTypeError`, so the Field carve-out is never reached for it. A syntax error still aborts the write.
  - The only production reader is cmd/state_hydrate.go:222, which belongs to a later task.
  - I checked the leaf criterion by reading. The production files of internal/fileutil and internal/resumemode import only the standard library. None of the three packages has a build-constrained file, so both lanes resolve the same set.

TESTS:
- Status: Adequate
- Coverage (internal/prefs/resume_mode_test.go):
  - Lines 15-39: eager and lazy are read, including beside other keys.
  - Lines 42-71: a table covering a missing key, an empty object, an empty value, "LAZY", " lazy", "off", a number, a boolean, null, an array and an object.
  - Lines 73-97: an absent file and an absent parent directory, plus the shared corrupt-file cases (syntax errors and top-level type mismatches).
  - Lines 99-110: an unreadable file. It is seeded with "eager" before `themetest.DenyRead`, so a leaked value would fail the test. The test asserts that `os.ErrPermission` propagates and that `Default` is returned beside it.
  - Lines 113-134: wrong-typed neighbour survival. This test discriminates: with a plain string field, `readFile` would zero the record and `Load` would answer ModeFlat.
  - Lines 137-153: round-trip through `Save` for eager, lazy and "off". The "off" case discriminates: it fails if the value were normalised through `Parse`.
  - Lines 155-166: the documented wrong-typed drop-on-write asymmetry.
  - Lines 168-182: round-trip through `SaveTheme` and `SaveThemeSlot` (light and dark).
  - Lines 184-196: an omitted key after Save, SaveTheme, SaveThemeSlot and SaveMigrationMarker.
  - Lines 200-226: a malformed file still aborts the write with a `*json.SyntaxError`, and the file stays byte-identical.
  - leaf_guard_test.go runs the leaf check over both lanes.
- Notes:
  - Several tests would fail if the feature broke: the wrong-typed-neighbour test, the verbatim "off" round-trip, the deny-read test seeded with "eager", and the omit test.
  - The wrong-typed cases in the fallback table would also pass under a plain string field, because `readFile` returns a zero record. That gap is covered by the discriminating record-survival test, so it is not a gap in coverage.
  - The tests reuse the package's shared case lists (`undecodablePrefsCases`, `absentPathCases`, `themeSaverCases`, `markerWriterCases`) rather than restating them. No redundancy worth reporting.

CODE QUALITY:
- Project conventions: Followed. The tolerant field type mirrors `migrationMarker`, and the accessor mirrors `Load`'s policy. No `internal/log` import. Comments carry no task, phase or spec references.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`errors.AsType` in the test, consistent with the existing suite on Go 1.26)
- Readability: Good. The comments on `resumeModeValue`, the `ResumeMode` field and `LoadResumeMode` hold true against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
