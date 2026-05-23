// White-box tests for the exported diff/format helpers consumed by
// out-of-package integration tests. These pin the contract shared by
// the three downstream tests (kill-barrier escalation, daemon
// self-supervision, composition ABC) so any future change to the
// Fingerprint shape requires a single coordinated edit here.

package portaltest

import (
	"strings"
	"testing"
)

// --- DiffFingerprints contract tests -----------------------------

func TestDiffFingerprints_EmptyMaps_NoDeltas(t *testing.T) {
	got := DiffFingerprints(nil, nil)
	if len(got) != 0 {
		t.Errorf("expected zero deltas, got %d: %#v", len(got), got)
	}
}

func TestDiffFingerprints_Identical_NoDeltas(t *testing.T) {
	fp := Fingerprint{Exists: true, Size: 5, MtimeNanos: 100, CtimeNanos: 200, Hashed: true, Sha256: [32]byte{1, 2, 3}}
	pre := map[string]Fingerprint{"a": fp, "b": fp}
	post := map[string]Fingerprint{"a": fp, "b": fp}

	got := DiffFingerprints(pre, post)
	if len(got) != 0 {
		t.Errorf("expected zero deltas, got %d: %#v", len(got), got)
	}
}

func TestDiffFingerprints_AdditionsOnly(t *testing.T) {
	fp := Fingerprint{Exists: true, Size: 5}
	pre := map[string]Fingerprint{}
	post := map[string]Fingerprint{"new1": fp, "new2": fp}

	got := DiffFingerprints(pre, post)
	if len(got) != 2 {
		t.Fatalf("expected 2 deltas, got %d: %#v", len(got), got)
	}
	for _, d := range got {
		if d.Field != "created" {
			t.Errorf("expected Field=created, got %q for %q", d.Field, d.Path)
		}
		if d.Post != fp {
			t.Errorf("expected Post=fp for %q, got %#v", d.Path, d.Post)
		}
		if d.Pre != (Fingerprint{}) {
			t.Errorf("expected Pre=zero for created delta at %q, got %#v", d.Path, d.Pre)
		}
	}
	// Sort order: by Path.
	if got[0].Path != "new1" || got[1].Path != "new2" {
		t.Errorf("expected sorted paths [new1 new2], got [%s %s]", got[0].Path, got[1].Path)
	}
}

func TestDiffFingerprints_RemovalsOnly(t *testing.T) {
	fp := Fingerprint{Exists: true, Size: 5}
	pre := map[string]Fingerprint{"gone1": fp, "gone2": fp}
	post := map[string]Fingerprint{}

	got := DiffFingerprints(pre, post)
	if len(got) != 2 {
		t.Fatalf("expected 2 deltas, got %d: %#v", len(got), got)
	}
	for _, d := range got {
		if d.Field != "deleted" {
			t.Errorf("expected Field=deleted, got %q for %q", d.Field, d.Path)
		}
		if d.Pre != fp {
			t.Errorf("expected Pre=fp for %q, got %#v", d.Path, d.Pre)
		}
		if d.Post != (Fingerprint{}) {
			t.Errorf("expected Post=zero for deleted delta at %q, got %#v", d.Path, d.Post)
		}
	}
}

func TestDiffFingerprints_SizeMutation(t *testing.T) {
	pre := map[string]Fingerprint{"f": {Exists: true, Size: 5}}
	post := map[string]Fingerprint{"f": {Exists: true, Size: 10}}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "f", "size") {
		t.Errorf("expected size delta, got %#v", got)
	}
}

func TestDiffFingerprints_MtimeMutation(t *testing.T) {
	pre := map[string]Fingerprint{"f": {Exists: true, Size: 5, MtimeNanos: 100}}
	post := map[string]Fingerprint{"f": {Exists: true, Size: 5, MtimeNanos: 200}}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "f", "mtime") {
		t.Errorf("expected mtime delta, got %#v", got)
	}
}

func TestDiffFingerprints_CtimeMutation(t *testing.T) {
	pre := map[string]Fingerprint{"f": {Exists: true, Size: 5, CtimeNanos: 100}}
	post := map[string]Fingerprint{"f": {Exists: true, Size: 5, CtimeNanos: 200}}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "f", "ctime") {
		t.Errorf("expected ctime delta, got %#v", got)
	}
}

func TestDiffFingerprints_ContentMutation(t *testing.T) {
	pre := map[string]Fingerprint{"f": {Exists: true, Size: 5, Hashed: true, Sha256: [32]byte{1}}}
	post := map[string]Fingerprint{"f": {Exists: true, Size: 5, Hashed: true, Sha256: [32]byte{2}}}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "f", "content") {
		t.Errorf("expected content delta, got %#v", got)
	}
}

func TestDiffFingerprints_HashedFlipped_HashedDelta(t *testing.T) {
	// File grew past 1 MiB → Hashed flipped true→false. Diff must
	// report a "hashed" field delta independent of Sha256 equality.
	pre := map[string]Fingerprint{"f": {Exists: true, Size: 100, Hashed: true, Sha256: [32]byte{1}}}
	post := map[string]Fingerprint{"f": {Exists: true, Size: 100, Hashed: false}}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "f", "hashed") {
		t.Errorf("expected hashed delta, got %#v", got)
	}
}

func TestDiffFingerprints_SymlinkTargetMutation(t *testing.T) {
	pre := map[string]Fingerprint{"link": {Exists: true, IsSymlink: true, SymlinkTarget: "/a"}}
	post := map[string]Fingerprint{"link": {Exists: true, IsSymlink: true, SymlinkTarget: "/b"}}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "link", "symlink-target") {
		t.Errorf("expected symlink-target delta, got %#v", got)
	}
}

func TestDiffFingerprints_BecameSymlink(t *testing.T) {
	pre := map[string]Fingerprint{"f": {Exists: true, Size: 5, IsSymlink: false}}
	post := map[string]Fingerprint{"f": {Exists: true, IsSymlink: true, SymlinkTarget: "/a"}}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "f", "became-symlink") {
		t.Errorf("expected became-symlink delta, got %#v", got)
	}
	// On a type swap, the symlink-target and other field deltas are
	// noise; the became-symlink delta should be the sole reported
	// signal so the diagnostic stays focused on the root cause.
	for _, d := range got {
		if d.Path == "f" && d.Field != "became-symlink" {
			t.Errorf("on type swap, expected sole delta became-symlink; got extra delta %q", d.Field)
		}
	}
}

func TestDiffFingerprints_SymlinkBothSides_TargetChanged_NotBecameSymlink(t *testing.T) {
	// Both pre and post are symlinks (IsSymlink=true), only target
	// differs. Field must be "symlink-target", not "became-symlink".
	pre := map[string]Fingerprint{"link": {Exists: true, IsSymlink: true, SymlinkTarget: "/a"}}
	post := map[string]Fingerprint{"link": {Exists: true, IsSymlink: true, SymlinkTarget: "/b"}}

	got := DiffFingerprints(pre, post)
	for _, d := range got {
		if d.Field == "became-symlink" {
			t.Errorf("symlink→symlink with target change must not emit became-symlink; got %#v", got)
		}
	}
	if !containsDelta(got, "link", "symlink-target") {
		t.Errorf("expected symlink-target delta, got %#v", got)
	}
}

func TestDiffFingerprints_MixedDeltas(t *testing.T) {
	pre := map[string]Fingerprint{
		"a": {Exists: true, Size: 5},
		"b": {Exists: true, Size: 10},
		"c": {Exists: true, Size: 20},
	}
	post := map[string]Fingerprint{
		"a": {Exists: true, Size: 5},   // unchanged
		"b": {Exists: true, Size: 100}, // size mutated
		// "c" deleted
		"d": {Exists: true, Size: 30}, // created
	}

	got := DiffFingerprints(pre, post)
	if !containsDelta(got, "b", "size") {
		t.Errorf("missing b/size delta in %#v", got)
	}
	if !containsDelta(got, "c", "deleted") {
		t.Errorf("missing c/deleted delta in %#v", got)
	}
	if !containsDelta(got, "d", "created") {
		t.Errorf("missing d/created delta in %#v", got)
	}
}

func TestDiffFingerprints_StableSortOrder(t *testing.T) {
	// Multiple field-level deltas at one path AND multiple paths —
	// output must be sorted by (Path, Field).
	pre := map[string]Fingerprint{
		"z/path": {Exists: true, Size: 5, MtimeNanos: 100, CtimeNanos: 200},
		"a/path": {Exists: true, Size: 5},
	}
	post := map[string]Fingerprint{
		"z/path": {Exists: true, Size: 10, MtimeNanos: 999, CtimeNanos: 888},
		"a/path": {Exists: true, Size: 7},
	}

	got := DiffFingerprints(pre, post)
	if len(got) < 2 {
		t.Fatalf("expected multiple deltas, got %d: %#v", len(got), got)
	}
	// Verify sort: Path ascending, then Field ascending within a Path.
	for i := 1; i < len(got); i++ {
		prev, cur := got[i-1], got[i]
		if prev.Path > cur.Path {
			t.Errorf("paths out of order: %q before %q (full=%#v)", prev.Path, cur.Path, got)
		} else if prev.Path == cur.Path && prev.Field > cur.Field {
			t.Errorf("fields out of order at %q: %q before %q (full=%#v)",
				prev.Path, prev.Field, cur.Field, got)
		}
	}
	// And: all a/path deltas precede all z/path deltas.
	sawZ := false
	for _, d := range got {
		if d.Path == "z/path" {
			sawZ = true
		} else if d.Path == "a/path" && sawZ {
			t.Errorf("a/path emitted after z/path: %#v", got)
		}
	}
}

// --- FormatDelta contract tests ----------------------------------

func TestFormatDelta_CreatedMentionsPathAndField(t *testing.T) {
	d := FingerprintDelta{Path: "scrollback/x.bin", Field: "created",
		Post: Fingerprint{Exists: true, Size: 5}}
	out := FormatDelta(d)
	if !strings.Contains(out, "scrollback/x.bin") {
		t.Errorf("expected path in output, got %q", out)
	}
	if !strings.Contains(out, "created") {
		t.Errorf("expected field in output, got %q", out)
	}
}

func TestFormatDelta_SingleLine(t *testing.T) {
	d := FingerprintDelta{Path: "f", Field: "size",
		Pre:  Fingerprint{Exists: true, Size: 5},
		Post: Fingerprint{Exists: true, Size: 10}}
	out := FormatDelta(d)
	if strings.Contains(out, "\n") {
		t.Errorf("FormatDelta must produce a single line; got %q", out)
	}
}

// --- FormatFingerprint contract tests ----------------------------

func TestFormatFingerprint_RegularFile(t *testing.T) {
	fp := Fingerprint{Exists: true, Size: 5, MtimeNanos: 100, CtimeNanos: 200, Hashed: true, Sha256: [32]byte{0xab, 0xcd}}
	out := FormatFingerprint(fp)
	// Must mention size and sha256.
	for _, sub := range []string{"5", "sha256"} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in output, got %q", sub, out)
		}
	}
}

func TestFormatFingerprint_Symlink(t *testing.T) {
	fp := Fingerprint{Exists: true, IsSymlink: true, SymlinkTarget: "/x/y"}
	out := FormatFingerprint(fp)
	if !strings.Contains(out, "symlink") {
		t.Errorf("expected symlink marker in output, got %q", out)
	}
	if !strings.Contains(out, "/x/y") {
		t.Errorf("expected target in output, got %q", out)
	}
}

func TestFormatFingerprint_UnhashedFile(t *testing.T) {
	fp := Fingerprint{Exists: true, Size: 1 << 21, Hashed: false}
	out := FormatFingerprint(fp)
	if !strings.Contains(out, "hashed=false") {
		t.Errorf("expected hashed=false marker, got %q", out)
	}
}

// --- helpers -----------------------------------------------------

func containsDelta(deltas []FingerprintDelta, path, field string) bool {
	for _, d := range deltas {
		if d.Path == path && d.Field == field {
			return true
		}
	}
	return false
}
