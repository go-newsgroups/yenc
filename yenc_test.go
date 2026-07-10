package yenc

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"testing"
)

// uuencode is a small test-only encoder used to feed UUDecode.
func uuencode(name string, data []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "begin 644 %s\n", name)
	for i := 0; i < len(data); i += 45 {
		end := i + 45
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		b.WriteByte(byte(len(chunk)) + 32)
		for j := 0; j < len(chunk); j += 3 {
			var g [3]byte
			copy(g[:], chunk[j:])
			c := [4]byte{
				g[0] >> 2,
				(g[0]<<4 | g[1]>>4) & 63,
				(g[1]<<2 | g[2]>>6) & 63,
				g[2] & 63,
			}
			for _, v := range c {
				if v == 0 {
					b.WriteByte('`')
				} else {
					b.WriteByte(v + 32)
				}
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("`\nend\n")
	return b.Bytes()
}

func sampleData() []byte {
	// Include every byte value so both encode escape branches are exercised.
	d := make([]byte, 512)
	for i := range d {
		d[i] = byte(i)
	}
	return d
}

func TestRoundTrip(t *testing.T) {
	for _, size := range []int{0, 1, 127, 128, 256, 512} {
		data := sampleData()[:size]
		enc := Encode("payload.bin", data, 128)
		p, err := Decode(enc)
		if err != nil {
			t.Fatalf("size=%d: decode: %v", size, err)
		}
		if p.Name != "payload.bin" {
			t.Fatalf("size=%d: name %q", size, p.Name)
		}
		if p.Size != int64(size) {
			t.Fatalf("size=%d: Size=%d", size, p.Size)
		}
		if p.Line != 128 {
			t.Fatalf("size=%d: Line=%d", size, p.Line)
		}
		if p.Part != 0 {
			t.Fatalf("size=%d: Part=%d", size, p.Part)
		}
		if !bytes.Equal(p.Data, data) {
			t.Fatalf("size=%d: data mismatch", size)
		}
		if p.CRC32 != crc32.ChecksumIEEE(data) {
			t.Fatalf("size=%d: CRC32=%08x", size, p.CRC32)
		}
	}
}

func TestDecodeMultipart(t *testing.T) {
	data := []byte("Hello, multipart yEnc!")
	crc := crc32.ChecksumIEEE(data)
	var body bytes.Buffer
	for _, d := range data {
		e := d + 42
		if e == 0x00 || e == 0x0a || e == 0x0d || e == 0x3d {
			body.WriteByte('=')
			body.WriteByte(e + 64)
			continue
		}
		body.WriteByte(e)
	}
	art := fmt.Sprintf("=ybegin part=1 total=3 line=128 size=%d name=multi.dat\r\n"+
		"=ypart begin=1 end=%d\r\n"+
		"%s\r\n"+
		"=yend size=%d part=1 pcrc32=%08x\r\n",
		len(data), len(data), body.String(), len(data), crc)

	p, err := Decode([]byte(art))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Part != 1 || p.Total != 3 || p.Begin != 1 || p.End != int64(len(data)) {
		t.Fatalf("part fields: %+v", p)
	}
	if p.Name != "multi.dat" {
		t.Fatalf("name %q", p.Name)
	}
	if !bytes.Equal(p.Data, data) {
		t.Fatalf("data %q", p.Data)
	}
	if p.CRC32 != crc {
		t.Fatalf("crc %08x want %08x", p.CRC32, crc)
	}
}

func TestDecodeNoName(t *testing.T) {
	// Header without name= and a trailer without any CRC.
	art := "=ybegin line=128 size=1\r\n" + string([]byte{'A' + 42}) + "\r\n=yend size=1\r\n"
	p, err := Decode([]byte(art))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Name != "" {
		t.Fatalf("name %q", p.Name)
	}
	if p.CRC32 != 0 {
		t.Fatalf("CRC32 %08x", p.CRC32)
	}
	if !bytes.Equal(p.Data, []byte("A")) {
		t.Fatalf("data %q", p.Data)
	}
}

func TestDecodeCRCMismatch(t *testing.T) {
	art := "=ybegin line=128 size=1 name=x\r\n" + string([]byte{'A' + 42}) + "\r\n=yend size=1 crc32=deadbeef\r\n"
	p, err := Decode([]byte(art))
	if !errors.Is(err, ErrCRCMismatch) {
		t.Fatalf("err=%v", err)
	}
	if p == nil || !bytes.Equal(p.Data, []byte("A")) {
		t.Fatalf("part not returned on mismatch: %+v", p)
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"missing ybegin", "no header here\r\n"},
		{"bad int field", "=ybegin line=xx size=1 name=x\r\nA\r\n=yend size=1\r\n"},
		{"bad ypart int", "=ybegin part=1 line=1 size=1 name=x\r\n=ypart begin=zz end=1\r\nA\r\n=yend size=1 pcrc32=0\r\n"},
		{"missing yend", "=ybegin line=1 size=1 name=x\r\nAAA\r\n"},
		{"truncated escape", "=ybegin line=10 size=1 name=x\r\n=\r\n=yend size=1\r\n"},
		{"bad crc hex", "=ybegin line=1 size=1 name=x\r\n" + string([]byte{'A' + 42}) + "\r\n=yend size=1 crc32=zz\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Decode([]byte(c.in)); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestUUDecodeRoundTrip(t *testing.T) {
	data := sampleData()[:200]
	enc := uuencode("file name.bin", data)
	out, name, err := UUDecode(enc)
	if err != nil {
		t.Fatalf("uudecode: %v", err)
	}
	if name != "file name.bin" {
		t.Fatalf("name %q", name)
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("data mismatch: got %d bytes", len(out))
	}
}

func TestUUDecodeErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"missing begin", "no uu here\n"},
		{"malformed header", "begin 644\n`\nend\n"},
		{"bad mode", "begin xyz name\n`\nend\n"},
		{"short line", "begin 644 f\nM01\n`\nend\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := UUDecode([]byte(c.in)); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestUUDecodeBlankAndZeroLines(t *testing.T) {
	// A blank line and an explicit zero-count line before end must be tolerated.
	in := "begin 644 z.bin\n\n`\nend\n"
	out, name, err := UUDecode([]byte(in))
	if err != nil {
		t.Fatalf("uudecode: %v", err)
	}
	if name != "z.bin" || len(out) != 0 {
		t.Fatalf("name=%q out=%d", name, len(out))
	}
}
