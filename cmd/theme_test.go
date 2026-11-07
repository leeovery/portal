package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

type themeExportRun struct {
	stdout         []byte
	stderr         string
	err            error
	bootstrapCalls int
}

// The recorder is injected unconditionally so no test here can reach the real
// backend even if the command lost its bootstrapExempt entry.
func execThemeExport(t *testing.T, args ...string) themeExportRun {
	t.Helper()

	runner := &recordingRunner{}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

	var stdout, stderr bytes.Buffer
	resetRootCmd()
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(append([]string{"theme", "export"}, args...))
	err := rootCmd.Execute()

	return themeExportRun{
		stdout:         stdout.Bytes(),
		stderr:         stderr.String(),
		err:            err,
		bootstrapCalls: runner.calls,
	}
}

func requireCommentedSource(t *testing.T, source []byte) {
	t.Helper()

	if !bytes.Contains(source, []byte("#")) {
		t.Fatalf("fixture carries no # comment, so a verbatim assertion over it proves nothing:\n%s", source)
	}
	if !bytes.HasSuffix(source, []byte("\n")) {
		t.Fatalf("fixture has no trailing newline, so a verbatim assertion over it proves nothing:\n%s", source)
	}
}

// A built-in rather than a hand-written palette, so a fixture cannot drift out
// of validity as the token vocabulary evolves and no hex is restated in Go.
func validThemeSource(t *testing.T) []byte {
	t.Helper()

	source, found := theme.BuiltinBytes(theme.DefaultDarkSlug)
	if !found {
		t.Fatalf("BuiltinBytes(%q) reports not found", theme.DefaultDarkSlug)
	}
	return source
}

// An empty-but-present directory is the unknown-slug state: it resolves and
// reads, and the composed filename simply is not in it.
func useThemesDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("PORTAL_THEMES_DIR", dir)
	return dir
}

func seedThemesDir(t *testing.T, slug string, source []byte) string {
	t.Helper()

	dir := themesDirWith(t, map[string][]byte{slug + ".theme": source})
	t.Setenv("PORTAL_THEMES_DIR", dir)
	return dir
}

func TestThemeExport_IsBootstrapExempt(t *testing.T) {
	if !bootstrapExempt["theme"] {
		t.Error(`bootstrapExempt["theme"] = false; want true (printing a file needs no bootstrap)`)
	}

	run := execThemeExport(t, theme.DefaultDarkSlug)

	if run.err != nil {
		t.Fatalf("theme export %s returned %v (stderr: %q)", theme.DefaultDarkSlug, run.err, run.stderr)
	}
	if run.bootstrapCalls != 0 {
		t.Errorf("orchestrator Run call count = %d, want 0", run.bootstrapCalls)
	}
}

func TestThemeExport_BuiltinBytesAreVerbatim(t *testing.T) {
	slugs := theme.BuiltinSlugs()
	if len(slugs) == 0 {
		t.Fatal("BuiltinSlugs() is empty — every assertion below would be vacuous")
	}

	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			want, found := theme.BuiltinBytes(slug)
			if !found {
				t.Fatalf("BuiltinBytes(%q) reports not found", slug)
			}
			requireCommentedSource(t, want)

			run := execThemeExport(t, slug)

			if run.err != nil {
				t.Fatalf("theme export %s returned %v (stderr: %q)", slug, run.err, run.stderr)
			}
			if !bytes.Equal(run.stdout, want) {
				t.Errorf("theme export %s wrote:\n%s\nwant the embedded bytes verbatim:\n%s", slug, run.stdout, want)
			}
		})
	}
}

func TestThemeExport_DropInBytesAreVerbatim(t *testing.T) {
	t.Run("with a trailing newline", func(t *testing.T) {
		want := validThemeSource(t)
		requireCommentedSource(t, want)
		seedThemesDir(t, "nord-lee", want)

		run := execThemeExport(t, "nord-lee")

		if run.err != nil {
			t.Fatalf("theme export nord-lee returned %v (stderr: %q)", run.err, run.stderr)
		}
		if !bytes.Equal(run.stdout, want) {
			t.Errorf("theme export nord-lee wrote:\n%s\nwant the file's bytes verbatim:\n%s", run.stdout, want)
		}
	})

	t.Run("without a trailing newline", func(t *testing.T) {
		want := bytes.TrimRight(validThemeSource(t), "\n")
		if bytes.HasSuffix(want, []byte("\n")) {
			t.Fatal("fixture still ends in a newline — the no-newline case would be vacuous")
		}
		seedThemesDir(t, "nord-lee", want)

		run := execThemeExport(t, "nord-lee")

		if run.err != nil {
			t.Fatalf("theme export nord-lee returned %v (stderr: %q)", run.err, run.stderr)
		}
		if !bytes.Equal(run.stdout, want) {
			t.Errorf("theme export nord-lee wrote %d bytes ending %q, want the file's %d bytes ending %q — no newline may be added or stripped",
				len(run.stdout), lastBytes(run.stdout), len(want), lastBytes(want))
		}
	})
}

func lastBytes(b []byte) string {
	const tail = 24
	if len(b) > tail {
		return string(b[len(b)-tail:])
	}
	return string(b)
}

// Every fixture below derives from these rather than being hand-written, so no
// hex is restated in Go and a fixture cannot drift out of validity.
func themeKeyLines(t *testing.T) []string {
	t.Helper()

	var pairs []string
	for line := range strings.SplitSeq(string(validThemeSource(t)), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		pairs = append(pairs, text)
	}
	if len(pairs) < 2 {
		t.Fatalf("built-in yielded %d key lines — the fixtures derived from them need at least two", len(pairs))
	}
	return pairs
}

// Reversal is deliberate: file ordering carries nothing, so a re-serialising
// implementation would be free to emit its own order.
func scrambledThemeSource(t *testing.T) []byte {
	t.Helper()

	pairs := slices.Clone(themeKeyLines(t))
	slices.Reverse(pairs)

	var b strings.Builder
	b.WriteString("# nord-lee — a hand-edited copy\n#\n# Its keys are in no canonical order and its notes sit between them.\n\n")
	for i, pair := range pairs {
		fmt.Fprintf(&b, "# note %d\n%s\n\n", i, pair)
	}
	return []byte(b.String())
}

func TestThemeExport_IsNotAReserialisation(t *testing.T) {
	want := scrambledThemeSource(t)
	if bytes.Equal(want, validThemeSource(t)) {
		t.Fatal("the scrambled fixture equals the built-in — it would not distinguish a re-serialisation")
	}
	seedThemesDir(t, "nord-lee", want)

	run := execThemeExport(t, "nord-lee")

	if run.err != nil {
		t.Fatalf("theme export nord-lee returned %v (stderr: %q)", run.err, run.stderr)
	}
	if !bytes.Equal(run.stdout, want) {
		t.Errorf("theme export nord-lee wrote:\n%s\nwant the file's bytes verbatim (comments and key order intact):\n%s", run.stdout, want)
	}
}

func TestThemeExport_BuiltinNeverReadsThemesDirectory(t *testing.T) {
	t.Run("a built-in resolves through an unreadable themes directory", func(t *testing.T) {
		_ = themetest.DenyDir(t, seedThemesDir(t, "nord-lee", validThemeSource(t)))
		want := validThemeSource(t)

		run := execThemeExport(t, theme.DefaultDarkSlug)

		if run.err != nil {
			t.Fatalf("theme export %s returned %v — the embedded set must resolve before the themes directory", theme.DefaultDarkSlug, run.err)
		}
		if !bytes.Equal(run.stdout, want) {
			t.Errorf("theme export %s wrote:\n%s\nwant the embedded bytes verbatim:\n%s", theme.DefaultDarkSlug, run.stdout, want)
		}
	})

	t.Run("the directory really is unreadable", func(t *testing.T) {
		_ = themetest.DenyDir(t, seedThemesDir(t, "nord-lee", validThemeSource(t)))

		run := execThemeExport(t, "nord-lee")

		if run.err == nil {
			t.Fatal("theme export nord-lee succeeded against a mode-0000 directory — the ordering assertion above would be vacuous")
		}
	})

	// An unlocatable directory is the sharp discriminator: themesDirPath cannot
	// answer at all, so any implementation resolving the directory before the
	// embedded set surfaces that error instead of the theme.
	t.Run("a built-in resolves when the themes directory cannot be located", func(t *testing.T) {
		t.Setenv("PORTAL_THEMES_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		if _, err := themesDirPath(); err == nil {
			t.Fatal("themesDirPath resolved with no env var, no XDG_CONFIG_HOME and no HOME — this subtest would be vacuous")
		}
		want := validThemeSource(t)

		run := execThemeExport(t, theme.DefaultDarkSlug)

		if run.err != nil {
			t.Fatalf("theme export %s returned %v — a built-in must resolve without the themes directory being located at all", theme.DefaultDarkSlug, run.err)
		}
		if !bytes.Equal(run.stdout, want) {
			t.Errorf("theme export %s wrote:\n%s\nwant the embedded bytes verbatim:\n%s", theme.DefaultDarkSlug, run.stdout, want)
		}
	})
}

// An arity violation is a Cobra usage error, outside the refusal frames.
func TestThemeExport_ExactArgsOne(t *testing.T) {
	t.Run("the declared validator accepts one argument and nothing else", func(t *testing.T) {
		cases := []struct {
			name     string
			args     []string
			accepted bool
		}{
			{name: "zero", args: nil, accepted: false},
			{name: "one", args: []string{"tokyo-night"}, accepted: true},
			{name: "two", args: []string{"tokyo-night", "nord"}, accepted: false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				err := themeExportCmd.Args(themeExportCmd, tc.args)
				if tc.accepted && err != nil {
					t.Errorf("Args(%v) = %v, want nil", tc.args, err)
				}
				if !tc.accepted && err == nil {
					t.Errorf("Args(%v) = nil, want a usage error", tc.args)
				}
			})
		}
	})

	t.Run("a wrong arity writes nothing to stdout", func(t *testing.T) {
		for _, args := range [][]string{{}, {"tokyo-night", "nord"}} {
			run := execThemeExport(t, args...)

			if run.err == nil {
				t.Errorf("theme export %v returned nil, want a usage error", args)
			}
			if len(run.stdout) != 0 {
				t.Errorf("theme export %v wrote %q to stdout, want nothing", args, run.stdout)
			}
		}
	})
}

func TestThemeExport_EmitsNoThemeEvents(t *testing.T) {
	cases := []struct {
		name string
		slug string
		seed func(*testing.T)
	}{
		{name: "a built-in", slug: theme.DefaultDarkSlug},
		{
			name: "a valid drop-in",
			slug: "nord-lee",
			seed: func(t *testing.T) { seedThemesDir(t, "nord-lee", validThemeSource(t)) },
		},
		{
			name: "an invalid drop-in",
			slug: "nord-lee",
			seed: func(t *testing.T) { seedThemesDir(t, "nord-lee", sourceMissingTokens(t, "text.primary")) },
		},
		{name: "an unknown slug", slug: "no-such-theme"},
		{name: "a slug failing the charset check", slug: "Nord"},
		{
			name: "an unreadable drop-in",
			slug: "mine",
			seed: func(t *testing.T) {
				_ = themetest.DenyRead(t, filepath.Join(seedThemesDir(t, "mine", validThemeSource(t)), "mine.theme"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.seed != nil {
				tc.seed(t)
			}

			records := assertNoThemeRecords(t, func() { execThemeExport(t, tc.slug) })

			if len(records) != 0 {
				t.Errorf("theme export %s emitted %d log records, want none: %+v", tc.slug, len(records), records)
			}
		})
	}
}

// Lstat-based, so a file-to-symlink swap of identical content is a change.
func treeFingerprint(t *testing.T, root string) map[string]portaltest.Fingerprint {
	t.Helper()

	tree, err := portaltest.SnapshotStateDir(root)
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return tree
}

func assertTreeUnchanged(t *testing.T, root string, before map[string]portaltest.Fingerprint, subject string) {
	t.Helper()

	for _, delta := range portaltest.DiffFingerprints(before, treeFingerprint(t, root)) {
		t.Errorf("%s: %s", subject, portaltest.FormatDelta(delta))
	}
}

func assertNoThemeRecords(t *testing.T, run func()) []logtest.Record {
	t.Helper()

	sink := logtest.Install(t)

	run()

	records := sink.Records()
	if events := themeEvents(t, sink); len(events) != 0 {
		t.Errorf("the run emitted %d theme records, want none:\n  %s", len(events), strings.Join(events, "\n  "))
	}

	// Through the same sink the run was captured on: a fresh one proves only
	// that some sink records, not that this one was installed. records is
	// snapshotted above and Records() copies, so the probe cannot leak into it.
	theme.NewEventLogger(log.For("theme")).Rejected("mine", "", &theme.Rejection{Reason: theme.ReasonBadColour})
	if events := themeEvents(t, sink); len(events) != 1 {
		t.Fatalf("the capture harness recorded %d theme events, want 1 — the assertion above would be vacuous: %v", len(events), events)
	}
	return records
}

func TestThemeExport_ReadsNoPrefs(t *testing.T) {
	t.Run("it succeeds against an unreadable prefs.json", func(t *testing.T) {
		prefsPath := filepath.Join(t.TempDir(), "prefs.json")
		before := []byte(`{"session_list_mode":"by-tag","appearance":"light"}`)
		if err := os.WriteFile(prefsPath, before, 0o600); err != nil {
			t.Fatalf("seed prefs.json: %v", err)
		}
		_ = themetest.DenyRead(t, prefsPath)
		t.Setenv("PORTAL_PREFS_FILE", prefsPath)

		run := execThemeExport(t, theme.DefaultDarkSlug)

		if run.err != nil {
			t.Fatalf("theme export %s returned %v — prefs.json must never be read", theme.DefaultDarkSlug, run.err)
		}

		if err := os.Chmod(prefsPath, 0o600); err != nil {
			t.Fatalf("chmod 0600 prefs.json: %v", err)
		}
		after, err := os.ReadFile(prefsPath)
		if err != nil {
			t.Fatalf("read back prefs.json: %v", err)
		}
		if !bytes.Equal(after, before) {
			t.Errorf("prefs.json = %q, want it untouched: %q", after, before)
		}
	})

	t.Run("the export command names no prefs symbol", func(t *testing.T) {
		source := sourceguardtest.PackageSource(t, ".", "theme.go")

		banned := map[string]string{
			"prefsFilePath":  "resolves the prefs.json path",
			"loadPrefsStore": "opens the prefs store",
		}
		ast.Inspect(source.File, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ImportSpec:
				path, uerr := strconv.Unquote(node.Path.Value)
				if uerr == nil && strings.HasSuffix(path, "/internal/prefs") {
					t.Errorf("cmd/theme.go imports %s — export never reads prefs.json", path)
				}
			case *ast.Ident:
				if why, isBanned := banned[node.Name]; isBanned {
					t.Errorf("cmd/theme.go:%d names %s, which %s — export never reads prefs.json", source.Position(node.Pos()).Line, node.Name, why)
				}
			}
			return true
		})
	})

	t.Run("it writes no file anywhere", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("HOME", root)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
		t.Setenv("PORTAL_PREFS_FILE", filepath.Join(root, "prefs.json"))
		themes := themesDirIn(t, root, map[string][]byte{"nord-lee.theme": validThemeSource(t)})
		t.Setenv("PORTAL_THEMES_DIR", themes)

		before := treeFingerprint(t, root)
		if len(before) == 0 {
			t.Fatal("snapshot of the config root is empty — the comparison below would be vacuous")
		}

		for _, slug := range []string{theme.DefaultDarkSlug, "nord-lee", "no-such-theme"} {
			execThemeExport(t, slug)
		}

		assertTreeUnchanged(t, root, before, "the config tree changed")
	})
}

func themeLineIndex(t *testing.T, lines []string, key string) int {
	t.Helper()

	for i, line := range lines {
		declared, _, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(declared) == key {
			return i
		}
	}
	t.Fatalf("the built-in declares no %q line, so a fixture derived from it would prove nothing:\n  %s", key, strings.Join(lines, "\n  "))
	return -1
}

// Leaves the key's file position alone, which is what makes a `bad colour`
// fixture's offender order predictable: offenders enumerate in file order.
type themeOverride struct{ key, value string }

// The shared mutators leave an undeclared key alone rather than failing, so a
// key the built-in stopped declaring would turn the fixture back into a valid
// file unless the removal is checked here.
func missingTokenLines(t *testing.T, lines []string, keys ...string) []string {
	t.Helper()

	for _, key := range keys {
		shorter := themetest.WithoutKey(lines, key)
		if len(shorter) == len(lines) {
			t.Fatalf("the built-in declares no %q line, so a fixture missing it would prove nothing:\n  %s", key, strings.Join(lines, "\n  "))
		}
		lines = shorter
	}
	return lines
}

// The replacements still lex as well-formed pairs, so the file fails on its
// values rather than its syntax. The substitution is checked as removal is.
func badColourLines(t *testing.T, lines []string, overrides ...themeOverride) []string {
	t.Helper()

	for _, o := range overrides {
		lines = themetest.WithValue(lines, o.key, o.value)
		if !slices.Contains(lines, o.key+" = "+o.value) {
			t.Fatalf("the built-in declares no %q line, so a fixture overriding its value would prove nothing:\n  %s", o.key, strings.Join(lines, "\n  "))
		}
	}
	return lines
}

// Verified against the assembled lines, so the `line N` in the resulting
// `duplicate key` detail is a fact about the fixture, not about the built-in.
func duplicateKeyLines(t *testing.T, lines []string, key string, at int) []string {
	t.Helper()

	first := themeLineIndex(t, lines, key)
	if at < first+2 || at > len(lines)+1 {
		t.Fatalf("line %d cannot carry a duplicate of %q, first declared on line %d of %d", at, key, first+1, len(lines))
	}

	assembled := themetest.WithDuplicateKeyAt(lines, key, at)
	if got := themeLineIndex(t, assembled, key); got != first {
		t.Fatalf("assembled fixture declares %q first on line %d, want line %d", key, got+1, first+1)
	}
	if assembled[at-1] != lines[first] {
		t.Fatalf("assembled fixture carries %q on line %d, want the duplicate %q", assembled[at-1], at, lines[first])
	}
	return assembled
}

func sourceMissingTokens(t *testing.T, keys ...string) []byte {
	t.Helper()

	return themetest.Render(missingTokenLines(t, themeKeyLines(t), keys...))
}

func sourceBadColours(t *testing.T, overrides ...themeOverride) []byte {
	t.Helper()

	return themetest.Render(badColourLines(t, themeKeyLines(t), overrides...))
}

func sourceDuplicateKeyAt(t *testing.T, line int, key string) []byte {
	t.Helper()

	return themetest.Render(duplicateKeyLines(t, themeKeyLines(t), key, line))
}

// Fails if the read succeeds.
func osReadError(t *testing.T, path string) error {
	t.Helper()

	if _, err := os.ReadFile(path); err != nil {
		return err
	}
	t.Fatalf("reading %s succeeded — the unreadable fixture is readable, so the assertion over it would be vacuous", path)
	return nil
}

// Empty stdout rides along because export is a pipe-into-a-file tool: a byte on
// the wrong stream lands inside the file just created.
func requireExportRefusal(t *testing.T, run themeExportRun, want string) {
	t.Helper()

	if run.err == nil {
		t.Fatalf("theme export succeeded, want the refusal %q", want)
	}
	if got := run.err.Error(); got != want {
		t.Errorf("refusal = %q, want the refusal frame %q", got, want)
	}
	if len(run.stdout) != 0 {
		t.Errorf("stdout = %q, want nothing — a redirect would capture the refusal into the user's theme file", run.stdout)
	}
}

// The ordinary-error arm is what is left once every classified arm is ruled out.
func requireOrdinaryError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("theme export returned nil, want a refusal")
	}

	if _, ok := errors.AsType[*bootstrap.FatalError](err); ok {
		t.Errorf("refusal %q is a *bootstrap.FatalError — classify would suppress its message", err)
	}
	if IsSilentExitError(err) {
		t.Errorf("refusal %q is a silent-exit sentinel — classify would print nothing, and the reason string is the whole answer", err)
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Errorf("refusal %q is a *UsageError — classify would exit 2, and every failure class here must exit 1", err)
	}
}

type themeExportFailure struct {
	name string
	slug string
	seed func(t *testing.T)
}

func themeExportFailures() []themeExportFailure {
	return []themeExportFailure{
		{
			name: "an unknown slug",
			slug: "no-such-theme",
			seed: func(t *testing.T) { useThemesDir(t) },
		},
		{
			name: "an invalid drop-in",
			slug: "mine",
			seed: func(t *testing.T) { seedThemesDir(t, "mine", sourceMissingTokens(t, "text.primary")) },
		},
		{
			name: "a slug failing the charset check",
			slug: "Nord",
			seed: func(t *testing.T) { useThemesDir(t) },
		},
		{
			name: "an unreadable drop-in",
			slug: "mine",
			seed: func(t *testing.T) {
				_ = themetest.DenyRead(t, filepath.Join(seedThemesDir(t, "mine", validThemeSource(t)), "mine.theme"))
			},
		},
	}
}

func TestThemeExport_UnknownSlugFrame(t *testing.T) {
	t.Run("with an empty themes directory", func(t *testing.T) {
		useThemesDir(t)

		requireExportRefusal(t, execThemeExport(t, "nope"), "no theme named nope")
	})

	t.Run("with an absent themes directory", func(t *testing.T) {
		t.Setenv("PORTAL_THEMES_DIR", filepath.Join(t.TempDir(), "never-created"))

		requireExportRefusal(t, execThemeExport(t, "nope"), "no theme named nope")
	})
}

func TestThemeExport_InvalidDropInFrame(t *testing.T) {
	cases := []struct {
		name   string
		source func(*testing.T) []byte
		want   string
	}{
		{
			name:   "a duplicate key",
			source: func(t *testing.T) []byte { return sourceDuplicateKeyAt(t, 12, "text.primary") },
			want:   "theme mine is not valid: bad syntax",
		},
		{
			name:   "a bad hex",
			source: func(t *testing.T) []byte { return sourceBadColours(t, themeOverride{"canvas", "blue"}) },
			want:   "theme mine is not valid: bad colour",
		},
		{
			name:   "a missing token",
			source: func(t *testing.T) []byte { return sourceMissingTokens(t, "text.primary") },
			want:   "theme mine is not valid: missing tokens",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seedThemesDir(t, "mine", tc.source(t))

			requireExportRefusal(t, execThemeExport(t, "mine"), tc.want)
		})
	}
}

// The arguments arrive after `--`: a bare `-nord` never reaches the command,
// pflag claiming it as a shorthand cluster.
func TestThemeExport_BadNameFrame(t *testing.T) {
	t.Run("the argument is echoed back in the frame", func(t *testing.T) {
		cases := []struct {
			name string
			arg  string
			want string
		}{
			{name: "a traversal attempt", arg: "../evil", want: "theme ../evil is not valid: bad name"},
			{name: "a leading hyphen", arg: "-nord", want: "theme -nord is not valid: bad name"},
			{name: "the wrong case", arg: "Nord", want: "theme Nord is not valid: bad name"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				useThemesDir(t)

				requireExportRefusal(t, execThemeExport(t, "--", tc.arg), tc.want)
			})
		}
	})

	// A valid theme sits in the themes directory's PARENT: were the argument
	// ever joined onto the directory, `../evil` would resolve to it and its
	// bytes would land on stdout.
	t.Run("no path is composed from the argument", func(t *testing.T) {
		root := t.TempDir()
		escaped := validThemeSource(t)
		if err := os.WriteFile(filepath.Join(root, "evil.theme"), escaped, 0o644); err != nil {
			t.Fatalf("seed evil.theme: %v", err)
		}
		t.Setenv("PORTAL_THEMES_DIR", themesDirIn(t, root, nil))

		run := execThemeExport(t, "--", "../evil")

		requireExportRefusal(t, run, "theme ../evil is not valid: bad name")
		if bytes.Contains(run.stdout, escaped) {
			t.Error("stdout carries the escape target's bytes — a path was composed from the argument")
		}
	})

	// A charset failure outranks an unlocatable directory: the fold to
	// unreadable applies only to a `not found` rejection, so a bad name keeps
	// its own frame however badly the directory resolved.
	t.Run("a charset failure outranks an unlocatable directory", func(t *testing.T) {
		t.Setenv("PORTAL_THEMES_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		if _, err := themesDirPath(); err == nil {
			t.Fatal("themesDirPath resolved with no env var, no XDG_CONFIG_HOME and no HOME — this subtest would be vacuous")
		}

		requireExportRefusal(t, execThemeExport(t, "Nord"), "theme Nord is not valid: bad name")
	})

	t.Run("a bare leading hyphen is Cobra's usage error, not a frame", func(t *testing.T) {
		useThemesDir(t)

		run := execThemeExport(t, "-nord")

		if run.err == nil {
			t.Error("theme export -nord returned nil, want pflag's shorthand-cluster error")
		}
		if len(run.stdout) != 0 {
			t.Errorf("stdout = %q, want nothing", run.stdout)
		}
	})
}

// The expected OS error comes from performing the same read here, rather than
// hard-coding a platform's wording.
func TestThemeExport_UnreadableFrame(t *testing.T) {
	t.Run("an unreadable file", func(t *testing.T) {
		osErr := themetest.DenyRead(t, filepath.Join(seedThemesDir(t, "mine", validThemeSource(t)), "mine.theme"))

		requireExportRefusal(t, execThemeExport(t, "mine"), "theme mine could not be read: "+osErr.Error())
	})

	t.Run("an unreadable directory", func(t *testing.T) {
		dir := seedThemesDir(t, "mine", validThemeSource(t))
		_ = themetest.DenyDir(t, dir)
		osErr := osReadError(t, filepath.Join(dir, "mine.theme"))

		requireExportRefusal(t, execThemeExport(t, "mine"), "theme mine could not be read: "+osErr.Error())
	})

	// The expectation is derived from themesDirPath's own error rather than
	// spelling out a platform's wording.
	t.Run("a themes directory that cannot be located", func(t *testing.T) {
		t.Setenv("PORTAL_THEMES_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		_, err := themesDirPath()
		if err == nil {
			t.Fatal("themesDirPath resolved with no env var, no XDG_CONFIG_HOME and no HOME — this subtest would be vacuous")
		}

		requireExportRefusal(t, execThemeExport(t, "nord-lee"), "theme nord-lee could not be read: "+err.Error())
	})
}

// The dangling symlink separates "the read failed with ENOENT" from "there is
// nothing at this name": the read reports ENOENT either way, and only the name's
// own existence tells them apart.
func TestThemeExport_AbsentIsNotUnreadable(t *testing.T) {
	t.Run("an unreadable directory is could not be read", func(t *testing.T) {
		dir := seedThemesDir(t, "mine", validThemeSource(t))
		_ = themetest.DenyDir(t, dir)
		osErr := osReadError(t, filepath.Join(dir, "nope.theme"))

		requireExportRefusal(t, execThemeExport(t, "nope"), "theme nope could not be read: "+osErr.Error())
	})

	t.Run("a dangling symlink is could not be read", func(t *testing.T) {
		dir := useThemesDir(t)
		path := filepath.Join(dir, "mine.theme")
		if err := os.Symlink(filepath.Join(dir, "no-such-target"), path); err != nil {
			t.Fatalf("symlink %s: %v", path, err)
		}
		osErr := osReadError(t, path)
		if !os.IsNotExist(osErr) {
			t.Fatalf("reading the dangling link reports %v, want a not-exist error — this case would not exercise the distinction", osErr)
		}

		requireExportRefusal(t, execThemeExport(t, "mine"), "theme mine could not be read: "+osErr.Error())
	})
}

// A pasted newline would otherwise split one refusal into two lines, the second
// looking like a message Portal never wrote.
func TestThemeExport_ArgumentIsControlStripped(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{name: "a pasted newline", arg: "no\npe", want: "no theme named nope"},
		{name: "a pasted tab", arg: "no\tpe", want: "no theme named nope"},
		{name: "a pasted ANSI escape", arg: "\x1b[31mnope\x1b[0m", want: "no theme named nope"},
		{name: "a trailing newline on a charset failure", arg: "Nord\n", want: "theme Nord is not valid: bad name"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useThemesDir(t)

			run := execThemeExport(t, "--", tc.arg)

			requireExportRefusal(t, run, tc.want)
			for _, r := range run.err.Error() {
				if unicode.IsControl(r) {
					t.Fatalf("refusal %q carries the control character %q — the message must stay one line of ordinary text", run.err, r)
				}
			}
		})
	}
}

func TestThemeExport_AllFailuresExitOne(t *testing.T) {
	for _, tc := range themeExportFailures() {
		t.Run(tc.name, func(t *testing.T) {
			tc.seed(t)

			requireOrdinaryError(t, execThemeExport(t, tc.slug).err)
		})
	}
}

func TestThemeExport_StdoutIsEmptyOnFailure(t *testing.T) {
	for _, tc := range themeExportFailures() {
		t.Run(tc.name, func(t *testing.T) {
			tc.seed(t)

			run := execThemeExport(t, tc.slug)

			if run.err == nil {
				t.Fatalf("theme export %s succeeded, want a refusal", tc.slug)
			}
			if len(run.stdout) != 0 {
				t.Errorf("stdout = %q, want nothing", run.stdout)
			}
		})
	}

	t.Run("the success path does write to stdout", func(t *testing.T) {
		run := execThemeExport(t, theme.DefaultDarkSlug)

		if run.err != nil {
			t.Fatalf("theme export %s returned %v", theme.DefaultDarkSlug, run.err)
		}
		if len(run.stdout) == 0 {
			t.Fatal("the success path wrote nothing — the empty-stdout assertions above would be vacuous")
		}
	})
}

func TestThemeExport_UsesSharedByNameResolver(t *testing.T) {
	t.Run("the four refusal frames are unchanged", func(t *testing.T) {
		t.Run("an unknown slug", func(t *testing.T) {
			useThemesDir(t)

			requireExportRefusal(t, execThemeExport(t, "no-such-theme"), "no theme named no-such-theme")
		})

		t.Run("an invalid drop-in", func(t *testing.T) {
			seedThemesDir(t, "mine", sourceMissingTokens(t, "text.primary"))

			requireExportRefusal(t, execThemeExport(t, "mine"), "theme mine is not valid: missing tokens")
		})

		t.Run("a slug failing the charset check", func(t *testing.T) {
			useThemesDir(t)

			requireExportRefusal(t, execThemeExport(t, "Nord"), "theme Nord is not valid: bad name")
		})

		t.Run("an unreadable drop-in", func(t *testing.T) {
			osErr := themetest.DenyRead(t, filepath.Join(seedThemesDir(t, "mine", validThemeSource(t)), "mine.theme"))

			requireExportRefusal(t, execThemeExport(t, "mine"), "theme mine could not be read: "+osErr.Error())
		})

		t.Run("a valid drop-in still resolves", func(t *testing.T) {
			want := validThemeSource(t)
			seedThemesDir(t, "nord-lee", want)

			run := execThemeExport(t, "nord-lee")

			if run.err != nil {
				t.Fatalf("theme export nord-lee returned %v (stderr: %q)", run.err, run.stderr)
			}
			if !bytes.Equal(run.stdout, want) {
				t.Errorf("theme export nord-lee wrote:\n%s\nwant the file's bytes verbatim:\n%s", run.stdout, want)
			}
		})
	})

	// Each banned symbol is one step of the ordering that belongs to
	// internal/theme — a way two by-name resolvers could diverge.
	t.Run("the export command re-implements no step of the ordering", func(t *testing.T) {
		source := sourceguardtest.PackageSource(t, ".", "theme.go")

		banned := map[string]string{
			"ValidSlug":   "re-runs the charset check",
			"LoadBuiltin": "consults the embedded set",
			"LoadFile":    "reads the themes directory itself",
			"Join":        "composes a path from the slug",
			"Lstat":       "draws the absent-versus-unreadable line itself",
		}
		ast.Inspect(source.File, func(n ast.Node) bool {
			ident, isIdent := n.(*ast.Ident)
			if !isIdent {
				return true
			}
			if why, isBanned := banned[ident.Name]; isBanned {
				t.Errorf("cmd/theme.go:%d names %s, which %s — the by-name resolution ordering lives in theme.Loader.ResolveByName alone", source.Position(ident.Pos()).Line, ident.Name, why)
			}
			return true
		})
	})

	t.Run("it still emits no theme records", func(t *testing.T) {
		_ = themetest.DenyDir(t, seedThemesDir(t, "mine", validThemeSource(t)))
		sink := logtest.Install(t)

		execThemeExport(t, "mine")

		if records := sink.Records(); len(records) != 0 {
			t.Errorf("theme export emitted %d log records, want none — including the shared resolver's directory-unusable line: %+v", len(records), records)
		}
	})
}

func TestThemeExport_ReservedAndFilenameReasonsAreUnreachable(t *testing.T) {
	t.Run("a colliding drop-in never reports reserved name", func(t *testing.T) {
		slugs := theme.BuiltinSlugs()
		if len(slugs) == 0 {
			t.Fatal("BuiltinSlugs() is empty — every assertion below would be vacuous")
		}

		for _, slug := range slugs {
			t.Run(slug, func(t *testing.T) {
				shadow := scrambledThemeSource(t)
				seedThemesDir(t, slug, shadow)
				want, found := theme.BuiltinBytes(slug)
				if !found {
					t.Fatalf("BuiltinBytes(%q) reports not found", slug)
				}
				if bytes.Equal(shadow, want) {
					t.Fatal("the shadowing file equals the built-in — it would not distinguish the two")
				}

				run := execThemeExport(t, slug)

				if run.err != nil {
					t.Fatalf("theme export %s returned %v, want the built-in's bytes — a colliding file must never be reached", slug, run.err)
				}
				if !bytes.Equal(run.stdout, want) {
					t.Errorf("theme export %s wrote the drop-in's bytes, want the embedded ones", slug)
				}
			})
		}
	})

	// The extension is restated here because the composition itself belongs to
	// theme.Loader.ResolveByName — cmd holds no path-building code.
	const themeFileExtension = ".theme"

	t.Run("a composed filename always clears the filename rules", func(t *testing.T) {
		slugs := []string{"a", "0", "nord", "nord-lee", "nord-", "n0rd-2", strings.Repeat("x", 200)}

		for _, slug := range slugs {
			t.Run(slug, func(t *testing.T) {
				if !theme.ValidSlug(slug) {
					t.Fatalf("ValidSlug(%q) = false — the fixture is not a slug export would compose a path from", slug)
				}

				got, rejection := theme.SlugFromFilename(slug + themeFileExtension)

				if rejection != nil {
					t.Fatalf("SlugFromFilename(%q) = %v, want no rejection — the composed filename is always <valid-slug>.theme", slug+themeFileExtension, rejection)
				}
				if got != slug {
					t.Errorf("SlugFromFilename(%q) = %q, want %q", slug+themeFileExtension, got, slug)
				}
			})
		}
	})

	t.Run("a valid absent slug is unknown, never bad name", func(t *testing.T) {
		useThemesDir(t)

		requireExportRefusal(t, execThemeExport(t, "nord-"), "no theme named nord-")
	})
}
