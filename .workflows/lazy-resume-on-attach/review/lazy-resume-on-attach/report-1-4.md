TASK: lazy-resume-on-attach-1-4 — The Store's Reads Report the Registration's Mode

ACCEPTANCE CRITERIA:
- A hit on a string-form entry reports the command, `Found` true and no mode; a hit on an object-form entry carrying `resume` reports the command, `Found` true and that mode.
- An object whose `resume` is absent, empty, unrecognised or not a string reports the command with no mode — the entry inherits, and the lookup says nothing about the install default.
- An object with no `command` key, an object whose `command` is empty, and a value that is neither string nor object each report the zero result with a nil error.
- An empty `hookKey`, an absent key and an absent `on-resume` event each report the zero result with a nil error, and the empty key still reads no file.
- A genuine read failure returns the zero result alongside the error, and the hydrate helper still execs a bare shell on it; a malformed `hooks.json` still degrades to a miss after one DEBUG record rather than erroring.
- `List` carries each entry's mode beside its command, in the existing key-then-event order, and a string-form entry carries none.

STATUS: complete

SPEC CONTEXT: Section 3.2 makes an event's stored value either a command string or an object carrying `command` plus `resume`. A value the reader cannot make sense of never fails a pane: an absent, empty or unrecognised `resume` carries no mode and inherits. An object with no command, or an empty one, is not a registration, so the pane falls through to a plain shell (section 6.1). Section 3.4 reads the stored mode back through `hook list`'s fifth column, which reports what is stored and never the install default. Section 6.1 keeps the existing degradation: an unreadable store gives a bare shell. The plan's structural call is a small result struct rather than a fourth return, because the hydrate helper reads the command and the mode together.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/hooks/lookup.go:13-17 — `OnResume{Command, Mode, Found}`.
  - internal/hooks/lookup.go:29-42 — `LookupOnResume(hookKey string, via Via) (OnResume, error)`.
    - Refuses an empty key before `loadShared` (line 30).
    - Returns the zero value with a wrapped error on a read failure (line 35).
    - Returns the zero value for an absent key or event, and for `registration.Command == ""` (lines 37-39).
    - On a hit, fills `Command` and `Mode` from the stored `Registration` (line 41). Nothing is resolved against the install default.
  - internal/hooks/store.go:24-29 — `Hook` gains `Resume resumemode.Mode`.
  - internal/hooks/store.go:255-281 — `List` fills it at line 268. The key-then-event sort at lines 273-278 is unchanged.
  - internal/hooks/registration.go:31-48 — the decode every shape goes through:
    - A string decodes to `Command`.
    - An object reads `command` and `resume` only when each is a string (`stringAttribute`, lines 74-80). The strict `resumemode.Parse` (internal/resumemode/resumemode.go:51-60) is what drops an unrecognised value to `Unset`.
    - Anything else decodes to an empty registration. JSON `null` takes the string branch with no error and leaves `Command` empty.
    - So every command-less shape reaches the lookup's one miss rule.
  - Call sites:
    - cmd/state_hydrate.go:228-235 (`lookupOnResumeOrLog`) and cmd/state_resume_chain.go:152-165 (`resumeRegistrationOrLog`) read `.Found`/`.Command`. They keep the DEBUG `hook lookup` records (result=error/miss/hit) and the WARN `lookup on-resume hook failed`. A later task moved this code, but the behaviour is intact.
    - cmd/bootstrap/transient_listpanes_helpers_integration_test.go:121-130 reads `got.Found` and `got.Command`.
- Notes: The implementation matches every criterion, with no drift. The lookup reports the stored mode only, and resolution happens at the caller (cmd/state_hydrate.go:198). A whitespace-only key is still used verbatim (lookup.go:27-28).

TESTS:
- Status: Adequate
- Coverage:
  - internal/hooks/lookup_test.go:
    - String form → command, no mode: lines 74-79.
    - Object form → command and mode, eager and lazy subtests: lines 81-92.
    - No-mode table (absent, empty, unrecognised word, capitalised, not a string, null): lines 94-114.
    - Object with no command key: lines 116-121.
    - Object with an empty command: lines 123-128.
    - Value that is neither string nor object (7, true, null, an array): lines 130-139.
    - Zero result alongside a read error: lines 171-186.
    - Empty key reads no file (proved against an unreadable store): lines 205-212.
    - Absent key, absent event, missing and malformed file: lines 39-65.
    - Whitespace-only key taken literally: lines 155-169.
  - internal/hooks/store_test.go:555-574 checks each entry's mode through `List` in key-then-event order with `reflect.DeepEqual`, including a string-form entry carrying `Unset`.
  - internal/hooks/read_lock_test.go:304-342 and internal/hooks/event_test.go:35-41 are re-pinned to whole-struct comparisons.
  - The `cmd` bare-shell degradation on an unreadable store is covered by cmd/state_hydrate_test.go:1553-1595 (replay tail) and :1517-1551 (timeout tail). Both assert `[$SHELL]` and the WARN.
- Notes:
  - Assertions compare the whole `OnResume` value (`assertNoHook`/`assertHook`), so a dropped mode, a stray `Found`, or a hit returned for a command-less entry would each fail.
  - The lookup suite checks the malformed-file miss but not the one DEBUG `load-malformed` record. That record comes from `loadDegrading` (store.go:87-94), which is shared by every read door and was not changed by this task.
  - Nothing is over-tested: each table row covers a distinct stored shape the spec names.

CODE QUALITY:
- Project conventions: Followed. Suites use hookstest staging, and the `internal/hooks` → `internal/resumemode` edge is the leaf vocabulary the plan set up.
- SOLID principles: Good. The lookup reports and does not resolve, and there is one miss rule for every stored shape.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The doc comments on `OnResume` and `LookupOnResume` match the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
