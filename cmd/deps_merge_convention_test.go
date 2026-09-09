package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
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

		if deps.ServerRunning == nil {
			t.Error("ServerRunning = nil; an unset seam must fall through to its production default")
		}
		if deps.SaverPresent == nil {
			t.Error("SaverPresent = nil; an unset seam must fall through to its production default")
		}
		if deps.HookCounts == nil {
			t.Error("HookCounts = nil; an unset seam must fall through to its production default")
		}
		if deps.HookLister == nil {
			t.Error("HookLister = nil; an unset seam must fall through to its production default")
		}
		if deps.Detector == nil {
			t.Error("Detector = nil; an unset seam must fall through to its production default")
		}
		if deps.Resolve == nil {
			t.Error("Resolve = nil; an unset seam must fall through to its production default")
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

func TestResolveCommitNowDepsMergeConvention(t *testing.T) {
	t.Run("it resolves commit-now's seams by the same rule", func(t *testing.T) {
		for _, tc := range commitNowSeamCases() {
			t.Run(tc.field, func(t *testing.T) {
				tc.inject(t)
				tc.assert(t)
			})
		}
	})

	t.Run("every exported field of CommitNowDeps is covered", func(t *testing.T) {
		assertSeamCasesCoverFields(t, reflect.TypeFor[CommitNowDeps](), commitNowSeamCases())
	})

	t.Run("it falls through to the production default for an unset field", func(t *testing.T) {
		withCommitNowDeps(t, CommitNowDeps{TouchSaveRequested: func(string) error { return nil }})

		deps := resolveCommitNowDeps()

		if deps.ReadIndex == nil {
			t.Error("ReadIndex = nil; an unset seam must fall through to its production default")
		}
		if deps.CaptureStructure == nil {
			t.Error("CaptureStructure = nil; an unset seam must fall through to its production default")
		}
		if deps.Commit == nil {
			t.Error("Commit = nil; an unset seam must fall through to its production default")
		}
		if deps.NewClient == nil {
			t.Error("NewClient = nil; an unset seam must fall through to its production default")
		}
		if deps.IsRestoring == nil {
			t.Error("IsRestoring = nil; an unset seam must fall through to its production default")
		}
	})
}

// doctorSeamCases is a func rather than a var so each case's sentinels are minted
// fresh per run, and so the coverage guard reads the same table the cases do.
func doctorSeamCases() []seamCase {
	sentinelHookStore := hooks.NewStore("/sentinel/hooks.json")
	sentinelProjectStore := project.NewStore("/sentinel/projects.json")
	sentinelPrefsStore := prefs.NewStore("/sentinel/prefs.json")
	sentinelLister := &stubStaleSweepReader{}
	sentinelDetector := fakeTerminalDetector{id: spawn.NewIdentity("com.sentinel.term", "Sentinel")}

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
			field: "ServerRunning",
			inject: func(t *testing.T) {
				withDoctorDeps(t, DoctorDeps{ServerRunning: func() bool { return true }})
			},
			assert: func(t *testing.T) {
				if !resolveDoctorDeps().ServerRunning() {
					t.Error("ServerRunning() = false; want the injected seam's true")
				}
			},
		},
		{
			field: "SaverPresent",
			inject: func(t *testing.T) {
				withDoctorDeps(t, DoctorDeps{SaverPresent: func() (bool, error) { return true, nil }})
			},
			assert: func(t *testing.T) {
				present, err := resolveDoctorDeps().SaverPresent()
				if err != nil || !present {
					t.Errorf("SaverPresent() = (%v, %v); want the injected seam's (true, nil)", present, err)
				}
			},
		},
		{
			field: "HookCounts",
			inject: func(t *testing.T) {
				withDoctorDeps(t, DoctorDeps{HookCounts: func() (map[string]int, error) {
					return map[string]int{"sentinel": 1}, nil
				}})
			},
			assert: func(t *testing.T) {
				counts, err := resolveDoctorDeps().HookCounts()
				if err != nil || counts["sentinel"] != 1 {
					t.Errorf("HookCounts() = (%v, %v); want the injected seam's sentinel count", counts, err)
				}
			},
		},
		{
			field:  "HookLister",
			inject: func(t *testing.T) { withDoctorDeps(t, DoctorDeps{HookLister: sentinelLister}) },
			assert: func(t *testing.T) {
				if got := resolveDoctorDeps().HookLister; got != hooksweep.Reader(sentinelLister) {
					t.Errorf("HookLister = %#v; want the injected lister", got)
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

func commitNowSeamCases() []seamCase {
	return []seamCase{
		{
			field: "ReadIndex",
			inject: func(t *testing.T) {
				withCommitNowDeps(t, CommitNowDeps{ReadIndex: func(string) (state.Index, bool, error) {
					return state.Index{Version: 99}, true, nil
				}})
			},
			assert: func(t *testing.T) {
				idx, _, _ := resolveCommitNowDeps().ReadIndex("")
				if idx.Version != 99 {
					t.Errorf("ReadIndex() version = %d; want the injected seam's 99", idx.Version)
				}
			},
		},
		{
			field: "CaptureStructure",
			inject: func(t *testing.T) {
				withCommitNowDeps(t, CommitNowDeps{CaptureStructure: func(state.CaptureClient, map[string]struct{}, *state.Index, *slog.Logger) (state.Index, error) {
					return state.Index{Version: 98}, nil
				}})
			},
			assert: func(t *testing.T) {
				idx, _ := resolveCommitNowDeps().CaptureStructure(nil, nil, nil, nil)
				if idx.Version != 98 {
					t.Errorf("CaptureStructure() version = %d; want the injected seam's 98", idx.Version)
				}
			},
		},
		{
			field: "Commit",
			inject: func(t *testing.T) {
				committed := false
				withCommitNowDeps(t, CommitNowDeps{Commit: func(string, state.Index, bool, *slog.Logger) error {
					committed = true
					return nil
				}})
				t.Cleanup(func() {
					if !committed {
						t.Error("the injected Commit seam was never called; resolveCommitNowDeps must not overwrite it")
					}
				})
			},
			assert: func(t *testing.T) {
				if err := resolveCommitNowDeps().Commit(t.TempDir(), state.Index{}, false, nil); err != nil {
					t.Errorf("Commit() err = %v; want the injected seam's nil", err)
				}
			},
		},
		{
			field: "NewClient",
			inject: func(t *testing.T) {
				called := false
				withCommitNowDeps(t, CommitNowDeps{NewClient: func() state.CaptureClient {
					called = true
					return nil
				}})
				t.Cleanup(func() {
					if !called {
						t.Error("the injected NewClient seam was never called; resolveCommitNowDeps must not overwrite it")
					}
				})
			},
			assert: func(t *testing.T) {
				if got := resolveCommitNowDeps().NewClient(); got != nil {
					t.Errorf("NewClient() = %#v; want the injected seam's nil client", got)
				}
			},
		},
		{
			field: "IsRestoring",
			inject: func(t *testing.T) {
				withCommitNowDeps(t, CommitNowDeps{IsRestoring: func() (bool, error) { return true, nil }})
			},
			assert: func(t *testing.T) {
				restoring, err := resolveCommitNowDeps().IsRestoring()
				if err != nil || !restoring {
					t.Errorf("IsRestoring() = (%v, %v); want the injected seam's (true, nil)", restoring, err)
				}
			},
		},
		{
			field: "TouchSaveRequested",
			inject: func(t *testing.T) {
				touched := ""
				withCommitNowDeps(t, CommitNowDeps{TouchSaveRequested: func(dir string) error {
					touched = dir
					return nil
				}})
				t.Cleanup(func() {
					if touched != "/sentinel/state" {
						t.Errorf("the injected TouchSaveRequested seam saw %q; want /sentinel/state", touched)
					}
				})
			},
			assert: func(t *testing.T) {
				if err := resolveCommitNowDeps().TouchSaveRequested("/sentinel/state"); err != nil {
					t.Errorf("TouchSaveRequested() err = %v; want the injected seam's nil", err)
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
