TASK: Instrument hooks.Store Set/Remove (set/modify/rm + set-noop DEBUG, value/via/error_class) (portal-observability-layer-3-2)

ACCEPTANCE CRITERIA:
- Set new key → one INFO hooks: set hook_key=<k> value=<cmd> via=<via>, persists.
- Set existing key DIFFERENT value → one INFO op=modify with value.
- Set existing key SAME value → one DEBUG hooks: set-noop, no Save (mtime unchanged).
- Remove → one INFO hooks: rm hook_key=<k> via=<via> no value; absent-key Remove still INFO.
- AtomicWrite failure → one WARN with error (wrapped chain) + error_class from ClassifyWriteError.
- via=cli for portal hooks set/rm; closed cli/internal/migrate; no ad-hoc value.
- No logging in AtomicWrite or Save; emission only in Set/Remove.

STATUS: Complete

SPEC CONTEXT:
Spec § State-mutation audit trail (658-727). hooks.json component hooks; seam is store methods. One INFO success / one WARN failure after AtomicWrite. Required op/hook_key + error_class on WARN; optional value/via. set-noop → DEBUG. Verbatim privacy.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/hooks/store.go:29 (logger); Set 123-150 + classifySet 155-168; Remove 179-200. Callers cmd/hooks.go:118 (Set "cli"), :164 (Remove "cli").
- Notes: Set classifies from pre-write Load (absent→set, differs→modify, matches→set-noop DEBUG return without Save). op carried as both message AND op attr (documented; proven by TestSetEmitsOpAsJSONField). Remove always Saves → always INFO rm no value; absent-key documented. WARN passes wrapped err directly + ClassifyWriteError(err). No logging in Save/AtomicWrite. via free string documented (not enforced). migrate-rename uses separate SaveAudited (3-3).

TESTS:
- Status: Adequate
- Location: internal/hooks/store_test.go
- Coverage: TestSetLogging (set+value+via; modify; set-noop DEBUG + mtime-unchanged; WARN error_class=write-failed-temp-create + errors.Is on logged value; "does not log inside Save"); TestRemoveLogging (rm no value; absent-key still INFO; WARN); TestSetEmitsOpAsJSONField. Pre-existing TestSet/TestRemove updated for via arg.
- Notes: Behaviour (level/msg/attrs/mtime/errors.Is). WARN uses real 0500 dir → real ClassifyWriteError path. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (closed op vocab, op verb message, no Sprintf, log.For("hooks"), no t.Parallel).
- SOLID: Good — classifySet pure helper; Save/AtomicWrite audit-unaware.
- Complexity: Low.
- Modern idioms: Yes (%w, errors.Is sentinels, error value passed directly).
- Readability: Good — doc explains seam, op-as-attr, absent-key-still-Saves.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] via is unconstrained string documented as closed cli/internal/migrate but not enforced (matches task instruction); a named type with exported constants would prevent silent drift across the three stores. Cross-store cleanup, out of scope.
