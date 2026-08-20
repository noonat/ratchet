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
			reply := "[a/b.ts#1A2B]\nPUT 3.=3:\n" + Row(SigilMinus, c.content) + "\n" + Row(SigilPlus, c.content+" (edited)")

			p, err := Parse(reply)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(p.Hunks[0].Old).
				To(Equal([]string{c.content}), "content must survive the round trip unchanged")
			g.Expect(p.Hunks[0].New).To(Equal([]string{c.content + " (edited)"}))
		})
	}
}
