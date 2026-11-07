package state

import (
	"slices"
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
// then the hooks lock. However many the entries, it waits on the hooks lock
// twice at most, a bound each: once reading hooks.json, which degrades to an
// unlocked read past the bound, and once removing every hook it read under one
// hold, which keeps them all when refused. Two bounds sit inside commit.lock's,
// which every other writer of the index waits for.
//
// A hook goes only while it holds the command Forget read, compared under the
// hooks lock. Each removal is tried whatever the others do. The entries are
// gone already, so a failure is logged and leaves the rest to the sweeps of
// stale state. A nil hookStore keeps every hook, for a caller that could not
// find hooks.json and has said so. via is the surface the index write came
// from: cli, tui or internal.
func Forget(dir string, hookStore *hooks.Store, via string, entries ...sessionindex.Entry) {
	forgets := make([]*forgetting, 0, len(entries))
	for _, entry := range entries {
		f := &forgetting{name: entry.Name, via: via}
		if err := checkFileName(entry.Name); err != nil {
			f.kept("files", err)
		} else {
			f.file("hydrating flag", Hydrating.path(dir, entry.Name))
			f.file("resume-pending flag", ResumePending.path(dir, entry.Name))
			f.scrollback(dir, entry)
		}
		forgets = append(forgets, f)
	}
	if hookStore != nil {
		forgetHooks(hookStore, hookVia(via), forgets)
	}
	for _, f := range forgets {
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

var (
	loadHooks    = (*hooks.Store).Load
	discardHooks = (*hooks.Store).DiscardEach
)

// forgetHooks removes the resume hooks of forgets' sessions. It reads before
// it removes, so a batch with no hook takes no lock and creates no sidecar,
// and the hooks store's discard breadcrumbs carry each removed command, to
// copy back out of the log.
func forgetHooks(store *hooks.Store, via hooks.Via, forgets []*forgetting) {
	held, err := loadHooks(store, via)
	if err != nil {
		for _, f := range forgets {
			f.kept("resume hook", err)
		}
		return
	}
	shown := map[string]string{}
	for _, f := range forgets {
		if registration, ok := held[f.name][hooks.EventOnResume.String()]; ok {
			shown[f.name] = registration.Command
		}
	}
	if len(shown) == 0 {
		return
	}
	removed, err := discardHooks(store, hooks.EventOnResume, shown, via)
	for _, f := range forgets {
		_, had := shown[f.name]
		switch {
		case slices.Contains(removed, f.name):
			f.gone = append(f.gone, "resume hook")
		case had && err != nil:
			f.kept("resume hook", err)
		}
	}
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
