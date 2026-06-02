package log

import (
	"log/slog"
	"testing"
)

func TestOrDiscard_NilReturnsNonNilDiscardingLogger(t *testing.T) {
	got := OrDiscard(nil)
	if got == nil {
		t.Fatal("OrDiscard(nil) returned nil; expected the shared discard logger")
	}
	// The returned logger must accept records at every level without panicking,
	// and its handler must discard rather than emit anywhere observable.
	got.Debug("probe")
	got.Info("probe")
	got.Warn("probe")
	got.Error("probe")
}

func TestOrDiscard_NonNilReturnedUnchanged(t *testing.T) {
	l := slog.New(slog.NewTextHandler(discardWriterForTest{}, nil))
	got := OrDiscard(l)
	if got != l {
		t.Fatalf("OrDiscard(l) = %p; want the same logger %p", got, l)
	}
}

func TestOrDiscard_NilReturnsSharedInstance(t *testing.T) {
	first := OrDiscard(nil)
	second := OrDiscard(nil)
	if first != second {
		t.Fatal("OrDiscard(nil) returned distinct loggers; expected one shared package-level discard logger")
	}
}

func TestDiscard_ReturnsNonNilDiscardingLogger(t *testing.T) {
	got := Discard()
	if got == nil {
		t.Fatal("Discard() returned nil")
	}
	got.Info("probe")
}

func TestDiscard_MatchesOrDiscardNil(t *testing.T) {
	if Discard() != OrDiscard(nil) {
		t.Fatal("Discard() and OrDiscard(nil) returned different loggers; expected the same shared instance")
	}
}

// discardWriterForTest is an io.Writer that drops everything; used to build a
// distinct non-nil logger whose identity OrDiscard must preserve.
type discardWriterForTest struct{}

func (discardWriterForTest) Write(p []byte) (int, error) { return len(p), nil }
