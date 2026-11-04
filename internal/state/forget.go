package state

import (
	"errors"
	"strings"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/sessionindex"
)

// What Forget removes goes with an index entry, so it logs under the index's
// component.
var sessionsLogger = log.For("sessions")

// Forget removes what Portal keeps beside each of entries, index entries as
// they were before a write dropped them, in the state directory dir: its
// session's hydrating and resume-pending flags, the scrollback file the entry
// names, and the session's resume hook. Run it under the commit lock the write
// that removed the entries, or replaced them with another creation's, held, so
// no claim, and so no hook set, lands between the write and the forgetting.
//
// It takes the hooks lock but never commit.lock, so the order is commit.lock,
// then the hooks lock. Whatever the entries, it waits on the hooks lock at
// most twice: once reading hooks.json, which degrades to an unlocked read past
// the bound, and once for a removal the lock refuses, after which it keeps
// every hook left. Two hooks-lock bounds sit inside commit.lock's, which every
// other writer of the index waits for.
//
// A hook goes only while it holds the command Forget read, compared under the
// hooks lock, so one set again between the read and the removal stays. Each
// removal is tried whatever the others do. The entries are gone already, so a
// failure is logged and leaves the rest to the sweeps of stale state. A nil
// hookStore keeps every hook, for a caller that could not find hooks.json and
// has said so. via is the surface the index write came from: cli, tui or
// internal.
func Forget(dir string, hookStore *hooks.Store, via string, entries ...sessionindex.Entry) {
	resumeHooks := &hookRemover{store: hookStore, via: hookVia(via)}
	for _, entry := range entries {
		f := forgetting{name: entry.Name, via: via}
		if err := checkFileName(entry.Name); err != nil {
			f.kept("files", err)
		} else {
			f.file("hydrating flag", Hydrating.path(dir, entry.Name))
			f.file("resume-pending flag", ResumePending.path(dir, entry.Name))
			f.scrollback(dir, entry)
		}
		f.hook(resumeHooks)
		f.log()
	}
}

// hookVia is the hooks store's name for an index write's surface, its zero
// Via, logged as absent, for a surface it has none for.
func hookVia(via string) hooks.Via {
	switch via {
	case "cli":
		return hooks.ViaCLI
	case "tui":
		return hooks.ViaTUI
	case "internal":
		return hooks.ViaInternal
	}
	return 0
}

var loadHooks = (*hooks.Store).Load

// hookRemover removes the resume hooks of one Forget's entries. It reads
// hooks.json once, before it removes, so entries with no hook take no lock
// and create no sidecar.
type hookRemover struct {
	store *hooks.Store
	via   hooks.Via
	read  bool
	held  hooks.Snapshot
	// stopped is why it removes no more hooks: a read that failed, or a
	// removal the lock refused.
	stopped error
}

// remove removes name's resume hook, reporting whether it did. The hooks
// store's discard breadcrumb carries the removed command, to copy back out of
// the log.
func (r *hookRemover) remove(name string) (bool, error) {
	if r.stopped != nil {
		return false, r.stopped
	}
	if !r.read {
		r.read = true
		if r.held, r.stopped = loadHooks(r.store, r.via); r.stopped != nil {
			return false, r.stopped
		}
	}
	registration, ok := r.held[name][hooks.EventOnResume.String()]
	if !ok {
		return false, nil
	}
	removed, err := r.store.Discard(name, hooks.EventOnResume, registration.Command, r.via)
	if errors.Is(err, hooks.ErrLockHeld) {
		r.stopped = err
	}
	return removed, err
}

type forgetting struct {
	name, via string
	gone      []string
}

func (f *forgetting) file(what, path string) {
	removed, err := removeIfPresent(path)
	if err != nil {
		f.kept(what, err, "path", path)
		return
	}
	if removed {
		f.gone = append(f.gone, what)
	}
}

func (f *forgetting) scrollback(dir string, e sessionindex.Entry) {
	if e.ScrollbackFile == "" {
		return
	}
	path, err := ScrollbackPath(dir, e)
	if err != nil {
		f.kept("scrollback", err, "path", e.ScrollbackFile)
		return
	}
	f.file("scrollback", path)
}

func (f *forgetting) hook(r *hookRemover) {
	if r.store == nil {
		return
	}
	removed, err := r.remove(f.name)
	if err != nil {
		f.kept("resume hook", err)
		return
	}
	if removed {
		f.gone = append(f.gone, "resume hook")
	}
}

func (f *forgetting) kept(what string, err error, attrs ...any) {
	attrs = append([]any{"op", "forget", "session", f.name, "detail", "kept its " + what}, attrs...)
	sessionsLogger.Warn("forget", append(attrs, "error", err, "via", f.via)...)
}

func (f *forgetting) log() {
	if len(f.gone) == 0 {
		return
	}
	sessionsLogger.Info("forget", "op", "forget", "session", f.name, "detail", "removed its "+strings.Join(f.gone, ", "), "via", f.via)
}
