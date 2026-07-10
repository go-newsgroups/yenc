// Package yenc implements dependency-free yEnc and uuencode decoders and
// encoders for Usenet binary posts. It uses only the Go standard library and
// builds with CGO_ENABLED=0.
package yenc

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
)

// Part is one decoded yEnc part (single-part posts have Part==0).
type Part struct {
	Name  string // =ybegin name=
	Size  int64  // =ybegin size=
	Line  int    // =ybegin line=
	Part  int    // =ypart / =ybegin part= (0 for single-part)
	Total int    // =ybegin total= (0 if absent)
	Begin int64  // =ypart begin= (1-based; 0 for single-part)
	End   int64  // =ypart end=
	CRC32 uint32 // =yend pcrc32 (part) or crc32 (single); 0 if absent
	Data  []byte // decoded bytes
}

// ErrCRCMismatch is returned by Decode when the trailer CRC does not match the
// decoded data.
var ErrCRCMismatch = errors.New("yenc: CRC32 mismatch")

// splitLines splits data on LF and strips a trailing CR from each line.
func splitLines(data []byte) []string {
	raw := strings.Split(string(data), "\n")
	for i, l := range raw {
		raw[i] = strings.TrimSuffix(l, "\r")
	}
	return raw
}

// parseKV parses space-separated "key=value" tokens into a map. Tokens without
// a '=' are ignored.
func parseKV(s string) map[string]string {
	m := make(map[string]string)
	for _, f := range strings.Fields(s) {
		if i := strings.IndexByte(f, '='); i >= 0 {
			m[f[:i]] = f[i+1:]
		}
	}
	return m
}

// decodeBody decodes the raw (newline-stripped) yEnc body bytes.
func decodeBody(body []byte) ([]byte, error) {
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '=' {
			i++
			if i >= len(body) {
				return nil, errors.New("yenc: truncated escape sequence")
			}
			out = append(out, body[i]-64-42)
			continue
		}
		out = append(out, c-42)
	}
	return out, nil
}

// Decode decodes one yEnc-encoded article body (the region from =ybegin to
// =yend). It verifies the CRC when the trailer provides one, returning
// ErrCRCMismatch on a mismatch (Part is still returned so callers may inspect
// it).
func Decode(data []byte) (*Part, error) {
	lines := splitLines(data)

	var i int
	for i = 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "=ybegin") {
			break
		}
	}
	if i == len(lines) {
		return nil, errors.New("yenc: missing =ybegin header")
	}

	p := &Part{}

	// Header: name= runs to the end of the line, so split it off first.
	hdr := lines[i]
	if idx := strings.Index(hdr, "name="); idx >= 0 {
		p.Name = hdr[idx+len("name="):]
		hdr = hdr[:idx]
	}
	m := parseKV(strings.TrimPrefix(hdr, "=ybegin"))

	var perr error
	var partN, totalN, lineN, sizeN, beginN, endN int64
	setInt := func(mm map[string]string, key string, dst *int64) {
		if perr != nil {
			return
		}
		if v, ok := mm[key]; ok {
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil {
				perr = fmt.Errorf("yenc: bad %s field: %w", key, e)
				return
			}
			*dst = n
		}
	}

	setInt(m, "part", &partN)
	setInt(m, "total", &totalN)
	setInt(m, "line", &lineN)
	setInt(m, "size", &sizeN)
	i++

	// Optional =ypart line for multipart posts.
	if i < len(lines) && strings.HasPrefix(lines[i], "=ypart") {
		mp := parseKV(strings.TrimPrefix(lines[i], "=ypart"))
		setInt(mp, "begin", &beginN)
		setInt(mp, "end", &endN)
		i++
	}
	if perr != nil {
		return nil, perr
	}

	p.Part = int(partN)
	p.Total = int(totalN)
	p.Line = int(lineN)
	p.Size = sizeN
	p.Begin = beginN
	p.End = endN

	// Body up to the =yend trailer.
	var body []byte
	var trailer string
	found := false
	for ; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "=yend") {
			trailer = lines[i]
			found = true
			i++
			break
		}
		body = append(body, lines[i]...)
	}
	if !found {
		return nil, errors.New("yenc: missing =yend trailer")
	}

	out, err := decodeBody(body)
	if err != nil {
		return nil, err
	}
	p.Data = out

	// CRC: pcrc32 for multipart, crc32 for single-part.
	mt := parseKV(strings.TrimPrefix(trailer, "=yend"))
	key := "crc32"
	if p.Part > 0 {
		key = "pcrc32"
	}
	if v, ok := mt[key]; ok {
		n, e := strconv.ParseUint(v, 16, 32)
		if e != nil {
			return nil, fmt.Errorf("yenc: bad %s field: %w", key, e)
		}
		p.CRC32 = uint32(n)
		if crc32.ChecksumIEEE(out) != p.CRC32 {
			return p, ErrCRCMismatch
		}
	}
	return p, nil
}

// Encode yEnc-encodes data into a single-part article body using lineLen bytes
// per line (typical 128), with =ybegin/=yend and a crc32 trailer, for the given
// file name.
func Encode(name string, data []byte, lineLen int) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "=ybegin line=%d size=%d name=%s\r\n", lineLen, len(data), name)

	col := 0
	emit := func(c byte) {
		b.WriteByte(c)
		col++
		if col >= lineLen {
			b.WriteString("\r\n")
			col = 0
		}
	}
	for _, d := range data {
		e := d + 42
		if e == 0x00 || e == 0x0a || e == 0x0d || e == 0x3d {
			emit('=')
			emit(e + 64)
			continue
		}
		emit(e)
	}
	if col > 0 {
		b.WriteString("\r\n")
	}

	fmt.Fprintf(&b, "=yend size=%d crc32=%08x\r\n", len(data), crc32.ChecksumIEEE(data))
	return b.Bytes()
}
