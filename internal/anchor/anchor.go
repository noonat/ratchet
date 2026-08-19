// Package anchor addresses a line of a file in a way a model can transcribe and
// a tool can verify.
//
// Two things have to agree: the renderer that shows a model a file, and the
// resolver that interprets an address afterwards. They live together here so they
// cannot drift.
//
// A read is stamped with one tag for the whole file, and its lines carry bare
// numbers. The alternative, a hash on every line, was measured and rejected: it
// cost the weaker two of four models 15 and 19 correct answers out of 30, where a
// file tag cost nothing at all. Its one advantage, catching a slipped line number
// when the model copies the right line's hash, fired once in 120 attempts against
// 41 refusals a file tag never causes.
package anchor

import (
	"fmt"
	"strings"
)

// Tag is the four-hex fingerprint carried by a rendered read.
//
// It is the low 16 bits of xxHash32 over the normalized text. Sixteen bits is
// enough because the tag proves which content was served, not which of many
// candidates it is: the resolver recomputes and compares, so a collision has to
// happen between two versions of one file in one session.
func Tag(text string) string {
	return fmt.Sprintf("%04X", xxHash32(Normalize(text), 0)&0xFFFF)
}

// Normalize strips trailing blanks from each line and converts CRLF to LF, so a
// file whose copy has been display-trimmed or checked out with Windows line
// endings still produces the same tag. Without this, an editor that trims on save
// would invalidate every anchor it had issued.
func Normalize(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.Join(lines, "\n")
}

// The XXH32 prime constants, named as the reference implementation names them.
const (
	p1 uint32 = 2654435761
	p2 uint32 = 2246822519
	p3 uint32 = 3266489917
	p4 uint32 = 668265263
	p5 uint32 = 374761393
)

func rotl(x uint32, r uint) uint32 {
	return x<<r | x>>(32-r)
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// xxHash32 is XXH32, vendored rather than imported.
//
// It is 40 lines, it must agree byte for byte with the reference implementation
// forever, and the tag it produces is written into files that outlive any
// dependency decision. anchor_test.go pins it against the reference library's own
// vectors for that reason.
func xxHash32(s string, seed uint32) uint32 {
	b := []byte(s)
	n := len(b)
	var h uint32
	i := 0
	if n >= 16 {
		v1, v2, v3, v4 := seed+p1+p2, seed+p2, seed, seed-p1
		for ; i <= n-16; i += 16 {
			v1 = rotl(v1+le32(b[i:])*p2, 13) * p1
			v2 = rotl(v2+le32(b[i+4:])*p2, 13) * p1
			v3 = rotl(v3+le32(b[i+8:])*p2, 13) * p1
			v4 = rotl(v4+le32(b[i+12:])*p2, 13) * p1
		}
		h = rotl(v1, 1) + rotl(v2, 7) + rotl(v3, 12) + rotl(v4, 18)
	} else {
		h = seed + p5
	}
	h += uint32(n)
	for ; i <= n-4; i += 4 {
		h = rotl(h+le32(b[i:])*p3, 17) * p4
	}
	for ; i < n; i++ {
		h = rotl(h+uint32(b[i])*p5, 11) * p1
	}
	h ^= h >> 15
	h *= p2
	h ^= h >> 13
	h *= p3
	h ^= h >> 16
	return h
}
