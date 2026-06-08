TASK: session-tagging-and-grouping-3-1 — prefs.json store — read/write session_list_mode with tolerant decode

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: missing file → Flat; empty file → Flat; corrupt/unparseable JSON → Flat; unrecognised mode value → Flat; valid by-tag/by-project round-trip; AtomicWrite temp+rename.

SPEC CONTEXT: spec §206-211 — prefs.json owns last-used mode; schema {"session_list_mode": "flat"|"by-project"|"by-tag"} string enum; tolerant decode → Flat (treated as first-launch), no hard error; persist via AtomicWrite each toggle.

IMPLEMENTATION: Implemented.
- internal/prefs/store.go — SessionListMode int enum (ModeFlat=iota) with canonical strings; String() out-of-range → flat; parseMode unrecognised → ModeFlat; Load(): ErrNotExist → (Flat,nil), other read error → (Flat,err), unmarshal failure (empty+corrupt) → (Flat,nil); Save(): MarshalIndent + fileutil.AtomicWrite, error verbatim (caller decides non-fatality). Single source of truth, reused by tui/model.go. Leaf package (stdlib+fileutil only, documented no-log rationale to avoid import cycle).

TESTS: Adequate. store_test.go — every edge case (missing/empty/corrupt/unrecognised → Flat; by-tag/by-project round-trip); TestSaveWritesAtomically (nested non-existent dir proves MkdirAll, content assertion, no leftover .atomic- temp); TestModeString pins canonical strings. External prefs_test package (black-box). Real tempdirs.

CODE QUALITY: Conventions followed (NewStore/AtomicWrite/tolerant decode mirror other stores; no t.Parallel; leaf discipline); SOLID good (persistence only, String/parseMode split); low complexity; errors.Is/%w/iota idiomatic. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
