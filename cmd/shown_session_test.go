package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/tui"
)

// fakePickerLister serves the picker these sessions through the lister
// production wires, over a Fake.
func fakePickerLister(sessions ...backend.Session) tui.SessionLister {
	return pickerLister{ctx: context.Background(), b: backendtest.New(sessions...)}
}

// labelSession labels s in the index as Portal does, for s's own creation.
func labelSession(t *testing.T, stateDir string, s backend.Session, label string) {
	t.Helper()
	if _, err := sessionindex.Update(stateDir, "cli", func(idx *sessionindex.Index) error {
		idx.Claim(s.Name, "", s.Created, s.PID, claimRule(stateDir))
		return idx.SetLabel(s.Name, label)
	}); err != nil {
		t.Fatalf("label %s as %q: %v", s.Name, label, err)
	}
}

// storedLabel writes a label onto an entry recording no creation, for tests
// of what the index stores rather than what a session shows: such an entry is
// any live creation's to take over, so every surface shows it.
func storedLabel(t *testing.T, stateDir, name, label string) {
	t.Helper()
	if _, err := sessionindex.Update(stateDir, "cli", func(idx *sessionindex.Index) error {
		return idx.SetLabel(name, label)
	}); err != nil {
		t.Fatalf("label %s as %q: %v", name, label, err)
	}
}

// seedIndex stages a sessions.json holding entries verbatim, unprintable
// labels included, in a state directory of the test's own.
func seedIndex(t *testing.T, entries ...sessionindex.Entry) string {
	t.Helper()
	return seedSessionsJSON(t, string(indexJSON(t, entries)))
}

// entryIndexOf is the index a sessions.json holding entries reads as.
func entryIndexOf(t *testing.T, entries ...sessionindex.Entry) sessionindex.Index {
	t.Helper()
	idx, err := sessionindex.Decode(indexJSON(t, entries))
	if err != nil {
		t.Fatalf("decode sessions.json: %v", err)
	}
	return idx
}

func indexJSON(t *testing.T, entries []sessionindex.Entry) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"version": sessionindex.SchemaVersion, "sessions": append([]sessionindex.Entry{}, entries...)})
	if err != nil {
		t.Fatalf("encode sessions.json: %v", err)
	}
	return body
}

var joinCreated = time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC)

var errListBoom = errors.New("read zmx dir: permission denied")

// The join's world: one session the index labels for its own creation, one
// recreated under a name an earlier session labelled, one with no entry, one
// whose entry holds a label only a hand edit could write, and one that does
// not answer.
var (
	joinOwn         = backend.Session{Name: "api-x7Kd9a", Dir: "/work/api/internal", Clients: 1, Created: joinCreated}
	joinRecreated   = backend.Session{Name: "web-Qm3pLz", Dir: "/work/web", Created: joinCreated.Add(time.Hour)}
	joinUnindexed   = backend.Session{Name: "docs-Lp2Hx9", Dir: "/work/docs", Created: joinCreated}
	joinUnprintable = backend.Session{Name: "lab-Zz9q1w", Dir: "/work/lab", Created: joinCreated}
	joinSilent      = backend.Session{Name: "quiet-Rt5mKe", Dir: "/work/quiet", Created: joinCreated}
)

func joinEntries() []sessionindex.Entry {
	return []sessionindex.Entry{
		{Name: joinOwn.Name, Label: "api work", Dir: "/work/api", CreatedAt: joinCreated},
		{Name: joinRecreated.Name, Label: "old web", Dir: "/work/old-web", CreatedAt: joinCreated},
		{Name: joinUnprintable.Name, Label: "lab\nwork", Dir: "/work/lab", CreatedAt: joinCreated},
		{Name: joinSilent.Name, Label: "quiet", Dir: "/work/quiet", CreatedAt: joinCreated},
	}
}

// stageJoin stages the join's world as the backend, with its index and no
// bootstrap owed, and returns the Fake.
func stageJoin(t *testing.T) *backendtest.Fake {
	t.Helper()
	fake := backendtest.New(joinOwn, joinRecreated, joinUnindexed, joinUnprintable, joinSilent)
	fake.StopAnswering(joinSilent.Name)
	withBootstrapDeps(t, BootstrapDeps{})
	withBackendDeps(t, BackendDeps{Backend: fake})
	seedIndex(t, joinEntries()...)
	return fake
}

// wantJoined is the join's world as every surface shows it, in name order.
var wantJoined = []tui.Session{
	{Name: joinOwn.Name, Label: "api work", Dir: "/work/api", Cwd: "/work/api/internal", Attached: true},
	{Name: joinUnindexed.Name, Cwd: "/work/docs"},
	{Name: joinUnprintable.Name, Dir: "/work/lab", Cwd: "/work/lab"},
	{Name: joinSilent.Name, NotAnswering: true},
	{Name: joinRecreated.Name, Cwd: "/work/web"},
}

func TestShownSession(t *testing.T) {
	shown := shownIndex{idx: entryIndexOf(t, joinEntries()...), rule: claimRule("")}
	silent := backend.Session{Name: joinSilent.Name}
	for _, tc := range []struct {
		name string
		live backend.Session
		want tui.Session
	}{
		{"its own creation's entry gives its label and dir, beside its cwd", withAnswered(joinOwn), wantJoined[0]},
		{"an earlier creation's entry of this boot gives nothing", withAnswered(joinRecreated), wantJoined[4]},
		{"no entry gives nothing", withAnswered(joinUnindexed), wantJoined[1]},
		{"an unprintable label shows the name", withAnswered(joinUnprintable), wantJoined[2]},
		{"a session that did not answer shows its name", silent, wantJoined[3]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shown.show(tc.live); got != tc.want {
				t.Errorf("show = %+v, want %+v", got, tc.want)
			}
		})
	}

	t.Run("an entry from the same second with another pid is an earlier creation, and gives nothing", func(t *testing.T) {
		live := withAnswered(joinOwn)
		live.PID = 4243
		recorded := shownIndex{
			idx:  entryIndexOf(t, sessionindex.Entry{Name: joinOwn.Name, Label: "api work", Dir: "/work/api", CreatedAt: joinCreated, PID: 4242}),
			rule: claimRule(""),
		}

		if got := recorded.show(live); got.Label != "" || got.Dir != "" {
			t.Errorf("show = %+v, want no label or dir", got)
		}
		live.PID = 4242
		if got := recorded.show(live); got.Label != "api work" {
			t.Errorf("show of the recorded pid = %+v, want its label", got)
		}
	})

	for name, created := range map[string]time.Time{
		"saved before this boot":            testBoot.Add(-time.Hour),
		"whose creation was never recorded": {},
	} {
		t.Run("an entry "+name+" gives its label and dir, as a claim would take it over", func(t *testing.T) {
			recorded := shownIndex{
				idx:  entryIndexOf(t, sessionindex.Entry{Name: joinOwn.Name, Label: "api work", Dir: "/work/api", CreatedAt: created, PID: 1717}),
				rule: claimRule(""),
			}

			if got := recorded.show(withAnswered(joinOwn)); got != wantJoined[0] {
				t.Errorf("show = %+v, want %+v", got, wantJoined[0])
			}
		})
	}
}

func withAnswered(s backend.Session) backend.Session {
	s.Answered = true
	return s
}

func TestPickerLister(t *testing.T) {
	t.Run("it lists the user's sessions joined with the index", func(t *testing.T) {
		fake := stageJoin(t)
		fake.Add(backend.Session{Name: "_portal-saver"})

		got, err := pickerLister{ctx: context.Background(), b: fake}.ListSessions()

		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if !slices.Equal(got, wantJoined) {
			t.Errorf("ListSessions =\n%+v\nwant\n%+v", got, wantJoined)
		}
	})

	t.Run("it lists every session by its name when the index cannot be read", func(t *testing.T) {
		fake := backendtest.New(joinOwn)
		blockStateDir(t)

		got, err := pickerLister{ctx: context.Background(), b: fake}.ListSessions()

		want := []tui.Session{{Name: joinOwn.Name, Cwd: joinOwn.Dir, Attached: true}}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("ListSessions = %+v, %v; want %+v", got, err, want)
		}
	})

	t.Run("it passes a failed listing through", func(t *testing.T) {
		fake := backendtest.New(joinOwn)
		fake.Fail(backendtest.OpList, errListBoom)
		isolateStateDir(t)

		if _, err := (pickerLister{ctx: context.Background(), b: fake}).ListSessions(); !errors.Is(err, errListBoom) {
			t.Errorf("ListSessions error = %v, want %v", err, errListBoom)
		}
	})
}

func TestSearchCandidates_JoinsTheIndex(t *testing.T) {
	t.Run("it counts the shown sessions, less the current one", func(t *testing.T) {
		fake := stageJoin(t)
		fake.SetCurrent(joinUnindexed.Name)

		got, err := searchCandidates(context.Background(), fake)

		want := slices.DeleteFunc(slices.Clone(wantJoined), func(s tui.Session) bool { return s.Name == joinUnindexed.Name })
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("searchCandidates = %+v, %v; want %+v", got, err, want)
		}
	})

	for term, want := range map[string]string{
		"api work":  joinOwn.Name,
		"x7kd":      joinOwn.Name,
		"old web":   "",
		"old-web":   "",
		"lab\nwork": "",
	} {
		t.Run("the search form's "+term+" lands on "+want, func(t *testing.T) {
			fake := stageJoin(t)

			got, err := searchDecision(context.Background(), fake, term)()

			if err != nil || got != want {
				t.Errorf("searchDecision(%q) = %q, %v; want %q", term, got, err, want)
			}
		})
	}
}

func TestJoin_ListLong(t *testing.T) {
	stageJoin(t)
	withListDeps(t, ListDeps{IsTTY: func() bool { return false }})

	want := "api-x7Kd9a    api work    attached    /work/api\n" +
		"docs-Lp2Hx9    docs-Lp2Hx9    detached    /work/docs\n" +
		"lab-Zz9q1w    lab-Zz9q1w    detached    /work/lab\n" +
		"quiet-Rt5mKe    not answering\n" +
		"web-Qm3pLz    web-Qm3pLz    detached    /work/web\n"
	if got := runList(t, "--long"); got != want {
		t.Errorf("list --long =\n%s\nwant\n%s", got, want)
	}
}

func TestJoin_HookListLocation(t *testing.T) {
	body := map[string]map[string]string{}
	for _, s := range wantJoined {
		body[s.Name] = map[string]string{"on-resume": "true"}
	}
	hooksFileInTempDir(t, body)
	stageJoin(t)

	want := "api-x7Kd9a\ton-resume\ttrue\tapi work\t\n" +
		"docs-Lp2Hx9\ton-resume\ttrue\tdocs-Lp2Hx9\t\n" +
		"lab-Zz9q1w\ton-resume\ttrue\tlab-Zz9q1w\t\n" +
		"quiet-Rt5mKe\ton-resume\ttrue\tquiet-Rt5mKe\t\n" +
		"web-Qm3pLz\ton-resume\ttrue\tweb-Qm3pLz\t\n"
	if got := runHookList(t); got != want {
		t.Errorf("hook list =\n%s\nwant\n%s", got, want)
	}
}

func TestJoin_Current(t *testing.T) {
	for _, s := range wantJoined {
		t.Run(s.Name+" prints "+s.DisplayName(), func(t *testing.T) {
			fake := stageJoin(t)
			fake.SetCurrent(s.Name)

			stdout, _, err := runRootCmd(t, "current")

			if err != nil || stdout.String() != s.DisplayName()+"\n" {
				t.Errorf("current = %q, %v; want %q", stdout, err, s.DisplayName())
			}
		})
	}
}
