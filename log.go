package headroom

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
)

// ErrNotLogFormat is returned by LogCompressor.Compress when the input
// does not match a log-line signature.
var ErrNotLogFormat = errors.New("headroom: not log format")

// logLineRe matches a per-line log signature: an ISO8601-ish timestamp
// (or bracketed timestamp), a severity token, and message text. The
// regex is deliberately permissive so it matches the common formats
// (RFC3339, klog, docker JSON's "time=" form is covered by the leading
// timestamp alternative).
var logLineRe = regexp.MustCompile(`^\s*(?:\[?\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?\]?)\s+(?i:(?:TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERROR|ERR|FATAL|CRIT|CRITICAL|PANIC))[\s:]`)

// LogCompressor collapses consecutive identical log lines and truncates
// each line to MaxLineLen bytes. The zero value is NOT valid; use
// NewLogCompressor.
type LogCompressor struct {
	// MaxLineLen is the per-line byte cap. Default 1024.
	MaxLineLen int
}

// NewLogCompressor returns a LogCompressor with the default 1024-byte
// per-line cap.
func NewLogCompressor() *LogCompressor {
	return &LogCompressor{MaxLineLen: 1024}
}

// ContentTypes returns [ContentTypeLog].
func (c *LogCompressor) ContentTypes() []ContentType {
	return []ContentType{ContentTypeLog}
}

// Compress collapses consecutive identical log lines into
// "<line> [repeated N times]" and truncates each output line to
// MaxLineLen bytes. Non-log input (no line matches the signature)
// returns ErrNotLogFormat.
func (c *LogCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	max := c.MaxLineLen
	if max <= 0 {
		max = 1024
	}
	lines := bytes.Split(in, []byte("\n"))
	if len(lines) == 0 {
		return nil, ErrNotLogFormat
	}
	// Require at least one line to match the log signature; this is
	// what distinguishes a log from prose.
	matched := false
	for _, line := range lines {
		if logLineRe.Match(line) {
			matched = true
			break
		}
	}
	if !matched {
		return nil, ErrNotLogFormat
	}
	var out bytes.Buffer
	var prev []byte
	repeat := 0
	flush := func() {
		if prev == nil {
			return
		}
		line := append([]byte(nil), prev...)
		if len(line) > max {
			line = append(line[:max], []byte("...")...)
		}
		out.Write(line)
		if repeat > 0 {
			out.WriteString(" [repeated ")
			// Avoid fmt for allocation discipline.
			out.WriteString(itoa(repeat))
			out.WriteString(" times]")
		}
		out.WriteByte('\n')
		prev = nil
		repeat = 0
	}
	for _, line := range lines {
		if bytes.Equal(line, prev) {
			repeat++
			continue
		}
		flush()
		prev = append([]byte(nil), line...)
	}
	flush()
	// Drop the trailing newline added by flush; bytes.Split's final
	// element is the empty string after the last \n, which the loop
	// above treats as a distinct empty line. Strip exactly one '\n'.
	res := out.Bytes()
	if n := len(res); n > 0 && res[n-1] == '\n' {
		res = res[:n-1]
	}
	return res, nil
}

// itoa is a tiny, allocation-friendly int -> string converter for the
// non-negative counts the compressor emits.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// collapseBlankLines reduces runs of `target` or more consecutive blank
// lines (lines containing only whitespace) to exactly `target` blank
// lines. Used by GoASTCompressor and TextCompressor.
func collapseBlankLines(in []byte, target int) []byte {
	lines := strings.Split(string(in), "\n")
	var out []string
	blanks := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blanks++
			if blanks <= target {
				out = append(out, line)
			}
			continue
		}
		blanks = 0
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}
