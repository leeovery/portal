package sessionindex

import (
	"errors"
	"slices"
	"strings"
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

func printable(label string) bool {
	return utf8.ValidString(label) && !strings.ContainsFunc(label, func(r rune) bool {
		return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
	})
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
