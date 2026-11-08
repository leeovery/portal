package log

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

func snapshotInitState(t *testing.T) {
	t.Helper()
	restoreHandler := snapshotHandler()
	prevStart := startTime
	t.Cleanup(func() {
		restoreHandler()
		startTime = prevStart
	})
}

func TestInit_RoutesPreInitCachedLoggerToConfiguredHandler(t *testing.T) {
	snapshotInitState(t)

	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	cached.Info("after init")

	line := readPortalLog(t, dir)
	if !strings.Contains(line, " daemon: after init ") {
		t.Errorf("expected component prefix from cached logger, got: %q", line)
	}
	for _, want := range []string{"pid=", "version=0.5.0", "process_role=tui"} {
		if !strings.Contains(line, want) {
			t.Errorf("expected baseline %q on cached-logger line, got: %q", want, line)
		}
	}
	wantPID := "pid=" + strconv.Itoa(os.Getpid())
	if !strings.Contains(line, wantPID) {
		t.Errorf("expected captured pid baseline %q, got: %q", wantPID, line)
	}
}

func TestInit_AppliesResolvedLevelFromEnv(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "error")

	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	cached.Info("info-suppressed")
	cached.Error("error-emitted")

	line := readPortalLog(t, dir)
	if strings.Contains(line, "info-suppressed") {
		t.Errorf("INFO must be suppressed when resolved level is error, got: %q", line)
	}
	if !strings.Contains(line, "error-emitted") {
		t.Errorf("ERROR must be emitted when resolved level is error, got: %q", line)
	}
}

func TestInit_SecondInitRePointsHandlerWithoutPanic(t *testing.T) {
	snapshotInitState(t)

	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("first Init returned error: %v", err)
	}

	dir2 := t.TempDir()
	if err := Init(dir2, "0.5.0", "daemon"); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}

	cached.Info("after second init")

	line := readPortalLog(t, dir2)
	if !strings.Contains(line, "process_role=daemon") {
		t.Errorf("expected new process_role baseline after second Init, got: %q", line)
	}
	if strings.Contains(line, "process_role=tui") {
		t.Errorf("must not carry stale process_role after re-point, got: %q", line)
	}
}

func TestInit_CapturesStartTimeAndCloseComputesNonNegativeTook(t *testing.T) {
	snapshotInitState(t)

	before := time.Now()
	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	after := time.Now()

	if startTime.Before(before) || startTime.After(after) {
		t.Fatalf("startTime %v not captured within Init window [%v, %v]", startTime, before, after)
	}

	took := computeTook()
	if took < 0 {
		t.Errorf("computeTook returned negative duration %v", took)
	}
}

func TestInit_SecondInitResetsStartTime(t *testing.T) {
	snapshotInitState(t)

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("first Init returned error: %v", err)
	}
	first := startTime

	startTime = time.Time{}.Add(time.Hour)
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}

	if !startTime.After(first) {
		t.Errorf("second Init must reset startTime to a later instant; first=%v second=%v", first, startTime)
	}
	if startTime.Equal(time.Time{}.Add(time.Hour)) {
		t.Error("second Init did not overwrite the sentinel startTime")
	}
}

// Returning normally is the assertion: an os.Exit inside Close would kill the
// test binary instead.
func TestClose_ReturnsWithoutTerminatingProcess(t *testing.T) {
	snapshotInitState(t)

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	Close(0)
}

func TestClose_SafeBeforeAnyInit(t *testing.T) {
	snapshotInitState(t)

	SetTestHandler(t, &recordingHandler{})

	startTime = time.Time{}

	Close(0)
}

func TestInit_WritesThroughDateAwareSinkToDatedFileAndSymlink(t *testing.T) {
	snapshotInitState(t)

	day := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	cached.Info("dated")

	datedPath := filepath.Join(dir, "portal.log.2026-05-29")
	b, err := os.ReadFile(datedPath)
	if err != nil {
		t.Fatalf("reading dated file %s: %v", datedPath, err)
	}
	if !strings.Contains(string(b), " daemon: dated ") {
		t.Errorf("expected record in dated file, got: %q", string(b))
	}

	target, err := os.Readlink(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("readlink portal.log: %v", err)
	}
	if filepath.Base(target) != "portal.log.2026-05-29" {
		t.Errorf("portal.log symlink target = %q, want portal.log.2026-05-29", target)
	}
}

func TestInit_FallsBackToStderrAndReturnsErrorOnOpenFailure(t *testing.T) {
	snapshotInitState(t)

	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	badDir := filepath.Join(blocker, "state")

	if err := Init(badDir, "0.5.0", "tui"); err == nil {
		t.Error("expected advisory open error from Init on an unwritable stateDir, got nil")
	}

	For("daemon").Info("after-failure")
}

func TestInit_DoesNotImportInternalState(t *testing.T) {
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		for _, imp := range source.File.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasSuffix(path, "internal/state") {
				t.Errorf("%s imports %q — internal/log must not depend on internal/state (import-cycle guard)", source.Path, path)
			}
		}
	}
}

func TestInit_ProcessStartQuotesAnArgumentHoldingASpace(t *testing.T) {
	snapshotInitState(t)
	prevArgs := os.Args
	t.Cleanup(func() { os.Args = prevArgs })
	os.Args = []string{"/usr/local/bin/portal", "state", "hydrate", "--session", "portal-xn85z5 zmw rework", "--file", "/tmp/s.vt"}

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "hydrate"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	starts := processLinesByMessage(parseProcessLines(t, readPortalLog(t, dir)), "start")
	if len(starts) != 1 {
		t.Fatalf("got %d process:start lines, want exactly 1", len(starts))
	}
	const want = "state hydrate --session 'portal-xn85z5 zmw rework' --file /tmp/s.vt"
	if got := starts[0].attrs["args"]; got != want {
		t.Errorf("start args = %q, want %q", got, want)
	}
}

func TestInit_EmitsProcessStartThenLogLevelResolvedInOrder(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "debug")
	fixedClock(t, time.Date(2026, 5, 30, 14, 0, 0, 0, time.UTC))

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "daemon"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	lines := parseProcessLines(t, readPortalLog(t, dir))

	starts := processLinesByMessage(lines, "start")
	if len(starts) != 1 {
		t.Fatalf("got %d process:start lines, want exactly 1", len(starts))
	}
	resolved := processLinesByMessage(lines, "log-level resolved")
	if len(resolved) != 1 {
		t.Fatalf("got %d process:log-level resolved lines, want exactly 1", len(resolved))
	}

	if lines[0].message != "start" {
		t.Errorf("first process line message = %q, want start", lines[0].message)
	}
	if len(lines) < 2 || lines[1].message != "log-level resolved" {
		t.Errorf("second process line message = %q, want log-level resolved (immediately after start)", lines[1].message)
	}

	start := starts[0]
	if start.level != "INFO" {
		t.Errorf("start level = %q, want INFO", start.level)
	}
	if got := start.attrs["cmd"]; got != filepath.Base(os.Args[0]) {
		t.Errorf("start cmd = %q, want %q", got, filepath.Base(os.Args[0]))
	}
	if got, want := start.attrs["args"], shellquote.Readable(os.Args[1:]); got != want {
		t.Errorf("start args = %q, want %q", got, want)
	}

	r := resolved[0]
	if r.level != "INFO" {
		t.Errorf("log-level resolved level = %q, want INFO", r.level)
	}
	if got := r.attrs["resolved"]; got != "debug" {
		t.Errorf("resolved = %q, want debug", got)
	}
	if got := r.attrs["source"]; got != "env" {
		t.Errorf("source = %q, want env", got)
	}
	if got := r.attrs["raw"]; got != "debug" {
		t.Errorf("raw = %q, want debug (verbatim env value)", got)
	}
}

func TestInit_LogLevelResolvedSourceDefaultWhenUnset(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	r := singleProcessLine(t, readPortalLog(t, dir), "log-level resolved")
	if got := r.attrs["resolved"]; got != "info" {
		t.Errorf("resolved = %q, want info (unset default)", got)
	}
	if got := r.attrs["source"]; got != "default" {
		t.Errorf("source = %q, want default", got)
	}
	if got, ok := r.attrs["raw"]; !ok || got != "" {
		t.Errorf("raw = %q (present=%v), want empty string for unset env", got, ok)
	}
}

func TestInit_LogLevelResolvedSourceFallbackEmitsBootstrapWarn(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "trace")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "daemon"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	raw := readPortalLog(t, dir)

	r := singleProcessLine(t, raw, "log-level resolved")
	if got := r.attrs["resolved"]; got != "info" {
		t.Errorf("resolved = %q, want info (invalid value falls back)", got)
	}
	if got := r.attrs["source"]; got != "fallback" {
		t.Errorf("source = %q, want fallback", got)
	}
	if got := r.attrs["raw"]; got != "trace" {
		t.Errorf("raw = %q, want trace (verbatim invalid value)", got)
	}

	if !strings.Contains(raw, " bootstrap: invalid PORTAL_LOG_LEVEL raw=trace resolved=info ") {
		t.Errorf("expected bootstrap invalid PORTAL_LOG_LEVEL WARN line, got:\n%s", raw)
	}
	warns := bootstrapWarnLines(t, raw, "invalid PORTAL_LOG_LEVEL")
	if len(warns) != 1 {
		t.Fatalf("got %d bootstrap invalid-value WARN lines, want exactly 1", len(warns))
	}
	if warns[0].level != "WARN" {
		t.Errorf("invalid-value line level = %q, want WARN", warns[0].level)
	}
}

func TestInit_NoBootstrapWarnWhenSourceNotFallback(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	raw := readPortalLog(t, dir)
	if strings.Contains(raw, "invalid PORTAL_LOG_LEVEL") {
		t.Errorf("valid env value must NOT emit the invalid-value WARN, got:\n%s", raw)
	}
	r := singleProcessLine(t, raw, "log-level resolved")
	if got := r.attrs["source"]; got != "env" {
		t.Errorf("source = %q, want env for a valid value", got)
	}
}

func TestInit_BothProcessLinesVisibleAtWarnLevel(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "daemon"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	lines := parseProcessLines(t, readPortalLog(t, dir))
	if len(processLinesByMessage(lines, "start")) != 1 {
		t.Errorf("process:start must be visible at PORTAL_LOG_LEVEL=warn (level-filter bypass)")
	}
	resolved := processLinesByMessage(lines, "log-level resolved")
	if len(resolved) != 1 {
		t.Errorf("process:log-level resolved must be visible at PORTAL_LOG_LEVEL=warn (level-filter bypass)")
	}
	if len(resolved) == 1 && resolved[0].level != "INFO" {
		t.Errorf("log-level resolved level = %q, want INFO (semantically INFO, bypass is the mechanism)", resolved[0].level)
	}
}

func TestInit_ProcessLinesCarryAutoInjectedBaselinesNotDoubleEmitted(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "info")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "daemon"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	raw := readPortalLog(t, dir)
	for _, msg := range []string{"start", "log-level resolved"} {
		line := singleProcessLineRaw(t, raw, msg)
		wantPID := "pid=" + strconv.Itoa(os.Getpid())
		for _, want := range []string{wantPID, "version=0.5.0", "process_role=daemon"} {
			if !strings.Contains(line, want) {
				t.Errorf("%q line missing auto-injected baseline %q: %q", msg, want, line)
			}
			if n := strings.Count(line, want); n != 1 {
				t.Errorf("%q line carries baseline %q %d times, want exactly 1 (call site must NOT pass baselines)", msg, want, n)
			}
		}
	}
}

func TestInit_SecondInitReEmitsBothProcessLines(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "info")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("first Init returned error: %v", err)
	}

	dir2 := t.TempDir()
	if err := Init(dir2, "0.5.0", "daemon"); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}

	lines := parseProcessLines(t, readPortalLog(t, dir2))
	if len(processLinesByMessage(lines, "start")) != 1 {
		t.Errorf("second Init must re-emit exactly one process:start into the new dir")
	}
	if len(processLinesByMessage(lines, "log-level resolved")) != 1 {
		t.Errorf("second Init must re-emit exactly one process:log-level resolved into the new dir")
	}
}

func TestInit_MidnightRollWithRetentionDeletionDoesNotDeadlock(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")
	set := fixedClock(t, mustDate(2026, 5, 29))

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "daemon"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	old := touchFile(t, dir, "portal.log.2026-01-01")

	set(mustDate(2026, 5, 30))

	done := make(chan struct{})
	go func() {
		defer close(done)
		For("daemon").Info("cross-midnight tick")
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cross-midnight log call deadlocked: retention breadcrumb re-entered the sink under its mutex")
	}

	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("aged file %s still present (stat err = %v); the midnight roll must run the retention sweep", filepath.Base(old), err)
	}

	day2 := readDayFile(t, dir, "2026-05-30")
	if !strings.Contains(day2, " daemon: cross-midnight tick ") {
		t.Errorf("crossing record missing from day-two file:\n%s", day2)
	}
	if !strings.Contains(day2, " log-rotate: deleted ") {
		t.Errorf("retention deletion breadcrumb missing from day-two file:\n%s", day2)
	}

	For("daemon").Info("post-roll tick")
	if !strings.Contains(readDayFile(t, dir, "2026-05-30"), " daemon: post-roll tick ") {
		t.Error("post-roll record missing; logging did not stay live after the day roll")
	}
}

func TestInit_FirstOfDaySweepBreadcrumbLandsInPortalLog(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")
	fixedClock(t, mustDate(2026, 5, 30))

	dir := t.TempDir()
	old := touchFile(t, dir, "portal.log.2026-01-01")

	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("aged file %s still present (stat err = %v); Init's first record must fire the queued first-of-day sweep", filepath.Base(old), err)
	}

	raw := readPortalLog(t, dir)
	if !strings.Contains(raw, " log-rotate: deleted ") {
		t.Errorf("deletion breadcrumb missing from portal.log (must route through the configured handler, not pre-Init stderr):\n%s", raw)
	}
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], " process: start ") {
		t.Errorf("first portal.log record = %q, want process: start (sweep fires after it)", lines[0])
	}
}

func readDayFile(t *testing.T, dir, date string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "portal.log."+date))
	if err != nil {
		t.Fatalf("reading portal.log.%s under %s: %v", date, dir, err)
	}
	return string(b)
}

func processLinesByMessage(lines []logLine, msg string) []logLine {
	var out []logLine
	for _, l := range lines {
		if l.message == msg {
			out = append(out, l)
		}
	}
	return out
}

func singleProcessLine(t *testing.T, raw, msg string) logLine {
	t.Helper()
	got := processLinesByMessage(parseProcessLines(t, raw), msg)
	if len(got) != 1 {
		t.Fatalf("got %d process:%s lines, want exactly 1\nlog:\n%s", len(got), msg, raw)
	}
	return got[0]
}

func singleProcessLineRaw(t *testing.T, raw, msg string) string {
	t.Helper()
	prefix := " " + processComponent + ": " + msg + " "
	var found []string
	for line := range strings.SplitSeq(raw, "\n") {
		if strings.Contains(line, prefix) {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d raw %q lines, want exactly 1\nlog:\n%s", len(found), msg, raw)
	}
	return found[0]
}

func bootstrapWarnLines(t *testing.T, raw, msg string) []logLine {
	t.Helper()
	var out []logLine
	for line := range strings.SplitSeq(raw, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		comp, ok := strings.CutSuffix(fields[2], ":")
		if !ok || comp != "bootstrap" {
			continue
		}
		rest := fields[3:]
		if !strings.HasPrefix(strings.Join(rest, " "), msg) {
			continue
		}
		out = append(out, logLine{level: fields[1], component: comp, message: msg})
	}
	return out
}

type logLine struct {
	level     string
	component string
	message   string
	attrs     map[string]string
}

func parseProcessLines(t *testing.T, raw string) []logLine {
	t.Helper()
	var out []logLine
	for line := range strings.SplitSeq(raw, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		comp, ok := strings.CutSuffix(fields[2], ":")
		if !ok || comp != processComponent {
			continue
		}
		rest := fields[3:]
		msg, attrStart := matchProcessMessage(rest)
		out = append(out, logLine{
			level:     fields[1],
			component: comp,
			message:   msg,
			attrs:     parseAttrs(t, line, rest[attrStart:]),
		})
	}
	return out
}

func matchProcessMessage(tokens []string) (msg string, attrStart int) {
	if len(tokens) >= 2 && tokens[0] == "log-level" && tokens[1] == "resolved" {
		return "log-level resolved", 2
	}
	if len(tokens) >= 1 {
		return tokens[0], 1
	}
	return "", 0
}

func parseAttrs(t *testing.T, line string, tokens []string) map[string]string {
	t.Helper()
	attrs := map[string]string{}
	for i := 0; i < len(tokens); i++ {
		key, val, ok := strings.Cut(tokens[i], "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(val, `"`) && (len(val) < 2 || !strings.HasSuffix(val, `"`)) {
			if recovered, ok := recoverQuotedAttr(line, key); ok {
				attrs[key] = recovered
				for i+1 < len(tokens) && !strings.HasSuffix(tokens[i], `"`) {
					i++
				}
				continue
			}
		}
		attrs[key] = strings.Trim(val, `"`)
	}
	return attrs
}

func recoverQuotedAttr(line, key string) (string, bool) {
	anchor := " " + key + `="`
	idx := strings.Index(line, anchor)
	if idx < 0 {
		return "", false
	}
	start := idx + len(anchor)
	end := strings.IndexByte(line[start:], '"')
	if end < 0 {
		return "", false
	}
	return line[start : start+end], true
}

func readPortalLog(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("reading portal.log under %s: %v", dir, err)
	}
	if len(b) == 0 {
		t.Fatalf("portal.log under %s is empty", dir)
	}
	return string(b)
}
