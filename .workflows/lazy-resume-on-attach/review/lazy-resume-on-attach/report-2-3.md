TASK: lazy-resume-on-attach-2-3 — A Frozen Pane's Previous Record Is Found by Its Durable Token

ACCEPTANCE CRITERIA:
- A pending pane whose session, window index or pane index changed between captures keeps the `ScrollbackFile` of its previous record, and that path is in `ComputeReferencedSet` of the fresh index, so `gcOrphanScrollback` leaves the file on disk.
- The merged pane sits at its live address with the live `Index` and `Active`, and carries the previous record's `CWD`, `CurrentCommand` and `ScrollbackFile`.
- A pending pane carrying no token takes the previous record at its own address; one with neither a token match nor a previous record at its address keeps the fresh record, and its fresh `ScrollbackFile` names the file at its current address.
- A previous record whose pane is absent from the live enumeration is never added to the fresh index — covering a gone session, a gone window and a gone pane.
- A token held by two previous records resolves to the first in canonical order, and a second pending pane carrying the same token does not take that record again.
- The merge runs when the caller passed a nil skip set, so `portal state commit-now` gets it too.
- Two successive captures over an unchanged server, the first's index fed back as the second's `prev`, produce indexes equal once `SavedAt` is zeroed — so `structuralChange` reports none and the commit does not rewrite `sessions.json` every tick while a pane waits.
- The existing skeleton-marker merge behaves exactly as before for every case its suites cover.

STATUS: issues_found

SPEC CONTEXT: Section 7.2 (and its 2026-09-19 corrigendum) requires a waiting pane to keep its place in the saved set. Its previous record is merged back by matching the pane's `@portal-pane-id` rather than its address, and the merged record keeps pointing at the file that already holds the pane's bytes. The match is taken only against panes found in the same enumeration, which is what stops a gone pane coming back. Without this, a rearrangement during an indefinite wait leaves the fresh record naming a file nothing writes, and the housekeeping pass deletes the real one. The same section records that the positional file name is still a write address for whatever pane later takes that address. That gap is closed by task 2-6's re-file onto `scrollback/pane-<token>.bin`, which builds on this merge and is outside this task.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/capture.go:110-112 — `mergeFrozenPanes` runs after `mergeSkippedPanes` when `len(live) > 0 && prev != nil`, so it does not depend on the caller's skip set.
  - internal/state/capture.go:121-136 — `livePane`/`addPendingPanes` take each pending pane's token and active flag from the enumeration, and only for sessions that made it into the Index.
  - internal/state/capture.go:178-203 — `mergeFrozenPanes` walks only panes that are in the fresh index. It restores the live token and `Active`, then copies `CWD`/`CurrentCommand`/`ScrollbackFile` from the matched previous record.
  - internal/state/capture.go:207-253 — `indexPrevPanes`/`canonicalPrevPanes` build the token and address lookups in canonical order (session, window, pane). Empty tokens are skipped and the first holder of a token wins.
  - internal/state/capture.go:257-270 — `takePrevRecord` looks up by token, falls back to the address only for a pane with no token, and consumes whatever it returns.
  - internal/state/capture.go:146-171 — `mergeSkippedPanes` is intact: live-structure guard, positional key, `resortIndex`.
  - cmd/state_commit_now.go:121 — commit-now passes a nil skip set and a non-nil `&prev`, so the merge runs on that route too.
  - CLAUDE.md `state` row — has the required sentence (token match, record keeps the path it was filed under, live address plus previous content, the file stays referenced).
- Notes: The signature differs from the plan. The plan had `mergeFrozenPanes(fresh, prev, pending map[string]struct{})`; the code passes `map[string]livePane`, which carries the enumeration's token and active flag. This is sound and better than the plan. For a pane carrying both markers, `mergeSkippedPanes` has already replaced the fresh record with the positional previous record, including that record's `Active` and `PortalPaneID`. Reading from the fresh index would then take the token and active flag from the wrong record. Taking them from the enumeration keeps "Active stays live" true on that path. The test "it ends a pane carrying both markers on the token-matched record" pins it. No append path exists: the merge only mutates panes already in the fresh index, which structurally rules out resurrecting a gone record.

TESTS:
- Status: Adequate
- Coverage: internal/state/capture_frozen_merge_test.go has every planned test:
  - moved window, moved pane index and renamed session
  - the file kept and the fresh path not referenced after a move
  - live `Index`/`Active` with the previous content, asserted field by field; the previous record has `Active=false` and the live pane `true`, so a merge that carried the stale flag would fail
  - the tokenless positional fallback
  - no previous record, so the fresh record and fresh path are kept
  - a gone session, window and pane
  - a duplicated token consumed once, with the second pending pane on its fresh record and not the second previous record
  - a nil skip set
  - a second capture over an unchanged server, compared with `reflect.DeepEqual` after zeroing `SavedAt`

  There is also a both-markers ordering test. internal/state/commit_test.go:449-472 runs `Commit` (and so `gcOrphanScrollback`) over a moved frozen pane: the transcript file survives and an unreferenced file is reclaimed, so both halves of the edge case are asserted. The skeleton-merge suites use `paneLine`, whose pending column is empty, so the frozen merge never runs under them.
- Notes: None that clear the bar.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, black-box `state_test` package, shared `captureMock`/`commandertest` fake, no process-artifact references in comments.
- SOLID principles: Good. Indexing, lookup and mutation are separate small functions.
- Complexity: Acceptable. The pane walk is a straightforward three-level nest.
- Modern idioms: Yes
- Readability: Good, apart from the doc comment below.
- Issues: The exported contract on `CaptureStructure` states the match as token-only, but the code falls back to the address for a pane with no token (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/state/capture.go:37-39 — The `CaptureStructure` doc says a pending pane keeps its previous record's `CWD`, `CurrentCommand` and `ScrollbackFile` "matched on the pane's durable token rather than its address". The code matches a pane with no token on its address instead (`takePrevRecord`, internal/state/capture.go:265-269). The `mergeFrozenPanes` doc at internal/state/capture.go:173-174 leaves out the same fallback. Replacement for lines 37-39: "Independently of skipSet, a pane carrying the resume pending marker keeps the CWD, CurrentCommand and ScrollbackFile of the previous record carrying its durable token, whatever its address has become — or, for a pane carrying no token, of the previous record at its own address." Also add "(on its address when it carries none)" after "matched on the pane's durable token" at line 174. Comment-only; not blocking. — FAILS: the contract says a pending pane is never matched by address, which the tokenless branch contradicts. Anyone reasoning about a tokenless waiting pane from the exported doc (for example alongside `RefilePendingScrollback`'s tokenless rule) will conclude it ends on its fresh record, when it actually takes whatever record sits at its current address.

UNSETTLED:
- None
