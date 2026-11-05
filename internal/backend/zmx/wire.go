package zmx

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

// ProtocolVersion is the version of zmx's socket protocol the driver speaks,
// frozen on zmx main on 16 Oct 2026. Support is judged by it, never by zmx's
// version string.
const ProtocolVersion = 1

// tag is a message's type, zmx's ipc.Tag. The values are frozen.
type tag uint8

const (
	tagDetachAll   tag = 4
	tagKill        tag = 5
	tagInfo        tag = 6
	tagHistory     tag = 8
	tagEnvGet      tag = 19
	tagEnvData     tag = 21
	tagCapture     tag = 22
	tagUnsupported tag = 255
)

// zmx's frozen record sizes and limits (ipc.zig). Every record is an extern
// struct written in the daemon's native byte order, which is this machine's.
const (
	headerSize  = 8
	infoSize    = 552
	captureSize = 8
	maxCmd      = 256
	maxCwd      = 256
	// maxPayload is zmx's own framing limit; a longer frame is corrupt.
	maxPayload = 64 << 20
)

// historyVT is zmx's HistoryFormat for the screen as terminal escape
// sequences.
const historyVT = 1

var (
	errProtocolVersion = errors.New("unsupported zmx protocol version")
	errInfoSize        = errors.New("zmx info record has the wrong size")
	errFrame           = errors.New("corrupt zmx frame")
)

// header is a message's ipc.Header: tag u8, protocol_version u8, a reserved
// u16, len u32.
type header struct {
	tag     tag
	version uint8
	len     uint32
}

func encodeHeader(t tag, payloadLen int) []byte {
	b := make([]byte, headerSize)
	b[0] = byte(t)
	b[1] = ProtocolVersion
	binary.NativeEndian.PutUint32(b[4:], uint32(payloadLen))
	return b
}

// decodeHeader refuses a header of another protocol version, whose layout,
// length included, cannot be trusted.
func decodeHeader(b []byte) (header, error) {
	h := header{tag: tag(b[0]), version: b[1], len: binary.NativeEndian.Uint32(b[4:])}
	if h.version != ProtocolVersion {
		return h, fmt.Errorf("%w %d in a frame header", errProtocolVersion, h.version)
	}
	if h.len > maxPayload {
		return h, &oversizedFrame{tag: h.tag, len: h.len}
	}
	return h, nil
}

// oversizedFrame is a frame whose header announces a payload over maxPayload,
// which zmx refuses to read but writes all the same: its answer to History
// is the whole scrollback, serialised with no cap in bytes.
type oversizedFrame struct {
	tag tag
	len uint32
}

func (e *oversizedFrame) Error() string {
	return fmt.Sprintf("%v: a tag %d frame of %d bytes, over the %d zmx frames hold", errFrame, e.tag, e.len, maxPayload)
}

func (e *oversizedFrame) Unwrap() error { return errFrame }

// encodeCapture is an ipc.Capture request: format u8, three reserved bytes,
// rows u32.
func encodeCapture(format uint8, rows uint32) []byte {
	b := make([]byte, captureSize)
	b[0] = format
	binary.NativeEndian.PutUint32(b[4:], rows)
	return b
}

// info is the part of an ipc.Info record Portal reads.
type info struct {
	clients uint64
	pid     int32
	cmd     string
	cwd     string
	created uint64
}

// ipc.Info's field offsets. task_ended_at u64 and task_exit_code u8 sit
// between created_at and protocol_version, unread.
const (
	offClients = 0
	offPID     = 8
	offCmdLen  = 12
	offCwdLen  = 14
	offCmd     = 16
	offCwd     = offCmd + maxCmd
	offCreated = offCwd + maxCwd
	offVersion = offCreated + 8 + 8 + 1
)

// decodeInfo reads an ipc.Info record exactly. zmx refuses a record of any
// other size, and so does Portal. The lengths come off the wire, so they are
// clamped to their arrays as zmx clamps them.
func decodeInfo(p []byte) (info, error) {
	if len(p) != infoSize {
		return info{}, fmt.Errorf("%w: %d bytes, want %d", errInfoSize, len(p), infoSize)
	}
	if v := p[offVersion]; v != ProtocolVersion {
		return info{}, fmt.Errorf("%w %d in an info record", errProtocolVersion, v)
	}
	cmdLen := min(int(binary.NativeEndian.Uint16(p[offCmdLen:])), maxCmd)
	cwdLen := min(int(binary.NativeEndian.Uint16(p[offCwdLen:])), maxCwd)
	return info{
		clients: binary.NativeEndian.Uint64(p[offClients:]),
		pid:     int32(binary.NativeEndian.Uint32(p[offPID:])),
		cmd:     string(p[offCmd : offCmd+cmdLen]),
		cwd:     string(p[offCwd : offCwd+cwdLen]),
		created: binary.NativeEndian.Uint64(p[offCreated:]),
	}, nil
}

// createdAt is zero for a record that carries no creation time.
func (i info) createdAt() time.Time {
	if i.created == 0 {
		return time.Time{}
	}
	return time.Unix(int64(min(i.created, math.MaxInt64)), 0)
}

// cwdTruncated reports whether the cwd filled its array, so may have been cut.
func (i info) cwdTruncated() bool { return len(i.cwd) == maxCwd }

// decodeEnv reads an EnvData payload: a line per tracked variable, NAME=value
// for one the client had set and -NAME for one it lacked. An empty payload is
// a session no client leads. A line of neither form is skipped: zmx writes
// values verbatim, so one holding a newline splits across lines.
func decodeEnv(p []byte) backend.ClientEnv {
	var env backend.ClientEnv
	for line := range strings.SplitSeq(string(p), "\n") {
		name, value, set := strings.Cut(line, "=")
		switch {
		case set && name != "":
			env = append(env, backend.EnvVar{Name: name, Value: value})
		case !set && len(line) > 1 && line[0] == '-':
			env = append(env, backend.EnvVar{Name: line[1:], Unset: true})
		}
	}
	return env
}
