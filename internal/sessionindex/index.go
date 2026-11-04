package sessionindex

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrLabelUnprintable refuses a label that is not valid UTF-8 or holds a
// control, format or line/paragraph-separator character: a newline, tab,
// escape or bidi override would break or disguise the picker row and the
// footers that print it. Its text is the refusal every surface shows.
var ErrLabelUnprintable = errors.New("label refused: it holds a control or formatting character, such as a newline, tab or escape")

// The two ops that drop an entry, which Update's outcome reports.
const (
	opRemove  = "rm"
	opReplace = "replace"
)

type change struct {
	op      string
	session string
	value   string
	// detail says what the change dropped or displaced, empty when nothing.
	detail string
	// level is the change's log level once written: INFO, its zero value,
	// unless the change is one the saver makes on most polls.
	level slog.Level
}

// Lookup returns a copy of the entry named name as stored, whatever creation
// it records; surfaces that show a live session read through LookupLive.
// Changes go through Index's own methods, which record them for Update to log.
func (idx *Index) Lookup(name string) (Entry, bool) {
	i := idx.indexOf(name)
	if i < 0 {
		return Entry{}, false
	}
	return idx.Sessions[i], true
}

// LookupLive returns a copy of the live session's entry: the session named
// name that its backend created at created, running pid, each zero when it
// reports none. An entry counts only when it records that same creation; any
// other belonged to an earlier session under the name, so the live one has
// none, and shows its name and no dir. A label only a hand-edited file can
// hold, one SetLabel would refuse or clear, unprintable or blank, is dropped
// too, so no surface prints it.
func (idx *Index) LookupLive(name string, created time.Time, pid int) (Entry, bool) {
	e, ok := idx.Lookup(name)
	if !ok || !e.sameCreation(created, pid) {
		return Entry{}, false
	}
	if !printable(e.Label) || strings.TrimSpace(e.Label) == "" {
		e.Label = ""
	}
	return e, true
}

// SetLabel sets the session's label, trimmed of surrounding whitespace. An
// empty label, or one equal to the name, clears it back to the default. A
// session with no entry gets a bare one only when it is given a label;
// clearing one with no entry changes nothing. A label still holding an
// unprintable character once trimmed is refused with ErrLabelUnprintable and
// changes nothing.
func (idx *Index) SetLabel(name, label string) error {
	label = strings.TrimSpace(label)
	if !printable(label) {
		return ErrLabelUnprintable
	}
	if label == name {
		label = ""
	}
	i := idx.indexOf(name)
	if i < 0 {
		if label == "" {
			return nil
		}
		idx.Sessions = append(idx.Sessions, Entry{Name: name})
		i = len(idx.Sessions) - 1
	}
	if idx.Sessions[i].Label == label {
		return nil
	}
	idx.Sessions[i].Label = label
	if label == "" {
		idx.changes = append(idx.changes, change{op: "clear-label", session: name})
	} else {
		idx.changes = append(idx.changes, change{op: "set-label", session: name, value: label})
	}
	return nil
}

// The zero-width joiner is format-class but builds joined emoji, so it stays.
func printable(label string) bool {
	return utf8.ValidString(label) && !strings.ContainsFunc(label, func(r rune) bool {
		return r != '\u200d' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp))
	})
}

// Claim makes the session's entry the live session's: the one its backend
// created at created, running pid, each zero when it reports none. A session
// recreated under a name is a new session, told apart only by its creation.
// So an entry from the same creation keeps its label and scrollback, taking
// dir and pid only where it records none; an entry from another creation, or
// with no time, belonged to an earlier session and is replaced whole; a
// session with no entry gets one. A replacement is its own change, replace,
// saying what it dropped, so the log shows a label being discarded.
func (idx *Index) Claim(name, dir string, created time.Time, pid int) {
	fresh := Entry{Name: name, Dir: dir, CreatedAt: created, PID: pid}
	i := idx.indexOf(name)
	switch {
	case i < 0:
		idx.Sessions = append(idx.Sessions, fresh)
	case idx.Sessions[i].sameCreation(created, pid):
		if !idx.Sessions[i].fillIn(dir, pid) {
			return
		}
	case idx.Sessions[i] == fresh:
		return
	default:
		earlier := idx.Sessions[i]
		idx.Sessions[i] = fresh
		idx.changes = append(idx.changes, change{op: opReplace, session: name, value: dir, detail: dropped(earlier)})
		return
	}
	idx.changes = append(idx.changes, change{op: "claim", session: name, value: dir})
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

// sameCreation reports whether the entry records the session created at
// created, running pid. zmx keeps created times to the second, so the pid
// tells two creations in one second apart wherever both sides know it. An
// entry with no time can't be told from an earlier one.
func (e Entry) sameCreation(created time.Time, pid int) bool {
	return !e.CreatedAt.IsZero() && e.CreatedAt.Equal(created) && (e.PID == 0 || pid == 0 || e.PID == pid)
}

// Restored records the creation restore just made for the session, created
// and pid, as its entry's own, keeping the label, dirs and scrollback the
// entry was saved with. A zero created records nothing, leaving the entry's
// earlier creation in place. It reports whether the session had an entry; one
// with none gets none.
func (idx *Index) Restored(name string, created time.Time, pid int) bool {
	i := idx.indexOf(name)
	if i < 0 {
		return false
	}
	e := &idx.Sessions[i]
	if created.IsZero() || e.CreatedAt.Equal(created) && e.PID == pid {
		return true
	}
	detail := fmt.Sprintf("pid %d, in place of %s", pid, creationOf(*e))
	e.CreatedAt, e.PID = created, pid
	idx.changes = append(idx.changes, change{op: "restored", session: name, value: created.UTC().Format(time.RFC3339), detail: detail})
	return true
}

func creationOf(e Entry) string {
	if e.CreatedAt.IsZero() {
		return "no recorded creation"
	}
	return "the creation of " + e.CreatedAt.UTC().Format(time.RFC3339)
}

// SetScrollback names the file, relative to the state directory, holding the
// session's saved scrollback. It changes only an entry the session has.
func (idx *Index) SetScrollback(name, file string) {
	idx.setField(name, "set-scrollback", file, slog.LevelInfo, func(e *Entry) *string { return &e.ScrollbackFile })
}

// RecordCwd records the session's working directory. It changes only an
// entry the session has, and logs at DEBUG: the saver records it on most
// polls.
func (idx *Index) RecordCwd(name, cwd string) {
	idx.setField(name, "record-cwd", cwd, slog.LevelDebug, func(e *Entry) *string { return &e.Cwd })
}

// setField sets the field of name's entry to value, recording op when that
// changes it.
func (idx *Index) setField(name, op, value string, level slog.Level, field func(*Entry) *string) {
	i := idx.indexOf(name)
	if i < 0 {
		return
	}
	if f := field(&idx.Sessions[i]); *f != value {
		*f = value
		idx.changes = append(idx.changes, change{op: op, session: name, value: value, level: level})
	}
}

// Remove drops the session's entry, reporting whether there was one.
func (idx *Index) Remove(name string) bool {
	i := idx.indexOf(name)
	if i < 0 {
		return false
	}
	idx.Sessions = slices.Delete(idx.Sessions, i, i+1)
	idx.changes = append(idx.changes, change{op: opRemove, session: name})
	return true
}

func (idx *Index) indexOf(name string) int {
	return slices.IndexFunc(idx.Sessions, func(e Entry) bool { return e.Name == name })
}
