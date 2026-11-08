package cmd

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/spawn"
)

// seamCase pins one field of a *Deps struct: what a test injects into it, and
// how the resolved struct proves it is that injection the command will run.
type seamCase struct {
	field  string
	inject func(*testing.T)
	assert func(*testing.T)
}

func TestResolveDoctorDepsMergeConvention(t *testing.T) {
	t.Run("it runs against the injected seam for every field a test sets", func(t *testing.T) {
		for _, tc := range doctorSeamCases() {
			t.Run(tc.field, func(t *testing.T) {
				tc.inject(t)
				tc.assert(t)
			})
		}
	})

	t.Run("every exported field of DoctorDeps is covered", func(t *testing.T) {
		assertSeamCasesCoverFields(t, reflect.TypeFor[DoctorDeps](), doctorSeamCases())
	})

	t.Run("it falls through to the production default for an unset field", func(t *testing.T) {
		withDoctorDeps(t, DoctorDeps{StateDir: t.TempDir()})

		deps := resolveDoctorDeps()

		if deps.ZmxDir == "" {
			t.Error("ZmxDir is empty; an unset seam must fall through to its production default")
		}
		if deps.Zmx == nil {
			t.Error("Zmx = nil; an unset seam must fall through to its production default")
		}
		if deps.ProtocolVersion == nil {
			t.Error("ProtocolVersion = nil; an unset seam must fall through to its production default")
		}
		if deps.ListSessions == nil {
			t.Error("ListSessions = nil; an unset seam must fall through to its production default")
		}
		if deps.Detector == nil {
			t.Error("Detector = nil; an unset seam must fall through to its production default")
		}
		if deps.Resolve == nil {
			t.Error("Resolve = nil; an unset seam must fall through to its production default")
		}
		if deps.Saver == nil {
			t.Error("Saver = nil; an unset seam must fall through to its production default")
		}
		if deps.Version != Version() {
			t.Errorf("Version = %q; want this binary's %q", deps.Version, Version())
		}
		if deps.DaemonSettle != saverhost.ReadyWait {
			t.Errorf("DaemonSettle = %v; want saverhost.ReadyWait, %v", deps.DaemonSettle, saverhost.ReadyWait)
		}
	})

	// The zmx dir check judges ZmxDir, so the sessions must be listed and
	// probed there too, not in whatever dir the environment names.
	t.Run("it lists and probes the sessions in the zmx dir it judges", func(t *testing.T) {
		dir, err := os.MkdirTemp("/tmp", "pd-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		ln, err := net.Listen("unix", filepath.Join(dir, "probe"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		withDoctorDeps(t, DoctorDeps{ZmxDir: dir})

		deps := resolveDoctorDeps()

		sessions, err := deps.ListSessions(t.Context())
		if err != nil || len(sessions) != 1 || sessions[0].Name != "probe" {
			t.Errorf("ListSessions() = (%+v, %v); want the session in %s", sessions, err, dir)
		}
		if _, err := deps.ProtocolVersion(t.Context(), "probe"); !errors.Is(err, backend.ErrTimeout) || errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("ProtocolVersion(probe) = %v; want the session in %s found and not answering", err, dir)
		}
	})

	t.Run("it loads no hook, project or prefs store when one is injected", func(t *testing.T) {
		staged := stageApplicationSupportConfig(t)

		withDoctorDeps(t, DoctorDeps{
			HookStore:    hooks.NewStore(filepath.Join(t.TempDir(), "hooks.json")),
			ProjectStore: project.NewStore(filepath.Join(t.TempDir(), "projects.json")),
			PrefsStore:   prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json")),
		})

		resolveDoctorDeps()

		for _, file := range staged.files {
			assertMigrated(t, staged, file, false)
		}
	})

	t.Run("it loads each store the injection left unset", func(t *testing.T) {
		staged := stageApplicationSupportConfig(t)

		withDoctorDeps(t, DoctorDeps{})

		resolveDoctorDeps()

		for _, file := range staged.files {
			assertMigrated(t, staged, file, true)
		}
	})
}

// doctorSeamCases is a func rather than a var so each case's sentinels are minted
// fresh per run, and so the coverage guard reads the same table the cases do.
func doctorSeamCases() []seamCase {
	sentinelNames := backendtest.New()
	sentinelHookStore := hooks.NewStore("/sentinel/hooks.json")
	sentinelProjectStore := project.NewStore("/sentinel/projects.json")
	sentinelPrefsStore := prefs.NewStore("/sentinel/prefs.json")
	sentinelDetector := fakeTerminalDetector{id: spawn.NewIdentity("com.sentinel.term", "Sentinel")}
	sentinelSaver := &saverAnswers{present: true, pid: 77}

	return []seamCase{
		{
			field:  "StateDir",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{StateDir: "/sentinel/state"}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().StateDir; got != "/sentinel/state" {
					t.Errorf("StateDir = %q; want the injected /sentinel/state", got)
				}
			},
		},
		{
			field:  "ThemesDir",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{ThemesDir: "/sentinel/themes"}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().ThemesDir; got != "/sentinel/themes" {
					t.Errorf("ThemesDir = %q; want the injected /sentinel/themes", got)
				}
			},
		},
		{
			field: "ListSessions",
			inject: func(t *testing.T) {
				withDoctorDeps(t, DoctorDeps{ListSessions: backendtest.New(backend.Session{Name: "sentinel"}).List})
			},
			assert: func(t *testing.T) {
				sessions, err := resolveDoctorDeps().ListSessions(t.Context())
				if err != nil || len(sessions) != 1 || sessions[0].Name != "sentinel" {
					t.Errorf("ListSessions() = (%+v, %v); want the injected seam's sentinel", sessions, err)
				}
			},
		},
		{
			field:  "ZmxDir",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{ZmxDir: "/sentinel/zmx"}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().ZmxDir; got != "/sentinel/zmx" {
					t.Errorf("ZmxDir = %q; want the injected /sentinel/zmx", got)
				}
			},
		},
		{
			field: "Zmx",
			inject: func(t *testing.T) {
				withDoctorDeps(t, DoctorDeps{Zmx: func(context.Context) (string, error) { return "/sentinel/zmx", nil }})
			},
			assert: func(t *testing.T) {
				if got, err := resolveDoctorDeps().Zmx(t.Context()); err != nil || got != "/sentinel/zmx" {
					t.Errorf("Zmx() = (%q, %v); want the injected seam's /sentinel/zmx", got, err)
				}
			},
		},
		{
			field: "ProtocolVersion",
			inject: func(t *testing.T) {
				withDoctorDeps(t, DoctorDeps{ProtocolVersion: func(context.Context, string) (int, error) { return 7, nil }})
			},
			assert: func(t *testing.T) {
				if got, err := resolveDoctorDeps().ProtocolVersion(t.Context(), "s"); err != nil || got != 7 {
					t.Errorf("ProtocolVersion() = (%d, %v); want the injected seam's 7", got, err)
				}
			},
		},
		{
			field:  "Names",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{Names: sentinelNames}) },
			assert: func(t *testing.T) {
				if got, ok := resolveDoctorDeps().Names.(*backendtest.Fake); !ok || got != sentinelNames {
					t.Errorf("Names = %v; want the injected backend %p", got, sentinelNames)
				}
			},
		},
		{
			field:  "HookStore",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{HookStore: sentinelHookStore}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().HookStore; got != sentinelHookStore {
					t.Errorf("HookStore = %p; want the injected store %p", got, sentinelHookStore)
				}
			},
		},
		{
			field:  "ProjectStore",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{ProjectStore: sentinelProjectStore}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().ProjectStore; got != sentinelProjectStore {
					t.Errorf("ProjectStore = %p; want the injected store %p", got, sentinelProjectStore)
				}
			},
		},
		{
			field:  "PrefsStore",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{PrefsStore: sentinelPrefsStore}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().PrefsStore; got != sentinelPrefsStore {
					t.Errorf("PrefsStore = %p; want the injected store %p", got, sentinelPrefsStore)
				}
			},
		},
		{
			field:  "Detector",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{Detector: sentinelDetector}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().Detector.Detect(); got != sentinelDetector.id {
					t.Errorf("Detector.Detect() = %#v; want the injected identity %#v", got, sentinelDetector.id)
				}
			},
		},
		{
			field:  "Saver",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{Saver: sentinelSaver}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().Saver; got != sentinelSaver {
					t.Errorf("Saver = %v; want the injected %v", got, sentinelSaver)
				}
			},
		},
		{
			field:  "Version",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{Version: "9.9.9-sentinel"}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().Version; got != "9.9.9-sentinel" {
					t.Errorf("Version = %q; want the injected 9.9.9-sentinel", got)
				}
			},
		},
		{
			field:  "DaemonSettle",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{DaemonSettle: 7 * time.Millisecond}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().DaemonSettle; got != 7*time.Millisecond {
					t.Errorf("DaemonSettle = %v; want the injected 7ms", got)
				}
			},
		},
		{
			field: "Resolve",
			inject: func(t *testing.T) {
				withDoctorDeps(t, DoctorDeps{Resolve: func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
					return nil, spawn.ResolutionNative
				}})
			},
			assert: func(t *testing.T) {
				if _, got := resolveDoctorDeps().Resolve(spawn.Identity{}); got != spawn.ResolutionNative {
					t.Errorf("Resolve() resolution = %q; want the injected seam's %q", got, spawn.ResolutionNative)
				}
			},
		},
	}
}

// assertSeamCasesCoverFields fails when a *Deps struct grows a field the table
// does not pin, so the merge convention is proven over the whole struct rather
// than over whichever fields were current when the table was written.
func assertSeamCasesCoverFields(t *testing.T, depsType reflect.Type, cases []seamCase) {
	t.Helper()

	var want []string
	for field := range depsType.Fields() {
		if field.IsExported() {
			want = append(want, field.Name)
		}
	}

	var got []string
	for _, tc := range cases {
		got = append(got, tc.field)
	}

	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("%s seam cases cover %v; want one case per exported field %v", depsType.Name(), got, want)
	}
}

// stagedConfig is an Application Support config layout whose one-shot migration
// is the observable that a config load ran: the file moves only when production
// resolves that file's path for itself.
type stagedConfig struct {
	oldDir string
	newDir string
	files  []string
}

func stageApplicationSupportConfig(t *testing.T) stagedConfig {
	t.Helper()

	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	for _, envVar := range []string{"PORTAL_HOOKS_FILE", "PORTAL_PROJECTS_FILE", "PORTAL_PREFS_FILE"} {
		t.Setenv(envVar, "")
	}

	staged := stagedConfig{
		oldDir: filepath.Join(home, "Library", "Application Support", "portal"),
		newDir: filepath.Join(configHome, "portal"),
		files:  []string{"hooks.json", "projects.json", "prefs.json"},
	}
	if err := os.MkdirAll(staged.oldDir, 0o755); err != nil {
		t.Fatalf("stage Application Support dir: %v", err)
	}
	for _, file := range staged.files {
		if err := os.WriteFile(filepath.Join(staged.oldDir, file), []byte("{}"), 0o644); err != nil {
			t.Fatalf("stage %s: %v", file, err)
		}
	}
	return staged
}

func assertMigrated(t *testing.T, staged stagedConfig, file string, want bool) {
	t.Helper()

	_, err := os.Stat(filepath.Join(staged.newDir, file))
	if got := err == nil; got != want {
		if want {
			t.Errorf("%s was not migrated; resolving an unset field must load that store", file)
		} else {
			t.Errorf("%s was migrated; resolving an injected field must read no config", file)
		}
	}
}
