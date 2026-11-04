package sessionindex

import (
	"errors"
	"log/slog"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrLabelUnprintable refuses a label that is not valid UTF-8 or holds a
// control, format or line/paragraph-separator character: a newline, tab,
// escape or bidi override would break or disguise the picker row and the
// footers that print it. Its text is the refusal every surface shows.
var ErrLabelUnprintable = errors.New("label refused: it holds a control or formatting character, such as a newline, tab or escape")

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
		i = idx.add(Entry{Name: name})
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
		return r != '‍' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp))
	})
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
	idx.changes = append(idx.changes, change{op: "rm", session: name})
	return true
}

// add appends e, an entry made afresh rather than read, and returns its
// position.
func (idx *Index) add(e Entry) int {
	idx.markFresh(e.Name)
	idx.Sessions = append(idx.Sessions, e)
	return len(idx.Sessions) - 1
}

// markFresh records that name's entry was made afresh rather than read, so
// Update reports an entry it read under that name as dropped.
func (idx *Index) markFresh(name string) {
	if idx.fresh == nil {
		idx.fresh = map[string]bool{}
	}
	idx.fresh[name] = true
}

func (idx *Index) indexOf(name string) int {
	return slices.IndexFunc(idx.Sessions, func(e Entry) bool { return e.Name == name })
}
