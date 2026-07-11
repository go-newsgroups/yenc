<p align="center"><img src="https://raw.githubusercontent.com/go-newsgroups/brand/main/social/go-newsgroups.png" alt="go-newsgroups/yenc" width="720"></p>

# yenc

[![CI](https://github.com/go-newsgroups/yenc/actions/workflows/ci.yml/badge.svg)](https://github.com/go-newsgroups/yenc/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-newsgroups/yenc.svg)](https://pkg.go.dev/github.com/go-newsgroups/yenc)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)

Pure-Go, dependency-free **yEnc** and **uuencode** decoders and encoders for
Usenet binary posts. Builds with `CGO_ENABLED=0` using only the Go standard
library, and is tested to **100% statement coverage** across all supported
architectures.

## Install

```sh
go get github.com/go-newsgroups/yenc
```

## Decode example

```go
package main

import (
	"fmt"

	"github.com/go-newsgroups/yenc"
)

func main() {
	// A yEnc article body runs from =ybegin to =yend. Here we build one with
	// Encode, then decode it back.
	article := yenc.Encode("hello.txt", []byte("hello world"), 128)

	part, err := yenc.Decode(article)
	if err != nil {
		panic(err) // e.g. yenc.ErrCRCMismatch on a corrupt post
	}
	fmt.Printf("name=%q size=%d crc=%08x\n", part.Name, part.Size, part.CRC32)
	fmt.Printf("data=%q\n", part.Data) // "hello world"
}
```

## Encode example

```go
body := yenc.Encode("payload.bin", data, 128) // 128 bytes per line
```

`Encode` produces a single-part article body (`=ybegin` / `=yend`) with a
`crc32` trailer. `Decode` accepts both single-part and multipart bodies,
verifies the CRC when the trailer supplies one (returning `ErrCRCMismatch` on a
mismatch), and populates a `Part`:

```go
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
```

## uudecode

```go
out, name, err := yenc.UUDecode(block) // parses "begin <mode> <name>" ... "end"
```

## License

BSD-3-Clause. See [LICENSE](LICENSE).
