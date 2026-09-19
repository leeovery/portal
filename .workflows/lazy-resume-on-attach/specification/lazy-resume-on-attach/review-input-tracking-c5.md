# Review Tracking: Lazy Resume On Attach - Input Review

## Findings

### 1. One hand-edited typo either erases itself or stops every later registration

**Source**: `.workflows/lazy-resume-on-attach/discussion/lazy-resume-on-attach.md` — *Unreadable Stored Registrations* (Context: "The store is hand-editable by design, and the object form invites exactly the mistakes a hand edit makes: `\"resume\": \"Lazy\"`, `\"resume\": true`, **a stray key**, or an object that carries settings and no command at all"; Decision: "Nothing is rewritten to correct either case. The file stays as the user left it"), read against *Eager Lazy Preference* ("The external Claude Code `SessionStart` hook fires `portal hook set` on every session start, and **each call rewrites the whole file**"). The stray key is named as an invited mistake and then never answered, and the promise to leave the file alone is never carried across the whole-file rewrite that every registration performs.
**Category**: Gap/Ambiguity
**Move**: settled
**Affects**: §3.2 The stored registration

**Problem**:
The registration store is hand-editable by design and the object form invites exactly the typos the feature documents surviving — a capitalised `Lazy`, a `true` where a word belongs, a note the user parked beside a command. A pane is promised never to fail over one, and the file is promised to stay as the user left it. But every single registration rewrites the whole file, and on this install that happens on every session start — so whatever the writer does with a value it could not make sense of, it does to entries nobody asked it to touch, minutes later.

Built the obvious way, one typo takes one of two outcomes, both bad and both silent. Either the malformed value stops the file loading, and the next pane that registers writes nothing — resume hooks quietly stop being recorded install-wide, and the user finds out on a reboot weeks later when the panes come back as bare shells. Or the value is quietly scrubbed out when an unrelated pane registers, so the user who hand-pinned a registration and mistyped it goes back to fix it and finds their edit gone, with nothing having said so. The tolerant read the feature is built around only holds if the write holds it too.

**Proposal**:
State that a rewrite leaves untouched entries exactly as it found them, and that a value the reader cannot model never fails the load. This is determined by §3.2's own two rules and nothing else: a value the reader cannot make sense of never fails a pane and nothing is rewritten to correct it, while §2.2 confines "written whole" to the entry actually being written. A rewrite that dropped or choked on what it could not model would break the first rule using the second, and would make the string-form-when-there-is-nothing-to-carry rule — which exists so a hand-edited file keeps looking as the user left it — apply to shape alone while content was normalised out from under them.

**Proposed Text**:
(Insert as a new paragraph in §3.2, immediately after the paragraph ending "Nothing is rewritten to correct either case; the file stays as the user left it.")

**A rewrite of one registration leaves every other entry exactly as it found it.** `portal hook set` rewrites the whole file, and an entry the call did not name is written back carrying what it carried — an attribute the reader does not model and a `resume` value it could not make sense of alike. Neither fails the load, so the typo that never fails a pane never fails another pane's registration either. Only the entry being written is written whole (§2.2): what that call is handed is all that entry keeps.

**Resolution**: Pending
**Notes**:

---

## Observations

- The removal breadcrumb's two new vocabulary members — the `op` token `discard` and the `via` token `panel` (§6.4) — are the specification's own; the source decides that the removed command is logged and that it takes the stale sweep's treatment, and names neither token.
- The source's note that a pane which fell through to eager because its marker could not be written also reads zero in the pending count is not carried into §8.2's fall-through paragraph; the indistinguishability it supports is stated.
- The `prefs.json` key name `resume_mode` (§3.1) and the pane option name and value `@portal-resume-pending = 1` (§7.3) are the specification's own, following the existing option vocabulary rather than any source naming.
- The source's testability note for the waiting program — that it sits beside the helper's existing unit-test files and its handover injection seam — is not carried, and belongs to planning rather than here.
