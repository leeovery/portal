TASK: Every Record the Phase Reshaped Reaches Its Siblings (tick-3e3e5a, lazy-resume-on-attach-16-3). Covers two things: the hooks store's refused-load handling, shared by Set and by removeEntry (Remove and Discard), and the hydrate helper's `disable terminal signals failed` WARN naming the parked pane by its durable token.

ACCEPTANCE CRITERIA:
- `Set` against a `hooks.json` it cannot read (permission denied) returns an error that `errors.Is` `ErrStoreRead`, leaves the file as it was, and leaves exactly one record at WARN or above: `hooks: set` with `op=set`, the key as `hook_key`, the caller's `via`, and the whole returned chain as `error` (naming the file and the OS's cause). The record carries no `error_class` and no `value`.
- `Set` against a malformed `hooks.json` returns an error that `errors.Is` both `ErrMalformed` and `ErrStoreRead`, leaves the file byte-identical, and leaves exactly one WARN: `hooks: set` with `op=set`.
- `Remove` and `Discard` refused by the same unreadable or malformed `hooks.json` answer as they do today: one WARN under their own op (`rm`, `discard`) carrying `hook_key`, `via` and the whole chain, plus an error that `errors.Is` `ErrStoreRead`.
- A `Set`, `Remove` or `Discard` that cannot take the sidecar lock leaves the same WARN it leaves today (the `hookstest.AssertLockWarn` pins for set, rm and discard stay green).
- A `Set` whose load succeeds records exactly as it does today: `set`, `modify` and `set-noop`, and the save-failure WARN with its `value` and `error_class`.
- `portal hook set` against a malformed `hooks.json` still exits non-zero, with the malformed refusal reaching stderr and the file byte-identical (`cmd/hooks_malformed_store_test.go` unchanged and green).
- A lazy pane whose signal generation cannot be turned off still parks on its chain. Its `disable terminal signals failed` WARN carries the pane's durable token as `hook_key` beside its positional `pane_key`, and the error.

STATUS: complete

SPEC CONTEXT: Two rules apply. The first is the audit-trail rule in CLAUDE.md: every `hooks.json` mutation leaves its breadcrumb from the store method, which is the chokepoint, not the caller. `hook set` is machine-invoked from the external SessionStart hook, and `main` prints a command error without logging it, so a registration refused at load time would otherwise leave no trace in portal.log. The second is the phase's own rule that every record on a waiting pane's wait and recovery paths names the pane by its durable token (`hook_key`) as well as by the positional `pane_key` restore baked. A rename, renumber or move leaves the positional key naming another pane.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/hooks/store.go:133-144 — `acquireMutationLockFor(op, key, via)`, the shared acquire-failure WARN (op, hook_key, via, error; no error_class, no value).
  - internal/hooks/store.go:146-158 — `loadForMutation(op, key, via)`, the refused-load handling, declared once. It wraps `ErrStoreRead` with `%w: %w`, so `ErrMalformed` and the OS `*PathError` stay in the chain. It writes the op's WARN with op, hook_key, via and the wrapped error, and no error_class or value.
  - internal/hooks/store.go:165-177 — `Set` routes its acquire and its load through both helpers under its method op `"set"`, never `classifySet`'s verdict. The comment at :166-167 states why.
  - internal/hooks/store.go:259-268 — `removeEntry` routes through the same two helpers under `r.op` (`rm` / `discard`).
  - internal/hooks/store.go:364-367 — the `ErrStoreRead` doc now states what the sentinel means (a read a mutation or a clean could not complete; a failed write never carries it) instead of listing the sites that carry it.
  - internal/hooks/store.go:414-423 — `deleteStale` keeps its bare acquire and its own `ErrStoreRead` wrap, as the task directs; `internal/hooksweep/sweep.go:186-190` records a clean's refusal. `loadDegrading` (:88-95) still degrades.
  - cmd/state_hydrate.go:269-271 — `disable terminal signals failed` logs through `payload.paneRef().logAttrs("error", err)...`. `payload.HookKey` is `cfg.HookKey` (:265), the baked token. This matches the enable side's `enableTTYSignalsOrLog` (cmd/state_resume_chain.go:207-211).
- Notes: All three per-key mutations now answer a refused load the same way, through a single declaration. On the load-success paths of `Set` (:179-199), the code that produces the set, modify and set-noop records and the save-failure WARN is unchanged. `cmd/hooks.go:227-229` returns the store error unwrapped and logs nothing of its own, so a refused `hook set` produces exactly one WARN. No `cmd` caller of `Set` discriminates on the error, so wrapping it in `ErrStoreRead` affects only the message prefix. `cmd/hooks_malformed_store_test.go` checks `errors.Is(err, ErrMalformed)`, which still holds through the `%w: %w` wrap. The file is not in the change-set.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: internal/hooks/store_test.go:256-304 ("it logs a load it could not complete and reports it as a read failure"). It checks `ErrStoreRead` and `os.ErrPermission` in the chain, and exactly one WARN-or-above record with msg/op `set`, component `hooks` and via `cli`. It also checks `hook_key`, that the logged error equals the returned chain and names the file path, that there is no `error_class` and no `value`, and that the file is byte-identical.
  - AC2: internal/hooks/store_test.go:231-254. The malformed-`Set` refusal now asserts both `ErrMalformed` and `ErrStoreRead`, the file unchanged, and exactly one WARN-or-above record `hooks: set` with op=set and via=cli.
  - AC3: internal/hooks/discard_test.go:406-447 pins Discard's unreadable and malformed refusals (ErrStoreRead, a single WARN under op=discard, hook_key, the whole chain, no error_class). Remove reaches the same `loadForMutation` call with `r.op`; its `rm` op is pinned on the acquire and save routes (internal/hooks/lock_write_test.go:120-138), and the internal/hooks/store_test.go:526-539 malformed refusal asserts ErrMalformed and the file byte-identical.
  - AC4: the `AssertLockWarn` pins for set, rm and discard (internal/hooks/lock_write_test.go:98, :117, :137, :200; internal/hooks/discard_test.go:259, :466; cmd/hooks_write_lock_test.go:77, :204) exercise the shared `acquireMutationLockFor`, whose emitted record is unchanged.
  - AC5: internal/hooks/store_set_test.go pins the set, modify and set-noop records and the save-failure WARN with `value` and `error_class` (e.g. :58-65, :107-114, :165-178, :191-205).
  - AC6: cmd/hooks_malformed_store_test.go:24-32 is unchanged.
  - AC7: cmd/state_resume_signals_test.go:103-122 checks that the pane parks on the expected chain argv and that the `disable terminal signals failed` line carries `pane_key=`, the error text and `hook_key=<lazyHookKey>`. `execLogLine` (cmd/state_hydrate_exec_log_test.go:14-27) matches on the `<LEVEL> <msg>` prefix and requires exactly one such line, so the `hook_key=` check would fail if the attr were dropped.
- Notes: No over-testing. Each new assertion pins a distinct property named in the criteria.

CODE QUALITY:
- Project conventions: Followed (the breadcrumb comes from the store method, the closed attr vocabulary is respected, and an attr is added only to the record the task named)
- SOLID principles: Good. The refused-acquire and refused-load handling are each declared once and parameterised by op.
- Complexity: Low
- Modern idioms: Yes (`%w: %w` multi-wrap)
- Readability: Good. I checked the helper docs, the `Set` op comment and the `ErrStoreRead` doc against the code, and each claim holds.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
