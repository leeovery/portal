package cmd

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/hookstest"
)

// The sessions the hook suites key their registrations by.
const (
	hookSessionA = "api-x7Kd9a"
	hookSessionB = "web-Qm3pLz"
)

// hookSessionCreated is when inSession's session was created, in this boot.
var hookSessionCreated = testBoot.Add(time.Hour)

// inSession stages a backend holding a session named name, created at
// hookSessionCreated, and running this process inside it, in a state directory
// of the test's own; an empty name stages one running outside any session.
func inSession(t *testing.T, name string, others ...backend.Session) *backendtest.Fake {
	t.Helper()
	fake := backendtest.New(others...)
	if name != "" {
		fake.Add(backend.Session{Name: name, Created: hookSessionCreated})
	}
	fake.SetCurrent(name)
	withBackendDeps(t, BackendDeps{Backend: fake})
	isolateStateDir(t)
	return fake
}

// runHookSetForKey drives `hook set` from inside the session named key, so the
// command writes under it.
func runHookSetForKey(t *testing.T, key, command string) error {
	t.Helper()
	inSession(t, key)
	_, err := runHookSet(t, command)
	return err
}

// seedHooksFile stages a hooks.json holding body verbatim and points the
// commands at it, for a fixture whose subject is a stored shape the store's own
// layout helpers do not express.
func seedHooksFile(t *testing.T, body string) string {
	t.Helper()
	_, hooksFile := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), Seed: body})
	t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
	return hooksFile
}

// assertNoBackendList fails when the hook list read the backend at all.
func assertNoBackendList(t *testing.T, fake *backendtest.Fake) {
	t.Helper()
	if calls := fake.CallsTo(backendtest.OpList); len(calls) != 0 {
		t.Errorf("backend List calls = %d, want none with no entries to locate", len(calls))
	}
}

func TestHooksListCommand(t *testing.T) {
	t.Run("it prints key, event, command, location and mode, tab-separated", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "claude --resume abc123"},
		})
		inSession(t, "", backend.Session{Name: hookSessionA})

		got := runHookList(t)
		want := hookSessionA + "\ton-resume\tclaude --resume abc123\t" + hookSessionA + "\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it prints nothing and reads no sessions with no hooks registered", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{})
		fake := inSession(t, "", backend.Session{Name: hookSessionA})

		if got := runHookList(t); got != "" {
			t.Errorf("output = %q, want empty", got)
		}
		assertNoBackendList(t, fake)
	})

	t.Run("it prints nothing and reads no sessions when hooks.json does not exist", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		fake := inSession(t, "", backend.Session{Name: hookSessionA})

		if got := runHookList(t); got != "" {
			t.Errorf("output = %q, want empty", got)
		}
		assertNoBackendList(t, fake)
	})

	t.Run("it sorts by key then event", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionB: {"on-resume": "claude --resume def456"},
			hookSessionA: {"on-resume": "npm start"},
		})
		inSession(t, "", backend.Session{Name: hookSessionA}, backend.Session{Name: hookSessionB})

		got := runHookList(t)
		want := hookSessionA + "\ton-resume\tnpm start\t" + hookSessionA + "\t\n" +
			hookSessionB + "\ton-resume\tclaude --resume def456\t" + hookSessionB + "\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it accepts no arguments", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		inSession(t, "")

		if _, _, err := runRootCmd(t, "hook", "list", "extraarg"); err == nil {
			t.Fatal("expected an error for an extra argument, got nil")
		}
	})
}

func TestHooksListLocationColumn(t *testing.T) {
	t.Run("it locates a key by its live session's label", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "npm start"},
			hookSessionB: {"on-resume": "make dev"},
		})
		web := backend.Session{Name: hookSessionB, Created: joinCreated}
		inSession(t, "", backend.Session{Name: hookSessionA}, web)
		labelSession(t, os.Getenv("PORTAL_STATE_DIR"), web, "web · checkout")

		got := runHookList(t)
		want := hookSessionA + "\ton-resume\tnpm start\t" + hookSessionA + "\t\n" +
			hookSessionB + "\ton-resume\tmake dev\tweb · checkout\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it leaves the location empty for a key that names no session", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA:           {"on-resume": "npm start"},
			hookstest.SubjectSeedA: {"on-resume": "claude --resume abc"},
			hookstest.TmuxEraSeedA: {"on-resume": "make dev"},
			"gone-Zx81Qa":          {"on-resume": "true"},
		})
		inSession(t, "", backend.Session{Name: hookSessionA})

		for line := range strings.SplitSeq(strings.TrimSuffix(runHookList(t), "\n"), "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) != 5 {
				t.Fatalf("line %q, want five tab-separated fields", line)
			}
			want := ""
			if fields[0] == hookSessionA {
				want = hookSessionA
			}
			if fields[3] != want {
				t.Errorf("location for %s = %q, want %q", fields[0], fields[3], want)
			}
		}
	})

	t.Run("it does not locate a key by one of Portal's own sessions", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			"_portal-saver": {"on-resume": "true"},
		})
		inSession(t, "", backend.Session{Name: "_portal-saver"})

		if got, want := runHookList(t), "_portal-saver\ton-resume\ttrue\t\t\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it locates a session that is not answering", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "npm start"},
		})
		fake := inSession(t, "", backend.Session{Name: hookSessionA})
		fake.StopAnswering(hookSessionA)

		if got, want := runHookList(t), hookSessionA+"\ton-resume\tnpm start\t"+hookSessionA+"\t\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it leaves every location empty when the listing fails, and still lists", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "npm start"},
			hookSessionB: {"on-resume": "make dev"},
		})
		fake := inSession(t, "", backend.Session{Name: hookSessionA}, backend.Session{Name: hookSessionB})
		fake.Fail(backendtest.OpList, errors.New("read zmx dir: permission denied"))

		got := runHookList(t)
		want := hookSessionA + "\ton-resume\tnpm start\t\t\n" + hookSessionB + "\ton-resume\tmake dev\t\t\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it locates by name when the index cannot be read", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "npm start"},
		})
		inSession(t, "", backend.Session{Name: hookSessionA})
		blockStateDir(t)

		if got, want := runHookList(t), hookSessionA+"\ton-resume\tnpm start\t"+hookSessionA+"\t\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it lists the sessions once for many entries", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA:           {"on-resume": "one"},
			hookSessionB:           {"on-resume": "two"},
			hookstest.SubjectSeedA: {"on-resume": "three"},
		})
		fake := inSession(t, "", backend.Session{Name: hookSessionA}, backend.Session{Name: hookSessionB})

		runHookList(t)

		if calls := fake.CallsTo(backendtest.OpList); len(calls) != 1 {
			t.Errorf("backend List calls = %d, want 1", len(calls))
		}
	})
}

func TestHooksListModeColumn(t *testing.T) {
	t.Run("it prints the registration's mode as the fifth column", func(t *testing.T) {
		for _, mode := range []string{"eager", "lazy"} {
			t.Run(mode, func(t *testing.T) {
				seedHooksFile(t, `{"`+hookSessionA+`":{"on-resume":{"command":"npm start","resume":"`+mode+`"}}}`)
				inSession(t, "", backend.Session{Name: hookSessionA})

				got := runHookList(t)
				want := hookSessionA + "\ton-resume\tnpm start\t" + hookSessionA + "\t" + mode + "\n"
				if got != want {
					t.Errorf("output = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("it leaves the fifth column empty for a registration carrying no recognised mode", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			stored string
		}{
			{name: "unrecognised word", stored: `"whenever"`},
			{name: "empty", stored: `""`},
			{name: "capitalised", stored: `"Lazy"`},
			{name: "non-string", stored: `7`},
			{name: "absent", stored: ``},
		} {
			t.Run(tt.name, func(t *testing.T) {
				registration := `{"command":"npm start"}`
				if tt.stored != "" {
					registration = `{"command":"npm start","resume":` + tt.stored + `}`
				}
				seedHooksFile(t, `{"`+hookSessionA+`":{"on-resume":`+registration+`}}`)
				inSession(t, "", backend.Session{Name: hookSessionA})

				got := runHookList(t)
				want := hookSessionA + "\ton-resume\tnpm start\t" + hookSessionA + "\t\n"
				if got != want {
					t.Errorf("output = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("it keeps the first four columns where a positional parser reads them", func(t *testing.T) {
		seedHooksFile(t, `{"`+hookSessionA+`":{"on-resume":{"command":"npm start","resume":"lazy"}},`+
			`"`+hookstest.SubjectSeedB+`":{"on-resume":"claude --resume abc123"}}`)
		inSession(t, "", backend.Session{Name: hookSessionA})

		wantFirstFour := map[string][]string{
			hookSessionA:           {hookSessionA, "on-resume", "npm start", hookSessionA},
			hookstest.SubjectSeedB: {hookstest.SubjectSeedB, "on-resume", "claude --resume abc123", ""},
		}
		lines := strings.Split(strings.TrimSuffix(runHookList(t), "\n"), "\n")
		if len(lines) != len(wantFirstFour) {
			t.Fatalf("lines = %q, want %d", lines, len(wantFirstFour))
		}
		for _, line := range lines {
			fields := strings.Split(line, "\t")
			if len(fields) != 5 {
				t.Fatalf("line %q, want five tab-separated fields", line)
			}
			if want := wantFirstFour[fields[0]]; !slices.Equal(fields[:4], want) {
				t.Errorf("first four fields = %q, want %q", fields[:4], want)
			}
		}
	})
}

func TestHooksSetCommand(t *testing.T) {
	t.Run("it registers the hook under the current session's name", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		fake := inSession(t, hookSessionA)

		if _, err := runHookSet(t, "claude --resume abc123"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if len(data) != 1 || len(data[hookSessionA]) != 1 || data[hookSessionA]["on-resume"].Command != "claude --resume abc123" {
			t.Errorf("hooks.json = %v, want one on-resume entry under %s", data, hookSessionA)
		}
		if calls := fake.Calls(); len(calls) != 1 || calls[0].Op != backendtest.OpSession || calls[0].Name != hookSessionA {
			t.Errorf("backend calls = %+v, want one read of %s, to claim it", calls, hookSessionA)
		}
	})

	t.Run("under the production backend it reads the session ZMX_SESSION names, and sets no hook it can't claim", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		isolateStateDir(t)
		t.Setenv("ZMX_SESSION", hookSessionB)

		_, err := runHookSet(t, "make dev")

		if err == nil || !strings.Contains(err.Error(), hookSessionB) {
			t.Errorf("hook set = %v, want the read of %s, refused under the poisoned ZMX_DIR", err, hookSessionB)
		}
		if _, err := os.Stat(hooksFile); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat hooks.json = %v, want nothing set", err)
		}
	})

	t.Run("it refuses outside a session in one line and writes nothing", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, "")

		_, err := runHookSet(t, "some-cmd")

		if !errors.Is(err, errNotInSession) || err.Error() != "not inside a Portal session" {
			t.Errorf("error = %v, want %q", err, "not inside a Portal session")
		}
		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks.json was created outside a session")
		}
	})

	t.Run("it requires --on-resume", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)

		_, _, err := runRootCmd(t, "hook", "set")
		if err == nil || !strings.Contains(err.Error(), "on-resume") {
			t.Errorf("error = %v, want it to name --on-resume", err)
		}
	})

	t.Run("it overwrites the session's earlier hook", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)

		for _, command := range []string{"old-cmd", "new-cmd"} {
			if _, err := runHookSet(t, command); err != nil {
				t.Fatalf("hook set %s: %v", command, err)
			}
		}

		if got := readHooksJSON(t, hooksFile)[hookSessionA]["on-resume"].Command; got != "new-cmd" {
			t.Errorf("hook command = %q, want %q", got, "new-cmd")
		}
	})

	t.Run("it claims the session's entry for its live creation first", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		live := backend.Session{Name: hookSessionA, Created: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), PID: 4242}
		inSession(t, "", live).SetCurrent(hookSessionA)

		if _, err := runHookSet(t, "npm start"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		e := entryOf(t, os.Getenv("PORTAL_STATE_DIR"), hookSessionA)
		if !e.CreatedAt.Equal(live.Created) || e.PID != live.PID {
			t.Errorf("entry = %+v, want it to record the live creation", e)
		}
	})

	t.Run("a claim that fails sets no hook", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)
		commitlock.HoldForTest(t, os.Getenv("PORTAL_STATE_DIR"))
		commitlock.SetTimeoutForTest(t, 30*time.Millisecond)

		if _, err := runHookSet(t, "npm start"); !errors.Is(err, commitlock.ErrHeld) {
			t.Errorf("hook set = %v, want the claim's ErrHeld", err)
		}
		if _, err := os.Stat(hooksFile); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat hooks.json = %v, want nothing set", err)
		}
	})

	t.Run("a session that does not answer is refused, setting no hook", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA).StopAnswering(hookSessionA)

		_, err := runHookSet(t, "npm start")

		if want := "session " + hookSessionA + " is not answering; no hook is set"; err == nil || err.Error() != want {
			t.Errorf("hook set = %v, want %q", err, want)
		}
		if _, err := os.Stat(hooksFile); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat hooks.json = %v, want nothing set", err)
		}
	})

	t.Run("a session that answers with no creation is refused, claiming and setting nothing", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, "", backend.Session{Name: hookSessionA, PID: 4242}).SetCurrent(hookSessionA)

		_, err := runHookSet(t, "npm start")

		if want := "session " + hookSessionA + " did not report when it was created; no hook is set"; err == nil || err.Error() != want {
			t.Errorf("hook set = %v, want %q", err, want)
		}
		if _, err := os.Stat(hooksFile); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat hooks.json = %v, want nothing set", err)
		}
		if entries := dirListing(t, os.Getenv("PORTAL_STATE_DIR")); len(entries) != 0 {
			t.Errorf("state directory = %v, want no claim written", entries)
		}
	})
}

func TestHooksRmCommand(t *testing.T) {
	t.Run("it removes the current session's hook, leaving the others", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "npm start"},
			hookSessionB: {"on-resume": "make dev"},
		})
		inSession(t, hookSessionA)

		if _, err := runHookRm(t); err != nil {
			t.Fatalf("hook rm: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookSessionA]; ok {
			t.Errorf("hooks.json = %v, want %s's entry removed", data, hookSessionA)
		}
		if data[hookSessionB]["on-resume"].Command != "make dev" {
			t.Errorf("hooks.json = %v, want %s's entry left in place", data, hookSessionB)
		}
	})

	t.Run("it refuses outside a session in one line", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "npm start"},
		})
		before := readFileBytes(t, hooksFile)
		inSession(t, "")

		if _, err := runHookRm(t); !errors.Is(err, errNotInSession) {
			t.Errorf("error = %v, want %v", err, errNotInSession)
		}
		assertHooksFileUnchanged(t, hooksFile, before)
	})

	t.Run("it requires --on-resume", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)

		_, _, err := runRootCmd(t, "hook", "rm")
		if err == nil || !strings.Contains(err.Error(), "on-resume") {
			t.Errorf("error = %v, want it to name --on-resume", err)
		}
	})

	t.Run("it removes the --pane-key entry verbatim, outside any session", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "claude --resume xyz"},
			hookstest.TmuxEraSeedA: {"on-resume": "npm start"},
		})
		inSession(t, "")

		for _, key := range []string{hookstest.SubjectSeedA, hookstest.TmuxEraSeedA} {
			if _, err := runHookRm(t, "--pane-key", key); err != nil {
				t.Fatalf("hook rm --pane-key %s: %v", key, err)
			}
		}

		if data := readHooksJSON(t, hooksFile); len(data) != 0 {
			t.Errorf("hooks.json = %v, want both keys removed", data)
		}
	})

	t.Run("it takes --pane-key over the current session", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "npm start"},
			hookSessionB: {"on-resume": "make dev"},
		})
		inSession(t, hookSessionA)

		if _, err := runHookRm(t, "--pane-key", hookSessionB); err != nil {
			t.Fatalf("hook rm --pane-key: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data[hookSessionB]; ok || data[hookSessionA]["on-resume"].Command != "npm start" {
			t.Errorf("hooks.json = %v, want only %s's entry removed", data, hookSessionB)
		}
	})

	t.Run("it describes --pane-key as a verbatim hook key", func(t *testing.T) {
		flag := hooksRmCmd.Flags().Lookup("pane-key")
		if flag == nil {
			t.Fatal("hook rm has no --pane-key flag")
		}

		want := "Hook key of the entry to remove, taken verbatim — any key, including an old-format one (defaults to the current session's name)"
		if flag.Usage != want {
			t.Errorf("--pane-key usage = %q, want %q", flag.Usage, want)
		}
	})
}

func TestHookCommandAlias(t *testing.T) {
	t.Run("it names the command hook, with hooks as an alias", func(t *testing.T) {
		if hookCmd.Name() != "hook" || !slices.Contains(hookCmd.Aliases, "hooks") {
			t.Errorf("hookCmd = %q aliases %v, want hook aliased by hooks", hookCmd.Name(), hookCmd.Aliases)
		}
	})

	t.Run("it resolves the hooks alias to the hook subcommands", func(t *testing.T) {
		for _, sub := range []string{"list", "set", "rm"} {
			c, _, err := rootCmd.Find([]string{"hooks", sub})
			if err != nil {
				t.Fatalf("Find([hooks %s]) error: %v", sub, err)
			}
			if c.Name() != sub || c.Parent() == nil || c.Parent().Name() != "hook" {
				t.Errorf("Find([hooks %s]) = %q under %v, want %s under hook", sub, c.Name(), c.Parent(), sub)
			}
		}
	})

	t.Run("it keeps hooks a silent alias", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{})
		inSession(t, "")

		outBuf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(outBuf)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("hooks list: %v", err)
		}

		combined := strings.ToLower(outBuf.String() + errBuf.String())
		if strings.Contains(combined, "deprecat") || strings.Contains(combined, "warning") {
			t.Errorf("hooks alias printed a notice: out=%q err=%q", outBuf.String(), errBuf.String())
		}
	})

	t.Run("it persists a machine-written hooks set through the alias", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)

		if _, _, err := runRootCmd(t, "hooks", "set", "--on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("hooks set: %v", err)
		}

		if got := readHooksJSON(t, hooksFile)[hookSessionA]["on-resume"].Command; got != "claude --resume abc123" {
			t.Errorf("hook command = %q, want %q", got, "claude --resume abc123")
		}
	})
}
