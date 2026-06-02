package log

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// snapshotInitState captures the package-private Init-owned state (handler +
// startTime) and returns a restore func so Init-exercising tests do not leak
// state into siblings. setHandler is restored via the shared snapshotHandler;
// startTime is restored directly because it is package-private to this _test.go.
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

	// Cache a logger BEFORE Init, mirroring package-init binding.
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

	// Second Init with a different process_role must re-point without panicking.
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

	// Force an observable gap, then re-Init.
	startTime = time.Time{}.Add(time.Hour) // sentinel distinct from any real now
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

func TestClose_ReturnsWithoutTerminatingProcess(t *testing.T) {
	snapshotInitState(t)

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// If Close called os.Exit, this test process would terminate and this test
	// would be reported as failed-to-complete (its t.Cleanup would not run, and
	// every sibling test would be skipped). Returning normally from the test
	// function is itself the proof that Close owns no control flow.
	Close(0)
}

func TestClose_SafeBeforeAnyInit(t *testing.T) {
	snapshotInitState(t)

	// Capture the now-real Close emission so it does not leak to the pre-Init
	// stderr default; the no-panic contract is what this test asserts.
	SetTestHandler(t, &recordingHandler{})

	// Reset startTime to its zero value to model a never-Init'd process.
	startTime = time.Time{}

	// Must not panic.
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

	// The record must land in the date-keyed file, proving Init wired the
	// date-aware rotating sink (not the Phase-1 plain portal.log open).
	datedPath := filepath.Join(dir, "portal.log.2026-05-29")
	b, err := os.ReadFile(datedPath)
	if err != nil {
		t.Fatalf("reading dated file %s: %v", datedPath, err)
	}
	if !strings.Contains(string(b), " daemon: dated ") {
		t.Errorf("expected record in dated file, got: %q", string(b))
	}

	// portal.log must be the live-target symlink pointing at today's file.
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

	// A stateDir that cannot hold the day file (a regular file in the path)
	// forces the eager open probe to fail; Init must surface the error
	// advisorily and still install a usable (stderr-fallback) handler.
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	badDir := filepath.Join(blocker, "state") // path component is a regular file.

	if err := Init(badDir, "0.5.0", "tui"); err == nil {
		t.Error("expected advisory open error from Init on an unwritable stateDir, got nil")
	}

	// The handler must still be usable (no panic) even after the open failure.
	For("daemon").Info("after-failure")
}

func TestInit_DoesNotImportInternalState(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, imp := range af.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasSuffix(path, "internal/state") {
				t.Errorf("%s imports %q — internal/log must not depend on internal/state (import-cycle guard)", f, path)
			}
		}
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

	// Order: start is the FIRST process line and log-level resolved is immediately
	// after it (no other process line between them).
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
	if got, want := start.attrs["args"], strings.Join(os.Args[1:], " "); got != want {
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
	// Empty resolves identically to unset (os.Getenv yields "" for both), and
	// t.Setenv restores cleanly without leaking env state into sibling tests.
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

	// Fallback ALSO emits the bootstrap-component invalid-value WARN. It is NOT a
	// lifecycle-bypass line, but source==fallback => resolved level is info, so the
	// configured handler (at INFO) renders the WARN (slog WARN >= INFO).
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
	// At warn level the resolved line is still INFO semantically, bypassing the gate.
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
			// Auto-injected exactly once — not double-emitted by the call site also
			// passing pid/version/process_role.
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

	// Second Init into a FRESH dir re-emits both process lines (the most recent
	// Init defines the logical start). Use a fresh dir so the count is unambiguous.
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

// processLinesByMessage returns the parsed process lines whose message equals msg.
func processLinesByMessage(lines []logLine, msg string) []logLine {
	var out []logLine
	for _, l := range lines {
		if l.message == msg {
			out = append(out, l)
		}
	}
	return out
}

// singleProcessLine parses raw and returns the sole process line for msg,
// failing the test if there is not exactly one.
func singleProcessLine(t *testing.T, raw, msg string) logLine {
	t.Helper()
	got := processLinesByMessage(parseProcessLines(t, raw), msg)
	if len(got) != 1 {
		t.Fatalf("got %d process:%s lines, want exactly 1\nlog:\n%s", len(got), msg, raw)
	}
	return got[0]
}

// singleProcessLineRaw returns the raw text of the sole "process: <msg>" line.
func singleProcessLineRaw(t *testing.T, raw, msg string) string {
	t.Helper()
	prefix := " " + processComponent + ": " + msg + " "
	var found []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, prefix) {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d raw %q lines, want exactly 1\nlog:\n%s", len(found), msg, raw)
	}
	return found[0]
}

// bootstrapWarnLines returns parsed bootstrap-component lines whose message equals msg.
func bootstrapWarnLines(t *testing.T, raw, msg string) []logLine {
	t.Helper()
	var out []logLine
	for _, line := range strings.Split(raw, "\n") {
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
		// "invalid PORTAL_LOG_LEVEL" is the two-word-plus message; match by prefix.
		if !strings.HasPrefix(strings.Join(rest, " "), msg) {
			continue
		}
		out = append(out, logLine{level: fields[1], component: comp, message: msg})
	}
	return out
}

// logLine is a parsed portal.log text-mode record: its level, component
// prefix, message, and key=value attrs (with quoteIfMultiWord quoting removed).
type logLine struct {
	level     string
	component string
	message   string
	attrs     map[string]string
}

// parseProcessLines extracts every "process:" component line from the raw
// portal.log text-mode output, in file order. It splits the
// "<time> <LEVEL> <component>: <msg> <attrs...>" shape, recognising the closed
// process lifecycle messages ("start", "log-level resolved") so the multi-word
// message is captured intact, then parses the trailing space-separated
// key=value attrs (unquoting quoteIfMultiWord values).
func parseProcessLines(t *testing.T, raw string) []logLine {
	t.Helper()
	var out []logLine
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// fields[0]=time, fields[1]=LEVEL, fields[2]=<component>: (prefix).
		comp, ok := strings.CutSuffix(fields[2], ":")
		if !ok || comp != processComponent {
			continue
		}
		rest := fields[3:]
		// Identify the message: the longest lifecycleBypassMsgs key that is a
		// space-joined prefix of the remaining tokens. The known process messages
		// here are single-word ("start") or two-word ("log-level resolved").
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

// matchProcessMessage returns the process message formed from the leading tokens
// and the index of the first attr token. It recognises the two-word
// "log-level resolved" and otherwise treats the single leading token as the msg.
func matchProcessMessage(tokens []string) (msg string, attrStart int) {
	if len(tokens) >= 2 && tokens[0] == "log-level" && tokens[1] == "resolved" {
		return "log-level resolved", 2
	}
	if len(tokens) >= 1 {
		return tokens[0], 1
	}
	return "", 0
}

// parseAttrs parses trailing key=value attr tokens into a map, unquoting values
// that quoteIfMultiWord wrapped in double quotes. A quoted value that spans
// multiple whitespace-split tokens (e.g. args="open .") is rejoined from the
// original line rather than the pre-split tokens.
func parseAttrs(t *testing.T, line string, tokens []string) map[string]string {
	t.Helper()
	attrs := map[string]string{}
	for i := 0; i < len(tokens); i++ {
		key, val, ok := strings.Cut(tokens[i], "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(val, `"`) && (len(val) < 2 || !strings.HasSuffix(val, `"`)) {
			// Quoted value split across tokens: recover the full quoted run from
			// the raw line via the key="..." anchor.
			if recovered, ok := recoverQuotedAttr(line, key); ok {
				attrs[key] = recovered
				// Skip the remaining tokens of this quoted value.
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

// recoverQuotedAttr extracts the unquoted value of a key="..." attr from the raw
// line, used when the value contains spaces (quoteIfMultiWord quoting).
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

// readPortalLog reads the portal.log written under dir, failing the test if it
// is missing or empty. Returns the full file contents.
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
