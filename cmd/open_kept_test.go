package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// keptExe is the hydrate helper a restored session runs in these tests. The
// Fake runs nothing, so it need not exist.
const keptExe = "/opt/portal/bin/portal"

// savedBeforeBoot is a creation before testBoot, so its entry is kept once
// no live session holds its name.
var savedBeforeBoot = time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC)

// fakeKept keeps the names it holds, recording each asked about.
type fakeKept struct {
	names []string
	asked []string
}

func (k *fakeKept) IsKept(name string) (bool, error) {
	k.asked = append(k.asked, name)
	return slices.Contains(k.names, name), nil
}

// keptOpen records what an open over seams did: the sessions it brought back,
// attached and minted.
type keptOpen struct {
	kept                      *fakeKept
	brought, attached, minted []string
}

// stageKeptOpen stages an open over live sessions, kept ones and aliases,
// each alias's directory existing, whose bring-back answers through bring.
func stageKeptOpen(t *testing.T, live, kept []string, aliases map[string]string, bring func(name string) (restore.Brought, error)) *keptOpen {
	t.Helper()
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})
	o := &keptOpen{kept: &fakeKept{names: kept}}
	existing := map[string]bool{}
	for _, dir := range aliases {
		existing[dir] = true
	}
	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: live},
		Kept:          o.kept,
		AliasLookup:   &testAliasLookup{aliases: aliases},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: existing},
	})
	withFuncSeam(t, &bringBackFunc, func(_ *cobra.Command, name string) (restore.Brought, error) {
		o.brought = append(o.brought, name)
		return bring(name)
	})
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
		o.attached = append(o.attached, name)
		return nil
	})
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, dir string, _ []string) error {
		o.minted = append(o.minted, dir)
		return nil
	})
	return o
}

func answer(brought restore.Brought, err error) func(string) (restore.Brought, error) {
	return func(string) (restore.Brought, error) { return brought, err }
}

func runOpen(args ...string) error {
	resetRootCmd()
	rootCmd.SetArgs(append([]string{"open"}, args...))
	return rootCmd.Execute()
}

// decision 78: x <name> brings back a saved session that isn't live, and
// -s/--session reaches it too.
func TestOpen_KeptSession(t *testing.T) {
	for form, target := range map[string][]string{"bare": {"work"}, "pinned": {"-s", "work"}} {
		t.Run(form+": it is brought back, then attached", func(t *testing.T) {
			o := stageKeptOpen(t, nil, []string{"work"}, nil, answer(restore.BroughtBack, nil))

			if err := runOpen(target...); err != nil {
				t.Fatalf("open: %v", err)
			}
			if !slices.Equal(o.brought, []string{"work"}) || !slices.Equal(o.attached, []string{"work"}) || len(o.minted) > 0 {
				t.Errorf("brought %q, attached %q, minted %q; want work brought back, then attached", o.brought, o.attached, o.minted)
			}
		})

		t.Run(form+": one live by the time the lock is held is attached", func(t *testing.T) {
			o := stageKeptOpen(t, nil, []string{"work"}, nil, answer(restore.AlreadyLive, nil))

			if err := runOpen(target...); err != nil {
				t.Fatalf("open: %v", err)
			}
			if !slices.Equal(o.attached, []string{"work"}) {
				t.Errorf("attached %q, want work", o.attached)
			}
		})

		t.Run(form+": a failure is one line, nothing attached", func(t *testing.T) {
			cause := errors.New("zmx attach: refused")
			o := stageKeptOpen(t, nil, []string{"work"}, nil, answer(0, cause))

			err := runOpen(target...)

			if want := "Portal couldn't restore work — kept for the next restore: zmx attach: refused"; err == nil || err.Error() != want || !errors.Is(err, cause) {
				t.Errorf("open = %v, want %q carrying the cause", err, want)
			}
			if len(o.attached) > 0 {
				t.Errorf("attached %q, want nothing", o.attached)
			}
		})

		t.Run(form+": a command is refused before anything is brought back", func(t *testing.T) {
			o := stageKeptOpen(t, nil, []string{"work"}, nil, answer(restore.BroughtBack, nil))

			err := runOpen(append(slices.Clone(target), "--", "make")...)

			if _, usage := errors.AsType[*UsageError](err); !usage || err.Error() != commandAttachOnlyMessage {
				t.Errorf("open = %v, want the attach-only usage error", err)
			}
			if len(o.brought) > 0 {
				t.Errorf("brought %q back, want nothing", o.brought)
			}
		})
	}

	// Create accepts ?, [ and a * anywhere but the end, so a kept name can
	// read as a glob, which expands over live sessions alone.
	for form, target := range map[string][]string{"bare": {"a?b"}, "pinned": {"-s", "a?b"}} {
		t.Run(form+": a kept name with glob characters is brought back, not expanded", func(t *testing.T) {
			o := stageKeptOpen(t, []string{"axb"}, []string{"a?b"}, nil, answer(restore.BroughtBack, nil))
			withFuncSeam(t, &openRawArgs, func() []string { return append([]string{"portal", "open"}, target...) })

			if err := runOpen(target...); err != nil {
				t.Fatalf("open: %v", err)
			}
			if !slices.Equal(o.brought, []string{"a?b"}) || !slices.Equal(o.attached, []string{"a?b"}) {
				t.Errorf("brought %q, attached %q; want a?b brought back, then attached", o.brought, o.attached)
			}
		})

		t.Run(form+": a name with glob characters kept no longer is read as the glob", func(t *testing.T) {
			o := stageKeptOpen(t, []string{"axb"}, []string{"a?b"}, nil, answer(restore.NoLongerKept, nil))
			withFuncSeam(t, &openRawArgs, func() []string { return append([]string{"portal", "open"}, target...) })

			if err := runOpen(target...); err != nil {
				t.Fatalf("open: %v", err)
			}
			if !slices.Equal(o.attached, []string{"axb"}) {
				t.Errorf("attached %q, want axb, the glob's one live match", o.attached)
			}
		})

		t.Run(form+": a name with glob characters kept no longer, matching nothing live, misses as the glob", func(t *testing.T) {
			stageKeptOpen(t, nil, []string{"a?b"}, nil, answer(restore.NoLongerKept, nil))
			withFuncSeam(t, &openRawArgs, func() []string { return append([]string{"portal", "open"}, target...) })

			if err := runOpen(target...); err == nil || err.Error() != singleMissError("a?b").Error() {
				t.Errorf("open = %v, want the glob's miss", err)
			}
		})
	}

	// A burst's window runs open --session <name> --ack <batch>:<token>, one
	// surface named exactly: read as a glob, the name could burst again.
	t.Run("a spawned window's name with glob characters is that session alone, never a glob", func(t *testing.T) {
		args := []string{"--session", "a?b", "--ack", "b1:t1"}
		o := stageKeptOpen(t, []string{"a?b", "axb"}, nil, nil, answer(restore.BroughtBack, nil))
		withFuncSeam(t, &openRawArgs, func() []string { return append([]string{"portal", "open"}, args...) })
		withFuncSeam(t, &runOpenBurstFunc, func(*cobra.Command, []spawn.Surface, resolver.QueryResult, []string) error {
			t.Error("a spawned window burst again")
			return nil
		})

		if err := runOpen(args...); err != nil {
			t.Fatalf("open: %v", err)
		}
		if !slices.Equal(o.attached, []string{"a?b"}) {
			t.Errorf("attached %q, want a?b alone", o.attached)
		}
	})

	t.Run("a spawned window's kept name, kept no longer, is no session, never the glob", func(t *testing.T) {
		args := []string{"--session", "a?b", "--ack", "b1:t1"}
		o := stageKeptOpen(t, []string{"axb"}, []string{"a?b"}, nil, answer(restore.NoLongerKept, nil))
		withFuncSeam(t, &openRawArgs, func() []string { return append([]string{"portal", "open"}, args...) })

		if err := runOpen(args...); err == nil || err.Error() != "No session found: a?b" {
			t.Errorf("open = %v, want No session found: a?b", err)
		}
		if len(o.attached) > 0 {
			t.Errorf("attached %q, want nothing", o.attached)
		}
	})

	t.Run("a live session of the name is attached, nothing brought back", func(t *testing.T) {
		o := stageKeptOpen(t, []string{"work"}, []string{"work"}, nil, answer(restore.BroughtBack, nil))

		if err := runOpen("work"); err != nil {
			t.Fatalf("open: %v", err)
		}
		if len(o.brought) > 0 || !slices.Equal(o.attached, []string{"work"}) {
			t.Errorf("brought %q, attached %q; want work attached as it is", o.brought, o.attached)
		}
	})

	t.Run("a bare name kept before its alias resolves no further", func(t *testing.T) {
		dir := t.TempDir()
		o := stageKeptOpen(t, nil, []string{"work"}, map[string]string{"work": dir}, answer(restore.BroughtBack, nil))

		if err := runOpen("work"); err != nil {
			t.Fatalf("open: %v", err)
		}
		if len(o.minted) > 0 || !slices.Equal(o.attached, []string{"work"}) {
			t.Errorf("minted %q, attached %q; want the kept session, not the alias", o.minted, o.attached)
		}
	})

	t.Run("a bare name kept no longer resolves on as if never kept", func(t *testing.T) {
		dir := t.TempDir()
		o := stageKeptOpen(t, nil, []string{"work"}, map[string]string{"work": dir}, answer(restore.NoLongerKept, nil))

		if err := runOpen("work"); err != nil {
			t.Fatalf("open: %v", err)
		}
		if !slices.Equal(o.minted, []string{dir}) || len(o.attached) > 0 {
			t.Errorf("minted %q, attached %q; want a mint at the alias's %s", o.minted, o.attached, dir)
		}
		if !slices.Equal(o.kept.asked, []string{"work"}) {
			t.Errorf("asked whether kept %q, want once: resolving again keeps nothing", o.kept.asked)
		}
	})

	t.Run("a bare name kept no longer, and nothing else, misses", func(t *testing.T) {
		stageKeptOpen(t, nil, []string{"work"}, nil, answer(restore.NoLongerKept, nil))

		if err := runOpen("work"); err == nil || err.Error() != singleMissError("work").Error() {
			t.Errorf("open = %v, want the miss", err)
		}
	})

	t.Run("a pinned name kept no longer is no session", func(t *testing.T) {
		stageKeptOpen(t, nil, []string{"work"}, map[string]string{"work": t.TempDir()}, answer(restore.NoLongerKept, nil))

		if err := runOpen("-s", "work"); err == nil || err.Error() != "No session found: work" {
			t.Errorf("open = %v, want No session found: work", err)
		}
	})

	t.Run("a bare kept name logs its decision in the kept domain, and resolving again its own", func(t *testing.T) {
		dir := t.TempDir()
		stageKeptOpen(t, nil, []string{"work"}, map[string]string{"work": dir}, answer(restore.NoLongerKept, nil))
		sink := logtest.Install(t)

		if err := runOpen("work"); err != nil {
			t.Fatalf("open: %v", err)
		}

		recs := sink.Records().Matching("resolve", "resolved")
		if len(recs) != 2 {
			t.Fatalf("resolve records = %d, want the kept decision, then the alias's", len(recs))
		}
		for i, want := range [][2]string{{"kept", "work"}, {"alias", dir}} {
			if got := [2]string{recs[i].AttrString(t, "domain"), recs[i].AttrString(t, "resolved_path")}; got != want {
				t.Errorf("decision %d = %q, want %q", i, got, want)
			}
		}
	})
}

// decision 78: a kept name among several targets opens in a window of its
// own, whose open --session brings it back, as the trigger's connect does.
func TestOpenBurst_KeptSession(t *testing.T) {
	t.Run("pre-flight brings nothing back, giving a kept name an attach surface, and the trigger's verdict", func(t *testing.T) {
		o := stageKeptOpen(t, []string{"api"}, []string{"work"}, nil, answer(restore.BroughtBack, nil))
		withFuncSeam(t, &openRawArgs, func() []string { return []string{"portal", "open", "work", "-s", "api"} })
		var surfaces []spawn.Surface
		var trigger resolver.QueryResult
		withFuncSeam(t, &runOpenBurstFunc, func(_ *cobra.Command, s []spawn.Surface, tr resolver.QueryResult, _ []string) error {
			surfaces, trigger = s, tr
			return nil
		})

		if err := runOpen("work", "-s", "api"); err != nil {
			t.Fatalf("open: %v", err)
		}
		want := []spawn.Surface{{Kind: spawn.SurfaceAttach, Value: "work"}, {Kind: spawn.SurfaceAttach, Value: "api"}}
		if !slices.Equal(surfaces, want) {
			t.Errorf("surfaces = %+v, want %+v", surfaces, want)
		}
		if kept, ok := trigger.(*resolver.KeptResult); !ok || kept.Name != "work" {
			t.Errorf("trigger = %#v, want work judged kept", trigger)
		}
		if len(o.brought) > 0 {
			t.Errorf("brought %q back during pre-flight, want nothing", o.brought)
		}
	})

	t.Run("a kept pinned name among several is an attach surface too", func(t *testing.T) {
		stageKeptOpen(t, []string{"api"}, []string{"work"}, nil, answer(restore.BroughtBack, nil))
		withFuncSeam(t, &openRawArgs, func() []string { return []string{"portal", "open", "-s", "work", "api"} })
		var surfaces []spawn.Surface
		withFuncSeam(t, &runOpenBurstFunc, func(_ *cobra.Command, s []spawn.Surface, _ resolver.QueryResult, _ []string) error {
			surfaces = s
			return nil
		})

		if err := runOpen("-s", "work", "api"); err != nil {
			t.Fatalf("open: %v", err)
		}
		if want := []spawn.Surface{{Kind: spawn.SurfaceAttach, Value: "work"}, {Kind: spawn.SurfaceAttach, Value: "api"}}; !slices.Equal(surfaces, want) {
			t.Errorf("surfaces = %+v, want %+v", surfaces, want)
		}
	})

	t.Run("the trigger's connect brings a kept session back, then attaches it", func(t *testing.T) {
		o := stageKeptOpen(t, nil, []string{"work"}, nil, answer(restore.BroughtBack, nil))

		connectTriggerJudged(t, &resolver.KeptResult{Name: "work"}, "work")

		if !slices.Equal(o.brought, []string{"work"}) || !slices.Equal(o.attached, []string{"work"}) {
			t.Errorf("brought %q, attached %q; want work brought back, then attached", o.brought, o.attached)
		}
	})

	t.Run("the trigger's connect attaches a live session as it is, resolving nothing again", func(t *testing.T) {
		o := stageKeptOpen(t, nil, nil, nil, answer(restore.BroughtBack, nil))
		withOpenDeps(t, OpenDeps{SessionLister: &testSessionLister{err: errors.New("list: refused")}, Kept: o.kept})

		connectTriggerJudged(t, &resolver.SessionResult{Name: "api", Domain: resolver.DomainSession}, "api")

		if !slices.Equal(o.attached, []string{"api"}) || len(o.brought) > 0 || len(o.kept.asked) > 0 {
			t.Errorf("attached %q, brought %q, asked whether kept %q; want api attached, nothing more", o.attached, o.brought, o.kept.asked)
		}
	})
}

// connectTriggerJudged connects the burst's trigger, name, as the pre-flight
// judged it, trigger.
func connectTriggerJudged(t *testing.T, trigger resolver.QueryResult, name string) {
	t.Helper()
	isolateTerminalsFile(t)
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
	withOpenBurstDeps(t, OpenBurstDeps{Logger: log.Discard()})
	deps, err := buildOpenBurstDeps(contextCommand(), trigger)
	if err != nil {
		t.Fatalf("buildOpenBurstDeps: %v", err)
	}
	if err := deps.Connector.Connect(name); err != nil {
		t.Fatalf("Connect: %v", err)
	}
}

// saveIndex writes sessions.json holding entries in the state directory dir.
func saveIndex(t *testing.T, dir string, entries ...sessionindex.Entry) {
	t.Helper()
	writeSessionsJSON(t, dir, indexJSON(t, entries))
}

func TestKeptSessions(t *testing.T) {
	t.Run("it judges by restore's own rule", func(t *testing.T) {
		dir := isolateStateDir(t)
		fake := backendtest.New(backend.Session{Name: "live"}, backend.Session{Name: "Spelt"})
		withBackendDeps(t, BackendDeps{Backend: fake})
		thisBoot := testBoot.Add(time.Hour)
		saveIndex(t, dir,
			sessionindex.Entry{Name: "saved", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "owed", CreatedAt: thisBoot, Owed: true},
			sessionindex.Entry{Name: "ended", CreatedAt: thisBoot},
			sessionindex.Entry{Name: "live", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "spelt", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "_portal-saver", CreatedAt: savedBeforeBoot},
			sessionindex.Entry{Name: "logs", CreatedAt: savedBeforeBoot},
		)

		for name, want := range map[string]bool{"saved": true, "owed": true, "ended": false, "live": false, "spelt": false, "_portal-saver": false, "logs": false, "never-saved": false} {
			if got, err := (keptSessions{ctx: t.Context()}).IsKept(name); got != want || err != nil {
				t.Errorf("IsKept(%s) = %v, %v; want %v", name, got, err, want)
			}
		}
	})

	t.Run("a name never saved costs no listing", func(t *testing.T) {
		dir := isolateStateDir(t)
		fake := backendtest.New()
		withBackendDeps(t, BackendDeps{Backend: fake})
		saveIndex(t, dir, sessionindex.Entry{Name: "saved", CreatedAt: savedBeforeBoot})

		if kept, err := (keptSessions{ctx: t.Context()}).IsKept("work"); kept || err != nil {
			t.Errorf("IsKept(work) = %v, %v; want false", kept, err)
		}
		if calls := fake.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v, want none", calls)
		}
	})

	// zmx.New refuses a relative directory, so every bind fails: a name that
	// reached the bind would log its failure.
	t.Run("a name Create refuses costs nothing: no backend bound, and an index it can't read unlogged", func(t *testing.T) {
		blockStateDir(t)
		t.Setenv("ZMX_DIR", "relative/zmx")
		sink := logtest.Install(t)

		for _, name := range []string{"~/code", "logs"} {
			if kept, err := (keptSessions{ctx: t.Context()}).IsKept(name); kept || err != nil {
				t.Errorf("IsKept(%s) = %v, %v; want false", name, kept, err)
			}
		}
		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("records = %v, want none", recs)
		}
		if _, err := sessionBackend(); err == nil {
			t.Error("a backend was bound: the test proves nothing unless the bind fails")
		}
	})

	t.Run("an index it can't read keeps nothing, and says so in the log", func(t *testing.T) {
		blockStateDir(t)
		withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
		sink := logtest.Install(t)

		if kept, err := (keptSessions{ctx: t.Context()}).IsKept("work"); kept || err == nil {
			t.Errorf("IsKept(work) = %v, %v; want false and the read's failure", kept, err)
		}
		rec := sink.Records().Matching("restore", "sessions.json unreadable").AtExactLevel(slog.LevelWarn).Only(t, "the unread index")
		if got := rec.AttrString(t, "session"); got != "work" {
			t.Errorf("warning names %q, want work", got)
		}
	})

	t.Run("sessions it can't list keep nothing, and say so in the log", func(t *testing.T) {
		dir := isolateStateDir(t)
		fake := backendtest.New()
		fake.Fail(backendtest.OpList, errors.New("list: refused"))
		withBackendDeps(t, BackendDeps{Backend: fake})
		saveIndex(t, dir, sessionindex.Entry{Name: "work", CreatedAt: savedBeforeBoot})
		sink := logtest.Install(t)

		if kept, err := (keptSessions{ctx: t.Context()}).IsKept("work"); kept || err == nil {
			t.Errorf("IsKept(work) = %v, %v; want false and the listing's failure", kept, err)
		}
		sink.Records().Matching("restore", "list sessions failed").AtExactLevel(slog.LevelWarn).Only(t, "the failed listing")
	})

	t.Run("an index it can't read lets the chain resolve on", func(t *testing.T) {
		blockStateDir(t)
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})
		dir := t.TempDir()
		withOpenDeps(t, OpenDeps{
			SessionLister: &testSessionLister{},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{"work": dir}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{dir: true}},
		})
		var minted []string
		withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, d string, _ []string) error {
			minted = append(minted, d)
			return nil
		})

		if err := runOpen("work"); err != nil {
			t.Fatalf("open: %v", err)
		}
		if !slices.Equal(minted, []string{dir}) {
			t.Errorf("minted %q, want the alias's %s", minted, dir)
		}
	})
}

// keptWorld is open's production bring-back over a Fake: bootstrap.lock in a
// state directory of the test's own, restore's own path, and an attach that
// records. sessions.json holds work, saved before the boot.
type keptWorld struct {
	fake  *backendtest.Fake
	rec   *attachRecorder
	dir   string
	saved sessionindex.Entry
}

func newKeptWorld(t *testing.T) *keptWorld {
	t.Helper()
	fake, rec := stageAttach(t)
	w := &keptWorld{fake: fake, rec: rec, dir: isolateStateDir(t)}
	withOpenDeps(t, OpenDeps{
		Execer:       rec,
		Runner:       rec.Run,
		Runners:      &rec.runners,
		AliasLookup:  &testAliasLookup{aliases: map[string]string{}},
		Zoxide:       &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator: &testDirValidator{existing: map[string]bool{}},
	})
	withFuncSeam(t, &bootstrapExe, func() (string, error) { return keptExe, nil })
	w.saved = sessionindex.Entry{Name: "work", Label: "work label", Dir: t.TempDir(), CreatedAt: savedBeforeBoot, PID: 42}
	saveIndex(t, w.dir, w.saved)
	return w
}

func (w *keptWorld) entry(t *testing.T) sessionindex.Entry {
	t.Helper()
	idx, err := sessionindex.Read(w.dir)
	if err != nil {
		t.Fatalf("read the index: %v", err)
	}
	e, ok := idx.Lookup("work")
	if !ok {
		t.Fatal("work's entry is gone")
	}
	return e
}

// assertKeptAsSaved checks work's entry is as it was saved, and still kept.
func (w *keptWorld) assertKeptAsSaved(t *testing.T) {
	t.Helper()
	if e := w.entry(t); !e.CreatedAt.Equal(w.saved.CreatedAt) || e.PID != w.saved.PID || e.Owed || e.Label != w.saved.Label {
		t.Errorf("entry = %+v, want it as saved, %+v", e, w.saved)
	}
	if kept, err := (keptSessions{ctx: t.Context()}).IsKept("work"); !kept || err != nil {
		t.Errorf("IsKept(work) = %v, %v; want it still kept", kept, err)
	}
}

// lockCheckedBackend is a backend whose Create fails the test unless
// bootstrap.lock in dir is held as it runs.
type lockCheckedBackend struct {
	backend.Backend
	t   *testing.T
	dir string
}

func (b lockCheckedBackend) Create(ctx context.Context, name, dir string, command []string, env ...string) (backend.Session, error) {
	release, err := state.AcquireBootstrapLock(ctx, b.dir, 0)
	switch {
	case err == nil:
		release()
		b.t.Errorf("Create(%s) ran with bootstrap.lock free, want it held", name)
	case !errors.Is(err, state.ErrBootstrapLockHeld):
		b.t.Errorf("take bootstrap.lock during Create(%s): %v", name, err)
	}
	return b.Backend.Create(ctx, name, dir, command, env...)
}

// syncBuffer is a buffer a command writes from one goroutine while the test
// reads it from another.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestBringBack_UnderTheLock(t *testing.T) {
	t.Run("it makes the session through restore's own path, holding bootstrap.lock, then attaches it", func(t *testing.T) {
		w := newKeptWorld(t)
		withBackendDeps(t, BackendDeps{Backend: lockCheckedBackend{Backend: envBackend{Fake: w.fake, env: attachEnv}, t: t, dir: w.dir}})

		if err := runOpen("work"); err != nil {
			t.Fatalf("open: %v", err)
		}

		creates := w.fake.CallsTo(backendtest.OpCreate)
		if want := []string{keptExe, "state", "hydrate", "--session", "work"}; len(creates) != 1 || !slices.Equal(creates[0].Command, want) || creates[0].Dir != w.saved.Dir {
			t.Fatalf("Create calls = %+v, want one running %q in %s", creates, want, w.saved.Dir)
		}
		if got := w.rec.onlyRun(t).Session; got != "work" {
			t.Errorf("attached %q, want work", got)
		}
		live, _ := w.fake.Held("work")
		if e := w.entry(t); !e.SameCreation(live.Created, live.PID) || !e.Owed || e.Label != w.saved.Label {
			t.Errorf("entry = %+v, want the new creation recorded, owed until its helper finishes, its label kept", e)
		}
		if up, err := state.Hydrating.Exists(w.dir, "work"); !up || err != nil {
			t.Errorf("hydrating flag up = %v, %v; want it raised for the helper to lower", up, err)
		}
		if up, err := state.Restoring.Exists(w.dir); up || err != nil {
			t.Errorf("restoring flag up = %v, %v; want it never raised for one session", up, err)
		}
	})

	t.Run("it waits for the lock, says so, then judges again: a session live by then is attached, nothing made", func(t *testing.T) {
		w := newKeptWorld(t)
		release, err := state.AcquireBootstrapLock(t.Context(), w.dir, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		stderr := &syncBuffer{}
		resetRootCmd()
		rootCmd.SetErr(stderr)
		rootCmd.SetArgs([]string{"open", "work"})
		done := make(chan error, 1)
		go func() { done <- rootCmd.Execute() }()

		const notice = "Waiting for another Portal to finish starting…\n"
		if !harnesstest.PollUntil(t, 10*time.Second, 20*time.Millisecond, func() bool { return strings.Contains(stderr.String(), notice) }) {
			release()
			t.Fatalf("stderr = %q, want the wait's notice; open returned %v", stderr.String(), <-done)
		}
		w.fake.Add(backend.Session{Name: "work"})
		release()

		if err := <-done; err != nil {
			t.Fatalf("open: %v", err)
		}
		if creates := w.fake.CallsTo(backendtest.OpCreate); len(creates) != 0 {
			t.Errorf("Create calls = %+v, want none: a bootstrap restored it while open waited", creates)
		}
		if got := w.rec.onlyRun(t).Session; got != "work" {
			t.Errorf("attached %q, want work", got)
		}
	})

	t.Run("a failure is one line, the entry left kept", func(t *testing.T) {
		w := newKeptWorld(t)
		w.fake.Fail(backendtest.OpCreate, errors.New("zmx attach: refused"))

		err := runOpen("work")

		if want := "Portal couldn't restore work — kept for the next restore: "; err == nil || !strings.HasPrefix(err.Error(), want) || strings.Contains(err.Error(), "\n") {
			t.Errorf("open = %v, want one line starting %q", err, want)
		}
		w.rec.assertNone(t)
		w.assertKeptAsSaved(t)
	})

	t.Run("a refusal of a session that speaks another protocol is the unsupported-create fatal", func(t *testing.T) {
		w := newKeptWorld(t)
		w.fake.SpeakOtherProtocol()
		var stderr bytes.Buffer
		withFuncSeam[io.Writer](t, &fatalErrorStderr, &stderr)

		resetRootCmd()
		rootCmd.SetArgs([]string{"open", "work"})
		err := Execute()

		fatal, ok := errors.AsType[*bootstrap.FatalError](err)
		if !ok || fatal.UserMessage != bootstrap.UnsupportedCreateMessage || !errors.Is(err, backend.ErrUnsupportedProtocol) {
			t.Fatalf("Execute = %v, want the unsupported-create fatal carrying the refusal", err)
		}
		if got, want := stderr.String(), bootstrap.UnsupportedCreateMessage+"\n"; got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
		w.rec.assertNone(t)
		w.assertKeptAsSaved(t)
	})
}
