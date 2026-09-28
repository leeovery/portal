TASK: The Marker and the Freeze Travel With the Pane (lazy-resume-on-attach-2-5) — a unit-lane real-tmux suite proving `@portal-resume-pending` and the frozen pane's record survive every tmux rearrangement, plus `Socket.MarkResumePending` / `Socket.ReadResumePending` fixture helpers

ACCEPTANCE CRITERIA:
- After each of `break-pane`, a `kill-window` of an earlier window under `renumber-windows on`, `move-pane` back into the original window, `move-pane` into another session, `respawn-pane -k` and `rename-session`, the subject pane is still reported in `CaptureStructure`'s pending set under its current pane key.
- After each of those, the subject pane's record still carries the baseline `ScrollbackFile` — the file its bytes were filed under before it was marked.
- Each assertion runs immediately after its own move, so a failure names the operation that broke the marker rather than only the end state.
- `renumber-windows on` is set explicitly on the fixture server, and the window-close subtest asserts the surviving window index actually changed.
- The session-rename subtest asserts both halves: the pane key's session component changed, and the record still names the file under the old name.
- Exactly one live pane carries the token and the marker after every move; no rearrangement duplicates either.
- The suite runs in the unit lane (`go test ./internal/state`), skips cleanly where tmux is absent, spawns no daemon, builds no binary, and leaves no server behind.

STATUS: complete

SPEC CONTEXT: Spec 7.3 moves the freeze from the positional `@portal-skeleton-*` server option to the pane user-option `@portal-resume-pending` (value `1`), read as the twelfth `captureFormat` column, on the claim that a pane option travels with the pane through `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename. Spec 7.2 has the frozen pane's previous record merged back on its durable `@portal-pane-id` token rather than its address, so the record keeps naming the file that holds its bytes whatever the pane's address becomes. (The later consolidation task re-files that file under `pane-<token>.bin` in `CaptureAndRefile`. That is a separate step after `CaptureStructure`, so `CaptureStructure`'s carry-forward contract, which this suite tests, is unchanged.) Spec 8.2 says the pane option has no staleness case, and that `respawn-pane -k` by hand is the one open route by which a marker can outlive its waiter.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmuxtest/stamp.go:37 (`MarkResumePending`) and :45 (`ReadResumePending`). Both use raw tmux on the fixture socket, take the option name from `state.ResumePendingOption`, and mirror `StampPaneToken` / `ReadPaneToken`. The value written (`1`) matches production's `SetResumePendingMarker` (internal/state/markers.go:106).
  - internal/state/resume_pending_durability_realtmux_test.go:47 — `TestResumePendingMarkerRealTmuxDurability`. It uses `package state_test`, `tmuxtest.SkipIfNoTmux`, a `ptl-pending-` socket prefix and no build tag.
  - :181-192 `seedPendingFixture`. It builds a three-window subject session whose middle window (index 1) has two panes, plus a second session, and sets `renumber-windows on` globally at :185.
  - :58-77 stamps the token, takes the baseline `CaptureStructure(client, nil, nil, nil)`, asserts the precondition that the pending set is empty and the scrollback file is non-empty, then marks the pane and reads the marker back.
  - :80-108 runs the six rearrangements as sequential subtests, chaining `prev`. Each asserts (a) the pending key, (b) the baseline `ScrollbackFile` and (c) the moved predicate, plus a nested "exactly one pending pane" subtest.
  - :111-174 declares the rearrangements in plan order.
- Notes:
  - Deliberate, sound divergence: the plan says to take the baseline before stamping the token and the marker. The implementation stamps the token before the baseline and the marker after it, with the reason stated at :55-57. This ordering is required, not stylistic. `takePrevRecord` (internal/state/capture.go:257-270) looks a token-bearing live pane up in `byToken` only, with no fallback to its address. A baseline recorded without the token would therefore match nothing on the first move, and the ScrollbackFile assertion would fail for reasons that have nothing to do with the property under test. The marker is still set after the baseline, as the plan intends, and production matches this shape: the token is stamped at registration or restore, long before any pane waits.
  - Traced the fixture by hand against vanilla `-f /dev/null` defaults (base-index 0; windows are sorted by `buildWindows`, capture.go:436-446):
    - The pane starts at subject:1.1.
    - break-pane moves it to subject:3.0.
    - kill-window closes window 0, the lowest other window. Under renumbering the pane goes to subject:2.0, so `windowChanged` does observe the renumber.
    - move-pane back goes to window 0, which is the original window renumbered, giving subject:0.1.
    - move-pane to the other session gives pending-other:0.1.
    - respawn-pane -k leaves it at pending-other:0.1.
    - rename-session gives pending-renamed:0.1.
  - Every ScrollbackFile assertion discriminates: at each step, a positional-only merge would produce a path different from the baseline `scrollback/pending-subject__1.1.bin`. That includes the respawn step, because by then the pane's live key is no longer the baseline key.
  - The rename step renames the pane's current session (pending-other), because the plan chains each step from where the previous one left the pane. The positional key's session half still changes, so a positional match misses. This meets the "file under the old name" criterion in substance.
  - Targets follow the project's pinning rule: `CoordTargetExact` for break-pane and rename-session, `=session:window` via `pendingWindowTarget`, and pane ids elsewhere.

TESTS:
- Status: Adequate
- Coverage: All seven named tests are present: six rearrangement subtests plus the nested "it reports exactly one pending pane after every move" under each. The suite would fail if any of the following broke:
  - the pane option did not survive a move: the pending key would be missing;
  - capture stopped reading the twelfth column: the pending set would be empty;
  - the merge fell back to position: the ScrollbackFile would not match the baseline;
  - a rearrangement duplicated the token (`locatePendingPane` fatals when the count is not 1) or the marker (`len(pending) != 1`);
  - renumbering did not happen: `windowChanged` would stay false once the settle poll times out.
  Preconditions guard the baseline: the pending set is empty, exactly one pane carries the token, the scrollback file is non-empty, and the marker reads back as `1`. Assertions go through `state.CaptureStructure`; raw tmux is used only to stage and to rearrange.
- Notes: There is no over-testing. The settle poll (`capturePendingUntil`, :203-222) returns the settled state or the state at the deadline, and the moved predicate is then asserted, so it does not mask failures. Failures are attributed to the step that caused them because each step is its own subtest.

CODE QUALITY:
- Project conventions: Followed. The suite runs in the unit lane on a `ptl-*` disposable `-S` socket, with no `portaltest` env, no daemon, no built binary and no `t.Parallel`. `tmuxtest.New` registers kill-server and dir removal. The `stamp.go` additions compose no concatenated `-t`.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (per-iteration loop variables under go 1.26, `slices.Sorted(maps.Keys(...))`)
- Readability: Good. The comments were checked against the code and tmux-default behaviour and hold (renumber-windows is off by default; the middle window has two panes; token-first matching).
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "After each of `break-pane`, a `kill-window` of an earlier window under `renumber-windows on`, `move-pane` back into the original window, `move-pane` into another session, `respawn-pane -k` and `rename-session`, the subject pane is still reported in `CaptureStructure`'s pending set under its current pane key." — The suite asserts this, but whether real tmux behaves this way is only proven by running `go test ./internal/state -run TestResumePendingMarkerRealTmuxDurability -v` against the installed tmux and seeing all six subtests and their nested exactly-one subtests pass. The same run settles the baseline-ScrollbackFile, window-index-changed, rename and exactly-one criteria.
- "The suite runs in the unit lane (`go test ./internal/state`), skips cleanly where tmux is absent, spawns no daemon, builds no binary, and leaves no server behind." — Reading settles the missing build tag, `SkipIfNoTmux`, and the absence of any daemon or binary. "Leaves no server behind" needs a run followed by a check that no `tmux -S /tmp/ptl-pending-*/s` server process survives.
