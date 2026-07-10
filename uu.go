package yenc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// UUDecode decodes a uuencoded block, returning the bytes and the declared file
// name from the "begin <mode> <name>" header.
func UUDecode(data []byte) (out []byte, name string, err error) {
	lines := splitLines(data)

	var i int
	for i = 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "begin ") {
			break
		}
	}
	if i == len(lines) {
		return nil, "", errors.New("uu: missing begin header")
	}

	fields := strings.SplitN(lines[i], " ", 3)
	if len(fields) < 3 {
		return nil, "", errors.New("uu: malformed begin header")
	}
	if _, e := strconv.ParseUint(fields[1], 8, 32); e != nil {
		return nil, "", fmt.Errorf("uu: bad mode: %w", e)
	}
	name = fields[2]
	i++

	for ; i < len(lines); i++ {
		line := lines[i]
		if line == "end" {
			break
		}
		if line == "" {
			continue
		}
		n := int((line[0] - 32) & 63)
		if n == 0 {
			continue
		}
		enc := line[1:]
		need := ((n + 2) / 3) * 4
		if len(enc) < need {
			return nil, "", errors.New("uu: short data line")
		}
		base := len(out)
		for g := 0; g < need; g += 4 {
			c0 := (enc[g] - 32) & 63
			c1 := (enc[g+1] - 32) & 63
			c2 := (enc[g+2] - 32) & 63
			c3 := (enc[g+3] - 32) & 63
			out = append(out, c0<<2|c1>>4, c1<<4|c2>>2, c2<<6|c3)
		}
		out = out[:base+n]
	}
	return out, name, nil
}
