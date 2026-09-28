TASK: Hook Set Writes the Registration Whole in the Shape Its Content Chooses (lazy-resume-on-attach-1-3, tick-6d9c49)

ACCEPTANCE CRITERIA:
- Setting a registration carrying a mode writes the object form holding `command` and `resume`; setting one carrying no mode writes the plain string form.
- Rewriting an entry with a registration identical in command and mode is a `set-noop`: no write, the file byte-unchanged, one DEBUG record, no INFO.
- Rewriting an entry with the same command and a different mode — in either direction, including to no mode — is a `modify` that writes.
- An object-form predecessor rewritten with a registration carrying no mode comes back as a plain string and keeps none of its unmodelled attributes.
- A rewrite still leaves every other entry in the file exactly as it found it.
- The `set` / `modify` / `set-noop` breadcrumbs carry the same message, `op`, `hook_key`, `via` and `value` (the command) as before, out of either stored shape, and a failed save still carries `error` and `error_class` and no `value`-less shape change.
- Both lanes build: `go test ./...` and `go test -tags integration -p 1 ./...` each compile the whole tree, so no integration-tagged suite is left calling the old signature.

STATUS: issues_found

SPEC CONTEXT: Section 2.2 says a registration is written whole: nothing from the replaced entry survives a re-registration it was not given, and a pin lives on the registration rather than on the pane. Section 3.2 defines the two permanent shapes. The string form holds a command only. The object form holds `command` plus `resume`. The writer picks the object form only when there is something to carry beside the command, so the external SessionStart hook does not turn every entry into the verbose shape. A rewrite leaves every entry it did not name byte-for-byte as it found it, including attributes the reader does not model and `resume` values it cannot recognise. Section 3.3 adds `--resume-mode` on `hook set`. That flag belongs to a later task; this task only needs the store signature that carries the mode.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/hooks/store.go:134-172. `Set(key string, event Event, registration Registration, via Via) error`.
  - internal/hooks/store.go:174-187. `classifySet` compares against the handed `Registration` through `sameRegistration` (internal/hooks/registration.go:68-70), which checks command and mode.
  - internal/hooks/store.go:162. The value is stored re-projected as `Registration{Command, Resume}`.
  - internal/hooks/registration.go:53-64. `MarshalJSON` picks the shape: string when no mode, object `{command, resume}` when a mode is set, and the verbatim raw bytes for an entry that was decoded and never replaced.
  - Breadcrumbs, internal/hooks/store.go:141, 153, 165-166 and 170: same ops (`set` / `modify` / `set-noop`), `hook_key`, `via`, and `value` = the command. The failed-save WARN still carries `value`, `error` and `error_class`.
  - Non-test call sites: cmd/hooks.go:227 (now `hooks.Registration{Command: command, Resume: mode}`, after the later flag task) and internal/hookstest/hooks.go:79 (`SeedHooksJSON`).
- Notes:
  - One deliberate divergence, and it is sound. The plan said to store the handed value directly into `h[key][event.String()]`. The code instead re-projects it (store.go:162, with the comment at :160-161) so that any retained raw bytes are dropped. Without this, a caller that loaded a registration, changed its `Resume` and handed it back would get its original bytes re-emitted by `MarshalJSON`, and the change would be silently lost. `TestSetReProjectsTheRegistrationItStores` (internal/hooks/store_set_test.go:207-336) pins this. It is an improvement on the plan's wording, not a loss.
  - The no-op rule ("unchanged on both command and mode") also covers one case worth recording. An object-form predecessor that carries no recognised mode — for example `{"command":"x","nickname":…}` or `"resume":"LAZY"` — rewritten with the same command and no mode is a `set-noop`, so its inert extra bytes survive. This matches the plan's edge case ("an identical object-form rewrite is still a no-op … unmodelled attributes survive"). Those bytes carry no mode and change no behaviour, so it is not a finding.
  - An empty command is not refused. It is written as `""` or as `{"command":"","resume":…}`, which matches the edge case.
  - Comments in the changed code match what it does: store.go:129-133, :160-161 and :137-140, and registration.go:10-24, :50-52 and :66-67.
  - Spot-checked integration-lane callers are on the new signature: internal/restore/exit_closes_pane_integration_test.go:139, internal/restore/reboot_fixture_test.go:73 and cmd/bootstrap/phase2_hook_fire_integration_test.go:47. So are these unit-lane callers: internal/hooks/event_test.go:23, internal/hooks/lock_write_test.go:23-196, internal/hooksweep/snapshot_order_test.go:22 and :80, and internal/hooks/store_test.go.

TESTS:
- Status: Adequate
- Coverage: Every test named in the plan exists in `TestSetWritesTheRegistrationWhole` (internal/hooks/store_set_test.go:17-205). The fixtures are staged through `hookstest.StageStore` with raw `Seed`s, and breadcrumbs are asserted through `logtest.Install` plus `logtest.AssertRecord`.
  - Object form written for a mode (:18-30).
  - String form written for no mode (:32-43).
  - Identical object-form rewrite is a no-op. Only one DEBUG record exists, the file bytes are unchanged, and the mtime is unchanged (:45-73).
  - A mode-only change is a `modify`, tested none→lazy and eager→none (:75-121).
  - An object-form predecessor with a mode and an unmodelled attribute comes back as a plain string (:123-137).
  - A sibling entry carrying an unmodelled attribute is left byte-identical, compared in compacted form (:139-154).
  - The `value` attr is the command out of an object-form write (:156-179).
  - A failed save carries error class and writes no file (:181-204).
  - `TestSetReProjectsTheRegistrationItStores` adds the loaded-then-adjusted route, covering both the `modify` breadcrumb out of an object-form predecessor (value and hook_key) and a straight hand-back as a no-op.
  - The pre-existing `TestSetLogging` (internal/hooks/store_test.go:1201-1321) still pins `set`, `modify` and `set-noop` for the string form, including that `set-noop` carries no `value`.
- Notes: One acceptance-criterion clause is not observed by a test: the failed-save WARN keeping its `value` attr. See FINDINGS. There is small overlap between `TestRegistrationRewriteClassification` (internal/hooks/registration_test.go:174-198) and the eager→none row here. It is cheap and not worth acting on.

CODE QUALITY:
- Project conventions: Followed. Tests use `t.Run` with "it …" names and no `t.Parallel`. Store fixtures go through `hookstest.StageStore`, logs through `logtest`, and seed keys come from the `hookstest` vocabulary.
- SOLID principles: Good. Classification lives in `classifySet`/`sameRegistration`, and the choice of shape is owned by `Registration.MarshalJSON` rather than by `Set`.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None beyond the test gap below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/hooks/store_set_test.go:199 — The test this task wrote for the failed save ("it reports a failed save with its error class and writes nothing", :181-204) asserts `error` and `error_class` through `logtest.AssertWriteFailure`. It does not assert the `value` attr, or `hook_key`, that the failed-save WARN carries at internal/hooks/store.go:165-166. The same gap exists in the older `TestSetLogging` WARN case at internal/hooks/store_test.go:1299-1320. Fix: add `rec.AttrString(t, "value") == "npm start"` and `rec.AttrString(t, "hook_key") == hookstest.LiveSeedA` to the record at :199. — FAILS: if `value` is dropped from the failed-save WARN at store.go:165, both of these tests still pass. The criterion's clause that a failed save keeps its shape, with no `value`-less change, would then go unnoticed, and a failed registration's log line would stop naming the command that failed to land.

UNSETTLED:
- "Both lanes build: `go test ./...` and `go test -tags integration -p 1 ./...` each compile the whole tree, so no integration-tagged suite is left calling the old signature." — Settling this needs both lanes compiled, for example `go vet ./...` and `go vet -tags integration ./...`, or the two test commands. Reading confirmed the new signature at the spot-checked call sites listed under IMPLEMENTATION, but it cannot prove that every caller across both lanes compiles.
