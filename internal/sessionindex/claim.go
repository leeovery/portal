package sessionindex

import (
	"fmt"
	"strings"
	"time"
)

// Rule is what a claim judges an entry from another creation by. A session
// recreated under a name outside Portal is a new session and starts clean, so
// an entry left by an earlier session of this boot is replaced. Any other
// belongs to a session Portal created or restored, whose label, dirs,
// scrollback and hook it must keep: one whose creation was never recorded,
// one saved before this boot or owed a restore and so awaiting one, and one
// whose session Portal is restoring now. NewRule builds the rule; the zero
// Rule can judge nothing, so a claim under it replaces no entry and can lose
// none.
type Rule struct {
	judges    bool
	booted    time.Time
	restoring func(name string) bool
}

// NewRule judges an entry created before booted as saved before this boot,
// and a session restoring reports, when restoring is given, as one Portal is
// restoring.
func NewRule(booted time.Time, restoring func(name string) bool) Rule {
	return Rule{judges: true, booted: booted, restoring: restoring}
}

// adoption says why the live session takes over e, an entry from another
// creation, rather than replacing it, and is empty when it replaces it.
func (r Rule) adoption(e Entry) string {
	switch why := r.predating(e); {
	case why != "":
		return why
	case e.Owed:
		return "it is owed a restore"
	case r.restoring != nil && r.restoring(e.Name):
		return "its session is restoring"
	}
	return ""
}

// MayPredateBoot reports whether e may be an entry saved before this boot,
// which a restore can yet bring back: one recording no creation, one recording
// a creation before the boot, and any entry under the zero Rule, which knows
// no boot.
func (r Rule) MayPredateBoot(e Entry) bool { return r.predating(e) != "" }

// predating says why e may be an entry saved before this boot, and is empty
// when it can't be.
func (r Rule) predating(e Entry) string {
	switch {
	case e.CreatedAt.IsZero():
		return "its creation was never recorded"
	case !r.judges:
		return "no rule to judge it by"
	case e.CreatedAt.Before(r.booted):
		return "it was saved before this boot"
	}
	return ""
}

// LookupLive returns a copy of the live session's entry: the one recorded for
// the session named name created at created, running pid, or one rule says a
// claim of it would take over. A live session reporting no creation can be
// matched to no entry. Any other entry belonged to an earlier session under
// the name, so the live one has none, and shows its name and no dir. A label
// only a hand-edited file can hold, one SetLabel would refuse or clear,
// unprintable or blank, is dropped too, so no surface prints it.
func (idx *Index) LookupLive(name string, created time.Time, pid int, rule Rule) (Entry, bool) {
	e, ok := idx.Lookup(name)
	if !ok || created.IsZero() || !e.SameCreation(created, pid) && rule.adoption(e) == "" {
		return Entry{}, false
	}
	if !printable(e.Label) || strings.TrimSpace(e.Label) == "" {
		e.Label = ""
	}
	return e, true
}

// Claim makes the session's entry the live session's: the one its backend
// created at created, running pid, each zero when it reports none.
//   - A session with no entry gets one, unless this Update removed its entry:
//     then the claim changes nothing.
//   - A claim knowing no creation can judge none, and changes nothing.
//   - An entry of the same creation keeps everything, taking the dir and pid
//     it records none of.
//   - An entry rule says the live session takes over gets its creation, and
//     keeps the rest, taking the dir it records none of. That change is adopt.
//   - Any other is replaced whole, a change of its own, replace, saying what it
//     dropped, and Update reports the entry it read dropped.
//
// An entry kept or taken over is owed no restore any more: its session is
// live. It reports whether it changed the session's entry.
func (idx *Index) Claim(name, dir string, created time.Time, pid int, rule Rule) bool {
	i := idx.indexOf(name)
	switch {
	case i < 0:
		if _, made := idx.add(Entry{Name: name, Dir: dir, CreatedAt: created, PID: pid}); !made {
			return false
		}
	case created.IsZero():
		return false
	case idx.sessions[i].SameCreation(created, pid):
		repaid := idx.repay(i, liveAgain)
		if !idx.sessions[i].fillIn(dir, pid) {
			return repaid
		}
	default:
		if why := rule.adoption(idx.sessions[i]); why != "" {
			idx.adopt(i, created, pid, "adopt", why)
			idx.repay(i, liveAgain)
			idx.sessions[i].fillIn(dir, 0)
			return true
		}
		earlier := idx.sessions[i]
		idx.sessions[i] = Entry{Name: name, Dir: dir, CreatedAt: created, PID: pid}
		idx.markFresh(name)
		idx.changes = append(idx.changes, change{op: "replace", session: name, value: dir, detail: dropped(earlier)})
		return true
	}
	idx.changes = append(idx.changes, change{op: "claim", session: name, value: dir})
	return true
}

// Restored records the creation restore just made, created and pid, as the
// entry's own that restore read as read, keeping everything else it was saved
// with, and owed no restore any more. It reports whether the entry now
// records that creation. It changes nothing in an entry that no longer
// records the creation restore read, one a claim has taken over for another
// creation since: that is no longer restore's to overwrite. A created of zero
// records no creation, so the entry keeps the one it was saved with for a
// later claim to take over. It consults no rule, so restore is its only
// caller, which a guard holds to.
func (idx *Index) Restored(read Entry, created time.Time, pid int) bool {
	i := idx.indexOf(read.Name)
	if i < 0 {
		return false
	}
	e := idx.sessions[i]
	same := e.SameCreation(created, pid)
	if !same && !e.recordsCreationOf(read) {
		return false
	}
	idx.repay(i, restoreMadeIt)
	switch {
	case same:
		return true
	case created.IsZero():
		return false
	}
	idx.adopt(i, created, pid, "restored", restoreMadeIt)
	return true
}

// Why an entry is owed no restore any more.
const (
	liveAgain     = "its session is live"
	restoreMadeIt = "restore made it"
)

// recordsCreationOf reports whether the entry still records the creation read
// recorded: none, if read recorded none.
func (e Entry) recordsCreationOf(read Entry) bool {
	if read.CreatedAt.IsZero() {
		return e.CreatedAt.IsZero()
	}
	return e.SameCreation(read.CreatedAt, read.PID)
}

// adopt makes the creation created, running pid, the entry at i's own,
// recording the change as op, why it was adopted in its detail.
func (idx *Index) adopt(i int, created time.Time, pid int, op, why string) {
	e := &idx.sessions[i]
	detail := fmt.Sprintf("%s; pid %d, in place of %s", why, pid, creationOf(*e))
	e.CreatedAt, e.PID = created, pid
	idx.changes = append(idx.changes, change{op: op, session: e.Name, value: created.UTC().Format(time.RFC3339), detail: detail})
}

func creationOf(e Entry) string {
	if e.CreatedAt.IsZero() {
		return "no recorded creation"
	}
	return "the creation of " + e.CreatedAt.UTC().Format(time.RFC3339)
}

// fillIn takes the dir and pid the entry records none of yet, reporting
// whether it took either.
func (e *Entry) fillIn(dir string, pid int) bool {
	took := false
	if e.Dir == "" && dir != "" {
		e.Dir, took = dir, true
	}
	if e.PID == 0 && pid != 0 {
		e.PID, took = pid, true
	}
	return took
}

// dropped says what replacing an earlier creation's entry discards.
func dropped(earlier Entry) string {
	what := "an earlier creation's entry"
	if !earlier.CreatedAt.IsZero() {
		what += " (created " + earlier.CreatedAt.UTC().Format(time.RFC3339) + ")"
	}
	label, scrollback := earlier.Label, earlier.ScrollbackFile
	if label == "" {
		label = "none"
	}
	if scrollback == "" {
		scrollback = "none"
	}
	return fmt.Sprintf("dropped %s, its label %q and its scrollback %q", what, label, scrollback)
}

// SameCreation reports whether the entry records the session created at
// created, running pid. zmx keeps created times to the second, so the pid
// tells two creations in one second apart wherever both sides know it. An
// entry with no time can't be told from an earlier one.
func (e Entry) SameCreation(created time.Time, pid int) bool {
	return !e.CreatedAt.IsZero() && e.CreatedAt.Equal(created) && (e.PID == 0 || pid == 0 || e.PID == pid)
}
