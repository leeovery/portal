package saver

import (
	"fmt"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
)

const (
	// dropAfter is how far apart two listings missing an entry's name must
	// be, with none showing it between them, before the entry is dropped.
	dropAfter = 10 * time.Second
	// vanishFloor is the fewest entries missing at once that can be a mass
	// vanish.
	vanishFloor = 3
)

// drops judges when the entry of a session missing from the listings is
// dropped.
type drops struct {
	// missing is when a listing first showed each judged entry's name
	// missing.
	missing map[string]time.Time
	// held is each entry a mass vanish keeps until it is listed again, by
	// name, as the index recorded it when held.
	held map[string]sessionindex.Entry
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
// a listing at least dropAfter before this one. It judges only the entries a
// drop may remove, and not those held: when so many of them are missing at
// once that it is a mass vanish, whether they went in this poll or in earlier
// ones still waiting, it holds every one of them instead.
func (d *drops) judge(idx *sessionindex.Index, l listing, rule sessionindex.Rule) []string {
	judged := d.judged(idx, l, rule)
	var gone []sessionindex.Entry
	for _, e := range judged {
		if !l.names[e.Name] {
			gone = append(gone, e)
		}
	}
	if vanished(len(gone), len(judged)) {
		d.hold(gone, len(judged))
		d.missing = map[string]time.Time{}
		return nil
	}
	missing := map[string]time.Time{}
	var due []string
	for _, e := range gone {
		since, waiting := d.missing[e.Name]
		if !waiting {
			since = l.at
		}
		missing[e.Name] = since
		if l.at.Sub(since) >= dropAfter {
			due = append(due, e.Name)
		}
	}
	d.missing = missing
	return due
}

// judged is every entry of idx a drop may remove that is not held. A held
// entry the index no longer records with the creation it was held at is let
// go.
func (d *drops) judged(idx *sessionindex.Index, l listing, rule sessionindex.Rule) []sessionindex.Entry {
	held := map[string]sessionindex.Entry{}
	var judged []sessionindex.Entry
	for _, e := range idx.Entries() {
		switch h, ok := d.held[e.Name]; {
		case ok && e.SameCreation(h.CreatedAt, h.PID):
			held[e.Name] = h
		case droppable(e, l, rule):
			judged = append(judged, e)
		}
	}
	d.held = held
	return judged
}

// droppable reports whether a drop may remove e: an entry recording a
// creation of this boot, older than the listing, of a session not Portal's
// own, owed no restore and with none unfinished. One that may predate the
// boot, is owed a restore, or whose restore its helper hasn't finished is kept
// for the next bootstrap to restore, and one created since the listing was
// taken is newer than it.
func droppable(e sessionindex.Entry, l listing, rule sessionindex.Rule) bool {
	return !backend.IsPortalOwned(e.Name) && !e.Owed && !rule.MayPredateBoot(e) && !rule.Hydrating(e.Name) &&
		e.CreatedAt.Before(l.at.Truncate(time.Second))
}

// vanished reports whether gone entries missing, of the of judged, are a mass
// vanish rather than sessions ending one by one: at least vanishFloor of them,
// and at least half. A zmx upgrade or a crash ends every session at once, and
// the next bootstrap restores them.
func vanished(gone, of int) bool {
	return gone >= vanishFloor && 2*gone >= of
}

func (d *drops) hold(gone []sessionindex.Entry, of int) {
	for _, e := range gone {
		d.held[e.Name] = e
	}
	logger.Warn("drops-held", "op", "drops-held", "sessions", len(gone),
		"detail", fmt.Sprintf("%d of the %d sessions this saver may drop are missing at once, so it drops none of them until each is listed again", len(gone), of),
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

func drop(idx *sessionindex.Index, names []string) {
	for _, name := range names {
		idx.Remove(name)
	}
}
