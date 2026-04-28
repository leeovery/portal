package state_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// rfc3339Pattern matches the timestamp prefix written by Logger.write.
// time.RFC3339 in UTC has the form 2006-01-02T15:04:05Z.
var rfc3339Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

func openLogger(t *testing.T, path string, rotate bool) *state.Logger {
	t.Helper()
	lg, err := state.OpenLogger(path, rotate)
	if err != nil {
		t.Fatalf("OpenLogger(%s, %v): %v", path, rotate, err)
	}
	return lg
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestLogger_FormatsLineWithPipeDelimitedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Warn("daemon", "starting up pid=%d", 1234)
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data := readFile(t, path)
	line := strings.TrimRight(string(data), "\n")
	parts := strings.Split(line, " | ")
	if len(parts) != 4 {
		t.Fatalf("expected 4 pipe-delimited fields, got %d (%q)", len(parts), line)
	}
	if !rfc3339Pattern.MatchString(parts[0]) {
		t.Errorf("timestamp %q does not match RFC3339 UTC pattern", parts[0])
	}
	if parts[1] != "WARN" {
		t.Errorf("level = %q, want WARN", parts[1])
	}
	if parts[2] != "daemon" {
		t.Errorf("component = %q, want daemon", parts[2])
	}
	if parts[3] != "starting up pid=1234" {
		t.Errorf("message = %q, want %q", parts[3], "starting up pid=1234")
	}
	if !bytes.HasSuffix(data, []byte{'\n'}) {
		t.Errorf("log line not newline-terminated: %q", data)
	}
}

func TestLogger_FiltersDebugAndInfoWhenLevelUnset(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Debug("daemon", "dropped-debug")
	lg.Info("daemon", "dropped-info")
	lg.Warn("daemon", "kept-warn")
	lg.Error("daemon", "kept-error")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data := readFile(t, path)
	got := string(data)
	if strings.Contains(got, "DEBUG") {
		t.Errorf("DEBUG line written despite default WARN; got:\n%s", got)
	}
	if strings.Contains(got, "| INFO |") {
		t.Errorf("INFO line written despite default WARN; got:\n%s", got)
	}
	if !strings.Contains(got, "kept-warn") {
		t.Errorf("WARN line missing; got:\n%s", got)
	}
	if !strings.Contains(got, "kept-error") {
		t.Errorf("ERROR line missing; got:\n%s", got)
	}
}

func TestLogger_IncludesDebugWhenEnvIsDebug(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "debug")

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Debug("daemon", "tick")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data := readFile(t, path)
	if !strings.Contains(string(data), "DEBUG") {
		t.Errorf("DEBUG line missing when PORTAL_LOG_LEVEL=debug; got:\n%s", data)
	}
	if !strings.Contains(string(data), "tick") {
		t.Errorf("DEBUG message missing; got:\n%s", data)
	}
}

func TestLogger_AllLevelLabelsRender(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "debug")

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Debug("daemon", "d")
	lg.Info("daemon", "i")
	lg.Warn("daemon", "w")
	lg.Error("daemon", "e")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data := readFile(t, path)
	for _, label := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		if !strings.Contains(string(data), "| "+label+" |") {
			t.Errorf("expected level label %q in output:\n%s", label, data)
		}
	}
}

func TestLogger_RotatesAtOneMiBOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	// Pre-write 1 MiB + 1 byte of distinguishable content.
	original := bytes.Repeat([]byte("A"), 1<<20)
	original = append(original, 'X')
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	lg := openLogger(t, path, true)
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	old, err := os.ReadFile(path + ".old")
	if err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	if !bytes.Equal(old, original) {
		t.Errorf(".old does not contain the original contents (len=%d, want %d)", len(old), len(original))
	}

	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("post-rotation portal.log missing: %v", err)
	}
	if len(cur) != 0 {
		t.Errorf("post-rotation portal.log not empty: len=%d", len(cur))
	}
}

func TestLogger_OverwritesExistingOldOnRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")
	oldPath := path + ".old"

	if err := os.WriteFile(oldPath, []byte("stale-old"), 0o600); err != nil {
		t.Fatalf("seed old: %v", err)
	}
	original := bytes.Repeat([]byte("B"), 1<<20)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed current: %v", err)
	}

	lg := openLogger(t, path, true)
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("read .old: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf(".old not overwritten with current contents")
	}
}

func TestLogger_DoesNotRotateBelowOneMiB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	original := bytes.Repeat([]byte("C"), 1024)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lg := openLogger(t, path, true)
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Errorf(".old should not exist after sub-1 MiB open; stat err = %v", err)
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current: %v", err)
	}
	if !bytes.Equal(cur, original) {
		t.Errorf("portal.log contents changed on sub-threshold open")
	}
}

func TestLogger_DoesNotRotateWhenFileAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, true)
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Errorf(".old should not exist when starting from no current log; stat err = %v", err)
	}
}

func TestLogger_DoesNotRotateWhenRotateFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	huge := bytes.Repeat([]byte("D"), 5*(1<<20))
	if err := os.WriteFile(path, huge, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lg := openLogger(t, path, false)
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Errorf(".old must not exist when rotate=false; stat err = %v", err)
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current: %v", err)
	}
	if len(cur) != len(huge) {
		t.Errorf("portal.log size changed despite rotate=false: got %d, want %d", len(cur), len(huge))
	}
}

func TestLogger_OpensWithMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Info("daemon", "hello")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("portal.log mode = %o; want 0600", perm)
	}
}

func TestLogger_AppendsAcrossReopens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	first := openLogger(t, path, false)
	first.Warn("daemon", "first")
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	second := openLogger(t, path, false)
	second.Warn("daemon", "second")
	if err := second.Close(); err != nil {
		t.Fatalf("close second: %v", err)
	}

	data := readFile(t, path)
	got := string(data)
	if !strings.Contains(got, "first") {
		t.Errorf("first entry lost across reopen:\n%s", got)
	}
	if !strings.Contains(got, "second") {
		t.Errorf("second entry missing:\n%s", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines after two appends, got %d:\n%s", len(lines), got)
	}
}

func TestLogger_CreatesParentDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "deep", "portal.log")

	lg := openLogger(t, nested, false)
	lg.Info("daemon", "hi")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("log file not created in nested dir: %v", err)
	}
}

func TestLogger_NilReceiverIsSafeNoOp(t *testing.T) {
	var lg *state.Logger
	// Each call must not panic.
	lg.Debug("c", "d")
	lg.Info("c", "i")
	lg.Warn("c", "w")
	lg.Error("c", "e")
	if err := lg.Close(); err != nil {
		t.Errorf("Close on nil logger returned error: %v", err)
	}
}

func TestLogger_EmitsInfoWarnErrorWhenLevelInfo(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Debug("daemon", "dropped")
	lg.Info("daemon", "kept-info")
	lg.Warn("daemon", "kept-warn")
	lg.Error("daemon", "kept-error")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got := string(readFile(t, path))
	if strings.Contains(got, "DEBUG") {
		t.Errorf("DEBUG line written despite PORTAL_LOG_LEVEL=info; got:\n%s", got)
	}
	for _, label := range []string{"INFO", "WARN", "ERROR"} {
		if !strings.Contains(got, "| "+label+" |") {
			t.Errorf("missing %s entry under PORTAL_LOG_LEVEL=info; got:\n%s", label, got)
		}
	}
}

func TestLogger_DefaultsToWarnOnInvalidLevel(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "trace")

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Debug("daemon", "dropped-debug")
	lg.Info("daemon", "dropped-info")
	lg.Warn("daemon", "kept-warn")
	lg.Error("daemon", "kept-error")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got := string(readFile(t, path))
	if strings.Contains(got, "DEBUG") {
		t.Errorf("DEBUG line written for invalid PORTAL_LOG_LEVEL=trace; got:\n%s", got)
	}
	if strings.Contains(got, "| INFO |") {
		t.Errorf("INFO line written for invalid PORTAL_LOG_LEVEL=trace; got:\n%s", got)
	}
	if !strings.Contains(got, "kept-warn") || !strings.Contains(got, "kept-error") {
		t.Errorf("WARN/ERROR missing under invalid level fallback; got:\n%s", got)
	}
}

func TestLogger_LevelParsingIsCaseInsensitive(t *testing.T) {
	for _, value := range []string{"DEBUG", "Debug", "dEbUg"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PORTAL_LOG_LEVEL", value)

			dir := t.TempDir()
			path := filepath.Join(dir, "portal.log")

			lg := openLogger(t, path, false)
			lg.Debug("daemon", "tick")
			if err := lg.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			got := string(readFile(t, path))
			if !strings.Contains(got, "DEBUG") {
				t.Errorf("DEBUG missing for PORTAL_LOG_LEVEL=%q; got:\n%s", value, got)
			}
		})
	}
}

func TestLogger_PreservesEmbeddedPipesInMessage(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "debug")

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Warn("restore", "a | b | c")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	line := strings.TrimRight(string(readFile(t, path)), "\n")
	parts := strings.SplitN(line, " | ", 4)
	if len(parts) != 4 {
		t.Fatalf("expected 4 fields after SplitN limit=4, got %d (%q)", len(parts), line)
	}
	if parts[3] != "a | b | c" {
		t.Errorf("message field = %q, want %q (embedded pipes must be verbatim)", parts[3], "a | b | c")
	}
}

func TestLogger_UsesUTCRFC3339TimestampsRegardlessOfLocalTimeZone(t *testing.T) {
	// Force a non-UTC local zone so a naive time.Now().Format(time.RFC3339)
	// would emit a "+04:00" suffix and fail the assertion below.
	prev := time.Local
	time.Local = time.FixedZone("TEST", 4*3600)
	t.Cleanup(func() { time.Local = prev })

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)
	lg.Warn("daemon", "ts-check")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	line := strings.TrimRight(string(readFile(t, path)), "\n")
	parts := strings.SplitN(line, " | ", 4)
	if len(parts) != 4 {
		t.Fatalf("expected 4 fields, got %d (%q)", len(parts), line)
	}
	ts := parts[0]
	if !strings.HasSuffix(ts, "Z") {
		t.Errorf("timestamp %q does not end in Z (UTC indicator)", ts)
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("timestamp %q does not parse as RFC3339: %v", ts, err)
	}
}

func TestLogger_IsSafeForConcurrentUse(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "debug")

	dir := t.TempDir()
	path := filepath.Join(dir, "portal.log")

	lg := openLogger(t, path, false)

	const goroutines = 16
	const linesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < linesPerGoroutine; i++ {
				lg.Info("daemon", "g=%d i=%d", id, i)
			}
		}(g)
	}
	wg.Wait()

	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data := readFile(t, path)
	// Trim trailing newline then split — every well-formed line ends in \n,
	// so split should produce exactly N fields, all matching the format.
	trimmed := strings.TrimRight(string(data), "\n")
	lines := strings.Split(trimmed, "\n")
	if got, want := len(lines), goroutines*linesPerGoroutine; got != want {
		t.Fatalf("got %d lines, want %d (concurrent writes lost or interleaved)", got, want)
	}
	// Match every line against the canonical "ts | LEVEL | comp | msg" shape.
	linePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z \| (DEBUG|INFO|WARN|ERROR) \| daemon \| g=\d+ i=\d+$`)
	for i, ln := range lines {
		if !linePattern.MatchString(ln) {
			t.Fatalf("line %d garbled: %q", i, ln)
		}
	}
}

func TestLogger_ExposesComponentConstants(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"daemon", state.ComponentDaemon, "daemon"},
		{"restore", state.ComponentRestore, "restore"},
		{"hydrate", state.ComponentHydrate, "hydrate"},
		{"notify", state.ComponentNotify, "notify"},
		{"hooks", state.ComponentHooks, "hooks"},
		{"bootstrap", state.ComponentBootstrap, "bootstrap"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("Component %q = %q, want %q", c.name, c.got, c.want)
			}
		})
	}
}
