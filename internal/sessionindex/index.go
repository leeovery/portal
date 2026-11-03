package sessionindex

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrLabelUnprintable refuses a label that is not valid UTF-8 or holds a
// control, format or line/paragraph-separator character: a newline, tab,
// escape or bidi override would break or disguise the picker row and the
// footers that print it.
var ErrLabelUnprintable = errors.New("session label contains an unprintable character")

type change struct {
	op      string
	session string
	value   string
	// detail says what the change dropped, empty when it dropped nothing.
	detail string
}

// Lookup returns a copy of the entry named name. Changes go through Index's
// own methods, which record them for Update to log.
func (idx *Index) Lookup(name string) (Entry, bool) {
	i := idx.indexOf(name)
	if i < 0 {
		return Entry{}, false
	}
	return idx.Sessions[i], true
}

// Label is what Portal shows for the session: its label, or its name when it
// has none.
func (idx *Index) Label(name string) string {
	if e, ok := idx.Lookup(name); ok && e.Label != "" {
		return e.Label
	}
	return name
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
// created at created, zero when it reports no time. A session recreated under
// a name is a new session, told apart only by that time. So an entry from the
// same creation keeps its label and scrollback, taking dir only when it
// records none; an entry from another creation, or with no time, belonged to
// an earlier session and is replaced whole; a session with no entry gets one.
// A replacement is its own change, replace, saying what it dropped, so the log
// shows a label being discarded.
func (idx *Index) Claim(name, dir string, created time.Time) {
	fresh := Entry{Name: name, Dir: dir, CreatedAt: created}
	i := idx.indexOf(name)
	switch {
	case i < 0:
		idx.Sessions = append(idx.Sessions, fresh)
	case sameCreation(idx.Sessions[i].CreatedAt, created):
		if idx.Sessions[i].Dir != "" || dir == "" {
			return
		}
		idx.Sessions[i].Dir = dir
	case idx.Sessions[i] == fresh:
		return
	default:
		earlier := idx.Sessions[i]
		idx.Sessions[i] = fresh
		idx.changes = append(idx.changes, change{op: "replace", session: name, value: dir, detail: dropped(earlier)})
		return
	}
	idx.changes = append(idx.changes, change{op: "claim", session: name, value: dir})
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

// sameCreation reports whether an entry created at recorded is the session
// created at live. An entry with no time can't be told from an earlier one.
func sameCreation(recorded, live time.Time) bool {
	return !recorded.IsZero() && recorded.Equal(live)
}

// Remove drops the session's entry, reporting whether there was one.
func (idx *Index) Remove(name string) bool {
	i := idx.indexOf(name)
	if i < 0 {
		return false
	}
	idx.Sessions = slices.Delete(idx.Sessions, i, i+1)
	idx.changes = append(idx.changes, change{op: "rm", session: name})
	return true
}

func (idx *Index) indexOf(name string) int {
	return slices.IndexFunc(idx.Sessions, func(e Entry) bool { return e.Name == name })
}
