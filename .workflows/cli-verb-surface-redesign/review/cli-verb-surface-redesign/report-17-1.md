TASK: Name the host-terminal resolution seam type in internal/spawn (cli-verb-surface-redesign-17-1)

ACCEPTANCE CRITERIA:
- type AdapterResolver func(Identity) (Adapter, Resolution) declared exactly once in internal/spawn with a doc comment.
- No inline func(spawn.Identity) (spawn.Adapter, spawn.Resolution) spelling remains in cmd/ or internal/tui/ production code; all 8 sites reference spawn.AdapterResolver.
- Wiring and behaviour unchanged; seam signature byte-identical.
- go build ./... succeeds, go test ./... passes, golangci-lint run clean.

STATUS: Complete

SPEC CONTEXT: internal/spawn is the shared multi-window spawn service. Adapter resolution (config terminals.json -> native Ghostty -> unsupported) maps a detected host-terminal Identity to an Adapter plus a Resolution classification. This resolver seam is shared by three callers (picker burst, multi-target open burst, doctor host-terminal check). The spawn package already names its single-return seam ExecutableResolver; this task extends that convention to the resolution seam. Pure naming extraction, no behaviour change.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/resolver.go:29 (declaration). All 8 production sites confirmed referencing spawn.AdapterResolver:
  - cmd/open.go:662 (openDeps.resolve field)
  - cmd/open_burst_run.go:33 (Resolve field)
  - cmd/doctor.go:129 (doctorDeps.Resolve field)
  - cmd/doctor.go:404 (checkHostTerminal parameter)
  - cmd/spawn_seams.go:36 (productionSpawnSeams.Resolve field)
  - internal/tui/build.go:54 (Resolve field)
  - internal/tui/spawn_detect.go:42 (WithResolve option parameter)
  - internal/tui/model.go:465 (model.resolve field)
- Notes:
  - Declared exactly once (grep "type AdapterResolver" returns only internal/spawn/resolver.go:29). Carries a substantial doc comment (resolver.go:21-28) explaining the shared seam, the production impl (Resolver.Resolve / NewResolver), and the ExecutableResolver precedent it mirrors.
  - Signature byte-identical: declared as func(Identity) (Adapter, Resolution) — identical modulo package qualifier to the prior inline func(spawn.Identity) (spawn.Adapter, spawn.Resolution).
  - No inline signature remains in ANY production (non-test) .go file across internal/spawn, cmd/, internal/tui/ (grep excluding _test.go returns nothing).
  - Go func types are structurally identical, so a field typed spawn.AdapterResolver remains assignable from the inline-spelled func literals still present in test files — no compile break introduced by the mixed spelling.

TESTS:
- Status: Adequate (no new tests required — pure refactor)
- Coverage: Existing spawn / open-burst / doctor / tui suites exercise every touched seam via the injected Resolve/resolve fields; the type substitution is compiler-verified at all 8 sites. Task explicitly requires no behavioural tests.
- Notes: Judged by reading only (no execution). Test files that inject the seam still use the inline func spelling at 8 sites (cmd/open_burst_run_test.go:143, cmd/open_burst_seams_test.go:34, cmd/doctor_test.go:741 & 827, internal/tui/burst_cached_adapter_test.go:37 & 154, internal/tui/burst_unsupported_noop_test.go:31, internal/tui/burst_dispatch_test.go:37). These compile fine (structural identity) and are outside the acceptance scope ("production code"). internal/tui/spawn_detect_test.go already adopted spawn.AdapterResolver (lines 44/58/82/97), so test-side adoption is partial/inconsistent — cosmetic only.

CODE QUALITY:
- Project conventions: Followed. Named func type with a doc comment starting with the type name (golang-naming / golang-code-style); mirrors the existing ExecutableResolver precedent (spawn already owns this convention). Type is exported and package-local to the domain that owns Identity/Adapter/Resolution.
- SOLID principles: Good. Single point of definition for the seam contract; DRY win (was respelled at 8 sites).
- Complexity: Low. Pure declaration + reference substitution.
- Modern idioms: Yes. Idiomatic Go named function type.
- Readability: Good. Doc comment is clear and correctly cites the shared callers and the production implementation.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_burst_run_test.go:143, cmd/open_burst_seams_test.go:34, cmd/doctor_test.go:741, cmd/doctor_test.go:827, internal/tui/burst_cached_adapter_test.go:37, internal/tui/burst_cached_adapter_test.go:154, internal/tui/burst_unsupported_noop_test.go:31, internal/tui/burst_dispatch_test.go:37 — replace the remaining inline func(spawn.Identity) (spawn.Adapter, spawn.Resolution) spellings with spawn.AdapterResolver for full consistency (internal/tui/spawn_detect_test.go already adopted it). Outside the acceptance scope but completes the naming sweep; mechanical, no behaviour change.
