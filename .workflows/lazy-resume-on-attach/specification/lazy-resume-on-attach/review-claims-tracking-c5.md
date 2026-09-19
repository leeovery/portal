# Review Tracking: Lazy Resume On Attach - Claims Verification

## Findings

### 1. `portal doctor` has more than pass and fail, and already reports facts without a tick

**Source**: Tree measurement — `sed -n '32,42p' cmd/doctor.go`; `grep -n -A9 'func doctorUnhealthy' cmd/doctor.go`; `grep -n -A14 'func doctorCheckCounts' cmd/doctor.go`; `sed -n '336,352p' cmd/doctor.go`; `portal doctor; echo "exit=$?"`
**Category**: Source defect
**Move**: route
**Affects**: §8.1 (`portal doctor` reports a count), §3.4 (Reading it back — the install-wide default reported by doctor)

**Problem**:
The specification has `portal doctor` render the pending-resume count and the install's resume mode as ticked lines that count toward the "N checks passed" tally, and it rests that on a stated fact about the tool: that a check passes or fails, and that the exit code is zero only if all pass. Neither half is true. A doctor line carries one of five statuses; two of them — an informational line and a not-evaluable one — neither pass nor fail, are excluded from the exit code *and* from the passed/total tally, and print with no tick. Doctor already puts a line of exactly this kind in the informational status: the host-terminal line, whose reason in source is that it names "an environmental state, not a Portal-health defect". Run against the live install today, `portal doctor` printed eight lines, summarised "7 checks passed" and exited 0 — an install where a line did not pass and the exit code was zero anyway.

What the user gets if this is built as written: a doctor report carrying `✓ resume mode: lazy` and `✓ pending resumes: 41 pending` counted as health checks, sitting directly beside an unticked `host terminal: …` line that reports the same kind of fact — two conventions in one report, and a "N checks passed" tally that no longer counts health conditions verified. Anyone reading the report, or scripting on that count, sees it move by two for reasons that have nothing to do with the install's health. The decision the specification reaches — that the count must never fail and never change the exit code — survives the correction; what the misstated fact hides is that Portal already has a shape for a line that reports rather than judges, and that the choice between the two is a real one.

The specification carries the claim faithfully from the discussion, which states it in the same words, so the correction belongs to the discussion record.

**Evidence**:

Claim (specification §8.1, verbatim):
> "That follows from what doctor's exit code means against what this feature produces. A check passes or fails, the exit code is zero only if all pass, and the catalog's nearest neighbours — the stale-hook and stale-project counts — fail the moment their count is non-zero (`cmd/doctor.go:398`, `:420`)."

Claim (specification §3.4, verbatim):
> "**It is reported by `portal doctor` instead, as a passing line** carrying the install's resume mode. Doctor already reports on this machinery, and this is the shape the pending count takes (§8.1) — a check that always passes, so it carries the `✓` and counts toward the total the summary line reports (`checkPass`, `cmd/doctor.go:400`)."

Measurement 1 — the status vocabulary is five members, not two:
```
$ sed -n '32,42p' cmd/doctor.go
type checkStatus int

const (
	// Zero value: an unset status counts as unhealthy, never as a passing check.
	checkUnknown checkStatus = iota
	checkPass
	checkFail
	// checkInfo and checkNotEvaluable never drive the exit code.
	checkInfo
	checkNotEvaluable
)
```

Measurement 2 — the exit code is driven by fail/unknown alone, not by "all pass":
```
$ grep -n -A9 'func doctorUnhealthy' cmd/doctor.go
579:func doctorUnhealthy(results []checkResult) bool {
580-	for _, r := range results {
581-		if r.status == checkFail || r.status == checkUnknown {
582-			return true
583-		}
584-	}
585-	return false
586-}
```

Measurement 3 — informational and not-evaluable lines are counted by neither side of the summary:
```
$ grep -n -A14 'func doctorCheckCounts' cmd/doctor.go
590:func doctorCheckCounts(results []checkResult) (passed, total int) {
591-	for _, r := range results {
592-		switch r.status {
593-		case checkPass:
594-			passed++
595-			total++
596-		case checkFail, checkUnknown:
597-			total++
598-		case checkInfo, checkNotEvaluable:
599-			// Outside the exit-code class: counted by neither, deliberately.
600-		}
601-	}
602-	return passed, total
603-}
```
(`checkMarker`, `cmd/doctor.go:564-577`: `checkPass` → `✓`, `checkFail` → `✗`, `checkNotEvaluable` → `·`, `checkInfo` → a blank cell.)

Measurement 4 — doctor already reports a non-health fact in the informational status:
```
$ sed -n '336,352p' cmd/doctor.go
// An unsupported or remote host is an environmental state, not a Portal-health
// defect, so this line never drives the exit code. A NULL identity short-circuits
// before Resolve, so a config `*` catch-all cannot reclassify a remote client.
func checkHostTerminal(detector TerminalDetector, resolve spawn.AdapterResolver) checkResult {
	const name = "host terminal"
	id := detector.Detect()
	if id.IsNull() {
		return checkResult{name: name, status: checkInfo, detail: "unsupported (remote session)"}
	}
	...
```

Measurement 5 — a live run exits 0 with a line that did not pass:
```
$ portal doctor; echo "exit=$?"
Portal doctor:
  ✓ daemon: running (pid 11035, version 0.12.0)
  ✓ saver: _portal-saver up
  ✓ hooks: hooks registered (one per event)
  ✓ state dir: /Users/leeovery/.config/portal/state
  ✓ sessions.json: 39 sessions, 40 panes
  ✓ stale hooks: no stale hooks
  ✓ stale projects: no stale projects
    host terminal: unsupported (remote session)
  7 checks passed
exit=0
```

Source carrying the claim: `.workflows/lazy-resume-on-attach/discussion/lazy-resume-on-attach.md`, `## Pending Visibility` (Decision), line 378 — "**Settled by derivation** (2026-09-18) — not discussed. Determined by what doctor's exit code means against what this feature produces: a check passes or fails, the exit code is zero only if all pass, and the catalog's nearest neighbours — the stale-hook and stale-project counts — fail the moment their count is non-zero. … (review-002 F8)"; and `## Eager Lazy Preference` (Decision), line 432 — "**It is reported by `portal doctor` instead, as a passing line.** Doctor already reports on this machinery and this feature already gives it a passing informational line for the pending-pane count".

**Resolution**: Routed
**Notes**: Re-measured independently: `cmd/doctor.go:34-42` declares five statuses with `checkInfo`/`checkNotEvaluable` commented "never drive the exit code"; `doctorUnhealthy` (:579) keys on fail/unknown alone; the host-terminal line's own comment reads "an environmental state, not a Portal-health defect". Confirmed. The decision resting on the claim — neither line fails or touches the exit code — survives unchanged; only the premise and the rendering that followed from it were wrong, so both were repaired in place in the discussion (the decision itself did not move) and §3.4/§8.1 were re-aligned. **This reverses cycle 1's claims finding 2**, which moved the resume-mode line from `checkInfo` to `checkPass` on the strength of the binary framing now refuted. Cycle 1 was right that the two lines must land alike and wrong about which way.

---

## Observations

- `internal/hooks/store.go:221` is cited for the typed removal's `op=rm` breadcrumb; line 221 is blank — the INFO emission is at `:222` and its failed-write twin at `:217`. Substance holds (the line carries no `value`).
- `internal/hooks/store.go:28-31` is cited for the on-disk `map[hook_key]map[event]command` shape; the comment stating it runs `:29-31` and `type Snapshot map[string]map[string]string` is at `:32`.
- `cmd/state_hydrate.go:172-196` is cited for the hook exec; the function runs `:172-198` and the `cfg.ExecShell("/bin/sh", args)` call is at `:197`.
- The install's point-in-time figures re-measured 2026-09-19 and agree in shape with those recorded: 41 live sessions, 40 holding one pane and 1 holding two, 41 windows all singletons, 38 `hooks.json` keys all 6-char token-shaped with no old-format key, `portal doctor` → "7 checks passed" exit 0, daemon RSS 22960 KB.
- §4.2's Go-binary floor figures (1680 KB / 1696 KB resident, 4.8 MB on disk) cannot be re-measured without compiling, which this pass does not do; the daemon's ~22 MB ceiling beside them re-measured at 22960 KB today.
- The tmux-behaviour measurements (`key-table` is a session option and absent from the pane-option set; a popup is "a rectangular box drawn over the top of any panes" bound to `target-client`; `display-menu` is client-scoped and takes name/key/command triples; `capture-pane -a` reads the alternate screen with the history inaccessible) were corroborated against tmux 3.7c's own manual on this machine rather than re-run, since re-running needs a live server.
