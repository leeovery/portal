package state

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
)

// tailChunkSize is the read stride for the reverse scan in TailScrollback.
// 64 KiB is large enough that even very wide lines (e.g. 2 KiB per line) fit
// comfortably while keeping per-call peak buffer use bounded.
const tailChunkSize = 64 * 1024

// openFileForTail is the seam used by TailScrollback to open the target file.
// Tests swap it via SetOpenFileForTest to assert the single-fd invariant
// (exactly one open per call). Production callers always go through os.Open.
var openFileForTail = os.Open

// SetOpenFileForTest swaps openFileForTail with the supplied function and
// returns a restore func. Test-only seam — the production code path always
// uses os.Open directly. Keeping the seam package-private at the symbol level
// (an unexported var) and the swap helper exported preserves the invariant
// that production callers cannot redirect file opens.
func SetOpenFileForTest(open func(name string) (*os.File, error)) (restore func()) {
	prev := openFileForTail
	openFileForTail = open
	return func() { openFileForTail = prev }
}

// TailScrollback returns the bytes of the last n newline-terminated lines
// from the .bin scrollback file at path. The returned slice always ends on
// a '\n' byte and contains complete records only — any trailing bytes after
// the final '\n' (a partial/in-progress record) are excluded.
//
// The implementation opens the file once, seeks to end, and reads backwards
// in fixed-size chunks against a single held file descriptor — never closing
// and reopening between reads. Cost is decoupled from total file size.
//
// If the file holds at least one but fewer than n terminated lines, the
// function returns every available terminated line (no padding, no error).
//
// All "no content available" outcomes converge on (nil, nil) with NO error:
// ENOENT on open, a zero-byte file, and a file whose reverse scan finds zero
// '\n' bytes (e.g. only an unterminated partial line).
//
// Any other open error (e.g. EACCES) and any Seek/Read error encountered
// during the reverse scan propagate as (nil, err) wrapped with the unified
// prefix "tail scrollback <path>: ..." and %w, so errors.Is works against
// fs.ErrPermission, os.ErrClosed, etc. There are no retries — a single
// attempt per call. The deferred Close runs on every return path, including
// errors, so the file descriptor is never leaked.
func TailScrollback(path string, n int) ([]byte, error) {
	f, err := openFileForTail(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("tail scrollback %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("tail scrollback %s: %w", path, err)
	}
	if size == 0 {
		return nil, nil
	}

	// Reverse-scan invariant: `cursor` is the absolute file offset of the
	// next byte we have NOT yet read; `tail` holds bytes already read,
	// concatenated in file order (oldest first). Each iteration reads the
	// chunk at [cursor-stride, cursor) and prepends it to `tail`.
	cursor := size
	var tail []byte
	// We need (n+1) newlines to pinpoint the start of the n-th-from-last
	// line: n delimiters between records plus the one immediately preceding
	// the slice we want to return. If the file has fewer than (n+1) newlines
	// total, we exhaust the file and return everything from byte 0.
	target := n + 1
	chunk := make([]byte, tailChunkSize)
	for cursor > 0 {
		stride := min(int64(tailChunkSize), cursor)
		readAt := cursor - stride
		if _, err := f.Seek(readAt, io.SeekStart); err != nil {
			return nil, fmt.Errorf("tail scrollback %s: %w", path, err)
		}
		buf := chunk[:stride]
		if _, err := io.ReadFull(f, buf); err != nil {
			return nil, fmt.Errorf("tail scrollback %s: %w", path, err)
		}
		// Prepend buf to tail (file-order). Allocate once per iteration.
		merged := make([]byte, len(buf)+len(tail))
		copy(merged, buf)
		copy(merged[len(buf):], tail)
		tail = merged
		cursor = readAt

		// Count newlines in `tail`; if we have ≥ target, locate the cut.
		if bytes.Count(tail, []byte{'\n'}) >= target {
			cut := indexOfNthNewlineFromEnd(tail, target)
			// cut is the index of the (target)th-from-last \n. The returned
			// slice starts at the byte after it so we keep n full lines.
			// Slice end is the last \n (inclusive) so any trailing partial
			// bytes after the final \n are excluded.
			last := bytes.LastIndexByte(tail, '\n')
			return tail[cut+1 : last+1], nil
		}
	}

	// Reverse scan exhausted the file. If it has zero newlines, the whole
	// file is a partial/in-progress record — collapse to "no content".
	last := bytes.LastIndexByte(tail, '\n')
	if last < 0 {
		return nil, nil
	}
	// Fewer than (n+1) newlines in the entire file. Return every fully
	// terminated record — bytes from start through the final \n inclusive,
	// excluding any trailing partial bytes.
	return tail[:last+1], nil
}

// indexOfNthNewlineFromEnd returns the byte index of the n-th '\n' counting
// backwards from the end of buf (1 = last newline, 2 = second-to-last, …).
// Caller must guarantee bytes.Count(buf, '\n') >= n.
func indexOfNthNewlineFromEnd(buf []byte, n int) int {
	seen := 0
	for i, v := range slices.Backward(buf) {
		if v != '\n' {
			continue
		}
		seen++
		if seen == n {
			return i
		}
	}
	return -1 // unreachable given precondition
}
