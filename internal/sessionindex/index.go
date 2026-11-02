package sessionindex

import (
	"errors"
	"slices"
	"strings"
	"unicode"
)

// ErrLabelControlChar refuses a label holding a control character: a newline,
// tab or escape would break the picker row and the footers that print it.
var ErrLabelControlChar = errors.New("session label contains a control character")

type change struct {
	op      string
	session string
	value   string
}

// Lookup returns the entry named name, which the caller may modify in place.
func (idx *Index) Lookup(name string) (*Entry, bool) {
	i := idx.indexOf(name)
	if i < 0 {
		return nil, false
	}
	return &idx.Sessions[i], true
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
// session with no entry gets a bare one. A label still holding a control
// character once trimmed is refused with ErrLabelControlChar and changes
// nothing.
func (idx *Index) SetLabel(name, label string) error {
	label = strings.TrimSpace(label)
	if strings.ContainsFunc(label, unicode.IsControl) {
		return ErrLabelControlChar
	}
	if label == name {
		label = ""
	}
	e, ok := idx.Lookup(name)
	if !ok {
		idx.Sessions = append(idx.Sessions, Entry{Name: name})
		e = &idx.Sessions[len(idx.Sessions)-1]
	}
	if e.Label == label {
		return nil
	}
	e.Label = label
	if label == "" {
		idx.changes = append(idx.changes, change{op: "clear-label", session: name})
	} else {
		idx.changes = append(idx.changes, change{op: "set-label", session: name, value: label})
	}
	return nil
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
