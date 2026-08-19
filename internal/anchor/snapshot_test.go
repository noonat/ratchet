package anchor

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestMintRecordsOnlyTheLinesDisplayed is the property the Lines field exists for.
// A windowed read stamps a tag for the whole file while showing part of it, and an
// edit to a line nobody saw is unreviewed however well-formed the anchor is.
func TestMintRecordsOnlyTheLinesDisplayed(t *testing.T) {
	g := NewWithT(t)
	text := "one\ntwo\nthree\nfour\nfive\n"
	s := Mint(text, []int{2, 3, 4})

	g.Expect(s.Tag).To(Equal(Tag(text)), "the tag must cover the text served")
	g.Expect(s.Text).To(Equal(text), "the served text is what a mismatch is attributed against")

	shown := []int{2, 3, 4}
	for _, n := range shown {
		g.Expect(s.Shows(n)).To(BeTrue(), "line %d was displayed", n)
	}

	hidden := []int{1, 5, 6, 0, -1}
	for _, n := range hidden {
		g.Expect(s.Shows(n)).To(BeFalse(), "line %d was never displayed", n)
	}
}

// TestMintAllCountsRealLines guards an off-by-one that would refuse a legitimate
// edit to the last line, or accept one to a line that does not exist. A trailing
// newline is not a line anyone saw.
func TestMintAllCountsRealLines(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		lines int
	}{
		{
			name:  "trailing newline",
			text:  "one\ntwo\nthree\n",
			lines: 3,
		},
		{
			name:  "no trailing newline",
			text:  "one\ntwo\nthree",
			lines: 3,
		},
		{
			name:  "single line",
			text:  "only\n",
			lines: 1,
		},
		{
			name:  "single line without a newline",
			text:  "only",
			lines: 1,
		},
		{
			name:  "empty file",
			text:  "",
			lines: 0,
		},
		{
			name:  "blank line in the middle",
			text:  "a\n\nb\n",
			lines: 3,
		},
		{
			name:  "file of one blank line",
			text:  "\n",
			lines: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := MintAll(c.text)

			g.Expect(s.Lines).To(HaveLen(c.lines), "text %q", c.text)
			if c.lines > 0 {
				g.Expect(s.Shows(c.lines)).To(BeTrue(), "last line %d", c.lines)
			}
			g.Expect(s.Shows(c.lines+1)).To(BeFalse(), "line %d does not exist", c.lines+1)
			g.Expect(s.Shows(0)).To(BeFalse(), "lines are 1-indexed")
		})
	}
}

// TestMintAllShowsEveryLineOfAdversarialFiles pairs with the tag tests: the shapes
// that break a line counter are the same ones that break a hash. Identical adjacent
// lines are the case a set keyed on content would collapse, which is why Lines is
// keyed on number.
func TestMintAllShowsEveryLineOfAdversarialFiles(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		lines int
	}{
		{
			name:  "identical adjacent lines",
			text:  "same\nsame\nsame\n",
			lines: 3,
		},
		{
			name:  "every line blank",
			text:  "\n\n\n",
			lines: 3,
		},
		{
			name:  "hash in the content",
			text:  "# one\n## two\n",
			lines: 2,
		},
		{
			name:  "windows line endings",
			text:  "a\r\nb\r\nc\r\n",
			lines: 3,
		},
		{
			name:  "no trailing newline",
			text:  "a\nb",
			lines: 2,
		},
		{
			name:  "last line is indentation, with no trailing newline",
			text:  "a\n    ",
			lines: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := MintAll(c.text)

			g.Expect(s.Lines).To(HaveLen(c.lines), "text %q", c.text)
			for n := 1; n <= c.lines; n++ {
				g.Expect(s.Shows(n)).To(BeTrue(), "line %d", n)
			}
			g.Expect(s.Shows(c.lines+1)).To(BeFalse(), "line %d does not exist", c.lines+1)
		})
	}
}

// TestSnapshotDetectsAChangedFile is the input to the refusal branch. The resolver
// has to tell "the model mistyped the tag" from "the file changed", and it can only
// do that by comparing against the text the read served.
func TestSnapshotDetectsAChangedFile(t *testing.T) {
	g := NewWithT(t)
	served := "alpha\nbeta\n"
	s := MintAll(served)

	g.Expect(s.Text).To(Equal(served))
	g.Expect(Tag(s.Text)).To(Equal(s.Tag), "a snapshot must agree with itself")

	g.Expect(Tag("alpha\nbetaRenamed\n")).
		NotTo(Equal(s.Tag), "the file moved and the tag must say so")
	g.Expect(Tag("alpha   \nbeta\t\n")).
		To(Equal(s.Tag), "trailing blanks are not a change and must not read as one")
}
