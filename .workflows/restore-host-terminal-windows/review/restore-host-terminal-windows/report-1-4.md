TASK: restore-host-terminal-windows-1-4 — Inside-tmux detection: list-clients NULL-filter + local-only activity tiebreak

ACCEPTANCE CRITERIA:
1. A session whose only clients walk to mosh-server/NULL => detectInsideTmux returns clean NULL (nil error).
2. A single local client (walks to a .app) => that client's Identity, with no reliance on client_activity.
3. Two local clients with different client_activity => the higher-activity client's Identity wins.
4. ListClients returning an error => detectInsideTmux returns an error satisfying errors.Is(err, ErrDetectTransient).
5. tmux.Client.ListClients parses "<pid> <activity>" lines into ClientInfo correctly and tolerates the no-clients case as an empty slice.

STATUS: Complete

SPEC CONTEXT:
Spec "Terminal Identity & Detection -> Detection model (items 2-4)": inside tmux, take the current session's clients, NULL-filter to local host clients (drop mosh/remote/other-machine); client_activity is demoted to a local-only tiebreak used only to choose among 2+ local clients, never the primary cross-client signal; a purely-remote trigger (no local client) is the honest "no host-local terminal" no-op. Live validation against ~33 clients found `focused` and raw highest-client_activity unreliable across clients, hence the walk-based NULL-filter first. Testing Strategy: detection resolution behind small (1-3 method) seams is unit-testable with fabricated data; the real walk/list-clients is integration/manual.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/clients.go:13-16 (ClientInfo), :28-68 (ListClients); internal/spawn/detect_inside.go:14-17 (ClientActivity), :22-24 (clientLister seam), :29-47 (tmuxClientLister adapter), :73-118 (detectInsideTmux).
- Notes:
  - tmux.ListClients runs `list-clients -t =<session> -F "#{client_pid} #{client_activity}"` via the Commander seam; session routed through exactTarget (=name) so a prefix collision cannot mis-resolve. Any command error collapses to empty slice + nil (mirrors ListSessions:199-204 no-server tolerance, exactly as the task directs); empty output => empty slice; malformed lines (field count != 2, non-numeric pid/activity) return an error. Matches AC5.
  - detectInsideTmux implements the five-step algorithm faithfully: list-clients error => ErrDetectTransient-wrapped (AC4); per-client walkToBundle, resolved non-NULL kept as (id, Activity), clean NULL dropped, transient walk error recorded (firstWalkErr) but scan continues; zero locals + no transient => clean NULL (AC1), zero locals + transient => the already-wrapped firstWalkErr; one local => its Identity (AC2); 2+ locals => max Activity via `client.Activity > bestActivity`, so first-listed wins an exact tie (AC3). The per-client transient policy matches the task's stated "reasonable reading" note.
  - The production adapter tmuxClientLister maps tmux.ClientInfo -> spawn.ClientActivity and carries a compile-time `var _ clientLister` assertion. It is consumed by Task 1.5's NewDetector (not orphaned).
  - Semantics check on AC4: production tmux.ListClients swallows the no-server case (empty+nil) rather than erroring, so only a genuine parse failure surfaces as an error and becomes transient. This is the correct/consistent reading of the seam contract (no-clients is data, not failure) and does not weaken AC4, which is exercised through the seam.
  - No shell injection surface (session passed as discrete argv via exactTarget); read-only.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/detect_inside_test.go covers every AC and every algorithm branch: remote-only => NULL (:46), single local no-tiebreak with Activity:0 explicitly proving activity is ignored (:65), higher-activity-second (:83), higher-activity-first (:101), exact-tie first-wins (:117), mixed local+remote where the remote has FAR higher activity proving NULL-filter precedes the tiebreak (:133), list-clients failure => ErrDetectTransient AND underlying error preserved (:151), walk-failure-with-nothing-local => transient + underlying ps error preserved (:171), local resolves despite a transient walk on another client (:196), and zero-clients => clean NULL (:220). Session passthrough asserted (lister.calls == [dev]).
  - internal/tmux/clients_test.go covers multi-line parse (:12), single line (:38), no-clients error tolerance (:54), empty output (:70), malformed lines table — non-numeric pid / non-numeric activity / missing field (:86), and argv shape asserting `-t =dev` exact target + the client_pid/client_activity format (:107).
- Notes:
  - The three tiebreak tests are complementary, not redundant: higher-first catches a "last-local-wins" regression, higher-second catches a "first-local-wins" regression, exact-tie pins first-wins-on-tie. Genuine value each.
  - Tests assert behaviour (returned Identity, error chain via errors.Is, NULL-ness) not implementation details. No excess mocking; fake seams are the intended DI shape. Not over-tested.
  - Would fail if the feature broke (e.g. dropping the NULL-filter would fail the mixed local+remote test; dropping the max comparison would fail a tiebreak test).

CODE QUALITY:
- Project conventions: Followed. Small 1-method DI seams; spawn-local ClientActivity mirror keeps resolution tmux-free and unit-testable (documented rationale); tmux method mirrors ListSessions' established no-server tolerance and the exactTarget mandate; unit-lane fake-Commander parse test matches tmux_test.go pattern. Real ps/defaults/list-clients boundary left to integration/manual per the skill and spec.
- SOLID principles: Good. detectInsideTmux depends on abstractions (clientLister/ProcessWalker/BundleReader); single responsibility per function; adapter isolates the tmux type mapping.
- Complexity: Low. Single bounded loop, clear branch structure; walk bounded at maxWalkHops.
- Modern idioms: Yes. `for range maxWalkHops`, errors.Is chain via double-%w wrapping (transient()), strings.Fields parsing.
- Readability: Good. Thorough doc comments enumerate every outcome; the "record-first-transient-but-continue" and "activity-only-as-tiebreak" invariants are explained in-line.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Considered: the ClientActivity/ClientInfo duplication is a deliberate, documented seam boundary; the N-client subprocess walk cost is the spec's live-validated design choice with no early-exit possible given the tiebreak needs all locals; an extra ">2 fields" parse case would exercise the same `!= 2` branch already covered — none propose a load-bearing concrete change, so all are dropped per the floor.)
