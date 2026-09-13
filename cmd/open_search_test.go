package cmd

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

// scanSearchForms runs argv through a cobra command shaped like open's own
// positional surface, so ArgsLenAtDash carries the real separator index.
func scanSearchForms(t *testing.T, argv []string) []string {
	t.Helper()

	var got []string
	c := &cobra.Command{
		Use:  "open",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			got = searchFormPositionals(cmd, args)
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.SetArgs(argv)
	if err := c.Execute(); err != nil {
		t.Fatalf("scan command failed: %v", err)
	}
	return got
}

func TestSearchFormPositionals(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{name: "sole positional", argv: []string{"/port"}, want: []string{"/port"}},
		{name: "second positional", argv: []string{"api", "/port"}, want: []string{"/port"}},
		{name: "third positional", argv: []string{"api", "~/Code/api", "/port"}, want: []string{"/port"}},
		{name: "several in argv order", argv: []string{"/port", "api", "/blog"}, want: []string{"/port", "/blog"}},
		{name: "bare slash", argv: []string{"/"}, want: []string{"/"}},
		{name: "multi-segment path is not a search form", argv: []string{"/Users/lee/Code/api"}, want: nil},
		{name: "trailing slash is not a search form", argv: []string{"/tmp/"}, want: nil},
		{name: "no positionals", argv: nil, want: nil},
		{name: "no search form among targets", argv: []string{"api", "~/Code/api"}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scanSearchForms(t, tt.argv); !slices.Equal(got, tt.want) {
				t.Errorf("searchFormPositionals(%v) = %v, want %v", tt.argv, got, tt.want)
			}
		})
	}
}

func TestSearchFormPositionals_StopsAtDashSeparator(t *testing.T) {
	t.Run("a search shape after the separator belongs to the command", func(t *testing.T) {
		got := scanSearchForms(t, []string{"~/Code/api", "--", "ls", "/tmp"})
		if got != nil {
			t.Errorf("searchFormPositionals = %v, want nil; words after -- are the command's own", got)
		}
	})

	t.Run("a search form before the separator is still found", func(t *testing.T) {
		got := scanSearchForms(t, []string{"/port", "--", "ls", "/tmp"})
		want := []string{"/port"}
		if !slices.Equal(got, want) {
			t.Errorf("searchFormPositionals = %v, want %v", got, want)
		}
	})
}

// recordingResolverSeams counts every consultation the resolution chain makes,
// so a test can assert that no resolution was attempted at all.
type recordingResolverSeams struct {
	listed    int
	aliased   int
	zoxided   int
	validated int
}

func (r *recordingResolverSeams) ListSessionNames() ([]string, error) {
	r.listed++
	return nil, nil
}

func (r *recordingResolverSeams) Get(string) (string, bool) {
	r.aliased++
	return "", false
}

func (r *recordingResolverSeams) Keys() []string {
	r.aliased++
	return nil
}

func (r *recordingResolverSeams) Query(string) (string, error) {
	r.zoxided++
	return "", resolver.ErrNoMatch
}

func (r *recordingResolverSeams) Exists(string) bool {
	r.validated++
	return false
}

func (r *recordingResolverSeams) consulted() bool {
	return r.listed+r.aliased+r.zoxided+r.validated > 0
}

type searchFormCapture struct {
	tuiCalled     bool
	landing       pickerLanding
	command       []string
	burstCalled   bool
	sessionCalled bool
	pathCalled    bool
	seams         *recordingResolverSeams
}

func installSearchFormSeams(t *testing.T, lister resolver.SessionLister) *searchFormCapture {
	t.Helper()

	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	sc := &searchFormCapture{seams: &recordingResolverSeams{}}

	deps := OpenDeps{
		SessionLister: sc.seams,
		AliasLookup:   sc.seams,
		Zoxide:        sc.seams,
		DirValidator:  sc.seams,
	}
	if lister != nil {
		deps.SessionLister = lister
	}
	withOpenDeps(t, deps)

	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string, _ bool) error {
		sc.tuiCalled = true
		sc.landing = landing
		sc.command = command
		return nil
	})
	withFuncSeam(t, &runOpenBurstFunc, func(*cobra.Command, []spawn.Surface, []string) error {
		sc.burstCalled = true
		return nil
	})
	withFuncSeam(t, &openSessionFunc, func(*cobra.Command, string) error {
		sc.sessionCalled = true
		return nil
	})
	withFuncSeam(t, &openPathFunc, func(*cobra.Command, string, []string) error {
		sc.pathCalled = true
		return nil
	})

	return sc
}

func executeOpen(t *testing.T, argv ...string) {
	t.Helper()

	resetRootCmd()
	rootCmd.SetArgs(append([]string{"open"}, argv...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("portal open %v: unexpected error: %v", argv, err)
	}
}

func TestOpenCommand_SearchForm_OpensPickerOnTheTerm(t *testing.T) {
	sc := installSearchFormSeams(t, nil)

	executeOpen(t, "/port")

	if !sc.tuiCalled {
		t.Fatal("openTUIFunc must be called for a search form")
	}
	if want := (pickerLanding{filter: "port", search: true}); sc.landing != want {
		t.Errorf("landing = %+v, want %+v", sc.landing, want)
	}
	if sc.command != nil {
		t.Errorf("command = %v, want nil", sc.command)
	}
	if sc.sessionCalled || sc.pathCalled || sc.burstCalled {
		t.Error("a search form must reach no other branch of open")
	}
}

func TestOpenCommand_SearchForm_ResolvesNothing(t *testing.T) {
	sc := installSearchFormSeams(t, nil)

	executeOpen(t, "/port")

	if sc.seams.consulted() {
		t.Errorf("resolution was attempted for a search form: %+v", sc.seams)
	}
}

func TestOpenCommand_SearchForm_GlobMetacharactersAreLiteralText(t *testing.T) {
	sc := installSearchFormSeams(t, &testSessionLister{names: []string{"portal-a", "portal-b"}})

	executeOpen(t, "/po*rt")

	if want := (pickerLanding{filter: "po*rt", search: true}); sc.landing != want {
		t.Errorf("landing = %+v, want %+v", sc.landing, want)
	}
	if sc.burstCalled {
		t.Error("a search term carrying glob metacharacters must not dispatch the burst")
	}
}

func TestOpenCommand_SearchForm_TermEqualToLiveSessionNameStillOpensPicker(t *testing.T) {
	sc := installSearchFormSeams(t, &testSessionLister{names: []string{"port"}})

	executeOpen(t, "/port")

	if sc.sessionCalled {
		t.Error("a search form must not attach, even when its term equals a live session name")
	}
	if !sc.tuiCalled {
		t.Fatal("openTUIFunc must be called for a search form")
	}
}

func TestOpenCommand_SearchForm_EmitsNoResolveLine(t *testing.T) {
	sink := logtest.Install(t)

	installSearchFormSeams(t, &testSessionLister{names: []string{"port"}})

	executeOpen(t, "/port")

	if records := sink.Records().Matching("resolve", "resolved"); len(records) != 0 {
		t.Errorf("search form emitted %d resolve records, want none: %v", len(records), records)
	}
}

func TestOpenCommand_MultiSegmentPathPositional_StillMints(t *testing.T) {
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	dir := t.TempDir()

	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	var gotPath string
	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, path string, _ []string) error {
		gotPath = path
		return nil
	})
	withFuncSeam(t, &openTUIFunc, func(*cobra.Command, pickerLanding, []string, bool) error {
		t.Error("a multi-segment path positional must mint rather than open the picker")
		return nil
	})

	executeOpen(t, dir)

	if gotPath != dir {
		t.Errorf("minted path = %q, want %q", gotPath, dir)
	}
}

func TestOpenCommand_SearchForm_BareSigilIsNotAUsageError(t *testing.T) {
	sc := installSearchFormSeams(t, nil)

	executeOpen(t, "/")

	if !sc.tuiCalled {
		t.Fatal("openTUIFunc must be called for a bare search sigil")
	}
	if want := (pickerLanding{search: true}); sc.landing != want {
		t.Errorf("landing = %+v, want %+v", sc.landing, want)
	}
	if sc.sessionCalled || sc.pathCalled || sc.burstCalled {
		t.Error("a bare search sigil must reach no other branch of open")
	}
}

// executeOpenExpectingUsage runs `portal open <argv>` and asserts it was refused
// with a *UsageError carrying wantMsg.
func executeOpenExpectingUsage(t *testing.T, wantMsg string, argv ...string) {
	t.Helper()

	resetRootCmd()
	rootCmd.SetArgs(append([]string{"open"}, argv...))

	err := rootCmd.Execute()

	usage, ok := errors.AsType[*UsageError](err)
	if !ok {
		t.Fatalf("portal open %v: error = %v (%T), want *UsageError", argv, err, err)
	}
	if usage.Error() != wantMsg {
		t.Errorf("portal open %v: message = %q, want %q", argv, usage.Error(), wantMsg)
	}
}

func TestValidateOpenArgs_RefusesSearchFormBesideAnotherTarget(t *testing.T) {
	installSearchFormSeams(t, nil)

	tests := []struct {
		name string
		argv []string
	}{
		{name: "search form first", argv: []string{"/port", "api"}},
		{name: "search form second", argv: []string{"api", "/port"}},
		{name: "two other targets", argv: []string{"/port", "api", "blog"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executeOpenExpectingUsage(t, "cannot use a /term search with another target", tt.argv...)
		})
	}
}

func TestValidateOpenArgs_RefusesSecondSearchForm(t *testing.T) {
	installSearchFormSeams(t, nil)

	executeOpenExpectingUsage(t, "cannot use a /term search with another /term search", "/port", "/blog")
}

func TestValidateOpenArgs_RefusesSearchFormWithACommand(t *testing.T) {
	installSearchFormSeams(t, nil)

	tests := []struct {
		name string
		argv []string
	}{
		{name: "exec flag", argv: []string{"/port", "-e", "ls"}},
		{name: "dash separator", argv: []string{"/port", "--", "ls"}},
		{name: "empty dash separator", argv: []string{"/port", "--"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executeOpenExpectingUsage(t, "cannot use a /term search with a command (-e/--)", tt.argv...)
		})
	}
}

func TestValidateOpenArgs_RefusesSearchFormWithFilter(t *testing.T) {
	installSearchFormSeams(t, nil)

	executeOpenExpectingUsage(t, "cannot use a /term search with -f/--filter", "/port", "-f", "blog")
}

func TestValidateOpenArgs_RefusesSearchFormWithEachDomainPin(t *testing.T) {
	installSearchFormSeams(t, nil)

	for _, pin := range []string{"-s", "-p", "-a", "-z"} {
		t.Run(pin, func(t *testing.T) {
			executeOpenExpectingUsage(t, "cannot use a /term search with a domain pin (-s/-p/-z/-a)", "/port", pin, "api")
		})
	}
}

func TestValidateOpenArgs_RefusesSearchFormWithAck(t *testing.T) {
	installSearchFormSeams(t, nil)

	executeOpenExpectingUsage(t, "cannot use a /term search with --ack", "/port", "--ack", "batch:token")
}

func TestValidateOpenArgs_RefusedLineStartsNoBootstrap(t *testing.T) {
	runner := &recordingRunner{}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})
	withFuncSeam(t, &openTUIFunc, func(*cobra.Command, pickerLanding, []string, bool) error {
		t.Error("a refused line must never reach the picker")
		return nil
	})

	executeOpenExpectingUsage(t, "cannot use a /term search with another target", "/port", "api")

	if runner.calls != 0 {
		t.Errorf("bootstrap ran %d times for a refused line, want 0", runner.calls)
	}
}

func TestValidateOpenArgs_StillAnswersHelpOnASearchFormLine(t *testing.T) {
	installSearchFormSeams(t, nil)

	out, _, err := runRootCmd(t, "open", "/port", "--help")
	if err != nil {
		t.Fatalf("portal open /port --help: unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("help output missing usage section: %q", out.String())
	}
}

func TestValidateOpenArgs_AdmitsEveryNonSearchLine(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{name: "lone search form", argv: []string{"/port"}},
		{name: "lone bare sigil", argv: []string{"/"}},
		{name: "slash word after the separator", argv: []string{"~/Code/api", "--", "ls", "/tmp"}},
		{name: "two positional targets", argv: []string{"api", "blog"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installSearchFormSeams(t, nil)

			resetRootCmd()
			rootCmd.SetArgs(append([]string{"open"}, tt.argv...))

			if err := rootCmd.Execute(); err != nil {
				if usage, ok := errors.AsType[*UsageError](err); ok {
					t.Fatalf("portal open %v was refused: %v", tt.argv, usage)
				}
			}
		})
	}
}
