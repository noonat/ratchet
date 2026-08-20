package anchor

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestReadsKeepsTheLatest records the choice, which is not obvious. An earlier tag
// was issued legitimately, so refusing it looks harsh; honoring it is worse. If the
// file changed after the second read, applying against the first snapshot writes over
// content the model was later shown differently, and if it did not change, both reads
// minted the same string anyway.
func TestReadsKeepsTheLatest(t *testing.T) {
	g := NewWithT(t)
	reads := NewReads()
	first := MintAll("one\n")
	second := MintAll("one\ntwo\n")

	reads.Record("a/b.ts", first)
	reads.Record("a/b.ts", second)

	got, issued := reads.Issued("a/b.ts")
	g.Expect(issued).To(BeTrue())
	g.Expect(got.Tag).To(Equal(second.Tag))
	g.Expect(got.Tag).NotTo(Equal(first.Tag), "the fixture has to distinguish the two reads")
}

func TestReadsHasNothingForAnUnreadPath(t *testing.T) {
	g := NewWithT(t)
	reads := NewReads()
	reads.Record("a/b.ts", MintAll("one\n"))

	_, issued := reads.Issued("a/other.ts")

	g.Expect(issued).To(BeFalse(), "provenance is per path: a read of one file issues nothing for another")
}

// TestReadsCleansThePathKey stops a lookup miss that reads as a missing read. A
// caller recording `./src/a.ts` while the render header shows `src/a.ts` would
// otherwise get a refusal telling the model to read a file it just read.
func TestReadsCleansThePathKey(t *testing.T) {
	cases := []struct {
		name     string
		recorded string
		asked    string
	}{
		{
			name:     "an already clean path is unchanged",
			recorded: "src/a.ts",
			asked:    "src/a.ts",
		},
		{
			name:     "a leading dot-slash",
			recorded: "./src/a.ts",
			asked:    "src/a.ts",
		},
		{
			name:     "a doubled separator",
			recorded: "src//a.ts",
			asked:    "src/a.ts",
		},
		{
			name:     "a traversal that cancels out",
			recorded: "src/../src/a.ts",
			asked:    "src/a.ts",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			reads := NewReads()
			reads.Record(c.recorded, MintAll("one\n"))

			_, issued := reads.Issued(c.asked)

			g.Expect(issued).To(BeTrue())
		})
	}
}
