package patch

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestRowNeedsNoEscape is the property the format rests on, and the reason the
// "doubling rule" name was not adopted: a round trip through Row and Parse returns
// the content unchanged, including content that opens with a sigil.
func TestRowNeedsNoEscape(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "ordinary code",
			content: "const n = 1;",
		},
		{
			name:    "leading whitespace",
			content: "    return 1",
		},
		{
			name:    "opens with a dash",
			content: "- item",
		},
		{
			name:    "opens with a plus",
			content: "+ added",
		},
		{
			name:    "opens with two dashes",
			content: "-- deeper",
		},
		{
			name:    "a whole diff-looking line",
			content: "-const n = 1;",
		},
		{
			name:    "empty",
			content: "",
		},
		{
			name:    "trailing whitespace, which a markdown line break depends on",
			content: "a line ending in two spaces  ",
		},
		{
			name:    "trailing tab",
			content: "indented\t",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			reply := "[a/b.ts#1A2B]\nPUT 3.=3:\n" + Row(Minus, c.content) + "\n" + Row(Plus, c.content+" (edited)")

			p, err := Parse(reply)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(p.Hunks[0].Old).
				To(Equal([]string{c.content}), "content must survive the round trip unchanged")
			g.Expect(p.Hunks[0].New).To(Equal([]string{c.content + " (edited)"}))
		})
	}
}

func TestReindentRefusesWhereWhitespaceIsSyntax(t *testing.T) {
	cases := []struct {
		name        string
		lang        string
		original    string
		replacement string
		want        string
		repaired    bool
	}{
		{
			name:        "javascript, indentation restored",
			lang:        "js",
			original:    "    return 1;",
			replacement: "return 2;",
			want:        "    return 2;",
			repaired:    true,
		},
		{
			name:        "tabs restored as tabs",
			lang:        "go",
			original:    "\t\treturn 1",
			replacement: "return 2",
			want:        "\t\treturn 2",
			repaired:    true,
		},
		{
			name:        "python, refused because indentation is syntax",
			lang:        "py",
			original:    "    return 1",
			replacement: "return 2",
			want:        "return 2",
			repaired:    false,
		},
		{
			name:        "yaml, refused for the same reason",
			lang:        "yaml",
			original:    "  key: 1",
			replacement: "key: 2",
			want:        "key: 2",
			repaired:    false,
		},
		{
			name:        "replacement already indented, left alone",
			lang:        "js",
			original:    "    return 1;",
			replacement: "  return 2;",
			want:        "  return 2;",
			repaired:    false,
		},
		{
			name:        "original has no indentation, nothing to copy",
			lang:        "js",
			original:    "return 1;",
			replacement: "return 2;",
			want:        "return 2;",
			repaired:    false,
		},
		{
			name:        "empty replacement is not indented into existence",
			lang:        "js",
			original:    "    return 1;",
			replacement: "",
			want:        "",
			repaired:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, repaired := Reindent(c.lang, c.original, c.replacement)

			g.Expect(repaired).To(Equal(c.repaired))
			g.Expect(got).To(Equal(c.want))
		})
	}
}

// TestIndentSensitiveErrsTowardRefusing records the direction the list is wrong in.
// A language wrongly listed forgoes a repair; a language wrongly omitted breaks a
// file silently, so anything uncertain belongs on the list.
func TestIndentSensitiveErrsTowardRefusing(t *testing.T) {
	cases := []struct {
		lang string
		want bool
	}{
		{lang: "py", want: true},
		{lang: "python", want: true},
		{lang: ".py", want: true},
		{lang: "PY", want: true},
		{lang: "yaml", want: true},
		{lang: "yml", want: true},
		{lang: "hs", want: true},
		{lang: "fs", want: true},
		{lang: "md", want: true},
		{lang: "pug", want: true},
		{lang: "slim", want: true},
		{lang: "styl", want: true},
		{lang: "cson", want: true},
		{lang: "js", want: false},
		{lang: "go", want: false},
		{lang: "ts", want: false},
		{lang: "", want: false},
	}

	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			NewWithT(t).Expect(IndentSensitive(c.lang)).To(Equal(c.want))
		})
	}
}
