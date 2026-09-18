# Review Tracking: Lazy Resume On Attach - Claims Verification

## Findings

### 1. A captured waiting pane loses the end of its transcript, not just gains a dead panel

**Source**: Tree measurement — `tmux capture-pane -e -p -S -` against a pane holding the alternate screen (tmux 3.7c, disposable socket)
**Category**: Source defect
**Move**: route
**Affects**: §7.1 (what the saver's capture returns over a waiting pane), and the severity §7.2's freeze and §7.3's marker ordering are argued from

**Problem**:
The specification tells the reader that if the saver captures a waiting pane before it is protected, the user's transcript survives and a dead picture of the panel is added after it — an accretion, milder than a replacement. Measured, that is not what happens. The capture the saver actually runs returns only the lines that had already scrolled out of view, then the panel: the most recent screenful of the pane's real output — the part the user was last reading, and the part a resumed session continues from — is absent from the capture and therefore from the pane's saved file. Those lines sit in the buffer tmux keeps behind the panel, which the saver never reads.

The user meets it on the next reboot: a session comes back with the end of its work missing and a dead panel drawn into the history in its place, permanently, with nothing to recover it from. Understated this way, the requirement that a waiting pane be protected before the panel is painted reads as tidiness; measured, the gap between those two steps costs the user a screenful of their own transcript every time it is hit.

**Evidence**:

Claim (§7.1): "Measured against that exact invocation on tmux 3.7c, with a live process holding the alternate screen open over a pane carrying 40 lines of prior output: the capture returns the **primary buffer's history with the alternate screen appended at the tail** — the real transcript, then the card's lines."

Claim (§7.1): "So an unfrozen waiting pane hashes differently from its last write and the saver rewrites its saved file as *transcript plus a picture of the card*. ... The transcript is not lost, which is milder than a replacement, but it is not recoverable either".

Measurement (disposable socket, live server untouched):

```
$ tmux -V
tmux 3.7c

$ tmux -L ptl-claims3 new-session -d -s alt -x 80 -y 24
$ tmux -L ptl-claims3 send-keys -t '=alt:' \
    "clear; for i in \$(seq 1 40); do printf 'T-%02d\n' \$i; done" Enter

# BEFORE the alternate screen — the saver's own invocation
$ tmux -L ptl-claims3 capture-pane -e -p -S - -t '=alt:' | grep -o 'T-[0-9][0-9]'
T-01 T-02 T-03 T-04 T-05 T-06 T-07 T-08 T-09 T-10 T-11 T-12 T-13 T-14 T-15 T-16
T-17 T-18 T-19 T-20 T-21 T-22 T-23 T-24 T-25 T-26 T-27 T-28 T-29 T-30 T-31 T-32
T-33 T-34 T-35 T-36 T-37 T-38 T-39 T-40

# enter the alternate screen, paint a card, hold it with a live process
$ tmux -L ptl-claims3 send-keys -t '=alt:' \
    "printf '\033[?1049h'; printf 'CARD-TOP\nCARD-BODY\n'; sleep 120" Enter
$ tmux -L ptl-claims3 display-message -p -t '=alt:' '#{alternate_on}'
1

# AFTER — the same saver invocation
$ tmux -L ptl-claims3 capture-pane -e -p -S - -t '=alt:' | grep -n 'T-[0-9][0-9]\|CARD'
5:T-01  6:T-02  ... 24:T-20  46:CARD-TOP  47:CARD-BODY
   → T-21 through T-40 are ABSENT; total non-empty lines fell 45 → 26

# where the missing screenful went — the buffer the saver never reads
$ tmux -L ptl-claims3 capture-pane -a -p -t '=alt:' | grep -o 'T-[0-9][0-9]'
T-21 T-22 T-23 T-24 T-25 T-26 T-27 T-28 T-29 T-30 T-31 T-32 T-33 T-34 T-35 T-36
T-37 T-38 T-39 T-40
```

Source carrying the same claim: `.workflows/lazy-resume-on-attach/discussion/lazy-resume-on-attach.md` — line 129 ("the capture returns the **primary buffer's history with the alternate screen appended at the tail** — the real transcript, then the card's lines"), line 131 ("The transcript is not lost, which is milder than a replacement"), and line 528 ("the saver's own `capture-pane -e -p -S -` returns the primary history with the alternate screen appended, so the real failure is accretion rather than loss").

Everything else the section measures holds: the card never enters the primary scrollback (`capture-pane -a -p` returns the original lines intact), and the capture does hash differently from the last write, so the saver does rewrite the file.

**Resolution**: Routed
**Notes**: Re-measured independently on a disposable `-L` socket (tmux 3.7c): `capture-pane -e -p -S -` over a pane holding `T-01…T-40` with the alternate screen on returned `T-01…T-20` plus the card; `capture-pane -a -p` returned exactly `T-21…T-40`. Confirmed. The corrected value undermines no conclusion — the freeze re-lands from it unchanged and with a wider margin — so it landed in the discussion as a dated timeline revision of the Waiting Pane Capture decision (trigger citing the failed measurement), with the Summary's Key Insight 3 repaired in place. The specification's §7.1, §7.2 and §7.3 were re-aligned to what the discussion now carries.

---

### 2. The resume-mode line in `portal doctor` is anchored to a status that does not pass

**Source**: Tree measurement — `portal doctor`, `cmd/doctor.go` status renderer and counter
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §3.4 (how the install-wide resume mode is reported by `portal doctor`), read against §8.1 (the pending-pane count)

**Problem**:
`portal doctor` is to gain two new lines: one reporting how many panes are waiting, one reporting which resume mode the install runs. Both are described as lines that pass and never affect the exit code, but the second is anchored to doctor's informational status, which is not a line that passes — it prints with no tick and is deliberately left out of the "N checks passed" tally the command ends on. Built to that anchor, the two new lines land unlike each other: the pending count carries a tick and is counted, the resume mode carries neither. A user running `portal doctor` after the upgrade sees one of the two new facts presented as a check that passed and the other as an unmarked aside sitting below the last tick, and the summary count silently disagrees with the number of lines above it.

**Proposal**:
The resume-mode line takes the passing-check shape the pending count takes — tick, counted in the summary total, never failing and never touching the exit code. Determined by measurement: doctor's informational status renders a blank marker and is excluded from both the passed and total counts, while a passing check renders `✓` and is counted; the live run shows the one existing informational line (`host terminal`) unmarked and outside the reported total.

**Evidence**:

Claim (§3.4): "**It is reported by `portal doctor` instead, as a passing informational line** carrying the install's resume mode. Doctor already reports on this machinery, and this shape already exists there (`checkInfo`, `cmd/doctor.go:345-350`). Like the pending count (§8.1), it never fails the check and never changes the exit code."

Claim (§8.1): "**Pending panes are visible as a passing count in `portal doctor`** ... The number is detail on a line that passes."

```
$ portal doctor
Portal doctor:
  ✓ daemon: running (pid 11035, version 0.12.0)
  ✓ saver: _portal-saver up
  ✓ hooks: hooks registered (one per event)
  ✓ state dir: /Users/leeovery/.config/portal/state
  ✓ sessions.json: 39 sessions, 40 panes
  ✓ stale hooks: no stale hooks
  ✓ stale projects: no stale projects
    host terminal: unsupported (remote session)      <- the checkInfo line: no tick
  7 checks passed                                     <- 8 lines rendered, 7 counted
exit=0

$ sed -n '564,577p' cmd/doctor.go
func checkMarker(s checkStatus) string {
	switch s {
	case checkPass:
		return "✓"
	case checkFail:
		return "✗"
	case checkNotEvaluable:
		return "·"
	case checkInfo:
		return " "
	...

$ sed -n '590,603p' cmd/doctor.go
func doctorCheckCounts(results []checkResult) (passed, total int) {
	for _, r := range results {
		switch r.status {
		case checkPass:
			passed++
			total++
		case checkFail, checkUnknown:
			total++
		case checkInfo, checkNotEvaluable:
			// Outside the exit-code class: counted by neither, deliberately.
		}
```

The source names the shape without naming the status: `.workflows/lazy-resume-on-attach/discussion/lazy-resume-on-attach.md:414` — "**It is reported by `portal doctor` instead, as a passing line.** Doctor already reports on this machinery and this feature already gives it a passing informational line for the pending-pane count; the install's resume mode is the same shape in the same place."

The rest of §3.4 holds: neither status changes the exit code (`doctorUnhealthy`, `cmd/doctor.go:579-586`, trips only on `checkFail`/`checkUnknown`).

**Current**:
"**It is reported by `portal doctor` instead, as a passing informational line** carrying the install's resume mode. Doctor already reports on this machinery, and this shape already exists there (`checkInfo`, `cmd/doctor.go:345-350`). Like the pending count (§8.1), it never fails the check and never changes the exit code."

**Proposed Text**:
"**It is reported by `portal doctor` instead, as a passing line** carrying the install's resume mode. Doctor already reports on this machinery, and this is the shape the pending count takes (§8.1) — a check that always passes, so it carries the `✓` and counts toward the total the summary line reports (`checkPass`, `cmd/doctor.go:400`). Like the pending count, it never fails the check and never changes the exit code."

**Resolution**: Pending
**Notes**:

---

## Observations

- The install figures re-measure in shape on 2026-09-18: 41 live sessions (40 single-pane, 1 two-pane), 41 of 41 single-window, 38 hook keys all token-shaped and all string-valued, `portal doctor` → "7 checks passed" with no stale hooks; live Claude processes 37 / 11.8 GB / avg 326 MB / peak 638 MB, pageouts 2,044,578 — the counts move as §1 says they do and every shape claim they carry holds.
- The tmux measurements all reproduce on tmux 3.7c: `set-option -p -t <pane> key-table` is accepted and reads back on the session and the sibling pane (unset on all three beforehand); a custom key table swallowed an unbound word entirely while firing its Enter binding; a `display-popup` opened over the left pane took the keystrokes the attached client sent while the right pane was active; `@portal-resume-pending` set with `-p` is invisible on the sibling and on the session, and survives `break-pane`, `move-pane` and a session rename while the positional address moved `pm:1.1 → pm:2.1 → pm:1.2 → renamed:1.2`; a pane process that traps SIGHUP survives `kill-server`, reparented to PID 1.
- §4.2's Go resident floor (1680 KB linking the rendering library, 1696 KB importing nothing, 4.8 MB on disk) can only be checked by compiling two programs, which this pass does not do; the daemon figure beside it re-measures at 22768 KB today, consistent with the recorded 22496 KB.
- Three line citations run a line or two short of the code they name — `internal/hooks/store.go:28-31` (the type is at :32), `:221` (the INFO is at :222), and `cmd/state_hydrate.go:139-148` (the exec is at :149) — each points at the right code.
- §5.5's three Paper frames were not checked; this pass has no route to the design file.
