package tmux_test

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// MockCommander implements Commander for testing.
type MockCommander struct {
	Output string
	Err    error
	// Calls records all invocations as joined arg strings.
	Calls [][]string
	// RunFunc, when set, is called instead of returning Output/Err.
	RunFunc func(args ...string) (string, error)
	// RunRawFunc, when set, is called by RunRaw instead of returning Output/Err.
	// When unset, RunRaw falls back to the same Output/Err that Run would return.
	RunRawFunc func(args ...string) (string, error)
}

// Run returns the configured output and error, or delegates to RunFunc.
func (m *MockCommander) Run(args ...string) (string, error) {
	m.Calls = append(m.Calls, args)
	if m.RunFunc != nil {
		return m.RunFunc(args...)
	}
	return m.Output, m.Err
}

// RunRaw mirrors Run but is the no-trim variant. Tests that don't care about
// raw vs trimmed semantics fall through to Output/Err.
func (m *MockCommander) RunRaw(args ...string) (string, error) {
	m.Calls = append(m.Calls, args)
	if m.RunRawFunc != nil {
		return m.RunRawFunc(args...)
	}
	return m.Output, m.Err
}

func TestListSessions(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		err     error
		want    []tmux.Session
		wantErr bool
	}{
		{
			name:   "parses multiple sessions correctly",
			output: "dev|3|1|\nwork|5|0|\nmisc|1|0|",
			want: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
				{Name: "work", Windows: 5, Attached: false},
				{Name: "misc", Windows: 1, Attached: false},
			},
		},
		{
			name:   "parses single session",
			output: "main|2|0|",
			want: []tmux.Session{
				{Name: "main", Windows: 2, Attached: false},
			},
		},
		{
			name:   "returns empty slice when tmux server is not running",
			output: "",
			err:    fmt.Errorf("exit status 1"),
			want:   []tmux.Session{},
		},
		{
			name:   "returns empty slice when output is empty",
			output: "",
			want:   []tmux.Session{},
		},
		{
			name:   "attached is true when session_attached > 0",
			output: "session1|2|3|",
			want: []tmux.Session{
				{Name: "session1", Windows: 2, Attached: true},
			},
		},
		{
			name:   "attached is false when session_attached is 0",
			output: "session1|2|0|",
			want: []tmux.Session{
				{Name: "session1", Windows: 2, Attached: false},
			},
		},
		{
			name:   "handles session name with special characters",
			output: "my-project.v2|4|1|",
			want: []tmux.Session{
				{Name: "my-project.v2", Windows: 4, Attached: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockCommander{Output: tt.output, Err: tt.err}
			client := tmux.NewClient(mock)

			got, err := client.ListSessions()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d sessions, want %d", len(got), len(tt.want))
			}

			for i, session := range got {
				if session.Name != tt.want[i].Name {
					t.Errorf("session[%d].Name = %q, want %q", i, session.Name, tt.want[i].Name)
				}
				if session.Windows != tt.want[i].Windows {
					t.Errorf("session[%d].Windows = %d, want %d", i, session.Windows, tt.want[i].Windows)
				}
				if session.Attached != tt.want[i].Attached {
					t.Errorf("session[%d].Attached = %v, want %v", i, session.Attached, tt.want[i].Attached)
				}
			}
		})
	}
}

func TestListSessionsParsesPortalDir(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantDir string
	}{
		{
			name:    "parses the stamped @portal-dir into Session.Dir",
			output:  "dev|3|1|/Users/me/code/portal",
			wantDir: "/Users/me/code/portal",
		},
		{
			name:    "parses an absent @portal-dir to an empty Dir",
			output:  "dev|3|1|",
			wantDir: "",
		},
		{
			name:    "preserves a pipe character in the directory value",
			output:  "dev|3|1|/Users/me/weird|path/portal",
			wantDir: "/Users/me/weird|path/portal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockCommander{Output: tt.output}
			client := tmux.NewClient(mock)

			got, err := client.ListSessions()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d sessions, want 1", len(got))
			}
			if got[0].Dir != tt.wantDir {
				t.Errorf("session.Dir = %q, want %q", got[0].Dir, tt.wantDir)
			}
		})
	}
}

func TestListSessionsFormatStringIncludesPortalDir(t *testing.T) {
	mock := &MockCommander{Output: "dev|1|0|"}
	client := tmux.NewClient(mock)

	if _, err := client.ListSessions(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls) == 0 {
		t.Fatal("expected at least one tmux call")
	}
	args := strings.Join(mock.Calls[0], " ")
	if !strings.Contains(args, "#{@portal-dir}") {
		t.Errorf("list-sessions format string %q does not include #{@portal-dir}", args)
	}
}

func TestListSessionsFiltersUnderscorePrefixed(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []tmux.Session
	}{
		{
			name:   "filters _* names from mixed output",
			output: fmt.Sprintf("dev|2|0|\n%s|1|0|\nwork|3|1|\n%s|1|0|", tmux.PortalSaverName, tmux.PortalBootstrapName),
			want: []tmux.Session{
				{Name: "dev", Windows: 2, Attached: false},
				{Name: "work", Windows: 3, Attached: true},
			},
		},
		{
			name:   "all underscore sessions yields non-nil empty slice",
			output: fmt.Sprintf("%s|1|0|\n%s|1|0|", tmux.PortalSaverName, tmux.PortalBootstrapName),
			want:   []tmux.Session{},
		},
		{
			name:   "underscore mid-name is not filtered (HasPrefix not Contains)",
			output: "foo_bar|1|0|",
			want: []tmux.Session{
				{Name: "foo_bar", Windows: 1, Attached: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockCommander{Output: tt.output}
			client := tmux.NewClient(mock)

			got, err := client.ListSessions()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got == nil {
				t.Fatal("ListSessions returned nil slice, want non-nil")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d sessions (%v), want %d (%v)", len(got), got, len(tt.want), tt.want)
			}

			for i, session := range got {
				if session.Name != tt.want[i].Name {
					t.Errorf("session[%d].Name = %q, want %q", i, session.Name, tt.want[i].Name)
				}
				if session.Windows != tt.want[i].Windows {
					t.Errorf("session[%d].Windows = %d, want %d", i, session.Windows, tt.want[i].Windows)
				}
				if session.Attached != tt.want[i].Attached {
					t.Errorf("session[%d].Attached = %v, want %v", i, session.Attached, tt.want[i].Attached)
				}
			}
		})
	}
}

// TestListSessions_PortalSaverExcludedAtSource is a behavioural regression
// pin: the Sessions-list source (Client.ListSessions) must omit
// _portal-saver even when tmux's raw output includes it. Pinned at this
// layer rather than the preview layer because § Cross-cutting Seams >
// _portal-saver Self-Reference of the spec mandates filtering at list-
// population, not preview.
func TestListSessions_PortalSaverExcludedAtSource(t *testing.T) {
	// Raw tmux output deliberately includes _portal-saver alongside two
	// real user sessions to verify the filter strips only the internal
	// session.
	rawOutput := fmt.Sprintf("dev|2|0|\n%s|1|0|\nwork|3|1|", tmux.PortalSaverName)
	mock := &MockCommander{Output: rawOutput}
	client := tmux.NewClient(mock)

	got, err := client.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, s := range got {
		if s.Name == tmux.PortalSaverName {
			t.Fatalf("ListSessions returned %q in rendered list; must be excluded at source", tmux.PortalSaverName)
		}
	}

	wantNames := []string{"dev", "work"}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d sessions %v, want %d %v", len(got), got, len(wantNames), wantNames)
	}
	for i, name := range wantNames {
		if got[i].Name != name {
			t.Errorf("session[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

// TestListSessions_PortalSaverExclusionRefactorPin is a refactor-resistance
// pin: a future change that strips the underscore-prefix filter from
// Client.ListSessions (or from any wrapper that consumes its output) must
// fail this test. The check is deliberately worded to fail loudly with a
// pointer to § Cross-cutting Seams > _portal-saver Self-Reference so the
// reviewer who removes the filter sees the spec invariant they are
// breaking.
func TestListSessions_PortalSaverExclusionRefactorPin(t *testing.T) {
	// Mix of similar prefixes to also pin the exact-match-on-prefix
	// invariant: _portal-saver and _portal-bootstrap must be filtered;
	// _foo (any underscore-prefixed) must be filtered; pigeon and
	// pigeon-saver (no underscore prefix, mid-name 'saver' substring)
	// must NOT be filtered.
	rawOutput := fmt.Sprintf(
		"pigeon|1|0|\n%s|1|0|\npigeon-saver|1|0|\n%s|1|0|\n_foo|1|0|",
		tmux.PortalSaverName,
		tmux.PortalBootstrapName,
	)
	mock := &MockCommander{Output: rawOutput}
	client := tmux.NewClient(mock)

	got, err := client.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rendered := make(map[string]bool, len(got))
	for _, s := range got {
		rendered[s.Name] = true
	}

	mustExclude := []string{tmux.PortalSaverName, tmux.PortalBootstrapName, "_foo"}
	for _, name := range mustExclude {
		if rendered[name] {
			t.Errorf("rendered Sessions list contains %q; must be excluded at list-population source per spec § Cross-cutting Seams > _portal-saver Self-Reference", name)
		}
	}

	mustInclude := []string{"pigeon", "pigeon-saver"}
	for _, name := range mustInclude {
		if !rendered[name] {
			t.Errorf("rendered Sessions list missing %q; exclusion must be exact prefix-match on '_', not substring", name)
		}
	}
}

func TestServerRunning(t *testing.T) {
	t.Run("returns true when tmux server is running", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		got := client.ServerRunning()

		if !got {
			t.Error("ServerRunning() = false, want true")
		}
	})

	t.Run("returns false when no tmux server is running", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running on /tmp/tmux-501/default")}
		client := tmux.NewClient(mock)

		got := client.ServerRunning()

		if got {
			t.Error("ServerRunning() = true, want false")
		}
	})

	t.Run("calls tmux info to check server status", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		client.ServerRunning()

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"info"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		if mock.Calls[0][0] != "info" {
			t.Errorf("called with %q, want %q", mock.Calls[0][0], "info")
		}
	})
}

func TestHasSession(t *testing.T) {
	t.Run("returns true when session exists", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		got := client.HasSession("my-session")

		if !got {
			t.Error("HasSession() = false, want true")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "has-session -t =my-session"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns false when session does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("exit status 1")}
		client := tmux.NewClient(mock)

		got := client.HasSession("nonexistent")

		if got {
			t.Error("HasSession() = true, want false")
		}
	})

	t.Run("returns false when no tmux server running", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running on /tmp/tmux-501/default")}
		client := tmux.NewClient(mock)

		got := client.HasSession("any-session")

		if got {
			t.Error("HasSession() = true, want false")
		}
	})
}

// TestHasSessionUsesExactMatchPrefix is a regression test that documents the
// prefix-collision rationale for tmux's "=" exact-match target syntax.
//
// Without the "=" prefix, tmux's default `-t <session>` resolution matches by
// prefix. A killed session "foo" coexisting with a live "foo-2" would silently
// resolve `has-session -t foo` to "foo-2" (zero exit), causing the
// session-killed-externally bail path in the preview Enter sequence to be
// missed and the connector to attach to (or auto-create) the wrong session.
//
// Uniform use of "=<session>" across HasSession / SelectWindow / SelectPane /
// SwitchClient / attach-session closes this hole. This test pins the prefix
// at the HasSession entry point so any future refactor that drops it will
// fail loudly here.
//
// Spec: .workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md
// § Pre-select + attach sequence > Exact-match target syntax.
func TestHasSessionUsesExactMatchPrefix(t *testing.T) {
	mock := &MockCommander{}
	client := tmux.NewClient(mock)

	_ = client.HasSession("foo")

	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	got := mock.Calls[0]
	want := []string{"has-session", "-t", "=foo"}
	if len(got) != len(want) {
		t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Prefix-collision regression: the target MUST begin with "=" so tmux's
	// exact-match resolution kicks in. A killed "foo" coexisting with a
	// live "foo-2" would otherwise prefix-match "foo-2" and bypass the
	// bail path.
	if !strings.HasPrefix(got[2], "=") {
		t.Errorf("target %q lacks exact-match prefix '='; prefix-collision regression hazard", got[2])
	}

	// Drive a commander that simulates real tmux's exact-match semantics:
	// "=foo" matches the literal session "foo" only, while bare "foo"
	// would prefix-match the live "foo-2" (the hazard). We pin HasSession
	// to the exact-match arm so a killed "foo" with live "foo-2"
	// coexisting reports absent — the precondition for the bail path.
	t.Run("killed foo with live foo-2 reports absent", func(t *testing.T) {
		exactMock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if len(args) >= 3 && args[0] == "has-session" && args[1] == "-t" {
					// Live sessions on the simulated server: {"foo-2"}.
					// "=foo" must NOT match (foo is dead); "=foo-2"
					// matches. Bare "foo" would prefix-match "foo-2",
					// which is the hazard we're guarding against.
					switch args[2] {
					case "=foo":
						return "", fmt.Errorf("can't find session: foo")
					case "=foo-2":
						return "", nil
					case "foo":
						// If we hit this case, HasSession dropped the
						// "=" prefix and tmux prefix-matched "foo-2".
						return "", nil
					}
				}
				return "", fmt.Errorf("unexpected args: %v", args)
			},
		}
		c := tmux.NewClient(exactMock)
		if c.HasSession("foo") {
			t.Errorf("HasSession(\"foo\") = true while only live session is \"foo-2\"; prefix-collision regression")
		}
		if !c.HasSession("foo-2") {
			t.Errorf("HasSession(\"foo-2\") = false; expected true")
		}
	})
}

// TestHasSessionProbe pins the three-shape discriminator contract documented
// on Client.HasSessionProbe: (true, nil) on a zero tmux exit (session
// present); (false, err) when the underlying error unwraps to *exec.ExitError
// (session absent — caller may bail); (true, err) when the underlying error
// does NOT unwrap to *exec.ExitError (OS-layer fault — caller proceeds and
// logs).
//
// Spec: .workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md
// § Pre-select + attach sequence > step 1.
func TestHasSessionProbe(t *testing.T) {
	t.Run("returns (true, nil) when tmux exits zero", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		present, err := client.HasSessionProbe("my-session")

		if !present {
			t.Errorf("present = false, want true")
		}
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "has-session -t =my-session"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns (false, err) when tmux exits non-zero", func(t *testing.T) {
		// Synthetic *exec.ExitError simulating a real non-zero tmux exit.
		// Construct via exec.Command of a failing process so the returned
		// error is a genuine *exec.ExitError that errors.As can recover.
		exitErr := syntheticExitError(t)
		mock := &MockCommander{Err: &tmux.CommandError{Err: exitErr}}
		client := tmux.NewClient(mock)

		present, err := client.HasSessionProbe("nonexistent")

		if present {
			t.Errorf("present = true, want false")
		}
		if err == nil {
			t.Fatal("err = nil, want non-nil")
		}

		// The returned err preserves *CommandError shape.
		var cmdErr *tmux.CommandError
		if !errors.As(err, &cmdErr) {
			t.Errorf("errors.As(err, &cmdErr) = false; want *CommandError shape preserved")
		}

		// The underlying error unwraps to *exec.ExitError.
		var asExit *exec.ExitError
		if !errors.As(err, &asExit) {
			t.Errorf("errors.As(err, &exitErr) = false; want underlying *exec.ExitError")
		}
	})

	t.Run("returns (true, err) on OS-layer failure", func(t *testing.T) {
		// A non-ExitError underlying cause (e.g. *exec.Error from a PATH
		// lookup failure, or any other transport fault). The probe must
		// treat this as 'session present' so the caller proceeds rather
		// than falsely triggering the externally-killed bail UX.
		osErr := errors.New("exec: \"tmux\": executable file not found in $PATH")
		mock := &MockCommander{Err: &tmux.CommandError{Err: osErr}}
		client := tmux.NewClient(mock)

		present, err := client.HasSessionProbe("any-session")

		if !present {
			t.Errorf("present = false, want true on OS-layer failure")
		}
		if err == nil {
			t.Fatal("err = nil, want non-nil")
		}

		// errors.As against *exec.ExitError must fail.
		var asExit *exec.ExitError
		if errors.As(err, &asExit) {
			t.Errorf("errors.As(err, &exitErr) = true; want false for non-ExitError cause")
		}
	})
}

// syntheticExitError returns a real *exec.ExitError by running a process
// guaranteed to exit non-zero. Used so errors.As discrimination is exercised
// against a genuine exec.ExitError instance, not a synthetic stand-in.
func syntheticExitError(t *testing.T) *exec.ExitError {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit 1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("could not synthesize *exec.ExitError; got %T: %v", err, err)
	}
	return exitErr
}

func TestNewSession(t *testing.T) {
	t.Run("creates session with name and directory", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewSession("my-session", "/home/user/project", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "new-session -d -s my-session -c /home/user/project"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("includes shell-command when provided", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		shellCmd := "/bin/zsh -ic 'claude; exec /bin/zsh'"
		err := client.NewSession("my-session", "/home/user/project", shellCmd)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"new-session", "-d", "-s", "my-session", "-c", "/home/user/project", shellCmd}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("no shell-command argument when empty string", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewSession("my-session", "/home/user/project", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		// Should be exactly 6 args: new-session -d -s <name> -c <dir>
		if len(mock.Calls[0]) != 6 {
			t.Errorf("got %d args %v, want 6 args (no shell-command)", len(mock.Calls[0]), mock.Calls[0])
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.NewSession("my-session", "/some/dir", "")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestCurrentSessionName(t *testing.T) {
	t.Run("returns session name from tmux output", func(t *testing.T) {
		mock := &MockCommander{Output: "my-project-x7k2m9"}
		client := tmux.NewClient(mock)

		got, err := client.CurrentSessionName()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "my-project-x7k2m9" {
			t.Errorf("CurrentSessionName() = %q, want %q", got, "my-project-x7k2m9")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "display-message -p #{session_name}"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running")}
		client := tmux.NewClient(mock)

		_, err := client.CurrentSessionName()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestKillSession(t *testing.T) {
	t.Run("runs kill-session with session name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.KillSession("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "kill-session -t my-session"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("session not found")}
		client := tmux.NewClient(mock)

		err := client.KillSession("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSwitchClient(t *testing.T) {
	t.Run("runs switch-client with session name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SwitchClient("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "switch-client -t =my-session"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("session not found")}
		client := tmux.NewClient(mock)

		err := client.SwitchClient("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestStartServer(t *testing.T) {
	t.Run("starts tmux server successfully", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.StartServer()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := strings.Join([]string{"new-session", "-d", "-s", tmux.PortalBootstrapName}, " ")
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when start-server fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.StartServer()

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantMsg := "failed to start tmux server (bootstrap session)"
		if !strings.Contains(err.Error(), wantMsg) {
			t.Errorf("error %q does not contain %q", err.Error(), wantMsg)
		}

		// Verify the original error is wrapped
		wantWrapped := "tmux failed"
		if !strings.Contains(err.Error(), wantWrapped) {
			t.Errorf("error %q does not contain wrapped error %q", err.Error(), wantWrapped)
		}
	})

	t.Run("does not retry on failure", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("server start failed")}
		client := tmux.NewClient(mock)

		_ = client.StartServer()

		if len(mock.Calls) != 1 {
			t.Errorf("expected exactly 1 call (no retry), got %d", len(mock.Calls))
		}
	})
}

func TestEnsureServer(t *testing.T) {
	t.Run("returns false when server is already running", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", nil // server is running
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		started, err := client.EnsureServer()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if started {
			t.Error("EnsureServer() started = true, want false")
		}
	})

	t.Run("starts server and returns true when server is not running", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", fmt.Errorf("no server running")
				}
				if args[0] == "new-session" {
					return "", nil
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		started, err := client.EnsureServer()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !started {
			t.Error("EnsureServer() started = false, want true")
		}
	})

	t.Run("returns true and error when start-server fails", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", fmt.Errorf("no server running")
				}
				if args[0] == "new-session" {
					return "", fmt.Errorf("start failed")
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		started, err := client.EnsureServer()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !started {
			t.Error("EnsureServer() started = false, want true")
		}
	})

	t.Run("does not call start-server when server is running", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", nil // server is running
				}
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		_, _ = client.EnsureServer()

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		if mock.Calls[0][0] != "info" {
			t.Errorf("expected call to %q, got %q", "info", mock.Calls[0][0])
		}
	})
}

func TestRenameSession(t *testing.T) {
	t.Run("runs rename-session with old and new name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.RenameSession("old-name", "new-name")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "rename-session -t old-name new-name"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("session not found")}
		client := tmux.NewClient(mock)

		err := client.RenameSession("old-name", "new-name")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSetServerOption(t *testing.T) {
	t.Run("runs set-option -s with name and value", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SetServerOption("@portal-active-%3", "1")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "set-option -s @portal-active-%3 1"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.SetServerOption("@portal-active-%3", "1")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set server option") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "@portal-active-%3") {
			t.Errorf("error %q does not contain option name", err.Error())
		}
	})
}

func TestSetSessionOption(t *testing.T) {
	t.Run("runs set-option -t with session, name, and value", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SetSessionOption("_portal-saver", "destroy-unattached", "off")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "set-option -t _portal-saver destroy-unattached off"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("does not pass -g flag (session-scoped, not global)", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		_ = client.SetSessionOption("_portal-saver", "destroy-unattached", "off")

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		for _, arg := range mock.Calls[0] {
			if arg == "-g" {
				t.Errorf("SetSessionOption must not include -g flag, got args %v", mock.Calls[0])
			}
		}
	})

	t.Run("returns error wrapped with session and option name", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.SetSessionOption("_portal-saver", "destroy-unattached", "off")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set session option") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "destroy-unattached") {
			t.Errorf("error %q does not contain option name", err.Error())
		}
		if !strings.Contains(err.Error(), "_portal-saver") {
			t.Errorf("error %q does not contain session name", err.Error())
		}
	})
}

func TestNewDetachedSessionNoCwd(t *testing.T) {
	t.Run("creates detached session with name and shell command, no -c", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewDetachedSessionNoCwd("_portal-saver", "portal state daemon")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"new-session", "-d", "-s", "_portal-saver", "portal state daemon"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
		// Belt-and-braces: ensure no -c anywhere
		for _, arg := range mock.Calls[0] {
			if arg == "-c" {
				t.Errorf("NewDetachedSessionNoCwd must not include -c flag, got args %v", mock.Calls[0])
			}
		}
	})

	t.Run("omits shell-command argument when empty string", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewDetachedSessionNoCwd("_portal-saver", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		// Should be exactly 4 args: new-session -d -s <name>
		wantArgs := []string{"new-session", "-d", "-s", "_portal-saver"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.NewDetachedSessionNoCwd("_portal-saver", "portal state daemon")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create tmux session") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "_portal-saver") {
			t.Errorf("error %q does not contain session name", err.Error())
		}
	})
}

func TestGetServerOption(t *testing.T) {
	t.Run("returns value when option exists", func(t *testing.T) {
		mock := &MockCommander{Output: "1"}
		client := tmux.NewClient(mock)

		got, err := client.GetServerOption("@portal-active-%3")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "1" {
			t.Errorf("GetServerOption() = %q, want %q", got, "1")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "show-option -sv @portal-active-%3"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns ErrOptionNotFound when option does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: &tmux.CommandError{
			Stderr: "unknown option: @portal-active-%3",
			Err:    errors.New("exit status 1"),
		}}
		client := tmux.NewClient(mock)

		got, err := client.GetServerOption("@portal-active-%3")

		if got != "" {
			t.Errorf("GetServerOption() = %q, want empty string", got)
		}
		if !errors.Is(err, tmux.ErrOptionNotFound) {
			t.Errorf("GetServerOption() error = %v, want ErrOptionNotFound", err)
		}
	})
}

// TestGetServerOption_TransportError covers the propagation path: failures
// whose stderr does not match any optionAbsentStderrPatterns entry must
// surface to the caller with the wrapped *CommandError intact, so consumers
// (e.g. the daemon's restoring-marker check) can distinguish transport faults
// from genuine option absence.
func TestGetServerOption_TransportError(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{
			name:   "socket_connect_failure",
			stderr: "error connecting to /tmp/tmux-501//default (No such file or directory)",
		},
		{
			name:   "lost_server",
			stderr: "lost server",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdErr := &tmux.CommandError{Stderr: tc.stderr, Err: errors.New("exit status 1")}
			mock := &MockCommander{Err: cmdErr}
			client := tmux.NewClient(mock)

			got, err := client.GetServerOption("@portal-restoring")

			if got != "" {
				t.Errorf("GetServerOption() = %q, want empty string", got)
			}
			if err == nil {
				t.Fatal("GetServerOption() error = nil, want non-nil")
			}
			if errors.Is(err, tmux.ErrOptionNotFound) {
				t.Errorf("GetServerOption() error = %v, must not be ErrOptionNotFound", err)
			}
			var recovered *tmux.CommandError
			if !errors.As(err, &recovered) {
				t.Fatalf("errors.As did not recover *CommandError from %v (%T)", err, err)
			}
			if recovered.Stderr != tc.stderr {
				t.Errorf("recovered Stderr = %q, want %q", recovered.Stderr, tc.stderr)
			}
		})
	}
}

// TestGetServerOption_NonExitErrorPropagates covers the case where the
// underlying tmux invocation could not even produce stderr (e.g. exec lookup
// failure). The *CommandError still wraps the underlying cause but Stderr is
// empty; the discriminator must not treat the empty string as a match.
func TestGetServerOption_NonExitErrorPropagates(t *testing.T) {
	cmdErr := &tmux.CommandError{Stderr: "", Err: errors.New("exec: \"tmux\": not found")}
	mock := &MockCommander{Err: cmdErr}
	client := tmux.NewClient(mock)

	got, err := client.GetServerOption("@portal-restoring")

	if got != "" {
		t.Errorf("GetServerOption() = %q, want empty string", got)
	}
	if err == nil {
		t.Fatal("GetServerOption() error = nil, want non-nil")
	}
	if errors.Is(err, tmux.ErrOptionNotFound) {
		t.Errorf("GetServerOption() error = %v, must not be ErrOptionNotFound", err)
	}
	var recovered *tmux.CommandError
	if !errors.As(err, &recovered) {
		t.Fatalf("errors.As did not recover *CommandError from %v (%T)", err, err)
	}
	if recovered.Stderr != "" {
		t.Errorf("recovered Stderr = %q, want empty string", recovered.Stderr)
	}
}

func TestUnsetServerOption(t *testing.T) {
	t.Run("runs set-option -su with name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.UnsetServerOption("@portal-restoring")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "set-option -su @portal-restoring"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("succeeds when option does not exist", func(t *testing.T) {
		mock := &MockCommander{} // tmux set-option -su is a no-op for missing options
		client := tmux.NewClient(mock)

		err := client.UnsetServerOption("@nonexistent-option")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.UnsetServerOption("@portal-restoring")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to unset server option") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "@portal-restoring") {
			t.Errorf("error %q does not contain option name", err.Error())
		}
	})
}

func TestListPanes(t *testing.T) {
	t.Run("returns structural keys for session with multiple panes", func(t *testing.T) {
		mock := &MockCommander{Output: "my-session:0.0\nmy-session:0.1\nmy-session:0.2"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my-session:0.0", "my-session:0.1", "my-session:0.2"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "list-panes -t my-session -F " + tmux.StructuralKeyFormat
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns structural keys for multi-window multi-pane session", func(t *testing.T) {
		mock := &MockCommander{Output: "my-session:0.0\nmy-session:0.1\nmy-session:1.0\nmy-session:1.1"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my-session:0.0", "my-session:0.1", "my-session:1.0", "my-session:1.1"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with colons", func(t *testing.T) {
		mock := &MockCommander{Output: "my:project:0.0\nmy:project:0.1"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my:project")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my:project:0.0", "my:project:0.1"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with dots", func(t *testing.T) {
		mock := &MockCommander{Output: "my.project:0.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my.project")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my.project:0.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		if got[0] != want[0] {
			t.Errorf("pane[0] = %q, want %q", got[0], want[0])
		}
	})

	t.Run("returns empty slice when session has no panes", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("empty-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(got) != 0 {
			t.Fatalf("got %d panes, want 0", len(got))
		}
	})

	t.Run("returns error when session does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("can't find session: nonexistent")}
		client := tmux.NewClient(mock)

		_, err := client.ListPanes("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to list panes") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("error %q does not contain session name", err.Error())
		}
	})
}

func TestListAllPanes(t *testing.T) {
	t.Run("returns structural keys across multiple sessions", func(t *testing.T) {
		mock := &MockCommander{Output: "dev-abc:0.0\ndev-abc:0.1\nwork-xyz:0.0\nwork-xyz:1.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"dev-abc:0.0", "dev-abc:0.1", "work-xyz:0.0", "work-xyz:1.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("returns structural keys for multi-window multi-pane session", func(t *testing.T) {
		mock := &MockCommander{Output: "proj:0.0\nproj:0.1\nproj:1.0\nproj:1.1\nproj:2.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"proj:0.0", "proj:0.1", "proj:1.0", "proj:1.1", "proj:2.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with colons", func(t *testing.T) {
		mock := &MockCommander{Output: "my:project:0.0\nmy:project:0.1"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my:project:0.0", "my:project:0.1"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with dots", func(t *testing.T) {
		mock := &MockCommander{Output: "my.project:0.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my.project:0.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		if got[0] != want[0] {
			t.Errorf("pane[0] = %q, want %q", got[0], want[0])
		}
	})

	t.Run("returns error when underlying commander fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running on /tmp/tmux-501/default")}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err == nil {
			t.Fatalf("expected non-nil error, got nil")
		}
		if got != nil {
			t.Fatalf("expected nil slice on error, got %#v", got)
		}
	})

	// Legitimate-empty contract: exit 0 + empty stdout ⇒ ([]string{}, nil).
	// This is the distinguishability boundary between failure mode (a)
	// "tmux failed" (non-nil err) and failure mode (b) "no panes exist"
	// (nil err, empty slice). Phase 2's hazard guard in cleanStaleAdapter /
	// `portal clean` relies on this shape to detect mode (b) and refuse to
	// wipe markers when the live pane set is authoritatively empty.
	// Do not delete this subtest or the whitespace-only sibling below.
	t.Run("returns empty slice when output is empty", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatalf("expected non-nil empty slice, got nil")
		}
		if len(got) != 0 {
			t.Fatalf("got %d panes, want 0", len(got))
		}
	})

	t.Run("returns empty slice when output is whitespace-only", func(t *testing.T) {
		mock := &MockCommander{Output: "  \n\n\t\n "}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatalf("expected non-nil empty slice, got nil")
		}
		if len(got) != 0 {
			t.Fatalf("got %d panes, want 0; slice = %#v", len(got), got)
		}
	})

	t.Run("calls list-panes with -a flag and structural key format", func(t *testing.T) {
		mock := &MockCommander{Output: "sess:0.0"}
		client := tmux.NewClient(mock)

		_, _ = client.ListAllPanes()

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "list-panes -a -F " + tmux.StructuralKeyFormat
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})
}

func TestResolveStructuralKey(t *testing.T) {
	t.Run("returns structural key for valid pane ID", func(t *testing.T) {
		mock := &MockCommander{Output: "my-project:0.1"}
		client := tmux.NewClient(mock)

		got, err := client.ResolveStructuralKey("%3")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "my-project:0.1" {
			t.Errorf("ResolveStructuralKey() = %q, want %q", got, "my-project:0.1")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "display-message -p -t %3 " + tmux.StructuralKeyFormat
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error for invalid pane ID", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("can't find pane: %%99")}
		client := tmux.NewClient(mock)

		_, err := client.ResolveStructuralKey("%99")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "%99") {
			t.Errorf("error %q does not contain pane ID", err.Error())
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running")}
		client := tmux.NewClient(mock)

		_, err := client.ResolveStructuralKey("%0")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve structural key") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "%0") {
			t.Errorf("error %q does not contain pane ID", err.Error())
		}
	})
}

func TestEnsureServerThenListSessions(t *testing.T) {
	t.Run("bootstrap session is queryable and server is running after EnsureServer starts server", func(t *testing.T) {
		infoCallCount := 0
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				switch args[0] {
				case "info":
					infoCallCount++
					if infoCallCount == 1 {
						return "", fmt.Errorf("no server running")
					}
					return "", nil
				case "new-session":
					return "", nil
				case "list-sessions":
					return fmt.Sprintf("%s|1|0|", tmux.PortalBootstrapName), nil
				default:
					t.Fatalf("unexpected command: %v", args)
					return "", nil
				}
			},
		}
		client := tmux.NewClient(mock)

		// Step 1: EnsureServer should start the server
		started, err := client.EnsureServer()
		if err != nil {
			t.Fatalf("EnsureServer() unexpected error: %v", err)
		}
		if !started {
			t.Error("EnsureServer() started = false, want true")
		}

		// Step 2: ListSessions filters _* sessions, so the reserved
		// bootstrap session must NOT appear in the user-facing slice.
		sessions, err := client.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions() unexpected error: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("ListSessions() returned %d sessions, want 0 (bootstrap session must be filtered): %v", len(sessions), sessions)
		}

		// Step 3: ServerRunning should return true
		if !client.ServerRunning() {
			t.Error("ServerRunning() = false, want true")
		}

		// Verify exactly 4 mock calls in correct order
		if len(mock.Calls) != 4 {
			t.Fatalf("expected 4 calls, got %d: %v", len(mock.Calls), mock.Calls)
		}

		wantCalls := [][]string{
			{"info"},
			{"new-session", "-d", "-s", tmux.PortalBootstrapName},
			{"list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}|#{@portal-dir}"},
			{"info"},
		}
		for i, wantArgs := range wantCalls {
			gotArgs := strings.Join(mock.Calls[i], " ")
			wantJoined := strings.Join(wantArgs, " ")
			if gotArgs != wantJoined {
				t.Errorf("call[%d] = %q, want %q", i, gotArgs, wantJoined)
			}
		}
	})
}

func TestSendKeys(t *testing.T) {
	t.Run("sends command followed by Enter to pane", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SendKeys("%3", "claude --resume abc123")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"send-keys", "-t", "%3", "claude --resume abc123", "Enter"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("returns error when pane does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: errors.New("can't find pane: %99")}
		client := tmux.NewClient(mock)

		err := client.SendKeys("%99", "some-command")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to send keys") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "%99") {
			t.Errorf("error %q does not contain pane ID", err.Error())
		}
	})
}

func TestListSessionNames(t *testing.T) {
	t.Run("returns just the names from list-sessions output", func(t *testing.T) {
		mock := &MockCommander{Output: "dev|3|1|\nwork|5|0|"}
		client := tmux.NewClient(mock)

		got, err := client.ListSessionNames()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"dev", "work"}
		if len(got) != len(want) {
			t.Fatalf("got %d names %v, want %d %v", len(got), got, len(want), want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("name[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns empty slice when no sessions exist", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ListSessionNames()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}

func TestShowEnvironment(t *testing.T) {
	t.Run("returns raw output from show-environment for the named session", func(t *testing.T) {
		mock := &MockCommander{Output: "LANG=en_US.UTF-8\nTERM=xterm-256color"}
		client := tmux.NewClient(mock)

		got, err := client.ShowEnvironment("work")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "LANG=en_US.UTF-8\nTERM=xterm-256color" {
			t.Errorf("ShowEnvironment() = %q, want %q", got, "LANG=en_US.UTF-8\nTERM=xterm-256color")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "show-environment -t work"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns wrapped error containing session name when tmux fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("can't find session")}
		client := tmux.NewClient(mock)

		_, err := client.ShowEnvironment("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to show environment") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("error %q does not contain session name", err.Error())
		}
	})

	t.Run("returns empty string when output is empty", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ShowEnvironment("empty")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("ShowEnvironment() = %q, want empty string", got)
		}
	})
}

func TestListAllPanesWithFormat(t *testing.T) {
	t.Run("returns raw output from list-panes -a with the given format", func(t *testing.T) {
		mock := &MockCommander{Output: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh"}
		client := tmux.NewClient(mock)

		format := "#{session_name}|||#{window_index}|||#{window_name}|||#{window_layout}|||#{window_zoomed_flag}|||#{window_active}|||#{pane_index}|||#{pane_current_path}|||#{pane_active}|||#{pane_current_command}"
		got, err := client.ListAllPanesWithFormat(format)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh" {
			t.Errorf("ListAllPanesWithFormat() = %q, want raw output", got)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"list-panes", "-a", "-F", format}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("returns wrapped error when tmux fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running")}
		client := tmux.NewClient(mock)

		_, err := client.ListAllPanesWithFormat("#{session_name}")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to list panes") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})
}

func TestCapturePane(t *testing.T) {
	t.Run("uses capture-pane -e -p -S - -t <target> verbatim", func(t *testing.T) {
		mock := &MockCommander{
			RunRawFunc: func(args ...string) (string, error) {
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		_, err := client.CapturePane("my-session:0.1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"capture-pane", "-e", "-p", "-S", "-", "-t", "my-session:0.1"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("preserves trailing whitespace and ANSI escapes via RunRaw", func(t *testing.T) {
		raw := "abc\n  \x1b[31m"
		mock := &MockCommander{
			RunRawFunc: func(args ...string) (string, error) {
				return raw, nil
			},
		}
		client := tmux.NewClient(mock)

		got, err := client.CapturePane("work:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != raw {
			t.Errorf("CapturePane() = %q, want %q (raw output must not be trimmed)", got, raw)
		}
	})

	t.Run("propagates errors with target in message", func(t *testing.T) {
		mock := &MockCommander{
			RunRawFunc: func(args ...string) (string, error) {
				return "", fmt.Errorf("can't find pane")
			},
		}
		client := tmux.NewClient(mock)

		_, err := client.CapturePane("missing:0.0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to capture pane") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "missing:0.0") {
			t.Errorf("error %q does not contain target", err.Error())
		}
	})
}

func TestShowAllServerOptions(t *testing.T) {
	t.Run("invokes show-options -s and returns output", func(t *testing.T) {
		mock := &MockCommander{Output: "@portal-skeleton-foo__0.0 \"1\"\n@portal-restoring \"1\""}
		client := tmux.NewClient(mock)

		got, err := client.ShowAllServerOptions()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "@portal-skeleton-foo__0.0 \"1\"\n@portal-restoring \"1\"" {
			t.Errorf("ShowAllServerOptions() = %q, want raw output", got)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "show-options -s"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux exploded")}
		client := tmux.NewClient(mock)

		_, err := client.ShowAllServerOptions()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to show server options") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})
}

func TestTryGetServerOption(t *testing.T) {
	t.Run("returns value and found=true when option exists", func(t *testing.T) {
		mock := &MockCommander{Output: "1"}
		client := tmux.NewClient(mock)

		val, found, err := client.TryGetServerOption("@portal-restoring")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Errorf("found = false, want true")
		}
		if val != "1" {
			t.Errorf("value = %q, want %q", val, "1")
		}
	})

	t.Run("returns found=false and no error when option not found", func(t *testing.T) {
		mock := &MockCommander{Err: &tmux.CommandError{
			Stderr: "unknown option: @portal-restoring",
			Err:    errors.New("exit status 1"),
		}}
		client := tmux.NewClient(mock)

		val, found, err := client.TryGetServerOption("@portal-restoring")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Errorf("found = true, want false")
		}
		if val != "" {
			t.Errorf("value = %q, want empty", val)
		}
	})
}

// TestTryGetServerOption_PropagatesTransportError verifies that
// TryGetServerOption's previously-dead transport-error branch now fires:
// a *CommandError whose stderr does not match any absence pattern must
// surface as ("", false, non-nil err) with the wrapped *CommandError
// recoverable via errors.As. This is the contract the daemon's
// restoring-marker check relies on to distinguish absence from transport
// failure.
func TestTryGetServerOption_PropagatesTransportError(t *testing.T) {
	cmdErr := &tmux.CommandError{
		Stderr: "error connecting to /tmp/tmux-501//default (No such file or directory)",
		Err:    errors.New("exit status 1"),
	}
	mock := &MockCommander{Err: cmdErr}
	client := tmux.NewClient(mock)

	val, found, err := client.TryGetServerOption("@portal-restoring")

	if val != "" {
		t.Errorf("value = %q, want empty", val)
	}
	if found {
		t.Errorf("found = true, want false")
	}
	if err == nil {
		t.Fatal("TryGetServerOption() error = nil, want non-nil")
	}
	var recovered *tmux.CommandError
	if !errors.As(err, &recovered) {
		t.Fatalf("errors.As did not recover *CommandError from %v (%T)", err, err)
	}
	if recovered.Stderr != cmdErr.Stderr {
		t.Errorf("recovered Stderr = %q, want %q", recovered.Stderr, cmdErr.Stderr)
	}
}

func TestNewSessionWithCommand(t *testing.T) {
	t.Run("creates session with name, cwd, and shell-command", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		shellCmd := "sh -c 'portal state hydrate --fifo X --file Y --hook-key Z; exec $SHELL'"
		err := client.NewSessionWithCommand("work", "/Users/me/project", shellCmd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		want := []string{"new-session", "-d", "-s", "work", "-c", "/Users/me/project", shellCmd}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("omits -c when cwd is empty", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewSessionWithCommand("work", "", "echo hi")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"new-session", "-d", "-s", "work", "echo hi"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("omits trailing shell-command arg when empty", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewSessionWithCommand("work", "/tmp", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"new-session", "-d", "-s", "work", "-c", "/tmp"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.NewSessionWithCommand("work", "/tmp", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create session") {
			t.Errorf("error %q lacks expected message", err.Error())
		}
		if !strings.Contains(err.Error(), `"work"`) {
			t.Errorf("error %q lacks session name", err.Error())
		}
	})
}

func TestNewWindow(t *testing.T) {
	t.Run("creates window with target, name, cwd, and shell-command", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewWindow("work:", "code", "/work", "echo hi")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"new-window", "-t", "work:", "-n", "code", "-c", "/work", "echo hi"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("omits -n when name is empty", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewWindow("work:", "", "/work", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"new-window", "-t", "work:", "-c", "/work"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
	})

	t.Run("omits -c when cwd is empty", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewWindow("work:", "code", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"new-window", "-t", "work:", "-n", "code"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.NewWindow("work:", "code", "", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create window") {
			t.Errorf("error %q lacks expected message", err.Error())
		}
	})
}

func TestSplitWindow(t *testing.T) {
	t.Run("splits window with cwd and shell-command", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SplitWindow("work:0", "/work", "echo hi")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"split-window", "-t", "work:0", "-c", "/work", "echo hi"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("omits -c when cwd is empty", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SplitWindow("work:0", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"split-window", "-t", "work:0"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.SplitWindow("work:0", "", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to split window") {
			t.Errorf("error %q lacks expected message", err.Error())
		}
	})
}

func TestSetSessionEnvironment(t *testing.T) {
	t.Run("sets environment variable on session", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SetSessionEnvironment("work", "LANG", "en_US.UTF-8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"set-environment", "-t", "work", "LANG", "en_US.UTF-8"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.SetSessionEnvironment("work", "LANG", "en_US.UTF-8")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set env") {
			t.Errorf("error %q lacks expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "LANG") {
			t.Errorf("error %q lacks env var name", err.Error())
		}
	})
}

func TestSelectLayout(t *testing.T) {
	t.Run("invokes select-layout with composed window target and saved layout string", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SelectLayout("work", 1, "abcd,80x24,0,0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"select-layout", "-t", "work:1", "abcd,80x24,0,0"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.SelectLayout("work", 0, "tiled")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to select-layout") {
			t.Errorf("error %q lacks expected prefix", err.Error())
		}
		if !strings.Contains(err.Error(), "work:0") {
			t.Errorf("error %q lacks target context", err.Error())
		}
	})
}

func TestSelectWindow(t *testing.T) {
	t.Run("invokes select-window with composed session:window target with exact-match prefix", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SelectWindow("work", 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"select-window", "-t", "=work:2"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("issues select-window exactly once", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		if err := client.SelectWindow("work", 2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 Commander.Run call, got %d", len(mock.Calls))
		}
	})

	t.Run("returns nil on zero exit", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		if err := client.SelectWindow("work", 0); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.SelectWindow("work", 0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to select-window") {
			t.Errorf("error %q lacks expected prefix", err.Error())
		}
		if !strings.Contains(err.Error(), "work:0") {
			t.Errorf("error %q lacks target context", err.Error())
		}
	})

	t.Run("wraps *CommandError so errors.As recovers it", func(t *testing.T) {
		cmdErr := &tmux.CommandError{Stderr: "can't find window: 99", Err: fmt.Errorf("exit status 1")}
		mock := &MockCommander{Err: cmdErr}
		client := tmux.NewClient(mock)

		err := client.SelectWindow("work", 99)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var recovered *tmux.CommandError
		if !errors.As(err, &recovered) {
			t.Fatalf("errors.As did not recover *CommandError from %T: %v", err, err)
		}
		if recovered.Stderr != "can't find window: 99" {
			t.Errorf("recovered Stderr = %q, want %q", recovered.Stderr, "can't find window: 99")
		}
		if !strings.Contains(err.Error(), "work:99") {
			t.Errorf("error %q lacks target context", err.Error())
		}
	})

	t.Run("prepends exact-match prefix to session segment", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		if err := client.SelectWindow("work", 2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := mock.Calls[0]
		if len(got) < 3 {
			t.Fatalf("call too short: %v", got)
		}
		if !strings.HasPrefix(got[2], "=") {
			t.Errorf("target %q lacks exact-match prefix '='", got[2])
		}
	})
}

func TestSelectPane(t *testing.T) {
	t.Run("invokes select-pane with composed window.pane target with exact-match prefix", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SelectPane("work", 2, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"select-pane", "-t", "=work:2.3"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.SelectPane("work", 0, 0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to select-pane") {
			t.Errorf("error %q lacks expected prefix", err.Error())
		}
		if !strings.Contains(err.Error(), "work:0.0") {
			t.Errorf("error %q lacks target context", err.Error())
		}
	})
}

func TestResizePaneZoom(t *testing.T) {
	t.Run("invokes resize-pane -Z with composed window.pane target with exact-match prefix", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.ResizePaneZoom("work", 1, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"resize-pane", "-Z", "-t", "=work:1.2"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.ResizePaneZoom("work", 0, 0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resize-pane -Z") {
			t.Errorf("error %q lacks expected prefix", err.Error())
		}
		if !strings.Contains(err.Error(), "work:0.0") {
			t.Errorf("error %q lacks target context", err.Error())
		}
	})
}

func TestListPanesInSession(t *testing.T) {
	t.Run("invokes list-panes -s -t <session> with window:pane format", func(t *testing.T) {
		mock := &MockCommander{Output: "0:0"}
		client := tmux.NewClient(mock)

		_, err := client.ListPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"list-panes", "-s", "-t", "work", "-F", "#{window_index}:#{pane_index}"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("parses single pane line", func(t *testing.T) {
		mock := &MockCommander{Output: "0:0"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []tmux.PaneCoord{{Window: 0, Pane: 0}}
		if len(got) != len(want) {
			t.Fatalf("got %d coords %v, want %d", len(got), got, len(want))
		}
		if got[0] != want[0] {
			t.Errorf("got[0] = %+v, want %+v", got[0], want[0])
		}
	})

	t.Run("parses multiple panes across windows", func(t *testing.T) {
		mock := &MockCommander{Output: "0:0\n0:1\n1:0\n1:1\n1:2"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []tmux.PaneCoord{
			{Window: 0, Pane: 0},
			{Window: 0, Pane: 1},
			{Window: 1, Pane: 0},
			{Window: 1, Pane: 1},
			{Window: 1, Pane: 2},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d coords %v, want %d", len(got), got, len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("sorts coords by window then pane", func(t *testing.T) {
		// tmux output deliberately out of order.
		mock := &MockCommander{Output: "1:2\n0:1\n1:0\n0:0"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []tmux.PaneCoord{
			{Window: 0, Pane: 0},
			{Window: 0, Pane: 1},
			{Window: 1, Pane: 0},
			{Window: 1, Pane: 2},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d coords %v, want %d", len(got), got, len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("returns empty slice when output is empty", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ListPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d coords %v, want 0", len(got), got)
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("session not found")}
		client := tmux.NewClient(mock)

		_, err := client.ListPanesInSession("work")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "work") {
			t.Errorf("error %q lacks session context", err.Error())
		}
		if !strings.Contains(err.Error(), "session not found") {
			t.Errorf("error %q does not wrap underlying error", err.Error())
		}
	})

	t.Run("returns error on unexpected line format", func(t *testing.T) {
		mock := &MockCommander{Output: "garbage-line"}
		client := tmux.NewClient(mock)

		_, err := client.ListPanesInSession("work")
		if err == nil {
			t.Fatal("expected error for malformed line, got nil")
		}
	})

	t.Run("returns error on non-integer window", func(t *testing.T) {
		mock := &MockCommander{Output: "abc:0"}
		client := tmux.NewClient(mock)

		_, err := client.ListPanesInSession("work")
		if err == nil {
			t.Fatal("expected error for non-integer window, got nil")
		}
	})

	t.Run("returns error on non-integer pane", func(t *testing.T) {
		mock := &MockCommander{Output: "0:abc"}
		client := tmux.NewClient(mock)

		_, err := client.ListPanesInSession("work")
		if err == nil {
			t.Fatal("expected error for non-integer pane, got nil")
		}
	})

	t.Run("skips blank lines and trims whitespace", func(t *testing.T) {
		mock := &MockCommander{Output: "0:0\n\n  0:1  \n"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []tmux.PaneCoord{
			{Window: 0, Pane: 0},
			{Window: 0, Pane: 1},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d coords %v, want %d", len(got), got, len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})
}

func TestRespawnPane(t *testing.T) {
	t.Run("kills existing process and respawns with shell command", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.RespawnPane("work:0.0", "sh -c 'echo hi; exec $SHELL'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		want := []string{"respawn-pane", "-k", "-t", "work:0.0", "sh -c 'echo hi; exec $SHELL'"}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns wrapped error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: errors.New("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.RespawnPane("work:0.0", "sh -c 'x'")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to respawn-pane") {
			t.Errorf("error %q lacks expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "work:0.0") {
			t.Errorf("error %q lacks pane target", err.Error())
		}
	})
}

func TestPaneTarget(t *testing.T) {
	tests := []struct {
		name    string
		session string
		window  int
		pane    int
		want    string
	}{
		{
			name:    "zero indices",
			session: "work",
			window:  0,
			pane:    0,
			want:    "work:0.0",
		},
		{
			name:    "non-zero indices",
			session: "my-project",
			window:  2,
			pane:    3,
			want:    "my-project:2.3",
		},
		{
			name:    "session name with hyphens and digits",
			session: "proj-abc123",
			window:  10,
			pane:    11,
			want:    "proj-abc123:10.11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tmux.PaneTarget(tt.session, tt.window, tt.pane)
			if got != tt.want {
				t.Errorf("PaneTarget(%q, %d, %d) = %q, want %q",
					tt.session, tt.window, tt.pane, got, tt.want)
			}
		})
	}
}

func TestListWindowsAndPanesInSession(t *testing.T) {
	// us is the ASCII unit separator (\x1f) used as the field delimiter in the
	// list-panes -F format string. Tests construct fixtures using this constant
	// so the chosen delimiter is visible at the call site.
	const us = "\x1f"

	t.Run("it uses the cmd.Run interface with list-panes -s -t <session> and unit-separator format", func(t *testing.T) {
		mock := &MockCommander{Output: "0" + us + "main" + us + "0"}
		client := tmux.NewClient(mock)

		_, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		want := []string{
			"list-panes", "-s", "-t", "work",
			"-F", "#{window_index}\x1f#{window_name}\x1f#{pane_index}",
		}
		got := mock.Calls[0]
		if len(got) != len(want) {
			t.Fatalf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("it returns window-grouped panes ordered by window_index then pane_index", func(t *testing.T) {
		mock := &MockCommander{Output: strings.Join([]string{
			"0" + us + "editor" + us + "0",
			"0" + us + "editor" + us + "1",
			"1" + us + "logs" + us + "0",
			"1" + us + "logs" + us + "1",
			"2" + us + "repl" + us + "0",
			"2" + us + "repl" + us + "1",
		}, "\n")}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0, 1}},
			{WindowIndex: 1, WindowName: "logs", PaneIndices: []int{0, 1}},
			{WindowIndex: 2, WindowName: "repl", PaneIndices: []int{0, 1}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it preserves non-contiguous window_index values verbatim", func(t *testing.T) {
		mock := &MockCommander{Output: strings.Join([]string{
			"0" + us + "alpha" + us + "0",
			"2" + us + "beta" + us + "0",
			"5" + us + "gamma" + us + "0",
		}, "\n")}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "alpha", PaneIndices: []int{0}},
			{WindowIndex: 2, WindowName: "beta", PaneIndices: []int{0}},
			{WindowIndex: 5, WindowName: "gamma", PaneIndices: []int{0}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it preserves base-index 1 raw values", func(t *testing.T) {
		mock := &MockCommander{Output: strings.Join([]string{
			"1" + us + "first" + us + "1",
			"1" + us + "first" + us + "2",
			"2" + us + "second" + us + "1",
		}, "\n")}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 1, WindowName: "first", PaneIndices: []int{1, 2}},
			{WindowIndex: 2, WindowName: "second", PaneIndices: []int{1}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it preserves window names containing whitespace", func(t *testing.T) {
		mock := &MockCommander{Output: "0" + us + "my window name" + us + "0"}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "my window name", PaneIndices: []int{0}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it preserves window names containing the pipe delimiter", func(t *testing.T) {
		// The unit-separator delimiter is non-printable, so a pipe character in
		// a window name must round-trip intact.
		mock := &MockCommander{Output: "0" + us + "name|with|pipes" + us + "0"}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "name|with|pipes", PaneIndices: []int{0}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it groups multiple panes within the same window correctly", func(t *testing.T) {
		// Output is deliberately out of order to exercise pane-index sorting.
		mock := &MockCommander{Output: strings.Join([]string{
			"0" + us + "main" + us + "2",
			"0" + us + "main" + us + "0",
			"0" + us + "main" + us + "1",
		}, "\n")}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1, 2}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it preserves first-seen window name when later rows share the index", func(t *testing.T) {
		// tmux always reports the same window name for a given window_index, but
		// the implementation documents "first-seen wins"; lock that contract.
		mock := &MockCommander{Output: strings.Join([]string{
			"0" + us + "first-seen" + us + "0",
			"0" + us + "ignored" + us + "1",
		}, "\n")}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "first-seen", PaneIndices: []int{0, 1}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it sorts windows ascending even when tmux output is unordered", func(t *testing.T) {
		mock := &MockCommander{Output: strings.Join([]string{
			"2" + us + "two" + us + "0",
			"0" + us + "zero" + us + "0",
			"1" + us + "one" + us + "0",
		}, "\n")}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "zero", PaneIndices: []int{0}},
			{WindowIndex: 1, WindowName: "one", PaneIndices: []int{0}},
			{WindowIndex: 2, WindowName: "two", PaneIndices: []int{0}},
		}
		assertWindowGroups(t, got, want)
	})

	t.Run("it returns an error when tmux exits non-zero", func(t *testing.T) {
		mock := &MockCommander{Output: "", Err: errors.New("exit status 1: no such session")}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err == nil {
			t.Fatalf("expected non-nil error, got nil")
		}
		if got != nil {
			t.Errorf("expected nil slice on error, got %+v", got)
		}
	})

	t.Run("it returns an empty slice when stdout is empty and exit is zero", func(t *testing.T) {
		mock := &MockCommander{Output: "", Err: nil}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatalf("expected non-nil empty slice, got nil")
		}
		if len(got) != 0 {
			t.Errorf("expected length 0, got %d (%+v)", len(got), got)
		}
	})

	t.Run("it returns an empty slice for whitespace-only stdout", func(t *testing.T) {
		mock := &MockCommander{Output: "\n", Err: nil}
		client := tmux.NewClient(mock)

		got, err := client.ListWindowsAndPanesInSession("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatalf("expected non-nil empty slice, got nil")
		}
		if len(got) != 0 {
			t.Errorf("expected length 0, got %d (%+v)", len(got), got)
		}
	})

	t.Run("the wrapped error includes the session name", func(t *testing.T) {
		const sessionName = "my-session-name"
		mock := &MockCommander{Err: errors.New("exit status 1")}
		client := tmux.NewClient(mock)

		_, err := client.ListWindowsAndPanesInSession(sessionName)
		if err == nil {
			t.Fatalf("expected non-nil error, got nil")
		}
		if !strings.Contains(err.Error(), sessionName) {
			t.Errorf("error %q does not contain session name %q", err.Error(), sessionName)
		}
	})

	t.Run("the wrapped error preserves the original via errors.Is", func(t *testing.T) {
		sentinel := errors.New("sentinel tmux failure")
		mock := &MockCommander{Err: sentinel}
		client := tmux.NewClient(mock)

		_, err := client.ListWindowsAndPanesInSession("work")
		if err == nil {
			t.Fatalf("expected non-nil error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false; err = %v", err)
		}
	})

	t.Run("the wrapped error uses the spec-mandated prefix without quoting the session name", func(t *testing.T) {
		// Spec mandates the wrap shape "list windows and panes for session %s: %w"
		// — bare %s, not %q. Lock the prefix and that the session name appears
		// unquoted so the message is grep-friendly and matches the spec contract.
		const sessionName = "work"
		sentinel := errors.New("boom")
		mock := &MockCommander{Err: sentinel}
		client := tmux.NewClient(mock)

		_, err := client.ListWindowsAndPanesInSession(sessionName)
		if err == nil {
			t.Fatalf("expected non-nil error, got nil")
		}
		want := "list windows and panes for session " + sessionName + ": boom"
		if err.Error() != want {
			t.Errorf("err.Error() = %q, want %q", err.Error(), want)
		}
	})
}

// assertWindowGroups compares two []WindowGroup slices field-by-field with
// useful diff messages. Centralised here so the multiple table-style tests for
// ListWindowsAndPanesInSession share one assertion shape.
func assertWindowGroups(t *testing.T, got, want []tmux.WindowGroup) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d groups %+v, want %d groups %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i].WindowIndex != want[i].WindowIndex {
			t.Errorf("group[%d].WindowIndex = %d, want %d", i, got[i].WindowIndex, want[i].WindowIndex)
		}
		if got[i].WindowName != want[i].WindowName {
			t.Errorf("group[%d].WindowName = %q, want %q", i, got[i].WindowName, want[i].WindowName)
		}
		if len(got[i].PaneIndices) != len(want[i].PaneIndices) {
			t.Errorf("group[%d].PaneIndices length = %d (%v), want %d (%v)",
				i, len(got[i].PaneIndices), got[i].PaneIndices,
				len(want[i].PaneIndices), want[i].PaneIndices)
			continue
		}
		for j := range want[i].PaneIndices {
			if got[i].PaneIndices[j] != want[i].PaneIndices[j] {
				t.Errorf("group[%d].PaneIndices[%d] = %d, want %d",
					i, j, got[i].PaneIndices[j], want[i].PaneIndices[j])
			}
		}
	}
}

func TestCommandError_Error(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		err    error
		want   string
	}{
		{
			name:   "stderr and err both present uses colon-space separator",
			stderr: "invalid option: @foo",
			err:    errors.New("exit status 1"),
			want:   "exit status 1: invalid option: @foo",
		},
		{
			name:   "stderr trimmed in rendered output",
			stderr: "  invalid option: @foo\n",
			err:    errors.New("exit status 1"),
			want:   "exit status 1: invalid option: @foo",
		},
		{
			name:   "empty stderr falls back to bare err message",
			stderr: "",
			err:    errors.New("exit status 1"),
			want:   "exit status 1",
		},
		{
			name:   "whitespace-only stderr falls back to bare err message",
			stderr: "\n   \t\n",
			err:    errors.New("exit status 1"),
			want:   "exit status 1",
		},
		{
			name:   "nil err returns trimmed stderr",
			stderr: "  some stderr  \n",
			err:    nil,
			want:   "some stderr",
		},
		{
			name:   "both fields empty returns sentinel no-error string",
			stderr: "",
			err:    nil,
			want:   "<no error>",
		},
		{
			name:   "nil err with whitespace-only stderr returns sentinel",
			stderr: "  \n\t",
			err:    nil,
			want:   "<no error>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := &tmux.CommandError{Stderr: tt.stderr, Err: tt.err}
			if got := ce.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandError_Unwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	ce := &tmux.CommandError{Stderr: "ignored", Err: sentinel}
	if got := ce.Unwrap(); got != sentinel {
		t.Errorf("Unwrap() = %v, want %v", got, sentinel)
	}
	if !errors.Is(ce, sentinel) {
		t.Errorf("errors.Is(ce, sentinel) = false, want true")
	}
}

func TestCommandError_UnwrapNil(t *testing.T) {
	ce := &tmux.CommandError{Stderr: "only stderr", Err: nil}
	if got := ce.Unwrap(); got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestCommandError_ErrorsAsThroughFmtWrap(t *testing.T) {
	inner := &tmux.CommandError{Stderr: "invalid option: @foo", Err: errors.New("exit status 1")}
	wrapped := fmt.Errorf("ctx: %w", inner)

	var got *tmux.CommandError
	if !errors.As(wrapped, &got) {
		t.Fatalf("errors.As did not extract *CommandError from %v", wrapped)
	}
	if got.Stderr != "invalid option: @foo" {
		t.Errorf("extracted Stderr = %q, want %q", got.Stderr, "invalid option: @foo")
	}
	if got.Err == nil || got.Err.Error() != "exit status 1" {
		t.Errorf("extracted Err = %v, want exit status 1", got.Err)
	}
}

func TestCommandError_StructLiteralConstruction(t *testing.T) {
	// Confirms the type is constructable as a bare struct literal from an
	// external package — the contract that mocks rely on (no NewCommandError
	// factory, fields Stderr and Err remain exported).
	var _ error = &tmux.CommandError{Stderr: "x", Err: errors.New("y")}
}

func TestActivePaneCurrentPath(t *testing.T) {
	t.Run("reads only the active pane via display-message, not list-panes", func(t *testing.T) {
		mock := &MockCommander{Output: "/home/user/project"}
		client := tmux.NewClient(mock)

		got, err := client.ActivePaneCurrentPath("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/home/user/project" {
			t.Errorf("got %q, want %q", got, "/home/user/project")
		}
		if len(mock.Calls) != 1 {
			t.Fatalf("expected exactly 1 call, got %d: %v", len(mock.Calls), mock.Calls)
		}
		wantArgs := []string{"display-message", "-p", "-t", "my-session", "-F", "#{pane_current_path}"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got args %v, want %v", mock.Calls[0], wantArgs)
		}
		for i, want := range wantArgs {
			if mock.Calls[0][i] != want {
				t.Errorf("arg[%d] = %q, want %q (full %v)", i, mock.Calls[0][i], want, mock.Calls[0])
			}
		}
		// The active-pane-only contract: must not use list-panes or the -a flag.
		joined := strings.Join(mock.Calls[0], " ")
		if strings.Contains(joined, "list-panes") {
			t.Errorf("must not enumerate panes via list-panes, got %q", joined)
		}
		if strings.Contains(joined, "-a") {
			t.Errorf("must not use the -a (all-panes) flag, got %q", joined)
		}
	})

	t.Run("wraps a no-such-session command error so errors.Is(ErrNoSuchSession) holds", func(t *testing.T) {
		cmdErr := &tmux.CommandError{Stderr: "no such session: gone", Err: errors.New("exit status 1")}
		mock := &MockCommander{Err: cmdErr}
		client := tmux.NewClient(mock)

		_, err := client.ActivePaneCurrentPath("gone")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("error %v is not classified as ErrNoSuchSession", err)
		}
		var recovered *tmux.CommandError
		if !errors.As(err, &recovered) {
			t.Errorf("underlying *CommandError not recoverable from %v", err)
		}
	})

	t.Run("propagates a non-session command error without the sentinel", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("some transport failure")}
		client := tmux.NewClient(mock)

		_, err := client.ActivePaneCurrentPath("my-session")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("unrelated error %v wrongly classified as ErrNoSuchSession", err)
		}
		if !strings.Contains(err.Error(), "my-session") {
			t.Errorf("error %q does not contain session name", err.Error())
		}
	})
}
