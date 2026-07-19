// Tests in this file MUST NOT use t.Parallel — they manipulate filesystem
// state under t.TempDir and exercise package-level seams in cmd.
package cmd

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

// silentLogger is a discard *slog.Logger used by migrate-rename tests that
// do not assert on log output.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newMigrateStore is a small helper that returns a *hooks.Store rooted at a
// fresh temp dir. The returned path is the hooks file (not necessarily
// pre-existing).
func newMigrateStore(t *testing.T) (*hooks.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	return hooks.NewStore(path), path
}

// newMigrateLogger returns a capturing *slog.Logger bound to the hooks
// component plus its sink so tests can read the rendered log body back.
func newMigrateLogger(t *testing.T) (*slog.Logger, *logtest.Sink) {
	t.Helper()
	return newCaptureLoggerForComponent(t, "hooks")
}

func TestRunMigrateRename_RewritesSingleMatchingKey(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"old:0.0": {"on-resume": "claude --resume abc"},
	})

	if err := runMigrateRename(store, "old", "new", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	got := readHooksJSON(t, path)
	if _, ok := got["old:0.0"]; ok {
		t.Errorf("old key still present after migrate: %v", got)
	}
	if got["new:0.0"]["on-resume"] != "claude --resume abc" {
		t.Errorf("new key wrong; got %v", got)
	}
}

func TestRunMigrateRename_RewritesMultipleMatchingKeys(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"work:0.0": {"on-resume": "a"},
		"work:0.1": {"on-resume": "b"},
		"work:1.0": {"on-resume": "c"},
	})

	if err := runMigrateRename(store, "work", "play", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	got := readHooksJSON(t, path)
	for _, oldKey := range []string{"work:0.0", "work:0.1", "work:1.0"} {
		if _, ok := got[oldKey]; ok {
			t.Errorf("%q still present after migrate", oldKey)
		}
	}
	want := map[string]string{"play:0.0": "a", "play:0.1": "b", "play:1.0": "c"}
	for k, v := range want {
		if got[k]["on-resume"] != v {
			t.Errorf("got[%q][on-resume] = %q, want %q (full=%v)", k, got[k]["on-resume"], v, got)
		}
	}
}

func TestRunMigrateRename_LeavesUnrelatedKeysUntouched(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"old:0.0":   {"on-resume": "match"},
		"other:0.0": {"on-resume": "untouched"},
	})

	if err := runMigrateRename(store, "old", "new", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	got := readHooksJSON(t, path)
	if got["new:0.0"]["on-resume"] != "match" {
		t.Errorf("expected migrated key new:0.0; got %v", got)
	}
	if got["other:0.0"]["on-resume"] != "untouched" {
		t.Errorf("expected other:0.0 untouched; got %v", got)
	}
}

func TestRunMigrateRename_NoMatchIsNoOp_NoFileWrite(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"other:0.0": {"on-resume": "untouched"},
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat seed file: %v", err)
	}
	beforeMtime := info.ModTime()

	// Sleep so that any rewrite would produce a different mtime.
	time.Sleep(20 * time.Millisecond)

	if err := runMigrateRename(store, "old", "new", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat after migrate: %v", err)
	}
	if !info.ModTime().Equal(beforeMtime) {
		t.Errorf("file was rewritten on no-op (mtime changed: %v -> %v)", beforeMtime, info.ModTime())
	}
}

func TestRunMigrateRename_PrefixAmbiguityViaTrailingColon(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"work:0.0":   {"on-resume": "match-this"},
		"work-2:0.0": {"on-resume": "do-not-match"},
	})

	if err := runMigrateRename(store, "work", "play", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	got := readHooksJSON(t, path)
	if got["play:0.0"]["on-resume"] != "match-this" {
		t.Errorf("expected play:0.0=match-this, got %v", got)
	}
	if got["work-2:0.0"]["on-resume"] != "do-not-match" {
		t.Errorf("expected work-2:0.0 untouched, got %v", got)
	}
	if _, ok := got["work:0.0"]; ok {
		t.Errorf("work:0.0 should have been removed")
	}
}

func TestRunMigrateRename_CollisionLogsAndOverwrites(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"old:0.0": {"on-resume": "from-old"},
		"new:0.0": {"on-resume": "pre-existing"},
	})

	logger, sink := newMigrateLogger(t)
	if err := runMigrateRename(store, "old", "new", logger); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	got := readHooksJSON(t, path)
	if got["new:0.0"]["on-resume"] != "from-old" {
		t.Errorf("collision should overwrite; got %v", got)
	}
	if _, ok := got["old:0.0"]; ok {
		t.Errorf("old:0.0 should have been removed")
	}
	msg := sink.Body()
	if !strings.Contains(msg, "WARN") {
		t.Errorf("expected WARN level on collision; got %q", msg)
	}
	if !strings.Contains(msg, "component=hooks") {
		t.Errorf("expected component %q on collision log; got %q", "hooks", msg)
	}
	// The colliding key now rides the hook_key attr (terse message must not
	// interpolate values).
	if !strings.Contains(msg, "hook_key=new:0.0") {
		t.Errorf("expected colliding key on collision log; got %q", msg)
	}
	if !strings.Contains(msg, "overwriting") {
		t.Errorf("expected 'overwriting' in log; got %q", msg)
	}
}

func TestRunMigrateRename_EmitsInternalSaveBreadcrumb(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"work:0.0": {"on-resume": "a"},
		"work:0.1": {"on-resume": "b"},
		"work:1.0": {"on-resume": "c"},
	})

	// The Save breadcrumb is emitted at the store seam (SaveAudited ->
	// log.For("hooks")), which routes through the process-wide swap handler —
	// NOT through the *slog.Logger injected into runMigrateRename (that one
	// carries only the collision / load diagnostics). Capture via
	// SetTestHandler to observe the store-seam breadcrumb.
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)

	if err := runMigrateRename(store, "work", "play", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	body := sink.Body()
	// Exactly one audit breadcrumb for the persisted rewrite: INFO modify with
	// entries=N (the number of rewritten keys) and via=internal, under hooks.
	if !strings.Contains(body, "INFO modify") {
		t.Errorf("expected INFO modify breadcrumb; got %q", body)
	}
	if !strings.Contains(body, "component=hooks") {
		t.Errorf("expected component=hooks; got %q", body)
	}
	if !strings.Contains(body, "entries=3") {
		t.Errorf("expected entries=3 (3 rewritten keys); got %q", body)
	}
	if !strings.Contains(body, "via=internal") {
		t.Errorf("expected via=internal; got %q", body)
	}
	// Terse message: no value interpolation, single summary line for the batch.
	if strings.Count(body, "INFO modify") != 1 {
		t.Errorf("expected exactly one INFO modify breadcrumb; got %q", body)
	}
}

func TestRunMigrateRename_MalformedJSONIsNoOp(t *testing.T) {
	store, path := newMigrateStore(t)
	if err := os.WriteFile(path, []byte("{not-valid-json"), 0o644); err != nil {
		t.Fatalf("seed malformed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}
	beforeMtime := info.ModTime()
	time.Sleep(20 * time.Millisecond)

	if err := runMigrateRename(store, "old", "new", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename should treat malformed as empty: %v", err)
	}

	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat after migrate: %v", err)
	}
	if !info.ModTime().Equal(beforeMtime) {
		t.Errorf("file rewritten despite no matching keys; mtime %v -> %v", beforeMtime, info.ModTime())
	}
	// Confirm content is still the malformed bytes (no rewrite at all).
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after migrate: %v", err)
	}
	if string(got) != "{not-valid-json" {
		t.Errorf("malformed file was modified; got %q", got)
	}
}

func TestRunMigrateRename_MissingFileIsNoOp(t *testing.T) {
	store, path := newMigrateStore(t)
	// Do not create the file.

	if err := runMigrateRename(store, "old", "new", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename should treat missing as empty: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should not have been created on no-op; stat err=%v", err)
	}
}

func TestRunMigrateRename_RejectsEmptyNewName(t *testing.T) {
	store, _ := newMigrateStore(t)
	err := runMigrateRename(store, "old", "", silentLogger())
	if err == nil {
		t.Fatal("expected error for empty new name, got nil")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error = %q, want it to mention 'non-empty'", err.Error())
	}
}

func TestRunMigrateRename_SaveFailurePropagatesAndWarns(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — chmod 0500 will not block writes")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	writeHooksJSON(t, path, map[string]map[string]string{
		"old:0.0": {"on-resume": "x"},
	})

	store := hooks.NewStore(path)

	// Restrict the parent dir so AtomicWrite's CreateTemp call fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod parent dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	// The save-failure WARN is now emitted at the store seam (SaveAudited),
	// which routes through the process-wide swap handler — capture it via
	// SetTestHandler. It is the terse audit WARN (op=modify) carrying the
	// write-failed-* error_class, not the old hand-rolled "save hooks failed".
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)

	err := runMigrateRename(store, "old", "new", silentLogger())
	if err == nil {
		t.Fatal("expected save failure error, got nil")
	}

	msg := sink.Body()
	if !strings.Contains(msg, "WARN modify") {
		t.Errorf("expected WARN modify breadcrumb on save failure; got %q", msg)
	}
	if !strings.Contains(msg, "component=hooks") {
		t.Errorf("expected component %q on save-failure log; got %q", "hooks", msg)
	}
	if !strings.Contains(msg, "via=internal") {
		t.Errorf("expected via=internal on save-failure log; got %q", msg)
	}
	if !strings.Contains(msg, "error_class=write-failed-") {
		t.Errorf("expected write-failed-* error_class on save-failure log; got %q", msg)
	}
}

func TestRunMigrateRename_PreservesEventMapVerbatim(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"old:0.0": {
			"on-resume": "resume-cmd",
			"on-attach": "attach-cmd",
		},
	})

	if err := runMigrateRename(store, "old", "new", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	got := readHooksJSON(t, path)
	events, ok := got["new:0.0"]
	if !ok {
		t.Fatalf("expected new:0.0 entry; got %v", got)
	}
	if events["on-resume"] != "resume-cmd" {
		t.Errorf("on-resume = %q, want %q", events["on-resume"], "resume-cmd")
	}
	if events["on-attach"] != "attach-cmd" {
		t.Errorf("on-attach = %q, want %q", events["on-attach"], "attach-cmd")
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d: %v", len(events), events)
	}
}

func TestRunMigrateRename_KeyWithColonInRemainder(t *testing.T) {
	// Trailing-colon prefix means oldName="foo" matches "foo:bar:0.0" —
	// the entire "bar:0.0" remainder is preserved.
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"foo:bar:0.0": {"on-resume": "preserved"},
	})

	if err := runMigrateRename(store, "foo", "baz", silentLogger()); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	got := readHooksJSON(t, path)
	if got["baz:bar:0.0"]["on-resume"] != "preserved" {
		t.Errorf("expected baz:bar:0.0=preserved; got %v", got)
	}
	if _, ok := got["foo:bar:0.0"]; ok {
		t.Errorf("foo:bar:0.0 should have been removed")
	}
}

// TestRunMigrateRename_EmitsHooksComponentToLogger asserts the migration
// surface routes its diagnostic warnings through the injected *slog.Logger
// under the canonical hooks component tag rather than fmt.Fprintf(os.Stderr,
// ...). This is the task 12-5 acceptance: routine reporting in
// state_migrate_rename must be visible to portal.log so `portal doctor`
// recent-warnings can surface it. We exercise the collision path (which is the
// deterministic non-fatal warning path) and verify both the level and
// component tag.
func TestRunMigrateRename_EmitsHooksComponentToLogger(t *testing.T) {
	store, path := newMigrateStore(t)
	writeHooksJSON(t, path, map[string]map[string]string{
		"old:0.0": {"on-resume": "from-old"},
		"new:0.0": {"on-resume": "pre-existing"},
	})

	logger, sink := newMigrateLogger(t)
	if err := runMigrateRename(store, "old", "new", logger); err != nil {
		t.Fatalf("runMigrateRename: %v", err)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN level entry: %q", logged)
	}
	if !strings.Contains(logged, "component=hooks") {
		t.Errorf("log missing %q component column: %q", "hooks", logged)
	}
}
