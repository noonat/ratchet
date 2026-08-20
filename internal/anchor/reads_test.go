package anchor

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestReadsKeepsTheLatest records the choice, which is not obvious. An earlier tag
// was served legitimately, so refusing it looks harsh; honoring it is worse. If the
// file changed after the second read, applying against the first snapshot writes over
// content the model was later shown differently, and if it did not change, both reads
// produced the same string anyway.
func TestReadsKeepsTheLatest(t *testing.T) {
	g := NewWithT(t)
	reads := NewReads()
	superseded := NewSnapshot("one\n")
	latest := NewSnapshot("one\ntwo\n")

	reads.Record("a/b.ts", superseded)
	reads.Record("a/b.ts", latest)

	got, ok := reads.Snapshot("a/b.ts")
	g.Expect(ok).To(BeTrue())
	g.Expect(got.Tag).To(Equal(latest.Tag))
	g.Expect(got.Tag).NotTo(Equal(superseded.Tag), "the fixture has to distinguish the two reads")
}

func TestReadsHasNothingForAnUnreadPath(t *testing.T) {
	g := NewWithT(t)
	reads := NewReads()
	reads.Record("a/b.ts", NewSnapshot("one\n"))

	_, ok := reads.Snapshot("a/other.ts")

	g.Expect(ok).To(BeFalse(), "provenance is per path: a read of one file issues nothing for another")
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
			reads.Record(c.recorded, NewSnapshot("one\n"))

			_, ok := reads.Snapshot(c.asked)

			g.Expect(ok).To(BeTrue())
		})
	}
}
