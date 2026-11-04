package state

import (
	"strings"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/sessionindex"
)

// What Forget removes goes with an index entry, so it logs under the index's
// component.
var sessionsLogger = log.For("sessions")

// Forget removes what Portal keeps beside entry, an index entry as it was
// before a write dropped it, in the state directory dir: its session's
// hydrating and resume-pending flags, the scrollback file the entry names, and
// the session's resume hook. Call it once the index write that removed the
// entry, or replaced it with another creation's, has landed. It takes the
// hooks lock but never commit.lock, so even run with commit.lock held the
// order is commit.lock, then the hooks lock.
//
// The hook goes only while it holds the command Forget read, compared under
// the hooks lock, so one set again between the read and the removal stays.
// Each removal is tried whatever the others do. The entry is gone already, so
// a failure is logged and leaves the rest to the sweeps of stale state. A nil
// hookStore keeps the hook, for a caller that could not find hooks.json and
// has said so. via is the surface the index write came from: cli, tui or
// internal.
func Forget(dir string, hookStore *hooks.Store, entry sessionindex.Entry, via string) {
	f := forgetting{name: entry.Name, via: via}
	if err := checkFileName(entry.Name); err != nil {
		f.kept("files", err)
	} else {
		f.file("hydrating flag", Hydrating.path(dir, entry.Name))
		f.file("resume-pending flag", ResumePending.path(dir, entry.Name))
		f.scrollback(dir, entry)
	}
	f.hook(hookStore)
	f.log()
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

// hook reads before it removes, so a session with no hook takes no lock and
// creates no sidecar. The hooks store's discard breadcrumb carries the
// removed command, to copy back out of the log.
func (f *forgetting) hook(store *hooks.Store) {
	if store == nil {
		return
	}
	via := hookVia(f.via)
	held, err := loadHooks(store, via)
	if err != nil {
		f.kept("resume hook", err)
		return
	}
	registration, ok := held[f.name][hooks.EventOnResume.String()]
	if !ok {
		return
	}
	removed, err := store.Discard(f.name, hooks.EventOnResume, registration.Command, via)
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
