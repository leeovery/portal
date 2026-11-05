package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/sessionindex"
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
	attached      string
	pathCalled    bool
	seams         *recordingResolverSeams
	fake          *backendtest.Fake
	// readsAtTUI is the number of session reads taken by the time the picker
	// seam ran, which is what separates an up-front count from a deferred one.
	readsAtTUI int
}

func installSearchFormSeams(t *testing.T) *searchFormCapture {
	t.Helper()

	sc := &searchFormCapture{seams: &recordingResolverSeams{}, fake: backendtest.New()}
	withBootstrapDeps(t, BootstrapDeps{})
	withBackendDeps(t, BackendDeps{Backend: sc.fake})

	withOpenDeps(t, OpenDeps{
		SessionLister: sc.seams,
		AliasLookup:   sc.seams,
		Zoxide:        sc.seams,
		DirValidator:  sc.seams,
	})

	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, landing pickerLanding, command []string) error {
		sc.tuiCalled = true
		sc.landing = landing
		sc.command = command
		sc.readsAtTUI = len(sc.fake.CallsTo(backendtest.OpList))
		return nil
	})
	withFuncSeam(t, &runOpenBurstFunc, func(*cobra.Command, []spawn.Surface, resolver.QueryResult, []string) error {
		sc.burstCalled = true
		return nil
	})
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
		sc.sessionCalled = true
		sc.attached = name
		return nil
	})
	withFuncSeam(t, &openPathFunc, func(*cobra.Command, string, []string) error {
		sc.pathCalled = true
		return nil
	})

	return sc
}

// live holds sessions of the names given in the capture's backend, each
// created at joinCreated.
func (sc *searchFormCapture) live(names ...string) {
	for _, name := range names {
		sc.fake.Add(backend.Session{Name: name, Created: joinCreated})
	}
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
	sc := installSearchFormSeams(t)

	executeOpen(t, "/port")

	if !sc.tuiCalled {
		t.Fatal("openTUIFunc must be called for a search form")
	}
	if got, want := shapeOfLanding(sc.landing), (landingShape{term: "port", search: true}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
	}
	if sc.command != nil {
		t.Errorf("command = %v, want nil", sc.command)
	}
	if sc.sessionCalled || sc.pathCalled || sc.burstCalled {
		t.Error("a search form must reach no other branch of open")
	}
}

func TestOpenCommand_SearchForm_ResolvesNothing(t *testing.T) {
	sc := installSearchFormSeams(t)

	executeOpen(t, "/port")

	if sc.seams.consulted() {
		t.Errorf("resolution was attempted for a search form: %+v", sc.seams)
	}
}

func TestOpenCommand_SearchForm_GlobMetacharactersAreLiteralText(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("po*rt-x", "portal-a1b2")

	executeOpen(t, "/po*")

	if sc.attached != "po*rt-x" {
		t.Errorf("attached session = %q, want %q; the term is literal text, not a glob", sc.attached, "po*rt-x")
	}
	if sc.burstCalled {
		t.Error("a search term carrying glob metacharacters must not dispatch the burst")
	}
}

func TestOpenCommand_SearchForm_AttachesASessionWhoseNameEqualsTheTerm(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("port")

	executeOpen(t, "/port")

	if sc.attached != "port" {
		t.Errorf("attached session = %q, want %q", sc.attached, "port")
	}
	if sc.tuiCalled {
		t.Error("a term matching exactly one live session must not open the picker")
	}
}

func TestOpenCommand_SearchForm_EmitsNoResolveLine(t *testing.T) {
	tests := []struct {
		name     string
		sessions []string
	}{
		{name: "no match"},
		{name: "one match", sessions: []string{"port"}},
		{name: "two matches", sessions: []string{"port", "portal-a1b2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := logtest.Install(t)

			sc := installSearchFormSeams(t)
			sc.live(tt.sessions...)

			executeOpen(t, "/port")

			if records := sink.Records().Matching("resolve", "resolved"); len(records) != 0 {
				t.Errorf("search form emitted %d resolve records, want none: %v", len(records), records)
			}
		})
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
	withFuncSeam(t, &openTUIFunc, func(*cobra.Command, pickerLanding, []string) error {
		t.Error("a multi-segment path positional must mint rather than open the picker")
		return nil
	})

	executeOpen(t, dir)

	if gotPath != dir {
		t.Errorf("minted path = %q, want %q", gotPath, dir)
	}
}

func TestOpenCommand_SearchForm_BareSigilIsNotAUsageError(t *testing.T) {
	sc := installSearchFormSeams(t)

	executeOpen(t, "/")

	if !sc.tuiCalled {
		t.Fatal("openTUIFunc must be called for a bare search sigil")
	}
	if got, want := shapeOfLanding(sc.landing), (landingShape{search: true}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
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
	installSearchFormSeams(t)

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
	installSearchFormSeams(t)

	executeOpenExpectingUsage(t, "cannot use a /term search with another /term search", "/port", "/blog")
}

func TestValidateOpenArgs_RefusesSearchFormWithACommand(t *testing.T) {
	installSearchFormSeams(t)

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
	installSearchFormSeams(t)

	executeOpenExpectingUsage(t, "cannot use a /term search with -f/--filter", "/port", "-f", "blog")
}

func TestValidateOpenArgs_RefusesSearchFormWithEachDomainPin(t *testing.T) {
	installSearchFormSeams(t)

	for _, pin := range []string{"-s", "-p", "-a", "-z"} {
		t.Run(pin, func(t *testing.T) {
			executeOpenExpectingUsage(t, "cannot use a /term search with a domain pin (-s/-p/-z/-a)", "/port", pin, "api")
		})
	}
}

func TestValidateOpenArgs_RefusesSearchFormWithAck(t *testing.T) {
	installSearchFormSeams(t)

	executeOpenExpectingUsage(t, "cannot use a /term search with --ack", "/port", "--ack", "batch:token")
}

// craftedOpenCommand returns a command shaped like open's positional surface
// carrying one throwaway flag. A pflag.FlagSet cannot un-register a flag, so the
// real openCmd is never given one.
func craftedOpenCommand(t *testing.T, name string) *cobra.Command {
	t.Helper()

	c := &cobra.Command{Use: "open", Args: cobra.ArbitraryArgs}
	c.Flags().String(name, "", "throwaway flag no named collision arm covers")
	return c
}

func TestValidateSearchFormCollisions_RefusesAFlagNoNamedArmCovers(t *testing.T) {
	c := craftedOpenCommand(t, "zzz")
	if err := c.Flags().Set("zzz", "v"); err != nil {
		t.Fatalf("setting --zzz: %v", err)
	}

	err := validateSearchFormCollisions(c, []string{"/port"})

	usage, ok := errors.AsType[*UsageError](err)
	if !ok {
		t.Fatalf("error = %v (%T), want *UsageError", err, err)
	}
	if got, want := usage.Error(), "cannot use a /term search with --zzz"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestValidateSearchFormCollisions_AdmitsASearchFormThatSetNoFlag(t *testing.T) {
	c := craftedOpenCommand(t, "zzz")

	if err := validateSearchFormCollisions(c, []string{"/port"}); err != nil {
		t.Errorf("a search form setting no flag must be admitted, got %v", err)
	}
}

func TestValidateOpenArgs_RefusedLineStartsNoBootstrap(t *testing.T) {
	runner := &recordingRunner{}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})
	withFuncSeam(t, &openTUIFunc, func(*cobra.Command, pickerLanding, []string) error {
		t.Error("a refused line must never reach the picker")
		return nil
	})

	executeOpenExpectingUsage(t, "cannot use a /term search with another target", "/port", "api")

	if runner.calls != 0 {
		t.Errorf("bootstrap ran %d times for a refused line, want 0", runner.calls)
	}
}

func TestValidateOpenArgs_StillAnswersHelpOnASearchFormLine(t *testing.T) {
	installSearchFormSeams(t)

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
			installSearchFormSeams(t)

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

func TestOpenCommand_SearchForm_AttachesTheSingleMatch(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("portal-a1b2", "blog-c3d4")

	executeOpen(t, "/port")

	if sc.attached != "portal-a1b2" {
		t.Errorf("attached session = %q, want %q", sc.attached, "portal-a1b2")
	}
	if sc.tuiCalled {
		t.Error("the picker must not open when exactly one live session matches")
	}
}

func TestOpenCommand_SearchForm_AttachesASessionMatchedOnlyByItsRecordedDirectory(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("api-work", "blog-c3d4")
	dir := filepath.Join(os.Getenv("HOME"), "Code", "portal")
	seedIndex(t, sessionindex.Entry{Name: "api-work", Dir: dir, CreatedAt: joinCreated})

	executeOpen(t, "/portal")

	if sc.attached != "api-work" {
		t.Errorf("attached session = %q, want %q", sc.attached, "api-work")
	}
	if sc.tuiCalled {
		t.Error("a directory-only match is still a match: the picker must not open")
	}
}

func TestOpenCommand_SearchForm_OpensPickerWhenNothingMatches(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("blog-c3d4", "api-e5f6")

	_, errBuf, err := runRootCmd(t, "open", "/port")
	if err != nil {
		t.Fatalf("portal open /port: error = %v, want nil; zero matches is a filter result", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errBuf.String())
	}
	if got, want := shapeOfLanding(sc.landing), (landingShape{term: "port", search: true}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
	}
	if sc.sessionCalled {
		t.Error("nothing matched, so nothing may be attached")
	}
}

func TestOpenCommand_SearchForm_OpensPickerWhenTwoOrMoreMatch(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("portal-a1b2", "port-agent", "blog-c3d4")

	executeOpen(t, "/port")

	if !sc.tuiCalled {
		t.Fatal("two matches must open the picker")
	}
	if got, want := shapeOfLanding(sc.landing), (landingShape{term: "port", search: true}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
	}
	if sc.sessionCalled {
		t.Error("two matches leave a choice: nothing may be attached")
	}
}

func TestOpenCommand_SearchForm_OpensPickerWhenNoSessionsAreLive(t *testing.T) {
	sc := installSearchFormSeams(t)

	executeOpen(t, "/port")

	if !sc.tuiCalled {
		t.Fatal("an empty session list must open the picker")
	}
	if sc.sessionCalled {
		t.Error("no live session: nothing may be attached")
	}
}

func TestOpenCommand_SearchForm_TermLessFormTakesNoCount(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("portal-a1b2")

	executeOpen(t, "/")

	if n := len(sc.fake.CallsTo(backendtest.OpList)); n != 0 {
		t.Errorf("term-less form listed the sessions %d times, want 0", n)
	}
	if got, want := shapeOfLanding(sc.landing), (landingShape{search: true}); got != want {
		t.Errorf("landing = %+v, want %+v", got, want)
	}
	if sc.sessionCalled {
		t.Error("a term-less form counts nothing, so it can attach nothing")
	}
}

func TestOpenCommand_SearchForm_ExcludesTheCurrentSessionFromTheCount(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("portal-a1b2", "blog-c3d4")
	sc.fake.SetCurrent("portal-a1b2")

	executeOpen(t, "/port")

	if sc.sessionCalled {
		t.Errorf("attached %q: the session the user is in is not a candidate", sc.attached)
	}
	if !sc.tuiCalled {
		t.Fatal("with the only match excluded the picker must open")
	}
}

func TestOpenCommand_SearchForm_AttachesTheOtherMatchWhenTheCurrentSessionAlsoMatches(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("portal-a1b2", "port-agent")
	sc.fake.SetCurrent("portal-a1b2")

	executeOpen(t, "/port")

	if sc.attached != "port-agent" {
		t.Errorf("attached session = %q, want %q", sc.attached, "port-agent")
	}
	if sc.tuiCalled {
		t.Error("one candidate match remains, so the picker must not open")
	}
}

func TestOpenCommand_SearchForm_ExcludesNothingOutsideASession(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("portal-a1b2", "port-agent")

	executeOpen(t, "/port")

	if !sc.tuiCalled {
		t.Fatal("outside a session both sessions are candidates, so two matches open the picker")
	}
}

func TestOpenCommand_SearchForm_CountsOverTheUserSessionsOnly(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.live("_portal-saver", "portal-a1b2")

	executeOpen(t, "/portal")

	if sc.attached != "portal-a1b2" {
		t.Errorf("attached %q, want portal-a1b2: Portal's own sessions are no candidates", sc.attached)
	}
	if n := len(sc.fake.CallsTo(backendtest.OpList)); n != 1 {
		t.Errorf("the sessions were listed %d times, want 1", n)
	}
}

func TestOpenCommand_SearchForm_ReturnsAnEnumerationError(t *testing.T) {
	sc := installSearchFormSeams(t)
	sc.fake.Fail(backendtest.OpList, errors.New("unreadable zmx dir"))

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "/port"})

	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unreadable zmx dir") {
		t.Fatalf("error = %v, want the enumeration failure", err)
	}
	if usage, ok := errors.AsType[*UsageError](err); ok {
		t.Errorf("error = %v, want an ordinary failure rather than a refusal of the command line", usage)
	}
	if sc.tuiCalled || sc.sessionCalled {
		t.Error("a failed read is not a zero match: neither the picker nor an attach may follow it")
	}
}

// executeOpenExpectingPreBootstrapUsage runs an open line expected to be refused
// from the Args validator, and asserts the refusal cost the user nothing: no
// bootstrap, no resolution, no picker.
func executeOpenExpectingPreBootstrapUsage(t *testing.T, wantMsg string, argv ...string) {
	t.Helper()

	runner := &recordingRunner{}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

	seams := &recordingResolverSeams{}
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
	withOpenDeps(t, OpenDeps{
		SessionLister: seams,
		AliasLookup:   seams,
		Zoxide:        seams,
		DirValidator:  seams,
	})
	withFuncSeam(t, &openTUIFunc, func(*cobra.Command, pickerLanding, []string) error {
		t.Error("a refused line must never reach the picker")
		return nil
	})

	executeOpenExpectingUsage(t, wantMsg, argv...)

	if runner.calls != 0 {
		t.Errorf("bootstrap ran %d times for a refused line, want 0", runner.calls)
	}
	if seams.consulted() {
		t.Error("a refused line must consult no resolver seam")
	}
}

func TestValidateOpenArgs_RefusesAMalformedFilterLineBeforeBootstrap(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "empty value",
			argv: []string{"-f", ""},
			want: "-f/--filter value must not be empty",
		},
		{
			name: "beside a positional target",
			argv: []string{"-f", "blog", "api"},
			want: "cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executeOpenExpectingPreBootstrapUsage(t, tt.want, tt.argv...)
		})
	}
}

func TestValidateOpenArgs_RefusesFilterBesideEachDomainPinBeforeBootstrap(t *testing.T) {
	want := "cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)"

	for _, pin := range []string{"-s", "-p", "-a", "-z"} {
		t.Run(pin, func(t *testing.T) {
			executeOpenExpectingPreBootstrapUsage(t, want, "-f", "blog", pin, "api")
		})
	}
}

func TestValidateOpenArgs_RefusesAMalformedCommandScopeBeforeBootstrap(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "empty exec value",
			argv: []string{"-e", ""},
			want: "-e/--exec value must not be empty",
		},
		{
			name: "exec together with a separator",
			argv: []string{"-e", "vim", "--", "claude"},
			want: "cannot use both -e/--exec and -- to specify a command",
		},
		{
			name: "bare separator",
			argv: []string{"--"},
			want: "no command specified after --",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executeOpenExpectingPreBootstrapUsage(t, tt.want, tt.argv...)
		})
	}
}

func TestValidateOpenArgs_KeepsTodaysRefusalPrecedence(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "a search form outranks an empty -f",
			argv: []string{"/port", "-f", ""},
			want: "cannot use a /term search with -f/--filter",
		},
		{
			name: "an empty -e outranks an empty -f",
			argv: []string{"-e", "", "-f", ""},
			want: "-e/--exec value must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executeOpenExpectingPreBootstrapUsage(t, tt.want, tt.argv...)
		})
	}
}

func TestValidateOpenArgs_StillAnswersHelpOnAMalformedFilterLine(t *testing.T) {
	installSearchFormSeams(t)

	out, _, err := runRootCmd(t, "open", "-f", "", "--help")
	if err != nil {
		t.Fatalf(`portal open -f "" --help: unexpected error: %v`, err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("help output missing usage section: %q", out.String())
	}
}

func TestOpenCommand_MalformedAckRefusedFromTheCommandBody(t *testing.T) {
	runner := &recordingRunner{}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

	seams := &recordingResolverSeams{}
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
	withOpenDeps(t, OpenDeps{
		SessionLister: seams,
		AliasLookup:   seams,
		Zoxide:        seams,
		DirValidator:  seams,
	})

	executeOpenExpectingUsage(t, "open: --ack must be <batch>:<token>", "dev", "--ack", "notcolon")

	if runner.calls != 1 {
		t.Errorf("bootstrap ran %d times, want 1: --ack is refused from the command body, downstream of bootstrap", runner.calls)
	}
}
