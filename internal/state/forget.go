package state

import (
	"strings"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
)

// What Forget removes goes with an index entry, so it logs under the index's
// component.
var sessionsLogger = log.For("sessions")

// Forget removes what Portal keeps for the session name beside its index
// entry in the state directory dir: its hydrating and resume-pending flags,
// its scrollback file and its resume hook. Call it once the index write that
// removed the entry, or replaced it with another creation's, has landed. It
// takes the hooks lock but never commit.lock, so even run with commit.lock
// held the order is commit.lock, then the hooks lock.
//
// Each removal is tried whatever the others do. The entry is gone already, so
// a failure is logged and leaves the rest to the sweeps of stale state. A nil
// hookStore keeps the hook, for a caller that could not find hooks.json and
// has said so. via is the surface the index write came from.
func Forget(dir string, hookStore *hooks.Store, name, via string) {
	f := forgetting{name: name, via: via}
	if err := checkFileName(name); err != nil {
		f.kept("files", err)
	} else {
		f.file("hydrating flag", Hydrating.path(dir, name))
		f.file("resume-pending flag", ResumePending.path(dir, name))
		f.file("scrollback", sessionScrollbackPath(dir, name))
	}
	f.hook(hookStore)
	f.log()
}

type forgetting struct {
	name, via string
	gone      []string
	// hookCommand is the removed hook's command, logged so it can be copied
	// back out of the log.
	hookCommand string
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

// hook reads before it removes, so a session with no hook takes no lock and
// creates no sidecar.
func (f *forgetting) hook(store *hooks.Store) {
	if store == nil {
		return
	}
	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		f.kept("resume hook", err)
		return
	}
	registration, ok := held[f.name][hooks.EventOnResume.String()]
	if !ok {
		return
	}
	removed, err := store.Remove(f.name, hooks.EventOnResume, hooks.ViaInternal)
	if err != nil {
		f.kept("resume hook", err)
		return
	}
	if removed {
		f.gone = append(f.gone, "resume hook")
		f.hookCommand = registration.Command
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
	attrs := []any{"op", "forget", "session", f.name, "detail", "removed its " + strings.Join(f.gone, ", ")}
	if f.hookCommand != "" {
		attrs = append(attrs, "value", f.hookCommand)
	}
	sessionsLogger.Info("forget", append(attrs, "via", f.via)...)
}
