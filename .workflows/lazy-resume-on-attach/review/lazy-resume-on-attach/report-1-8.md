TASK: lazy-resume-on-attach-1-8 (tick-7b5d9b) — Set Re-Projects the Registration It Stores

ACCEPTANCE CRITERIA:
- A registration loaded from hooks.json, adjusted and handed back to Set is written from its command and mode, never from the bytes it was decoded from: a string-form entry pinned lazy comes back as {"command":…,"resume":"lazy"}, and an object-form entry seeded eager and adjusted to lazy carries lazy on disk.
- The write the breadcrumb reports is the write that lands — after a mode-only read-modify-write the INFO modify record and the file agree, with the op, hook_key, via and value vocabulary unchanged.
- A rewrite whose value came out of the loaded snapshot drops the replaced entry's unmodelled attributes, exactly as a freshly-constructed one does.
- The preservation property is untouched: every entry the write did not name comes back byte-identical (unmodelled attribute, unrecognised resume value, neither-string-nor-object value), whether the written value was constructed fresh or loaded from the same snapshot.
- Handing a loaded registration straight back unchanged is still a set-noop: no write, file byte-unchanged and mtime unchanged, one DEBUG record and no INFO.
- Every existing caller is unchanged in behaviour (cmd/hooks.go hooksSetCmd, internal/hookstest SeedHooksJSON), and the existing internal/hooks suites pass untouched.
- The raw field comment no longer asserts a property of callers the store does not check.
- go test ./... passes and go vet -tags integration ./... is clean; signature unchanged.

STATUS: complete

SPEC CONTEXT: The spec's hooks.json section states that a rewrite of one registration leaves every other entry exactly as it found it (unmodelled attributes and unrecognised resume values alike), and that only the entry being written is written whole: "what that call is handed is all that entry keeps". The task closes a latent trap where a registration loaded from the snapshot (carrying the unexported raw bytes) and handed back to Set would be marshalled from those raw bytes rather than from its adjusted Command/Resume, while classifySet reports a modify.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/hooks/store.go:160-162 — Set stores Registration{Command: registration.Command, Resume: registration.Resume}, with a two-line warning comment that storing the handed value reinstates the decoded bytes.
  - internal/hooks/store.go:146-171 — lock, load, classifySet, save and every breadcrumb are unchanged; the INFO/WARN records still carry op, hook_key, value (registration.Command) and via.
  - internal/hooks/registration.go:17-23 — the raw field comment now states the enforced property (a value a mutation stores is re-projected from its command and mode first) in place of the claim about callers. MarshalJSON's raw-first branch and its doc comment (registration.go:50-56) are untouched.
- Notes: Set is the only mutation that stores a Registration: the only Registration composite literals in non-test internal/hooks code are registration.go:32 (decode) and store.go:162, and Remove/Discard/CleanStale only delete. So the raw comment's "a value a mutation stores" holds. A freshly constructed value carries no raw, so re-projecting it yields an identical value. The two production callers (cmd/hooks.go:227 and internal/hookstest/hooks.go:79) both construct fresh, so their behaviour is unchanged. Siblings keep their raw bytes because Set reloads the snapshot under its own hold and only replaces the named entry.

TESTS:
- Status: Adequate
- Coverage: TestSetReProjectsTheRegistrationItStores (internal/hooks/store_set_test.go:207-336) drives every case through loadedRegistration (store_set_test.go:341-352), which reads via store.Load(hooks.ViaInternal). That is the path that brings a raw-carrying value into Set.
  - Table (208-259): a string-form seed and an object-form eager seed are each adjusted to lazy. Each case asserts the compacted stored bytes are {"command":"npm start","resume":"lazy"}, checks the single INFO modify record with RecordWant (level, msg, component, op, via), and checks hook_key and value. Without the re-projection both cases would fail, because the stored bytes would be the seeded raw.
  - The unmodelled-attribute drop (261-277) seeds "nickname", loads the entry, adjusts it and asserts the two-attribute object. Without the re-projection this fails too.
  - Preservation under a loaded-value rewrite (279-304) covers three siblings: an unmodelled attribute, an unrecognised resume "whenever", and the value 42. Each is compared with bytes.Equal on storedValue.
  - The no-op case (306-335) seeds an object with nickname and hands the loaded registration straight back. It asserts exactly one DEBUG set-noop record (Only), a byte-unchanged file (which also shows the nickname is still on disk) and an unchanged mtime.
  - Fresh-construct preservation of all three sibling shapes already exists in internal/hooks/registration_test.go:159-169 and store_set_test.go:139-154, so AC4's "constructed fresh" half is covered by existing tests.
- Notes: The sink is installed after loadedRegistration, so the Load read's breadcrumbs cannot pollute the Only assertions. The commit adds tests only and removes no line of an existing test. The tests are focused and match the four the plan names, with no redundant setup.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel. Staging goes through hookstest.StageStore, and log assertions use logtest.Install, AssertRecord and Records().Only, per the logtest/hookstest conventions. Comments carry no task ids or spec references.
- SOLID principles: Good. The store now enforces its own documented whole-registration contract instead of relying on callers to honour it.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The warning comment at store.go:160-161 names the failure mode that collapsing the literal back would reinstate.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes and `go vet -tags integration ./...` is clean: the signature is unchanged, so no integration-tagged suite needs an edit." — Run `go test ./...` and `go vet -tags integration ./...`. By reading, the Set signature is unchanged and the existing internal/hooks suites are behaviourally unaffected, since a fresh value re-projects to itself. Whether they pass has to be observed by running them.
