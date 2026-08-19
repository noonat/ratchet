package anchor

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// TestTagVectors pins Tag against fixed values from the reference xxhash library.
//
// Re-verified 2026-08-19 against python-xxhash: xxh32(s, seed=0) & 0xFFFF gives
// 5D05, 7456, 53FF and A4DE for these four inputs. Checking mattered, because the
// vectors arrived from another implementation in this project, and a test pinning
// one port against another passes on a shared mistake.
func TestTagVectors(t *testing.T) {
	vectors := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "5D05",
		},
		{
			name: "one byte",
			in:   "a",
			want: "7456",
		},
		{
			name: "three bytes",
			in:   "abc",
			want: "53FF",
		},
		{
			name: "over sixteen bytes, which takes the other branch",
			in:   "The quick brown fox jumps over the lazy dog",
			want: "A4DE",
		},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(Tag(v.in)).To(Equal(v.want), "Tag(%q)", v.in)
		})
	}
}

// TestTagSurvivesWhitespaceAndLineEndings is why Normalize exists. An editor that
// trims trailing blanks on save, or a checkout with CRLF endings, would otherwise
// invalidate every anchor already issued for a file nobody meaningfully changed.
func TestTagSurvivesWhitespaceAndLineEndings(t *testing.T) {
	base := "func main() {\n\tprintln(1)\n}\n"
	cases := []struct {
		name    string
		base    string
		variant string
	}{
		{
			name:    "trailing spaces",
			base:    base,
			variant: "func main() {   \n\tprintln(1)\t\n}\n",
		},
		{
			name:    "CRLF endings",
			base:    base,
			variant: "func main() {\r\n\tprintln(1)\r\n}\r\n",
		},
		{
			name:    "CRLF endings and trailing blanks",
			base:    base,
			variant: "func main() {  \r\n\tprintln(1) \r\n}\r\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(Tag(c.variant)).To(Equal(Tag(c.base)), "same content, different tag")
		})
	}
}

// TestTagDistinguishesRealChanges is the other half: normalising must not make the
// tag blind to a change that matters. Leading whitespace is content, because in
// Python it is syntax.
func TestTagDistinguishesRealChanges(t *testing.T) {
	base := "if x:\n    return 1\n"
	cases := []struct {
		name    string
		base    string
		variant string
	}{
		{
			name:    "body dedented out of the block",
			base:    base,
			variant: "if x:\nreturn 1\n",
		},
		{
			name:    "indentation changed",
			base:    base,
			variant: "if x:\n\t\treturn 1\n",
		},
		{
			name:    "text changed",
			base:    base,
			variant: "if x:\n    return 2\n",
		},
		{
			name:    "line added",
			base:    base,
			variant: "if x:\n    return 1\n    # note\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(Tag(c.variant)).NotTo(Equal(Tag(c.base)), "content differs, tag does not")
		})
	}
}

// TestTagOnAdversarialShapes covers the files the architecture names as the ones a
// tag scheme gets wrong: identical adjacent lines, a one-line file, a file ending
// without a newline, and content containing the characters an address uses.
func TestTagOnAdversarialShapes(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		variant string
	}{
		{
			name:    "identical adjacent lines",
			base:    "same\nsame\n",
			variant: "same\nsame\nsame\n",
		},
		{
			name:    "one line, with and without a trailing newline",
			base:    "only",
			variant: "only\n",
		},
		{
			name:    "hash in the content",
			base:    "# heading\n",
			variant: "## heading\n",
		},
		{
			name:    "address-shaped content",
			base:    "see [file.go#1A2B]\n",
			variant: "see [file.go#1A2C]\n",
		},
		{
			name:    "blank line added",
			base:    "a\nb\n",
			variant: "a\n\nb\n",
		},
		{
			name:    "lines reordered",
			base:    "a\nb\n",
			variant: "b\na\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(Tag(c.base)).NotTo(Equal(Tag(c.variant)), "distinct content, same tag")
		})
	}
}

// TestNormalizeIsIdempotent matters because the tag is recomputed on every resolve.
// If normalising twice differed from normalising once, a file would stop matching
// its own anchor after a round trip through the renderer.
func TestNormalizeIsIdempotent(t *testing.T) {
	inputs := []string{
		"",
		"a",
		"a\r\n b \r\n",
		"trailing\t\t\n",
		"no newline at end   ",
		strings.Repeat("x   \r\n", 40),
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			g := NewWithT(t)
			once := Normalize(in)
			g.Expect(Normalize(once)).To(Equal(once), "Normalize(%q) is not idempotent", in)
		})
	}
}
