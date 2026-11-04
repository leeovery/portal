package saver

import (
	"fmt"
	"slices"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
)

const (
	// dropAfter is how far apart two listings missing an entry's name must
	// be, with none showing it between them, before the entry is dropped.
	dropAfter = 10 * time.Second
	// vanishFloor is the fewest entries going missing in one poll that can be
	// a mass vanish.
	vanishFloor = 3
)

// drops judges when the entry of a session missing from the listings is
// dropped.
type drops struct {
	// missing is when a listing first showed each droppable entry's name
	// missing.
	missing map[string]time.Time
	// held names the entries a mass vanish keeps until they are listed again.
	held map[string]bool
}

// seen takes each name the listing shows out of missing and held, even when
// the poll goes no further: a name listed is not missing, and an entry held
// that is listed again is judged as any other.
func (d *drops) seen(l listing) {
	for name := range l.names {
		delete(d.missing, name)
		delete(d.held, name)
	}
}

// judge returns the names of the entries of idx, the index as read under the
// commit lock, due to be dropped: those whose names have stayed missing since
// a listing at least dropAfter before this one. An entry newly missing starts
// its wait, unless so many went missing at once that it is a mass vanish,
// which holds them all instead. The entries of Portal's own sessions, named
// with a leading _, are neither judged nor counted.
func (d *drops) judge(idx *sessionindex.Index, l listing, rule sessionindex.Rule) []string {
	entries := userEntries(idx)
	d.keepHeld(entries)
	missing := map[string]time.Time{}
	var newly, due []string
	for _, e := range entries {
		if !d.droppable(e, l, rule) {
			continue
		}
		since, waiting := d.missing[e.Name]
		if !waiting {
			newly = append(newly, e.Name)
			continue
		}
		missing[e.Name] = since
		if l.at.Sub(since) >= dropAfter {
			due = append(due, e.Name)
		}
	}
	if vanished(len(newly), len(entries)) {
		d.hold(newly, len(entries))
	} else {
		for _, name := range newly {
			missing[name] = l.at
		}
	}
	d.missing = missing
	return due
}

// droppable reports whether the listing shows e's session ended: its name not
// listed, answering or not, and e not held, recording a creation of this boot
// from before the listing. An entry that may predate the boot is never
// dropped, for the next bootstrap to restore, and one created since the
// listing was taken is newer than it.
func (d *drops) droppable(e sessionindex.Entry, l listing, rule sessionindex.Rule) bool {
	return !l.names[e.Name] && !d.held[e.Name] && !rule.MayPredateBoot(e) && e.CreatedAt.Before(l.at.Truncate(time.Second))
}

// keepHeld lets go of each held name the index no longer holds.
func (d *drops) keepHeld(entries []sessionindex.Entry) {
	held := map[string]bool{}
	for _, e := range entries {
		if d.held[e.Name] {
			held[e.Name] = true
		}
	}
	d.held = held
}

// vanished reports whether newly missing entries, of the index's whole, are a
// mass vanish rather than sessions ending one by one: at least vanishFloor,
// and at least half the index. A zmx upgrade or a crash ends every session at
// once, and the next bootstrap restores them.
func vanished(newly, of int) bool {
	return newly >= vanishFloor && 2*newly >= of
}

func (d *drops) hold(names []string, of int) {
	for _, name := range names {
		d.held[name] = true
	}
	logger.Warn("drops-held", "op", "drops-held", "sessions", len(names),
		"detail", fmt.Sprintf("%d of the index's %d sessions went missing in one poll, so this saver drops none of them until each is listed again", len(names), of),
		"via", via)
}

// dropped logs each entry a landed write dropped, as of the listing at at,
// and ends its wait.
func (d *drops) dropped(removed []sessionindex.Entry, at time.Time) {
	for _, e := range removed {
		logger.Info("session dropped", "session", e.Name, "detail", fmt.Sprintf("missing from every listing for %s", at.Sub(d.missing[e.Name])), "via", via)
		delete(d.missing, e.Name)
	}
}

// drop removes the entries of sessions that ended.
func drop(idx *sessionindex.Index, names []string) {
	for _, name := range names {
		idx.Remove(name)
	}
}

// userEntries is every entry of idx but those of Portal's own sessions.
func userEntries(idx *sessionindex.Index) []sessionindex.Entry {
	return slices.DeleteFunc(idx.Entries(), func(e sessionindex.Entry) bool {
		return backend.Session{Name: e.Name}.PortalOwned()
	})
}
