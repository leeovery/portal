TASK: resume-hooks-silently-lost-4-1 — The Store Answers Whether It Removed Anything

ACCEPTANCE CRITERIA:
- `Remove` returns `(true, nil)` when it deleted the named event and the save succeeded
- `Remove` returns `(false, nil)` when the key is absent, when the key is present but the named event is not, and when `hooks.json` does not exist
- No `Save` occurs on the no-removal path: an absent `hooks.json` stays absent, a malformed file keeps its bytes, and a key mapped to an empty event map is left in place
- A no-removal call emits no log record of any kind
- A key holding several events loses only the named one and keeps its outer key (reports `true`); a key holding only that event has its outer key dropped (reports `true`)
- A failed `Save` returns `(false, err)` alongside the existing WARN carrying `op=rm`, `hook_key`, `via`, `error`, `error_class`
- The answer is computed from the map `Remove` loaded and mutated: no `Get`, no second `Load`, no pre-read
- A genuine removal still emits exactly one INFO with `op=rm`, `hook_key`, `via`, carrying no `value` attr
- `portal hook rm` behaves exactly as it does today at the CLI (the boolean is discarded by this task; 4-2 consumes it)
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§4.2 fixes the rule that `hook rm` exits 0 **iff** it removed an entry, and states explicitly that "whether anything was removed is reported by the locked removal itself, not by a read taken before it: the store's removal answers from the file it mutated, under the exclusive hold of §6.3" — a pre-read would decide the answer from a snapshot the mutation never saw. §4.3 extends the same exit rule to the `--pane-key` pass-through, which is only possible because the store reports the answer without any reading of its own. §6.5 closes the `hooks` `op` vocabulary amendment at three values (`load-unlocked`, `touch-save-requested`, `clean-stale-skipped`), so no `rm-noop` verb is available — silence is the required record for a call that changed nothing, and the log-forensics motivation is that 61 of 63 degenerate `portal.log` lines were `op=rm` naming removals that never happened. §9.2 pins "removing nothing is non-zero every way" as a verification item.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/store.go:168-209` — `Remove(key string, event Event, via Via) (bool, error)`. After the exclusive acquire and `s.load()`, it looks the key up once in the loaded map (`:188`), returns `(false, nil)` for an absent key (`:190`) and for a present key whose named event is absent (`:193`) — both **before** any `save` or `logger` call. Otherwise it deletes the event, drops the outer key when its last event goes (`:196-199`), and falls through to the unchanged save/WARN/INFO tail (`:201-208`), returning `(false, err)` on a failed save and `(true, nil)` on success.
  - `internal/hooks/store.go:168-172` — the doc comment was rewritten; the old "It rewrites the file even when the key or event is absent" claim is gone, replaced by an accurate statement of the three no-removal cases, the no-write/no-breadcrumb consequence, and the "answer comes from the map this call loaded and mutated" provenance.
  - `cmd/hooks.go:292` — the one production caller now takes two values. It has since been advanced by task 4-2 to act on the boolean (`cmd/hooks.go:295-297`, returning `no resume hook registered for <key>`), which is that task's contract rather than a drift in this one.
- Notes:
  - The answer is derived solely from the loaded map — verified by reading the whole method: there is no `Get`, no second `Load`, and no `os.Stat`/`ReadFile` pre-check anywhere in it. This matters because the load-mutate-save region now sits inside one exclusive hold (`acquireMutationLock`, `internal/hooks/lock.go:81-84`) and `flock` is not re-entrant from the same process.
  - The no-removal path leaves `hooks.json` untouched: `save` is unreachable before the two early returns, so an absent file stays absent and a tolerantly-decoded malformed file is not flattened to `{}`. The only artefacts a no-removal call can still create are the config directory and the `hooks.json.lock` sidecar (`internal/hooks/lock.go:82-83`), which is the documented mutation-acquire contract in CLAUDE.md ("created only by a mutation acquire — a `hook set`, a `hook rm`, or a clean's deletion phase") and not `hooks.json` itself.
  - A key decoded as `"k": null` gives a nil `events` map; the lookup at `:192` returns `!ok` on a nil map rather than panicking, so a hand-edited null entry takes the silent no-removal path.
  - The lock-acquire WARN at `:178` is not a violation of the "no record at all" criterion: it is a failed operation (the store could not look), explicitly distinguished from the silent no-removal by the comment at `:176-177`, and it is outside the three cases the doc comment enumerates. That path is later-phase lock work, not this task's.

TESTS:
- Status: Adequate
- Coverage (`internal/hooks/store_test.go`):
  - `TestRemove` (`:258-494`) covers every state criterion, one subtest per plan-named test: removal reported (`:259`), outer key dropped (`:295`), sibling events retained (`:325`), absent key (`:362`), present key / absent event (`:395`), absent `hooks.json` stays absent (`:431`, asserted with `os.Stat` + `errors.Is(err, os.ErrNotExist)`), malformed file byte-identical (`:449`), key mapped to `{}` left in place (`:464`), failed save reports no removal (`:479`).
  - The two no-op subtests assert **both** byte-identity (`hookstest.AssertHooksFileUnchanged`, `internal/hookstest/hooks.go:113`) **and** an unchanged mtime (`modTime`, `store_test.go:37`) — the mtime check is what makes them fail if a `Save` were reintroduced that happened to produce byte-identical content, so the tests actually observe the "no Save" property rather than only its usual visible effect.
  - `TestRemoveLogging` (`:1280-1388`): the real-removal subtest asserts exactly one record via `sink.Records().Only(t, …)` with `op=rm`/`hook_key`/`via` and the **absence** of a `value` attr; the inverted subtest (`:1314`, renamed to "it emits no record at all when it removes nothing") is a three-case table — absent key, absent event, absent file — asserting `len(sink.Records()) == 0`; the write-failure subtest (`:1369`) asserts `removed == false`, `errors.Is(err, fileutil.ErrWriteTempCreate)`, the WARN shape and `error_class=write-failed-temp-create`.
  - The write-failure fixture is seeded, as the task required: `hookstest.StageStore(Staging{Seed: …, WritesDenied: true})` writes a matching entry, creates the sidecar, and only then chmods the directory to `0o500` (`internal/hookstest/staging.go:82-95`), so the call has a real write left to fail and the classification stays exercised. (The plan asked for a seeded `readOnlyDirPath` variant; the equivalent capability landed as the shared `Staging` description — a legitimate consolidation rather than a gap.)
  - Every other `Remove` call site in the tree was re-pointed to the two-value signature: `internal/hooks/lock_test.go:224`, `internal/hooks/lock_write_test.go:58,80,129,149`, `internal/hooks/cleanstale_snapshot_test.go:149`, `internal/hooks/event_test.go:43`, `internal/hooksweep/snapshot_order_test.go:77`.
- Notes: `TestRemove`'s "it reports no removal when the save fails" (`:479`) is a strict subset of `TestRemoveLogging`'s write-failure subtest (`:1369`) — same staging, same `removed`/`errors.Is` assertions, with the logging one adding the record checks. Both were explicitly requested by the plan's Tests list and they sit in the state-vs-logging split the file already uses, so the overlap is deliberate rather than bloat. No other redundancy; nothing in the suite would still pass if the reporting or the skip-the-save behaviour broke.

CODE QUALITY:
- Project conventions: Followed. `logger` is the package-level `log.For("hooks")` binding; the emissions stay inside the closed `hooks` `op` vocabulary (`rm` only — no `rm-noop` was invented, per §6.5); the mutation runs under the single exclusive hold with no nested acquire; all new tests are unit-lane and touch no tmux server or real state directory.
- SOLID principles: Good. The store keeps sole responsibility for persistence and now reports the one fact only it can know; the CLI's exit rule is layered above it rather than duplicated inside it.
- Complexity: Low. Two guard clauses replace an unconditional write; no branching was added to the success path.
- Modern idioms: Yes. Comma-ok map lookups, `delete` on the mutated map, `(bool, error)` return in the conventional order.
- Readability: Good. The doc comment states the contract including the provenance of the answer, and the inline comment at `:176-177` names why the acquire failure is loud where the no-removal is silent.
- Issues: None found.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
