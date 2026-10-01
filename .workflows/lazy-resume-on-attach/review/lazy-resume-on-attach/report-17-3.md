TASK: Corrections — CLAUDE.md "Resume hooks" discard sentence brought in line with the conditional `hooks.Store.Discard` (tick-bc4a74, lazy-resume-on-attach-17-3)

ACCEPTANCE CRITERIA:
- CLAUDE.md's "Resume hooks" discard sentence (`:186`) states that the confirmed discard removes the pane's `on-resume` entry only when its stored command matches the command the confirmation showed byte-for-byte, and that the resume mode is not compared.
- The same sentence states that an entry rewritten to a different command during the wait is left in place and answered as nothing to remove, with no write and no `op=discard` line.
- The sentence's other claims stand as they read today: the key's other events and every other entry stand; no token is unstamped; a removed command is logged as `value` under `op=discard`; `rm`, `discard` and `clean-stale` stay greppable apart.
- No other CLAUDE.md text changes, and no source or test file changes.

STATUS: complete

SPEC CONTEXT: Specification section 6.2 makes a confirmed discard remove the registration the confirmation named: the stored entry goes only when its command is byte-for-byte the command both screens showed, compared under the store's lock, with the resume mode not compared. An entry already gone or replaced by a re-registration naming a different command is "still a discard" (marker clears, pane falls to a shell). Section 6.4 records the removed command as `value` under the new `op=discard` / `via=panel` so the three removal routes stay greppable apart. Section 6.2 also holds that discarding neither unstamps the durable token nor touches any other entry.

IMPLEMENTATION:
- Status: Implemented
- Location: CLAUDE.md:186 (the "A lazy resume's waiting panel is a third removal route…" passage); checked against internal/hooks/store.go:234-240 (`Discard`, `matches: r.Command == shown`), internal/hooks/store.go:259 (mutation lock acquired), :265 (load), :274-277 (mismatch returns `false, nil` before the save at :284 and the INFO breadcrumb at :294), :290-293 (`value` carried only for `carryValue`); cmd/state_resume_wait.go:68-70 (`DiscardRegistration(hookKey, shown)` — the waiter hands the shown command through).
- Notes: Every claim in the edited passage holds against the code. "Byte-for-byte" matches the plain `==` comparison. "Compared under the store's lock" matches the lock being acquired before the load and the comparison. "The resume mode is not compared, so the same command re-registered under another mode is still removed" matches the predicate reading only `Command`, and `TestDiscard_OnlyTheShownCommand` pins it. "Left in place and answered as nothing to remove, with no write and no `op=discard` line" matches the early `false, nil` return, which comes before both `s.save` and `logger.Info`. The retained claims (other events and entries stand, no token unstamped, `value` under `op=discard`, routes greppable apart, recoverable from the log) are unchanged in substance. The rewritten-entry clause sits in its own sentence directly after the discard sentence rather than inside it. The criterion's "same sentence" wording is met in substance: the paragraph states the rule, and nothing is lost.

TESTS:
- Status: Adequate
- Coverage: This is a documentation-only task, so no new tests are expected. The behaviour the paragraph now documents is already pinned in internal/hooks/discard_test.go. `TestDiscard_OnlyTheShownCommand` covers re-registered, hand-edited and whitespace-differing rewrites being left in place with no records and an unchanged file, and the same command under another mode still being removed. `TestDiscard` covers other events surviving, other entries being byte-unchanged, `value` carried, and `discard` / `rm` greppable apart.
- Notes: None

CODE QUALITY:
- Project conventions: Followed. No spec section numbers or task ids in the CLAUDE.md text.
- SOLID principles: Good (N/A — prose edit)
- Complexity: Low
- Modern idioms: Yes (N/A)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "No other CLAUDE.md text changes, and no source or test file changes." — Settling this needs the diff of the commit carrying tick-bc4a74 (for example, `git show --stat` plus the CLAUDE.md hunk of that commit). It must confirm that the only change is the CLAUDE.md:186 discard passage and that no source or test file was touched. The current file content cannot show what changed.
