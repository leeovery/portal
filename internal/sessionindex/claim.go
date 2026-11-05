package sessionindex

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrNameRefused is matched by a claim's refusal of a name the backend
	// could never create: restore could never make the session again, so
	// nothing is kept for it.
	ErrNameRefused = errors.New("a name Portal could never restore")
	// ErrNoCreation refuses a claim knowing no creation: it can judge no entry
	// by it, and an entry recording none could never be dropped.
	ErrNoCreation = errors.New("it reported no creation")
)

// The refusal of a name the backend's Create refuses, as rename, the picker's
// r and hook set show it.
const (
	// nameRefusedCopy names the session, then the backend's reason.
	nameRefusedCopy = "Portal could never restore %s: %v"
	// nameRefusedNamedCopy is for a reason naming the session itself, as
	// zmx's do, so the name isn't said twice.
	nameRefusedNamedCopy = "Portal could never restore this session: %v"
)

// nameRefusal is a claim's refusal of a name the backend's Create refuses,
// for reason. It matches ErrNameRefused and reason.
type nameRefusal struct {
	name   string
	reason error
}

func (e nameRefusal) Error() string {
	if strings.Contains(e.reason.Error(), strconv.Quote(e.name)) {
		return fmt.Sprintf(nameRefusedNamedCopy, e.reason)
	}
	return fmt.Sprintf(nameRefusedCopy, e.name, e.reason)
}

func (e nameRefusal) Unwrap() []error { return []error{ErrNameRefused, e.reason} }

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
	hydrating func(name string) bool
	names     func(name string) error
}

// NewRule judges an entry created before booted as saved before this boot,
// and a session restoring reports, when restoring is given, as one Portal is
// restoring.
func NewRule(booted time.Time, restoring func(name string) bool) Rule {
	return Rule{judges: true, booted: booted, restoring: restoring}
}

// ReadingHydrating is the rule reading, through hydrating, whether a
// session's restore is unfinished, its hydrating flag raised. A claim of such
// a session repays no restore, and one taking over its entry owes one: only
// its helper's claim repays it (Hydrated).
func (r Rule) ReadingHydrating(hydrating func(name string) bool) Rule {
	r.hydrating = hydrating
	return r
}

// RefusingNames is the rule refusing, through names, a claim of a name the
// backend's Create refuses, which restore could never make again.
func (r Rule) RefusingNames(names func(name string) error) Rule {
	r.names = names
	return r
}

// restoreUnfinished reports whether the named session's restore is
// unfinished, as the rule reads it. Under a rule reading none, no session's
// is.
func (r Rule) restoreUnfinished(name string) bool { return r.hydrating != nil && r.hydrating(name) }

// refusal says why a claim of the session named name, created at created, is
// refused, nil when it isn't.
func (r Rule) refusal(name string, created time.Time) error {
	if r.names != nil {
		if err := r.names(name); err != nil {
			return nameRefusal{name: name, reason: err}
		}
	}
	if created.IsZero() {
		return fmt.Errorf("session %s: %w", name, ErrNoCreation)
	}
	return nil
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
// created at created, running pid, the pid zero when it reports none.
//   - A name rule refuses is refused, wrapping ErrNameRefused, and a claim
//     knowing no creation, which can judge none, with ErrNoCreation. Neither
//     changes anything.
//   - A session with no entry gets one, unless this Update removed its entry:
//     then the claim changes nothing.
//   - An entry of the same creation keeps everything, taking the dir and pid
//     it records none of.
//   - An entry rule says the live session takes over gets its creation, and
//     keeps the rest, taking the dir it records none of. That change is adopt.
//   - Any other is replaced whole, a change of its own, replace, saying what it
//     dropped, and Update reports the entry it read dropped.
//
// An entry kept or taken over is owed no restore any more, its session live,
// unless rule reads that session's restore as unfinished: then it repays
// nothing, and an entry it takes over is owed a restore, which only the
// session's helper repays (Hydrated). It reports whether it changed the
// session's entry.
func (idx *Index) Claim(name, dir string, created time.Time, pid int, rule Rule) (bool, error) {
	if err := rule.refusal(name, created); err != nil {
		return false, err
	}
	i := idx.indexOf(name)
	switch {
	case i < 0:
		if _, made := idx.add(Entry{Name: name, Dir: dir, CreatedAt: created, PID: pid}); !made {
			return false, nil
		}
	case idx.sessions[i].SameCreation(created, pid):
		repaid := !rule.restoreUnfinished(name) && idx.repay(i, liveAgain)
		if !idx.sessions[i].fillIn(dir, pid) {
			return repaid, nil
		}
	default:
		if why := rule.adoption(idx.sessions[i]); why != "" {
			idx.adopt(i, created, pid, "adopt", why)
			if rule.restoreUnfinished(name) {
				idx.owe(i, restoreUnfinished)
			} else {
				idx.repay(i, liveAgain)
			}
			idx.sessions[i].fillIn(dir, 0)
			return true, nil
		}
		earlier := idx.sessions[i]
		idx.sessions[i] = Entry{Name: name, Dir: dir, CreatedAt: created, PID: pid}
		idx.markFresh(name)
		idx.changes = append(idx.changes, change{op: "replace", session: name, value: dir, detail: dropped(earlier)})
		return true, nil
	}
	idx.changes = append(idx.changes, change{op: "claim", session: name, value: dir})
	return true, nil
}

// Restored records the creation restore just made, created and pid, as the
// entry's own that restore read as read, keeping everything else it was saved
// with, and owes it a restore until the session's helper finishes it
// (Hydrated): an entry recording a creation of this boot is no longer one that
// may predate it, so the mark alone keeps it from a drop and brings it back
// should the helper never finish. It reports whether the entry now records
// that creation. It changes nothing in an entry that records it already, as
// one the helper's claim reached first does, nor in one that no longer records
// the creation restore read, one a claim has taken over for another creation
// since: that is no longer restore's to overwrite. A created of zero records
// no creation, so the entry keeps the one it was saved with, and its owed
// mark, for a later claim to take over: the claim rule adopts an owed entry
// whatever creation it records. It consults no rule, so restore is its only
// caller, which a guard holds to.
func (idx *Index) Restored(read Entry, created time.Time, pid int) bool {
	i := idx.indexOf(read.Name)
	if i < 0 {
		return false
	}
	e := idx.sessions[i]
	switch {
	case e.SameCreation(created, pid):
		return true
	case !e.RecordsCreationOf(read), created.IsZero():
		return false
	}
	idx.adopt(i, created, pid, "restored", restoreMadeIt)
	idx.owe(i, restoreUnfinished)
	return true
}

// Hydrated records that the hydrate helper has finished restoring the session
// created at created, running pid: the entry recording that creation is owed
// no restore any more. It reports whether that changed the entry. A restore
// completes here and nowhere else, so the helper's claim is its only caller,
// which a guard holds to.
func (idx *Index) Hydrated(name string, created time.Time, pid int) bool {
	i := idx.indexOf(name)
	if i < 0 || !idx.sessions[i].SameCreation(created, pid) {
		return false
	}
	return idx.repay(i, restoreFinished)
}

// Why an entry is owed a restore, or no longer.
const (
	liveAgain         = "its session is live"
	restoreMadeIt     = "restore made it"
	restoreUnfinished = "its restore is unfinished until its helper's claim"
	restoreFinished   = "its helper finished its restore"
)

// RecordsCreationOf reports whether the entry still records the creation read
// recorded: none, if read recorded none.
func (e Entry) RecordsCreationOf(read Entry) bool {
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
